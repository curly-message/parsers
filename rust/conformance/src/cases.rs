use std::panic::{AssertUnwindSafe, catch_unwind};

use crate::RunError;
use crate::adapter::{
    Adapter, DESCEND, Descend, ExposedRequest, RENESTED, Renest, Report, Resolution, Resolved,
};
use crate::host::FormatHost;
use crate::js;
use crate::plan::{Failure, Outcome};
use crate::set::{
    Api, Behaviour, CATEGORIES, Categories, ConcreteCase, ExpectedFormat, GeneratedCase, Keeping,
};
use crate::value::{Value, decode, decode_optional, plain, without_undefined};

/// A formatting request as the runner's host is handed it: the facility, its
/// options and its input.
#[derive(Clone, Debug)]
pub struct FormatRequest {
    /// The facility.
    pub api: Api,
    /// The options the facility is constructed with, or the host's absence.
    pub options: Value,
    /// The input.
    pub input: Value,
}

impl FormatRequest {
    fn shown(&self) -> Value {
        let mut members = vec![("api".to_owned(), Value::from(self.api.as_str()))];

        if !self.options.is_undefined() {
            members.push(("options".to_owned(), self.options.clone()));
        }

        members.push(("input".to_owned(), self.input.clone()));

        Value::object(members)
    }
}

impl From<&ExpectedFormat> for FormatRequest {
    fn from(format: &ExpectedFormat) -> Self {
        Self {
            api: format.api,
            options: format.options.as_ref().map_or(Value::Undefined, plain),
            input: plain(&format.input),
        }
    }
}

// What a case expects to be produced: text it writes or a generated case
// computes, or the request a locale-dependent case states.
enum Want<'c> {
    Text(String),
    Request(&'c ExpectedFormat),
}

// A report as a case expects it; a generated case also expects the limit the
// adapter declared, where the report is about one.
struct Wanted {
    code: &'static str,
    origin: &'static str,
    id: Option<String>,
    limit: Option<i64>,
}

impl Wanted {
    fn shown(&self) -> Value {
        let mut members = vec![
            ("code".to_owned(), Value::from(self.code)),
            ("origin".to_owned(), Value::from(self.origin)),
        ];

        if let Some(id) = &self.id {
            members.push(("id".to_owned(), id.as_str().into()));
        }

        if let Some(limit) = self.limit {
            members.push(("limit".to_owned(), Value::Number(limit as f64)));
        }

        Value::object(members)
    }
}

// A case with its inputs built and its expectation computed.
pub(crate) struct Prepared<'c> {
    input: Resolution,
    want: Want<'c>,
    categories: Option<&'c Categories>,
    reports: Vec<Wanted>,
}

fn failed(reason: impl Into<String>, expected: String, actual: String) -> Outcome {
    Outcome::Failed(Failure {
        reason: reason.into(),
        expected,
        actual,
    })
}

fn text(value: &str) -> String {
    js::quote(value)
}

fn optional(value: Option<&str>) -> String {
    value.map_or_else(|| "undefined".to_owned(), text)
}

// A selection case names the output of every category. One that leaves a
// category out is the set's defect and not the implementation's, whichever
// reading it is measured on and whichever category a host answers, so it ends
// the run before anything is compared.
fn selects(case: &ConcreteCase) -> Result<&Categories, RunError> {
    let named = case.expected.categories.as_ref();
    let missing = CATEGORIES
        .iter()
        .find(|category| named.and_then(|named| named.get(category)).is_none());

    match (named, missing) {
        (Some(named), None) => Ok(named),
        (_, missing) => Err(RunError::new(format!(
            "The case {} names no output for the category {}.",
            case.id,
            optional(missing.copied())
        ))),
    }
}

// What the adapter is handed: the inputs as decoded where it states a
// stand-in for `undefined`, and with every member that holds it left out
// where it states none.
fn handed(adapter: &dyn Adapter, input: Resolution) -> Resolution {
    if adapter.undefined() {
        return input;
    }

    Resolution {
        message: without_undefined(&input.message),
        payload: without_undefined(&input.payload),
        props: without_undefined(&input.props),
        id: without_undefined(&input.id),
        defaults: without_undefined(&input.defaults),
        ..input
    }
}

