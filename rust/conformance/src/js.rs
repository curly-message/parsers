//! Numbers and JSON written the way ECMAScript writes them. The set was
//! written against a JavaScript host, and two of its readings are text that
//! host makes of a value: a request is compared by the canonical text of its
//! entries, and the `props` behaviour answers with each value as
//! `JSON.stringify` writes it. A runner on another host has to write them the
//! same way, or `1e21` and `1e+21` would differ for no reason the format
//! gives.

use std::fmt::Write as _;

use crate::value::Value;

/// A number as ECMAScript's Number::toString writes it: `1e+21`, `0.000001`,
/// `1e-7`, `0` for negative zero, `Infinity`, `NaN`.
pub fn number(value: f64) -> String {
    if value.is_nan() {
        return "NaN".to_owned();
    }

    if value == 0.0 {
        return "0".to_owned();
    }

    if value.is_infinite() {
        return if value > 0.0 { "Infinity" } else { "-Infinity" }.to_owned();
    }

    if value < 0.0 {
        return format!("-{}", number(-value));
    }

    // Rust writes the shortest digits that read back as the same double, as
    // ECMAScript does, but for a tie between two of them; their layout
    // differs too.
    let scientific = format!("{value:e}");
    let (mantissa, exponent) = scientific.split_once('e').unwrap_or((&scientific, "0"));
    let digits: String = mantissa.chars().filter(char::is_ascii_digit).collect();
    let exponent: i64 = exponent.parse().unwrap_or(0);
    let digits = even_at_a_tie(value, digits, exponent);
    let k = digits.len() as i64;
    let n = exponent + 1;

    if k <= n && n <= 21 {
        format!("{digits}{}", "0".repeat((n - k) as usize))
    } else if 0 < n && n <= 21 {
        let (whole, fraction) = digits.split_at(n as usize);

        format!("{whole}.{fraction}")
    } else if -6 < n && n <= 0 {
        format!("0.{}{digits}", "0".repeat((-n) as usize))
    } else {
        let sign = if n - 1 < 0 { '-' } else { '+' };
        let (first, rest) = digits.split_at(1);
        let rest = if rest.is_empty() {
            String::new()
        } else {
            format!(".{rest}")
        };

        format!("{first}{rest}e{sign}{}", (n - 1).abs())
    }
}

// Of two shortest digit strings equally close to the double, ECMAScript's
// `Number::toString` takes the even one, where Rust's `{:e}` takes the upper.
// They tie only where the double's exact decimal expansion is the shortest
// digits and one more, a 5; that expansion is read off the double exactly,
// and the even string is taken where it reads back as the double.
fn even_at_a_tie(value: f64, digits: String, exponent: i64) -> String {
    let Some(exact) = exact_digits(value) else {
        return digits;
    };

    if exact.checked_ilog10().map(|log| log as usize) != Some(digits.len()) || exact % 10 != 5 {
        return digits;
    }

    let truncated = exact / 10;
    let even = if truncated % 2 == 0 {
        truncated
    } else {
        truncated + 1
    };
    let shortest = digits.parse::<u128>().ok();
    let is_candidate = shortest == Some(truncated) || shortest == Some(truncated + 1);
    let places = exponent - digits.len() as i64 + 1;
    let even_digits = even.to_string();

    if is_candidate
        && shortest != Some(even)
        && even_digits.len() == digits.len()
        && format!("{even_digits}e{places}").parse::<f64>() == Ok(value)
    {
        even_digits
    } else {
        digits
    }
}

// The digits of a finite, non-zero double's exact decimal expansion as an
// integer `N`, the double being `N` times a power of ten, where `N` fits in
// a `u128`; a tie needs at most 18 digits. The double is `m * 2^q` with
// `m` odd: for a negative `q` that is `m * 5^-q * 10^q`, and for a positive
// one `m / 5^q * 10^q`, where `5^q` divides `m`, and otherwise its digits end
// in an even one.
fn exact_digits(value: f64) -> Option<u128> {
    let bits = value.to_bits();
    let fraction = bits & ((1 << 52) - 1);
    let biased = (bits >> 52) & 0x7ff;
    let (significand, power) = if biased == 0 {
        (fraction, -1074)
    } else {
        (fraction | (1 << 52), biased as i64 - 1075)
    };
    let zeros = significand.trailing_zeros();
    let odd = significand >> zeros;
    let power = power + i64::from(zeros);

    if power >= 0 {
        let five = 5_u64.checked_pow(u32::try_from(power).ok()?)?;

        (odd % five == 0).then(|| u128::from(odd / five))
    } else {
        5_u128
            .checked_pow(u32::try_from(power.unsigned_abs()).ok()?)?
            .checked_mul(u128::from(odd))
    }
}

