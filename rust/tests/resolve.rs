#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects,
    clippy::string_slice
)]

use std::fmt;
use std::sync::{Arc, Mutex};

use curly_message_parser::{
    Code, Context, Level, Message, NodeKind, Object, Origin, Parser, Report, Tree, Value, cst,
};

// A parser whose reports are kept, and the codes it reported.
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

fn resolve(message: &str, payload: &Object) -> String {
    Parser::new().resolve(message, &Context::new().payload(payload))
}

struct Refuses;

impl fmt::Display for Refuses {
    fn fmt(&self, _: &mut fmt::Formatter<'_>) -> fmt::Result {
        Err(fmt::Error)
    }
}

#[test]
fn a_placeholder_resolves_to_its_value() {
    let payload = Object::new().with("name", "Ada").with("count", 3);

    assert_eq!(resolve("Hi {{name}}, {{ count }}.", &payload), "Hi Ada, 3.");
    assert_eq!(resolve("plain", &payload), "plain");
    assert_eq!(resolve("", &payload), "");
}

#[test]
fn text_is_unescaped_and_a_value_is_never_read_again() {
    let payload = Object::new()
        .with("v", "{{w}} \\{")
        .with("w", "no")
        .with("v}", "brace");

    assert_eq!(resolve("{{v}}", &payload), "{{w}} \\{");
    assert_eq!(resolve("\\{{v}}", &payload), "{{v}}");
    assert_eq!(resolve("{{v\\}}", &payload), "{{v}}");
    assert_eq!(resolve("{{v\\}}}", &payload), "brace");
    assert_eq!(resolve("a\\\\b\\d", &payload), "a\\b\\d");
}

#[test]
fn prototype_names_are_payload_keys_like_any_other() {
    let payload = Object::new().with("__proto__", "p");

    assert_eq!(resolve("{{__proto__}}", &payload), "p");
    assert_eq!(resolve("{{toString; default:none}}", &payload), "none");
    assert_eq!(resolve("{{constructor}}", &payload), "");
}

#[test]
fn a_value_converts_by_section_4() {
    let payload = Object::new()
        .with("null", Value::Null)
        .with("yes", true)
        .with("half", 0.5)
        .with("big", 1e21)
        .with("negative", -7)
        .with("unsigned", u64::MAX)
        .with("date", Value::date(0.0))
        .with("invalid", Value::date(f64::NAN))
        .with(
            "array",
            vec![Value::from(1), Value::Undefined, Value::from("x")],
        )
        .with(
            "object",
            Object::new()
                .with("b", f64::INFINITY)
                .with("a", Value::Undefined)
                .with("c", Value::date(0.0)),
        )
        .with("host", Value::host(42));

    let cases = [
        ("{{null}}", "null"),
        ("{{yes}}", "true"),
        ("{{half}}", "0.5"),
        ("{{big}}", "1e+21"),
        ("{{negative}}", "-7"),
        ("{{unsigned}}", "18446744073709551615"),
        ("{{date}}", "1970-01-01T00:00:00.000Z"),
        ("{{invalid}}", "Invalid Date"),
        ("{{array}}", "[1,null,\"x\"]"),
        (
            "{{object}}",
            "{\"b\":null,\"c\":\"1970-01-01T00:00:00.000Z\"}",
        ),
        ("{{host}}", "42"),
    ];

    for (message, expected) in cases {
        assert_eq!(resolve(message, &payload), expected, "{message}");
    }
}

#[test]
fn a_value_nobody_passed_takes_the_chain_and_one_no_conversion_describes_reports() {
    let (parser, reports) = recording();
    let payload = Object::new()
        .with("gone", Value::Undefined)
        .with("refuses", Value::host(Refuses))
        .with("inside", vec![Value::host(Refuses)]);
    let context = Context::new().payload(&payload);

    assert_eq!(parser.resolve("{{gone; default:D}}", &context), "D");
    assert_eq!(codes(&reports), Vec::<&str>::new());
    assert_eq!(parser.resolve("{{refuses; default:D}}", &context), "D");
    assert_eq!(parser.resolve("{{inside; default:D}}", &context), "D");
    assert_eq!(
        codes(&reports),
        ["unserializable-value", "unserializable-value"]
    );
}

#[test]
fn a_message_written_as_a_value_is_converted_before_it_is_read() {
    let payload = Object::new().with("x", "X");
    let context = Context::new().payload(&payload);
    let parser = Parser::new();

    assert_eq!(parser.resolve(&Value::from(5), &context), "5");
    assert_eq!(parser.resolve(&Value::from("{{x}}!"), &context), "X!");
    assert_eq!(parser.resolve(None::<&str>, &context), "");
    assert_eq!(parser.resolve(Message::Absent, &context), "");
    assert_eq!(parser.resolve(&Value::host(Refuses), &context), "");
    assert_eq!(parser.resolve(&String::from("{{x}}"), &context), "X");
}