/// Decodes a written-out case's inputs and computes its expectation.
pub(crate) fn concrete<'c>(
    adapter: &dyn Adapter,
    case: &'c ConcreteCase,
) -> Result<Prepared<'c>, RunError> {
    let (want, categories) = match (&case.expected.format, &case.expected.output) {
        (Some(format), _) if format.api == Api::PluralRules => {
            (Want::Request(format), Some(selects(case)?))
        }
        (Some(format), _) => (Want::Request(format), None),
        (None, Some(output)) => (Want::Text(output.clone()), None),
        (None, None) => {
            return Err(RunError::new(format!(
                "The case {} expects neither an output nor a format.",
                case.id
            )));
        }
    };
    let reports = case
        .expected
        .reports
        .iter()
        .flatten()
        .map(|report| Wanted {
            code: report.code.as_str(),
            origin: report.origin.as_str(),
            id: report.id.clone(),
            limit: None,
        })
        .collect();

    Ok(Prepared {
        input: handed(
            adapter,
            Resolution {
                message: decode(&case.message)?,
                payload: decode_optional(case.payload.as_ref())?,
                props: decode_optional(case.props.as_ref())?,
                locale: case.locale.clone(),
                id: decode_optional(case.message_id.as_ref())?,
                defaults: decode_optional(case.defaults.as_ref())?,
                modifiers: case.modifiers.clone(),
                keeping: case.keeping.clone(),
                descend: None,
                renest: None,
            },
        ),
        want,
        categories,
        reports,
    })
}

// Every generated case reports under this id.
const ID: &str = "limits";

fn count(length: i64) -> usize {
    usize::try_from(length).unwrap_or(0)
}

// An output of the stated length written entirely by the message: the
// placeholder selects an option, and an option value is the message's own
// text, so what reaches the output is the placeholder's result while the
// payload was read for one character.
fn writes(length: i64) -> String {
    format!("{{{{v; a:{};}}}}", "x".repeat(count(length)))
}

// A value of the stated length the placeholder selects nothing from: the read
// budget is spent whole and two characters reach the output.
const READS: &str = "{{v:eq; nomatch:X; default:ok;}}";

// A value converted whole and compared against a key its text cannot equal:
// the option is chosen only where the value was converted, so a conversion
// abandoned shows in the output as the default, and none of the value's text
// reaches the output either way.
const CONVERTS: &str = "{{v:ne; nomatch:ok; default:refused;}}";

// A value whose serialization visits the stated number of nodes and writes
// little text. Section 13 counts a member the walk visits and then omits, and
// a value reached twice twice: the root holds `s` references to one object of
// `s - 1` members holding the host's undefined, each reference visiting `s`
// nodes, and enough members holding undefined itself to make up the rest. So
// the node count meets the limit long before any bound on the text, and an
// implementation that counts a shared object once, or skips what it omits,
// stops short of it.
fn visiting(nodes: i64) -> Value {
    let nodes = count(nodes).max(1);
    let side = (nodes - 1).isqrt();
    let shared = Value::object(
        (0..side.saturating_sub(1))
            .map(|index| (format!("k{index}"), Value::Undefined))
            .collect(),
    );

    Value::object(
        (0..side)
            .map(|index| (format!("s{index}"), shared.clone()))
            .chain(
                (0..nodes - 1 - side * side).map(|index| (format!("u{index}"), Value::Undefined)),
            )
            .collect(),
    )
}

// A message nesting the stated number of levels, each selecting the option
// that holds the next. The innermost is written last, so a level the nesting
// limit refuses is visible as the chain it takes. It is written as the levels
// around the innermost and their closings, so its cost is its length.
fn nest(levels: i64, innermost: &str) -> String {
    let around = count(levels).saturating_sub(1);

    format!(
        "{}{innermost}{}",
        "{{v; a:".repeat(around),
        ";}}".repeat(around)
    )
}

const SETTLED: &str = "{{v; a:settled; default:fallback;}}";

// How many levels past the nesting limit the chain construction nests: far
// enough that a walk recursing once per level overflows a default call stack
// on some hosts.
const CHAIN: usize = 20_000;

