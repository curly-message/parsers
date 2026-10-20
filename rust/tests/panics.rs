#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

// Every panic this binary provokes is one the crate contains, so the hook that
// would print each is silenced. The binary is its own, so the silence reaches
// no other test.

use std::fmt;
use std::sync::{Arc, Mutex, Once};

use curly_message_parser::{Context, MAX_REENTRY, Object, Parser, Report, Value};

fn quiet() {
    static QUIET: Once = Once::new();

    QUIET.call_once(|| std::panic::set_hook(Box::new(|_| {})));
}

struct Panics;

impl fmt::Display for Panics {
    fn fmt(&self, _: &mut fmt::Formatter<'_>) -> fmt::Result {
        panic!("a host value that panics");
    }
}

#[test]
fn a_value_whose_description_panics_is_one_no_conversion_describes() {
    quiet();

    let reports: Arc<Mutex<Vec<&'static str>>> = Arc::default();
    let sink = Arc::clone(&reports);
    let parser = Parser::builder()
        .on_report(move |report| sink.lock().unwrap().push(report.code.as_str()))
        .build();
    let payload = Object::new()
        .with("p", Value::host(Panics))
        .with("inside", Object::new().with("p", Value::host(Panics)));
    let context = Context::new().payload(&payload);

    assert_eq!(
        parser.resolve("a{{p; default:D}}b{{inside; default:E}}", &context),
        "aDbE"
    );
    assert_eq!(
        *reports.lock().unwrap(),
        ["unserializable-value", "unserializable-value"]
    );
    assert_eq!(parser.resolve(&Value::host(Panics), &context), "");
}

// What a guard resolves as it drops, and where it puts the answer.
#[derive(Clone)]
struct Guard {
    parser: Parser,
    message: &'static str,
    payload: Object,
    answer: Arc<Mutex<Option<String>>>,
}

// A guard whose drop resolves a message, as one that writes a localised line
// does.
struct ResolvesOnDrop(Guard);

impl Drop for ResolvesOnDrop {
    fn drop(&mut self) {
        let Guard {
            parser,
            message,
            payload,
            answer,
        } = &self.0;
        let text = parser.resolve(*message, &Context::new().payload(payload));

        *answer.lock().unwrap() = Some(text);
    }
}

// A value describing itself by resolving the next as a message, `levels`
// resolutions deep. The deepest holds the guard, asks for one resolution more
// where `ask` says so, then panics or, where `panics` says not, describes
// itself as `ok`.
struct Unwinds {
    levels: usize,
    ask: bool,
    panics: bool,
    guard: Guard,
}

impl fmt::Display for Unwinds {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let parser = &self.guard.parser;

        if self.levels > 1 {
            let next = Value::host(Self {
                levels: self.levels - 1,
                guard: self.guard.clone(),
                ..*self
            });
            let inner = parser.resolve(&next, &Context::new());

            return write!(formatter, "<{inner}>");
        }

        let _guard = ResolvesOnDrop(self.guard.clone());

        if self.ask {
            parser.resolve("x", &Context::new());
        }

        if self.panics {
            panic!("a host value that panics with a guard held");
        }

        formatter.write_str("ok")
    }
}

// Starts `levels` resolutions inside one another, the deepest dropping a
// guard that resolves `message` over `payload`: what the outermost answers,
// and what the guard's resolution did.
fn unwinding(
    parser: &Parser,
    levels: usize,
    ask: bool,
    panics: bool,
    message: &'static str,
    payload: Object,
) -> (String, String) {
    let answer: Arc<Mutex<Option<String>>> = Arc::default();
    let value = Value::host(Unwinds {
        levels,
        ask,
        panics,
        guard: Guard {
            parser: parser.clone(),
            message,
            payload,
            answer: Arc::clone(&answer),
        },
    });
    let text = parser.resolve(&value, &Context::new());
    let dropped = answer.lock().unwrap().take().unwrap();

    (text, dropped)
}

fn wrapped(levels: usize, inner: &str) -> String {
    format!(
        "{}{inner}{}",
        "<".repeat(levels - 1),
        ">".repeat(levels - 1)
    )
}

// A resolution asked for while the thread unwinds is refused without
// unwinding again, which would end the process: it answers the empty string.
// One fewer in progress, it resolves as ever.
#[test]
fn a_resolution_asked_for_while_the_thread_unwinds_is_refused_without_unwinding() {
    quiet();

    let parser = Parser::new();
    let resolve = |levels: usize, ask: bool| {
        let payload = Object::new().with("w", "dropped");
        let (text, dropped) = unwinding(&parser, levels, ask, true, "{{w}}", payload);

        assert_eq!(text, wrapped(levels, ""));

        dropped
    };

    // The deepest of `MAX_REENTRY` panics, or is refused the one it asks
    // for, and the guard's drop asks for one more as the thread unwinds.
    assert_eq!(resolve(MAX_REENTRY, false), "");
    assert_eq!(resolve(MAX_REENTRY, true), "");
    assert_eq!(resolve(MAX_REENTRY - 1, false), "dropped");
    assert_eq!(resolve(MAX_REENTRY, false), "");
}