#[test]
fn the_chain_reads_the_wrapper_then_the_payload_then_the_message() {
    let wrapped = Object::new()
        .with("w", Object::new().with("default", "wrapper"))
        .with("default", "payload");
    let plain = Object::new().with("default", "payload");

    assert_eq!(resolve("{{w; default:inline}}", &wrapped), "wrapper");
    assert_eq!(resolve("{{v; default:inline}}", &wrapped), "payload");
    assert_eq!(resolve("{{v; default:inline}}", &Object::new()), "inline");
    assert_eq!(resolve("{{v}}", &Object::new()), "");
    assert_eq!(resolve("{{v; default:}}", &plain), "payload");
    assert_eq!(
        resolve("{{v; default:{{w}}; default:second}}", &plain),
        "payload"
    );
    assert_eq!(
        resolve("{{v; default:first; default:second}}", &Object::new()),
        "first"
    );
    assert_eq!(resolve("{{v; Default:upper}}", &Object::new()), "");
}

#[test]
fn a_placeholder_naming_default_reads_that_entry_once() {
    let (parser, reports) = recording();
    let refusing = Object::new().with("default", Value::host(Refuses));
    let payload = Object::new().with("default", "D");

    assert_eq!(
        parser.resolve(
            "{{default; default:inline}}",
            &Context::new().payload(&refusing)
        ),
        "inline"
    );
    assert_eq!(codes(&reports), ["unserializable-value"]);
    assert_eq!(
        parser.resolve("{{default}}", &Context::new().payload(&payload)),
        "D"
    );
}

#[test]
fn a_wrapper_configures_its_value_and_a_near_miss_is_a_value() {
    let payload = Object::new()
        .with("wrapped", Object::new().with("value", 1))
        .with(
            "measured",
            Object::new().with("value", 1).with("unit", "kg"),
        )
        .with("empty", Object::new())
        .with("props", Object::new().with("props", Object::new()));

    assert_eq!(resolve("{{wrapped}}", &payload), "1");
    assert_eq!(
        resolve("{{measured}}", &payload),
        "{\"value\":1,\"unit\":\"kg\"}"
    );
    assert_eq!(resolve("{{empty; default:D}}", &payload), "{}");
    assert_eq!(resolve("{{props; default:D}}", &payload), "D");

    let unrecognized = Parser::builder().recognize_wrappers(false).build();

    assert_eq!(
        unrecognized.resolve("{{wrapped}}", &Context::new().payload(&payload)),
        "{\"value\":1}"
    );
}

#[test]
fn the_payload_default_is_the_fallback_itself_never_a_wrapper() {
    let payload = Object::new().with("default", Object::new().with("value", "root"));

    assert_eq!(resolve("{{default}}", &payload), "{\"value\":\"root\"}");
    assert_eq!(resolve("{{absent}}", &payload), "{\"value\":\"root\"}");
}

#[test]
fn a_comparison_selects_by_text_ignoring_case() {
    let message = "{{v:eq; strasse:yes; default:no}}";

    assert_eq!(resolve(message, &Object::new().with("v", "STRASSE")), "yes");
    assert_eq!(
        resolve(message, &Object::new().with("v", "stra\u{df}e")),
        "no"
    );

    let number = "{{v:eq; 2:two; default:other}}";

    assert_eq!(resolve(number, &Object::new().with("v", 2.0)), "two");
    assert_eq!(
        resolve(
            "{{v:eq; 2.0:two; default:other}}",
            &Object::new().with("v", 2)
        ),
        "other"
    );
    assert_eq!(
        resolve("{{v:ne; a:first; b:second}}", &Object::new().with("v", "A")),
        "second"
    );
    assert_eq!(
        resolve("{{v; a:first; b:second}}", &Object::new().with("v", "b")),
        "second"
    );
    assert_eq!(
        resolve("{{v; a; b:second}}", &Object::new().with("v", "a")),
        "a"
    );
    assert_eq!(
        resolve("{{v; a:; b:second}}", &Object::new().with("v", "a")),
        ""
    );
}

