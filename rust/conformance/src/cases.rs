use std::panic::{AssertUnwindSafe, catch_unwind};

use crate::RunError;
use crate::adapter::{Adapter, ExposedRequest, Limits, Report, Resolution, Resolved};
use crate::host::FormatHost;
use crate::js;
use crate::plan::{Failure, Outcome};
use crate::set::{Api, CATEGORIES, Categories, ConcreteCase, ExpectedFormat, GeneratedCase};
use crate::value::{Value, decode, decode_optional, nodes, plain};

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

/// Decodes a written-out case's inputs and computes its expectation.
pub(crate) fn concrete(case: &ConcreteCase) -> Result<Prepared<'_>, RunError> {
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
        input: Resolution {
            message: decode(&case.message)?,
            payload: decode_optional(case.payload.as_ref())?,
            props: decode_optional(case.props.as_ref())?,
            locale: case.locale.clone(),
            id: decode_optional(case.message_id.as_ref())?,
            defaults: decode_optional(case.defaults.as_ref())?,
            modifiers: case.modifiers.clone(),
        },
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

// A message nesting the stated number of levels, each selecting the option
// that holds the next. The innermost declares a fallback, so a level the
// nesting limit refuses is visible as the chain it takes. It is written as the
// levels around the innermost and their closings, so its cost is its length.
fn nest(levels: i64) -> String {
    let around = count(levels).saturating_sub(1);

    format!(
        "{}{{{{v; a:settled; default:fallback;}}}}{}",
        "{{v; a:".repeat(around),
        ";}}".repeat(around)
    )
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

fn generated_input(message: String, payload: Value) -> Resolution {
    Resolution {
        payload,
        id: ID.into(),
        ..Resolution::message(message)
    }
}

/// Builds a generated case from the adapter's limits.
pub(crate) fn generated(
    case: &GeneratedCase,
    limits: Limits,
) -> Result<Prepared<'static>, RunError> {
    let x = |length: i64| Value::from("x".repeat(count(length)));
    let (message, payload, output, reports) = match case.generate.as_str() {
        "output-at-limit" => (
            writes(limits.output),
            payload(&[("v", "a".into())]),
            "x".repeat(count(limits.output)),
            vec![],
        ),
        // The placeholder resolves to the empty string and the message's own
        // text still renders, because that text is the caller's and is
        // counted against no limit.
        "output-over-limit" => (
            format!("A{}B", writes(limits.output.saturating_add(1))),
            payload(&[("v", "a".into())]),
            "AB".to_owned(),
            vec![limit("output-limit", "limit", limits.output)],
        ),
        // A result the output has no room for spends nothing, so the
        // placeholder after it is resolved and carried.
        "output-over-limit-continues" => (
            format!("{}{{{{w}}}}", writes(limits.output.saturating_add(1))),
            payload(&[("v", "a".into()), ("w", "tail".into())]),
            "tail".to_owned(),
            vec![limit("output-limit", "limit", limits.output)],
        ),
        "read-at-limit" => (
            READS.to_owned(),
            payload(&[("v", x(limits.read))]),
            "ok".to_owned(),
            vec![],
        ),
        // The budget is tested before a placeholder reads, so the one that
        // spent it past the limit still resolves and the one after it pays.
        "read-over-limit" => (
            format!("{READS}{{{{w}}}}"),
            payload(&[
                ("v", x(limits.read.saturating_add(1))),
                ("w", "tail".into()),
            ]),
            "ok".to_owned(),
            vec![limit("read-limit", "limit", limits.read)],
        ),
        "conversion-over-limit" => (
            "{{v; default:D}}".to_owned(),
            payload(&[("v", nodes(limits.conversion as f64 + 1.0))]),
            "D".to_owned(),
            vec![Wanted {
                code: "unserializable-value",
                origin: "payload",
                id: Some(ID.to_owned()),
                limit: None,
            }],
        ),
        "nesting-at-limit" => (
            nest(limits.nesting),
            payload(&[("v", "a".into())]),
            "settled".to_owned(),
            vec![],
        ),
        // The innermost placeholder is the one refused, and a nesting limit is
        // a defect of the message: it takes its fallback chain, and every
        // level around it selects the option that holds it.
        "nesting-over-limit" => (
            nest(limits.nesting.saturating_add(1)),
            payload(&[("v", "a".into())]),
            "fallback".to_owned(),
            vec![limit("nesting-limit", "message", limits.nesting)],
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
        input: generated_input(message, payload),
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
