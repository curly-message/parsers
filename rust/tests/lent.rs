#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

// A modifier cannot keep its input past its call, but it can lend it for the
// length of that call: to a scoped thread-local, in safe code, where the host
// code the parser calls meanwhile finds it. What that code reads through it
// is no read of the walk's (section 13). Each test runs on a thread with a
// small stack, since a read that went on without bound would abort the
// process there rather than panic.

use std::fmt;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};

use curly_message_parser::{
    Category, Context, Intl, MAX_NESTING, MAX_REENTRY, ModifierError, ModifierInput, Object,
    Parser, ParserBuilder, Refusal, Report, Request, Value,
};
use scoped_tls_hkt::scoped_thread_local;

scoped_thread_local!(static LENT: for<'a> &'a ModifierInput<'a>);

type Answer = Result<Option<Value>, ModifierError>;

fn text(text: impl Into<String>) -> Answer {
    Ok(Some(Value::from(text.into())))
}

type Reports = Arc<Mutex<Vec<(&'static str, String)>>>;

fn recording(builder: ParserBuilder) -> (Parser, Reports) {
    let reports: Reports = Arc::default();
    let sink = Arc::clone(&reports);
    let parser = builder
        .on_report(move |report: &Report| {
            sink.lock()
                .unwrap()
                .push((report.code.as_str(), report.text.clone()));
        })
        .build();

    (parser, reports)
}

fn taken(reports: &Reports) -> Vec<(&'static str, String)> {
    std::mem::take(&mut *reports.lock().unwrap())
}

// Lends its input while it reads its first option, which it answers in
// brackets, then its default where its value says so.
fn lend(input: &ModifierInput<'_>) -> Answer {
    let option = LENT.set(input, || input.option_value(0).unwrap_or("none").to_owned());

    if input.value() == "with default" {
        let default = LENT.set(input, || input.default().to_owned());

        return text(format!("[{option}|{default}]"));
    }

    text(format!("[{option}]"))
}

// Reads the lent input where its value says: the option at that index, or
// the default for `d`. It answers what it read in angle brackets, and `e`
// where the placeholder declares no such option.
fn read(calls: Arc<AtomicUsize>) -> impl Fn(&ModifierInput<'_>) -> Answer {
    move |input: &ModifierInput<'_>| {
        calls.fetch_add(1, Ordering::Relaxed);

        let read = LENT.with(|lent| match input.value() {
            "d" => Some(lent.default().to_owned()),
            at => lent.option_value(at.parse().unwrap()).map(str::to_owned),
        });

        text(read.map_or_else(|| "e".to_owned(), |read| format!("<{read}>")))
    }
}

// Reads every option it declares, then its default, itself, and lends
// nothing.
fn own(input: &ModifierInput<'_>) -> Answer {
    let options: Vec<&str> = (0..input.options())
        .map(|at| input.option_value(at).unwrap())
        .collect();

    text(format!("({}|{})", options.join(","), input.default()))
}

fn parser() -> (Parser, Reports, Arc<AtomicUsize>) {
    let calls: Arc<AtomicUsize> = Arc::default();
    let (parser, reports) = recording(
        Parser::builder()
            .modifier("lend", lend)
            .modifier("own", own)
            .modifier("read", read(Arc::clone(&calls))),
    );

    (parser, reports, calls)
}

// A payload whose key `x` is the lender's value and whose every other key
// used here holds its own name: `{{1:read}}` reads option 1, `{{d:read}}` the
// default.
fn payload(options: usize) -> Object {
    (0..=options).fold(
        Object::new().with("x", "1").with("d", "d"),
        |payload, at| payload.with(at.to_string(), at.to_string()),
    )
}

fn on_small_stack(test: impl FnOnce() + Send + 'static) {
    std::thread::Builder::new()
        .stack_size(512 * 1024)
        .spawn(test)
        .unwrap()
        .join()
        .unwrap();
}

#[test]
fn an_option_read_inside_its_own_production_is_refused_and_fails_nothing() {
    on_small_stack(|| {
        let (parser, reports, calls) = parser();
        let payload = payload(1);
        let context = Context::new().payload(&payload);

        // The reader is answered the empty string, and its answer stands.
        assert_eq!(
            parser.resolve("{{x:lend; o:a{{0:read}}b}}", &context),
            "[a<>b]"
        );
        assert_eq!(taken(&reports), []);
        assert_eq!(calls.swap(0, Ordering::Relaxed), 1);

        // A flat placeholder whose every option reads the one it is in: the
        // lender reads the first alone.
        let options: String = (0..200)
            .map(|at| format!("; o:{{{{{at}:read}}}}"))
            .collect();

        assert_eq!(
            parser.resolve(format!("{{{{x:lend{options}}}}}").as_str(), &context),
            "[<>]"
        );
        assert_eq!(taken(&reports), []);
        assert_eq!(calls.swap(0, Ordering::Relaxed), 1);

        // The near miss: another option of the same placeholder is not in
        // progress.
        assert_eq!(
            parser.resolve("{{x:lend; o:a{{1:read}}b; p:P}}", &context),
            "[a<P>b]"
        );
        assert_eq!(taken(&reports), []);
        assert_eq!(calls.swap(0, Ordering::Relaxed), 1);
    });
}

#[test]
fn a_default_read_inside_its_own_production_is_refused_and_fails_nothing() {
    on_small_stack(|| {
        let (parser, reports, calls) = parser();
        let payload = payload(1).with("x", "with default");
        let context = Context::new().payload(&payload);

        assert_eq!(
            parser.resolve("{{x:lend; o:O; default:a{{d:read}}b}}", &context),
            "[O|a<>b]"
        );
        assert_eq!(taken(&reports), []);
        assert_eq!(calls.swap(0, Ordering::Relaxed), 1);

        // The default is not in progress while an option renders, and what
        // that read produced is what the lender's own read answers.
        assert_eq!(
            parser.resolve("{{x:lend; o:a{{d:read}}b; default:D}}", &context),
            "[a<D>b|D]"
        );
        assert_eq!(taken(&reports), []);
    });
}

// A host value whose description reads an option of the lent input.
struct Describes(usize);

impl fmt::Display for Describes {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let read = LENT.with(|lent| lent.option_value(self.0).unwrap().to_owned());

        write!(formatter, "<{read}>")
    }
}

