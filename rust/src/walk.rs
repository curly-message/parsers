use std::cell::Cell;

use crate::builtins::{Builtin, Selected};
use crate::convert::{Converted, Record, Txt};
use crate::depth::{Activation, host_call};
use crate::parser::Config;
use crate::report::{Code, Report};
use crate::scan::Scanner;
use crate::syntax::{self, unescape};
use crate::value::{Object, Value};
use crate::{MAX_NESTING, MAX_OUTPUT_LENGTH, MAX_READ_LENGTH};

// Reserved by the payload for a value's own configuration (section 4.1).
const WRAPPED: [&str; 3] = ["value", "default", "props"];

/// One resolution: the message, what it reads and what it has spent.
pub(crate) struct Walker<'a> {
    config: &'a Config,
    // One scanner for the walk: a verdict is final, so the span an opening
    // brace derives is settled once however often the walk asks for it.
    scan: Scanner<'a>,
    payload: Option<&'a Object>,
    id: Option<&'a str>,
    // What the output carries and what the payload is read for are budgets of
    // the resolution rather than of a placeholder: what one placeholder
    // spends is gone for every placeholder after it (section 13).
    output: Cell<usize>,
    read: Cell<isize>,
    record: Record,
}

/// How a placeholder resolved: to text, or to a span of the message to be
/// rendered one level deeper in its place.
enum Outcome<'a> {
    Done(Txt<'a>),
    Inline(usize, usize),
}

// A span being rendered: the text the walk has made of it so far, and the
// placeholder it stands for, where it is a default or an option the walk
// renders in that placeholder's place.
struct Frame {
    to: usize,
    depth: usize,
    at: usize,
    out: String,
    placeholder: Option<(usize, usize)>,
}

// What a placeholder's fallback chain reads (section 10).
struct Chain<'a> {
    open: usize,
    close: usize,
    wrapper: Option<&'a Object>,
    names_default: bool,
    inline_default: Option<(usize, usize)>,
}

fn is_wrapper(object: &Object) -> bool {
    !object.is_empty() && object.iter().all(|(key, _)| WRAPPED.contains(&key))
}

fn push_text(out: &mut String, text: &str, from: usize, to: usize) {
    out.push_str(&unescape(text.get(from..to).unwrap_or_default()));
}

impl<'a> Walker<'a> {
    pub(crate) fn new(
        config: &'a Config,
        message: &'a str,
        payload: Option<&'a Object>,
        id: Option<&'a str>,
        record: Record,
    ) -> Self {
        Self {
            config,
            scan: Scanner::new(message),
            payload,
            id,
            output: Cell::new(MAX_OUTPUT_LENGTH),
            read: Cell::new(isize::try_from(MAX_READ_LENGTH).unwrap_or(isize::MAX)),
            record,
        }
    }

    /// A span of the message: the characters it writes itself, with their
    /// escape sequences removed, and the placeholders it writes resolved. The
    /// whole message is one such span.
    ///
    /// A default or an option value standing in for a placeholder is rendered
    /// on a stack of the walk's own rather than by a call per level, so how
    /// deeply a message nests never reaches the host's call stack. Nothing,
    /// where this thread's stack holds no room for another walk.
    pub(crate) fn render(&self, from: usize, to: usize, depth: usize) -> Option<String> {
        let _activation = Activation::enter()?;
        let text = self.scan.text();
        let mut stack = vec![Frame {
            to,
            depth,
            at: from,
            out: String::new(),
            placeholder: None,
        }];

        while let Some(top) = stack.last_mut() {
            let Some((open, close)) = self.scan.next(top.at, top.to) else {
                push_text(&mut top.out, text, top.at, top.to);

                let done = stack.pop()?;

                match (stack.last_mut(), done.placeholder) {
                    (Some(parent), Some((open, close))) => {
                        self.deliver(parent, open, close, &done.out)
                    }
                    _ => return Some(done.out),
                }

                continue;
            };

            push_text(&mut top.out, text, top.at, open);
            top.at = close;

            let depth = top.depth;

            match self.placeholder(open, close, depth) {
                Outcome::Done(resolved) => {
                    if let Some(top) = stack.last_mut() {
                        self.deliver(top, open, close, &resolved);
                    }
                }
                Outcome::Inline(from, to) => stack.push(Frame {
                    to,
                    depth: depth.saturating_add(1),
                    at: from,
                    out: String::new(),
                    placeholder: Some((open, close)),
                }),
            }
        }

        None
    }

    // Only what the output carries is counted, and a result nested in another
    // reaches the output through the one around it, so the charge is made
    // where the walk meets the message itself and a result is counted once.
    // The message's own text is the caller's and always renders.
    fn deliver(&self, frame: &mut Frame, open: usize, close: usize, resolved: &str) {
        if frame.depth > 1 {
            frame.out.push_str(resolved);

            return;
        }

        let left = self.output.get();

        match left.checked_sub(resolved.len()) {
            Some(rest) => {
                self.output.set(rest);
                frame.out.push_str(resolved);
            }
            None => self.report(Code::OutputLimit, open, close),
        }
    }

