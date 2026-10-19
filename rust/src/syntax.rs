use std::borrow::Cow;

use crate::scan::{Scanner, char_at, escape_end, is_blank};

/// Whether a backslash in front of this character cancels a structural
/// meaning rather than denoting itself (section 7). The syntax reserves a
/// colon, a semicolon, either brace, a backslash and whitespace.
pub(crate) fn cancels(character: char) -> bool {
    matches!(character, ':' | ';' | '{' | '}' | '\\') || is_blank(character)
}

/// Text as the format reads it, with every escape sequence it claims
/// resolved. Text holding no backslash is returned as it is.
pub(crate) fn unescape(text: &str) -> Cow<'_, str> {
    if !text.contains('\\') {
        return Cow::Borrowed(text);
    }

    let mut output = String::with_capacity(text.len());
    let mut characters = text.chars();

    while let Some(character) = characters.next() {
        if character != '\\' {
            output.push(character);
            continue;
        }

        match characters.next() {
            Some(consumed) => {
                if !cancels(consumed) {
                    output.push('\\');
                }

                output.push(consumed);
            }
            None => output.push('\\'),
        }
    }

    Cow::Owned(output)
}

/// The span between `from` and `to` with its blank padding dropped, empty at
/// `from` where the span is padding throughout. Whitespace an escape sequence
/// claims is text, not padding around it.
///
/// Only the padding is read, from either end: an option value is a span that
/// holds every placeholder nested in it, so reading the whole span at each
/// level would read a message nested n levels deep n times over.
pub(crate) fn trimmed(text: &str, from: usize, to: usize) -> (usize, usize) {
    let mut start = from;

    while start < to {
        match char_at(text, start) {
            Some(character) if is_blank(character) => {
                start = start.saturating_add(character.len_utf8())
            }
            _ => break,
        }
    }

    if start >= to {
        return (from, from);
    }

    let mut end = to;
    let bytes = text.as_bytes();

    while end > start {
        let Some(last) = text.get(..end).and_then(|head| head.chars().next_back()) else {
            break;
        };

        if !is_blank(last) {
            break;
        }

        let blank = end.saturating_sub(last.len_utf8());
        // A blank is text where a backslash consumes it, which is where an
        // odd run of them inside the span precedes it.
        let run = bytes.get(from..blank).map_or(0, |head| {
            head.iter().rev().take_while(|&&byte| byte == b'\\').count()
        });

        if run % 2 == 1 {
            break;
        }

        end = blank;
    }

    (start, end)
}

/// Where the first `separator` between `from` and `to` stands, passing over
/// every occurrence an escape sequence claims and every one a nested
/// placeholder writes (section 6, note 5). A caller that passes no scanner
/// divides by escapes alone: a key, a modifier name and an option key hold no
/// placeholder (note 10).
pub(crate) fn find(
    text: &str,
    from: usize,
    to: usize,
    separator: u8,
    scan: Option<&Scanner<'_>>,
) -> Option<usize> {
    let bytes = text.as_bytes();
    let mut at = from;

    while at < to {
        let byte = bytes.get(at).copied();

        if byte == Some(b'\\') {
            at = escape_end(text, at, to);
            continue;
        }

        if let Some(scan) = scan
            && byte == Some(b'{')
            && bytes.get(at.saturating_add(1)) == Some(&b'{')
            && let Some(close) = scan.end(at)
            && close <= to
        {
            at = close;
            continue;
        }

        if byte == Some(separator) {
            return Some(at);
        }

        at = at.saturating_add(1);
    }

    None
}

/// A span with its padding dropped, and the name it answers to.
pub(crate) struct Named<'m> {
    pub(crate) start: usize,
    pub(crate) end: usize,
    pub(crate) name: Cow<'m, str>,
}

pub(crate) fn named(text: &str, from: usize, to: usize) -> Named<'_> {
    let (start, end) = trimmed(text, from, to);

    Named {
        start,
        end,
        name: unescape(text.get(start..end).unwrap_or_default()),
    }
}
