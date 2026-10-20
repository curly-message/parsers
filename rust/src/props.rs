use std::borrow::Cow;
use std::fmt;

use crate::value::{Object, Value};

/// The layers of section 11.2, bottom first: the parser's modifier defaults,
/// the resolution's props and the wrapper's own.
pub(crate) type Layers<'a> = [Option<&'a Object>; 3];

// The most layers one set of props reads: a modifier's three, and the three
// of another modifier it reads properties from.
const DEPTH: usize = 6;

/// The props a modifier is configured with (section 11.2): the properties
/// each layer holds under the modifier's name, composed per property, so a
/// layer overrides only what it names and [`Value::Undefined`] names nothing.
/// The options of a formatting [`Request`](crate::Request) also hold what the
/// modifier sets itself: a property it pins over every layer (`currency`'s
/// `style`, the `type` of `plural` and `ordinal`), or one it sets only where
/// no layer names it (`number`'s widened `maximumFractionDigits`).
///
/// The layers are read where they lie, never copied: what a placeholder
/// costs does not grow with what its props hold. [`Props::to_object`] makes
/// an object of one's own.
///
/// ```
/// use curly_message_parser::{Object, Props, Value};
///
/// let options = Object::new().with("style", "percent").with("unit", None::<&str>);
/// let props = Props::from(&options);
///
/// assert_eq!(props.get("style").and_then(Value::as_str), Some("percent"));
/// assert!(props.get("unit").is_none());
/// assert_eq!(props.len(), 1);
/// ```
#[derive(Clone, Default)]
pub struct Props<'a> {
    // Bottom first: each overrides what those beneath it name.
    layers: [Option<Layer<'a>>; DEPTH],
    // What the modifier sets itself, over every layer or where none names it.
    set: [Option<Set<'a>>; 2],
}

#[derive(Clone, Copy)]
struct Layer<'a> {
    object: &'a Object,
    // The only properties read from it, where it lends a few to another
    // modifier.
    only: Option<&'static [&'static str]>,
}

#[derive(Clone)]
struct Set<'a> {
    key: &'static str,
    value: Cow<'a, Value>,
    // Over every layer, or only where no layer names the property.
    pinned: bool,
}

impl<'a> Layer<'a> {
    fn get(&self, key: &str) -> Option<&'a Value> {
        if self.only.is_some_and(|only| !only.contains(&key)) {
            return None;
        }

        self.object.get(key).filter(|value| !value.is_undefined())
    }
}

impl<'a> Props<'a> {
    /// The props the layers hold under a modifier's name. A layer's entry
    /// that is no object names nothing.
    pub(crate) fn layered(layers: &Layers<'a>, name: &str) -> Self {
        Self::default().over(layers, name, None)
    }