// A message nesting the stated number of levels, each the inline default of
// the one around it. The payload names none of the keys, so every level takes
// its chain down to the next, and the innermost writes the text. It is built
// without recursion.
fn chain(levels: usize) -> String {
    format!("{}x{}", "{{v; default:".repeat(levels), "}}".repeat(levels))
}

fn payload(members: &[(&str, Value)]) -> Value {
    Value::object(
        members
            .iter()
            .map(|(name, value)| ((*name).to_owned(), value.clone()))
            .collect(),
    )
}

fn limit(code: &'static str, origin: &'static str, declared: i64) -> Wanted {
    Wanted {
        code,
        origin,
        id: Some(ID.to_owned()),
        limit: Some(declared),
    }
}

fn unserializable() -> Wanted {
    Wanted {
        code: "unserializable-value",
        origin: "payload",
        id: Some(ID.to_owned()),
        limit: None,
    }
}

fn generated_input(message: String, payload: Value) -> Resolution {
    Resolution {
        payload,
        id: ID.into(),
        ..Resolution::message(message)
    }
}

/// The constructions built from the bound the adapter states on counted reads
/// (section 13) rather than from its limits.
pub(crate) const COUNTED: [&str; 7] = [
    "reentry-at-limit",
    "reentry-over-limit",
    "reentry-refused-not-recorded",
    "reentry-own-reads-free",
    "reentry-over-limit-by-a-conversion",
    "reentry-own-read-in-a-nested-resolution",
    DEFINED,
];

/// Those of them that make their read in a resolution a modifier asks for
/// with the bound reached, and are built from resolutions one inside another
/// where the adapter states that its bound counts them, which keeps no input
/// past the call it was handed to.
pub(crate) const FRAMED: [&str; 2] = ["reentry-own-read-in-a-nested-resolution", DEFINED];

/// The construction whose read is refused beneath a modifier of the format.
pub(crate) const DEFINED: &str = "reentry-over-limit-beneath-a-defined-modifier";

/// The construction built from that bound where the adapter states that it
/// counts a resolution asked for inside another, which keeps no input.
pub(crate) const RESOLUTIONS: &str = "reentry-resolutions-over-limit";

/// The output of that construction at a bound: one resolution more than the
/// bound stands, each writing its brackets.
pub(crate) fn renested(bound: usize) -> String {
    format!(
        "{}F{}",
        "[".repeat(bound.saturating_add(1)),
        "]".repeat(bound.saturating_add(1))
    )
}

// A placeholder that reads the input kept last that no `take` has claimed.
// Its inline default stands for a `take` that failed, which no case expects.
const TAKE: &str = "{{x:take; default:F}}";

// A placeholder whose modifier answers a value that reads the input kept last
// that no `take` has claimed, as it is converted.
const TAKING: &str = "{{x:taking; default:F}}";

// A placeholder whose modifier keeps its default unread and asks for a
// resolution in which the conversion of an answer reads it.
const KEEP_NEST: &str = "{{x:keep-nest; default:z}}";

// One whose modifier does the same, of a resolution in which that conversion
// runs while a modifier the format defines produces the option it selected,
// between brackets. Its default holds another: where the bound counts the
// resolution, the read of the first default is one past it; where it counts
// reads alone, that read takes the last place and produces the second
// placeholder, whose default is then read one past the bound. Either read is
// refused, and the text is the same.
const KEEP_NEST_NE: &str = "<{{x:keep-nest-ne; default:{{x:keep-nest-ne; default:z}}}}>";

// Placeholders that each keep their default unread, the default taking the
// input kept before it. A `take` after them claims the last one kept and
// reads it, and each read takes the one kept before, so the reads stand one
// inside another as deep as the placeholders kept, while the message nests two
// levels. What each answers is the text it read, or `empty`, so no result
// grows with the bound, and `x` holds the empty string, so no placeholder
// spends the read budget.
fn keeps(count: usize, body: &str) -> String {
    format!("{{{{x:keep; default:{body}}}}}").repeat(count)
}

