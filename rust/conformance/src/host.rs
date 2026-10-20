use std::collections::HashMap;
use std::sync::OnceLock;

use serde_json::Value as Json;

use crate::RunError;
use crate::cases::FormatRequest;
use crate::js;
use crate::set::{Api, VERSION};
use crate::value::{Value, plain};

/// The runner's own host, which performs a case's request where the adapter
/// exposes none (the text reading of `RUNNER.md`).
///
/// It must not be the implementation under test: a host that shares the
/// implementation's formatter puts the same defect on both sides of the
/// comparison, and the comparison then measures nothing.
pub trait FormatHost: Sync {
    /// The text a formatting request makes on this host, or the category a
    /// selection request answers. `None` where the host holds no answer for
    /// the request, which fails the case; `Err` where the host raised over
    /// it, which ends the run as the JavaScript runner's host would.
    fn perform(
        &self,
        locale: Option<&str>,
        request: &FormatRequest,
    ) -> Option<Result<String, String>>;
}

/// What the recorded host answered one request with.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Recorded {
    /// The text a formatting request made.
    Text(String),
    /// The category a selection request answered.
    Category(String),
    /// The name of the error the facility raised.
    Error(String),
}

/// A host that answers from a table recorded on Node: the answer of Node's
/// `Intl` to each request the set states, performed as the JavaScript
/// runner's `format()` performs it, in UTC. Rust has no `Intl` of its own,
/// and the formatter at hand is the one an implementation in Rust would use,
/// so the host the text reading compares against is a recording of the one
/// the set was written against.
#[derive(Debug)]
pub struct GoldenHost {
    node: String,
    icu: String,
    cldr: String,
    entries: Vec<Entry>,
    answers: HashMap<String, usize>,
}

#[derive(Debug)]
struct Entry {
    name: String,
    locale: Option<String>,
    api: Api,
    options: Option<Json>,
    input: Json,
    answer: Recorded,
}

#[derive(serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct File {
    node: String,
    icu: String,
    cldr: String,
    answers: Vec<RawEntry>,
}

#[derive(serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct RawEntry {
    name: String,
    locale: Option<String>,
    api: Api,
    options: Option<Json>,
    input: Json,
    answer: RawAnswer,
}

#[derive(serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct RawAnswer {
    text: Option<String>,
    category: Option<String>,
    error: Option<String>,
}

const RECORDED: &str = include_str!("../set/golden-4.1.0.json");

// The text a request is looked up by: its entries, as the runner compares
// them, except that the sign of a zero is kept, since a host writes `-0` and
// `0` apart.
fn exact(value: &Value) -> String {
    match value {
        Value::Number(number) if *number == 0.0 && number.is_sign_negative() => "-0".to_owned(),
        Value::Array(elements) => format!(
            "[{}]",
            elements.iter().map(exact).collect::<Vec<_>>().join(",")
        ),
        Value::Object(members) => {
            let mut sorted: Vec<_> = members.iter().collect();

            sorted.sort_by(|a, b| js::by_code_units(&a.0, &b.0));

            format!(
                "{{{}}}",
                sorted
                    .iter()
                    .map(|(key, member)| format!("{}:{}", js::quote(key), exact(member)))
                    .collect::<Vec<_>>()
                    .join(",")
            )
        }
        other => js::canonical(other),
    }
}

fn options_or_empty(options: &Value) -> Value {
    match options {
        Value::Undefined | Value::Null => Value::object(Vec::new()),
        other => other.clone(),
    }
}

/// The text a request is looked up by in a recorded table.
pub(crate) fn key(locale: Option<&str>, request: &FormatRequest) -> String {
    format!(
        "{}\n{}\n{}\n{}",
        locale.map_or_else(|| "-".to_owned(), |locale| format!("={locale}")),
        request.api.as_str(),
        exact(&options_or_empty(&request.options)),
        exact(&request.input)
    )
}