#[test]
fn the_current_modifiers_own_read_inside_its_production_is_refused() {
    on_small_stack(|| {
        let (parser, reports, _) = parser();

        // The description runs while the lender is the current modifier,
        // inside the option it reads: it is refused that option, and
        // describes the value with what it was answered.
        let payload = Object::new()
            .with("x", "1")
            .with("v", Value::host(Describes(0)));

        assert_eq!(
            parser.resolve("{{x:lend; o:a{{v}}b}}", &Context::new().payload(&payload)),
            "[a<>b]"
        );
        assert_eq!(taken(&reports), []);

        // The near miss: another option of the lender renders for it.
        let payload = Object::new()
            .with("x", "1")
            .with("v", Value::host(Describes(1)));

        assert_eq!(
            parser.resolve(
                "{{x:lend; o:a{{v}}b; p:P}}",
                &Context::new().payload(&payload)
            ),
            "[a<P>b]"
        );
        assert_eq!(taken(&reports), []);
    });
}

// One placeholder of `options` options, each reading the next through the
// lent input, the last reading one the placeholder does not declare.
fn each_reading_the_next(options: usize) -> String {
    format!("{{{{x:lend{}}}}}", reading_the_next(options))
}

fn reading_the_next(options: usize) -> String {
    (1..=options)
        .map(|next| format!("; o:{{{{{next}:read}}}}"))
        .collect()
}

fn nested(levels: usize, innermost: &str) -> String {
    (0..levels).fold(innermost.to_owned(), |inner, _| format!("<{inner}>"))
}

