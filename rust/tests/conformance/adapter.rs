#![cfg_attr(
    not(feature = "intl"),
    expect(
        dead_code,
        reason = "the text reading and the parity trial run on the Intl level alone"
    )
)]

// The adapter of SPEC.md section 14.3: what this implementation supplies so
// the conformance set can drive it. The levels, the limits and what the
// facility cannot express are the statements the README makes, read off the
// crate rather than spelled a second time.

use std::collections::HashMap;
use std::fmt;
use std::rc::Rc;
use std::sync::{Arc, Mutex, PoisonError};

use curly_message_conformance::{
    Adapter, Answer, BehaviourInput, CstClaim, ExposedRequest, Limits, RNode, Report, Resolution,
    Resolved, TreeAnswer, Unexpressible, Value as RunnerValue, behave, js,
};
use curly_message_parser::{
    Api, Array, Category, Context, Input, MAX_CONVERSION_NODES, MAX_NESTING, MAX_OUTPUT_LENGTH,
    MAX_READ_LENGTH, Message, ModifierError, ModifierInput, Object, Parser, ParserBuilder, Request,
    Unit, Value, cst,
};
#[cfg(feature = "intl")]
use curly_message_parser::{IcuIntl, Intl, Refusal};

/// The requests of the set whose text ICU4X's locale data spells otherwise
/// than the Node release the set's expectations were recorded on: the
/// locale, the request as [`request_text`] writes it, and why. On the text
/// reading the adapter exposes the request of a resolution that makes one of
/// them, so the case is measured on the request; the parity trial holds the
/// list to the requests that differ.
pub const TEXT_DIFFERENCES: &[(&str, &str, &str)] = &[(
    "en",
    r#"DateTimeFormat {"dateStyle":"medium","timeStyle":"short","timeZone":"UTC"} 1709632800000"#,
    "CLDR writes a narrow no-break space before the day period, which V8 writes as a space",
)];

/// How the adapter answers.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Reading {
    /// Every resolution exposes the requests it made.
    Exposure,
    /// No resolution exposes them, but for those of [`TEXT_DIFFERENCES`]: the
    /// runner's host performs each case's request and the output is held to
    /// its text.
    Text,
}

/// The parser, driven for the set.
#[derive(Debug)]
pub struct ParserAdapter {
    reading: Reading,
}

impl ParserAdapter {
    pub const fn new(reading: Reading) -> Self {
        Self { reading }
    }
}

/// A request as the parity trial and [`TEXT_DIFFERENCES`] name it.
pub fn request_text(api: &str, options: &RunnerValue, input: &RunnerValue) -> String {
    let options = match options {
        RunnerValue::Undefined => RunnerValue::object(Vec::new()),
        other => other.clone(),
    };

    format!("{api} {} {}", js::canonical(&options), js::canonical(input))
}

/// A request held past the resolution that made it, its options copied.
#[derive(Clone, Debug)]
pub struct Kept {
    pub api: Api,
    pub options: Object,
    pub input: Input,
}

impl Kept {
    fn of(request: &Request<'_>) -> Self {
        Self {
            api: request.api,
            options: request.options.to_object(),
            input: request.input,
        }
    }

    /// The request, as the parser hands one to its facility.
    pub fn request(&self) -> Request<'_> {
        Request::new(self.api, &self.options, self.input)
    }
}

/// A request the set states.
pub fn request(api: &str, options: &RunnerValue, input: &RunnerValue) -> Option<Kept> {
    let api = [
        Api::NumberFormat,
        Api::DateTimeFormat,
        Api::RelativeTimeFormat,
        Api::PluralRules,
    ]
    .into_iter()
    .find(|known| known.as_str() == api)?;
    let input = match input {
        RunnerValue::Number(number) => Input::Number(*number),
        RunnerValue::Array(pair) => match pair.as_slice() {
            [RunnerValue::Number(count), RunnerValue::String(unit)] => Input::Relative(
                *count,
                [
                    Unit::Second,
                    Unit::Minute,
                    Unit::Hour,
                    Unit::Day,
                    Unit::Week,
                    Unit::Month,
                    Unit::Year,
                ]
                .into_iter()
                .find(|known| known.as_str() == unit)?,
            ),
            _ => return None,
        },
        _ => return None,
    };
    let options = Converter::default()
        .object(options)
        .unwrap_or_else(Object::new);

    Some(Kept {
        api,
        options,
        input,
    })
}

