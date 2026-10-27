use std::cmp::Ordering;

use crate::number::to_number;
use crate::syntax::Segment;

/// The comparisons of section 11.1.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum Builtin {
    Eq,
    Ne,
    Lt,
    Gt,
    Lte,
    Gte,
}

/// What a comparison selected: one of the options, by its place in source
/// order, or the fallback chain.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum Selected {
    Option(usize),
    Default,
}

impl Builtin {
    pub(crate) fn named(name: &str) -> Option<Self> {
        match name {
            "eq" => Some(Self::Eq),
            "ne" => Some(Self::Ne),
            "lt" => Some(Self::Lt),
            "gt" => Some(Self::Gt),
            "lte" => Some(Self::Lte),
            "gte" => Some(Self::Gte),
            _ => None,
        }
    }

    /// Whether the modifier selects among the options a placeholder declares,
    /// so that one declaring none was asked to select from nothing.
    pub(crate) fn is_selection(self) -> bool {
        matches!(
            self,
            Self::Eq | Self::Ne | Self::Lt | Self::Gt | Self::Lte | Self::Gte
        )
    }

    // A comparison reads keys alone. The option it selects is message text,
    // which the walk renders after it, so no comparison reaches the walk
    // again.
    pub(crate) fn select(self, value: &str, options: &[Segment<'_>]) -> Selected {
        let found = match self {
            Self::Eq => equal(value, options),
            Self::Ne => unequal(value, options),
            Self::Lt => ordered(value, options, Ordering::Less),
            Self::Gt => ordered(value, options, Ordering::Greater),
            Self::Lte => equal(value, options).or_else(|| ordered(value, options, Ordering::Less)),
            Self::Gte => {
                equal(value, options).or_else(|| ordered(value, options, Ordering::Greater))
            }
        };

        found.map_or(Selected::Default, Selected::Option)
    }
}

// The value is lowercased once, ahead of the options: what that costs grows
// with the value, and a comparison's cost adds it to its options rather than
// multiplying the two.
fn equal(value: &str, options: &[Segment<'_>]) -> Option<usize> {
    let needle = value.to_lowercase();

    options
        .iter()
        .position(|option| option.key.to_lowercase() == needle)
}

fn unequal(value: &str, options: &[Segment<'_>]) -> Option<usize> {
    let needle = value.to_lowercase();

    options
        .iter()
        .position(|option| option.key.to_lowercase() != needle)
}

// A numeric comparison reads only the options it can order: a key that is no
// number can never be selected by one. The order is a list of places sorted
// stably, so ties keep source order and the options themselves stay as
// written. `Less` asks for the first key greater than the value, scanning
// upward; `Greater` for the first key less than it, scanning downward.
fn ordered(value: &str, options: &[Segment<'_>], direction: Ordering) -> Option<usize> {
    let input = to_number(value);
    let mut keys: Vec<(usize, f64)> = options
        .iter()
        .enumerate()
        .map(|(at, option)| (at, to_number(&option.key)))
        .filter(|(_, key)| !key.is_nan())
        .collect();

    keys.sort_by(|(_, a), (_, b)| {
        let order = a.partial_cmp(b).unwrap_or(Ordering::Equal);

        if direction == Ordering::Less {
            order
        } else {
            order.reverse()
        }
    });

    keys.into_iter()
        .find(|(_, key)| input.partial_cmp(key) == Some(direction))
        .map(|(at, _)| at)
}