#[test]
fn options_read_each_from_inside_the_one_before_stop_at_the_reentry_limit() {
    on_small_stack(|| {
        let (parser, reports, calls) = parser();
        let payload = payload(1000);
        let context = Context::new().payload(&payload);

        // The resolution holds one place, and the lender's own read of its
        // first option none: every read after it holds one.
        assert_eq!(
            parser.resolve(each_reading_the_next(MAX_REENTRY).as_str(), &context),
            format!("[{}]", nested(MAX_REENTRY - 1, "e"))
        );
        assert_eq!(taken(&reports), []);
        assert_eq!(calls.swap(0, Ordering::Relaxed), MAX_REENTRY);

        // One more, and the read past the bound answers the empty string to
        // its reader, which answers with it: nothing fails or reports.
        for options in [MAX_REENTRY + 1, 200, 1000] {
            assert_eq!(
                parser.resolve(each_reading_the_next(options).as_str(), &context),
                format!("[{}]", nested(MAX_REENTRY, "")),
                "{options} options"
            );
            assert_eq!(taken(&reports), [], "{options} options");
            assert_eq!(
                calls.swap(0, Ordering::Relaxed),
                MAX_REENTRY,
                "{options} options"
            );
        }
    });
}

#[test]
fn a_refused_read_records_nothing_and_a_later_read_renders() {
    on_small_stack(|| {
        let (parser, reports, _) = parser();
        let payload = payload(MAX_REENTRY + 1).with("x", "with default");
        let context = Context::new().payload(&payload);

        // The last option reads the default past the bound, and the lender
        // reads it afterwards, as its own, where it renders.
        let message = format!(
            "{{{{x:lend{}; o:{{{{d:read}}}}; default:D}}}}",
            reading_the_next(MAX_REENTRY - 1)
        );

        assert_eq!(
            parser.resolve(message.as_str(), &context),
            format!("[{}|D]", nested(MAX_REENTRY, ""))
        );
        assert_eq!(taken(&reports), []);
    });
}

#[test]
fn a_read_gives_its_place_back() {
    on_small_stack(|| {
        let (parser, reports, calls) = parser();
        let payload = payload(100);
        let context = Context::new().payload(&payload);

        // One option reading a hundred others, one after another.
        let readers: String = (1..=100).map(|at| format!("{{{{{at}:read}}}}")).collect();
        let read: String = (1..=100).map(|at| format!("; o:{at}")).collect();
        let message = format!("{{{{x:lend; o:{readers}{read}}}}}");
        let expected: String = (1..=100).map(|at| format!("<{at}>")).collect();

        assert_eq!(
            parser.resolve(message.as_str(), &context),
            format!("[{expected}]")
        );
        assert_eq!(taken(&reports), []);
        assert_eq!(calls.swap(0, Ordering::Relaxed), 100);
    });
}

// The worst case for the thread's stack: as many reads of the lent input as
// the bound allows, one inside another, and beneath the innermost a
// placeholder at every level the nesting limit allows, each reading its own
// options and then its default, after the modifier called beneath it
// returned.
#[test]
fn the_current_modifiers_reads_of_its_own_placeholder_hold_no_place() {
    on_small_stack(|| {
        let (parser, reports, _) = parser();
        let payload = payload(MAX_REENTRY);
        let context = Context::new().payload(&payload);
        let levels = MAX_NESTING - 1;
        let deep = (0..levels).fold("leaf".to_owned(), |inner, _| {
            format!("{{{{x:own; o:{inner}; default:{inner}}}}}")
        });
        let leaf = (0..levels).fold("leaf".to_owned(), |inner, _| format!("({inner}|{inner})"));
        let message = format!(
            "{{{{x:lend{}; o:{deep}}}}}",
            reading_the_next(MAX_REENTRY - 1)
        );

        assert_eq!(
            parser.resolve(message.as_str(), &context),
            format!("[{}]", nested(MAX_REENTRY - 1, &leaf))
        );
        assert_eq!(taken(&reports), []);
    });
}

#[test]
fn a_display_read_past_the_bound_answers_the_empty_string_and_fails_nothing() {
    on_small_stack(|| {
        let (parser, reports, _) = parser();
        let spare = MAX_REENTRY;
        let payload = payload(MAX_REENTRY).with("v", Value::host(Describes(spare)));
        let context = Context::new().payload(&payload);

        // The count is full where the value is described, so the read of the
        // spare option is refused.
        let message = format!(
            "{{{{x:lend{}; o:[{{{{v}}}}]; o:S}}}}",
            reading_the_next(MAX_REENTRY - 1)
        );

        assert_eq!(
            parser.resolve(message.as_str(), &context),
            format!("[{}]", nested(MAX_REENTRY - 1, "[<>]"))
        );
        assert_eq!(taken(&reports), []);

        // The near miss: the same read with the lender's own read alone in
        // progress.
        let message = format!(
            "{{{{x:lend; o:[{{{{v}}}}]{}; o:S}}}}",
            "; o:-".repeat(spare - 1)
        );

        assert_eq!(parser.resolve(message.as_str(), &context), "[[<S>]]");
        assert_eq!(taken(&reports), []);
    });
}