// One input more than the bound lets be read at once. The read past it is
// made by the placeholder in the second input's default, written between
// brackets, so what it answers reaches the output as it answered it; where
// the bound is 0 it is the placeholder after them. It is a `take`, or one
// whose answer's conversion makes the read.
fn refusing(bound: usize, reader: &str) -> String {
    if bound == 0 {
        return format!("{}{reader}", keeps(1, TAKE));
    }

    format!(
        "{}{}{}{TAKE}",
        keeps(1, TAKE),
        keeps(1, &format!("[{reader}]")),
        keeps(bound - 1, TAKE)
    )
}

// The stated number of levels of own reads: each placeholder's modifier reads
// its own default, which holds the next, and the innermost answers `z`.
fn own(levels: usize) -> String {
    format!(
        "{}z{}",
        "{{x:read; default:".repeat(levels),
        "}}".repeat(levels)
    )
}

// The behaviours over what was kept every construction of kept reads
// registers are `keep`, `take` and `first`; `more` are registered beside them.
fn reentering(message: String, more: &[Keeping]) -> Resolution {
    let keeping = [Keeping::Keep, Keeping::Take, Keeping::First]
        .iter()
        .chain(more)
        .map(|behaviour| (behaviour.as_str().to_owned(), *behaviour))
        .collect();

    Resolution {
        modifiers: Some(vec![("read".to_owned(), Behaviour::Default)]),
        keeping: Some(keeping),
        ..generated_input(message, payload(&[("x", "".into())]))
    }
}

// `inner` resolved with one place fewer taken than the bound lets stand, so
// that the resolution its modifier asks for takes the last where the bound
// counts resolutions. Where the adapter states that it does, the places are
// resolutions one inside another, the innermost of `inner`. Otherwise they
// are counted reads, the innermost producing `inner`, and the bound may count
// the resolution or not: the adapter need not say.
fn standing(
    adapter: &dyn Adapter,
    bound: usize,
    inner: &'static str,
    keeper: Keeping,
) -> Resolution {
    if matches!(adapter.resolutions(), Value::Bool(true)) {
        return Resolution {
            keeping: Some(
                Keeping::ALL
                    .iter()
                    .map(|behaviour| (behaviour.as_str().to_owned(), *behaviour))
                    .collect(),
            ),
            descend: Some(Descend::new(bound.saturating_sub(1), inner)),
            ..generated_input(
                if bound > 1 { DESCEND } else { inner }.to_owned(),
                payload(&[("x", "".into())]),
            )
        };
    }

    reentering(
        if bound > 1 {
            format!(
                "{}{}{TAKE}",
                keeps(1, inner),
                keeps(bound.saturating_sub(2), TAKE)
            )
        } else {
            inner.to_owned()
        },
        &[Keeping::Taking, keeper],
    )
}

fn empty(bound: usize, text: &str) -> String {
    if bound == 0 {
        text.to_owned()
    } else {
        format!("[{text}]")
    }
}

