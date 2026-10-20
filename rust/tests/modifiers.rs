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
    Context, MAX_HOST_DEPTH, MAX_NESTING, Modifier, ModifierError, ModifierInput, Object, Parser,
    ParserBuilder, Report, Value,
};

type Answer = Result<Option<Value>, ModifierError>;

fn recording(builder: ParserBuilder) -> (Parser, Arc<Mutex<Vec<Report>>>) {
    let reports: Arc<Mutex<Vec<Report>>> = Arc::default();
    let sink = Arc::clone(&reports);
    let parser = builder
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

fn text(text: impl Into<String>) -> Answer {
    Ok(Some(Value::from(text.into())))
}

// A modifier answering with what it was handed, written out.
fn shown(input: &ModifierInput<'_>) -> Answer {
    let options: Vec<String> = (0..input.options())
        .map(|at| {
            format!(
                "{}={}",
                input.option_key(at).unwrap(),
                input.option_value(at).unwrap()
            )
        })
        .collect();

    text(format!("{}[{}]", input.value(), options.join(",")))
}

// A modifier that counts its calls.
fn counting(calls: &Arc<AtomicUsize>, answer: &'static str) -> impl Modifier + 'static {
    let calls = Arc::clone(calls);

    move |_: &ModifierInput<'_>| {
        calls.fetch_add(1, Ordering::Relaxed);
        text(answer)
    }
}

#[test]
fn a_registered_modifier_answers_for_its_placeholder() {
    let parser = Parser::builder()
        .modifier("upper", |input: &ModifierInput<'_>| {
            text(input.value().to_uppercase())
        })
        .modifier("shown", shown)
        .build();
    let payload = Object::new().with("name", "Ada").with("count", 3);
    let context = Context::new().payload(&payload);

    assert_eq!(parser.resolve("Hi {{name:upper}}!", &context), "Hi ADA!");
    assert_eq!(parser.resolve("{{count:shown}}", &context), "3[]");
    assert_eq!(
        parser.resolve("{{ count : shown ; one : a ; two:{{name}} }}", &context),
        "3[one=a,two=Ada]"
    );
    assert_eq!(
        format!("{parser:?}"),
        "Parser { levels: [Core, Extensions], recognize_wrappers: true, on_report: false, modifiers: 2 }"
    );
}