#[test]
fn a_numeric_comparison_reads_text_as_a_number() {
    let lt = "{{v:lt; 20:small; default:big}}";
    let gt = "{{v:gt; 15:over; default:under}}";
    let cases = [
        (lt, " 12 ", "small"),
        (lt, "12px", "big"),
        (gt, "0x10", "over"),
        (gt, "0x1g", "under"),
        (gt, "Infinity", "over"),
        (gt, "inf", "under"),
        (gt, "", "under"),
        (lt, "", "small"),
    ];

    for (message, value, expected) in cases {
        assert_eq!(
            resolve(message, &Object::new().with("v", value)),
            expected,
            "{value:?}"
        );
    }
}

#[test]
fn a_numeric_comparison_orders_its_keys_and_keeps_ties_in_source_order() {
    let five = Object::new().with("v", 5);

    assert_eq!(resolve("{{v:gt; 1:a; 3:b; x:c; 1:d}}", &five), "b");
    assert_eq!(resolve("{{v:gt; 1:a; 1:b}}", &five), "a");
    assert_eq!(resolve("{{v:lt; 9:a; 6:b; 6:c}}", &five), "b");
    assert_eq!(resolve("{{v:lt; 5:a; default:none}}", &five), "none");
    assert_eq!(resolve("{{v:lte; 5:a; default:none}}", &five), "a");
    assert_eq!(resolve("{{v:gte; 05:a; 4:b}}", &five), "b");
    assert_eq!(
        resolve("{{v:gte; 05:a; 4:b}}", &Object::new().with("v", "05")),
        "a"
    );
}

#[test]
fn a_selected_option_renders_the_placeholders_it_writes() {
    let payload = Object::new().with("v", "a").with("n", 3);

    assert_eq!(
        resolve("{{v; a:you have {{n}} left; default:none}}", &payload),
        "you have 3 left"
    );
    assert_eq!(
        resolve("{{v; b:{{n}}; default:{{n}} by default}}", &payload),
        "3 by default"
    );
}

#[test]
fn a_defect_in_the_message_reports_and_takes_the_chain() {
    let (parser, reports) = recording();
    let payload = Object::new().with("v", "a");
    let context = Context::new().payload(&payload);

    assert_eq!(parser.resolve("{{v:zz; default:D}}", &context), "D");
    assert_eq!(codes(&reports), ["unknown-modifier"]);
    assert_eq!(parser.resolve("{{v:eq; default:D}}", &context), "D");
    assert_eq!(parser.resolve("{{absent:gt}}", &context), "");
    assert_eq!(codes(&reports), ["missing-options", "missing-options"]);
    assert_eq!(parser.resolve("{{:eq}}{{ }}{{}}", &context), "");
    assert_eq!(codes(&reports), Vec::<&str>::new());
    assert_eq!(parser.resolve("{{:zz}}", &context), "");
    assert_eq!(codes(&reports), ["unknown-modifier"]);
    assert_eq!(parser.resolve("{{v:EQ; a:x}}", &context), "");
    assert_eq!(codes(&reports), ["unknown-modifier"]);
}

#[test]
fn reports_are_emitted_in_walk_order() {
    let (parser, reports) = recording();
    let payload = Object::new().with("v", "a");
    let context = Context::new().payload(&payload);

    assert_eq!(
        parser.resolve("{{a:zz; default:{{b:qq}}}}{{v:lt}}", &context),
        ""
    );
    assert_eq!(
        codes(&reports),
        ["unknown-modifier", "unknown-modifier", "missing-options"]
    );
}

#[test]
fn a_report_says_what_reported_and_whose_defect_it_is() {
    let (parser, reports) = recording();

    parser.resolve("x {{v\t:zz \"q\"\u{b}}} y", &Context::new().id("greeting"));

    let report = reports.lock().unwrap().pop().unwrap();

    assert_eq!(report.code, Code::UnknownModifier);
    assert_eq!(report.origin, Origin::Message);
    assert_eq!(report.origin.as_str(), "message");
    assert_eq!(report.id.as_deref(), Some("greeting"));
    assert_eq!(report.limit, None);
    assert_eq!(report.text, "{{v\\t:zz \\\"q\\\"\\u000b}}");
    assert_eq!(
        report.message,
        "A placeholder named a modifier this parser does not know."
    );

    parser.resolve("{{v:zz}}", &Context::new());

    assert_eq!(reports.lock().unwrap().pop().unwrap().id, None);
}

#[test]
fn a_report_cuts_its_text_on_a_character_boundary() {
    let (parser, reports) = recording();
    let message = format!("{{{{x{}:zz}}}}", "\u{e9}".repeat(100));

    parser.resolve(message.as_str(), &Context::new());

    let text = reports.lock().unwrap().pop().unwrap().text;

    assert_eq!(text, format!("{{{{x{}...", "\u{e9}".repeat(58)));
    assert_eq!(text.len(), 119 + 3);

    parser.resolve(
        format!("{{{{{}:zz}}}}", "a".repeat(113)).as_str(),
        &Context::new(),
    );

    assert_eq!(reports.lock().unwrap().pop().unwrap().text.len(), 120);
}

