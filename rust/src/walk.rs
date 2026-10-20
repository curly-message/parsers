use std::borrow::Cow;
use std::cell::{Cell, OnceCell};

use crate::builtins::{Builtin, Selected};
use crate::convert::{Converted, Record, Txt};
use crate::depth::{Activation, host_call};
use crate::formatting::{Asked, Formatting};
use crate::modifier::{Modifier, ModifierInput};
use crate::parser::{Config, Context};
use crate::props::{Layers, Props};
use crate::report::{Code, Report};
use crate::scan::Scanner;
use crate::syntax::{self, Segment, unescape};
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
    props: Option<&'a Object>,
    locale: Option<&'a str>,
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

/// What reading a placeholder came to: its outcome, or a host's modifier or
/// a formatting modifier to call for it. The call is made from the walk's
/// loop, so the frame that read the placeholder is gone from the stack while
/// the host's code holds it.
enum Step<'a> {
    Settled(Outcome<'a>),
    Call(&'a dyn Modifier, Box<Ph<'a>>),
    Intl(Formatting, Box<Ph<'a>>),
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

/// One placeholder as the walk resolves it: what it declares, what it read,
/// and what a modifier has asked of it so far. A modifier reaches it through
/// a shared borrow, so each memo is a cell filled once.
pub(crate) struct Ph<'a> {
    open: usize,
    close: usize,
    depth: usize,
    names_default: bool,
    // The name the modifier answers to, `eq` where the placeholder names
    // none: the name its props are composed under.
    name: Cow<'a, str>,
    options: Vec<Segment<'a>>,
    rendered: Vec<OnceCell<Box<str>>>,
    inline_default: Option<(usize, usize)>,
    wrapper: Option<&'a Object>,
    value_text: Option<Txt<'a>>,
    // The chain's payload links, read at most once: what they came to, or
    // nothing where neither holds text.
    links: OnceCell<Option<Box<str>>>,
    // The inline default, rendered for a modifier that asked for it.
    default: OnceCell<Box<str>>,
}

impl<'a> Ph<'a> {
    pub(crate) fn value_text(&self) -> Option<&str> {
        self.value_text.as_deref()
    }

    pub(crate) fn options(&self) -> &[Segment<'a>] {
        &self.options
    }
}

/// The modifier a name answers to: one the format defines, or one the host
/// registered, which replaces the format's under the same name.
#[derive(Clone, Copy)]
enum Resolved<'c> {
    Builtin(Builtin),
    Intl(Formatting),
    Host(&'c dyn Modifier),
}

impl Resolved<'_> {
    fn is_selection(self) -> bool {
        match self {
            Self::Builtin(builtin) => builtin.is_selection(),
            Self::Intl(formatting) => formatting.is_selection(),
            Self::Host(_) => false,
        }
    }
}