/// Builds a generated case from the adapter's limits, or from the bound it
/// states on counted reads.
pub(crate) fn generated(
    case: &GeneratedCase,
    adapter: &dyn Adapter,
) -> Result<Prepared<'static>, RunError> {
    let limits = adapter.limits();
    let x = |length: i64| Value::from("x".repeat(count(length)));
    let a = || ("v", Value::from("a"));
    let construction = case.generate.as_str();
    let bound = if COUNTED.contains(&construction) || construction == RESOLUTIONS {
        match adapter.reentry() {
            Some(bound) => count(bound),
            None => {
                return Err(RunError::new(format!(
                    "The case {} is built from a bound on counted reads the adapter does not state.",
                    case.id
                )));
            }
        }
    } else {
        0
    };
    let (input, output, reports) = match construction {
        "output-at-limit" => (
            generated_input(writes(limits.output), payload(&[a()])),
            "x".repeat(count(limits.output)),
            vec![],
        ),
        // The placeholder resolves to the empty string and the message's own
        // text still renders, because that text is the caller's and is
        // counted against no limit.
        "output-over-limit" => (
            generated_input(
                format!("A{}B", writes(limits.output.saturating_add(1))),
                payload(&[a()]),
            ),
            "AB".to_owned(),
            vec![limit("output-limit", "limit", limits.output)],
        ),
        // A result the output has no room for spends nothing, so the
        // placeholder after it is resolved and carried.
        "output-over-limit-continues" => (
            generated_input(
                format!("{}{{{{w}}}}", writes(limits.output.saturating_add(1))),
                payload(&[a(), ("w", "tail".into())]),
            ),
            "tail".to_owned(),
            vec![limit("output-limit", "limit", limits.output)],
        ),
        "read-at-limit" => (
            generated_input(READS.to_owned(), payload(&[("v", x(limits.read))])),
            "ok".to_owned(),
            vec![],
        ),
        // The budget is tested before a placeholder reads, so the one that
        // spent it past the limit still resolves and the one after it pays.
        "read-over-limit" => (
            generated_input(
                format!("{READS}{{{{w}}}}"),
                payload(&[
                    ("v", x(limits.read.saturating_add(1))),
                    ("w", "tail".into()),
                ]),
            ),
            "ok".to_owned(),
            vec![limit("read-limit", "limit", limits.read)],
        ),
        "conversion-at-limit" => (
            generated_input(
                CONVERTS.to_owned(),
                payload(&[("v", visiting(limits.conversion))]),
            ),
            "ok".to_owned(),
            vec![],
        ),
        "conversion-over-limit" => (
            generated_input(
                "{{v; default:D}}".to_owned(),
                payload(&[("v", visiting(limits.conversion.saturating_add(1)))]),
            ),
            "D".to_owned(),
            vec![unserializable()],
        ),
        // One node past the limit, each element long enough that the text a
        // finished serialization would write runs past the read budget: a
        // refused serialization produces no text, so it spends none, and the
        // placeholder after it is resolved and carried.
        "conversion-over-limit-continues" => {
            let elements = count(limits.conversion);
            let each = "x".repeat(count(limits.read).div_ceil(elements.max(1)) + 1);

            (
                generated_input(
                    "{{v; default:D}}{{w}}".to_owned(),
                    payload(&[
                        (
                            "v",
                            Value::array(vec![Value::from(each.as_str()); elements]),
                        ),
                        ("w", "tail".into()),
                    ]),
                ),
                "Dtail".to_owned(),
                vec![unserializable()],
            )
        }
        "nesting-at-limit" => (
            generated_input(nest(limits.nesting, SETTLED), payload(&[a()])),
            "settled".to_owned(),
            vec![],
        ),
        // The innermost placeholder is the one refused, and a nesting limit is
        // a defect of the message: it takes its fallback chain, and every
        // level around it selects the option that holds it.
        "nesting-over-limit" => (
            generated_input(
                nest(limits.nesting.saturating_add(1), SETTLED),
                payload(&[a()]),
            ),
            "fallback".to_owned(),
            vec![limit("nesting-limit", "message", limits.nesting)],
        ),
        // Section 9 decides the nesting limit after the value is looked up
        // and converted, so a placeholder nested too deep still converts its
        // value, reports what the conversion reports and spends the read
        // budget, and one reached with the budget spent resolves to the empty
        // string however deeply it nests.
        "nesting-over-limit-converts" => (
            generated_input(
                nest(limits.nesting.saturating_add(1), "{{w; default:fallback;}}"),
                payload(&[a(), ("w", Value::Unserializable)]),
            ),
            "fallback".to_owned(),
            vec![
                unserializable(),
                limit("nesting-limit", "message", limits.nesting),
            ],
        ),
        "nesting-over-limit-reads" => (
            generated_input(
                format!(
                    "{}{{{{v}}}}",
                    nest(limits.nesting.saturating_add(1), "{{w; default:fallback;}}")
                ),
                payload(&[a(), ("w", x(limits.read.saturating_add(1)))]),
            ),
            "fallback".to_owned(),
            vec![
                limit("nesting-limit", "message", limits.nesting),
                limit("read-limit", "limit", limits.read),
            ],
        ),
        "nesting-over-limit-read-spent" => (
            generated_input(
                nest(
                    limits.nesting.saturating_add(1),
                    "{{w:eq; nomatch:X;}}{{v; default:fallback;}}",
                ),
                payload(&[a(), ("w", x(limits.read.saturating_add(1)))]),
            ),
            String::new(),
            vec![
                limit("nesting-limit", "message", limits.nesting),
                limit("read-limit", "limit", limits.read),
            ],
        ),
        // The nesting limit is the first of the message errors, and a
        // placeholder that meets one meets none after it.
        "nesting-over-limit-unknown-modifier" => (
            generated_input(
                nest(
                    limits.nesting.saturating_add(1),
                    "{{w:zz; default:fallback;}}",
                ),
                payload(&[a(), ("w", "x".into())]),
            ),
            "fallback".to_owned(),
            vec![limit("nesting-limit", "message", limits.nesting)],
        ),
        "nesting-over-limit-missing-options" => (
            generated_input(
                nest(
                    limits.nesting.saturating_add(1),
                    "{{w:eq; default:fallback;}}",
                ),
                payload(&[a(), ("w", "x".into())]),
            ),
            "fallback".to_owned(),
            vec![limit("nesting-limit", "message", limits.nesting)],
        ),
        // A refused placeholder takes its chain, whose last link is its inline
        // default, so each level past the limit is refused and reported once
        // and the walk carries on to the innermost text.
        "nesting-chain-over-limit" => (
            generated_input(
                chain(count(limits.nesting).saturating_add(CHAIN)),
                payload(&[]),
            ),
            "x".to_owned(),
            (0..CHAIN)
                .map(|_| limit("nesting-limit", "message", limits.nesting))
                .collect(),
        ),
        // As many reads as the bound lets stand at once: the innermost `take`
        // finds nothing left to claim.
        "reentry-at-limit" => (
            reentering(format!("{}{TAKE}", keeps(bound, TAKE)), &[]),
            "none".to_owned(),
            vec![],
        ),
        // One read more: it is refused, produces nothing and answers the
        // empty string, and the `take` that made it goes on with it.
        "reentry-over-limit" => (
            reentering(refusing(bound, TAKE), &[]),
            empty(bound, "empty"),
            vec![],
        ),
        // The refused read was not recorded, so a read of the same input
        // afterwards, with no other read in progress, produces it: its `take`
        // finds every input claimed. Where the bound is 0, that read is
        // refused as well.
        "reentry-refused-not-recorded" => (
            reentering(
                format!("{}{{{{x:first; default:F}}}}", refusing(bound, TAKE)),
                &[],
            ),
            if bound == 0 {
                "emptyempty".to_owned()
            } else {
                "[empty]none".to_owned()
            },
            vec![],
        ),
        // The innermost of as many counted reads as the bound lets stand
        // produces a stack of own reads as deep as the nesting limit leaves
        // room for: they are the walk's and count toward no bound, so every
        // one of them resolves.
        "reentry-own-reads-free" => (
            reentering(
                format!(
                    "{}{}{TAKE}",
                    keeps(1, &own(count(limits.nesting).saturating_sub(1).max(1))),
                    keeps(bound.saturating_sub(1), TAKE)
                ),
                &[],
            ),
            "z".to_owned(),
            vec![],
        ),
        // One read more, made by the conversion of an answer rather than by a
        // modifier: it is refused, and the conversion goes on with the empty
        // string.
        "reentry-over-limit-by-a-conversion" => (
            reentering(refusing(bound, TAKING), &[Keeping::Taking]),
            empty(bound, "empty"),
            vec![],
        ),
        // With the bound reached, a modifier keeps its default and asks for a
        // resolution, where the conversion of an answer reads that default.
        // The modifier is current there, so the read is its own: a counted
        // read there would be one past the bound.
        "reentry-own-read-in-a-nested-resolution" => (
            standing(adapter, bound, KEEP_NEST, Keeping::KeepNest),
            "z".to_owned(),
            vec![],
        ),
        // The same, with that conversion run while a modifier the format
        // defines produces the option it selected: that modifier is the
        // current one, so the read is no modifier's own, is counted one past
        // the bound and is refused, and the conversion goes on with the empty
        // string.
        DEFINED => (
            standing(adapter, bound, KEEP_NEST_NE, Keeping::KeepNestNe),
            "<empty>".to_owned(),
            vec![],
        ),
        // One resolution more than the bound lets stand inside the one asked
        // for: it is refused, the modifier that asked fails in the innermost
        // resolution standing, and reports there. Only where the bound is 0
        // is that the resolution the case asks for.
        "reentry-resolutions-over-limit" => (
            Resolution {
                renest: Some(Renest::new(bound)),
                ..generated_input(RENESTED.to_owned(), payload(&[("x", "".into())]))
            },
            renested(bound),
            if bound == 0 {
                vec![Wanted {
                    code: "failed-modifier",
                    origin: "payload",
                    id: Some(ID.to_owned()),
                    limit: None,
                }]
            } else {
                vec![]
            },
        ),
        other => {
            return Err(RunError::new(format!(
                "The case {} names no construction: {}.",
                case.id,
                text(other)
            )));
        }
    };

    Ok(Prepared {
        input: handed(adapter, input),
        want: Want::Text(output),
        categories: None,
        reports,
    })
}

