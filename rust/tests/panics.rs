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
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex, Once};

use curly_message_parser::{
    Context, MAX_HOST_DEPTH, MAX_NESTING, ModifierError, ModifierInput, Object, Parser, Report,
    Value,
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

// The worst case of section 13 for the thread's stack: every activation the
// thread allows held at once, each a modifier reading an option that holds the
// next, and each resolution started again from inside its innermost one.
#[test]
fn a_resolution_reaching_itself_through_modifiers_stops_at_the_host_depth() {
    quiet();

    std::thread::Builder::new()
        .stack_size(512 * 1024)
        .spawn(|| {
            let nested = (1..MAX_NESTING).fold("{{v:again; a:x}}".to_owned(), |inner, _| {
                format!("{{{{v:deeper; a:{inner}}}}}")
            });
            let (reports, handler) = recording();
            let resolutions = Arc::new(AtomicUsize::new(0));
            let counted = Arc::clone(&resolutions);
            let parser: Arc<Mutex<Option<Parser>>> = Arc::default();
            let again = Arc::clone(&parser);
            let message = nested.clone();
            let built = Parser::builder()
                .on_report(handler)
                .modifier("deeper", |input: &ModifierInput<'_>| {
                    text(input.option_value(0).unwrap_or_default())
                })
                .modifier("again", move |input: &ModifierInput<'_>| {
                    let parser = again.lock().unwrap().clone().unwrap();
                    let payload = Object::new().with("v", input.value());

                    counted.fetch_add(1, Ordering::Relaxed);

                    let inner = parser.resolve(message.as_str(), &Context::new().payload(&payload));

                    // The leaf: what the thread had no room for comes back
                    // empty, and the modifier around it panics.
                    if inner.is_empty() {
                        panic!("the leaf");
                    }

                    text(inner)
                })
                .build();

            *parser.lock().unwrap() = Some(built.clone());

            let payload = Object::new().with("v", "a");
            let text = built.resolve(nested.as_str(), &Context::new().payload(&payload));

            // Each resolution holds one activation per level it nests, so the
            // thread holds this many before one is refused.
            let held = MAX_HOST_DEPTH / MAX_NESTING;

            assert_eq!(text, "");
            assert_eq!(resolutions.load(Ordering::Relaxed), held);
            assert_eq!(
                std::mem::take(&mut *reports.lock().unwrap()),
                vec!["failed-modifier"; held]
            );
            assert_eq!(
                built.resolve("{{v:deeper; a:ok}}", &Context::new().payload(&payload)),
                "ok"
            );

            *parser.lock().unwrap() = None;
        })
        .unwrap()
        .join()
        .unwrap();
}
