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
use std::panic::AssertUnwindSafe;
use std::sync::{Arc, Mutex, Once};

use curly_message_parser::{
    Context, MAX_NESTING, MAX_REENTRY, ModifierError, ModifierInput, Object, Parser, ReentryLimit,
    Report, Value,
};

type Answer = Result<Option<Value>, ModifierError>;

fn text(text: impl Into<String>) -> Answer {
    Ok(Some(Value::from(text.into())))
}

fn recording() -> (
    Arc<Mutex<Vec<&'static str>>>,
    impl Fn(&Report) + Send + Sync + 'static,
) {
    let reports: Arc<Mutex<Vec<&'static str>>> = Arc::default();
    let sink = Arc::clone(&reports);

    (reports, move |report: &Report| {
        sink.lock().unwrap().push(report.code.as_str());
    })
}

fn quiet() {
    static QUIET: Once = Once::new();

    QUIET.call_once(|| std::panic::set_hook(Box::new(|_| {})));
}

struct Panics;

// A panic payload whose drop panics in turn.
struct DropPanics;

impl Drop for DropPanics {
    fn drop(&mut self) {
        panic!("a payload that panics as it is dropped");
    }
}

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

// The same where the host code that asked is a modifier: its placeholder
// takes its fallback chain and reports `failed-modifier`, whether the thread
// unwinds or not.
#[test]
fn a_modifier_refused_while_the_thread_unwinds_fails() {
    quiet();

    let slot: Slot = Arc::default();
    let again = Arc::clone(&slot);
    let (reports, record) = recording();
    let parser = Parser::builder()
        .on_report(record)
        .modifier("ask", move |_: &ModifierInput<'_>| {
            let inner = parser_in(&again).resolve("x", &Context::new());

            text(format!("<{inner}>"))
        })
        .build();

    *slot.lock().unwrap() = Some(parser.clone());

    let resolve = |levels: usize, panics: bool| {
        let payload = Object::new().with("v", "a");
        let (text, dropped) = unwinding(
            &parser,
            levels,
            false,
            panics,
            "{{v:ask; default:D}}",
            payload,
        );

        assert_eq!(text, wrapped(levels, if panics { "" } else { "ok" }));

        (dropped, std::mem::take(&mut *reports.lock().unwrap()))
    };

    for panics in [true, false] {
        assert_eq!(
            resolve(MAX_REENTRY - 1, panics),
            ("D".to_owned(), vec!["failed-modifier"])
        );
        assert_eq!(
            resolve(MAX_REENTRY - 2, panics),
            ("<x>".to_owned(), Vec::new())
        );
    }

    *slot.lock().unwrap() = None;
}

// Answered as a value, the payload describes itself as `y`.
impl fmt::Display for AsksOnDrop {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str("y")
    }
}

// The same where the host code is a modifier: one panics with a payload that
// resolves as it drops, the other answers a value that does, though the one
// resolution it asked for itself was refused. Where the thread unwinds, the
// refusal made as that payload or answer drops fails the modifier, which
// failed already, and nothing around it; where it does not, the answer stands.
#[test]
fn a_modifier_whose_drop_is_refused_while_the_thread_unwinds_fails_alone() {
    quiet();

    let slot: Slot = Arc::default();
    let boom = Arc::clone(&slot);
    let gives = Arc::clone(&slot);
    let (reports, record) = recording();
    let parser = Parser::builder()
        .on_report(record)
        .modifier("boom", move |_: &ModifierInput<'_>| -> Answer {
            std::panic::panic_any(AsksOnDrop {
                parser: parser_in(&boom),
            })
        })
        .modifier("gives", move |_: &ModifierInput<'_>| {
            let parser = parser_in(&gives);
            let _ =
                std::panic::catch_unwind(AssertUnwindSafe(|| parser.resolve("x", &Context::new())));

            Ok(Some(Value::host(AsksOnDrop { parser })))
        })
        .build();

    *slot.lock().unwrap() = Some(parser.clone());

    let resolve = |levels: usize, panics: bool, message: &'static str| {
        let payload = Object::new().with("v", "a");
        let (text, dropped) = unwinding(&parser, levels, false, panics, message, payload);

        assert_eq!(text, wrapped(levels, if panics { "" } else { "ok" }));

        (dropped, std::mem::take(&mut *reports.lock().unwrap()))
    };
    let failed = || ("aDb".to_owned(), vec!["failed-modifier"]);
    let answered = || ("ayb".to_owned(), Vec::new());

    for levels in [MAX_REENTRY - 1, MAX_REENTRY - 2] {
        for panics in [true, false] {
            assert_eq!(resolve(levels, panics, "a{{v:boom; default:D}}b"), failed());
        }
    }

    assert_eq!(
        resolve(MAX_REENTRY - 1, true, "a{{v:gives; default:D}}b"),
        failed()
    );

    for (levels, panics) in [
        (MAX_REENTRY - 1, false),
        (MAX_REENTRY - 2, true),
        (MAX_REENTRY - 2, false),
    ] {
        assert_eq!(
            resolve(levels, panics, "a{{v:gives; default:D}}b"),
            answered()
        );
    }

    *slot.lock().unwrap() = None;
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