#[test]
fn a_report_handler_refused_a_read_fails_nothing() {
    on_small_stack(|| {
        let heard: Arc<Mutex<Vec<String>>> = Arc::default();
        let sink = Arc::clone(&heard);
        let parser = Parser::builder()
            .modifier("lend", lend)
            .on_report(move |report: &Report| {
                let at = if report.text.contains('!') { 1 } else { 0 };
                let read = LENT.with(|lent| lent.option_value(at).unwrap().to_owned());

                sink.lock().unwrap().push(read);
            })
            .build();
        let payload = Object::new().with("x", "1");
        let context = Context::new().payload(&payload);

        // The handler is told of a placeholder inside the option it reads.
        assert_eq!(
            parser.resolve("{{x:lend; o:<{{x:nobody; default:D}}>}}", &context),
            "[<D>]"
        );
        assert_eq!(std::mem::take(&mut *heard.lock().unwrap()), [""]);

        // The near miss: it reads another option, which renders for it.
        assert_eq!(
            parser.resolve("{{x:lend; o:<{{x:nobody; default:D!}}>; p:P}}", &context),
            "[<D!>]"
        );
        assert_eq!(std::mem::take(&mut *heard.lock().unwrap()), ["P"]);
    });
}

// A host value whose description reads the first option of the lent input.
struct ReadsLent;

impl fmt::Display for ReadsLent {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let read = LENT.with(|lent| lent.option_value(0).unwrap().to_owned());

        write!(formatter, "<{read}>")
    }
}

// A facility that formats every number as `f` and puts every one in the
// category `one`.
struct One;

impl Intl for One {
    fn format(&self, _: &str, _: &Request) -> Result<String, Refusal> {
        Ok("f".to_owned())
    }

    fn select(&self, _: &str, _: &Request) -> Result<Category, Refusal> {
        Ok(Category::One)
    }
}

// A facility that refuses every request.
struct Refuses;

impl Intl for Refuses {
    fn format(&self, _: &str, _: &Request) -> Result<String, Refusal> {
        Err(Refusal::new("refused"))
    }

    fn select(&self, _: &str, _: &Request) -> Result<Category, Refusal> {
        Err(Refusal::new("refused"))
    }
}

thread_local! {
    static READ: std::cell::RefCell<Vec<String>> = const { std::cell::RefCell::new(Vec::new()) };
}

// A host value that records what it read of the lent input's first option,
// `none` where no input is lent, and describes itself as `2`.
struct RecordsLent;

impl fmt::Display for RecordsLent {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let read = if LENT.is_set() {
            LENT.with(|lent| lent.option_value(0).unwrap().to_owned())
        } else {
            "none".to_owned()
        };

        READ.with(|reads| reads.borrow_mut().push(read));

        write!(formatter, "2")
    }
}

// What the reads answered since the last call. A facility may convert a
// prop again for itself, as its request holds it, so the walk's conversion
// is not the only one.
fn reads() -> Vec<String> {
    let reads = READ.with(|reads| std::mem::take(&mut *reads.borrow_mut()));

    assert!(!reads.is_empty(), "no read was made");

    reads
}

fn answered_each(reads: &[String], answer: &str) -> bool {
    reads.iter().all(|read| read == answer)
}

// What the inner resolution reads besides `v`, `n` and `s`.
#[derive(Clone, Copy)]
enum Besides {
    Nothing,
    // The payload's default, which describes itself with `m`'s option.
    PayloadDefault,
    // Props whose `number.minimumFractionDigits` records what it read.
    Props,
}