#[test]
fn a_modifier_registered_under_a_comparison_replaces_it() {
    let (builtin, builtin_reports) = recording(Parser::builder());
    let (hosted, hosted_reports) = recording(
        Parser::builder()
            .modifier("eq", |_: &ModifierInput<'_>| text("first"))
            .modifier("eq", shown)
            .modifier("lt", shown),
    );
    let payload = Object::new().with("v", "a");
    let context = Context::new().payload(&payload);

    // Written without a modifier, a selection asks the modifier answering to
    // `eq`: the host's, once it registered one.
    assert_eq!(builtin.resolve("{{v; a:x; b:y}}", &context), "x");
    assert_eq!(hosted.resolve("{{v; a:x; b:y}}", &context), "a[a=x,b=y]");
    assert_eq!(hosted.resolve("{{v:eq; A:x}}", &context), "a[A=x]");

    // The host's modifier is no selection, so it needs no options.
    assert_eq!(builtin.resolve("{{v:lt; default:D}}", &context), "D");
    assert_eq!(codes(&builtin_reports), ["missing-options"]);
    assert_eq!(hosted.resolve("{{v:lt; default:D}}", &context), "a[]");
    assert_eq!(hosted.resolve("{{v:eq}}", &context), "a[]");
    assert_eq!(codes(&hosted_reports), Vec::<&str>::new());

    // Every other comparison is still the format's.
    assert_eq!(hosted.resolve("{{v:ne; a:x; default:D}}", &context), "D");
    assert_eq!(hosted.resolve("{{v:gt; default:D}}", &context), "D");
    assert_eq!(codes(&hosted_reports), ["missing-options"]);
}

#[test]
fn a_modifier_is_looked_up_by_its_exact_name() {
    let (parser, reports) = recording(
        Parser::builder()
            .modifier("Upper", |_: &ModifierInput<'_>| text("U"))
            .modifier("__proto__", |_: &ModifierInput<'_>| text("P")),
    );
    let payload = Object::new().with("v", "a");
    let context = Context::new().payload(&payload);

    assert_eq!(parser.resolve("{{v:Upper}}{{v:__proto__}}", &context), "UP");
    assert_eq!(codes(&reports), Vec::<&str>::new());

    for name in [
        "upper",
        "UPPER",
        "toString",
        "constructor",
        "EQ",
        "hasOwnProperty",
    ] {
        let message = format!("{{{{v:{name}; a:x; default:D}}}}");

        assert_eq!(parser.resolve(message.as_str(), &context), "D", "{name}");
        assert_eq!(codes(&reports), ["unknown-modifier"], "{name}");
    }
}

#[test]
fn a_modifier_answering_nothing_leaves_the_placeholder_its_chain() {
    let (parser, reports) = recording(
        Parser::builder()
            .modifier("none", |_: &ModifierInput<'_>| Ok(None))
            .modifier("undefined", |_: &ModifierInput<'_>| {
                Ok(Some(Value::Undefined))
            })
            .modifier("fails", |_: &ModifierInput<'_>| {
                Err(ModifierError::new("no answer"))
            })
            .modifier("empty", |_: &ModifierInput<'_>| text("")),
    );
    let payload = Object::new().with("v", "a");
    let context = Context::new().payload(&payload);

    assert_eq!(parser.resolve("{{v:none; default:D}}", &context), "D");
    assert_eq!(parser.resolve("{{v:undefined; default:D}}", &context), "D");
    assert_eq!(parser.resolve("{{v:none}}", &context), "");
    assert_eq!(codes(&reports), Vec::<&str>::new());

    // The empty string is an answer, and the chain is not consulted.
    assert_eq!(parser.resolve("{{v:empty; default:D}}", &context), "");
    assert_eq!(codes(&reports), Vec::<&str>::new());

    assert_eq!(parser.resolve("{{v:fails; default:D}}", &context), "D");
    assert_eq!(codes(&reports), ["failed-modifier"]);
    assert_eq!(ModifierError::new("no answer").to_string(), "no answer");
}

struct Failing;

impl fmt::Display for Failing {
    fn fmt(&self, _: &mut fmt::Formatter<'_>) -> fmt::Result {
        Err(fmt::Error)
    }
}

#[test]
fn an_answer_converts_as_a_payload_value_does() {
    let (parser, reports) = recording(
        Parser::builder()
            .modifier("object", |input: &ModifierInput<'_>| {
                Ok(Some(Value::from(
                    Object::new()
                        .with("value", input.value())
                        .with("list", vec![1, 2]),
                )))
            })
            .modifier("half", |_: &ModifierInput<'_>| Ok(Some(Value::from(0.5))))
            .modifier("null", |_: &ModifierInput<'_>| Ok(Some(Value::Null)))
            .modifier("failing", |_: &ModifierInput<'_>| {
                Ok(Some(Value::host(Failing)))
            }),
    );
    let payload = Object::new().with("v", "a");
    let context = Context::new().payload(&payload);

    assert_eq!(
        parser.resolve("{{v:object}}", &context),
        r#"{"value":"a","list":[1,2]}"#
    );
    assert_eq!(
        parser.resolve("{{v:half}}|{{v:null}}", &context),
        "0.5|null"
    );
    assert_eq!(codes(&reports), Vec::<&str>::new());
    assert_eq!(parser.resolve("{{v:failing; default:D}}", &context), "D");
    assert_eq!(codes(&reports), ["unserializable-value"]);
}

#[test]
fn an_absent_value_never_reaches_a_modifier() {
    let calls = Arc::new(AtomicUsize::new(0));
    let (parser, reports) = recording(Parser::builder().modifier("m", counting(&calls, "M")));
    let payload = Object::new()
        .with("v", "a")
        .with("u", Value::Undefined)
        .with("w", Object::new().with("default", "W"));
    let context = Context::new().payload(&payload);

    assert_eq!(
        parser.resolve(
            "{{missing:m; default:D}}{{u:m}}{{w:m}}{{:m; default:K}}",
            &context
        ),
        "DWK"
    );
    assert_eq!(calls.load(Ordering::Relaxed), 0);
    assert_eq!(codes(&reports), Vec::<&str>::new());

    // Nor does a placeholder nested past the limit, which takes its chain.
    let nested = (0..MAX_NESTING).fold("{{v:m; default:deep}}".to_owned(), |inner, _| {
        format!("{{{{v; a:{inner}}}}}")
    });

    assert_eq!(parser.resolve(nested.as_str(), &context), "deep");
    assert_eq!(calls.load(Ordering::Relaxed), 0);
    assert_eq!(codes(&reports), ["nesting-limit"]);
    assert_eq!(parser.resolve("{{v:m}}", &context), "M");
    assert_eq!(calls.load(Ordering::Relaxed), 1);
}

#[test]
fn the_options_are_handed_over_as_the_placeholder_writes_them() {
    let parser = Parser::builder().modifier("shown", shown).build();
    let payload = Object::new().with("v", "a").with("w", "W");
    let context = Context::new().payload(&payload);

    // In source order with duplicates kept; the inline default is none of
    // them, a segment naming no key is skipped, and a key alone stands for
    // itself.
    assert_eq!(
        parser.resolve(
            r"{{v:shown; a:1; b; a:{{w}}; default:D; ; x:; default:E; y\;z:\{q\}}}",
            &context
        ),
        "a[a=1,b=b,a=W,x=,y;z={q}]"
    );

    let parser = Parser::builder()
        .modifier("probe", |input: &ModifierInput<'_>| {
            text(format!(
                "{:?}{:?}{:?}",
                input.option_key(1),
                input.option_value(1),
                input.option_value(2)
            ))
        })
        .build();

    assert_eq!(parser.resolve("{{v:probe; a:1}}", &context), "NoneNoneNone");
}

#[test]
fn an_option_is_rendered_on_the_first_ask_and_not_again() {
    let (parser, reports) = recording(Parser::builder().modifier(
        "first",
        |input: &ModifierInput<'_>| {
            let once = input.option_value(0).unwrap();
            let twice = input.option_value(0).unwrap();

            text(format!("{once}{twice}"))
        },
    ));
    let payload = Object::new().with("v", "a");
    let context = Context::new().payload(&payload);

    // Each option holds a placeholder that reports where it is rendered: the
    // one asked for twice reports once, and the one never asked for, however
    // deeply it nests, never does.
    let deep = (0..100).fold("{{v:never}}".to_owned(), |inner, _| {
        format!("{{{{v; a:{inner}}}}}")
    });
    let message = format!("{{{{v:first; a:{{{{v:asked; default:x}}}}; b:{deep}}}}}");

    assert_eq!(parser.resolve(message.as_str(), &context), "xx");
    assert_eq!(codes(&reports), ["unknown-modifier"]);
}

#[test]
fn the_default_is_walked_on_the_first_ask_and_not_again() {
    let (parser, reports) = recording(
        Parser::builder()
            .modifier("twice", |input: &ModifierInput<'_>| {
                assert_eq!(input.default(), input.default());
                Ok(None)
            })
            .modifier("shows", |input: &ModifierInput<'_>| {
                text(format!("<{}>", input.default()))
            })
            .modifier("ignores", |_: &ModifierInput<'_>| text("I")),
    );
    let payload = Object::new()
        .with("v", "a")
        .with(
            "bad",
            Object::new()
                .with("value", "b")
                .with("default", Value::host(Failing)),
        )
        .with("default", "P");
    let context = Context::new().payload(&payload);

    let empty = Object::new().with("v", "a");

    // The inline default renders once: for the modifier, and from the memo
    // for the chain the placeholder takes afterwards.
    assert_eq!(
        parser.resolve(
            "{{v:twice; default:{{v:zz; default:Q}}}}",
            &Context::new().payload(&empty)
        ),
        "Q"
    );
    assert_eq!(codes(&reports), ["unknown-modifier"]);

    // A link that cannot become text is reported once, however often it is
    // asked for; the next link answers.
    assert_eq!(parser.resolve("{{bad:twice}}", &context), "P");
    assert_eq!(codes(&reports), ["unserializable-value"]);
    assert_eq!(parser.resolve("{{bad:shows}}", &context), "<P>");
    assert_eq!(codes(&reports), ["unserializable-value"]);

    // A link nobody asks for is never read.
    assert_eq!(parser.resolve("{{bad:ignores}}", &context), "I");
    assert_eq!(codes(&reports), Vec::<&str>::new());

    // The payload's `default`, named as the key, is the value and the link
    // at once.
    assert_eq!(parser.resolve("{{default:shows}}", &context), "<P>");
    assert_eq!(
        parser.resolve("{{v:shows}}", &Context::new().payload(&empty)),
        "<>"
    );
}

