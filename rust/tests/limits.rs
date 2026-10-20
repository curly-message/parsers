#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

use std::fmt;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};

use curly_message_parser::{
    Array, Context, MAX_CONVERSION_NODES, MAX_HOST_DEPTH, MAX_NESTING, MAX_OUTPUT_LENGTH,
    MAX_READ_LENGTH, MAX_SERIALIZED_LENGTH, Object, Parser, Report, Value,
};

fn recording() -> (Parser, Arc<Mutex<Vec<Report>>>) {
    let reports: Arc<Mutex<Vec<Report>>> = Arc::default();
    let sink = Arc::clone(&reports);
    let parser = Parser::builder()
        .on_report(move |report| sink.lock().unwrap().push(report.clone()))
        .build();

    (parser, reports)
}

fn codes(reports: &Mutex<Vec<Report>>) -> Vec<&'static str> {
    std::mem::take(&mut *reports.lock().unwrap())
        .iter()
        .map(|report| report.code.as_str())
        .collect()
}

// A value whose serialization visits at least `count` nodes, holding no
// cycle: each level an array naming the level below twice.
fn nodes(count: usize) -> Value {
    let mut value = Value::from(0);
    let mut visits = 1;

    while visits < count {
        value = Value::from(Arc::new(Array::from(vec![value.clone(), value])));
        visits = visits * 2 + 1;
    }

    value
}

fn on_small_stack(test: impl FnOnce() + Send + 'static) {
    std::thread::Builder::new()
        .stack_size(64 * 1024)
        .spawn(test)
        .unwrap()
        .join()
        .unwrap();
}

#[test]
fn the_output_carries_what_it_has_room_for() {
    let (parser, reports) = recording();
    let payload = Object::new().with("v", "a").with("w", "tail");
    let context = Context::new().payload(&payload);
    let at = format!("{{{{v; a:{};}}}}", "x".repeat(MAX_OUTPUT_LENGTH));
    let over = format!(
        "A{{{{v; a:{};}}}}B{{{{w}}}}",
        "x".repeat(MAX_OUTPUT_LENGTH + 1)
    );

    assert_eq!(
        parser.resolve(at.as_str(), &context).len(),
        MAX_OUTPUT_LENGTH
    );
    assert_eq!(codes(&reports), Vec::<&str>::new());
    assert_eq!(parser.resolve(over.as_str(), &context), "ABtail");
    assert_eq!(codes(&reports), ["output-limit"]);
}

#[test]
fn the_output_is_counted_in_bytes_once_per_result() {
    let (parser, reports) = recording();
    let half = "\u{e9}".repeat(MAX_OUTPUT_LENGTH / 4);
    let payload = Object::new().with("v", "a").with("n", "x");
    let message = format!("{{{{v; a:{{{{v; a:{half}}}}}}}}}{{{{v; a:{half}}}}}{{{{n}}}}");

    assert_eq!(
        parser
            .resolve(message.as_str(), &Context::new().payload(&payload))
            .len(),
        MAX_OUTPUT_LENGTH
    );
    assert_eq!(codes(&reports), ["output-limit"]);
}

#[test]
fn the_read_budget_is_tested_before_a_placeholder_reads() {
    let (parser, reports) = recording();
    let message = "{{v:eq; nomatch:X; default:ok;}}{{w}}";
    let at = Object::new()
        .with("v", "x".repeat(MAX_READ_LENGTH))
        .with("w", "");
    let over = Object::new()
        .with("v", "x".repeat(MAX_READ_LENGTH + 1))
        .with("w", "tail");

    assert_eq!(parser.resolve(message, &Context::new().payload(&at)), "ok");
    assert_eq!(codes(&reports), Vec::<&str>::new());
    assert_eq!(
        parser.resolve(message, &Context::new().payload(&over)),
        "ok"
    );
    assert_eq!(codes(&reports), ["read-limit"]);
}

