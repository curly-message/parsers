use crate::convert::display;
use crate::scan::is_blank;
use crate::value::Value;

/// A double as ECMAScript's `Number::toString` writes it: `1e+21`, `1e-7`,
/// `0.000001`, `0` for `-0`, `Infinity`, `NaN`. Rust's `{:e}` gives the
/// shortest digits that read back as the same double, and the digits are laid
/// out by that operation's rules, so a number renders the same text here as in
/// JavaScript.
#[expect(
    clippy::arithmetic_side_effects,
    reason = "a double's decimal exponent and digit count lie within a few hundred"
)]
pub(crate) fn to_js_string(number: f64) -> String {
    if number.is_nan() {
        return "NaN".to_owned();
    }

    if number == 0.0 {
        return "0".to_owned();
    }

    if number.is_infinite() {
        return if number > 0.0 {
            "Infinity"
        } else {
            "-Infinity"
        }
        .to_owned();
    }

    let scientific = format!("{:e}", number.abs());
    let (mantissa, exponent) = scientific.split_once('e').unwrap_or((&scientific, "0"));
    let digits: String = mantissa.chars().filter(char::is_ascii_digit).collect();
    let exponent: i64 = exponent.parse().unwrap_or(0);
    // `k` digits with the decimal point after the `n`th, as ECMAScript names
    // them.
    let k = digits.len() as i64;
    let n = exponent + 1;
    let sign = if number < 0.0 { "-" } else { "" };
    let zeros = |count: i64| "0".repeat(usize::try_from(count).unwrap_or(0));

    let body = if k <= n && n <= 21 {
        format!("{digits}{}", zeros(n - k))
    } else if 0 < n && n <= 21 {
        let (whole, fraction) = digits.split_at(usize::try_from(n).unwrap_or(0).min(digits.len()));

        format!("{whole}.{fraction}")
    } else if -6 < n && n <= 0 {
        format!("0.{}{digits}", zeros(-n))
    } else {
        let (first, rest) = digits.split_at(1.min(digits.len()));
        let point = if rest.is_empty() { "" } else { "." };
        let exponent_sign = if n > 0 { "+" } else { "-" };

        format!("{first}{point}{rest}e{exponent_sign}{}", (n - 1).abs())
    };

    format!("{sign}{body}")
}

/// ECMAScript's StringToNumber: the number text denotes, or NaN.
///
/// Members of section 6's whitespace class are removed from both ends, which
/// is exactly ECMAScript's StrWhiteSpaceChar; empty text is 0; `0x`, `0o` and
/// `0b` introduce an unsigned integer in that base; `Infinity` is spelled
/// exactly; anything else must be a StrDecimalLiteral. So `" 12 "` is 12,
/// `0x10` is 16, and `1_000`, `inf` and `nan` are NaN, as `Number()` reads
/// them.
pub(crate) fn to_number(text: &str) -> f64 {
    let text = text.trim_matches(is_blank);

    match text {
        "" => return 0.0,
        "Infinity" | "+Infinity" => return f64::INFINITY,
        "-Infinity" => return f64::NEG_INFINITY,
        _ => {}
    }

    if let [b'0', prefix, digits @ ..] = text.as_bytes() {
        let radix = match prefix {
            b'x' | b'X' => Some(16),
            b'o' | b'O' => Some(8),
            b'b' | b'B' => Some(2),
            _ => None,
        };

        if let Some(radix) = radix {
            return radix_number(digits, radix);
        }
    }

    if is_decimal_literal(text.as_bytes()) {
        // The literal is checked first, so Rust's own grammar, which accepts
        // `inf` and `nan` in any case, never reads text ECMAScript rejects.
        text.parse().unwrap_or(f64::NAN)
    } else {
        f64::NAN
    }
}