type Seen = Arc<Mutex<Vec<(Kept, Option<Category>)>>>;

// The requests a resolution makes, in the order it makes them, each handed on
// to ICU4X; a selection is recorded with the category it was answered.
#[cfg(feature = "intl")]
struct Recording {
    inner: IcuIntl,
    seen: Seen,
}

#[cfg(feature = "intl")]
impl Intl for Recording {
    fn format(&self, locale: &str, request: &Request<'_>) -> Result<String, Refusal> {
        self.seen
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .push((Kept::of(request), None));
        self.inner.format(locale, request)
    }

    fn select(&self, locale: &str, request: &Request<'_>) -> Result<Category, Refusal> {
        let answer = self.inner.select(locale, request);

        self.seen
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .push((Kept::of(request), answer.as_ref().ok().copied()));

        answer
    }
}

#[cfg(feature = "intl")]
fn with_intl(builder: ParserBuilder, seen: &Seen) -> ParserBuilder {
    builder.intl(Recording {
        inner: IcuIntl::new(),
        seen: Arc::clone(seen),
    })
}

#[cfg(not(feature = "intl"))]
fn with_intl(builder: ParserBuilder, _: &Seen) -> ParserBuilder {
    builder
}

// The `unserializable` tag: a value of the application's own type whose
// `Display` fails, which section 4's conversion describes as nothing.
struct Undescribable;

impl fmt::Display for Undescribable {
    fn fmt(&self, _: &mut fmt::Formatter<'_>) -> fmt::Result {
        Err(fmt::Error)
    }
}

// The runner's values into the parser's. A container the runner shares is
// shared here too, so a tree that doubles its visits per level by sharing is
// built once per level rather than once per visit.
#[derive(Default)]
struct Converter {
    arrays: HashMap<*const Vec<RunnerValue>, Value>,
    objects: HashMap<*const Vec<(String, RunnerValue)>, Value>,
}

impl Converter {
    fn value(&mut self, value: &RunnerValue) -> Value {
        match value {
            RunnerValue::Undefined => Value::Undefined,
            RunnerValue::Null => Value::Null,
            RunnerValue::Bool(flag) => Value::Bool(*flag),
            RunnerValue::Number(number) => Value::Number(*number),
            RunnerValue::String(text) => Value::from(text.as_str()),
            RunnerValue::Unserializable => Value::host(Undescribable),
            RunnerValue::Array(elements) => {
                if let Some(built) = self.arrays.get(&Rc::as_ptr(elements)) {
                    return built.clone();
                }

                let built = Value::from(
                    elements
                        .iter()
                        .map(|element| self.value(element))
                        .collect::<Array>(),
                );

                self.arrays.insert(Rc::as_ptr(elements), built.clone());
                built
            }
            RunnerValue::Object(members) => {
                if let Some(built) = self.objects.get(&Rc::as_ptr(members)) {
                    return built.clone();
                }

                let built = Value::from(self.object_of(members));

                self.objects.insert(Rc::as_ptr(members), built.clone());
                built
            }
        }
    }

    fn object_of(&mut self, members: &[(String, RunnerValue)]) -> Object {
        members.iter().fold(Object::new(), |object, (key, member)| {
            let member = self.value(member);

            object.with(key.as_str(), member)
        })
    }

    // A payload, props or defaults: keyed data, or nothing passed.
    fn object(&mut self, value: &RunnerValue) -> Option<Object> {
        value.as_object().map(|members| self.object_of(members))
    }
}

