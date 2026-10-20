//! [`IcuIntl`]: the requests of section 11.2 performed by ICU4X, with the
//! IANA time zone database bundled by jiff.

use fixed_decimal::{
    Decimal, RoundingIncrement, Sign, SignDisplay, SignedRoundingMode, UnsignedRoundingMode,
};
use icu_datetime::DateTimeFormatter;
use icu_datetime::fieldsets::enums::{
    CompositeDateTimeFieldSet, DateAndTimeFieldSet, DateFieldSet, TimeFieldSet,
};
use icu_datetime::fieldsets::{T, YMD, YMDE, YMDET, YMDT, zone};
use icu_datetime::options::{Length, TimePrecision, YearStyle};
use icu_decimal::DecimalFormatter;
use icu_decimal::options::GroupingStrategy;
use icu_experimental::dimension::currency::CurrencyType;
use icu_experimental::dimension::currency::formatter::CurrencyFormatter;
use icu_experimental::dimension::currency::options::CurrencyUsage;
use icu_experimental::dimension::percent::formatter::PercentFormatter;
use icu_experimental::dimension::percent::options::Display;
use icu_experimental::relativetime::RelativeTimeFormatter;
use icu_experimental::relativetime::options::{Numeric, RelativeTimeFormatterOptions};
use icu_locale_core::Locale;
use icu_locale_core::extensions::unicode::{Value as Keyword, key};
use icu_plurals::{PluralCategory, PluralRules};
use icu_time::zone::iana::IanaParser;
use icu_time::zone::{TimeZone, UtcOffset};
use icu_time::{DateTime, ZonedDateTime};

use crate::ecma::{
    self, DateOptions, Digits, Grouping, Mode, NumberOptions, RelativeOptions, Rounding, Style,
    Zone,
};
use crate::intl::{Api, Category, Input, Intl, Refusal, Request, Unexpressible, Unit};

/// The facility of sections 11.2 and 11.5 over ICU4X and its compiled locale
/// data, available with the `intl` feature.
///
/// The options are read as ECMA-402 reads them, so a property a caller writes
/// asks for what it asks for in JavaScript, and a request ECMA-402 raises for
/// is refused. What ICU4X cannot express is listed in [`UNEXPRESSIBLE`]: a
/// request naming it formats with what is expressed.
///
/// ```
/// use curly_message_parser::{Context, IcuIntl, Object, Parser};
///
/// let parser = Parser::builder().intl(IcuIntl::new()).build();
/// let payload = Object::new().with("n", 1234.5678).with("count", 21);
/// let context = Context::new().payload(&payload).locale("en");
///
/// assert_eq!(parser.resolve("{{n:number}}", &context), "1,234.57");
/// assert_eq!(
///     parser.resolve("{{count:plural; one:# file; other:# files}}", &context.locale("ru")),
///     "# file"
/// );
/// ```
#[derive(Clone, Copy, Debug, Default)]
pub struct IcuIntl {
    _private: (),
}

impl IcuIntl {
    /// The facility, over the locale data compiled into the binary.
    pub fn new() -> Self {
        Self::default()
    }
}

impl Intl for IcuIntl {
    // The options are read before a formatter's frame is on the stack: reading
    // one runs a host value's `Display`, which may resolve again, and each
    // such resolution holds the stack beneath it (section 13).
    fn format(&self, locale: &str, request: &Request<'_>) -> Result<String, Refusal> {
        let options = &request.options;

        match (request.api, request.input) {
            (Api::NumberFormat, Input::Number(number)) => {
                format_number(locale, &ecma::number_options(options)?, number)
            }
            (Api::DateTimeFormat, Input::Number(time)) => {
                format_date(locale, &ecma::date_options(options)?, time)
            }
            (Api::RelativeTimeFormat, Input::Relative(count, unit)) => {
                format_relative(locale, &ecma::relative_options(options)?, count, unit)
            }
            _ => Err(Refusal::new("the facility takes no such input")),
        }
    }

