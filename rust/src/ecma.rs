//! The options of a request read as ECMA-402 reads them: its abstract
//! operations GetOption, GetNumberOption and SetNumberFormatDigitOptions, and
//! the constructors' reading of what each facility takes. The property names
//! are ECMAScript's vocabulary, so their coercions are part of what they
//! mean: a property a caller writes asks for what it asks for in JavaScript.

use std::borrow::Cow;

use crate::convert::display;
use crate::intl::Refusal;
use crate::number::{iso_date, to_js_string, to_number_value};
use crate::props::Props;
use crate::value::Value;

fn range_error(property: &str) -> Refusal {
    Refusal::new(format!("`{property}` is out of range"))
}

// A property no layer names reads as ECMAScript's `undefined`.
fn get<'o>(options: &'o Props<'_>, property: &str) -> Option<&'o Value> {
    options.get(property).filter(|value| !value.is_undefined())
}

// ECMAScript's ToString over the value model. An array or an object has no
// text a property of the vocabulary could hold.
fn to_string(value: &Value) -> Result<Cow<'_, str>, Refusal> {
    Ok(match value {
        Value::Undefined => Cow::Borrowed("undefined"),
        Value::Null => Cow::Borrowed("null"),
        Value::Bool(flag) => Cow::Borrowed(if *flag { "true" } else { "false" }),
        Value::Number(number) => Cow::Owned(to_js_string(*number)),
        Value::Integer(number) => Cow::Owned(number.to_string()),
        Value::Unsigned(number) => Cow::Owned(number.to_string()),
        Value::String(text) => Cow::Borrowed(&**text),
        Value::Date(milliseconds) => {
            Cow::Owned(iso_date(*milliseconds).unwrap_or_else(|| "Invalid Date".to_owned()))
        }
        Value::Host(host) => {
            Cow::Owned(display(&**host).ok_or_else(|| Refusal::new("a property has no text"))?)
        }
        Value::Array(_) | Value::Object(_) => {
            return Err(Refusal::new("a property holds a structure"));
        }
    })
}

// ECMAScript's ToBoolean over the value model.
fn to_boolean(value: &Value) -> bool {
    match value {
        Value::Undefined | Value::Null => false,
        Value::Bool(flag) => *flag,
        Value::Number(number) => !(*number == 0.0 || number.is_nan()),
        Value::Integer(number) => *number != 0,
        Value::Unsigned(number) => *number != 0,
        Value::String(text) => !text.is_empty(),
        Value::Date(_) | Value::Array(_) | Value::Object(_) | Value::Host(_) => true,
    }
}

/// GetOption for a string property: its text, or none where it is not given.
pub(crate) fn string<'o>(
    options: &'o Props<'_>,
    property: &str,
) -> Result<Option<Cow<'o, str>>, Refusal> {
    get(options, property).map(to_string).transpose()
}

/// GetOption for a string property that takes one of `values`.
pub(crate) fn choice(
    options: &Props<'_>,
    property: &str,
    values: &[&'static str],
    fallback: &'static str,
) -> Result<&'static str, Refusal> {
    match string(options, property)? {
        None => Ok(fallback),
        Some(text) => values
            .iter()
            .find(|value| **value == text)
            .copied()
            .ok_or_else(|| range_error(property)),
    }
}

/// GetOption for a string property that takes one of `values`, or none.
pub(crate) fn optional_choice(
    options: &Props<'_>,
    property: &str,
    values: &[&'static str],
) -> Result<Option<&'static str>, Refusal> {
    if get(options, property).is_none() {
        return Ok(None);
    }

    choice(options, property, values, "").map(Some)
}

/// GetOption for a boolean property.
pub(crate) fn boolean(options: &Props<'_>, property: &str) -> Option<bool> {
    get(options, property).map(to_boolean)
}

// DefaultNumberOption: an integer within the bounds, or a RangeError.
fn default_number(
    value: Option<&Value>,
    property: &str,
    minimum: f64,
    maximum: f64,
) -> Result<Option<i16>, Refusal> {
    let Some(value) = value else {
        return Ok(None);
    };
    let number = to_number_value(value);

    if number.is_nan() || number < minimum || number > maximum {
        return Err(range_error(property));
    }

    // The bounds keep the floor inside every digit count ECMA-402 names.
    #[expect(
        clippy::cast_possible_truncation,
        reason = "the floor lies within the bounds just tested"
    )]
    Ok(Some(number.floor() as i16))
}