#[test]
fn a_modifier_that_panics_is_contained() {
    quiet();

    let (reports, handler) = recording();
    let parser = Parser::builder()
        .on_report(handler)
        .modifier("panics", |_: &ModifierInput<'_>| -> Answer {
            panic!("a modifier that panics")
        })
        .modifier("asks", |input: &ModifierInput<'_>| -> Answer {
            let _ = input.default();
            let _ = input.option_value(0);
            panic!("a modifier that panics after asking")
        })
        .modifier("payload", |_: &ModifierInput<'_>| -> Answer {
            std::panic::panic_any(DropPanics)
        })
        .build();
    let payload = Object::new().with("v", "a");
    let context = Context::new().payload(&payload);

    assert_eq!(
        parser.resolve(
            "{{v:panics; default:D}}|{{v:asks; a:{{v:zz}}; default:{{v:yy; default:E}}}}|{{v:payload}}",
            &context
        ),
        "D|E|"
    );
    assert_eq!(
        *reports.lock().unwrap(),
        [
            "failed-modifier",
            "unknown-modifier",
            "unknown-modifier",
            "failed-modifier",
            "failed-modifier"
        ]
    );
}

// A resolution asked for past `MAX_REENTRY` unwinds into the host code that
// asked, before anything is read. Each test runs on a thread of the size the
// README says a thread needs for the worst case.
fn on_reentry_stack(test: impl FnOnce() + Send + 'static) {
    std::thread::Builder::new()
        .stack_size(512 * 1024)
        .spawn(test)
        .unwrap()
        .join()
        .unwrap();
}

type Slot = Arc<Mutex<Option<Parser>>>;

fn parser_in(slot: &Slot) -> Parser {
    slot.lock().unwrap().clone().unwrap()
}

// A modifier resolving the message it is in again, as many levels deep as its
// value says, each level on the thread of the one before.
fn down(slot: Slot, calls: Arc<Mutex<usize>>) -> impl Fn(&ModifierInput<'_>) -> Answer {
    move |input: &ModifierInput<'_>| {
        *calls.lock().unwrap() += 1;

        let levels: usize = input.value().parse().unwrap();

        if levels == 1 {
            return text("leaf");
        }

        let payload = Object::new().with("v", (levels - 1).to_string());
        let inner =
            parser_in(&slot).resolve("{{v:down; default:D}}", &Context::new().payload(&payload));

        text(format!("<{inner}>"))
    }
}

#[test]
fn a_resolution_asked_for_past_the_reentry_limit_is_refused_to_the_code_that_asked() {
    on_reentry_stack(|| {
        let slot: Slot = Arc::default();
        let calls: Arc<Mutex<usize>> = Arc::default();
        let (reports, record) = recording();
        let parser = Parser::builder()
            .on_report(record)
            .modifier("down", down(Arc::clone(&slot), Arc::clone(&calls)))
            .build();

        *slot.lock().unwrap() = Some(parser.clone());

        let resolve = |levels: usize| {
            let payload = Object::new().with("v", levels.to_string());

            parser.resolve("{{v:down; default:D}}", &Context::new().payload(&payload))
        };
        let wrapped = |inner: &str| {
            format!(
                "{}{inner}{}",
                "<".repeat(MAX_REENTRY - 1),
                ">".repeat(MAX_REENTRY - 1)
            )
        };

        // `MAX_REENTRY` in progress at once, and none refused.
        assert_eq!(resolve(MAX_REENTRY), wrapped("leaf"));
        assert_eq!(std::mem::take(&mut *calls.lock().unwrap()), MAX_REENTRY);
        assert_eq!(
            std::mem::take(&mut *reports.lock().unwrap()),
            Vec::<&str>::new()
        );

        // One more is refused before its message is read: no modifier call,
        // no report of its own. The modifier that asked let the refusal pass,
        // so its placeholder takes the default, and every resolution around
        // it returns.
        assert_eq!(resolve(MAX_REENTRY + 1), wrapped("D"));
        assert_eq!(std::mem::take(&mut *calls.lock().unwrap()), MAX_REENTRY);
        assert_eq!(
            std::mem::take(&mut *reports.lock().unwrap()),
            ["failed-modifier"]
        );

        // The refusal left the count as it found it.
        assert_eq!(resolve(MAX_REENTRY), wrapped("leaf"));
        assert_eq!(
            std::mem::take(&mut *reports.lock().unwrap()),
            Vec::<&str>::new()
        );

        *slot.lock().unwrap() = None;
    });
}

