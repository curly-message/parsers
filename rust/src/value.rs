use std::collections::HashMap;
use std::fmt;
use std::ops::Deref;
use std::sync::Arc;

/// A value a payload, a props layer or a modifier's answer holds (section 4).
///
/// Rust has no dynamic value of its own, so this is the host type section 4
/// reads: [`Value::Object`] and [`Value::Array`] are the plain shapes and
/// serialize as JSON; every other value becomes text by its own conversion.
/// A value of the application's own type is handed over as [`Value::Host`]
/// and converts by its [`Display`](fmt::Display).
///
/// [`Value::Undefined`] is the host's "nothing": an entry holding it is an
/// entry nobody passed, and `None` converts to it.
#[derive(Clone, Default)]
pub enum Value {
    /// No value: an absent entry, a wrapper without a value, an answer that is
    /// nothing.
    #[default]
    Undefined,
    /// The null value, which converts to `null`.
    Null,
    /// A boolean, which converts to `true` or `false`.
    Bool(bool),
    /// A double, which converts as ECMAScript's `Number::toString` writes it.
    Number(f64),
    /// A signed integer, which converts to its decimal digits.
    Integer(i64),
    /// An unsigned integer, which converts to its decimal digits.
    Unsigned(u64),
    /// Text, which is its own conversion.
    String(String),
    /// An instant authored as a date: milliseconds since the epoch. It
    /// converts as ECMAScript's `toISOString` writes it, or to `Invalid Date`.
    Date(f64),
    /// The plain array, which converts to JSON.
    Array(Arc<Array>),
    /// The plain object, which converts to JSON.
    Object(Arc<Object>),
    /// A value of a type the application declared, which converts by its
    /// `Display`. A `Display` that fails or panics describes nothing.
    Host(Arc<dyn fmt::Display + Send + Sync>),
}

impl Value {
    /// A value of the application's own type, converted by its `Display`.
    pub fn host<T: fmt::Display + Send + Sync + 'static>(value: T) -> Self {
        Self::Host(Arc::new(value))
    }

    /// An instant authored as a date, in milliseconds since the epoch.
    pub fn date(milliseconds: f64) -> Self {
        Self::Date(milliseconds)
    }

    /// The text this value holds, where it is [`Value::String`].
    pub fn as_str(&self) -> Option<&str> {
        match self {
            Self::String(text) => Some(&**text),
            _ => None,
        }
    }

    /// The object this value is, where it is [`Value::Object`].
    pub fn as_object(&self) -> Option<&Object> {
        match self {
            Self::Object(object) => Some(object),
            _ => None,
        }
    }

    /// The array this value is, where it is [`Value::Array`].
    pub fn as_array(&self) -> Option<&Array> {
        match self {
            Self::Array(array) => Some(array),
            _ => None,
        }
    }

    /// Whether this is [`Value::Undefined`].
    pub fn is_undefined(&self) -> bool {
        matches!(self, Self::Undefined)
    }

    fn is_container(&self) -> bool {
        matches!(self, Self::Array(_) | Self::Object(_))
    }
}

// Debug output walks a tree a caller may have nested a million levels deep, so
// it runs off a stack of its own rather than one call per level.
impl fmt::Debug for Value {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        enum Open<'v> {
            Items(std::slice::Iter<'v, Value>, bool),
            Entries(std::slice::Iter<'v, (Arc<str>, Value)>, bool),
        }

        fn leaf(value: &Value, f: &mut fmt::Formatter<'_>) -> fmt::Result {
            match value {
                Value::Undefined => f.write_str("Undefined"),
                Value::Null => f.write_str("Null"),
                Value::Bool(flag) => write!(f, "Bool({flag:?})"),
                Value::Number(number) => write!(f, "Number({number:?})"),
                Value::Integer(number) => write!(f, "Integer({number:?})"),
                Value::Unsigned(number) => write!(f, "Unsigned({number:?})"),
                Value::String(text) => write!(f, "String({text:?})"),
                Value::Date(milliseconds) => write!(f, "Date({milliseconds:?})"),
                Value::Host(_) => f.write_str("Host(..)"),
                Value::Array(_) | Value::Object(_) => Ok(()),
            }
        }

        fn open<'v>(
            value: &'v Value,
            f: &mut fmt::Formatter<'_>,
            stack: &mut Vec<Open<'v>>,
        ) -> fmt::Result {
            match value {
                Value::Array(array) => {
                    stack.push(Open::Items(array.iter(), false));
                    f.write_str("[")
                }
                Value::Object(object) => {
                    stack.push(Open::Entries(object.entries.iter(), false));
                    f.write_str("{")
                }
                _ => leaf(value, f),
            }
        }

