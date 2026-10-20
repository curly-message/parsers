use crate::scan::is_blank;

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

#[cfg(test)]
mod tests {
    #![allow(clippy::unwrap_used, clippy::indexing_slicing)]

    use super::{iso_date, to_js_string, to_number};

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
}