#[test]
fn a_value_visiting_more_nodes_than_the_limit_describes_nothing() {
    let (parser, reports) = recording();
    let under = Object::new().with("v", nodes(MAX_CONVERSION_NODES / 2));
    let over = Object::new().with("v", nodes(MAX_CONVERSION_NODES + 1));
    let message = "{{v:eq; nomatch:X; default:D}}";

    assert_eq!(
        parser.resolve(message, &Context::new().payload(&under)),
        "D"
    );
    assert_eq!(codes(&reports), Vec::<&str>::new());
    assert_eq!(parser.resolve(message, &Context::new().payload(&over)), "D");
    assert_eq!(codes(&reports), ["unserializable-value"]);
}

// A host value describing itself as so many bytes.
struct Wide(usize);

impl fmt::Display for Wide {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&"x".repeat(self.0))
    }
}

// An array naming one child `count` times, each element and its comma taking
// 100 000 bytes of the serialization: the payload holds the child once.
fn wide(count: usize, child: Value) -> Object {
    Object::new().with("v", Array::from(vec![child; count]))
}

// A description written a run at a time, `length` bytes in all.
struct Runs(usize);

impl fmt::Display for Runs {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let run = "x".repeat(1 << 16);
        let mut left = self.0;

        while left > 0 {
            let next = left.min(run.len());

            formatter.write_str(run.get(..next).unwrap())?;
            left -= next;
        }

        Ok(())
    }
}

// A value's own description is a string as a serialization is, held to the
// same bound: one writing past it describes nothing, however it writes.
#[test]
fn a_description_longer_than_a_string_holds_describes_nothing() {
    let (parser, reports) = recording();
    let message = "{{v; default:D}}";
    let resolve = |length| {
        let payload = Object::new().with("v", Value::host(Runs(length)));

        parser.resolve(message, &Context::new().payload(&payload))
    };

    assert_eq!(resolve(MAX_SERIALIZED_LENGTH), "");
    assert_eq!(codes(&reports), ["output-limit"]);
    assert_eq!(resolve(MAX_SERIALIZED_LENGTH + 1), "D");
    assert_eq!(codes(&reports), ["unserializable-value"]);
}

// A description that goes on after the bound refused one of its runs, the
// error thrown away, then writes what it holds, which would fit: it has lost
// that run, so it describes nothing, whether it writes more or ends there.
struct Ignoring(&'static str);

impl fmt::Display for Ignoring {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        Runs(MAX_SERIALIZED_LENGTH - 10).fmt(formatter)?;
        let _ = formatter.write_str(&"x".repeat(100));

        formatter.write_str(self.0)
    }
}

#[test]
fn a_description_going_on_past_a_refusal_describes_nothing() {
    let (parser, reports) = recording();

    for then in ["ok", ""] {
        let payload = Object::new().with("v", Value::host(Ignoring(then)));

        assert_eq!(
            parser.resolve("{{v; default:D}}", &Context::new().payload(&payload)),
            "D",
            "{then:?}"
        );
        assert_eq!(codes(&reports), ["unserializable-value"], "{then:?}");
    }
}