        let mut stack = Vec::new();

        open(self, f, &mut stack)?;

        while let Some(top) = stack.last_mut() {
            match top {
                Open::Items(items, written) => match items.next() {
                    Some(item) => {
                        if *written {
                            f.write_str(", ")?;
                        }
                        *written = true;
                        open(item, f, &mut stack)?;
                    }
                    None => {
                        stack.pop();
                        f.write_str("]")?;
                    }
                },
                Open::Entries(entries, written) => match entries.next() {
                    Some((key, item)) => {
                        if *written {
                            f.write_str(", ")?;
                        }
                        *written = true;
                        write!(f, "{key:?}: ")?;
                        open(item, f, &mut stack)?;
                    }
                    None => {
                        stack.pop();
                        f.write_str("}")?;
                    }
                },
            }
        }

        Ok(())
    }
}

/// A plain array: the values it holds, in order.
#[derive(Clone, Default)]
pub struct Array {
    items: Vec<Value>,
}

impl Array {
    /// An empty array.
    pub fn new() -> Self {
        Self::default()
    }

    /// Appends a value.
    pub fn push(&mut self, value: impl Into<Value>) {
        self.items.push(value.into());
    }
}

impl Deref for Array {
    type Target = [Value];

    fn deref(&self) -> &[Value] {
        &self.items
    }
}

impl From<Vec<Value>> for Array {
    fn from(items: Vec<Value>) -> Self {
        Self { items }
    }
}

impl<T: Into<Value>> FromIterator<T> for Array {
    fn from_iter<I: IntoIterator<Item = T>>(iter: I) -> Self {
        Self {
            items: iter.into_iter().map(Into::into).collect(),
        }
    }
}

impl fmt::Debug for Array {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::Debug::fmt(&Value::Array(Arc::new(self.clone())), f)
    }
}

// Past this many entries an object answers a lookup from an index rather than
// by reading its entries in turn.
const INDEXED: usize = 8;

/// A plain object: its own entries, in the order they were first inserted.
///
/// Every key is a name and nothing else: `__proto__`, `constructor` and
/// `toString` are keys like any other, and an object holds no entry it was
/// not given.
#[derive(Clone, Default)]
pub struct Object {
    entries: Vec<(Arc<str>, Value)>,
    index: Option<HashMap<Arc<str>, usize>>,
}

impl Object {
    /// An empty object.
    pub const fn new() -> Self {
        Self {
            entries: Vec::new(),
            index: None,
        }
    }

    /// Inserts an entry and answers the value it replaced. A key already
    /// present keeps its place in the order.
    pub fn insert(&mut self, key: impl Into<Arc<str>>, value: impl Into<Value>) -> Option<Value> {
        let key = key.into();
        let value = value.into();

        if let Some(at) = self.position(&key) {
            return self
                .entries
                .get_mut(at)
                .map(|entry| std::mem::replace(&mut entry.1, value));
        }

        let at = self.entries.len();

        if let Some(index) = &mut self.index {
            index.insert(Arc::clone(&key), at);
        } else if at >= INDEXED {
            let mut index = HashMap::with_capacity(at.saturating_add(1));

            for (position, (name, _)) in self.entries.iter().enumerate() {
                index.insert(Arc::clone(name), position);
            }

            index.insert(Arc::clone(&key), at);
            self.index = Some(index);
        }

        self.entries.push((key, value));

        None
    }

    /// The value under a key, where the object holds an entry under it.
    pub fn get(&self, key: &str) -> Option<&Value> {
        self.position(key)
            .and_then(|at| self.entries.get(at))
            .map(|(_, value)| value)
    }

    /// Whether the object holds an entry under a key.
    pub fn contains_key(&self, key: &str) -> bool {
        self.position(key).is_some()
    }

    /// How many entries the object holds.
    pub fn len(&self) -> usize {
        self.entries.len()
    }

    /// Whether the object holds no entry.
    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }

    /// The entries, in order.
    pub fn iter(&self) -> impl ExactSizeIterator<Item = (&str, &Value)> {
        self.entries.iter().map(|(key, value)| (&**key, value))
    }

    /// Inserts an entry and answers the object, for building one inline.
    #[must_use]
    pub fn with(mut self, key: impl Into<Arc<str>>, value: impl Into<Value>) -> Self {
        self.insert(key, value);
        self
    }

    fn position(&self, key: &str) -> Option<usize> {
        match &self.index {
            Some(index) => index.get(key).copied(),
            None => self.entries.iter().position(|(name, _)| &**name == key),
        }
    }
}