    // The steps of one placeholder, in the order their reports are observable.
    fn placeholder(&self, open: usize, close: usize, depth: usize) -> Outcome<'a> {
        // The budget is tested before this placeholder reads, so one that found
        // budget and spent it past the limit resolved all the same, and this is
        // the one that pays for it.
        if self.read.get() < 0 {
            self.report(Code::ReadLimit, open, close);

            return Outcome::Done(Txt::Borrowed(""));
        }

        let declared = syntax::placeholder(&self.scan, open, close);
        let key = declared.key.as_deref();
        let entry = key.and_then(|key| self.payload?.get(key));
        // The payload's root `default` is the fallback itself, never
        // configuration, and a caller that turned recognition off passes
        // entries it did not write: an entry shaped like a wrapper is a value
        // then.
        let wrapper = match entry {
            Some(Value::Object(object))
                if self.config.recognize_wrappers
                    && key != Some("default")
                    && is_wrapper(object) =>
            {
                Some(&**object)
            }
            _ => None,
        };
        let value = match wrapper {
            Some(wrapper) => wrapper.get("value"),
            None => entry,
        };
        let value_text = value.and_then(|value| self.payload_text(value, open, close));
        let chain = Chain {
            open,
            close,
            wrapper,
            names_default: key == Some("default"),
            inline_default: declared.inline_default,
        };

        // The nesting limit bounds resolution and not derivation: what it
        // refuses is resolving this placeholder, which takes its chain.
        if depth > MAX_NESTING {
            self.report(Code::NestingLimit, open, close);

            return self.chain(&chain, value_text);
        }

        let modifier = if declared.modifier.is_empty() {
            None
        } else {
            // A modifier nobody registered is a defect in the message, not a
            // comparison to run in its place.
            let Some(modifier) = self.modifier_for(&declared.modifier) else {
                self.report(Code::UnknownModifier, open, close);

                return self.chain(&chain, value_text);
            };

            Some(modifier)
        };

        // A selection declaring no options was asked to select from nothing.
        // How the placeholder is written says so, so the report does not wait
        // on the value. A placeholder naming no key has nothing to compare and
        // is no selection.
        if key.is_some()
            && declared.options.is_empty()
            && modifier.is_some_and(Builtin::is_selection)
        {
            self.report(Code::MissingOptions, open, close);

            return self.chain(&chain, value_text);
        }

        // An absent value is nothing to compare against, whatever the modifier
        // asks.
        let Some(value_text) = value_text else {
            return self.chain(&chain, None);
        };

        if modifier.is_none() && declared.options.is_empty() {
            return Outcome::Done(value_text);
        }

        let Some(modifier) = modifier.or_else(|| self.modifier_for("eq")) else {
            return self.chain(&chain, Some(value_text));
        };

        match modifier.select(&value_text, &declared.options) {
            Selected::Option(at) => match declared.options.get(at) {
                Some(option) => Outcome::Inline(option.value.0, option.value.1),
                None => self.chain(&chain, Some(value_text)),
            },
            Selected::Default => self.chain(&chain, Some(value_text)),
        }
    }

    fn modifier_for(&self, name: &str) -> Option<Builtin> {
        Builtin::named(name)
    }

    // The wrapper speaks for its own value, the payload for every key it does
    // not carry, and the message only for what neither says. A link waits for
    // the one before it to come back empty, since converting one can cost a
    // full serialization. A placeholder naming `default` itself has read the
    // payload's link already, as its value: one entry is read once.
    fn chain(&self, chain: &Chain<'a>, value_text: Option<Txt<'a>>) -> Outcome<'a> {
        let wrapped = chain
            .wrapper
            .and_then(|wrapper| wrapper.get("default"))
            .and_then(|value| self.payload_text(value, chain.open, chain.close));

        if let Some(text) = wrapped {
            return Outcome::Done(text);
        }

        let payload = if chain.names_default {
            value_text
        } else {
            self.payload
                .and_then(|payload| payload.get("default"))
                .and_then(|value| self.payload_text(value, chain.open, chain.close))
        };

        if let Some(text) = payload {
            return Outcome::Done(text);
        }

        match chain.inline_default {
            Some((from, to)) => Outcome::Inline(from, to),
            None => Outcome::Done(Txt::Borrowed("")),
        }
    }

    // Value text is what a placeholder takes from the payload, and every byte
    // of it is spent whether or not any of it reaches the output. A value
    // nobody passed is no defect; a value that cannot become text is, and
    // each link that finds one is a defect of its own.
    fn payload_text(&self, value: &'a Value, open: usize, close: usize) -> Option<Txt<'a>> {
        match self.record.convert(value) {
            Converted::Absent => None,
            Converted::Undescribable => {
                self.report(Code::UnserializableValue, open, close);

                None
            }
            Converted::Text(text) => {
                let spent = isize::try_from(text.len()).unwrap_or(isize::MAX);

                self.read.set(self.read.get().saturating_sub(spent));

                Some(text)
            }
        }
    }

    // Reporting is an observation, not a step of the resolution: a handler
    // that fails still gets its message back.
    fn report(&self, code: Code, open: usize, close: usize) {
        let Some(handler) = &self.config.on_report else {
            return;
        };
        let spelling = self.scan.text().get(open..close).unwrap_or_default();
        let report = Report::new(code, spelling, self.id);

        let _ = host_call(|| handler(&report));
    }
}
