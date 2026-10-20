use crate::js;
use crate::set::Behaviour;
use crate::value::Value;

/// The implementation's side of the contract of the set's README: what it
/// claims about itself, and the calls that drive it.
///
/// The claims are typed loosely on purpose. A level is text and a limit a
/// signed count, so a claim outside the format's vocabulary can be written,
/// and the runner refuses the run over it rather than the type system ruling
/// it out: the catalogue of defects holds adapters that make such claims, and
/// a runner that could not be handed one could not be audited on it.
///
/// An adapter is shared between the threads a test harness runs cases on, so
/// it is `Sync`; what it hands back is the runner's to read on the thread
/// that asked.
pub trait Adapter: Sync {
    /// The levels of section 2 the implementation claims: `core`, `intl`,
    /// `extensions`.
    fn levels(&self) -> Vec<String>;

    /// The limits of section 13 the implementation permits, which the
    /// generated cases are built from.
    fn limits(&self) -> Limits;

    /// The properties of a formatting request the host's facility cannot
    /// express (section 11.2), or `None` where it states none.
    fn unexpressible(&self) -> Option<Vec<Unexpressible>> {
        None
    }

    /// The tree of `CST.md`, where the implementation offers one: the unit its
    /// spans are counted in. `None` leaves the tree cases out.
    fn cst(&self) -> Option<CstClaim> {
        None
    }

    /// The tree of a message, as an arena whose first node is the root. An
    /// `Err` is a raise.
    fn parse(&self, message: &str) -> Result<TreeAnswer, String> {
        let _ = message;

        Err("The adapter offers no concrete syntax tree.".to_owned())
    }

    /// One resolution: an `Err` is a raise, and `Ok(None)` an answer that is
    /// no resolution.
    fn resolve(&self, input: &Resolution) -> Result<Option<Resolved>, String>;
}

/// The limits of section 13, as an adapter declares them.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Limits {
    /// The output limit, in the host's string unit.
    pub output: i64,
    /// The read limit, in the host's string unit.
    pub read: i64,
    /// The nodes one conversion may visit.
    pub conversion: i64,
    /// How deep messages may nest.
    pub nesting: i64,
}

/// A property of a request the host's facility cannot express. With `when`
/// empty it holds for every request of the facility, which is what release
/// 4.1.0 of the set can state; with `when`, only for a request whose
/// properties hold each of those values, the shape the Rust implementation
/// proposes for a facility that expresses a property under one style and not
/// under another.
#[derive(Clone, Debug)]
pub struct Unexpressible {
    /// The facility, as ECMA-402 names it.
    pub api: String,
    /// The property.
    pub property: String,
    /// The values other properties of the request hold where the entry holds.
    pub when: Vec<(String, Value)>,
}

/// What an implementation states about the tree it offers.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct CstClaim {
    /// The unit its spans are counted in: `utf-8`, `utf-16` or `code-point`.
    pub unit: String,
}

/// A tree as an adapter answers with it: an arena of nodes, the root first.
/// A child is an index into the arena, so an answer may point anywhere,
/// itself included; the runner fails such an answer rather than following it
/// forever.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct TreeAnswer {
    /// The nodes, the root at index 0.
    pub nodes: Vec<RNode>,
}

/// A node of a [`TreeAnswer`] (`CST.md` section 8).
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct RNode {
    /// The kind, as `CST.md` section 6 spells it.
    pub kind: String,
    /// Where the span starts, in the declared unit.
    pub start: i64,
    /// Where the span ends, in the declared unit.
    pub end: i64,
    /// The children, as indices into the arena; none for a leaf.
    pub children: Vec<usize>,
    /// For a key, a modifier or an option key: the span unescaped.
    pub name: Option<String>,
    /// For an escape: whether it cancels.
    pub cancels: Option<bool>,
}

/// One resolution's inputs, decoded.
#[derive(Clone, Debug)]
pub struct Resolution {
    /// The message: text, or a value of another shape, or the host's absence.
    pub message: Value,
    /// The payload.
    pub payload: Value,
    /// The caller's props.
    pub props: Value,
    /// The locale.
    pub locale: Option<String>,
    /// The message's id, as the case writes it.
    pub id: Value,
    /// The implementation-configured defaults.
    pub defaults: Value,
    /// The host-defined modifiers to register, by name, each a behaviour of
    /// the catalogue; [`behave`] answers one.
    pub modifiers: Option<Vec<(String, Behaviour)>>,
}

impl Resolution {
    /// A resolution of a message alone.
    pub fn message(message: impl Into<Value>) -> Self {
        Self {
            message: message.into(),
            payload: Value::Undefined,
            props: Value::Undefined,
            locale: None,
            id: Value::Undefined,
            defaults: Value::Undefined,
            modifiers: None,
        }
    }
}

/// What an implementation produced for one resolution.
#[derive(Clone, Debug, PartialEq)]
pub struct Resolved {
    /// The output.
    pub output: String,
    /// The reports, in the order they were made; `None` where the
    /// implementation does not report.
    pub reports: Option<Vec<Report>>,
    /// The formatting requests the resolution made, where the implementation
    /// exposes them (section 11.2).
    pub formats: Option<Vec<ExposedRequest>>,
}

