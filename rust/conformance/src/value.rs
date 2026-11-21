use std::collections::HashMap;
use std::rc::Rc;

use serde_json::Value as Json;

use crate::RunError;

/// A value of the set's inputs, decoded: JSON's shapes, plus what the tags of
/// the README stand for. The runner depends on no implementation, so it
/// decodes into this neutral value and an adapter maps it into its own.
///
/// It is plain data: reading an entry never runs code, so the `refusing` tag
/// has no value here, and the plan leaves out the cases that hold one.
#[derive(Clone, Debug)]
pub enum Value {
    /// The host's absence: the `undefined` tag, or an input a case leaves out.
    Undefined,
    /// `null`.
    Null,
    /// A boolean.
    Bool(bool),
    /// A number, as JSON and ECMAScript hold one: a double.
    Number(f64),
    /// Text.
    String(String),
    /// An array. Its elements may be shared, which the `nodes` tag builds on.
    Array(Rc<Vec<Value>>),
    /// An object, its members in the order the case writes them.
    Object(Rc<Vec<(String, Value)>>),
    /// The `unserializable` tag: a value no conversion can describe.
    Unserializable,
}

impl Value {
    /// The text this value holds, where it is text.
    pub fn as_str(&self) -> Option<&str> {
        match self {
            Self::String(text) => Some(text),
            _ => None,
        }
    }

    /// The members of this value, where it is an object.
    pub fn as_object(&self) -> Option<&[(String, Value)]> {
        match self {
            Self::Object(members) => Some(members),
            _ => None,
        }
    }

    /// The member under `key`, where this value is an object holding one.
    pub fn get(&self, key: &str) -> Option<&Value> {
        self.as_object()?
            .iter()
            .find(|(name, _)| name == key)
            .map(|(_, value)| value)
    }

    /// Whether this is the host's absence.
    pub fn is_undefined(&self) -> bool {
        matches!(self, Self::Undefined)
    }

    /// An object of the given members.
    pub fn object(members: Vec<(String, Value)>) -> Self {
        Self::Object(Rc::new(members))
    }

    /// An array of the given elements.
    pub fn array(elements: Vec<Value>) -> Self {
        Self::Array(Rc::new(elements))
    }
}

impl From<&str> for Value {
    fn from(text: &str) -> Self {
        Self::String(text.to_owned())
    }
}

impl From<String> for Value {
    fn from(text: String) -> Self {
        Self::String(text)
    }
}

impl From<f64> for Value {
    fn from(number: f64) -> Self {
        Self::Number(number)
    }
}

const TAG: &str = "$curly";

/// A value whose serialization visits at least `count` nodes. Serialization
/// follows a shared reference again every time it meets one, so each level of
/// `[below, below]` spends one node on itself and twice what `below` spends,
/// and the tree doubles its visits per level without ever holding a cycle.
pub fn nodes(count: f64) -> Value {
    let mut tree = Value::array(Vec::new());
    let mut visits = 1.0;

    while visits < count {
        tree = Value::array(vec![tree.clone(), tree]);
        visits = visits * 2.0 + 1.0;
    }

    tree
}

/// The value a fixture's JSON stands for, with no tag read: what an
/// expectation holds, compared as written.
pub fn plain(json: &Json) -> Value {
    match json {
        Json::Null => Value::Null,
        Json::Bool(flag) => Value::Bool(*flag),
        Json::Number(number) => Value::Number(number.as_f64().unwrap_or(f64::NAN)),
        Json::String(text) => Value::String(text.clone()),
        Json::Array(elements) => Value::array(elements.iter().map(plain).collect()),
        Json::Object(members) => Value::object(
            members
                .iter()
                .map(|(key, value)| (key.clone(), plain(value)))
                .collect(),
        ),
    }
}

/// The value a case's input stands for: every tagged object replaced by what
/// its tag names, at every depth, and everything else as JSON describes it.
/// An unknown tag, or a `nodes` tag without a numeric count, is the set's
/// defect and ends the run.
pub fn decode(json: &Json) -> Result<Value, RunError> {
    match json {
        Json::Array(elements) => Ok(Value::array(
            elements.iter().map(decode).collect::<Result<_, _>>()?,
        )),
        Json::Object(members) => match members.get(TAG) {
            Some(tag) => tagged(tag, members.get("count")),
            None => Ok(Value::object(
                members
                    .iter()
                    .map(|(key, value)| Ok((key.clone(), decode(value)?)))
                    .collect::<Result<_, RunError>>()?,
            )),
        },
        other => Ok(plain(other)),
    }
}

fn tagged(tag: &Json, count: Option<&Json>) -> Result<Value, RunError> {
    match tag.as_str() {
        Some("undefined") => Ok(Value::Undefined),
        Some("unserializable") => Ok(Value::Unserializable),
        Some("nodes") => match count.and_then(Json::as_f64) {
            Some(count) => Ok(nodes(count)),
            None => Err(RunError::new("A nodes tag needs a numeric count.")),
        },
        Some(REFUSING) => Err(RunError::new(
            "A refusing tag has no value here, so its case is left out rather than run.",
        )),
        _ => Err(RunError::new(format!("Unknown tag {tag}."))),
    }
}

const REFUSING: &str = "refusing";

/// Whether an input holds a `refusing` tag, at any depth.
pub(crate) fn refuses(json: &Json) -> bool {
    match json {
        Json::Array(elements) => elements.iter().any(refuses),
        Json::Object(members) => match members.get(TAG) {
            Some(tag) => tag.as_str() == Some(REFUSING),
            None => members.values().any(refuses),
        },
        _ => false,
    }
}

/// A value with every member that holds the host's absence left out, at
/// every depth: what the runner hands an adapter that states no stand-in
/// for `undefined`, for which an entry that holds it is an entry not there.
/// A shared array or object is rebuilt once and shared again, so a value the
/// `nodes` tag built stays as small as it was.
pub(crate) fn without_undefined(value: &Value) -> Value {
    strip(value, &mut HashMap::new())
}

fn strip(value: &Value, built: &mut HashMap<usize, Value>) -> Value {
    let address = match value {
        Value::Array(elements) => Rc::as_ptr(elements) as usize,
        Value::Object(members) => Rc::as_ptr(members) as usize,
        other => return other.clone(),
    };

    if let Some(stripped) = built.get(&address) {
        return stripped.clone();
    }

    let stripped = match value {
        Value::Array(elements) => Value::array(
            elements
                .iter()
                .map(|element| strip(element, built))
                .collect(),
        ),
        Value::Object(members) => Value::object(
            members
                .iter()
                .filter(|(_, member)| !member.is_undefined())
                .map(|(key, member)| (key.clone(), strip(member, built)))
                .collect(),
        ),
        other => other.clone(),
    };

    built.insert(address, stripped.clone());

    stripped
}

/// An input a case may leave out: decoded where it is written, and the
/// host's absence where it is not.
pub(crate) fn decode_optional(json: Option<&Json>) -> Result<Value, RunError> {
    json.map_or(Ok(Value::Undefined), decode)
}