    fn select(&self, locale: &str, request: &Request<'_>) -> Result<Category, Refusal> {
        let (Api::PluralRules, Input::Number(number)) = (request.api, request.input) else {
            return Err(Refusal::new("only `PluralRules` selects"));
        };
        let read = ecma::plural_options(&request.options)?;
        let locale = parse_locale(locale, None, None)?;
        let mut value = decimal(number)?;

        round(&mut value, &read.digits);

        let rules = if read.ordinal {
            PluralRules::try_new_ordinal((&locale).into())
        } else {
            PluralRules::try_new_cardinal((&locale).into())
        }
        .map_err(refused)?;

        Ok(match rules.category_for(&value) {
            PluralCategory::Zero => Category::Zero,
            PluralCategory::One => Category::One,
            PluralCategory::Two => Category::Two,
            PluralCategory::Few => Category::Few,
            PluralCategory::Many => Category::Many,
            PluralCategory::Other => Category::Other,
        })
    }
}

/// What [`IcuIntl`] reads and cannot express. A property under the currency
/// style is listed because ICU4X formats a currency amount with the
/// currency's own digits and default grouping, whatever the request names.
pub const UNEXPRESSIBLE: &[Unexpressible] = &[
    always(Api::NumberFormat, "notation"),
    always(Api::NumberFormat, "compactDisplay"),
    always(Api::NumberFormat, "unit"),
    always(Api::NumberFormat, "unitDisplay"),
    always(Api::NumberFormat, "localeMatcher"),
    currency("minimumIntegerDigits"),
    currency("minimumFractionDigits"),
    currency("maximumFractionDigits"),
    currency("minimumSignificantDigits"),
    currency("maximumSignificantDigits"),
    currency("roundingIncrement"),
    currency("roundingMode"),
    currency("roundingPriority"),
    currency("trailingZeroDisplay"),
    currency("useGrouping"),
    always(Api::DateTimeFormat, "weekday"),
    always(Api::DateTimeFormat, "era"),
    always(Api::DateTimeFormat, "year"),
    always(Api::DateTimeFormat, "month"),
    always(Api::DateTimeFormat, "day"),
    always(Api::DateTimeFormat, "dayPeriod"),
    always(Api::DateTimeFormat, "hour"),
    always(Api::DateTimeFormat, "minute"),
    always(Api::DateTimeFormat, "second"),
    always(Api::DateTimeFormat, "fractionalSecondDigits"),
    always(Api::DateTimeFormat, "timeZoneName"),
    always(Api::DateTimeFormat, "hour12"),
    always(Api::DateTimeFormat, "hourCycle"),
    always(Api::DateTimeFormat, "formatMatcher"),
    always(Api::DateTimeFormat, "localeMatcher"),
    always(Api::RelativeTimeFormat, "localeMatcher"),
    always(Api::PluralRules, "localeMatcher"),
];

const fn always(api: Api, property: &'static str) -> Unexpressible {
    Unexpressible {
        api,
        property,
        when: &[],
    }
}

const fn currency(property: &'static str) -> Unexpressible {
    Unexpressible {
        api: Api::NumberFormat,
        property,
        when: &[("style", "currency")],
    }
}

fn refused(error: impl std::fmt::Display) -> Refusal {
    Refusal::new(error.to_string())
}

// A locale ECMA-402 would read: a BCP 47 tag, with the numbering system and
// the calendar a request names set as its Unicode extension keywords.
fn parse_locale(
    tag: &str,
    numbering_system: Option<&str>,
    calendar: Option<&str>,
) -> Result<Locale, Refusal> {
    // ICU4X reads `_` as a separator; a language tag has none.
    if tag.contains('_') {
        return Err(Refusal::new("the locale is no language tag"));
    }

    let mut locale = Locale::try_from_str(tag).map_err(refused)?;

    if let Some(system) = numbering_system {
        let system = Keyword::try_from_str(system).map_err(refused)?;

        locale.extensions.unicode.keywords.set(key!("nu"), system);
    }

    if let Some(calendar) = calendar {
        let calendar = Keyword::try_from_str(calendar).map_err(refused)?;

        locale.extensions.unicode.keywords.set(key!("ca"), calendar);
    }

    Ok(locale)
}