#[test]
fn a_report_handler_may_resolve_again_with_its_own_state() {
    let inner: Arc<Mutex<Vec<String>>> = Arc::default();
    let seen = Arc::clone(&inner);
    let parser = Parser::builder()
        .on_report(move |report| {
            let nested = Parser::builder()
                .on_report({
                    let seen = Arc::clone(&seen);
                    move |report| seen.lock().unwrap().push(format!("{:?}", report.id))
                })
                .build();
            let payload = Object::new().with("v", report.code.as_str());

            let resolved = nested.resolve(
                "{{v}}{{w:zz}}",
                &Context::new().payload(&payload).id("nested"),
            );

            seen.lock().unwrap().push(resolved);
        })
        .build();

    assert_eq!(
        parser.resolve("{{v:zz; default:D}}", &Context::new().id("outer")),
        "D"
    );
    assert_eq!(
        *inner.lock().unwrap(),
        ["Some(\"nested\")", "unknown-modifier"]
    );
}

#[test]
fn every_code_names_its_origin_and_its_limit() {
    let (parser, reports) = recording();
    let payload = Object::new().with("v", "a");

    parser.resolve(
        format!("{}x{}", "{{v; a:".repeat(9), "}}".repeat(9)).as_str(),
        &Context::new().payload(&payload),
    );

    let report = reports.lock().unwrap().pop().unwrap();

    assert_eq!(report.code.as_str(), "nesting-limit");
    assert_eq!(report.origin, Origin::Message);
    assert_eq!(report.limit, Some(8));
    assert_eq!(Code::FailedModifier.origin(), Origin::Payload);
    assert_eq!(Code::MissingLocale.origin().as_str(), "payload");
    assert_eq!(Code::ReadLimit.origin().as_str(), "limit");
}

#[test]
fn the_tree_and_the_resolution_read_the_same_placeholders() {
    // With nothing in the payload and no default anywhere, every placeholder
    // resolves to nothing and every other leaf of the root to its text.
    fn text_of(message: &str, tree: &Tree) -> String {
        tree.root()
            .children()
            .map(|node| {
                let spelling = &message[node.span()];

                match node.kind() {
                    NodeKind::Placeholder => String::new(),
                    NodeKind::Escape if node.cancels() == Some(true) => spelling[1..].to_owned(),
                    _ => spelling.to_owned(),
                }
            })
            .collect()
    }

    let messages = [
        "a{{b}}c",
        "\\{{v}}",
        "{{v\\}}",
        "{{v\\}}}",
        "}}{{",
        "{{a; b:{{c}}x}}",
        "{{{{a}}}}",
        "{{a\u{2028}}}",
        "x{{ }}y{{}}",
        "{{a\\",
        "\u{e9}{{\u{e9}}}\\\u{e9}",
        "{{a; b:{{c}}",
        "\\\\{{a}}\\ \\:",
        "{{a:b:c; d:e:f;}}",
    ];

    for message in messages {
        assert_eq!(
            Parser::new().resolve(message, &Context::new()),
            text_of(message, &cst(message)),
            "{message:?}"
        );
    }
}

#[test]
fn a_parser_is_shared_across_threads() {
    fn shared<T: Send + Sync + Clone>() {}
    fn sendable<T: Send + Sync>() {}

    shared::<Parser>();
    sendable::<Value>();
    sendable::<Tree>();
    sendable::<Report>();

    let parser = Parser::new();
    let payload = Object::new().with("v", "x");
    let handles: Vec<_> = (0..4)
        .map(|_| {
            let parser = parser.clone();
            let payload = payload.clone();

            std::thread::spawn(move || parser.resolve("{{v}}", &Context::new().payload(&payload)))
        })
        .collect();

    for handle in handles {
        assert_eq!(handle.join().unwrap(), "x");
    }
}

#[test]
fn a_parser_claims_the_core_level() {
    assert_eq!(Parser::new().levels(), [Level::Core]);
    assert_eq!(Level::Core.as_str(), "core");
    assert_eq!(Level::Intl.as_str(), "intl");
    assert_eq!(Level::Extensions.as_str(), "extensions");
    assert_eq!(
        format!("{:?}", Parser::default()),
        "Parser { levels: [Core], recognize_wrappers: true, on_report: false }"
    );
}