// The parser's values back into the runner's, for what a modifier is handed
// and what a resolution asked of its facility.
fn out(value: &Value) -> RunnerValue {
    match value {
        Value::Undefined => RunnerValue::Undefined,
        Value::Null => RunnerValue::Null,
        Value::Bool(flag) => RunnerValue::Bool(*flag),
        Value::Number(number) | Value::Date(number) => RunnerValue::Number(*number),
        #[expect(clippy::cast_precision_loss, reason = "a JSON number is a double")]
        Value::Integer(number) => RunnerValue::Number(*number as f64),
        #[expect(clippy::cast_precision_loss, reason = "a JSON number is a double")]
        Value::Unsigned(number) => RunnerValue::Number(*number as f64),
        Value::String(text) => RunnerValue::String(text.to_string()),
        Value::Array(elements) => RunnerValue::array(elements.iter().map(out).collect()),
        Value::Object(members) => out_object(members),
        Value::Host(host) => {
            let mut text = String::new();

            match fmt::write(&mut text, format_args!("{host}")) {
                Ok(()) => RunnerValue::String(text),
                Err(fmt::Error) => RunnerValue::Unserializable,
            }
        }
    }
}

fn out_object(object: &Object) -> RunnerValue {
    RunnerValue::object(
        object
            .iter()
            .map(|(key, member)| (key.to_owned(), out(member)))
            .collect(),
    )
}

fn exposed((request, category): &(Kept, Option<Category>)) -> ExposedRequest {
    ExposedRequest {
        api: request.api.as_str().to_owned(),
        options: out_object(&request.options),
        input: match request.input {
            Input::Number(number) => RunnerValue::Number(number),
            Input::Relative(count, unit) => {
                RunnerValue::array(vec![count.into(), unit.as_str().into()])
            }
        },
        category: category.map(|category| category.as_str().to_owned()),
    }
}

struct Handed<'a, 'w>(&'a ModifierInput<'w>);

impl BehaviourInput for Handed<'_, '_> {
    fn value(&self) -> String {
        self.0.value().to_owned()
    }

    fn options(&self) -> Vec<(String, String)> {
        (0..self.0.options())
            .map(|at| {
                (
                    self.0.option_key(at).unwrap_or_default().to_owned(),
                    self.0.option_value(at).unwrap_or_default().to_owned(),
                )
            })
            .collect()
    }

    fn props(&self) -> Vec<(String, RunnerValue)> {
        self.0
            .props()
            .iter()
            .map(|(key, value)| (key.to_owned(), out(value)))
            .collect()
    }

    fn locale(&self) -> Option<String> {
        self.0.locale().map(str::to_owned)
    }

    fn default(&self) -> String {
        self.0.default().to_owned()
    }
}

type Reports = Arc<Mutex<Vec<Report>>>;

impl Adapter for ParserAdapter {
    fn levels(&self) -> Vec<String> {
        with_intl(Parser::builder(), &Seen::default())
            .build()
            .levels()
            .iter()
            .map(|level| level.as_str().to_owned())
            .collect()
    }

    fn limits(&self) -> Limits {
        let count = |limit: usize| i64::try_from(limit).unwrap_or(i64::MAX);

        Limits {
            output: count(MAX_OUTPUT_LENGTH),
            read: count(MAX_READ_LENGTH),
            conversion: count(MAX_CONVERSION_NODES),
            nesting: count(MAX_NESTING),
        }
    }

    #[cfg(feature = "intl")]
    fn unexpressible(&self) -> Option<Vec<Unexpressible>> {
        Some(
            curly_message_parser::UNEXPRESSIBLE
                .iter()
                .map(|entry| Unexpressible {
                    api: entry.api.as_str().to_owned(),
                    property: entry.property.to_owned(),
                    when: entry
                        .when
                        .iter()
                        .map(|(key, value)| ((*key).to_owned(), (*value).into()))
                        .collect(),
                })
                .collect(),
        )
    }

    #[cfg(not(feature = "intl"))]
    fn unexpressible(&self) -> Option<Vec<Unexpressible>> {
        None
    }

