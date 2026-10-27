use std::borrow::Cow;
use std::sync::LazyLock;

use crate::intl::{Api, Input, Request, Unit};
use crate::number::{date_input, modifier_input, to_number_value};
use crate::props::{Layers, Props};
use crate::syntax::Segment;
use crate::value::Value;

/// The formatting modifiers of section 11.2 and the plural selections of
/// section 11.5, which delegate to the configured [`Intl`](crate::Intl).
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum Formatting {
    Number,
    Date,
    Ago,
    Currency,
    Plural,
    Ordinal,
}

/// What a modifier asks of the facility, or what it settled without asking.
pub(crate) enum Asked {
    Format,
    Select,
    /// The option at this place, whose key is a number the value equals.
    Option(usize),
    /// The value or a property is one the modifier cannot take.
    Failed,
}

/// What a modifier's layers configure it with: the request it hands the
/// facility, whose input each value sets, and the properties it reads
/// itself.
pub(crate) struct Configured<'a> {
    pub(crate) request: Request<'a>,
    // What `currency` multiplies an amount by.
    ratio: f64,
    // The rung `ago` stops at, `None` for `auto`; nothing where `format`
    // names no rung.
    resolution: Option<Option<usize>>,
}

// The properties that decide which digits a number is shown with: `plural`
// reads them from `number`'s layers, so a count and the word agreeing with it
// are decided on the same digits.
const SHOWN_DIGITS: [&str; 9] = [
    "minimumIntegerDigits",
    "minimumFractionDigits",
    "maximumFractionDigits",
    "minimumSignificantDigits",
    "maximumSignificantDigits",
    "roundingIncrement",
    "roundingMode",
    "roundingPriority",
    "trailingZeroDisplay",
];

// The rungs `ago` climbs, each with how many of the rung below it make one.
const LADDER: [(Unit, f64); 7] = [
    (Unit::Second, 1000.0),
    (Unit::Minute, 60.0),
    (Unit::Hour, 60.0),
    (Unit::Day, 24.0),
    (Unit::Week, 7.0),
    (Unit::Month, 13.0 / 3.0),
    (Unit::Year, 12.0),
];

// What a modifier pins, the same for every placeholder.
static CURRENCY: LazyLock<Value> = LazyLock::new(|| Value::from("currency"));
static CARDINAL: LazyLock<Value> = LazyLock::new(|| Value::from("cardinal"));
static ORDINAL: LazyLock<Value> = LazyLock::new(|| Value::from("ordinal"));
static AUTO: LazyLock<Value> = LazyLock::new(|| Value::from("auto"));

// What takes a property out of every layer.
const NOTHING: Cow<'static, Value> = Cow::Owned(Value::Undefined);