#[test]
fn the_props_compose_under_the_modifier_name() {
    let props_of = |input: &ModifierInput<'_>| Ok(Some(Value::from(input.props().to_object())));
    let parser = Parser::builder()
        .modifier("m", props_of)
        .modifier("eq", props_of)
        .modifier_defaults(
            Object::new()
                .with("m", Object::new().with("a", 1).with("b", 1).with("c", 1))
                .with("other", Object::new().with("z", 1)),
        )
        .build();
    let props = Object::new()
        .with(
            "m",
            Object::new()
                .with("b", 2)
                .with("c", Value::Undefined)
                .with("d", 2),
        )
        .with("eq", Object::new().with("e", 1));
    let payload = Object::new().with("v", "x").with(
        "w",
        Object::new().with("value", "x").with(
            "props",
            Object::new()
                .with("m", Object::new().with("c", Value::Null))
                .with("eq", "not an object"),
        ),
    );
    let context = Context::new().payload(&payload).props(&props);

    // `Undefined` names nothing and `Null` names a property; a property keeps
    // the place it was first composed at.
    assert_eq!(
        parser.resolve("{{v:m}}", &context),
        r#"{"a":1,"b":2,"c":1,"d":2}"#
    );
    assert_eq!(
        parser.resolve("{{w:m}}", &context),
        r#"{"a":1,"b":2,"c":null,"d":2}"#
    );
    // A placeholder written without a modifier composes under `eq`, and a
    // layer holding no object under the name contributes nothing.
    assert_eq!(parser.resolve("{{w; a:x}}", &context), r#"{"e":1}"#);
    assert_eq!(
        parser.resolve("{{v:m}}", &Context::new().payload(&payload)),
        r#"{"a":1,"b":1,"c":1}"#
    );

    let bare = Parser::builder().modifier("m", props_of).build();

    assert_eq!(
        bare.resolve("{{v:m}}", &Context::new().payload(&payload)),
        "{}"
    );
}

#[test]
fn the_locale_is_handed_over_verbatim() {
    let parser = Parser::builder()
        .modifier("locale", |input: &ModifierInput<'_>| {
            text(format!("{:?}", input.locale()))
        })
        .build();
    let payload = Object::new().with("v", "a");
    let context = Context::new().payload(&payload);

    assert_eq!(parser.resolve("{{v:locale}}", &context), "None");
    assert_eq!(
        parser.resolve("{{v:locale}}", &context.locale("")),
        r#"Some("")"#
    );
    assert_eq!(
        parser.resolve("{{v:locale}}", &context.locale("not a locale")),
        r#"Some("not a locale")"#
    );
}

