use std::cell::RefCell;
use std::collections::HashMap;
use std::fmt::{self, Write as _};
use std::ops::Deref;
use std::sync::Arc;

use crate::depth::host_call;
use crate::number::{iso_date, to_js_string};
use crate::value::{Object, Value};
use crate::{MAX_CONVERSION_NODES, MAX_SERIALIZED_LENGTH};

/// Text a resolution holds: borrowed from the message or the payload, shared
/// with the conversion record, or its own.
#[derive(Clone, Debug)]
pub(crate) enum Txt<'a> {
    Borrowed(&'a str),
    Shared(Arc<String>),
    Owned(String),
}

impl Deref for Txt<'_> {
    type Target = str;

    fn deref(&self) -> &str {
        match self {
            Self::Borrowed(text) => text,
            Self::Shared(text) => text,
            Self::Owned(text) => text,
        }
    }
}

impl From<Txt<'_>> for Box<str> {
    fn from(text: Txt<'_>) -> Self {
        match text {
            Txt::Owned(text) => text.into_boxed_str(),
            other => Box::from(&*other),
        }
    }
}

/// What a value converts to (section 4).
pub(crate) enum Converted<'a> {
    /// No value: nobody passed one.
    Absent,
    /// A value no conversion describes.
    Undescribable,
    Text(Txt<'a>),
}

/// One conversion per value per resolution (section 13): what each array,
/// object and host value came out as, keyed by the address of its
/// allocation, "undescribable" included. The value is kept alongside, so the
/// address cannot be reused by a value built later in the resolution.
#[derive(Default)]
pub(crate) struct Record {
    converted: RefCell<HashMap<usize, Recorded>>,
}

// A value, held so its address stays its own, and what it converted to.
type Recorded = (Value, Option<Arc<String>>);

fn address(value: &Value) -> Option<usize> {
    match value {
        Value::Array(array) => Some(Arc::as_ptr(array).cast::<()>() as usize),
        Value::Object(object) => Some(Arc::as_ptr(object).cast::<()>() as usize),
        Value::Host(host) => Some(Arc::as_ptr(host).cast::<()>() as usize),
        _ => None,
    }
}

impl Record {
    /// The text a value converts to. A recorded value is converted once; the
    /// record is never borrowed while a host's `Display` runs.
    pub(crate) fn convert<'v>(&self, value: &'v Value) -> Converted<'v> {
        let text = |text: &'v str| Converted::Text(Txt::Borrowed(text));
        let owned = |text: String| Converted::Text(Txt::Owned(text));

        match value {
            Value::Undefined => Converted::Absent,
            Value::Null => text("null"),
            Value::Bool(flag) => text(if *flag { "true" } else { "false" }),
            Value::Number(number) => owned(to_js_string(*number)),
            Value::Integer(number) => owned(number.to_string()),
            Value::Unsigned(number) => owned(number.to_string()),
            Value::String(string) => text(string),
            Value::Date(milliseconds) => {
                owned(iso_date(*milliseconds).unwrap_or_else(|| "Invalid Date".to_owned()))
            }
            Value::Array(_) | Value::Object(_) | Value::Host(_) => {
                let key = address(value).unwrap_or_default();
                let known = self
                    .converted
                    .borrow()
                    .get(&key)
                    .map(|(_, text)| text.clone());
                let text = match known {
                    Some(text) => text,
                    None => {
                        let text = match value {
                            Value::Host(host) => display(&**host),
                            _ => json(value),
                        }
                        .map(Arc::new);

                        self.converted
                            .borrow_mut()
                            .insert(key, (value.clone(), text.clone()));

                        text
                    }
                };

                text.map_or(Converted::Undescribable, |text| {
                    Converted::Text(Txt::Shared(text))
                })
            }
        }
    }
}

// A host value's own string conversion. One that fails, or panics, describes
// nothing, and so does one writing more than a string holds in V8: past
// `MAX_SERIALIZED_LENGTH` the writer refuses, as a serialization does, and a
// `Display` that goes on writing after a refusal still describes nothing.
fn display(host: &(dyn fmt::Display + Send + Sync)) -> Option<String> {
    host_call(|| {
        let mut text = Written::new();
        let written = write!(text, "{host}");

        written.ok().and_then(|()| text.finish())
    })
    .flatten()
}