#[test]
fn the_code_a_refusal_unwinds_into_may_handle_it() {
    on_reentry_stack(|| {
        let slot: Slot = Arc::default();
        let again = Arc::clone(&slot);
        let (reports, record) = recording();
        let parser = Parser::builder()
            .on_report(record)
            .modifier("self", move |_: &ModifierInput<'_>| {
                let parser = parser_in(&again);
                let payload = Object::new().with("v", "a");
                let inner = std::panic::catch_unwind(AssertUnwindSafe(|| {
                    parser.resolve("<{{v:self; default:D}}>", &Context::new().payload(&payload))
                }));

                match inner {
                    Ok(inner) => text(inner),
                    Err(refusal) => {
                        assert_eq!(refusal.downcast_ref(), Some(&ReentryLimit));

                        text("refused")
                    }
                }
            })
            .build();

        *slot.lock().unwrap() = Some(parser.clone());

        let payload = Object::new().with("v", "a");

        assert_eq!(
            parser.resolve("<{{v:self; default:D}}>", &Context::new().payload(&payload)),
            format!(
                "{}refused{}",
                "<".repeat(MAX_REENTRY),
                ">".repeat(MAX_REENTRY)
            )
        );
        assert_eq!(*reports.lock().unwrap(), Vec::<&str>::new());

        *slot.lock().unwrap() = None;
    });
}

// The count is the thread's: a resolution host code hands to a thread of its
// own is one asked for where none is in progress, whatever the thread that
// handed it holds.
#[test]
fn a_chain_handed_to_another_thread_is_not_counted_there() {
    on_reentry_stack(|| {
        let slot: Slot = Arc::default();
        let (down_slot, hop_slot) = (Arc::clone(&slot), Arc::clone(&slot));
        let (reports, record) = recording();
        let elsewhere = move |slot: &Slot, levels: usize| {
            let parser = parser_in(slot);

            std::thread::scope(|scope| {
                scope
                    .spawn(move || {
                        let payload = Object::new().with("w", levels.to_string());

                        parser.resolve("{{w:hop; default:D}}", &Context::new().payload(&payload))
                    })
                    .join()
                    .unwrap()
            })
        };
        let parser = Parser::builder()
            .on_report(record)
            .modifier("down", move |input: &ModifierInput<'_>| {
                let levels: usize = input.value().parse().unwrap();

                // The deepest of `MAX_REENTRY` in progress on this thread
                // hands the chain on.
                let inner = if levels == 1 {
                    elsewhere(&down_slot, MAX_REENTRY)
                } else {
                    let payload = Object::new().with("v", (levels - 1).to_string());

                    parser_in(&down_slot)
                        .resolve("{{v:down; default:D}}", &Context::new().payload(&payload))
                };

                text(format!("<{inner}>"))
            })
            .modifier("hop", move |input: &ModifierInput<'_>| {
                let levels: usize = input.value().parse().unwrap();

                if levels == 1 {
                    return text("leaf");
                }

                text(format!("<{}>", elsewhere(&hop_slot, levels - 1)))
            })
            .build();

        *slot.lock().unwrap() = Some(parser.clone());

        let payload = Object::new().with("v", MAX_REENTRY.to_string());
        let whole = 2 * MAX_REENTRY - 1;

        assert_eq!(
            parser.resolve("{{v:down; default:D}}", &Context::new().payload(&payload)),
            format!("{}leaf{}", "<".repeat(whole), ">".repeat(whole))
        );
        assert_eq!(*reports.lock().unwrap(), Vec::<&str>::new());

        *slot.lock().unwrap() = None;
    });
}