    // CST.md section 4 lets an implementation count its spans in the unit its
    // strings are indexed by: a `str` is indexed by its UTF-8 bytes.
    fn cst(&self) -> Option<CstClaim> {
        Some(CstClaim {
            unit: "utf-8".to_owned(),
        })
    }

    fn parse(&self, message: &str) -> Result<TreeAnswer, String> {
        let tree = cst(message);
        let node = |at: curly_message_parser::NodeRef<'_>| {
            let span = at.span();

            RNode {
                kind: at.kind().as_str().to_owned(),
                start: i64::try_from(span.start).unwrap_or(i64::MAX),
                end: i64::try_from(span.end).unwrap_or(i64::MAX),
                children: Vec::new(),
                name: at.name().map(str::to_owned),
                cancels: at.cancels(),
            }
        };
        let mut nodes = vec![node(tree.root())];
        let mut stack = vec![(tree.root(), 0)];

        while let Some((parent, at)) = stack.pop() {
            for child in parent.children() {
                let index = nodes.len();

                nodes.push(node(child));
                nodes[at].children.push(index);
                stack.push((child, index));
            }
        }

        Ok(TreeAnswer { nodes })
    }

    fn resolve(&self, input: &Resolution) -> Result<Option<Resolved>, String> {
        let mut into = Converter::default();
        let reports = Reports::default();
        let seen = Seen::default();
        let mut builder = with_intl(Parser::builder(), &seen).on_report({
            let reports = Arc::clone(&reports);

            move |report| {
                reports
                    .lock()
                    .unwrap_or_else(PoisonError::into_inner)
                    .push(Report {
                        code: report.code.as_str().to_owned(),
                        origin: Some(report.origin.as_str().to_owned()),
                        id: report.id.clone(),
                        limit: report
                            .limit
                            .map(|limit| i64::try_from(limit).unwrap_or(i64::MAX)),
                    });
            }
        });

        for (name, behaviour) in input.modifiers.iter().flatten() {
            let behaviour = *behaviour;

            builder = builder.modifier(
                name.as_str(),
                move |handed: &ModifierInput<'_>| match behave(behaviour, &Handed(handed)) {
                    Answer::Text(text) => Ok(Some(Value::from(text))),
                    Answer::Nothing => Ok(None),
                    Answer::Raise(reason) => Err(ModifierError::new(reason)),
                    Answer::Value(value) => Ok(Some(Converter::default().value(&value))),
                },
            );
        }

        if let Some(defaults) = into.object(&input.defaults) {
            builder = builder.modifier_defaults(defaults);
        }

        let parser = builder.build();
        let payload = into.object(&input.payload);
        let props = into.object(&input.props);
        let mut context = Context::new();

        if let Some(payload) = &payload {
            context = context.payload(payload);
        }

        if let Some(props) = &props {
            context = context.props(props);
        }

        if let Some(locale) = &input.locale {
            context = context.locale(locale);
        }

        if let Some(id) = input.id.as_str() {
            context = context.id(id);
        }

        let message = into.value(&input.message);
        let output = parser.resolve(
            match &input.message {
                RunnerValue::String(text) => Message::Text(text),
                RunnerValue::Undefined => Message::Absent,
                _ => Message::Value(&message),
            },
            &context,
        );
        let made: Vec<ExposedRequest> = seen
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .iter()
            .map(exposed)
            .collect();
        let differs = |request: &ExposedRequest| {
            let text = request_text(&request.api, &request.options, &request.input);

            TEXT_DIFFERENCES.iter().any(|(locale, listed, _)| {
                input.locale.as_deref() == Some(*locale) && *listed == text
            })
        };
        let formats = match self.reading {
            Reading::Exposure => Some(made),
            Reading::Text => made.iter().any(differs).then_some(made),
        };
        let reports = reports
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .clone();

        Ok(Some(Resolved {
            output,
            reports: Some(reports),
            formats,
        }))
    }
}
