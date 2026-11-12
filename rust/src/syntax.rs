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

/// An option a placeholder declares, its value left as a span to render: a
/// value is message text, and rendering it is work the modifier asks for
/// rather than work reading the placeholder does (section 9.4).
pub(crate) struct Segment<'m> {
    pub(crate) key: Cow<'m, str>,
    pub(crate) value: (usize, usize),
}

/// What a placeholder declares (section 6): the key it names, the modifier it
/// names, its options in source order and the default it states inline.
pub(crate) struct Placeholder<'m> {
    pub(crate) key: Option<Cow<'m, str>>,
    pub(crate) modifier: Cow<'m, str>,
    pub(crate) options: Vec<Segment<'m>>,
    pub(crate) inline_default: Option<(usize, usize)>,
}

/// Reads the placeholder `scan` derived between `open` and `close`.
pub(crate) fn placeholder<'m>(scan: &Scanner<'m>, open: usize, close: usize) -> Placeholder<'m> {
    let text = scan.text();
    let from = open.saturating_add(2);
    let to = close.saturating_sub(2);
    let declaration = find(text, from, to, b';', Some(scan)).unwrap_or(to);
    // The selector's colon is the first one no escape claims, and everything
    // after it is the modifier name (section 6, note 3).
    let key_end = find(text, from, declaration, b':', None).unwrap_or(declaration);
    let key = named(text, from, key_end);
    let modifier = named(
        text,
        key_end.saturating_add(1).min(declaration),
        declaration,
    );
    let mut options = Vec::new();
    let mut inline_default = None;
    let mut at = declaration;

    while at < to {
        let start = at.saturating_add(1);
        let end = find(text, start, to, b';', Some(scan)).unwrap_or(to);
        let colon = find(text, start, end, b':', Some(scan));
        let option_key = named(text, start, colon.unwrap_or(end));
        // The first colon is the separator and every later one is value. An
        // option that names no value stands for itself; one that ends at its
        // colon declares the empty string.
        let value = match colon {
            Some(colon) => trimmed(text, colon.saturating_add(1), end),
            None => trimmed(text, start, end),
        };

        at = end;

        if option_key.start == option_key.end {
            continue;
        }

        // `default` is reserved in lowercase, so a segment is either the
        // inline default or an option, and a second `default` is neither.
        if option_key.name == "default" {
            inline_default.get_or_insert(value);
        } else {
            options.push(Segment {
                key: option_key.name,
                value,
            });
        }
    }

    Placeholder {
        key: (key.start != key.end).then_some(key.name),
        modifier: modifier.name,
        options,
        inline_default,
    }
}