#[test]
fn a_value_writing_more_than_a_serialization_holds_describes_nothing() {
    let (parser, reports) = recording();
    let leaf = Value::from(Array::from(vec![Value::from("x".repeat(99_995))]));
    let fits = MAX_SERIALIZED_LENGTH / 100_000;
    let message = "{{v}}|{{w; default:after}}";

    // What fits is read, and the placeholder after it finds the read budget
    // spent.
    assert_eq!(
        parser.resolve(message, &Context::new().payload(&wide(fits, leaf.clone()))),
        "|"
    );
    assert_eq!(codes(&reports), ["output-limit", "read-limit"]);

    // The bound counts code units of UTF-16, as V8 counts a string: a last
    // element filling it to the unit, though it takes more bytes of UTF-8,
    // is written, and one unit more is not.
    let last = |extra: &str| {
        let text = format!(
            "{}{}\"\n\u{1}{}{extra}",
            "\u{e9}".repeat(50),
            "\u{1f600}".repeat(25),
            "x".repeat(70_772)
        );
        let mut items = vec![leaf.clone(); fits];

        items.push(Value::from(Array::from(vec![Value::from(text)])));

        Object::new().with("v", Array::from(items))
    };

    assert_eq!(
        parser.resolve(message, &Context::new().payload(&last(""))),
        "|"
    );
    assert_eq!(codes(&reports), ["output-limit", "read-limit"]);
    assert_eq!(
        parser.resolve(message, &Context::new().payload(&last("x"))),
        "|after"
    );
    assert_eq!(codes(&reports), ["unserializable-value"]);

    // Few nodes, so only the length says it cannot be written.
    assert!(2 * (fits + 1) < MAX_CONVERSION_NODES);
    assert_eq!(
        parser.resolve(message, &Context::new().payload(&wide(fits + 1, leaf))),
        "|after"
    );
    assert_eq!(codes(&reports), ["unserializable-value"]);

    // A host value's description written inside a structure counts the same.
    let hosts = wide(fits + 1, Value::host(Wide(99_997)));

    assert_eq!(
        parser.resolve(message, &Context::new().payload(&hosts)),
        "|after"
    );
    assert_eq!(codes(&reports), ["unserializable-value"]);
}

#[test]
fn a_placeholder_nested_past_the_limit_takes_its_chain() {
    let (parser, reports) = recording();
    let payload = Object::new().with("v", "a");
    let context = Context::new().payload(&payload);
    let nested = |depth: usize| {
        (1..depth).fold(
            "{{v; a:settled; default:fallback;}}".to_owned(),
            |inner, _| format!("{{{{v; a:{inner};}}}}"),
        )
    };

    assert_eq!(
        parser.resolve(nested(MAX_NESTING).as_str(), &context),
        "settled"
    );
    assert_eq!(codes(&reports), Vec::<&str>::new());
    assert_eq!(
        parser.resolve(nested(MAX_NESTING + 1).as_str(), &context),
        "fallback"
    );
    assert_eq!(codes(&reports), ["nesting-limit"]);
}

#[test]
fn a_message_nested_a_hundred_thousand_levels_resolves_without_a_call_per_level() {
    on_small_stack(|| {
        let (parser, reports) = recording();
        let depth = 100_000;
        let defaults = format!("{}x{}", "{{a; default:".repeat(depth), "}}".repeat(depth));

        assert_eq!(parser.resolve(defaults.as_str(), &Context::new()), "x");

        let reported = std::mem::take(&mut *reports.lock().unwrap());

        assert_eq!(reported.len(), depth - MAX_NESTING);
        assert!(
            reported
                .iter()
                .all(|report| report.code.as_str() == "nesting-limit")
        );

        let payload = Object::new().with("v", "a");
        let options = format!("{}x{}", "{{v; a:".repeat(depth), "}}".repeat(depth));

        assert_eq!(
            parser.resolve(options.as_str(), &Context::new().payload(&payload)),
            ""
        );
        assert_eq!(codes(&reports), ["nesting-limit"]);
    });
}

#[test]
fn a_value_nested_a_million_levels_converts_without_a_call_per_level() {
    on_small_stack(|| {
        let (parser, reports) = recording();
        let nest = |depth: usize| {
            (0..depth).fold(Value::from(1), |inner, _| {
                Value::from(Array::from(vec![inner]))
            })
        };
        let deep = Object::new().with("v", nest(1_000_000));
        let shallow = Object::new().with("v", nest(40_000));

        assert_eq!(
            parser.resolve("{{v; default:D}}", &Context::new().payload(&deep)),
            "D"
        );
        assert_eq!(codes(&reports), ["unserializable-value"]);

        let text = parser.resolve("{{v}}", &Context::new().payload(&shallow));

        assert_eq!(text.len(), 40_000 * 2 + 1);
        assert_eq!(
            text,
            format!("{}1{}", "[".repeat(40_000), "]".repeat(40_000))
        );
        assert_eq!(codes(&reports), Vec::<&str>::new());
    });
}