// An unsigned integer in a base that is a power of two, rounded to the nearest
// double as ECMAScript rounds it: the first 128 bits are kept, any non-zero
// digit beyond them sets the lowest bit, and the conversion to a double then
// rounds once and correctly.
fn radix_number(digits: &[u8], radix: u32) -> f64 {
    if digits.is_empty() {
        return f64::NAN;
    }

    let bits = radix.trailing_zeros();
    let mut kept: u128 = 0;
    let mut dropped: u32 = 0;
    let mut sticky = false;

    for &byte in digits {
        let Some(digit) = char::from(byte).to_digit(radix) else {
            return f64::NAN;
        };

        match kept
            .checked_mul(u128::from(radix))
            .and_then(|wide| wide.checked_add(u128::from(digit)))
        {
            Some(wide) if dropped == 0 => kept = wide,
            _ => {
                dropped = dropped.saturating_add(1);
                sticky |= digit != 0;
            }
        }
    }

    let shift = i32::try_from(dropped.saturating_mul(bits))
        .unwrap_or(i32::MAX)
        .min(2048);

    (kept | u128::from(sticky)) as f64 * 2f64.powi(shift)
}

// `[+-]? (digits (. digits?)? | . digits) ([eE] [+-]? digits)?`, the whole text.
fn is_decimal_literal(bytes: &[u8]) -> bool {
    let digits = |from: usize| {
        bytes.get(from..).map_or(0, |rest| {
            rest.iter().take_while(|byte| byte.is_ascii_digit()).count()
        })
    };
    let mut at = usize::from(matches!(bytes.first(), Some(b'+' | b'-')));
    let whole = digits(at);

    at = at.saturating_add(whole);

    let mut fraction = 0;

    if bytes.get(at) == Some(&b'.') {
        fraction = digits(at.saturating_add(1));
        at = at.saturating_add(1).saturating_add(fraction);
    }

    if whole == 0 && fraction == 0 {
        return false;
    }

    if matches!(bytes.get(at), Some(b'e' | b'E')) {
        at = at.saturating_add(1);

        if matches!(bytes.get(at), Some(b'+' | b'-')) {
            at = at.saturating_add(1);
        }

        let exponent = digits(at);

        if exponent == 0 {
            return false;
        }

        at = at.saturating_add(exponent);
    }

    at == bytes.len()
}

const DAY: i64 = 86_400_000;

// The largest time value ECMAScript's TimeClip keeps, either side of the epoch.
const TIME_CLIP: f64 = 8.64e15;

/// An instant as ECMAScript's `toISOString` writes it, or nothing where the
/// milliseconds are not a time value (NaN, or outside TimeClip).
pub(crate) fn iso_date(milliseconds: f64) -> Option<String> {
    if !milliseconds.is_finite() || milliseconds.abs() > TIME_CLIP {
        return None;
    }

    // TimeClip truncates, and the bound above keeps the value well inside i64.
    let time = milliseconds.trunc() as i64;
    let (days, within) = (time.div_euclid(DAY), time.rem_euclid(DAY));
    let (year, month, day) = civil_from_days(days);
    let year = if (0..=9999).contains(&year) {
        format!("{year:04}")
    } else {
        format!(
            "{}{:06}",
            if year < 0 { '-' } else { '+' },
            year.unsigned_abs()
        )
    };

    Some(format!(
        "{year}-{month:02}-{day:02}T{:02}:{:02}:{:02}.{:03}Z",
        within / 3_600_000,
        within / 60_000 % 60,
        within / 1000 % 60,
        within % 1000,
    ))
}

// The proleptic Gregorian date of a day counted from the epoch.
#[expect(
    clippy::arithmetic_side_effects,
    reason = "TimeClip keeps the day count within 1e8 of the epoch"
)]
fn civil_from_days(days: i64) -> (i64, i64, i64) {
    let shifted = days + 719_468;
    let era = shifted.div_euclid(146_097);
    let day_of_era = shifted.rem_euclid(146_097);
    let year_of_era =
        (day_of_era - day_of_era / 1460 + day_of_era / 36_524 - day_of_era / 146_096) / 365;
    let day_of_year = day_of_era - (365 * year_of_era + year_of_era / 4 - year_of_era / 100);
    let shifted_month = (5 * day_of_year + 2) / 153;
    let day = day_of_year - (153 * shifted_month + 2) / 5 + 1;
    let month = if shifted_month < 10 {
        shifted_month + 3
    } else {
        shifted_month - 9
    };
    let year = year_of_era + era * 400 + i64::from(month <= 2);

    (year, month, day)
}