// Text an answer converted to, held past the answer itself.
fn detached(text: Txt<'_>) -> Txt<'static> {
    match text {
        Txt::Borrowed(text) => Txt::Owned(text.to_owned()),
        Txt::Shared(text) => Txt::Shared(text),
        Txt::Owned(text) => Txt::Owned(text),
    }
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
        context: Context<'a>,
        record: Record,
    ) -> Self {
        Self {
            config,
            scan: Scanner::new(message),
            payload: context.payload,
            props: context.props,
            locale: context.locale,
            id: context.id,
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

            let outcome = match self.placeholder(open, close, depth) {
                Step::Settled(outcome) => outcome,
                Step::Call(modifier, ph) => self.call(modifier, ph),
                Step::Intl(formatting, ph) => self.intl(formatting, *ph),
            };

            match outcome {
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
    fn placeholder(&self, open: usize, close: usize, depth: usize) -> Step<'a> {
        // The budget is tested before this placeholder reads, so one that found
        // budget and spent it past the limit resolved all the same, and this is
        // the one that pays for it.
        if self.read.get() < 0 {
            self.report(Code::ReadLimit, open, close);

            return Step::Settled(Outcome::Done(Txt::Borrowed("")));
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
        let names_default = key == Some("default");
        let names_key = key.is_some();
        let mut ph = Ph {
            open,
            close,
            depth,
            names_default,
            name: declared.modifier,
            options: declared.options,
            rendered: Vec::new(),
            inline_default: declared.inline_default,
            wrapper,
            value_text,
            links: OnceCell::new(),
            default: OnceCell::new(),
        };

        // The nesting limit bounds resolution and not derivation: what it
        // refuses is resolving this placeholder, which takes its chain.
        if depth > MAX_NESTING {
            self.report(Code::NestingLimit, open, close);

            return Step::Settled(self.chain(ph));
        }

        let modifier = if ph.name.is_empty() {
            None
        } else {
            // A modifier nobody registered is a defect in the message, not a
            // comparison to run in its place.
            let Some(modifier) = self.modifier_for(&ph.name) else {
                self.report(Code::UnknownModifier, open, close);

                return Step::Settled(self.chain(ph));
            };

            Some(modifier)
        };

        // A selection declaring no options was asked to select from nothing.
        // How the placeholder is written says so, so the report does not wait
        // on the value. A placeholder naming no key has nothing to compare and
        // is no selection, and neither is a host's modifier registered under a
        // selection's name: it replaced the format's.
        if names_key && ph.options.is_empty() && modifier.is_some_and(Resolved::is_selection) {
            self.report(Code::MissingOptions, open, close);

            return Step::Settled(self.chain(ph));
        }

        // An absent value is nothing to compare against, whatever the modifier
        // asks.
        if ph.value_text.is_none() {
            return Step::Settled(self.chain(ph));
        }

        if modifier.is_none() && ph.options.is_empty() {
            return Step::Settled(Outcome::Done(ph.value_text.unwrap_or(Txt::Borrowed(""))));
        }

        let modifier = match modifier {
            Some(modifier) => modifier,
            None => {
                ph.name = Cow::Borrowed("eq");

                match self.modifier_for("eq") {
                    Some(modifier) => modifier,
                    None => return Step::Settled(self.chain(ph)),
                }
            }
        };

        match modifier {
            Resolved::Builtin(builtin) => {
                let value = ph.value_text.as_deref().unwrap_or_default();

                Step::Settled(match builtin.select(value, &ph.options) {
                    Selected::Option(at) => match ph.options.get(at) {
                        Some(option) => Outcome::Inline(option.value.0, option.value.1),
                        None => self.chain(ph),
                    },
                    Selected::Default => self.chain(ph),
                })
            }
            Resolved::Intl(formatting) => Step::Intl(formatting, Box::new(ph)),
            Resolved::Host(modifier) => Step::Call(modifier, Box::new(ph)),
        }
    }

    // The formatting modifiers are known only where a facility was configured
    // to delegate them to.
    fn modifier_for(&self, name: &str) -> Option<Resolved<'a>> {
        if let Some(modifier) = self.config.modifiers.get(name) {
            return Some(Resolved::Host(&**modifier));
        }

        if let Some(builtin) = Builtin::named(name) {
            return Some(Resolved::Builtin(builtin));
        }

        self.config
            .intl
            .as_ref()
            .and(Formatting::named(name))
            .map(Resolved::Intl)
    }

    // The steps of sections 11.2 and 11.5 in their order: the locale, then
    // what the modifier tests itself, then the request. The facility is host
    // code, so how it fails and whether it came back are the placeholder's to
    // settle. A locale nobody supplied is the one failure the chain does not
    // answer: a default stands in for a value, never for the locale it would
    // have been formatted in.
    fn intl(&self, formatting: Formatting, ph: Ph<'a>) -> Outcome<'a> {
        let Some(facility) = self.config.intl.as_deref() else {
            return self.chain(ph);
        };
        let Some(locale) = self.locale.filter(|locale| !locale.is_empty()) else {
            self.report(Code::MissingLocale, ph.open, ph.close);

            return Outcome::Done(Txt::Borrowed(""));
        };
        let value = ph.value_text.as_deref().unwrap_or_default();
        let input = match formatting.input(value, &ph.options) {
            Ok(input) => input,
            Err(Asked::Option(at)) => return self.option(ph, at),
            Err(_) => return self.failed(ph),
        };
        let mut configured = formatting.configure(&self.layers(&ph));
        let answer = match formatting.ask(input, &mut configured) {
            Asked::Format => Some(
                host_call(|| facility.format(locale, &configured.request))
                    .and_then(Result::ok)
                    .map(Ok),
            ),
            Asked::Select => Some(
                host_call(|| facility.select(locale, &configured.request))
                    .and_then(Result::ok)
                    .map(Err),
            ),
            Asked::Option(_) | Asked::Failed => None,
        };

        match answer {
            Some(Some(Ok(text))) => Outcome::Done(Txt::Owned(text)),
            Some(Some(Err(category))) => {
                let category = category.as_str();

                match ph.options.iter().position(|option| option.key == category) {
                    Some(at) => self.option(ph, at),
                    None => self.chain(ph),
                }
            }
            Some(None) | None => self.failed(ph),
        }
    }

    fn option(&self, ph: Ph<'a>, at: usize) -> Outcome<'a> {
        match ph.options.get(at) {
            Some(option) => Outcome::Inline(option.value.0, option.value.1),
            None => self.chain(ph),
        }
    }

    fn failed(&self, ph: Ph<'a>) -> Outcome<'a> {
        self.report(Code::FailedModifier, ph.open, ph.close);

        self.chain(ph)
    }

    // A modifier is host code: what it answers, how it fails and whether it
    // came back at all are each the placeholder's to settle, and none of them
    // leaves the resolution.
    fn call(&self, modifier: &dyn Modifier, mut ph: Box<Ph<'a>>) -> Outcome<'a> {
        ph.rendered = ph.options.iter().map(|_| OnceCell::new()).collect();

        let answer = host_call(|| modifier.call(&ModifierInput::new(self, &ph)));

        match answer {
            Some(Ok(Some(value))) => self.answer(value, *ph),
            Some(Ok(None)) => self.chain(*ph),
            Some(Err(_)) | None => {
                self.report(Code::FailedModifier, ph.open, ph.close);

                self.chain(*ph)
            }
        }
    }

    // An answer converts as a payload entry does, so a structure a modifier
    // built stays structured. It is this placeholder's result rather than
    // text it read, so it is bounded by the output and spends nothing of the
    // read budget.
    fn answer(&self, value: Value, ph: Ph<'a>) -> Outcome<'a> {
        if let Value::String(text) = value {
            return Outcome::Done(Txt::Owned(text));
        }

        match self.record.convert(&value) {
            Converted::Text(text) => Outcome::Done(detached(text)),
            Converted::Absent => self.chain(ph),
            Converted::Undescribable => {
                self.report(Code::UnserializableValue, ph.open, ph.close);

                self.chain(ph)
            }
        }
    }

    // The wrapper speaks for its own value, the payload for every key it does
    // not carry, and the message only for what neither says. A link waits for
    // the one before it to come back empty, since converting one can cost a
    // full serialization. A placeholder naming `default` itself has read the
    // payload's link already, as its value: one entry is read once.
    fn links(&self, ph: &Ph<'a>, value_text: Option<Txt<'a>>) -> Option<Txt<'a>> {
        let wrapped = ph
            .wrapper
            .and_then(|wrapper| wrapper.get("default"))
            .and_then(|value| self.payload_text(value, ph.open, ph.close));

        if wrapped.is_some() {
            return wrapped;
        }

        if ph.names_default {
            return value_text;
        }

        self.payload
            .and_then(|payload| payload.get("default"))
            .and_then(|value| self.payload_text(value, ph.open, ph.close))
    }

    // The chain as the walk takes it, after whatever a modifier asked of it:
    // what the modifier was given is what the placeholder resolves to, and
    // an inline default nobody rendered is rendered on the walk's own stack.
    fn chain(&self, mut ph: Ph<'a>) -> Outcome<'a> {
        match ph.links.take() {
            Some(Some(text)) => return Outcome::Done(Txt::Owned(text.into_string())),
            Some(None) => {}
            None => {
                let value_text = ph.value_text.take();

                if let Some(text) = self.links(&ph, value_text) {
                    return Outcome::Done(text);
                }
            }
        }

        if let Some(text) = ph.default.take() {
            return Outcome::Done(Txt::Owned(text.into_string()));
        }

        match ph.inline_default {
            Some((from, to)) => Outcome::Inline(from, to),
            None => Outcome::Done(Txt::Borrowed("")),
        }
    }

    /// An option's value for a modifier, rendered on the first ask. A render
    /// this thread had no room for answers the empty string and is not kept:
    /// the modifier's call is answered as one that did not come back.
    pub(crate) fn option_value<'p>(&self, ph: &'p Ph<'_>, at: usize) -> Option<&'p str> {
        let option = ph.options.get(at)?;
        let cell = ph.rendered.get(at)?;

        if let Some(text) = cell.get() {
            return Some(text);
        }

        let Some(text) = self.render(option.value.0, option.value.1, ph.depth.saturating_add(1))
        else {
            return Some("");
        };
        let _ = cell.set(text.into_boxed_str());

        Some(cell.get().map_or("", |text| text))
    }

    /// The chain for a modifier, walked on the first ask.
    pub(crate) fn default_text<'p>(&self, ph: &'p Ph<'a>) -> &'p str {
        const NOTHING: &Option<Box<str>> = &None;

        let links = match ph.links.get() {
            Some(links) => links,
            None => {
                let value_text = if ph.names_default {
                    ph.value_text.clone()
                } else {
                    None
                };
                let found = self.links(ph, value_text).map(Box::from);
                let _ = ph.links.set(found);

                ph.links.get().unwrap_or(NOTHING)
            }
        };

        if let Some(text) = links {
            return text;
        }

        if let Some(text) = ph.default.get() {
            return text;
        }

        let text = match ph.inline_default {
            Some((from, to)) => match self.render(from, to, ph.depth.saturating_add(1)) {
                Some(text) => text.into_boxed_str(),
                None => return "",
            },
            None => Box::from(""),
        };
        let _ = ph.default.set(text);

        ph.default.get().map_or("", |text| text)
    }

    /// The props the layers hold under the modifier's name (section 11.2).
    pub(crate) fn props<'p>(&self, ph: &'p Ph<'_>) -> Props<'p>
    where
        'a: 'p,
    {
        Props::layered(&self.layers(ph), &ph.name)
    }

    fn layers<'p>(&self, ph: &'p Ph<'_>) -> Layers<'p>
    where
        'a: 'p,
    {
        [
            self.config.modifier_defaults.as_ref(),
            self.props,
            ph.wrapper
                .and_then(|wrapper| wrapper.get("props"))
                .and_then(Value::as_object),
        ]
    }

    pub(crate) fn locale(&self) -> Option<&'a str> {
        self.locale
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