/// GetNumberOption.
pub(crate) fn number(
    options: &Props<'_>,
    property: &str,
    minimum: f64,
    maximum: f64,
    fallback: i16,
) -> Result<i16, Refusal> {
    Ok(default_number(get(options, property), property, minimum, maximum)?.unwrap_or(fallback))
}

/// The `type` of a Unicode locale extension keyword's value: one or more
/// segments of three to eight letters and digits.
pub(crate) fn is_type(text: &str) -> bool {
    !text.is_empty()
        && text.split('-').all(|part| {
            (3..=8).contains(&part.len()) && part.bytes().all(|b| b.is_ascii_alphanumeric())
        })
}

/// How a number is rounded, as `roundingMode` names it.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum Mode {
    Ceil,
    Floor,
    Expand,
    Trunc,
    HalfCeil,
    HalfFloor,
    HalfExpand,
    HalfTrunc,
    HalfEven,
}

/// Which of the two roundings a number is shown with.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum Rounding {
    Fraction,
    Significant,
    /// The rounding of the two that keeps more precision.
    More,
    /// The rounding of the two that keeps less precision.
    Less,
}

/// The digits a number is shown with: SetNumberFormatDigitOptions.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) struct Digits {
    pub(crate) minimum_integer: i16,
    pub(crate) fraction: (i16, i16),
    pub(crate) significant: (i16, i16),
    pub(crate) rounding: Rounding,
    pub(crate) increment: u16,
    pub(crate) mode: Mode,
    pub(crate) strip_if_integer: bool,
}

const INCREMENTS: [u16; 15] = [
    1, 2, 5, 10, 20, 25, 50, 100, 200, 250, 500, 1000, 2000, 2500, 5000,
];

const MODES: [(&str, Mode); 9] = [
    ("ceil", Mode::Ceil),
    ("floor", Mode::Floor),
    ("expand", Mode::Expand),
    ("trunc", Mode::Trunc),
    ("halfCeil", Mode::HalfCeil),
    ("halfFloor", Mode::HalfFloor),
    ("halfExpand", Mode::HalfExpand),
    ("halfTrunc", Mode::HalfTrunc),
    ("halfEven", Mode::HalfEven),
];

/// SetNumberFormatDigitOptions, for a notation other than `compact`.
pub(crate) fn digits(
    options: &Props<'_>,
    minimum_fraction_default: i16,
    maximum_fraction_default: i16,
) -> Result<Digits, Refusal> {
    let minimum_integer = number(options, "minimumIntegerDigits", 1.0, 21.0, 1)?;
    let minimum_fraction = get(options, "minimumFractionDigits");
    let maximum_fraction = get(options, "maximumFractionDigits");
    let minimum_significant = get(options, "minimumSignificantDigits");
    let maximum_significant = get(options, "maximumSignificantDigits");
    let increment = number(options, "roundingIncrement", 1.0, 5000.0, 1)?;
    let increment = u16::try_from(increment).map_err(|_| range_error("roundingIncrement"))?;

    if !INCREMENTS.contains(&increment) {
        return Err(range_error("roundingIncrement"));
    }

    let names = MODES.map(|(name, _)| name);
    let mode = choice(options, "roundingMode", &names, "halfExpand")?;
    let mode = MODES
        .iter()
        .find(|(name, _)| *name == mode)
        .map_or(Mode::HalfExpand, |(_, mode)| *mode);
    let priority = choice(
        options,
        "roundingPriority",
        &["auto", "morePrecision", "lessPrecision"],
        "auto",
    )?;
    let strip_if_integer = choice(
        options,
        "trailingZeroDisplay",
        &["auto", "stripIfInteger"],
        "auto",
    )? == "stripIfInteger";
    let maximum_fraction_default = if increment == 1 {
        maximum_fraction_default
    } else {
        minimum_fraction_default
    };
    let has_significant = minimum_significant.is_some() || maximum_significant.is_some();
    let has_fraction = minimum_fraction.is_some() || maximum_fraction.is_some();
    let (need_significant, need_fraction) = if priority == "auto" {
        (has_significant, !has_significant)
    } else {
        (true, true)
    };
    let mut significant = (1, 21);
    let mut fraction = (minimum_fraction_default, maximum_fraction_default);

    if need_significant && has_significant {
        let minimum = default_number(minimum_significant, "minimumSignificantDigits", 1.0, 21.0)?
            .unwrap_or(1);
        let maximum = default_number(
            maximum_significant,
            "maximumSignificantDigits",
            f64::from(minimum),
            21.0,
        )?
        .unwrap_or(21);

        significant = (minimum, maximum);
    }

    if need_fraction && has_fraction {
        let minimum = default_number(minimum_fraction, "minimumFractionDigits", 0.0, 100.0)?;
        let maximum = default_number(maximum_fraction, "maximumFractionDigits", 0.0, 100.0)?;

        fraction = match (minimum, maximum) {
            (None, Some(maximum)) => (minimum_fraction_default.min(maximum), maximum),
            (Some(minimum), None) => (minimum, maximum_fraction_default.max(minimum)),
            (Some(minimum), Some(maximum)) if minimum > maximum => {
                return Err(range_error("minimumFractionDigits"));
            }
            (Some(minimum), Some(maximum)) => (minimum, maximum),
            (None, None) => fraction,
        };
    }

    let rounding = match priority {
        "morePrecision" => Rounding::More,
        "lessPrecision" => Rounding::Less,
        _ if need_significant => Rounding::Significant,
        _ => Rounding::Fraction,
    };

    if increment != 1 {
        if rounding != Rounding::Fraction {
            return Err(Refusal::new(
                "`roundingIncrement` takes fraction digits alone",
            ));
        }

        if fraction.0 != fraction.1 {
            return Err(range_error("roundingIncrement"));
        }
    }

    Ok(Digits {
        minimum_integer,
        fraction,
        significant,
        rounding,
        increment,
        mode,
        strip_if_integer,
    })
}

