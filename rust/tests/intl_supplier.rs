#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

// The Intl level over a supplier of the test's own, which records each
// request it is handed: what the modifiers ask of a facility, built with no
// feature at all.

use std::sync::{Arc, Mutex};

use curly_message_parser::{
    Api, Category, Context, Input, Intl, Level, ModifierInput, Object, Parser, ParserBuilder,
    Props, Refusal, Report, Request, Unit, Value,
};

#[derive(Clone, Copy)]
enum Answers {
    Text,
    Category(Category),
    Refuse,
}

// A request, written out: the facility, the options in the order they were
// composed, and the input.
type Seen = (String, String, String, String);

#[derive(Clone)]
struct Recording {
    seen: Arc<Mutex<Vec<Seen>>>,
    answers: Answers,
}

fn options(options: &Props<'_>) -> String {
    options
        .iter()
        .map(|(key, value)| format!("{key}={value:?}"))
        .collect::<Vec<_>>()
        .join(" ")
}

fn input(input: Input) -> String {
    match input {
        Input::Number(number) => format!("{number:?}"),
        Input::Relative(count, unit) => format!("{count:?} {}", unit.as_str()),
    }
}

impl Recording {
    fn record(&self, locale: &str, request: &Request) {
        self.seen.lock().unwrap().push((
            locale.to_owned(),
            request.api.as_str().to_owned(),
            options(&request.options),
            input(request.input),
        ));
    }
}

impl Intl for Recording {
    fn format(&self, locale: &str, request: &Request) -> Result<String, Refusal> {
        self.record(locale, request);

        match self.answers {
            Answers::Refuse => Err(Refusal::new("refused")),
            _ => Ok(format!("<{}>", input(request.input))),
        }
    }

    fn select(&self, locale: &str, request: &Request) -> Result<Category, Refusal> {
        self.record(locale, request);

        match self.answers {
            Answers::Category(category) => Ok(category),
            Answers::Refuse => Err(Refusal::new("refused")),
            Answers::Text => Ok(Category::Other),
        }
    }
}

struct Run {
    parser: Parser,
    seen: Arc<Mutex<Vec<Seen>>>,
    reports: Arc<Mutex<Vec<&'static str>>>,
}

impl Run {
    fn new(answers: Answers) -> Self {
        Self::with(answers, |builder| builder)
    }

    fn with(answers: Answers, configure: impl FnOnce(ParserBuilder) -> ParserBuilder) -> Self {
        let seen: Arc<Mutex<Vec<Seen>>> = Arc::default();
        let reports: Arc<Mutex<Vec<&'static str>>> = Arc::default();
        let sink = Arc::clone(&reports);
        let builder = Parser::builder()
            .intl(Recording {
                seen: Arc::clone(&seen),
                answers,
            })
            .on_report(move |report: &Report| sink.lock().unwrap().push(report.code.as_str()));
        let parser = configure(builder).build();

        Self {
            parser,
            seen,
            reports,
        }
    }

    fn resolve(&self, message: &str, context: &Context<'_>) -> String {
        self.parser.resolve(message, context)
    }

    fn seen(&self) -> Vec<(String, String, String, String)> {
        std::mem::take(&mut *self.seen.lock().unwrap())
    }

    fn reports(&self) -> Vec<&'static str> {
        std::mem::take(&mut *self.reports.lock().unwrap())
    }
}

fn seen(locale: &str, api: &str, options: &str, input: &str) -> Seen {
    (
        locale.to_owned(),
        api.to_owned(),
        options.to_owned(),
        input.to_owned(),
    )
}