fn panicked(payload: &(dyn std::any::Any + Send)) -> String {
    payload
        .downcast_ref::<&str>()
        .map(|text| (*text).to_owned())
        .or_else(|| payload.downcast_ref::<String>().cloned())
        .unwrap_or_else(|| "The adapter panicked.".to_owned())
}

/// Calls into an adapter, a panic being a raise like any other: a raise from
/// the implementation is a result, and the run goes on.
pub(crate) fn contained<T>(call: impl FnOnce() -> Result<T, String>) -> Result<T, String> {
    catch_unwind(AssertUnwindSafe(call)).unwrap_or_else(|payload| Err(panicked(&*payload)))
}

fn empty_if_absent(options: &Value) -> Value {
    match options {
        Value::Undefined | Value::Null => Value::object(Vec::new()),
        other => other.clone(),
    }
}

fn listed(values: impl Iterator<Item = Value>) -> String {
    js::show(&Value::array(values.collect()))
}

// Section 11.2 lets an implementation expose the request it makes, and an
// adapter that does is measured on it rather than on the text its own locale
// data made. A case that states a request writes the placeholder alone, so the
// resolution it describes makes exactly one.
fn compare_formats(expected: &FormatRequest, actual: &[ExposedRequest]) -> Option<Outcome> {
    let [request] = actual else {
        return Some(failed(
            format!("Expected one formatting request, got {}.", actual.len()),
            listed(std::iter::once(expected.shown())),
            listed(actual.iter().map(ExposedRequest::shown)),
        ));
    };
    let differs = |reason: &str| {
        Some(failed(
            reason,
            js::show(&expected.shown()),
            js::show(&request.shown()),
        ))
    };

    if request.api != expected.api.as_str() {
        return differs("The facility of the request differs.");
    }

    if js::canonical(&empty_if_absent(&request.options))
        != js::canonical(&empty_if_absent(&expected.options))
    {
        return differs("The properties of the request differ.");
    }

    if js::canonical(&request.input) != js::canonical(&expected.input) {
        return differs("The input of the request differs.");
    }

    None
}