// A double as the decimal its shortest round-trip digits write, which is the
// decimal ECMA-402's implementations format: 1.005 is 1.005, and rounds to
// 1.01.
fn decimal(number: f64) -> Result<Decimal, Refusal> {
    if !number.is_finite() {
        return Err(Refusal::new("the number is not finite"));
    }

    Decimal::try_from_str(&format!("{number:e}")).map_err(refused)
}

fn rounding_mode(mode: Mode) -> SignedRoundingMode {
    match mode {
        Mode::Ceil => SignedRoundingMode::Ceil,
        Mode::Floor => SignedRoundingMode::Floor,
        Mode::HalfCeil => SignedRoundingMode::HalfCeil,
        Mode::HalfFloor => SignedRoundingMode::HalfFloor,
        Mode::Expand => SignedRoundingMode::Unsigned(UnsignedRoundingMode::Expand),
        Mode::Trunc => SignedRoundingMode::Unsigned(UnsignedRoundingMode::Trunc),
        Mode::HalfExpand => SignedRoundingMode::Unsigned(UnsignedRoundingMode::HalfExpand),
        Mode::HalfTrunc => SignedRoundingMode::Unsigned(UnsignedRoundingMode::HalfTrunc),
        Mode::HalfEven => SignedRoundingMode::Unsigned(UnsignedRoundingMode::HalfEven),
    }
}

// An increment of ECMA-402 is 1, 2, 5 or 25 times a power of ten: the
// multiple, and how many places above the last fraction digit it rounds at.
fn increment(increment: u16) -> (RoundingIncrement, i16) {
    let (multiple, places) = match increment {
        2 => (2, 0),
        5 => (5, 0),
        10 => (1, 1),
        20 => (2, 1),
        25 => (25, 0),
        50 => (5, 1),
        100 => (1, 2),
        200 => (2, 2),
        250 => (25, 1),
        500 => (5, 2),
        1000 => (1, 3),
        2000 => (2, 3),
        2500 => (25, 2),
        5000 => (5, 3),
        _ => (1, 0),
    };
    let multiple = match multiple {
        2 => RoundingIncrement::MultiplesOf2,
        5 => RoundingIncrement::MultiplesOf5,
        25 => RoundingIncrement::MultiplesOf25,
        _ => RoundingIncrement::MultiplesOf1,
    };

    (multiple, places)
}

// ToRawFixed: rounded at the maximum fraction digit, trailing zeros cut down
// to the minimum. The magnitude is the place it rounded at.
fn round_fraction(value: &Decimal, digits: &Digits) -> (Decimal, i16) {
    let (minimum, maximum) = digits.fraction;
    let (multiple, places) = increment(digits.increment);
    let mut value = value.clone();

    value.round_with_mode_and_increment(
        places.saturating_sub(maximum),
        rounding_mode(digits.mode),
        multiple,
    );
    value.absolute.trim_end();
    value.absolute.pad_end(minimum.saturating_neg());

    (value, maximum.saturating_neg())
}

// ToRawPrecision: rounded at the maximum significant digit, trailing zeros
// cut down to the minimum.
fn round_significant(value: &Decimal, digits: &Digits) -> (Decimal, i16) {
    let (minimum, maximum) = digits.significant;
    let mut value = value.clone();

    if value.absolute.is_zero() {
        value.absolute.pad_end(1_i16.saturating_sub(minimum));

        return (value, 1_i16.saturating_sub(maximum));
    }

    let start = value.absolute.nonzero_magnitude_start();

    value.round_with_mode_and_increment(
        start.saturating_sub(maximum).saturating_add(1),
        rounding_mode(digits.mode),
        RoundingIncrement::MultiplesOf1,
    );

    // Rounding may carry into a new leading digit.
    let start = value.absolute.nonzero_magnitude_start();

    value.absolute.trim_end();
    value
        .absolute
        .pad_end(start.saturating_sub(minimum).saturating_add(1));

    (value, start.saturating_sub(maximum).saturating_add(1))
}