// A message nesting a modifier that reads an option holding the next, as
// deep as a placeholder resolves, around `innermost`, nested past it.
fn reading_options(innermost: &str) -> String {
    (1..MAX_NESTING).fold(format!("x{{{{v:deeper; a:{innermost}}}}}"), |inner, _| {
        format!("{{{{v:deeper; a:{inner}}}}}")
    })
}

// The worst case of section 13 for the thread's stack: `MAX_REENTRY`
// resolutions in progress, each as deep into its options as a placeholder
// resolves. The one nested past them reports from there, and the report
// handler starts the resolution again.
#[test]
fn a_resolution_reaching_itself_through_modifiers_stops_at_the_reentry_limit() {
    quiet();

    on_reentry_stack(|| {
        let nested = reading_options("{{v:again}}");
        let (reports, record) = recording();
        let slot: Slot = Arc::default();
        let again = Arc::clone(&slot);
        let message = nested.clone();
        let built = Parser::builder()
            .on_report(move |report| {
                let payload = Object::new().with("v", "a");

                record(report);

                // The refusal of the one past the bound unwinds out of the
                // handler, which leaves its report undelivered.
                parser_in(&again).resolve(message.as_str(), &Context::new().payload(&payload));
            })
            .modifier("deeper", |input: &ModifierInput<'_>| {
                text(input.option_value(0).unwrap_or_default())
            })
            .build();

        *slot.lock().unwrap() = Some(built.clone());

        let payload = Object::new().with("v", "a");
        let context = Context::new().payload(&payload);
        let text = built.resolve(nested.as_str(), &context);

        assert_eq!(text, "x");
        assert_eq!(
            std::mem::take(&mut *reports.lock().unwrap()),
            vec!["nesting-limit"; MAX_REENTRY]
        );
        assert_eq!(built.resolve("{{v:deeper; a:ok}}", &context), "ok");

        *slot.lock().unwrap() = None;
    });
}

// A payload `default` that describes itself by resolving the message it is
// the default of, over the same payload.
struct Restarts {
    slot: Slot,
    message: String,
    payload: Arc<Mutex<Option<Object>>>,
}

impl fmt::Display for Restarts {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let payload = self.payload.lock().unwrap().clone().unwrap();
        let text =
            parser_in(&self.slot).resolve(self.message.as_str(), &Context::new().payload(&payload));

        formatter.write_str(&text)
    }
}

// The same through the fallback chain: each resolution reads its options as
// deep as a placeholder resolves, and the one nested past them reads the
// payload's `default`, whose `Display` starts the next. Every one of
// `MAX_REENTRY` resolves whole, its nesting never cut short by the ones
// around it.
#[test]
fn a_resolution_begun_from_the_deepest_option_resolves_whole() {
    on_reentry_stack(|| {
        let message = reading_options("{{x}}");
        let slot: Slot = Arc::default();
        let calls: Arc<Mutex<usize>> = Arc::default();
        let counted = Arc::clone(&calls);
        let (reports, record) = recording();
        let parser = Parser::builder()
            .on_report(record)
            .modifier("deeper", move |input: &ModifierInput<'_>| {
                *counted.lock().unwrap() += 1;

                text(input.option_value(0).unwrap_or_default())
            })
            .build();
        let cell: Arc<Mutex<Option<Object>>> = Arc::default();
        let payload = Object::new().with("v", "a").with(
            "default",
            Value::host(Restarts {
                slot: Arc::clone(&slot),
                message: message.clone(),
                payload: Arc::clone(&cell),
            }),
        );

        *slot.lock().unwrap() = Some(parser.clone());
        *cell.lock().unwrap() = Some(payload.clone());

        let text = parser.resolve(message.as_str(), &Context::new().payload(&payload));

        // The one past the bound is refused, so the `default` that asked
        // for it describes nothing, and the deepest resolution's chain ends
        // there.
        assert_eq!(text, "x".repeat(MAX_REENTRY));
        assert_eq!(*calls.lock().unwrap(), MAX_REENTRY * MAX_NESTING);

        let mut expected = vec!["nesting-limit"; MAX_REENTRY];

        expected.push("unserializable-value");

        assert_eq!(*reports.lock().unwrap(), expected);

        *slot.lock().unwrap() = None;
        *cell.lock().unwrap() = None;
    });
}