/// What a number is formatted as, by `style`.
#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) enum Style {
    Decimal,
    Percent,
    Currency {
        code: String,
        display: &'static str,
        accounting: bool,
    },
    /// A unit, which is formatted as a decimal: the unit is not expressed.
    Unit,
}

/// When grouping separators are shown, by `useGrouping`.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum Grouping {
    Auto,
    Always,
    Min2,
    Never,
}

/// When a sign is shown, by `signDisplay`.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum Sign {
    Auto,
    Never,
    Always,
    ExceptZero,
    Negative,
}

/// The options of a `NumberFormat` request.
#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct NumberOptions {
    pub(crate) numbering_system: Option<String>,
    pub(crate) style: Style,
    pub(crate) digits: Digits,
    pub(crate) grouping: Grouping,
    pub(crate) sign: Sign,
}

const LOCALE_MATCHERS: [&str; 2] = ["lookup", "best fit"];

fn numbering_system(options: &Props<'_>) -> Result<Option<String>, Refusal> {
    match string(options, "numberingSystem")? {
        Some(system) if !is_type(&system) => Err(range_error("numberingSystem")),
        system => Ok(system.map(Cow::into_owned)),
    }
}

/// InitializeNumberFormat. A currency is formatted with its own digits
/// whatever the options say, so its default digits are read as two, which is
/// what bounds a stated minimum or maximum for most currencies.
pub(crate) fn number_options(options: &Props<'_>) -> Result<NumberOptions, Refusal> {
    choice(options, "localeMatcher", &LOCALE_MATCHERS, "best fit")?;

    let numbering_system = numbering_system(options)?;
    let style = choice(
        options,
        "style",
        &["decimal", "percent", "currency", "unit"],
        "decimal",
    )?;
    let currency = string(options, "currency")?;
    let currency = match currency {
        Some(code) if code.len() == 3 && code.bytes().all(|b| b.is_ascii_alphabetic()) => {
            Some(code.to_ascii_uppercase())
        }
        Some(_) => return Err(range_error("currency")),
        None => None,
    };
    let display = choice(
        options,
        "currencyDisplay",
        &["code", "symbol", "narrowSymbol", "name"],
        "symbol",
    )?;
    let accounting = choice(
        options,
        "currencySign",
        &["standard", "accounting"],
        "standard",
    )? == "accounting";
    let unit = string(options, "unit")?;

    if unit.as_deref().is_some_and(|unit| !is_unit(unit)) {
        return Err(range_error("unit"));
    }

    choice(
        options,
        "unitDisplay",
        &["short", "narrow", "long"],
        "short",
    )?;

    let style = match style {
        "currency" => Style::Currency {
            code: currency.ok_or_else(|| Refusal::new("the currency style names no `currency`"))?,
            display,
            accounting,
        },
        "unit" if unit.is_none() => {
            return Err(Refusal::new("the unit style names no `unit`"));
        }
        "unit" => Style::Unit,
        "percent" => Style::Percent,
        _ => Style::Decimal,
    };
    let (minimum_default, maximum_default) = match style {
        Style::Currency { .. } => (2, 2),
        Style::Percent => (0, 0),
        Style::Decimal | Style::Unit => (0, 3),
    };

    choice(
        options,
        "notation",
        &["standard", "scientific", "engineering", "compact"],
        "standard",
    )?;

    let digits = digits(options, minimum_default, maximum_default)?;

    choice(options, "compactDisplay", &["short", "long"], "short")?;

    let grouping = match get(options, "useGrouping") {
        None => Grouping::Auto,
        Some(Value::Bool(true)) => Grouping::Always,
        Some(value) if !to_boolean(value) => Grouping::Never,
        Some(value) => match &*to_string(value)? {
            "min2" => Grouping::Min2,
            "always" => Grouping::Always,
            "auto" | "true" | "false" => Grouping::Auto,
            _ => return Err(range_error("useGrouping")),
        },
    };
    let sign = match choice(
        options,
        "signDisplay",
        &["auto", "never", "always", "exceptZero", "negative"],
        "auto",
    )? {
        "never" => Sign::Never,
        "always" => Sign::Always,
        "exceptZero" => Sign::ExceptZero,
        "negative" => Sign::Negative,
        _ => Sign::Auto,
    };

    Ok(NumberOptions {
        numbering_system,
        style,
        digits,
        grouping,
        sign,
    })
}

