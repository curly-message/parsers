#![cfg(feature = "intl")]
#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

// Resolution through `IcuIntl`, configured explicitly, as an application
// configures it. What each request formats to is the parity test's; this is
// what a message resolves to around it.

use std::fmt;
use std::sync::{Arc, Mutex};

use curly_message_parser::{
    Api, Context, IcuIntl, Level, Object, Parser, Report, UNEXPRESSIBLE, Value,
};

fn recording() -> (Parser, Arc<Mutex<Vec<&'static str>>>) {
    let reports: Arc<Mutex<Vec<&'static str>>> = Arc::default();
    let sink = Arc::clone(&reports);
    let parser = Parser::builder()
        .intl(IcuIntl::new())
        .on_report(move |report: &Report| sink.lock().unwrap().push(report.code.as_str()))
        .build();

    (parser, reports)
}

#[test]
fn the_formatting_modifiers_format_in_the_callers_locale() {
    let (parser, reports) = recording();
    let payload = Object::new()
        .with("n", 1234.5678)
        .with("price", 2.5)
        .with("when", "2024-03-05T10:00:00Z")
        .with("delta", -86_400_000);
    let props = Object::new().with("currency", Object::new().with("currency", "EUR"));
    let context = Context::new().payload(&payload).props(&props);

    assert_eq!(
        parser.resolve(
            "{{n:number}} | {{price:currency}} | {{when:date}} | {{delta:ago}}",
            &context.locale("en")
        ),
        "1,234.57 | \u{20ac}2.50 | 3/5/2024 | yesterday"
    );
    assert_eq!(
        parser.resolve("{{n:number}} | {{delta:ago}}", &context.locale("cs")),
        "1\u{a0}234,57 | v\u{10d}era"
    );
    assert!(reports.lock().unwrap().is_empty());
}

#[test]
fn a_date_without_a_zone_is_in_utc() {
    let (parser, _) = recording();
    let payload = Object::new().with("when", Value::date(1_709_632_800_000.0));
    let props = Object::new().with("date", Object::new().with("timeStyle", "long"));
    let prague = Object::new().with(
        "date",
        Object::new()
            .with("timeStyle", "long")
            .with("timeZone", "Europe/Prague"),
    );
    let context = Context::new().payload(&payload).locale("en");

    assert_eq!(
        parser.resolve("{{when:date}}", &context.props(&props)),
        "10:00:00\u{202f}AM UTC"
    );
    assert_eq!(
        parser.resolve("{{when:date}}", &context.props(&prague)),
        "11:00:00\u{202f}AM GMT+1"
    );
}

// Czech `1` is `one` and `1.0` is `many`: a count and the word agreeing
// with it are decided on the digits `number` shows.
#[test]
fn plural_selects_on_the_digits_number_shows() {
    let (parser, _) = recording();
    let payload = Object::new().with("n", 1).with("almost", 1.999);
    let shown = Object::new().with("number", Object::new().with("minimumFractionDigits", 1));
    let context = Context::new().payload(&payload).locale("cs");
    let message =
        "{{n:number}} {{n:plural; one:soubor; few:soubory; many:souboru; other:soubor\u{16f}}}";

    assert_eq!(parser.resolve(message, &context), "1 soubor");
    assert_eq!(
        parser.resolve(message, &context.props(&shown)),
        "1,0 souboru"
    );
    assert_eq!(
        parser.resolve(
            "{{almost:number}} {{almost:plural; one:a; few:b; many:c; other:d}}",
            &context
        ),
        "2 b"
    );
}

#[test]
fn ordinal_selects_a_position() {
    let (parser, _) = recording();
    let message = "{{n:ordinal; one:st; two:nd; few:rd; other:th}}";
    let context = Context::new().locale("en");
    let suffixes: Vec<String> = [1, 2, 3, 4, 11, 12, 13, 21, 22, 101]
        .into_iter()
        .map(|n| parser.resolve(message, &context.payload(&Object::new().with("n", n))))
        .collect();

    assert_eq!(
        suffixes,
        ["st", "nd", "rd", "th", "th", "th", "th", "st", "nd", "st"]
    );
}

#[test]
fn a_request_the_facility_refuses_takes_the_chain() {
    let (parser, reports) = recording();
    let payload = Object::new().with("n", 1);
    let props = Object::new()
        .with("date", Object::new().with("timeZone", "Mars/Olympus"))
        .with(
            "number",
            Object::new()
                .with("minimumFractionDigits", 3)
                .with("maximumFractionDigits", 1),
        )
        .with("currency", Object::new());
    let context = Context::new().payload(&payload).props(&props);

    assert_eq!(
        parser.resolve(
            "{{n:date; default:a}}{{n:number; default:b}}{{n:currency; default:c}}",
            &context.locale("en")
        ),
        "abc"
    );
    assert_eq!(
        parser.resolve("{{n:plural; one:x; default:d}}", &context.locale("en_US")),
        "d"
    );
    assert_eq!(
        std::mem::take(&mut *reports.lock().unwrap()),
        vec!["failed-modifier"; 4]
    );
}