// FormatNumericToString: the rounding the digits name, or of the two the one
// keeping more or less precision, then the integer digits and the trailing
// zeros of an integer.
fn round(value: &mut Decimal, digits: &Digits) {
    let rounded = match digits.rounding {
        Rounding::Fraction => round_fraction(value, digits).0,
        Rounding::Significant => round_significant(value, digits).0,
        Rounding::More | Rounding::Less => {
            let (significant, significant_at) = round_significant(value, digits);
            let (fraction, fraction_at) = round_fraction(value, digits);
            let fixed_is_more_precise = fraction_at < significant_at;

            if fixed_is_more_precise == (digits.rounding == Rounding::More) {
                fraction
            } else {
                significant
            }
        }
    };

    *value = rounded;
    value.absolute.pad_start(digits.minimum_integer);

    if digits.strip_if_integer {
        value.absolute.trim_end_if_integer();
    }
}

fn sign_display(sign: ecma::Sign) -> SignDisplay {
    match sign {
        ecma::Sign::Auto => SignDisplay::Auto,
        ecma::Sign::Never => SignDisplay::Never,
        ecma::Sign::Always => SignDisplay::Always,
        ecma::Sign::ExceptZero => SignDisplay::ExceptZero,
        ecma::Sign::Negative => SignDisplay::Negative,
    }
}

#[inline(never)]
fn format_number(tag: &str, read: &NumberOptions, number: f64) -> Result<String, Refusal> {
    let locale = parse_locale(tag, read.numbering_system.as_deref(), None)?;
    let mut value = decimal(number)?;
    let grouping = match read.grouping {
        Grouping::Auto => GroupingStrategy::Auto,
        Grouping::Always => GroupingStrategy::Always,
        Grouping::Min2 => GroupingStrategy::Min2,
        Grouping::Never => GroupingStrategy::Never,
    };

    if let Style::Currency {
        code,
        display,
        accounting,
    } = &read.style
    {
        let code = CurrencyType::try_from_str(code).map_err(refused)?;
        let usage = if *accounting {
            CurrencyUsage::Accounting
        } else {
            CurrencyUsage::Standard
        };
        let prefs = (&locale).into();

        value.apply_sign_display(sign_display(read.sign));

        let formatter = match *display {
            "code" => CurrencyFormatter::try_new_code(prefs, code, usage.into()),
            "narrowSymbol" => CurrencyFormatter::try_new_symbol_narrow(prefs, code, usage.into()),
            "name" => CurrencyFormatter::try_new_name(prefs, code),
            _ => CurrencyFormatter::try_new_symbol(prefs, code, usage.into()),
        }
        .map_err(refused)?;

        return Ok(formatter.format_fixed_decimal(&value).to_string());
    }

    if read.style == Style::Percent {
        value.multiply_pow10(2);
    }

    round(&mut value, &read.digits);
    value.apply_sign_display(sign_display(read.sign));

    let formatter =
        DecimalFormatter::try_new((&locale).into(), grouping.into()).map_err(refused)?;

    if read.style != Style::Percent {
        return Ok(formatter.format(&value).to_string());
    }

    // The percent pattern shows a plus sign only where it is asked to, and
    // writes the number without its sign, except a plus sign, which would
    // then show twice.
    let display = if value.sign == Sign::Positive {
        value.sign = Sign::None;

        Display::ExplicitSign
    } else {
        Display::Standard
    };
    let formatter = PercentFormatter::try_new_with_decimal_formatter(
        (&locale).into(),
        formatter,
        display.into(),
    )
    .map_err(refused)?;

    Ok(formatter.format(&value).to_string())
}

// The digits `Intl.RelativeTimeFormat` shows a count with: its number
// format's defaults.
const COUNT_DIGITS: Digits = Digits {
    minimum_integer: 1,
    fraction: (0, 3),
    significant: (1, 21),
    rounding: Rounding::Fraction,
    increment: 1,
    mode: Mode::HalfExpand,
    strip_if_integer: false,
};