#[test]
fn number_asks_for_two_fraction_digits_unless_a_layer_names_the_maximum() {
    let run = Run::new(Answers::Text);
    let payload = Object::new().with("v", " 12 ");
    let props = Object::new().with("number", Object::new().with("minimumFractionDigits", 3));
    let named = Object::new().with("number", Object::new().with("maximumFractionDigits", 0));
    let context = Context::new().payload(&payload).locale("en-GB");

    assert_eq!(run.resolve("{{v:number}}", &context), "<12.0>");
    assert_eq!(
        run.resolve("{{v:number}}", &context.props(&props)),
        "<12.0>"
    );
    assert_eq!(
        run.resolve("{{v:number}}", &context.props(&named)),
        "<12.0>"
    );
    assert_eq!(
        run.seen(),
        [
            seen(
                "en-GB",
                "NumberFormat",
                "maximumFractionDigits=Number(2.0)",
                "12.0"
            ),
            seen(
                "en-GB",
                "NumberFormat",
                "minimumFractionDigits=Integer(3) maximumFractionDigits=Number(3.0)",
                "12.0"
            ),
            seen(
                "en-GB",
                "NumberFormat",
                "maximumFractionDigits=Integer(0)",
                "12.0"
            ),
        ]
    );
    assert!(run.reports().is_empty());
}

#[test]
fn the_layers_compose_per_property_under_the_modifiers_name() {
    let run = Run::with(Answers::Text, |builder| {
        builder.modifier_defaults(
            Object::new().with(
                "number",
                Object::new()
                    .with("useGrouping", false)
                    .with("style", "decimal")
                    .with("signDisplay", "always"),
            ),
        )
    });
    let props = Object::new()
        .with(
            "number",
            Object::new()
                .with("style", "percent")
                .with("signDisplay", Value::Undefined),
        )
        .with("date", Object::new().with("timeZone", "UTC"));
    let payload = Object::new().with(
        "w",
        Object::new().with("value", 1).with(
            "props",
            Object::new().with("number", Object::new().with("useGrouping", Value::Null)),
        ),
    );
    let context = Context::new().payload(&payload).props(&props).locale("en");

    assert_eq!(run.resolve("{{w:number}}", &context), "<1.0>");
    assert_eq!(
        run.seen(),
        [seen(
            "en",
            "NumberFormat",
            "useGrouping=Null style=String(\"percent\") signDisplay=String(\"always\") \
             maximumFractionDigits=Number(2.0)",
            "1.0"
        )]
    );
}

#[test]
fn currency_multiplies_by_its_ratio_and_pins_its_style() {
    let run = Run::new(Answers::Text);
    let payload = Object::new().with("v", "2.5").with("huge", "1e308");
    let props = Object::new().with(
        "currency",
        Object::new()
            .with("currency", "EUR")
            .with("ratio", 100)
            .with("style", "percent"),
    );
    let blank = Object::new().with("currency", Object::new().with("ratio", "  "));
    let overflowing = Object::new().with("currency", Object::new().with("ratio", 10));
    let context = Context::new().payload(&payload).locale("de");

    assert_eq!(
        run.resolve("{{v:currency}}", &context.props(&props)),
        "<250.0>"
    );
    assert_eq!(
        run.resolve("{{v:currency}}", &context.props(&blank)),
        "<0.0>"
    );
    assert_eq!(run.resolve("{{v:currency}}", &context), "<2.5>");
    assert_eq!(
        run.seen(),
        [
            seen(
                "de",
                "NumberFormat",
                "currency=String(\"EUR\") style=String(\"currency\")",
                "250.0"
            ),
            seen("de", "NumberFormat", "style=String(\"currency\")", "0.0"),
            seen("de", "NumberFormat", "style=String(\"currency\")", "2.5"),
        ]
    );

    // A product that is no finite number is no amount.
    assert_eq!(
        run.resolve("{{huge:currency; default:D}}", &context.props(&overflowing)),
        "D"
    );
    assert!(run.seen().is_empty());
    assert_eq!(run.reports(), ["failed-modifier"]);
}