/// Writes text as a JSON string's contents: `"`, `\` and U+0000 to U+001F
/// escaped, as `JSON.stringify` escapes them, and nothing else. Each is ASCII,
/// so the text is searched by byte and handed to `put` a run at a time.
pub(crate) fn escape_json(text: &str, mut put: impl FnMut(&str) -> Option<()>) -> Option<()> {
    let mut rest = text;

    while let Some(at) = rest
        .as_bytes()
        .iter()
        .position(|&byte| byte < 0x20 || byte == b'"' || byte == b'\\')
    {
        put(rest.get(..at)?)?;

        match rest.as_bytes().get(at)? {
            b'"' => put("\\\"")?,
            b'\\' => put("\\\\")?,
            0x08 => put("\\b")?,
            0x0c => put("\\f")?,
            b'\n' => put("\\n")?,
            b'\r' => put("\\r")?,
            b'\t' => put("\\t")?,
            &control => put(control_escape(control).as_str()?)?,
        }

        rest = rest.get(at.saturating_add(1)..)?;
    }

    put(rest)
}

/// What one serialization has written. It refuses to write past
/// [`MAX_SERIALIZED_LENGTH`] code units of UTF-16, the length V8 gives a
/// string, or past what the allocator grants, and the serialization is
/// abandoned instead of the process. A run is refused before any of it is
/// copied, so a refusal costs no more room than the bound. A refusal is the
/// serialization's, not the run's: nothing is written after it, and the text
/// is never handed out, whatever the writer does with the error.
struct Written {
    text: String,
    // The code units of the text up to `counted`. A code unit takes at least
    // a byte, so they are counted only once the bytes pass the bound.
    units: usize,
    counted: usize,
    refused: bool,
}

// The escape of a control character, `\u00xx` in lower case, held where it
// is made rather than allocated for each one.
struct ControlEscape([u8; 6]);

impl ControlEscape {
    fn as_str(&self) -> Option<&str> {
        std::str::from_utf8(&self.0).ok()
    }
}

fn control_escape(control: u8) -> ControlEscape {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let digit = |at: u8| HEX.get(usize::from(at & 0x0f)).copied().unwrap_or(b'0');

    ControlEscape([b'\\', b'u', b'0', b'0', digit(control >> 4), digit(control)])
}

impl Written {
    const fn new() -> Self {
        Self {
            text: String::new(),
            units: 0,
            counted: 0,
            refused: false,
        }
    }

    // The text written, where nothing was refused.
    fn finish(self) -> Option<String> {
        (!self.refused).then_some(self.text)
    }

    fn put(&mut self, run: &str) -> Option<()> {
        if !self.fits(run.len()) {
            self.admit(run)?;
        }

        self.text.push_str(run);

        Some(())
    }

    // Whether `more` bytes fit the room the text has and keep it within the
    // bound, which is what nearly every run does: only the others are
    // admitted.
    #[inline]
    fn fits(&self, more: usize) -> bool {
        self.text.capacity().saturating_sub(self.text.len()) >= more
            && self.text.len().saturating_add(more) <= MAX_SERIALIZED_LENGTH
    }

    fn string(&mut self, text: &str) -> Option<()> {
        self.mark('"')?;

        // Room for a string that needs no escape, asked for at once, where it
        // fits: what would not fit is refused run by run instead.
        if self.text.len().saturating_add(text.len()) < MAX_SERIALIZED_LENGTH {
            self.grow(text.len().saturating_add(1))?;
        }

        escape_json(text, |run| self.put(run))?;
        self.mark('"')
    }

    // One mark of ASCII punctuation, pushed rather than copied.
    fn mark(&mut self, mark: char) -> Option<()> {
        if !self.fits(mark.len_utf8()) {
            self.admit(mark.encode_utf8(&mut [0; 4]))?;
        }

        self.text.push(mark);

        Some(())
    }

    // Room for a run where it keeps the text within the bound. While the
    // bytes stay within it so do the code units, which take a byte at least;
    // past it the units are counted, of what is written once and of the run
    // without copying it.
    #[cold]
    fn admit(&mut self, run: &str) -> Option<()> {
        if self.refused {
            return None;
        }

        if self.text.len().saturating_add(run.len()) > MAX_SERIALIZED_LENGTH {
            let written = utf16_length(self.text.get(self.counted..)?);

            self.units = self.units.saturating_add(written);
            self.counted = self.text.len();

            // A code unit takes three bytes at most, so a run too long even
            // at that is refused unread.
            if self.units.saturating_add(run.len().div_ceil(3)) > MAX_SERIALIZED_LENGTH
                || self.units.saturating_add(utf16_length(run)) > MAX_SERIALIZED_LENGTH
            {
                return self.refuse();
            }
        }

        self.grow(run.len())
    }

    fn grow(&mut self, more: usize) -> Option<()> {
        match room(&mut self.text, more) {
            Some(()) => Some(()),
            None => self.refuse(),
        }
    }

    // The text is let go at once, since it is never handed out; with no room
    // left, every later run goes to `admit`, which refuses it.
    fn refuse(&mut self) -> Option<()> {
        self.refused = true;
        self.text = String::new();

        None
    }
}