/// The number a formatting modifier formats (section 11.2): none for text
/// that is blank, since `""` reads as 0 and would format a number nobody
/// wrote, and none for text that is not a finite number.
pub(crate) fn modifier_input(text: &str) -> Option<f64> {
    if text.chars().all(is_blank) {
        return None;
    }

    let number = to_number(text);

    number.is_finite().then_some(number)
}

/// ECMAScript's ToNumber over the value model: `Undefined` is NaN, `Null` 0,
/// a boolean 1 or 0, text and a host value's text by StringToNumber, and a
/// date its time value. An array or an object is NaN, where ECMAScript would
/// read the text its `toString` gives.
pub(crate) fn to_number_value(value: &Value) -> f64 {
    match value {
        Value::Undefined | Value::Array(_) | Value::Object(_) => f64::NAN,
        Value::Null => 0.0,
        Value::Bool(flag) => f64::from(u8::from(*flag)),
        Value::Number(number) => *number,
        #[expect(
            clippy::cast_precision_loss,
            reason = "ToNumber rounds an integer to the nearest double"
        )]
        Value::Integer(number) => *number as f64,
        #[expect(
            clippy::cast_precision_loss,
            reason = "ToNumber rounds an integer to the nearest double"
        )]
        Value::Unsigned(number) => *number as f64,
        Value::String(text) => to_number(text),
        Value::Host(host) => display(&**host).map_or(f64::NAN, |text| to_number(&text)),
        Value::Date(milliseconds) => {
            if milliseconds.is_finite() && milliseconds.abs() <= TIME_CLIP {
                milliseconds.trunc() + 0.0
            } else {
                f64::NAN
            }
        }
    }
}

/// The instant text names, in milliseconds since the epoch, by the grammar
/// `date` reads after a timestamp: `YYYY[-MM[-DD]]`, or `+YYYYYY[-MM[-DD]]`
/// or `-YYYYYY[-MM[-DD]]`; then optionally `T`, `t` or one space and
/// `HH:mm[:ss[.fraction]]`; then, after at most one space, `Z`, `z`, `UTC`,
/// or an offset of `+` or `-` and `HH:mm`, `HHmm` or `HH:mm:ss`; then any
/// number of bracketed annotations, read and ignored.
/// What names no offset is in UTC, since there is no local zone to read it
/// in. A day past the month's last rolls into the next, as JavaScript's
/// parser rolls it, and an instant outside TimeClip is none.
pub(crate) fn date_input(text: &str) -> Option<f64> {
    let mut reader = Reader {
        bytes: text.as_bytes(),
        at: 0,
    };
    let year = match reader.peek() {
        Some(sign @ (b'+' | b'-')) => {
            reader.at = reader.at.checked_add(1)?;

            let digits = reader.digits(6)?;

            // ECMAScript gives the year zero one spelling.
            if sign == b'-' && digits == 0 {
                return None;
            }

            if sign == b'-' {
                digits.checked_neg()?
            } else {
                digits
            }
        }
        _ => reader.digits(4)?,
    };
    let month = if reader.eat(b'-') {
        Some(reader.digits(2).filter(|month| (1..=12).contains(month))?)
    } else {
        None
    };
    let day = if month.is_some() && reader.eat(b'-') {
        reader.digits(2).filter(|day| (1..=31).contains(day))?
    } else {
        1
    };
    let mut time = 0;
    let mut offset = 0;

    if reader.eat(b'T') || reader.eat(b't') || reader.eat(b' ') {
        time = reader.time()?;

        let spaced = reader.eat(b' ');
        let start = reader.at;

        // A zone begun and not finished names none, and is no date.
        match reader.zone() {
            Some(zone) => offset = zone,
            None if spaced || reader.at != start => return None,
            None => {}
        }
    }

    while reader.eat(b'[') {
        while !reader.eat(b']') {
            reader.peek()?;
            reader.at = reader.at.checked_add(1)?;
        }
    }

    if reader.at != text.len() {
        return None;
    }

    let days = days_from_civil(year, month.unwrap_or(1), day)?;
    let milliseconds = days
        .checked_mul(DAY)?
        .checked_add(time)?
        .checked_sub(offset)?;

    #[expect(
        clippy::cast_precision_loss,
        reason = "TimeClip bounds the instant far inside a double's integers"
    )]
    let milliseconds = milliseconds as f64;

    (milliseconds.abs() <= TIME_CLIP).then_some(milliseconds)
}