#[test]
fn date_reads_a_timestamp_and_then_a_date() {
    let run = Run::new(Answers::Text);
    let payload = Object::new()
        .with("stamp", "1709632800000")
        .with("iso", "2024-03-05T10:00:00Z")
        .with("spaced", "2024-03-05 11:00:00+01:00")
        .with("bare", "2024-03-05")
        .with("wrong", "2024-03-05T10:00:00Q");
    let props = Object::new().with("date", Object::new().with("timeZone", "UTC"));
    let context = Context::new().payload(&payload).props(&props).locale("en");

    assert_eq!(
        run.resolve(
            "{{stamp:date}}|{{iso:date}}|{{spaced:date}}|{{bare:date}}",
            &context
        ),
        "<1709632800000.0>|<1709632800000.0>|<1709632800000.0>|<1709596800000.0>"
    );
    assert_eq!(
        run.seen(),
        vec![
            seen(
                "en",
                "DateTimeFormat",
                "timeZone=String(\"UTC\")",
                "1709632800000.0"
            );
            3
        ]
        .into_iter()
        .chain([seen(
            "en",
            "DateTimeFormat",
            "timeZone=String(\"UTC\")",
            "1709596800000.0"
        )])
        .collect::<Vec<_>>()
    );
    assert_eq!(run.resolve("{{wrong:date; default:D}}", &context), "D");
    assert!(run.seen().is_empty());
    assert_eq!(run.reports(), ["failed-modifier"]);
}

#[test]
fn ago_climbs_its_ladder_and_states_the_count_without_its_format() {
    let run = Run::new(Answers::Text);
    let payload = Object::new()
        .with("day", -86_400_000)
        .with("soon", 90_000)
        .with("half", 1_500);
    let minutes = Object::new().with("ago", Object::new().with("format", "minutes"));
    let years = Object::new().with(
        "ago",
        Object::new()
            .with("format", "year")
            .with("numeric", "always"),
    );
    let context = Context::new().payload(&payload).locale("en");

    assert_eq!(run.resolve("{{day:ago}}", &context), "<-1.0 day>");
    assert_eq!(run.resolve("{{soon:ago}}", &context), "<2.0 minute>");
    assert_eq!(run.resolve("{{half:ago}}", &context), "<2.0 second>");
    assert_eq!(
        run.resolve("{{day:ago}}", &context.props(&minutes)),
        "<-1440.0 minute>"
    );
    assert_eq!(
        run.resolve("{{day:ago}}", &context.props(&years)),
        "<-0.0 year>"
    );
    assert_eq!(
        run.seen(),
        [
            seen(
                "en",
                "RelativeTimeFormat",
                "numeric=String(\"auto\")",
                "-1.0 day"
            ),
            seen(
                "en",
                "RelativeTimeFormat",
                "numeric=String(\"auto\")",
                "2.0 minute"
            ),
            seen(
                "en",
                "RelativeTimeFormat",
                "numeric=String(\"auto\")",
                "2.0 second"
            ),
            seen(
                "en",
                "RelativeTimeFormat",
                "numeric=String(\"auto\")",
                "-1440.0 minute"
            ),
            seen(
                "en",
                "RelativeTimeFormat",
                "numeric=String(\"always\")",
                "-0.0 year"
            ),
        ]
    );

    // A format is a rung spelled exactly, singular or plural.
    let shouted = Object::new().with("ago", Object::new().with("format", "YEAR"));
    let doubled = Object::new().with("ago", Object::new().with("format", "yearss"));

    assert_eq!(
        run.resolve("{{day:ago; default:D}}", &context.props(&shouted)),
        "D"
    );
    assert_eq!(
        run.resolve("{{day:ago; default:D}}", &context.props(&doubled)),
        "D"
    );
    assert!(run.seen().is_empty());
    assert_eq!(run.reports(), ["failed-modifier", "failed-modifier"]);
}

