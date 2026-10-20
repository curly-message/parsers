use std::fmt;

use crate::value::{Object, Value};

/// The layers of section 11.2, bottom first: the parser's modifier defaults,
/// the resolution's props and the wrapper's own.
pub(crate) type Layers<'a> = [Option<&'a Object>; 3];

/// The props a modifier is configured with (section 11.2): the properties
/// each layer holds under the modifier's name, composed per property, so a
/// layer overrides only what it names and [`Value::Undefined`] names nothing.
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
    layers: Layers<'a>,
}

fn named<'a>(layer: &'a Object, key: &str) -> Option<&'a Value> {
    layer.get(key).filter(|value| !value.is_undefined())
}

impl<'a> Props<'a> {
    /// The props the layers hold under a modifier's name. A layer's entry
    /// that is no object names nothing.
    pub(crate) fn layered(layers: &Layers<'a>, name: &str) -> Self {
        let mut props = Self::default();
        let objects = layers
            .iter()
            .flatten()
            .filter_map(|layer| layer.get(name).and_then(Value::as_object));

        for (slot, object) in props.layers.iter_mut().zip(objects) {
            *slot = Some(object);
        }

        props
    }

    /// The property's value, from the topmost layer that names it.
    pub fn get(&self, key: &str) -> Option<&Value> {
        self.layers
            .iter()
            .rev()
            .flatten()
            .find_map(|layer| named(layer, key))
    }

    /// Whether any layer names the property.
    pub fn contains_key(&self, key: &str) -> bool {
        self.get(key).is_some()
    }

    /// The properties, each where a layer first names it, bottom first and
    /// in the layer's order, with the value the topmost one gives it.
    pub fn iter(&self) -> impl Iterator<Item = (&str, &Value)> {
        Iter {
            props: self,
            layer: 0,
            at: 0,
        }
    }

    // Whether a layer beneath the one at `at` names the property, so it was
    // met there first.
    fn named_beneath(&self, at: usize, key: &str) -> bool {
        self.layers
            .iter()
            .take(at)
            .flatten()
            .any(|layer| named(layer, key).is_some())
    }

    /// How many properties the layers name.
    pub fn len(&self) -> usize {
        self.iter().count()
    }

    /// Whether the layers name no property.
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

// Where an iteration of props stands: at an entry of a layer.
struct Iter<'p, 'a> {
    props: &'p Props<'a>,
    layer: usize,
    at: usize,
}

impl<'p> Iterator for Iter<'p, '_> {
    type Item = (&'p str, &'p Value);

    fn next(&mut self) -> Option<Self::Item> {
        let props = self.props;

        while let Some(slot) = props.layers.get(self.layer) {
            let entry = slot
                .and_then(|layer| layer.entries().get(self.at))
                .map(|(key, value)| (&**key, value));
            let Some((key, value)) = entry else {
                self.layer = self.layer.saturating_add(1);
                self.at = 0;
                continue;
            };

            self.at = self.at.saturating_add(1);

            if !value.is_undefined()
                && !props.named_beneath(self.layer, key)
                && let Some(value) = props.get(key)
            {
                return Some((key, value));
            }
        }

        None
    }
}

impl<'a> From<&'a Object> for Props<'a> {
    fn from(object: &'a Object) -> Self {
        Self {
            layers: [Some(object), None, None],
        }
    }
}

impl fmt::Debug for Props<'_> {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.debug_map().entries(self.iter()).finish()
    }
}