struct Reader<'t> {
    bytes: &'t [u8],
    at: usize,
}

impl Reader<'_> {
    fn peek(&self) -> Option<u8> {
        self.bytes.get(self.at).copied()
    }

    fn eat(&mut self, byte: u8) -> bool {
        if self.peek() == Some(byte) {
            self.at = self.at.saturating_add(1);

            true
        } else {
            false
        }
    }

    // Exactly `count` ASCII digits, as a number.
    fn digits(&mut self, count: usize) -> Option<i64> {
        let end = self.at.checked_add(count)?;
        let digits = self.bytes.get(self.at..end)?;
        let mut number: i64 = 0;

        for digit in digits {
            if !digit.is_ascii_digit() {
                return None;
            }

            number = number
                .checked_mul(10)?
                .checked_add(i64::from(digit.wrapping_sub(b'0')))?;
        }

        self.at = end;

        Some(number)
    }

    // `HH:mm[:ss[.fraction]]`, in milliseconds into the day; `24:00` is the
    // day's end.
    fn time(&mut self) -> Option<i64> {
        let hour = self.digits(2).filter(|hour| *hour <= 24)?;

        if !self.eat(b':') {
            return None;
        }

        let minute = self.digits(2).filter(|minute| *minute <= 59)?;
        let mut second = 0;
        let mut millisecond = 0;

        if self.eat(b':') {
            second = self.digits(2).filter(|second| *second <= 59)?;

            if self.eat(b'.') {
                let start = self.at;
                let mut scale = 100;

                while let Some(digit) = self.peek().filter(u8::is_ascii_digit) {
                    millisecond = i64::from(digit.wrapping_sub(b'0'))
                        .checked_mul(scale)?
                        .checked_add(millisecond)?;
                    scale /= 10;
                    self.at = self.at.saturating_add(1);
                }

                if self.at == start {
                    return None;
                }
            }
        }

        if hour == 24 && (minute, second, millisecond) != (0, 0, 0) {
            return None;
        }

        hour.checked_mul(3_600_000)?
            .checked_add(minute.checked_mul(60_000)?)?
            .checked_add(second.checked_mul(1000)?)?
            .checked_add(millisecond)
    }

    // An offset from UTC in milliseconds, where the text names one.
    fn zone(&mut self) -> Option<i64> {
        if self.eat(b'Z') || self.eat(b'z') {
            return Some(0);
        }

        if self.bytes.get(self.at..)?.starts_with(b"UTC") {
            self.at = self.at.checked_add(3)?;

            return Some(0);
        }

        let sign = match self.peek()? {
            b'+' => 1,
            b'-' => -1,
            _ => return None,
        };

        self.at = self.at.checked_add(1)?;

        let hours = self.digits(2).filter(|hours| *hours <= 23)?;
        let colon = self.eat(b':');
        let minutes = self.digits(2).filter(|minutes| *minutes <= 59)?;
        let seconds = if colon && self.eat(b':') {
            self.digits(2).filter(|seconds| *seconds <= 59)?
        } else {
            0
        };

        hours
            .checked_mul(3600)?
            .checked_add(minutes.checked_mul(60)?)?
            .checked_add(seconds)?
            .checked_mul(1000)?
            .checked_mul(sign)
    }
}