// IsWellFormedUnitIdentifier: a sanctioned unit, or two joined by `-per-`.
fn is_unit(unit: &str) -> bool {
    const SANCTIONED: [&str; 45] = [
        "acre",
        "bit",
        "byte",
        "celsius",
        "centimeter",
        "day",
        "degree",
        "fahrenheit",
        "fluid-ounce",
        "foot",
        "gallon",
        "gigabit",
        "gigabyte",
        "gram",
        "hectare",
        "hour",
        "inch",
        "kilobit",
        "kilobyte",
        "kilogram",
        "kilometer",
        "liter",
        "megabit",
        "megabyte",
        "meter",
        "microsecond",
        "mile",
        "mile-scandinavian",
        "milliliter",
        "millimeter",
        "millisecond",
        "minute",
        "month",
        "nanosecond",
        "ounce",
        "percent",
        "petabyte",
        "pound",
        "second",
        "stone",
        "terabit",
        "terabyte",
        "week",
        "yard",
        "year",
    ];

    if SANCTIONED.contains(&unit) {
        return true;
    }

    unit.split_once("-per-")
        .is_some_and(|(numerator, denominator)| {
            SANCTIONED.contains(&numerator) && SANCTIONED.contains(&denominator)
        })
}

/// The zone a `DateTimeFormat` request formats in.
#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) enum Zone {
    /// A fixed offset from UTC, in seconds.
    Offset(i32),
    /// A zone of the IANA database, by a name not yet looked up.
    Named(String),
}

/// The options of a `DateTimeFormat` request.
#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct DateOptions {
    pub(crate) calendar: Option<String>,
    pub(crate) numbering_system: Option<String>,
    pub(crate) zone: Zone,
    pub(crate) date_style: Option<&'static str>,
    pub(crate) time_style: Option<&'static str>,
}

const STYLES: [&str; 4] = ["full", "long", "medium", "short"];

// The components a request may name in place of a style, with the values
// each takes. They are read and checked, and not expressed.
const COMPONENTS: [(&str, &[&str]); 10] = [
    ("weekday", &["narrow", "short", "long"]),
    ("era", &["narrow", "short", "long"]),
    ("year", &["2-digit", "numeric"]),
    ("month", &["2-digit", "numeric", "narrow", "short", "long"]),
    ("day", &["2-digit", "numeric"]),
    ("dayPeriod", &["narrow", "short", "long"]),
    ("hour", &["2-digit", "numeric"]),
    ("minute", &["2-digit", "numeric"]),
    ("second", &["2-digit", "numeric"]),
    (
        "timeZoneName",
        &[
            "short",
            "long",
            "shortOffset",
            "longOffset",
            "shortGeneric",
            "longGeneric",
        ],
    ),
];