// A selection request exposed on its own says what was asked and not what the
// host answered, and the answer is what chose the output (section 11.5). So
// the category it carries is read, and the output held to the one the case
// names for it: the category is the host's, and the choice is the format's.
fn compare_selection(
    categories: Option<&Categories>,
    request: &ExposedRequest,
    output: &str,
) -> Option<Outcome> {
    let Some(category) = request
        .category
        .as_deref()
        .filter(|category| CATEGORIES.contains(category))
    else {
        return Some(failed(
            "The selection request carries no category the plural rules name.",
            listed(CATEGORIES.iter().map(|category| Value::from(*category))),
            optional(request.category.as_deref()),
        ));
    };
    let chosen = categories.and_then(|categories| categories.get(category));

    (chosen != Some(output)).then(|| {
        failed(
            format!("The output differs from the one the category {category} selects."),
            optional(chosen),
            text(output),
        )
    })
}

fn reports_counted(length: usize) -> String {
    format!(
        "{length} {}",
        if length == 1 { "report" } else { "reports" }
    )
}

// A field is compared where the adapter's report carries it and the case
// expects something of it; the specification prescribes a report no shape.
fn compare_reports(expected: &[Wanted], actual: &[Report]) -> Option<Outcome> {
    if expected.len() != actual.len() {
        return Some(failed(
            format!(
                "Expected {}, got {}.",
                reports_counted(expected.len()),
                reports_counted(actual.len())
            ),
            listed(expected.iter().map(Wanted::shown)),
            listed(actual.iter().map(Report::shown)),
        ));
    }

    for (index, (wanted, report)) in expected.iter().zip(actual).enumerate() {
        let position = index + 1;
        let differs = |field: &str| {
            Some(failed(
                format!("The {field} of report {position} differs."),
                js::show(&wanted.shown()),
                js::show(&report.shown()),
            ))
        };

        if report.code != wanted.code {
            return differs("code");
        }

        if report
            .origin
            .as_deref()
            .is_some_and(|origin| origin != wanted.origin)
        {
            return differs("origin");
        }

        if let (Some(id), Some(wanted)) = (&report.id, &wanted.id)
            && id != wanted
        {
            return differs("id");
        }

        if let (Some(limit), Some(wanted)) = (report.limit, wanted.limit)
            && limit != wanted
        {
            return differs("limit");
        }
    }

    None
}