#[test]
fn a_modifier_holds_every_answer_it_asked_for_at_once() {
    let parser = Parser::builder()
        .modifier("all", |input: &ModifierInput<'_>| {
            let value = input.value();
            let first = input.option_value(0);
            let second = input.option_value(1);
            let default = input.default();
            let props = input.props();

            text(format!(
                "{value}{first:?}{second:?}{default}{}",
                props.len()
            ))
        })
        .build();
    let payload = Object::new().with("v", "a");

    assert_eq!(
        parser.resolve(
            "{{v:all; x:1; y:2; default:D}}",
            &Context::new().payload(&payload)
        ),
        r#"aSome("1")Some("2")D0"#
    );
}

#[test]
fn a_resolution_started_by_a_modifier_keeps_its_own_budgets_and_id() {
    let (inner, inner_reports) = recording(Parser::builder());
    let big = Object::new().with("big", "x".repeat(90_000));
    let (outer, outer_reports) = recording(Parser::builder().modifier(
        "inner",
        move |_: &ModifierInput<'_>| {
            let read = inner.resolve(
                "{{big}}{{big:zz; default:}}",
                &Context::new().payload(&big).id("inner"),
            );

            text(read.len().to_string())
        },
    ));
    let payload = Object::new().with("v", "a");

    assert_eq!(
        outer.resolve(
            "{{v:inner}}{{v:inner}}{{v:zz}}",
            &Context::new().payload(&payload).id("outer")
        ),
        "9000090000"
    );

    let inner_ids: Vec<_> = std::mem::take(&mut *inner_reports.lock().unwrap())
        .into_iter()
        .map(|report| (report.code.as_str(), report.id))
        .collect();
    let outer_ids: Vec<_> = std::mem::take(&mut *outer_reports.lock().unwrap())
        .into_iter()
        .map(|report| (report.code.as_str(), report.id))
        .collect();

    assert_eq!(
        inner_ids,
        [
            ("unknown-modifier", Some("inner".into())),
            ("unknown-modifier", Some("inner".into()))
        ]
    );
    assert_eq!(outer_ids, [("unknown-modifier", Some("outer".into()))]);
}