// Room for `more` bytes, asked of the allocator only when the text has none
// left: asking costs a call for each run of a string. The text grows as a
// `String` does, from eight bytes and then by doubling, but not past the
// bytes the bound lets it hold: doubling would otherwise ask for up to twice
// that.
fn room(text: &mut String, more: usize) -> Option<()> {
    if text.capacity().saturating_sub(text.len()) >= more {
        return Some(());
    }

    let needed = text.len().saturating_add(more);
    let most = if needed <= MAX_SERIALIZED_LENGTH {
        MAX_SERIALIZED_LENGTH
    } else {
        MAX_SERIALIZED_LENGTH.saturating_mul(3)
    };
    let grown = text
        .capacity()
        .saturating_mul(2)
        .max(8)
        .min(most)
        .max(needed);

    text.try_reserve_exact(grown.saturating_sub(text.len()))
        .ok()
}

// The length of text in code units of UTF-16: each byte that starts a code
// point is one, and one that starts a code point past the Basic Multilingual
// Plane, from 0xF0, is two. A run of ASCII is a unit a byte, and is told
// apart a word at a time.
fn utf16_length(text: &str) -> usize {
    text.as_bytes().chunks(4096).fold(0, |units: usize, chunk| {
        let more = if chunk.is_ascii() {
            chunk.len()
        } else {
            chunk.iter().fold(0, |more: usize, &byte| {
                more.saturating_add(usize::from(byte & 0xc0 != 0x80))
                    .saturating_add(usize::from(byte >= 0xf0))
            })
        };

        units.saturating_add(more)
    })
}

impl fmt::Write for Written {
    fn write_str(&mut self, text: &str) -> fmt::Result {
        self.put(text).ok_or(fmt::Error)
    }
}

enum Open<'v> {
    Items(std::slice::Iter<'v, Value>, bool),
    Entries(std::slice::Iter<'v, (Arc<str>, Value)>, bool),
}

/// The JSON a plain value serializes to, written as `JSON.stringify` writes
/// it, or nothing where it visits more than [`MAX_CONVERSION_NODES`] nodes,
/// would write more than [`MAX_SERIALIZED_LENGTH`] code units, or holds a host
/// value that fails its conversion. It runs off a stack of its own, so a
/// value's depth never reaches the host's call stack.
pub(crate) fn json(value: &Value) -> Option<String> {
    let mut output = Written::new();
    let mut stack = Vec::new();
    let mut visits: usize = 1;

    write(&mut output, &mut stack, value)?;

    while let Some(top) = stack.last_mut() {
        let (next, written) = match top {
            Open::Items(items, written) => (items.next().map(|item| (None, item)), written),
            Open::Entries(entries, written) => {
                (entries.next().map(|(key, item)| (Some(key), item)), written)
            }
        };

        let Some((key, item)) = next else {
            output.mark(if matches!(top, Open::Items(..)) {
                ']'
            } else {
                '}'
            })?;
            stack.pop();
            continue;
        };

        visits = visits.saturating_add(1);

        if visits > MAX_CONVERSION_NODES {
            return None;
        }

        // A member holding nothing is left out of an object; in an array it
        // keeps its place as `null`.
        if key.is_some() && item.is_undefined() {
            continue;
        }

        if *written {
            output.mark(',')?;
        }

        *written = true;

        if let Some(key) = key {
            output.string(key)?;
            output.mark(':')?;
        }

        write(&mut output, &mut stack, item)?;
    }

    output.finish()
}

fn write<'v>(output: &mut Written, stack: &mut Vec<Open<'v>>, value: &'v Value) -> Option<()> {
    match value {
        Value::Undefined | Value::Null => output.put("null"),
        Value::Bool(flag) => output.put(if *flag { "true" } else { "false" }),
        Value::Number(number) if number.is_finite() => output.put(&to_js_string(*number)),
        Value::Number(_) => output.put("null"),
        Value::Integer(number) => write!(output, "{number}").ok(),
        Value::Unsigned(number) => write!(output, "{number}").ok(),
        Value::String(text) => output.string(text),
        Value::Date(milliseconds) => match iso_date(*milliseconds) {
            Some(text) => output.string(&text),
            None => output.put("null"),
        },
        Value::Host(host) => output.string(&display(&**host)?),
        Value::Array(array) => {
            stack.push(Open::Items(array.iter(), false));
            output.mark('[')
        }
        Value::Object(object) => {
            stack.push(Open::Entries(object_entries(object), false));
            output.mark('{')
        }
    }
}

fn object_entries(object: &Object) -> std::slice::Iter<'_, (Arc<str>, Value)> {
    object.entries().iter()
}