// `{{n:m; o:T}}` resolved where `m`, `levels` resolutions deep, lends its
// input and resolves `inner`: there `v` is `a`, `n` is `1`, and `s`
// describes itself with `m`'s option.
fn beneath(
    with: impl FnOnce(ParserBuilder) -> ParserBuilder,
    levels: usize,
    inner: &str,
    besides: Besides,
) -> String {
    let parser: Arc<std::sync::OnceLock<Parser>> = Arc::default();
    let nesting = Arc::clone(&parser);
    let inner = inner.to_owned();
    let builder = Parser::builder()
        .modifier("h", |input: &ModifierInput<'_>| {
            text(input.option_value(0).unwrap())
        })
        .modifier("fails", |_: &ModifierInput<'_>| {
            Err(ModifierError::new("fails"))
        })
        .modifier("m", move |input: &ModifierInput<'_>| {
            let parser = nesting.get().unwrap();
            let left: usize = input.value().parse().unwrap();

            if left > 0 {
                let payload = Object::new().with("n", (left - 1).to_string());

                return text(parser.resolve("{{n:m; o:T}}", &Context::new().payload(&payload)));
            }

            let payload = Object::new()
                .with("v", "a")
                .with("n", "1")
                .with("s", Value::host(ReadsLent));
            let payload = match besides {
                Besides::PayloadDefault => payload.with("default", Value::host(ReadsLent)),
                _ => payload,
            };
            let props = Object::new().with(
                "number",
                Object::new().with("minimumFractionDigits", Value::host(RecordsLent)),
            );
            let context = Context::new().payload(&payload).locale("en");
            let context = match besides {
                Besides::Props => context.props(&props),
                _ => context,
            };

            text(LENT.set(input, || parser.resolve(inner.as_str(), &context)))
        });

    parser.set(with(builder).build()).ok().unwrap();

    let payload = Object::new().with("n", levels.to_string());

    parser
        .get()
        .unwrap()
        .resolve("{{n:m; o:T}}", &Context::new().payload(&payload))
}

// What a modifier renders of its own options and its default, the format's
// own among them; the payload's default is read for those that name none.
const RENDERED_BY_THE_MODIFIER: [(&str, Besides); 11] = [
    ("{{v:eq; a:{{s}}}}", Besides::Nothing),
    ("{{v; a:{{s}}}}", Besides::Nothing),
    ("{{v:ne; b:{{s}}}}", Besides::Nothing),
    ("{{n:lt; 2:{{s}}}}", Besides::Nothing),
    ("{{n:gt; 0:{{s}}}}", Besides::Nothing),
    ("{{n:lte; 1:{{s}}}}", Besides::Nothing),
    ("{{n:gte; 1:{{s}}}}", Besides::Nothing),
    ("{{v:eq; b:x; default:{{s}}}}", Besides::Nothing),
    ("{{n:lt; 0:x; default:{{s}}}}", Besides::Nothing),
    ("{{v:eq; b:x}}", Besides::PayloadDefault),
    ("{{v:h; a:{{s}}}}", Besides::Nothing),
];

const RENDERED_BY_A_SELECTION: [(&str, Besides); 6] = [
    ("{{n:plural; one:{{s}}}}", Besides::Nothing),
    ("{{n:plural; 1:{{s}}}}", Besides::Nothing),
    ("{{n:plural; few:x; default:{{s}}}}", Besides::Nothing),
    ("{{n:plural; few:x}}", Besides::PayloadDefault),
    ("{{n:ordinal; one:{{s}}}}", Besides::Nothing),
    ("{{n:ordinal; few:x; default:{{s}}}}", Besides::Nothing),
];

// What no modifier renders: the walk's own reads, so `m` stays current.
const RENDERED_BY_THE_WALK: [(&str, Besides); 4] = [
    ("{{s}}", Besides::Nothing),
    ("{{absent:eq; a:x; default:{{s}}}}", Besides::Nothing),
    ("{{v:plural; one:x; default:{{s}}}}", Besides::Nothing),
    ("{{v:nobody; default:{{s}}}}", Besides::Nothing),
];