// Two fraction digits is what `number` shows where no layer names a maximum:
// a default, not a cap. It widens to reach a larger minimum, as ECMA-402's own
// default does; held at two under a minimum of three it would contradict the
// minimum and fail a number nobody asked to fail.
fn widen(props: Props<'_>) -> Props<'_> {
    let minimum = props
        .get("minimumFractionDigits")
        .map_or(f64::NAN, to_number_value);
    let minimum = if minimum.is_nan() { 0.0 } else { minimum };

    props.or(
        "maximumFractionDigits",
        Cow::Owned(Value::from(minimum.max(2.0))),
    )
}

// A step of the ladder is rounded on its magnitude, half away from zero, and
// given its sign back, so a delta and its negation climb alike.
fn step(value: f64) -> f64 {
    value.abs().round().copysign(value)
}

// Where `ago` stops climbing: at the rung `format` names, singular or
// plural, spelled exactly; `None` for `auto`, which climbs while a rung counts
// at least one.
fn resolution(format: Option<&Value>) -> Option<Option<usize>> {
    let format = match format {
        None => return Some(None),
        Some(Value::String(format)) if &**format == "auto" => return Some(None),
        Some(Value::String(format)) => format,
        Some(_) => return None,
    };

    LADDER
        .iter()
        .position(|(unit, _)| {
            let unit = unit.as_str();

            format
                .strip_prefix(unit)
                .is_some_and(|rest| rest.is_empty() || rest == "s")
        })
        .map(Some)
}

fn climb(milliseconds: f64, resolution: Option<usize>) -> (f64, Unit) {
    let mut value = milliseconds;
    let mut current: Option<usize> = None;

    for (index, (_, multiplier)) in LADDER.iter().enumerate() {
        if current.is_some() && current == resolution {
            break;
        }

        let next = current.is_none_or(|current| current.checked_add(1) == Some(index));

        if !next {
            break;
        }

        let output = step(value / multiplier);

        if current.is_some() && output.abs() < 1.0 && resolution.is_none() {
            break;
        }

        value = output;
        current = Some(index);
    }

    let unit = current
        .and_then(|at| LADDER.get(at))
        .map_or(Unit::Second, |(unit, _)| *unit);

    (value, unit)
}

impl Formatting {
    pub(crate) fn named(name: &str) -> Option<Self> {
        match name {
            "number" => Some(Self::Number),
            "date" => Some(Self::Date),
            "ago" => Some(Self::Ago),
            "currency" => Some(Self::Currency),
            "plural" => Some(Self::Plural),
            "ordinal" => Some(Self::Ordinal),
            _ => None,
        }
    }

    pub(crate) fn is_selection(self) -> bool {
        matches!(self, Self::Plural | Self::Ordinal)
    }

    /// The input a value is, tested before any property is read, or what the
    /// placeholder settles without one.
    pub(crate) fn input(self, value: &str, options: &[Segment<'_>]) -> Result<f64, Asked> {
        let input = match self {
            Self::Date => modifier_input(value).or_else(|| date_input(value)),
            _ => modifier_input(value),
        }
        .ok_or(Asked::Failed)?;

        if self.is_selection() {
            // An ordinal of a fraction names no position.
            if self == Self::Ordinal && input.fract() != 0.0 {
                return Err(Asked::Failed);
            }

            // A key that is a number selects for the value it equals,
            // wherever it is written, before any category is asked for.
            if let Some(at) = options
                .iter()
                .position(|option| modifier_input(&option.key) == Some(input))
            {
                return Err(Asked::Option(at));
            }
        }

        Ok(input)
    }

    /// What the layers configure the modifier with. Nothing is copied out of
    /// them: the request reads each property where it lies.
    pub(crate) fn configure<'l>(self, layers: &Layers<'l>) -> Configured<'l> {
        let mut ratio = 1.0;
        let mut resolved = Some(None);
        let (api, options) = match self {
            Self::Number => (Api::NumberFormat, widen(Props::layered(layers, "number"))),
            Self::Currency => {
                let props = Props::layered(layers, "currency");

                ratio = props.get("ratio").map_or(1.0, to_number_value);

                // The style is what this modifier is, not one of the options
                // it layers, so it is pinned over every layer.
                let props = props
                    .pin("ratio", NOTHING)
                    .pin("style", Cow::Borrowed(&*CURRENCY));

                (Api::NumberFormat, props)
            }
            Self::Date => (Api::DateTimeFormat, Props::layered(layers, "date")),
            Self::Ago => {
                let props = Props::layered(layers, "ago");

                resolved = resolution(props.get("format"));

                let props = props
                    .pin("format", NOTHING)
                    .or("numeric", Cow::Borrowed(&*AUTO));

                (Api::RelativeTimeFormat, props)
            }
            Self::Plural => {
                let props = Props::default()
                    .over(layers, "number", Some(&SHOWN_DIGITS))
                    .over(layers, "plural", None);
                let props = widen(props).pin("type", Cow::Borrowed(&*CARDINAL));

                (Api::PluralRules, props)
            }
            Self::Ordinal => {
                let props = Props::layered(layers, "ordinal").pin("type", Cow::Borrowed(&*ORDINAL));

                (Api::PluralRules, props)
            }
        };

        Configured {
            request: Request::new(api, options, Input::Number(0.0)),
            ratio,
            resolution: resolved,
        }
    }

    /// Sets the request's input for one value, or fails where a property the
    /// modifier reads itself cannot take it.
    pub(crate) fn ask(self, input: f64, configured: &mut Configured<'_>) -> Asked {
        configured.request.input = match self {
            Self::Currency => {
                let input = input * configured.ratio;

                if !input.is_finite() {
                    return Asked::Failed;
                }

                Input::Number(input)
            }
            Self::Ago => {
                let Some(resolution) = configured.resolution else {
                    return Asked::Failed;
                };
                let (count, unit) = climb(input, resolution);

                Input::Relative(count, unit)
            }
            _ => Input::Number(input),
        };

        if self.is_selection() {
            Asked::Select
        } else {
            Asked::Format
        }
    }
}