// On the text reading, the request is performed on the runner's own host and
// the output held to what that host made of it, or, for a selection, to the
// output the case names for the category the host answered.
fn performed(
    host: Option<&dyn FormatHost>,
    locale: Option<&str>,
    request: &FormatRequest,
    categories: Option<&Categories>,
    output: &str,
) -> Result<Option<Outcome>, RunError> {
    let Some(host) = host else {
        return Ok(Some(failed(
            "The adapter exposes no request, and the runner has no host to perform it on.",
            js::show(&request.shown()),
            text(output),
        )));
    };
    let answer = match host.perform(locale, request) {
        None => {
            return Ok(Some(failed(
                "The runner's host holds no answer recorded for the request.",
                js::show(&request.shown()),
                text(output),
            )));
        }
        Some(Err(error)) => {
            return Err(RunError::new(format!(
                "The runner's host raised performing {}: {error}",
                js::show(&request.shown())
            )));
        }
        Some(Ok(answer)) => answer,
    };
    let expected = if request.api == Api::PluralRules {
        categories.and_then(|categories| categories.get(&answer))
    } else {
        Some(answer.as_str())
    };

    Ok((expected != Some(output))
        .then(|| failed("The output differs.", optional(expected), text(output))))
}

fn produced(
    prepared: &Prepared<'_>,
    host: Option<&dyn FormatHost>,
    resolved: &Resolved,
) -> Result<Option<Outcome>, RunError> {
    let output = resolved.output.as_str();

    match (&prepared.want, &resolved.formats) {
        (Want::Request(format), Some(formats)) => {
            let request = FormatRequest::from(*format);

            Ok(
                compare_formats(&request, formats).or_else(|| match formats.first() {
                    Some(exposed) if format.api == Api::PluralRules => {
                        compare_selection(prepared.categories, exposed, output)
                    }
                    _ => None,
                }),
            )
        }
        (Want::Request(format), None) => performed(
            host,
            prepared.input.locale.as_deref(),
            &FormatRequest::from(*format),
            prepared.categories,
            output,
        ),
        (Want::Text(expected), _) => Ok((expected != output)
            .then(|| failed("The output differs.", text(expected), text(output)))),
    }
}

/// Runs a prepared case, in the order of `RUNNER.md`: the answer's shape, the
/// output or the request, then the reports.
pub(crate) fn execute(
    adapter: &dyn Adapter,
    host: Option<&dyn FormatHost>,
    prepared: &Prepared<'_>,
) -> Result<Outcome, RunError> {
    let expected = match &prepared.want {
        Want::Text(output) => text(output),
        Want::Request(format) => js::show(&FormatRequest::from(*format).shown()),
    };
    let resolved = match contained(|| adapter.resolve(&prepared.input)) {
        Err(raised) => return Ok(failed("The adapter raised.", expected, text(&raised))),
        Ok(None) => {
            return Ok(failed(
                "The adapter answered with no resolution.",
                expected,
                "undefined".to_owned(),
            ));
        }
        Ok(Some(resolved)) => resolved,
    };

    if let Some(failure) = produced(prepared, host, &resolved)? {
        return Ok(failure);
    }

    let Some(reports) = &resolved.reports else {
        return Ok(Outcome::PassedReportsUnobserved);
    };

    Ok(compare_reports(&prepared.reports, reports).unwrap_or(Outcome::Passed))
}