fn at_the_bound_and_short_of_it(
    with: impl Fn(ParserBuilder) -> ParserBuilder,
    cases: &[(&str, Besides)],
    at_the_bound: &str,
) {
    for &(inner, besides) in cases {
        assert_eq!(
            beneath(&with, MAX_REENTRY - 2, inner, besides),
            at_the_bound,
            "{inner} at the bound"
        );
        assert_eq!(
            beneath(&with, MAX_REENTRY - 3, inner, besides),
            "<T>",
            "{inner} one level short of the bound"
        );
    }
}

// A modifier the format defines is current while it renders what it
// selected, as one a host defines is while its call lasts (section 13): a
// read beneath it of an input lent from around it is no read of the walk's,
// and is refused at the bound.
#[test]
fn a_modifier_the_format_defines_is_current_as_a_hosts_is() {
    on_small_stack(|| {
        let plain = |builder: ParserBuilder| builder;

        at_the_bound_and_short_of_it(plain, &RENDERED_BY_THE_MODIFIER, "<>");
        at_the_bound_and_short_of_it(plain, &RENDERED_BY_THE_WALK, "<T>");

        let facility = |builder: ParserBuilder| builder.intl(One);

        at_the_bound_and_short_of_it(facility, &RENDERED_BY_A_SELECTION, "<>");
        at_the_bound_and_short_of_it(facility, &RENDERED_BY_THE_WALK, "<T>");
    });
}

#[cfg(feature = "intl")]
#[test]
fn a_selection_of_icu4x_is_current_as_a_hosts_modifier_is() {
    on_small_stack(|| {
        let icu = |builder: ParserBuilder| builder.intl(curly_message_parser::IcuIntl::new());

        at_the_bound_and_short_of_it(icu, &RENDERED_BY_A_SELECTION, "<>");
        at_the_bound_and_short_of_it(icu, &RENDERED_BY_THE_WALK, "<T>");
    });
}

// The chain a placeholder takes where its modifier failed, the format's or a
// host's, is the walk's, rendered after the call: `m` is current again.
#[test]
fn the_chain_after_a_failed_call_renders_outside_it() {
    on_small_stack(|| {
        let cases = [
            ("{{n:plural; one:x; default:{{s}}}}", Besides::Nothing),
            ("{{n:number; default:{{s}}}}", Besides::Nothing),
            ("{{v:fails; default:{{s}}}}", Besides::Nothing),
        ];

        at_the_bound_and_short_of_it(
            |builder: ParserBuilder| builder.intl(Refuses),
            &cases,
            "<T>",
        );
    });
}

// A formatting modifier is current while its props convert, as through the
// rest of its call: what their conversion reads of an input lent from
// around it is counted.
fn props_convert_while_the_modifier_is_current(with: impl Fn(ParserBuilder) -> ParserBuilder) {
    for inner in ["{{n:number}}", "{{n:plural; one:x; other:y}}"] {
        beneath(&with, MAX_REENTRY - 2, inner, Besides::Props);
        let read = reads();
        assert!(answered_each(&read, ""), "{inner} at the bound: {read:?}");

        beneath(&with, MAX_REENTRY - 3, inner, Besides::Props);
        let read = reads();
        assert!(
            answered_each(&read, "T"),
            "{inner} one level short of the bound: {read:?}"
        );
    }

    // With no modifier of a host's around, no input is lent, and each
    // conversion of the props reads that.
    let parser = with(Parser::builder()).build();
    let payload = Object::new().with("n", "1");
    let props = Object::new().with(
        "number",
        Object::new().with("minimumFractionDigits", Value::host(RecordsLent)),
    );
    let context = Context::new().payload(&payload).props(&props).locale("en");

    parser.resolve("{{n:number}}", &context);
    let read = reads();
    assert!(answered_each(&read, "none"), "{read:?}");
}

#[test]
fn a_formatting_modifier_is_current_while_its_props_convert() {
    on_small_stack(|| props_convert_while_the_modifier_is_current(|builder| builder.intl(One)));
}

#[cfg(feature = "intl")]
#[test]
fn a_formatting_modifier_of_icu4x_is_current_while_its_props_convert() {
    on_small_stack(|| {
        props_convert_while_the_modifier_is_current(|builder| {
            builder.intl(curly_message_parser::IcuIntl::new())
        });
    });
}