// Section 11.2: a property the facility cannot express formats with what it
// does express, rather than taking the chain.
#[test]
fn an_unexpressible_property_formats_with_what_is_expressed() {
    let (parser, reports) = recording();
    let payload = Object::new().with("v", 2).with("n", 1234);
    let props = Object::new()
        .with(
            "currency",
            Object::new()
                .with("currency", "USD")
                .with("maximumFractionDigits", 0),
        )
        .with("number", Object::new().with("notation", "compact"));
    let context = Context::new().payload(&payload).props(&props).locale("en");

    assert_eq!(
        parser.resolve("{{v:currency}} {{n:number}}", &context),
        "$2.00 1,234"
    );
    assert!(reports.lock().unwrap().is_empty());

    let declared = |api: Api, property: &str| {
        UNEXPRESSIBLE
            .iter()
            .any(|entry| entry.api == api && entry.property == property)
    };

    assert!(declared(Api::NumberFormat, "notation"));
    assert!(declared(Api::NumberFormat, "maximumFractionDigits"));
    assert!(!declared(Api::NumberFormat, "signDisplay"));
    assert!(!declared(Api::DateTimeFormat, "timeStyle"));
}

#[test]
fn a_parser_built_without_the_supplier_knows_none_of_the_six() {
    let reports: Arc<Mutex<Vec<&'static str>>> = Arc::default();
    let sink = Arc::clone(&reports);
    let parser = Parser::builder()
        .on_report(move |report: &Report| sink.lock().unwrap().push(report.code.as_str()))
        .build();
    let payload = Object::new().with("v", 1);

    assert_eq!(
        parser.resolve(
            "{{v:number; default:D}}",
            &Context::new().payload(&payload).locale("en")
        ),
        "D"
    );
    assert_eq!(*reports.lock().unwrap(), ["unknown-modifier"]);
    assert_eq!(parser.levels(), [Level::Core, Level::Extensions]);
    assert_eq!(
        Parser::builder().intl(IcuIntl::new()).build().levels(),
        [Level::Core, Level::Intl, Level::Extensions]
    );
}

// A property whose value describes itself by resolving the message that reads
// it again, over the same payload and props: a resolution reaching itself
// through the facility reading a request's options.
struct Again {
    resolution: Arc<Mutex<Option<(Parser, Object, Object)>>>,
    message: &'static str,
    text: &'static str,
}

impl fmt::Display for Again {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let (parser, payload, props) = self.resolution.lock().unwrap().clone().unwrap();

        parser.resolve(
            self.message,
            &Context::new().payload(&payload).props(&props).locale("en"),
        );

        formatter.write_str(self.text)
    }
}

// Each modifier whose request reads a property through `Display`, on the stack
// the README says a thread needs, in a debug build too: the innermost property
// the thread had no room for has no text, and the request holding it is
// refused.
#[test]
fn a_property_reaching_itself_stops_at_the_host_depth() {
    let cases: [(
        &'static str,
        &'static str,
        &'static str,
        &'static str,
        &'static str,
    ); 6] = [
        (
            "{{n:date}}",
            "date",
            "timeZone",
            "UTC",
            "Thursday, January 1, 1970 at 12:00:00\u{202f}AM Coordinated Universal Time",
        ),
        ("{{n:number}}", "number", "style", "decimal", "0"),
        ("{{n:currency}}", "currency", "currency", "USD", "$0.00"),
        (
            "{{n:plural; other:x}}",
            "plural",
            "minimumIntegerDigits",
            "1",
            "x",
        ),
        (
            "{{n:ordinal; other:x}}",
            "ordinal",
            "localeMatcher",
            "lookup",
            "x",
        ),
        ("{{n:ago}}", "ago", "style", "long", "now"),
    ];

    for (message, name, property, text, formatted) in cases {
        std::thread::Builder::new()
            .stack_size(512 * 1024)
            .spawn(move || {
                let (parser, reports) = recording();
                let resolution: Arc<Mutex<Option<(Parser, Object, Object)>>> = Arc::default();
                let again = Value::host(Again {
                    resolution: Arc::clone(&resolution),
                    message,
                    text,
                });
                let mut layer = Object::new().with(property, again);

                if name == "date" {
                    layer = layer.with("dateStyle", "full").with("timeStyle", "full");
                }

                let payload = Object::new().with("n", 0);
                let props = Object::new().with(name, layer);

                *resolution.lock().unwrap() =
                    Some((parser.clone(), payload.clone(), props.clone()));

                assert_eq!(
                    parser.resolve(
                        message,
                        &Context::new().payload(&payload).props(&props).locale("en")
                    ),
                    formatted
                );
                assert_eq!(*reports.lock().unwrap(), ["failed-modifier"]);

                *resolution.lock().unwrap() = None;
            })
            .unwrap()
            .join()
            .unwrap();
    }
}
