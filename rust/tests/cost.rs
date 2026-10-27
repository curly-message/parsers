#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

use std::fmt;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use curly_message_parser::{Context, Object, Parser, Value};

// A host value that counts how often it is asked to describe itself.
struct Counted(Arc<AtomicUsize>);

impl fmt::Display for Counted {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.0.fetch_add(1, Ordering::Relaxed);
        formatter.write_str("h")
    }
}

fn counted() -> (Value, Arc<AtomicUsize>) {
    let calls = Arc::new(AtomicUsize::new(0));

    (Value::host(Counted(Arc::clone(&calls))), calls)
}

#[test]
fn a_value_read_by_many_placeholders_converts_once() {
    let (host, calls) = counted();
    let payload = Object::new()
        .with("h", host.clone())
        .with("inside", vec![host]);
    let message = "{{h}}".repeat(1_000);
    let context = Context::new().payload(&payload);
    let parser = Parser::new();

    assert_eq!(
        parser.resolve(message.as_str(), &context),
        "h".repeat(1_000)
    );
    assert_eq!(calls.load(Ordering::Relaxed), 1);

    // The record is the resolution's: the next one asks again, once.
    assert_eq!(
        parser.resolve("{{h}}{{inside}}{{inside}}", &context),
        "h[\"h\"][\"h\"]"
    );
    assert_eq!(calls.load(Ordering::Relaxed), 3);
}

#[test]
fn what_resolution_never_reads_is_never_converted() {
    let (host, calls) = counted();
    let payload = Object::new()
        .with("k", "b")
        .with("h", host.clone())
        .with("w", Object::new().with("value", "v").with("default", host));
    let context = Context::new().payload(&payload);
    let parser = Parser::new();

    assert_eq!(parser.resolve("{{k; a:{{h}}; b:x}}", &context), "x");
    assert_eq!(
        parser.resolve("{{k:ne; b:{{h}}; default:d}}", &context),
        "d"
    );
    assert_eq!(parser.resolve("{{w}}{{k}}", &context), "vb");
    assert_eq!(
        parser.resolve("{{k:gt; x:{{h}}; default:{{k}}}}", &context),
        "b"
    );
    assert_eq!(calls.load(Ordering::Relaxed), 0);

    assert_eq!(parser.resolve("{{w:eq; nomatch:x}}", &context), "h");
    assert_eq!(calls.load(Ordering::Relaxed), 1);
}