macro_rules! relative_formatter {
    ($style:expr, $unit:expr, $prefs:expr, $options:expr, $(($name:literal, $unit_variant:ident, $long:ident, $short:ident, $narrow:ident)),* $(,)?) => {
        match ($style, $unit) {
            $(
                ("short", Unit::$unit_variant) => RelativeTimeFormatter::$short($prefs, $options),
                ("narrow", Unit::$unit_variant) => RelativeTimeFormatter::$narrow($prefs, $options),
                (_, Unit::$unit_variant) => RelativeTimeFormatter::$long($prefs, $options),
            )*
        }
    };
}

#[inline(never)]
fn format_relative(
    tag: &str,
    read: &RelativeOptions,
    count: f64,
    unit: Unit,
) -> Result<String, Refusal> {
    let locale = parse_locale(tag, read.numbering_system.as_deref(), None)?;
    let mut value = decimal(count)?;

    round(&mut value, &COUNT_DIGITS);

    // Where words are allowed, zero is "now" on either side of it.
    if !read.always && value.absolute.is_zero() {
        value.sign = Sign::None;
    }

    let mut formatter_options = RelativeTimeFormatterOptions::default();

    formatter_options.numeric = if read.always {
        Numeric::Always
    } else {
        Numeric::Auto
    };

    let prefs = (&locale).into();
    let formatter = relative_formatter!(
        read.style,
        unit,
        prefs,
        formatter_options,
        (
            "second",
            Second,
            try_new_long_second,
            try_new_short_second,
            try_new_narrow_second
        ),
        (
            "minute",
            Minute,
            try_new_long_minute,
            try_new_short_minute,
            try_new_narrow_minute
        ),
        (
            "hour",
            Hour,
            try_new_long_hour,
            try_new_short_hour,
            try_new_narrow_hour
        ),
        (
            "day",
            Day,
            try_new_long_day,
            try_new_short_day,
            try_new_narrow_day
        ),
        (
            "week",
            Week,
            try_new_long_week,
            try_new_short_week,
            try_new_narrow_week
        ),
        (
            "month",
            Month,
            try_new_long_month,
            try_new_short_month,
            try_new_narrow_month
        ),
        (
            "year",
            Year,
            try_new_long_year,
            try_new_short_year,
            try_new_narrow_year
        ),
    )
    .map_err(refused)?;

    Ok(formatter.format(value).to_string())
}

// The milliseconds jiff's time zone rules reach, and the 400-year cycle the
// Gregorian calendar repeats on. A zone's rules past its last transition are
// a yearly rule, which repeats on that cycle, so an instant past the end is
// read at the instant whole cycles earlier; before the start, every zone is
// at the local mean time it is at there.
const JIFF_FIRST: i64 = -377_705_023_201_000;
const JIFF_LAST: i64 = 253_402_207_200_000;
const CYCLE: i64 = 12_622_780_800_000;

fn zone_offset(name: &str, time: i64) -> Result<i32, Refusal> {
    let zone = jiff::tz::TimeZone::get(name).map_err(|_| Refusal::new("no such time zone"))?;
    let within = if time > JIFF_LAST {
        let cycles = time
            .saturating_sub(JIFF_LAST)
            .saturating_sub(1)
            .checked_div(CYCLE)
            .unwrap_or_default()
            .saturating_add(1);

        time.saturating_sub(cycles.saturating_mul(CYCLE))
    } else {
        time.max(JIFF_FIRST)
    };
    let instant = jiff::Timestamp::from_millisecond(within).map_err(refused)?;

    Ok(zone.to_offset(instant).seconds())
}

fn length(style: &str) -> Length {
    match style {
        "short" => Length::Short,
        "medium" => Length::Medium,
        _ => Length::Long,
    }
}

// TimeClip: the instants ECMAScript's `Date` holds.
const TIME_CLIP: f64 = 8.64e15;