impl Resolved {
    /// An output with nothing observed about it.
    pub fn output(output: impl Into<String>) -> Self {
        Self {
            output: output.into(),
            reports: None,
            formats: None,
        }
    }
}

/// A report as an adapter carries it: its code, and whichever of the fields
/// of section 14.2 the implementation carries.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Report {
    /// The code.
    pub code: String,
    /// The origin.
    pub origin: Option<String>,
    /// The id.
    pub id: Option<String>,
    /// The limit, where the report is about one.
    pub limit: Option<i64>,
}

impl Report {
    pub(crate) fn shown(&self) -> Value {
        let mut members = vec![("code".to_owned(), Value::from(self.code.as_str()))];

        if let Some(origin) = &self.origin {
            members.push(("origin".to_owned(), origin.as_str().into()));
        }

        if let Some(id) = &self.id {
            members.push(("id".to_owned(), id.as_str().into()));
        }

        if let Some(limit) = self.limit {
            members.push(("limit".to_owned(), Value::Number(limit as f64)));
        }

        Value::object(members)
    }
}

/// A formatting request a resolution made, as an implementation exposes it.
#[derive(Clone, Debug)]
pub struct ExposedRequest {
    /// The facility, as ECMA-402 names it.
    pub api: String,
    /// The options the facility is constructed with.
    pub options: Value,
    /// The input.
    pub input: Value,
    /// For a selection request: the category the host answered.
    pub category: Option<String>,
}

// Exposed requests are compared by their entries, so equality is the
// canonical text's: the order of a request's properties is the host's.
impl PartialEq for ExposedRequest {
    fn eq(&self, other: &Self) -> bool {
        self.api == other.api
            && js::canonical(&self.options) == js::canonical(&other.options)
            && js::canonical(&self.input) == js::canonical(&other.input)
            && self.category == other.category
    }
}

impl ExposedRequest {
    pub(crate) fn shown(&self) -> Value {
        let mut members = vec![
            ("api".to_owned(), Value::from(self.api.as_str())),
            ("options".to_owned(), self.options.clone()),
            ("input".to_owned(), self.input.clone()),
        ];

        if let Some(category) = &self.category {
            members.push(("category".to_owned(), category.as_str().into()));
        }

        Value::object(members)
    }
}

/// What a host-defined modifier is handed (section 11), as the catalogue's
/// behaviours read it. An adapter implements it over its implementation's
/// own input.
pub trait BehaviourInput {
    /// The value, as text.
    fn value(&self) -> String;
    /// Every option, in the order the message writes them, its value as text.
    fn options(&self) -> Vec<(String, String)>;
    /// The props the modifier is handed, in the order they are held.
    fn props(&self) -> Vec<(String, Value)>;
    /// The locale, where there is one.
    fn locale(&self) -> Option<String>;
    /// The default, read through the chain.
    fn default(&self) -> String;
}

/// What a behaviour answers.
#[derive(Clone, Debug)]
pub enum Answer {
    /// Text.
    Text(String),
    /// No answer.
    Nothing,
    /// A raise, saying why.
    Raise(String),
    /// A value of another shape.
    Value(Value),
}

/// A behaviour of the set's catalogue, answered over a modifier's input.
pub fn behave(behaviour: Behaviour, input: &dyn BehaviourInput) -> Answer {
    match behaviour {
        Behaviour::Upper => Answer::Text(input.value().to_ascii_uppercase()),
        Behaviour::Echo => Answer::Text(input.value()),
        Behaviour::Empty => Answer::Text(String::new()),
        Behaviour::Nothing => Answer::Nothing,
        Behaviour::Raise => Answer::Raise("The raise behaviour raised.".to_owned()),
        Behaviour::Default => Answer::Text(input.default()),
        Behaviour::Options => Answer::Text(
            input
                .options()
                .iter()
                .map(|(key, value)| format!("{key}={value}"))
                .collect::<Vec<_>>()
                .join(","),
        ),
        Behaviour::Props => {
            let mut props = input.props();

            props.sort_by(|a, b| js::by_code_units(&a.0, &b.0));

            let written = props
                .iter()
                .map(|(name, value)| match js::stringify(value) {
                    Ok(text) => Ok(format!(
                        "{name}={}",
                        text.unwrap_or_else(|| "undefined".to_owned())
                    )),
                    Err(js::Unserializable) => Err(()),
                })
                .collect::<Result<Vec<_>, _>>();

            match written {
                Ok(written) => Answer::Text(written.join(",")),
                Err(()) => Answer::Raise("A prop does not serialize.".to_owned()),
            }
        }
        Behaviour::Locale => Answer::Text(input.locale().unwrap_or_else(|| "none".to_owned())),
        Behaviour::Object => Answer::Value(Value::object(vec![(
            "answer".to_owned(),
            input.value().into(),
        )])),
    }
}