#[test]
fn plural_asks_with_the_digits_number_shows() {
    let run = Run::new(Answers::Category(Category::Few));
    let payload = Object::new().with("n", 1.5);
    let props = Object::new()
        .with(
            "number",
            Object::new()
                .with("style", "percent")
                .with("minimumFractionDigits", 1)
                .with("roundingMode", "floor"),
        )
        .with(
            "plural",
            Object::new()
                .with("type", "ordinal")
                .with("minimumFractionDigits", 0),
        );
    let context = Context::new().payload(&payload).locale("cs");

    assert_eq!(
        run.resolve(
            "{{n:plural; one:a; few:b; other:c}}",
            &context.props(&props)
        ),
        "b"
    );
    assert_eq!(
        run.seen(),
        [seen(
            "cs",
            "PluralRules",
            "minimumFractionDigits=Integer(0) roundingMode=String(\"floor\") \
             type=String(\"cardinal\") maximumFractionDigits=Number(2.0)",
            "1.5"
        )]
    );
}

#[test]
fn a_key_equal_to_the_value_selects_before_any_category_is_asked_for() {
    let run = Run::new(Answers::Category(Category::One));
    let payload = Object::new().with("n", "3").with("m", 4);
    let context = Context::new().payload(&payload).locale("en");

    assert_eq!(
        run.resolve("{{n:plural; one:a; 3.0:three; 3:other}}", &context),
        "three"
    );
    assert_eq!(run.resolve("{{n:ordinal; 0x3:three}}", &context), "three");
    assert!(run.seen().is_empty());

    // The near miss: no key equals 4, so the category selects.
    assert_eq!(run.resolve("{{m:plural; one:a; 3:three}}", &context), "a");
    assert_eq!(run.seen().len(), 1);
}

#[test]
fn a_category_no_option_names_takes_the_chain() {
    let run = Run::new(Answers::Category(Category::Many));
    let payload = Object::new().with("n", 5);
    let context = Context::new().payload(&payload).locale("pl");

    assert_eq!(
        run.resolve("{{n:plural; one:a; other:b; default:D}}", &context),
        "D"
    );
    assert!(run.reports().is_empty());
}

#[test]
fn a_selection_without_options_asks_nothing() {
    let run = Run::new(Answers::Category(Category::One));
    let payload = Object::new().with("n", 1);
    let context = Context::new().payload(&payload).locale("en");

    assert_eq!(run.resolve("{{n:plural; default:D}}", &context), "D");
    assert_eq!(run.resolve("{{n:ordinal}}", &context), "");
    assert!(run.seen().is_empty());
    assert_eq!(run.reports(), ["missing-options", "missing-options"]);
}

#[test]
fn ordinal_asks_with_its_own_layer_and_takes_no_fraction() {
    let run = Run::new(Answers::Category(Category::Two));
    let payload = Object::new().with("n", 22).with("half", 2.5);
    let props = Object::new()
        .with("number", Object::new().with("minimumFractionDigits", 1))
        .with(
            "ordinal",
            Object::new()
                .with("type", "cardinal")
                .with("localeMatcher", "lookup"),
        );
    let context = Context::new().payload(&payload).props(&props).locale("en");

    assert_eq!(
        run.resolve("{{n:ordinal; one:st; two:nd; other:th}}", &context),
        "nd"
    );
    assert_eq!(
        run.seen(),
        [seen(
            "en",
            "PluralRules",
            "type=String(\"ordinal\") localeMatcher=String(\"lookup\")",
            "22.0"
        )]
    );
    assert_eq!(
        run.resolve("{{half:ordinal; two:nd; default:D}}", &context),
        "D"
    );
    assert!(run.seen().is_empty());
    assert_eq!(run.reports(), ["failed-modifier"]);
}

#[test]
fn without_a_locale_a_formatting_renders_nothing_and_reports_it() {
    let run = Run::new(Answers::Text);
    let payload = Object::new().with("v", 10);
    let context = Context::new().payload(&payload);

    for name in ["number", "date", "ago", "currency"] {
        let message = format!("[{{{{v:{name}; default:D}}}}]");

        assert_eq!(run.resolve(&message, &context), "[]");
        assert_eq!(run.resolve(&message, &context.locale("")), "[]");
    }

    for name in ["plural", "ordinal"] {
        let message = format!("[{{{{v:{name}; other:x; default:D}}}}]");

        assert_eq!(run.resolve(&message, &context), "[]");
    }

    assert!(run.seen().is_empty());
    assert_eq!(run.reports(), vec!["missing-locale"; 10]);
}