impl<K: Into<Arc<str>>, V: Into<Value>> FromIterator<(K, V)> for Object {
    fn from_iter<I: IntoIterator<Item = (K, V)>>(iter: I) -> Self {
        let iter = iter.into_iter();
        let (lower, _) = iter.size_hint();
        let mut object = Self {
            entries: Vec::with_capacity(lower),
            index: (lower > INDEXED).then(|| HashMap::with_capacity(lower)),
        };

        for (key, value) in iter {
            object.insert(key, value);
        }

        object
    }
}

impl fmt::Debug for Object {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::Debug::fmt(&Value::Object(Arc::new(self.clone())), f)
    }
}

// A container drops the children it holds alone onto a list of its own, so a
// value nested a million levels deep is taken apart without a call per level.
fn dismantle(first: impl Iterator<Item = Value>) {
    let mut work: Vec<Value> = first.filter(Value::is_container).collect();

    while let Some(value) = work.pop() {
        match value {
            Value::Array(mut array) => {
                if let Some(array) = Arc::get_mut(&mut array) {
                    work.extend(array.items.drain(..).filter(Value::is_container));
                }
            }
            Value::Object(mut object) => {
                if let Some(object) = Arc::get_mut(&mut object) {
                    work.extend(
                        object
                            .entries
                            .drain(..)
                            .map(|(_, value)| value)
                            .filter(Value::is_container),
                    );
                }
            }
            _ => {}
        }
    }
}

impl Drop for Array {
    fn drop(&mut self) {
        if self.items.iter().any(Value::is_container) {
            dismantle(self.items.drain(..));
        }
    }
}

impl Drop for Object {
    fn drop(&mut self) {
        if self.entries.iter().any(|(_, value)| value.is_container()) {
            dismantle(self.entries.drain(..).map(|(_, value)| value));
        }
    }
}

impl From<bool> for Value {
    fn from(value: bool) -> Self {
        Self::Bool(value)
    }
}

impl From<f64> for Value {
    fn from(value: f64) -> Self {
        Self::Number(value)
    }
}

impl From<f32> for Value {
    fn from(value: f32) -> Self {
        Self::Number(f64::from(value))
    }
}

macro_rules! from_integer {
    ($variant:ident, $wide:ty, $($narrow:ty),*) => {
        $(
            impl From<$narrow> for Value {
                fn from(value: $narrow) -> Self {
                    Self::$variant(<$wide>::from(value))
                }
            }
        )*
    };
}

from_integer!(Integer, i64, i8, i16, i32, i64);
from_integer!(Unsigned, u64, u8, u16, u32, u64);

impl From<isize> for Value {
    fn from(value: isize) -> Self {
        // Every supported target's isize fits in 64 bits.
        Self::Integer(value as i64)
    }
}

impl From<usize> for Value {
    fn from(value: usize) -> Self {
        // Every supported target's usize fits in 64 bits.
        Self::Unsigned(value as u64)
    }
}

impl From<&str> for Value {
    fn from(value: &str) -> Self {
        Self::String(value.to_owned())
    }
}

impl From<&String> for Value {
    fn from(value: &String) -> Self {
        Self::String(value.clone())
    }
}

impl From<String> for Value {
    fn from(value: String) -> Self {
        Self::String(value)
    }
}

impl From<Object> for Value {
    fn from(value: Object) -> Self {
        Self::Object(Arc::new(value))
    }
}

impl From<Arc<Object>> for Value {
    fn from(value: Arc<Object>) -> Self {
        Self::Object(value)
    }
}

impl From<Array> for Value {
    fn from(value: Array) -> Self {
        Self::Array(Arc::new(value))
    }
}

impl From<Arc<Array>> for Value {
    fn from(value: Arc<Array>) -> Self {
        Self::Array(value)
    }
}

impl<T: Into<Value>> From<Vec<T>> for Value {
    fn from(value: Vec<T>) -> Self {
        Self::from(value.into_iter().collect::<Array>())
    }
}

impl<T: Into<Value>> From<Option<T>> for Value {
    fn from(value: Option<T>) -> Self {
        value.map_or(Self::Undefined, Into::into)
    }
}