// The days from the epoch to a proleptic Gregorian date; a day past the
// month's last counts on into the next month.
fn days_from_civil(year: i64, month: i64, day: i64) -> Option<i64> {
    let year = if month <= 2 {
        year.checked_sub(1)?
    } else {
        year
    };
    let era = year.div_euclid(400);
    let year_of_era = year.rem_euclid(400);
    let shifted_month = if month > 2 {
        month.checked_sub(3)?
    } else {
        month.checked_add(9)?
    };
    let day_of_year = shifted_month
        .checked_mul(153)?
        .checked_add(2)?
        .checked_div(5)?
        .checked_add(day)?
        .checked_sub(1)?;
    let day_of_era = year_of_era
        .checked_mul(365)?
        .checked_add(year_of_era / 4)?
        .checked_sub(year_of_era / 100)?
        .checked_add(day_of_year)?;

    era.checked_mul(146_097)?
        .checked_add(day_of_era)?
        .checked_sub(719_468)
}

#[cfg(test)]
mod tests {
    #![allow(clippy::unwrap_used, clippy::indexing_slicing)]

    use super::{date_input, iso_date, modifier_input, to_js_string, to_number, to_number_value};
    use crate::value::{Object, Value};

    // Each row was read off Node 22: `String(n)`.
    #[test]
    fn numbers_are_written_as_ecmascript_writes_them() {
        let cases: [(f64, &str); 22] = [
            (0.0, "0"),
            (-0.0, "0"),
            (42.0, "42"),
            (-42.5, "-42.5"),
            (0.1, "0.1"),
            (0.1 + 0.2, "0.30000000000000004"),
            (1e21, "1e+21"),
            (1e20, "100000000000000000000"),
            (123_456_789_012_345_680_000.0, "123456789012345680000"),
            (1e-6, "0.000001"),
            (1e-7, "1e-7"),
            (1.5e-7, "1.5e-7"),
            (2.5e25, "2.5e+25"),
            (f64::MAX, "1.7976931348623157e+308"),
            (f64::MIN_POSITIVE, "2.2250738585072014e-308"),
            (5e-324, "5e-324"),
            (f64::INFINITY, "Infinity"),
            (f64::NEG_INFINITY, "-Infinity"),
            (f64::NAN, "NaN"),
            (9_007_199_254_740_993.0, "9007199254740992"),
            (1.0, "1"),
            (123.456, "123.456"),
        ];

        for (number, text) in cases {
            assert_eq!(to_js_string(number), text, "{number:?}");
        }
    }