// A panic payload whose drop resolves one message more.
struct AsksOnDrop {
    parser: Parser,
}

impl Drop for AsksOnDrop {
    fn drop(&mut self) {
        self.parser.resolve("x", &Context::new());
    }
}

// A value whose description panics with a payload that resolves as it drops.
struct PanicsAsking {
    parser: Parser,
}

impl fmt::Display for PanicsAsking {
    fn fmt(&self, _: &mut fmt::Formatter<'_>) -> fmt::Result {
        std::panic::panic_any(AsksOnDrop {
            parser: self.parser.clone(),
        })
    }
}

// A guard dropped while the thread unwinds, whose resolution is admitted but
// reads a value whose `Display` panics with a payload that is refused the one
// it asks for as it drops: the refusal fails that `Display`, which failed
// already, and nothing around it, so the message resolves as it does where
// nothing unwinds. One fewer in progress, nothing is refused either way.
#[test]
fn a_payload_refused_while_the_thread_unwinds_fails_only_its_host_call() {
    quiet();

    let reports: Arc<Mutex<Vec<&'static str>>> = Arc::default();
    let sink = Arc::clone(&reports);
    let parser = Parser::builder()
        .on_report(move |report| sink.lock().unwrap().push(report.code.as_str()))
        .build();
    let payload = Object::new().with(
        "p",
        Value::host(PanicsAsking {
            parser: parser.clone(),
        }),
    );
    let resolve = |levels: usize, panics: bool| {
        let (text, dropped) = unwinding(
            &parser,
            levels,
            false,
            panics,
            "a{{p; default:D}}b",
            payload.clone(),
        );

        assert_eq!(text, wrapped(levels, if panics { "" } else { "ok" }));

        (dropped, std::mem::take(&mut *reports.lock().unwrap()))
    };

    for levels in [MAX_REENTRY - 1, MAX_REENTRY - 2] {
        for panics in [true, false] {
            assert_eq!(
                resolve(levels, panics),
                ("aDb".to_owned(), vec!["unserializable-value"])
            );
        }
    }
}

// A value whose description resolves one message more, and shows it.
struct Asks {
    parser: Parser,
}

impl fmt::Display for Asks {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let inner = self.parser.resolve("x", &Context::new());

        write!(formatter, "<{inner}>")
    }
}

// A guard dropped while the thread unwinds, whose resolution is admitted but
// reads a value whose `Display` is refused the one it asks for: that
// `Display` describes nothing, as it does where nothing unwinds, though the
// refusal answered it the empty string. One fewer in progress, nothing is
// refused either way.
#[test]
fn a_display_refused_while_the_thread_unwinds_describes_nothing() {
    quiet();

    let reports: Arc<Mutex<Vec<&'static str>>> = Arc::default();
    let sink = Arc::clone(&reports);
    let parser = Parser::builder()
        .on_report(move |report| sink.lock().unwrap().push(report.code.as_str()))
        .build();
    let payload = Object::new().with(
        "p",
        Value::host(Asks {
            parser: parser.clone(),
        }),
    );
    let resolve = |levels: usize, panics: bool| {
        let (text, dropped) = unwinding(
            &parser,
            levels,
            false,
            panics,
            "{{p; default:D}}",
            payload.clone(),
        );

        assert_eq!(text, wrapped(levels, if panics { "" } else { "ok" }));

        (dropped, std::mem::take(&mut *reports.lock().unwrap()))
    };

    for panics in [true, false] {
        assert_eq!(
            resolve(MAX_REENTRY - 1, panics),
            ("D".to_owned(), vec!["unserializable-value"])
        );
        assert_eq!(
            resolve(MAX_REENTRY - 2, panics),
            ("<x>".to_owned(), Vec::new())
        );
    }
}

#[test]
fn a_report_handler_that_panics_is_contained() {
    quiet();

    let calls: Arc<Mutex<usize>> = Arc::default();
    let counted = Arc::clone(&calls);
    let parser = Parser::builder()
        .on_report(move |_: &Report| {
            *counted.lock().unwrap() += 1;
            panic!("a handler that panics");
        })
        .build();

    assert_eq!(
        parser.resolve("{{a:zz; default:x}}{{b:zz; default:y}}", &Context::new()),
        "xy"
    );
    assert_eq!(*calls.lock().unwrap(), 2);
}