/// A UTC offset as ECMA-402 accepts one for `timeZone`, `+` or `-` and `HH`,
/// `HHMM` or `HH:MM`, in seconds.
pub(crate) fn offset(text: &str) -> Option<i32> {
    let (sign, rest) = match text.as_bytes().first()? {
        b'+' => (1, text.get(1..)?),
        b'-' => (-1, text.get(1..)?),
        _ => return None,
    };
    let two = |digits: &str| -> Option<i32> {
        (digits.len() == 2 && digits.bytes().all(|b| b.is_ascii_digit()))
            .then(|| digits.parse().ok())
            .flatten()
    };
    let (hours, minutes) = match rest.len() {
        2 => (two(rest)?, 0),
        4 => (two(rest.get(..2)?)?, two(rest.get(2..)?)?),
        5 if rest.get(2..3) == Some(":") => (two(rest.get(..2)?)?, two(rest.get(3..)?)?),
        _ => return None,
    };

    if hours > 23 || minutes > 59 {
        return None;
    }

    hours
        .checked_mul(3600)?
        .checked_add(minutes.checked_mul(60)?)?
        .checked_mul(sign)
}

/// CreateDateTimeFormat. No `timeZone` is UTC: there is no host zone to
/// read, and reading the system's would make a message's text depend on
/// the machine it is resolved on.
pub(crate) fn date_options(options: &Props<'_>) -> Result<DateOptions, Refusal> {
    choice(options, "localeMatcher", &LOCALE_MATCHERS, "best fit")?;

    let calendar = match string(options, "calendar")? {
        Some(calendar) if !is_type(&calendar) => return Err(range_error("calendar")),
        calendar => calendar.map(Cow::into_owned),
    };
    let numbering_system = numbering_system(options)?;

    boolean(options, "hour12");
    optional_choice(options, "hourCycle", &["h11", "h12", "h23", "h24"])?;

    let zone = match string(options, "timeZone")? {
        None => Zone::Named("UTC".to_owned()),
        Some(name) => match offset(&name) {
            Some(seconds) => Zone::Offset(seconds),
            // The names ECMA-402 canonicalizes to UTC.
            None if ["UTC", "Etc/UTC", "Etc/GMT", "GMT"]
                .iter()
                .any(|utc| name.eq_ignore_ascii_case(utc)) =>
            {
                Zone::Named("UTC".to_owned())
            }
            None => Zone::Named(name.into_owned()),
        },
    };
    let mut components = false;

    for (index, (property, values)) in COMPONENTS.iter().enumerate() {
        // `fractionalSecondDigits` is read between `second` and
        // `timeZoneName`.
        if index == 9 {
            components |= default_number(
                get(options, "fractionalSecondDigits"),
                "fractionalSecondDigits",
                1.0,
                3.0,
            )?
            .is_some();
        }

        components |= optional_choice(options, property, values)?.is_some();
    }

    choice(options, "formatMatcher", &["basic", "best fit"], "best fit")?;

    let date_style = optional_choice(options, "dateStyle", &STYLES)?;
    let time_style = optional_choice(options, "timeStyle", &STYLES)?;

    if (date_style.is_some() || time_style.is_some()) && components {
        return Err(Refusal::new("a style and a component are both named"));
    }

    Ok(DateOptions {
        calendar,
        numbering_system,
        zone,
        date_style,
        time_style,
    })
}

/// The options of a `RelativeTimeFormat` request.
#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct RelativeOptions {
    pub(crate) numbering_system: Option<String>,
    pub(crate) style: &'static str,
    pub(crate) always: bool,
}

/// InitializeRelativeTimeFormat.
pub(crate) fn relative_options(options: &Props<'_>) -> Result<RelativeOptions, Refusal> {
    choice(options, "localeMatcher", &LOCALE_MATCHERS, "best fit")?;

    let numbering_system = numbering_system(options)?;
    let style = choice(options, "style", &["long", "short", "narrow"], "long")?;
    let numeric = choice(options, "numeric", &["always", "auto"], "always")?;

    Ok(RelativeOptions {
        numbering_system,
        style,
        always: numeric == "always",
    })
}

/// The options of a `PluralRules` request.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) struct PluralOptions {
    pub(crate) ordinal: bool,
    pub(crate) digits: Digits,
}

