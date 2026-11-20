use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use crate::js;
use crate::set::{Behaviour, Keeping};
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

    /// Whether the adapter states a stand-in for the host's `undefined`: a
    /// value its implementation reads as the caller having passed nothing.
    /// Where it states none, the runner leaves out whatever holds
    /// [`Value::Undefined`] before the adapter is handed it, and the
    /// constructions at and one past the conversion limit, which count their
    /// nodes in members that hold it.
    fn undefined(&self) -> bool {
        false
    }

    /// The bound section 13 lets an implementation keep on the counted reads
    /// producing their text at once on one thread, the resolution asked for
    /// not counted, or `None` where it states none. It builds the
    /// constructions of kept reads; a bound below 0 is no bound, and refuses
    /// the run.
    fn reentry(&self) -> Option<i64> {
        None
    }

    /// Whether a modifier can keep an input past its call (section 11.3).
    /// `false` leaves out the cases that read a kept input after the modifier
    /// that kept it returned.
    fn keeps(&self) -> bool {
        true
    }

    /// Whether the bound [`reentry`](Adapter::reentry) states counts a
    /// resolution asked for inside another as it counts a read (section 13):
    /// `Value::Bool(true)` builds `reentry-resolutions-over-limit`, whose
    /// [`Renest`] the adapter registers. Unset or `false`, the claim is
    /// unmade and that case is left out. It is typed as loosely as the other
    /// claims: a statement that is neither refuses the run.
    fn resolutions(&self) -> Value {
        Value::Undefined
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

/// A property of a request the host's facility cannot express (section
/// 11.2). With `values` and `when` empty it holds for every value of the
/// property in every request of the facility; `values` narrows it to the
/// values listed, and `when` to the requests whose other properties hold
/// each of the values it names. An empty list is no narrowing, and an entry
/// that names no property is no claim.
#[derive(Clone, Debug)]
pub struct Unexpressible {
    /// The facility, as ECMA-402 names it.
    pub api: String,
    /// The property.
    pub property: String,
    /// The values of the property the entry holds for.
    pub values: Vec<Value>,
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
    /// The host-defined modifiers to register over one list of what was kept,
    /// by name, each a behaviour of [`Keeping`]. The adapter builds them
    /// afresh for this resolution, sharing that list among them and with the
    /// resolution a `nest`, a `keep-nest` or a `keep-nest-ne` asks for while
    /// its call is in progress.
    pub keeping: Option<Vec<(String, Keeping)>>,
    /// The `descend` to register under that name beside them, sharing their
    /// list.
    pub descend: Option<Descend>,
    /// The `renest` to register under that name, alone.
    pub renest: Option<Renest>,
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
            keeping: None,
            descend: None,
            renest: None,
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

/// The message a `nest` asks a resolution of.
pub const NESTED: &str = "{{x:take; default:F}}";

/// The message a `keep-nest` asks a resolution of.
pub const NESTED_TAKING: &str = "{{x:taking; default:F}}";

/// The message a `keep-nest-ne` asks a resolution of. Its `ne` is the format's
/// own: `x` holds the empty string, which differs from `b`, so it selects that
/// option and produces it during its call.
pub const NESTED_DEFINED: &str = "{{x:ne; b:{{x:taking; default:F}}}}";

/// The message a [`Descend`] asks a resolution of while it has further to go.
pub const DESCEND: &str = "{{x:descend; default:F}}";

/// The `descend` of the set's README, which the two constructions built
/// around a resolution a modifier asks for register beside the behaviours of
/// [`Keeping`], under that name, where the adapter states that its bound
/// counts resolutions. The adapter builds it into a modifier whose call
/// answers what [`Descend::call`] answers, over a resolution, by its
/// implementation and through the means it hands a modifier, of the message
/// it is handed, with `x` holding the empty string and the same behaviours
/// and this same `descend` registered, sharing their list of what was kept,
/// asked for while the call is in progress. Its clones share one count of
/// the calls in progress.
#[derive(Clone, Debug)]
pub struct Descend {
    hops: usize,
    inner: &'static str,
    standing: Arc<AtomicUsize>,
}

impl Descend {
    pub(crate) fn new(hops: usize, inner: &'static str) -> Self {
        Self {
            hops,
            inner,
            standing: Arc::default(),
        }
    }

    /// What `descend` answers: the output `resolve` answers of the message it
    /// is handed, [`DESCEND`] with fewer calls in progress than the descent
    /// has hops and the message the descent leads to with as many, or a raise
    /// where it answers none. A refusal that unwinds out of `resolve` passes
    /// on.
    pub fn call(&self, resolve: impl FnOnce(&str) -> Option<String>) -> Answer {
        let standing = self.standing.fetch_add(1, Ordering::SeqCst) + 1;
        let _standing = Standing(&self.standing);

        match resolve(if standing < self.hops {
            DESCEND
        } else {
            self.inner
        }) {
            Some(output) => Answer::Text(output),
            None => {
                Answer::Raise("The resolution descend asked for answered no output.".to_owned())
            }
        }
    }
}

/// The message a [`Renest`] asks a resolution of: the one it is registered in.
pub const RENESTED: &str = "[{{x:renest; default:F}}]";

/// The `renest` of the set's README, which `reentry-resolutions-over-limit`
/// registers alone, under that name. The adapter builds it into a modifier
/// whose call answers what [`Renest::call`] answers, over a resolution, by
/// its implementation and through the means it hands a modifier, of
/// [`RENESTED`], with `x` holding the empty string and this same `renest`
/// registered alone, asked for while the call is in progress. Its clones
/// share one count of the calls in progress.
#[derive(Clone, Debug)]
pub struct Renest {
    bound: usize,
    standing: Arc<AtomicUsize>,
}

// One call of `renest` or `descend` in progress, counted while it lives, so a refusal
// that unwinds through it leaves the count right.
struct Standing<'a>(&'a AtomicUsize);

impl Drop for Standing<'_> {
    fn drop(&mut self) {
        self.0.fetch_sub(1, Ordering::SeqCst);
    }
}

impl Renest {
    pub(crate) fn new(bound: usize) -> Self {
        Self {
            bound,
            standing: Arc::default(),
        }
    }

    /// What `renest` answers: the output `resolve` answers, or a raise where
    /// it answers none. With more calls in progress than the bound the
    /// adapter states, it calls nothing and answers `unrefused`, so an
    /// implementation that refuses no resolution fails the case in place of
    /// nesting until its host ends it. A refusal that unwinds out of
    /// `resolve` passes on.
    pub fn call(&self, resolve: impl FnOnce() -> Option<String>) -> Answer {
        if self.standing.load(Ordering::SeqCst) > self.bound {
            return Answer::Text("unrefused".to_owned());
        }

        self.standing.fetch_add(1, Ordering::SeqCst);

        let _standing = Standing(&self.standing);

        match resolve() {
            Some(output) => Answer::Text(output),
            None => Answer::Raise("The resolution renest asked for answered no output.".to_owned()),
        }
    }
}

/// A read of a kept input as `take` and `first` answer it: the text read, or
/// `empty` for the empty string, so a modifier that did not go on shows.
pub fn shown(text: &str) -> String {
    if text.is_empty() {
        "empty".to_owned()
    } else {
        text.to_owned()
    }
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