    // Each row was read off Node 22: `Number(text)`.
    #[test]
    fn text_is_read_as_ecmascript_reads_it() {
        let cases: [(&str, f64); 30] = [
            ("", 0.0),
            ("   ", 0.0),
            (" 12 ", 12.0),
            ("\u{a0}\u{3000}12\u{feff}\u{2028}", 12.0),
            ("12px", f64::NAN),
            ("0x10", 16.0),
            ("0X1f", 31.0),
            ("0x1g", f64::NAN),
            ("0x", f64::NAN),
            ("-0x10", f64::NAN),
            ("0o17", 15.0),
            ("0b101", 5.0),
            ("0b102", f64::NAN),
            ("Infinity", f64::INFINITY),
            ("+Infinity", f64::INFINITY),
            ("-Infinity", f64::NEG_INFINITY),
            ("inf", f64::NAN),
            ("INF", f64::NAN),
            ("nan", f64::NAN),
            ("infinity", f64::NAN),
            ("1_000", f64::NAN),
            ("1.", 1.0),
            (".5", 0.5),
            (".", f64::NAN),
            ("+.5e-3", 0.0005),
            ("1e", f64::NAN),
            ("1e+", f64::NAN),
            ("-1E3", -1000.0),
            ("1e400", f64::INFINITY),
            ("2.0", 2.0),
        ];

        for (text, number) in cases {
            let read = to_number(text);

            assert!(
                read == number || (read.is_nan() && number.is_nan()),
                "{text:?}: {read:?}"
            );
        }

        assert!(to_number("-0").is_sign_negative());
        assert_eq!(to_number("0x1fffffffffffff"), 9_007_199_254_740_991.0);
        assert_eq!(to_number("0x20000000000001"), 9_007_199_254_740_992.0);
        assert_eq!(to_number("0x20000000000003"), 9_007_199_254_740_996.0);
        assert_eq!(
            to_number(&format!("0x{}", "f".repeat(40))),
            1.461_501_637_330_903e48
        );
        assert_eq!(
            to_number(&format!("0x1{}1", "0".repeat(40))),
            2f64.powi(164)
        );
        assert_eq!(to_number(&format!("0x{}", "f".repeat(300))), f64::INFINITY);
        assert_eq!(to_number(&"1".repeat(400)), f64::INFINITY);
        assert_eq!(to_number(&"1".repeat(300)), 1.111_111_111_111_111_2e299);
    }

    #[test]
    fn instants_are_written_as_iso_strings() {
        let cases: [(f64, Option<&str>); 9] = [
            (0.0, Some("1970-01-01T00:00:00.000Z")),
            (-0.0, Some("1970-01-01T00:00:00.000Z")),
            (1_709_632_800_000.0, Some("2024-03-05T10:00:00.000Z")),
            (-1.0, Some("1969-12-31T23:59:59.999Z")),
            (1.9, Some("1970-01-01T00:00:00.001Z")),
            (8.64e15, Some("+275760-09-13T00:00:00.000Z")),
            (-8.64e15, Some("-271821-04-20T00:00:00.000Z")),
            (-62_198_755_200_000.0, Some("-000001-01-01T00:00:00.000Z")),
            (8.64e15 + 1.0, None),
        ];

        for (milliseconds, text) in cases {
            assert_eq!(iso_date(milliseconds).as_deref(), text, "{milliseconds:?}");
        }

        assert_eq!(iso_date(f64::NAN), None);
        assert_eq!(
            iso_date(253_402_300_799_999.0).as_deref(),
            Some("9999-12-31T23:59:59.999Z")
        );
        assert_eq!(
            iso_date(253_402_300_800_000.0).as_deref(),
            Some("+010000-01-01T00:00:00.000Z")
        );
    }

    // The values a formatting modifier reads as numbers, and their near
    // misses.
    #[test]
    fn a_modifier_reads_finite_numbers_written_as_text() {
        let cases: [(&str, Option<f64>); 10] = [
            (" 12 ", Some(12.0)),
            ("12px", None),
            ("0x10", Some(16.0)),
            ("0x1g", None),
            ("-0", Some(-0.0)),
            ("1e3", Some(1000.0)),
            ("Infinity", None),
            ("inf", None),
            ("", None),
            (" \t\n", None),
        ];

        for (text, number) in cases {
            assert_eq!(modifier_input(text), number, "{text:?}");
        }

        assert!(modifier_input("-0").unwrap().is_sign_negative());
    }