// The stack the README says a thread needs for every activation
// `MAX_HOST_DEPTH` allows, a debug build's included.
fn on_host_depth_stack(test: impl FnOnce() + Send + 'static) {
    std::thread::Builder::new()
        .stack_size(512 * 1024)
        .spawn(test)
        .unwrap()
        .join()
        .unwrap();
}

// A host value describing itself by resolving a message that reads it again:
// a resolution reaching itself without end.
struct Recursive {
    parser: Parser,
    message: &'static str,
    payload: Arc<Mutex<Option<Object>>>,
}

impl fmt::Display for Recursive {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let payload = self.payload.lock().unwrap().clone().unwrap();
        let text = self
            .parser
            .resolve(self.message, &Context::new().payload(&payload));

        formatter.write_str(&text)
    }
}

// A payload whose `self` entry holds, where `place` puts it, a value
// describing itself by resolving `message` over the same payload. The cell
// is emptied once the test is done, so the payload can drop.
fn reaching_itself(
    parser: &Parser,
    message: &'static str,
    place: fn(Value) -> Value,
) -> (Object, Arc<Mutex<Option<Object>>>) {
    let cell: Arc<Mutex<Option<Object>>> = Arc::default();
    let payload = Object::new().with(
        "self",
        place(Value::host(Recursive {
            parser: parser.clone(),
            message,
            payload: Arc::clone(&cell),
        })),
    );

    *cell.lock().unwrap() = Some(payload.clone());

    (payload, cell)
}

#[test]
fn a_resolution_reaching_itself_without_end_stops_at_the_host_depth() {
    on_host_depth_stack(|| {
        let (parser, reports) = recording();
        let (payload, cell) = reaching_itself(&parser, "<{{self}}>", |value| value);

        let text = parser.resolve("{{self; default:D}}", &Context::new().payload(&payload));
        // The walk the thread had no room for answered nothing, so the
        // value whose description started it is the one that reports.
        // Every walk around it saw an ordinary answer.
        assert_eq!(
            text,
            format!(
                "{}{}",
                "<".repeat(MAX_HOST_DEPTH - 1),
                ">".repeat(MAX_HOST_DEPTH - 1)
            )
        );
        assert_eq!(codes(&reports), ["unserializable-value"]);
        assert_eq!(parser.resolve("{{self}}", &Context::new()), "");
        assert_eq!(parser.resolve("ok", &Context::new()), "ok");

        *cell.lock().unwrap() = None;
    });
}

// The same, through the serialization of the structure the value sits in.
#[test]
fn a_value_inside_a_structure_reaching_itself_stops_at_the_host_depth() {
    on_host_depth_stack(|| {
        let (parser, reports) = recording();
        let message = "{{self:eq; nomatch:X; default:<>}}";
        let (payload, cell) = reaching_itself(&parser, message, |value| {
            Value::from(Array::from(vec![value]))
        });

        assert_eq!(
            parser.resolve(message, &Context::new().payload(&payload)),
            "<>"
        );
        assert_eq!(codes(&reports), ["unserializable-value"]);

        *cell.lock().unwrap() = None;
    });
}

// The same, through a report handler that resolves again.
#[test]
fn a_report_handler_reaching_itself_stops_at_the_host_depth() {
    on_host_depth_stack(|| {
        let calls = Arc::new(AtomicUsize::new(0));
        let slot: Arc<Mutex<Option<Parser>>> = Arc::default();
        let (counted, again) = (Arc::clone(&calls), Arc::clone(&slot));
        let parser = Parser::builder()
            .on_report(move |_| {
                let parser = again.lock().unwrap().clone().unwrap();

                counted.fetch_add(1, Ordering::Relaxed);
                parser.resolve("{{x:none}}", &Context::new());
            })
            .build();

        *slot.lock().unwrap() = Some(parser.clone());

        assert_eq!(
            parser.resolve("{{x:none; default:D}}", &Context::new()),
            "D"
        );
        // Each resolution the thread had room for reported once, from inside
        // the handler of the one before it.
        assert_eq!(calls.load(Ordering::Relaxed), MAX_HOST_DEPTH);

        *slot.lock().unwrap() = None;
    });
}