impl GoldenHost {
    /// Reads a recorded table.
    pub fn read(text: &str) -> Result<Self, RunError> {
        let file: File = serde_json::from_str(text)
            .map_err(|error| RunError::new(format!("The recorded answers do not read: {error}")))?;
        let mut answers = HashMap::new();
        let mut entries = Vec::with_capacity(file.answers.len());

        for raw in file.answers {
            let answer = match raw.answer {
                RawAnswer {
                    text: Some(text),
                    category: None,
                    error: None,
                } => Recorded::Text(text),
                RawAnswer {
                    text: None,
                    category: Some(category),
                    error: None,
                } => Recorded::Category(category),
                RawAnswer {
                    text: None,
                    category: None,
                    error: Some(error),
                } => Recorded::Error(error),
                _ => {
                    return Err(RunError::new(format!(
                        "The recorded answer {} is not one text, category or error.",
                        raw.name
                    )));
                }
            };
            let entry = Entry {
                name: raw.name,
                locale: raw.locale,
                api: raw.api,
                options: raw.options,
                input: raw.input,
                answer,
            };
            let looked_up = key(entry.locale.as_deref(), &entry.request());

            if answers.insert(looked_up, entries.len()).is_some() {
                return Err(RunError::new(format!(
                    "The request {} is recorded twice.",
                    entry.name
                )));
            }

            entries.push(entry);
        }

        Ok(Self {
            node: file.node,
            icu: file.icu,
            cldr: file.cldr,
            entries,
            answers,
        })
    }

    /// Node's answers to the requests of release 4.1.0 of the set, read once.
    pub fn recorded() -> Result<&'static Self, RunError> {
        static HOST: OnceLock<Result<GoldenHost, RunError>> = OnceLock::new();

        HOST.get_or_init(|| {
            let host = Self::read(RECORDED)?;

            if host.entries.is_empty() {
                return Err(RunError::new(format!(
                    "The recorded answers of set {VERSION} are empty."
                )));
            }

            Ok(host)
        })
        .as_ref()
        .map_err(Clone::clone)
    }

    /// The version of Node the answers were recorded on.
    pub fn node(&self) -> &str {
        &self.node
    }

    /// The version of ICU that Node carried.
    pub fn icu(&self) -> &str {
        &self.icu
    }

    /// The version of CLDR that Node carried.
    pub fn cldr(&self) -> &str {
        &self.cldr
    }

    /// Every recorded request, in the order of the table: its name, its
    /// locale, the request and the answer.
    pub fn requests(&self) -> Vec<(&str, Option<&str>, FormatRequest, &Recorded)> {
        self.entries
            .iter()
            .map(|entry| {
                (
                    entry.name.as_str(),
                    entry.locale.as_deref(),
                    entry.request(),
                    &entry.answer,
                )
            })
            .collect()
    }
}

impl Entry {
    fn request(&self) -> FormatRequest {
        FormatRequest {
            api: self.api,
            options: self.options.as_ref().map_or(Value::Undefined, plain),
            input: plain(&self.input),
        }
    }
}

impl FormatHost for GoldenHost {
    fn perform(
        &self,
        locale: Option<&str>,
        request: &FormatRequest,
    ) -> Option<Result<String, String>> {
        let entry = self
            .entries
            .get(*self.answers.get(&key(locale, request))?)?;

        Some(match &entry.answer {
            Recorded::Text(text) | Recorded::Category(text) => Ok(text.clone()),
            Recorded::Error(error) => Err(error.clone()),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_recorded_answer_keeps_the_sign_of_zero_and_ignores_the_order_of_properties() {
        let host = GoldenHost::read(
            r#"{ "node": "n", "icu": "i", "cldr": "c", "answers": [
                { "name": "zero", "locale": "en", "api": "NumberFormat", "input": 0, "answer": { "text": "0" } },
                { "name": "minus", "locale": "en", "api": "NumberFormat", "input": -0, "answer": { "text": "-0" } },
                { "name": "styled", "api": "NumberFormat", "options": { "a": 1, "b": 2 }, "input": 1, "answer": { "error": "RangeError" } }
            ] }"#,
        )
        .unwrap();
        let number = |options: Value, input: f64| FormatRequest {
            api: Api::NumberFormat,
            options,
            input: Value::Number(input),
        };
        let swapped = Value::object(vec![
            ("b".to_owned(), Value::Number(2.0)),
            ("a".to_owned(), Value::Number(1.0)),
        ]);

        assert_eq!(
            host.perform(Some("en"), &number(Value::Undefined, -0.0)),
            Some(Ok("-0".to_owned()))
        );
        assert_eq!(
            host.perform(Some("en"), &number(Value::object(Vec::new()), 0.0)),
            Some(Ok("0".to_owned()))
        );
        assert_eq!(
            host.perform(None, &number(swapped, 1.0)),
            Some(Err("RangeError".to_owned()))
        );
        assert_eq!(
            host.perform(Some("de"), &number(Value::Undefined, 0.0)),
            None
        );
        assert!(
            GoldenHost::read(
                r#"{ "node": "n", "icu": "i", "cldr": "c", "answers": [
                    { "name": "a", "api": "PluralRules", "input": 1, "answer": { "category": "one" } },
                    { "name": "b", "api": "PluralRules", "input": 1, "answer": { "category": "one" } }
                ] }"#
            )
            .is_err()
        );
    }
}