    #[test]
    fn a_property_is_read_as_ecmascript_reads_a_value_as_a_number() {
        let object = Value::from(Object::new().with("a", 1));
        let cases: [(Value, f64); 11] = [
            (Value::Null, 0.0),
            (Value::Bool(true), 1.0),
            (Value::Bool(false), 0.0),
            (Value::Number(2.5), 2.5),
            (Value::Integer(-3), -3.0),
            (Value::Unsigned(7), 7.0),
            (Value::from("  "), 0.0),
            (Value::from(" 0x10 "), 16.0),
            (Value::host(42), 42.0),
            (Value::date(1.9), 1.0),
            (Value::date(-0.0), 0.0),
        ];

        for (value, number) in cases {
            let read = to_number_value(&value);

            assert_eq!(read, number, "{value:?}");
            assert!(
                read.is_sign_positive() || number.is_sign_negative(),
                "{value:?}"
            );
        }

        for value in [
            Value::Undefined,
            Value::from("12px"),
            Value::date(8.64e15 + 1.0),
            Value::date(f64::NAN),
            Value::from(vec![1]),
            object,
        ] {
            assert!(to_number_value(&value).is_nan(), "{value:?}");
        }
    }

    // Node's `Date.parse` where it reads the text too, and the spellings of
    // the date crates' `Display` it does not read.
    #[test]
    fn date_text_is_read_by_its_grammar() {
        let cases: [(&str, Option<f64>); 34] = [
            ("2024-03-05T10:00:00Z", Some(1_709_632_800_000.0)),
            ("2024-03-05T10:00:00.000Z", Some(1_709_632_800_000.0)),
            (
                "2024-03-05T10:00:00.123456789+01:00",
                Some(1_709_629_200_123.0),
            ),
            ("2024-03-05t10:00:00z", Some(1_709_632_800_000.0)),
            ("2024-03-05T10:00Z", Some(1_709_632_800_000.0)),
            ("2024-03-05T10:00:00.5Z", Some(1_709_632_800_500.0)),
            ("2024-03-05T10:00:00+0100", Some(1_709_629_200_000.0)),
            ("2024-03-05T10:00:00", Some(1_709_632_800_000.0)),
            ("2024-03-05", Some(1_709_596_800_000.0)),
            ("2024-03", Some(1_709_251_200_000.0)),
            ("+002024-03-05T10:00:00Z", Some(1_709_632_800_000.0)),
            ("-000001-01-01T00:00:00Z", Some(-62_198_755_200_000.0)),
            ("2024-02-30", Some(1_709_251_200_000.0)),
            ("2024-03-05T24:00:00Z", Some(1_709_683_200_000.0)),
            ("+275760-09-13T00:00:00Z", Some(8.64e15)),
            // chrono's `DateTime<Utc>`, `DateTime<FixedOffset>` and
            // `NaiveDateTime`.
            ("2024-03-05 10:00:00 UTC", Some(1_709_632_800_000.0)),
            ("2024-03-05 10:00:00 +01:00", Some(1_709_629_200_000.0)),
            ("2024-03-05 10:00:00", Some(1_709_632_800_000.0)),
            // The `time` crate's `OffsetDateTime`.
            ("2024-03-05 10:00:00.0 +00:00:00", Some(1_709_632_800_000.0)),
            ("2024-03-05 10:00:00.0 -01:30:00", Some(1_709_638_200_000.0)),
            // jiff's `Zoned`, with its annotations.
            (
                "2024-03-05T11:00:00+01:00[Europe/Prague]",
                Some(1_709_632_800_000.0),
            ),
            (
                "2024-03-05T05:00:00-05:00[America/New_York][u-ca=iso8601]",
                Some(1_709_632_800_000.0),
            ),
            ("-000000-01-01T00:00:00Z", None),
            ("+275760-09-13T00:00:00.001Z", None),
            ("2024-03-05T24:00:01Z", None),
            ("2024-03-05T10:00:60Z", None),
            ("2024-03-05T10Z", None),
            ("2024-03-05T10:00:00+01", None),
            ("2024-13-01", None),
            ("2024-03-32", None),
            ("24-03-05", None),
            ("2024-03-05x", None),
            ("2024-03-05 10:00:00 ", None),
            ("2024-03-05T10:00:00[Europe/Prague", None),
        ];

        for (text, milliseconds) in cases {
            assert_eq!(date_input(text), milliseconds, "{text:?}");
        }
    }
}