/// Text as a JSON string literal, escaped as `JSON.stringify` escapes it: the
/// quote, the backslash and the control characters, and nothing else.
pub fn quote(text: &str) -> String {
    let mut quoted = String::with_capacity(text.len() + 2);

    quoted.push('"');

    for character in text.chars() {
        match character {
            '"' => quoted.push_str("\\\""),
            '\\' => quoted.push_str("\\\\"),
            '\u{8}' => quoted.push_str("\\b"),
            '\u{c}' => quoted.push_str("\\f"),
            '\n' => quoted.push_str("\\n"),
            '\r' => quoted.push_str("\\r"),
            '\t' => quoted.push_str("\\t"),
            control if u32::from(control) < 0x20 => {
                let _ = write!(quoted, "\\u{:04x}", u32::from(control));
            }
            other => quoted.push(other),
        }
    }

    quoted.push('"');
    quoted
}

/// A value as `JSON.stringify` writes it: `None` where it writes nothing (the
/// host's absence), and `Err` where it raises (a value no serialization
/// describes). An object's integer-like keys come first, in ascending order,
/// as a JavaScript object holds them.
pub fn stringify(value: &Value) -> Result<Option<String>, Unserializable> {
    match value {
        Value::Undefined => Ok(None),
        Value::Null => Ok(Some("null".to_owned())),
        Value::Bool(flag) => Ok(Some(flag.to_string())),
        Value::Number(n) if n.is_finite() => Ok(Some(number(*n))),
        Value::Number(_) => Ok(Some("null".to_owned())),
        Value::String(text) => Ok(Some(quote(text))),
        Value::Array(elements) => {
            let written = elements
                .iter()
                .map(|element| Ok(stringify(element)?.unwrap_or_else(|| "null".to_owned())))
                .collect::<Result<Vec<_>, _>>()?;

            Ok(Some(format!("[{}]", written.join(","))))
        }
        Value::Object(members) => {
            let mut written = Vec::with_capacity(members.len());

            for (key, member) in in_object_order(members) {
                if let Some(text) = stringify(member)? {
                    written.push(format!("{}:{text}", quote(key)));
                }
            }

            Ok(Some(format!("{{{}}}", written.join(","))))
        }
        Value::Unserializable => Err(Unserializable),
    }
}

/// What `JSON.stringify` raises over: a value that holds itself, or what
/// stands in for one.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Unserializable;

// ECMAScript's array index: the canonical decimal text of an integer below
// 2^32 - 1.
fn index(key: &str) -> Option<u32> {
    let parsed: u32 = key.parse().ok()?;

    (parsed != u32::MAX && parsed.to_string() == key).then_some(parsed)
}

fn in_object_order(members: &[(String, Value)]) -> Vec<&(String, Value)> {
    let mut indices: Vec<_> = members
        .iter()
        .filter_map(|member| index(&member.0).map(|at| (at, member)))
        .collect();

    indices.sort_by_key(|(at, _)| *at);

    indices
        .into_iter()
        .map(|(_, member)| member)
        .chain(members.iter().filter(|member| index(&member.0).is_none()))
        .collect()
}

/// Text with its leading and trailing white space removed, as
/// `String.prototype.trim` removes it: ECMAScript's white space and line
/// terminators, which are not Rust's (U+FEFF is one, U+0085 is not).
pub fn trim(text: &str) -> &str {
    let space = |character: char| {
        matches!(
            character,
            '\u{9}'
                | '\u{a}'
                | '\u{b}'
                | '\u{c}'
                | '\u{d}'
                | ' '
                | '\u{a0}'
                | '\u{1680}'
                | '\u{2000}'
                ..='\u{200a}'
                    | '\u{2028}'
                    | '\u{2029}'
                    | '\u{202f}'
                    | '\u{205f}'
                    | '\u{3000}'
                    | '\u{feff}'
        )
    };

    text.trim_matches(space)
}

/// Keys compared as ECMAScript's default sort compares them: by UTF-16 code
/// units.
pub fn by_code_units(a: &str, b: &str) -> std::cmp::Ordering {
    a.encode_utf16().cmp(b.encode_utf16())
}