#[test]
fn a_value_that_is_no_number_is_one_the_modifier_cannot_take() {
    let run = Run::new(Answers::Text);
    let payload = Object::new()
        .with("blank", " ")
        .with("word", "12px")
        .with("infinite", "Infinity")
        .with("hex", "0x10");
    let context = Context::new().payload(&payload).locale("en");

    assert_eq!(
        run.resolve(
            "{{blank:number; default:a}}{{word:ago; default:b}}{{infinite:plural; other:x; default:c}}",
            &context
        ),
        "abc"
    );
    assert!(run.seen().is_empty());
    assert_eq!(
        run.reports(),
        ["failed-modifier", "failed-modifier", "failed-modifier"]
    );

    // The near miss: ECMAScript reads hexadecimal digits as a number.
    assert_eq!(run.resolve("{{hex:number}}", &context), "<16.0>");
}

#[test]
fn a_refusal_is_a_modifier_that_could_not_answer() {
    let run = Run::new(Answers::Refuse);
    let payload = Object::new().with("v", 1);
    let context = Context::new().payload(&payload).locale("en");

    assert_eq!(
        run.resolve(
            "{{v:number; default:a}}{{v:plural; one:x; default:b}}",
            &context
        ),
        "ab"
    );
    assert_eq!(run.seen().len(), 2);
    assert_eq!(run.reports(), ["failed-modifier", "failed-modifier"]);
}

#[test]
fn a_parser_without_a_supplier_knows_none_of_the_six() {
    let reports: Arc<Mutex<Vec<&'static str>>> = Arc::default();
    let sink = Arc::clone(&reports);
    let parser = Parser::builder()
        .on_report(move |report: &Report| sink.lock().unwrap().push(report.code.as_str()))
        .build();
    let payload = Object::new().with("v", 1);
    let context = Context::new().payload(&payload).locale("en");

    for name in ["number", "date", "ago", "currency", "plural", "ordinal"] {
        let message = format!("{{{{v:{name}; one:x; default:D}}}}");

        assert_eq!(parser.resolve(&message, &context), "D");
    }

    assert_eq!(*reports.lock().unwrap(), vec!["unknown-modifier"; 6]);
    assert_eq!(parser.levels(), [Level::Core, Level::Extensions]);
}

#[test]
fn a_parser_with_a_supplier_claims_the_intl_level() {
    let run = Run::new(Answers::Text);

    assert_eq!(
        run.parser.levels(),
        [Level::Core, Level::Intl, Level::Extensions]
    );
    assert_eq!(
        format!("{:?}", run.parser),
        "Parser { levels: [Core, Intl, Extensions], recognize_wrappers: true, on_report: true, modifiers: 0, intl: true }"
    );
}

#[test]
fn a_modifier_a_host_registers_replaces_the_formats() {
    let run = Run::with(Answers::Text, |builder| {
        builder.modifier("plural", |input: &ModifierInput<'_>| {
            Ok(Some(Value::from(format!("host {}", input.value()))))
        })
    });
    let payload = Object::new().with("n", 1);
    let context = Context::new().payload(&payload).locale("en");

    assert_eq!(
        run.resolve("{{n:plural}}{{n:number}}", &context),
        "host 1<1.0>"
    );
    assert_eq!(run.seen().len(), 1);
    assert!(run.reports().is_empty());
}

#[test]
fn a_request_states_its_input() {
    let request = Request::new(
        Api::RelativeTimeFormat,
        Props::default(),
        Input::Relative(-1.0, Unit::Day),
    );

    assert_eq!(request.number(), None);
    assert_eq!(
        Request::new(Api::PluralRules, Props::default(), Input::Number(2.0)).number(),
        Some(2.0)
    );
}