// A payload `default` describing itself by resolving a message whose modifier
// asks for that default again: a resolution reaching itself through a chain
// link, on the stack the README says a thread needs.
struct Link {
    parser: Arc<Mutex<Option<(Parser, Object)>>>,
}

impl fmt::Display for Link {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let (parser, payload) = self.parser.lock().unwrap().clone().unwrap();

        formatter.write_str(&parser.resolve("{{v:d}}", &Context::new().payload(&payload)))
    }
}

#[test]
fn a_chain_link_reaching_itself_stops_at_the_host_depth() {
    std::thread::Builder::new()
        .stack_size(512 * 1024)
        .spawn(|| {
            let (parser, reports) = recording(
                Parser::builder().modifier("d", |input: &ModifierInput<'_>| {
                    text(format!("<{}>", input.default()))
                }),
            );
            let cell: Arc<Mutex<Option<(Parser, Object)>>> = Arc::default();
            let payload = Object::new().with("v", "a").with(
                "default",
                Value::host(Link {
                    parser: Arc::clone(&cell),
                }),
            );

            *cell.lock().unwrap() = Some((parser.clone(), payload.clone()));

            // The link the thread had no room for describes nothing, and the
            // modifier asking for it is handed the end of the chain.
            assert_eq!(
                parser.resolve("{{v:d}}", &Context::new().payload(&payload)),
                format!(
                    "{}{}",
                    "<".repeat(MAX_HOST_DEPTH),
                    ">".repeat(MAX_HOST_DEPTH)
                )
            );
            assert_eq!(codes(&reports), ["unserializable-value"]);

            *cell.lock().unwrap() = None;
        })
        .unwrap()
        .join()
        .unwrap();
}
