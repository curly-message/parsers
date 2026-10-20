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

use curly_message_parser::{Context, Object, Parser, Report, Value};

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