/// The text a request is compared by: its entries with keys sorted, so the
/// order of a request's properties, which is the host's, is never compared.
/// The sign of zero is the request's, though JSON writes both zeros alike: a
/// relative time reads `-0` as the past and `0` as the future.
pub fn canonical(value: &Value) -> String {
    match value {
        Value::Number(number) if *number == 0.0 && number.is_sign_negative() => "-0".to_owned(),
        Value::Array(elements) => format!(
            "[{}]",
            elements.iter().map(canonical).collect::<Vec<_>>().join(",")
        ),
        Value::Object(members) => {
            let mut sorted: Vec<_> = members.iter().collect();

            sorted.sort_by(|a, b| by_code_units(&a.0, &b.0));

            format!(
                "{{{}}}",
                sorted
                    .iter()
                    .map(|(key, member)| format!("{}:{}", quote(key), canonical(member)))
                    .collect::<Vec<_>>()
                    .join(",")
            )
        }
        Value::Undefined => "undefined".to_owned(),
        Value::Unserializable => "unserializable".to_owned(),
        other => stringify(other)
            .ok()
            .flatten()
            .unwrap_or_else(|| "undefined".to_owned()),
    }
}

/// A value for a failure line: as JSON where JSON describes it.
pub fn show(value: &Value) -> String {
    match stringify(value) {
        Ok(Some(text)) => text,
        Ok(None) => "undefined".to_owned(),
        Err(Unserializable) => "[unserializable]".to_owned(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn numbers_are_written_as_ecmascript_writes_them() {
        for (value, written) in [
            (0.1, "0.1"),
            (42.0, "42"),
            (1e21, "1e+21"),
            (1e20, "100000000000000000000"),
            (1e-6, "0.000001"),
            (1e-7, "1e-7"),
            (-0.0, "0"),
            (-1.5, "-1.5"),
            (1234.5678, "1234.5678"),
            (1.7976931348623157e308, "1.7976931348623157e+308"),
            (5e-324, "5e-324"),
            (1.5e-7, "1.5e-7"),
            (123456789012345680000.0, "123456789012345680000"),
            (f64::INFINITY, "Infinity"),
            (f64::NEG_INFINITY, "-Infinity"),
            (f64::NAN, "NaN"),
        ] {
            assert_eq!(number(value), written, "{value:e}");
        }
    }

    #[test]
    #[expect(
        clippy::excessive_precision,
        reason = "a tie is written as the double's exact value, its last digit a 5"
    )]
    fn a_tie_between_two_shortest_digit_strings_takes_the_even_one() {
        for (value, written) in [
            (2516583712.37890625, "2516583712.3789062"),
            (-1600987034202908.25, "-1600987034202908.2"),
            (0.1 + 0.2, "0.30000000000000004"),
            (1e23, "1e+23"),
            (5e-324, "5e-324"),
        ] {
            assert_eq!(number(value), written, "{value:e}");
        }
    }

    #[test]
    fn json_is_written_as_json_stringify_writes_it() {
        let value = Value::object(vec![
            ("b".to_owned(), Value::Number(1.0)),
            ("10".to_owned(), Value::Bool(true)),
            ("2".to_owned(), Value::Null),
            ("u".to_owned(), Value::Undefined),
            (
                "a".to_owned(),
                Value::array(vec![
                    Value::Undefined,
                    Value::Number(f64::NAN),
                    "\u{1}\"\n".into(),
                ]),
            ),
            ("4294967295".to_owned(), Value::Number(1e21)),
        ]);

        assert_eq!(
            stringify(&value).unwrap().unwrap(),
            r#"{"2":null,"10":true,"b":1,"a":[null,null,"\u0001\"\n"],"4294967295":1e+21}"#
        );
        assert_eq!(stringify(&Value::Undefined), Ok(None));
        assert_eq!(
            stringify(&Value::array(vec![Value::Unserializable])),
            Err(Unserializable)
        );
    }

    #[test]
    fn text_is_trimmed_as_ecmascript_trims_it() {
        assert_eq!(trim("\u{feff}\u{2028} a b\u{3000}\n"), "a b");
        assert_eq!(trim("\u{85}a\u{85}"), "\u{85}a\u{85}");
        assert_eq!(trim("\u{200b}a"), "\u{200b}a");
    }

    #[test]
    fn a_request_is_compared_by_its_entries() {
        let one = Value::object(vec![
            ("style".to_owned(), "currency".into()),
            ("currency".to_owned(), "USD".into()),
        ]);
        let other = Value::object(vec![
            ("currency".to_owned(), "USD".into()),
            ("style".to_owned(), "currency".into()),
        ]);

        assert_eq!(canonical(&one), canonical(&other));
        assert_eq!(canonical(&one), r#"{"currency":"USD","style":"currency"}"#);
        assert_ne!(canonical(&Value::Number(2.0)), canonical(&"2".into()));
        assert_ne!(
            canonical(&Value::Number(-0.0)),
            canonical(&Value::Number(0.0))
        );
        assert_eq!(
            canonical(&Value::array(vec![Value::Number(-0.0), "second".into()])),
            r#"[-0,"second"]"#
        );
    }
}