    /// These props with the properties the layers hold under `name` over
    /// them, only those of `only` where it is given.
    pub(crate) fn over(
        mut self,
        layers: &Layers<'a>,
        name: &str,
        only: Option<&'static [&'static str]>,
    ) -> Self {
        let objects = layers
            .iter()
            .flatten()
            .filter_map(|layer| layer.get(name).and_then(Value::as_object));
        let free = self.layers.iter_mut().filter(|slot| slot.is_none());

        for (slot, object) in free.zip(objects) {
            *slot = Some(Layer { object, only });
        }

        self
    }

    /// These props with a property set over every layer; `Undefined` takes
    /// it out.
    pub(crate) fn pin(self, key: &'static str, value: Cow<'a, Value>) -> Self {
        self.set(Set {
            key,
            value,
            pinned: true,
        })
    }

    /// These props with a property set where no layer names it.
    pub(crate) fn or(self, key: &'static str, value: Cow<'a, Value>) -> Self {
        self.set(Set {
            key,
            value,
            pinned: false,
        })
    }

    fn set(mut self, set: Set<'a>) -> Self {
        if let Some(slot) = self.set.iter_mut().find(|slot| slot.is_none()) {
            *slot = Some(set);
        }

        self
    }

    fn set_value(&self, key: &str, pinned: bool) -> Option<&Value> {
        self.set
            .iter()
            .flatten()
            .find(|set| set.pinned == pinned && set.key == key)
            .map(|set| &*set.value)
    }

    fn layer_value(&self, key: &str) -> Option<&'a Value> {
        self.layers
            .iter()
            .rev()
            .flatten()
            .find_map(|layer| layer.get(key))
    }

    /// The property's value: what the modifier pins, else what the topmost
    /// layer naming it gives, else what the modifier sets where no layer
    /// names it. `Undefined` names nothing.
    pub fn get(&self, key: &str) -> Option<&Value> {
        let value = match self.set_value(key, true) {
            Some(pinned) => Some(pinned),
            None => self.layer_value(key).or_else(|| self.set_value(key, false)),
        };

        value.filter(|value| !value.is_undefined())
    }

    /// Whether [`get`](Self::get) finds the property.
    pub fn contains_key(&self, key: &str) -> bool {
        self.get(key).is_some()
    }

    /// Each property [`get`](Self::get) finds, once, with the value it gives:
    /// first those the layers name, where the lowest layer naming one names
    /// it, bottom layer first and each in its own order (a layer lent for a
    /// few of its properties, as `number`'s digit options are to `plural`, in
    /// the order of those), then those only the modifier sets.
    pub fn iter(&self) -> impl Iterator<Item = (&str, &Value)> {
        Iter {
            props: self,
            layer: 0,
            at: 0,
            set: 0,
        }
    }

    // Whether a layer beneath the one at `at` names the property, so it was
    // met there first.
    fn named_beneath(&self, at: usize, key: &str) -> bool {
        self.layers
            .iter()
            .take(at)
            .flatten()
            .any(|layer| layer.get(key).is_some())
    }

    /// How many properties [`iter`](Self::iter) yields.
    pub fn len(&self) -> usize {
        self.iter().count()
    }

    /// Whether [`iter`](Self::iter) yields no property.
    pub fn is_empty(&self) -> bool {
        self.iter().next().is_none()
    }

    /// The properties as an object of one's own, in their order.
    pub fn to_object(&self) -> Object {
        self.iter()
            .map(|(key, value)| (key, value.clone()))
            .collect()
    }
}

// Where an iteration of props stands: at an entry of a layer, then at what
// the modifier sets itself.
struct Iter<'p, 'a> {
    props: &'p Props<'a>,
    layer: usize,
    at: usize,
    set: usize,
}

impl<'p> Iterator for Iter<'p, '_> {
    type Item = (&'p str, &'p Value);

    fn next(&mut self) -> Option<Self::Item> {
        let props = self.props;

        while let Some(slot) = props.layers.get(self.layer) {
            let entry = slot.as_ref().and_then(|layer| match layer.only {
                Some(only) => only.get(self.at).map(|key| (*key, layer.object.get(key))),
                None => layer
                    .object
                    .entries()
                    .get(self.at)
                    .map(|(key, value)| (&**key, Some(value))),
            });
            let Some((key, value)) = entry else {
                self.layer = self.layer.saturating_add(1);
                self.at = 0;
                continue;
            };

            self.at = self.at.saturating_add(1);

            if value.is_some_and(|value| !value.is_undefined())
                && !props.named_beneath(self.layer, key)
                && let Some(value) = props.get(key)
            {
                return Some((key, value));
            }
        }

        while let Some(slot) = props.set.get(self.set) {
            self.set = self.set.saturating_add(1);

            if let Some(set) = slot
                && props.layer_value(set.key).is_none()
                && let Some(value) = props.get(set.key)
            {
                return Some((set.key, value));
            }
        }

        None
    }
}

impl<'a> From<&'a Object> for Props<'a> {
    fn from(object: &'a Object) -> Self {
        let mut props = Self::default();

        if let Some(slot) = props.layers.first_mut() {
            *slot = Some(Layer { object, only: None });
        }

        props
    }
}

impl fmt::Debug for Props<'_> {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.debug_map().entries(self.iter()).finish()
    }
}