/// InitializePluralRules.
pub(crate) fn plural_options(options: &Props<'_>) -> Result<PluralOptions, Refusal> {
    choice(options, "localeMatcher", &LOCALE_MATCHERS, "best fit")?;

    let ordinal = choice(options, "type", &["cardinal", "ordinal"], "cardinal")? == "ordinal";
    let digits = digits(options, 0, 3)?;

    Ok(PluralOptions { ordinal, digits })
}

#[cfg(test)]
mod tests {
    #![allow(clippy::unwrap_used)]

    use super::{Digits, Grouping, NumberOptions, Rounding, offset};
    use crate::intl::Refusal;
    use crate::props::Props;
    use crate::value::{Object, Value};

    fn digits(options: &Object, minimum: i16, maximum: i16) -> Result<Digits, Refusal> {
        super::digits(&Props::from(options), minimum, maximum)
    }

    fn number_options(options: &Object) -> Result<NumberOptions, Refusal> {
        super::number_options(&Props::from(options))
    }

    #[test]
    fn a_maximum_under_a_minimum_is_a_range_error() {
        let options = Object::new()
            .with("minimumFractionDigits", 3)
            .with("maximumFractionDigits", 2);

        assert!(digits(&options, 0, 3).is_err());
        assert!(digits(&Object::new().with("minimumFractionDigits", 3), 0, 3).is_ok());
    }

    #[test]
    fn null_is_zero_digits_and_a_fraction_is_floored() {
        let options = Object::new()
            .with("maximumFractionDigits", Value::Null)
            .with("minimumIntegerDigits", 2.9);
        let read = digits(&options, 0, 3).unwrap();

        assert_eq!(read.fraction, (0, 0));
        assert_eq!(read.minimum_integer, 2);
    }

    #[test]
    fn significant_digits_win_unless_a_priority_is_named() {
        let options = Object::new()
            .with("maximumSignificantDigits", 2)
            .with("maximumFractionDigits", 1);

        assert_eq!(
            digits(&options, 0, 3).unwrap().rounding,
            Rounding::Significant
        );

        let options = options.with("roundingPriority", "lessPrecision");
        let read = digits(&options, 0, 3).unwrap();

        assert_eq!(read.rounding, Rounding::Less);
        assert_eq!((read.fraction, read.significant), ((0, 1), (1, 2)));
    }

    #[test]
    fn an_increment_needs_equal_fraction_digits() {
        let options = Object::new().with("roundingIncrement", 25);

        assert_eq!(digits(&options, 0, 3).unwrap().fraction, (0, 0));
        assert!(digits(&options.clone().with("maximumFractionDigits", 2), 0, 3).is_err());
        assert!(
            digits(
                &options
                    .with("maximumFractionDigits", 2)
                    .with("minimumFractionDigits", 2),
                0,
                3
            )
            .is_ok()
        );
        assert!(digits(&Object::new().with("roundingIncrement", 3), 0, 3).is_err());
    }

    #[test]
    fn grouping_reads_booleans_and_strings() {
        let grouping = |value: Value| {
            number_options(&Object::new().with("useGrouping", value)).map(|read| read.grouping)
        };

        assert_eq!(grouping(Value::Bool(true)), Ok(Grouping::Always));
        assert_eq!(grouping(Value::Bool(false)), Ok(Grouping::Never));
        assert_eq!(grouping(Value::from("false")), Ok(Grouping::Auto));
        assert_eq!(grouping(Value::from("min2")), Ok(Grouping::Min2));
        assert_eq!(grouping(Value::from("")), Ok(Grouping::Never));
        assert!(grouping(Value::from(1)).is_err());
    }

    #[test]
    fn a_currency_is_required_and_well_formed() {
        let currency = Object::new().with("style", "currency");

        assert!(number_options(&currency).is_err());
        assert!(number_options(&currency.clone().with("currency", "US")).is_err());
        assert!(number_options(&currency.with("currency", "usd")).is_ok());
    }

    #[test]
    fn offsets_are_read_without_seconds() {
        assert_eq!(offset("+01:00"), Some(3600));
        assert_eq!(offset("-0530"), Some(-19_800));
        assert_eq!(offset("+01"), Some(3600));
        assert_eq!(offset("+01:00:00"), None);
        assert_eq!(offset("+24:00"), None);
        assert_eq!(offset("01:00"), None);
    }
}