#[inline(never)]
fn format_date(tag: &str, read: &DateOptions, time: f64) -> Result<String, Refusal> {
    if !time.is_finite() || time.abs() > TIME_CLIP {
        return Err(Refusal::new("the instant is no time value"));
    }

    #[expect(
        clippy::cast_possible_truncation,
        reason = "TimeClip keeps the instant far inside an i64"
    )]
    let time = time.trunc() as i64;
    let (seconds, zone) = match &read.zone {
        Zone::Offset(seconds) => (*seconds, TimeZone::UNKNOWN),
        Zone::Named(name) => (zone_offset(name, time)?, IanaParser::new().parse(name)),
    };
    let offset = UtcOffset::try_from_seconds(seconds).map_err(refused)?;
    let local = ZonedDateTime::from_epoch_milliseconds_and_utc_offset(time, offset);
    let date_time = DateTime {
        date: local.date,
        time: local.time,
    };
    let locale = parse_locale(
        tag,
        read.numbering_system.as_deref(),
        read.calendar.as_deref(),
    )?;
    let prefs = (&locale).into();
    let zoned = ZonedDateTime {
        date: date_time.date,
        time: date_time.time,
        zone: zone.with_offset(Some(offset)).at_date_time(date_time),
    };

    macro_rules! composite {
        ($set:expr) => {
            DateTimeFormatter::try_new(prefs, $set)
                .map(|formatter| formatter.format(&date_time).to_string())
        };
    }

    macro_rules! zoned {
        ($set:expr) => {
            DateTimeFormatter::try_new(prefs, $set)
                .map(|formatter| formatter.format(&zoned).to_string())
        };
    }

    let precision = match read.time_style {
        Some("short") => TimePrecision::Minute,
        _ => TimePrecision::Second,
    };
    let full = read.date_style == Some("full");
    let date_length = length(read.date_style.unwrap_or("long"));
    let formatted = match (read.date_style, read.time_style) {
        (None, None) => composite!(CompositeDateTimeFieldSet::Date(DateFieldSet::YMD(
            YMD::short().with_year_style(YearStyle::Full)
        ))),
        (Some(_), None) if full => {
            composite!(CompositeDateTimeFieldSet::Date(DateFieldSet::YMDE(
                YMDE::long()
            )))
        }
        (Some(_), None) => composite!(CompositeDateTimeFieldSet::Date(DateFieldSet::YMD(
            YMD::for_length(date_length)
        ))),
        (None, Some("long")) => zoned!(
            T::medium()
                .with_time_precision(precision)
                .with_zone(zone::SpecificShort)
        ),
        (None, Some("full")) => zoned!(
            T::medium()
                .with_time_precision(precision)
                .with_zone(zone::SpecificLong)
        ),
        (None, Some(_)) => composite!(CompositeDateTimeFieldSet::Time(TimeFieldSet::T(
            T::medium().with_time_precision(precision)
        ))),
        (Some(_), Some("long")) if full => zoned!(
            YMDET::long()
                .with_time_precision(precision)
                .with_zone(zone::SpecificShort)
        ),
        (Some(_), Some("full")) if full => zoned!(
            YMDET::long()
                .with_time_precision(precision)
                .with_zone(zone::SpecificLong)
        ),
        (Some(_), Some(_)) if full => composite!(CompositeDateTimeFieldSet::DateTime(
            DateAndTimeFieldSet::YMDET(YMDET::long().with_time_precision(precision))
        )),
        (Some(_), Some("long")) => zoned!(
            YMDT::for_length(date_length)
                .with_time_precision(precision)
                .with_zone(zone::SpecificShort)
        ),
        (Some(_), Some("full")) => zoned!(
            YMDT::for_length(date_length)
                .with_time_precision(precision)
                .with_zone(zone::SpecificLong)
        ),
        (Some(_), Some(_)) => composite!(CompositeDateTimeFieldSet::DateTime(
            DateAndTimeFieldSet::YMDT(YMDT::for_length(date_length).with_time_precision(precision))
        )),
    };

    formatted.map_err(refused)
}
