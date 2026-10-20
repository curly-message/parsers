use crate::convert::escape_json;
use crate::scan::LINE_TERM;
use crate::{MAX_NESTING, MAX_OUTPUT_LENGTH, MAX_READ_LENGTH, MAX_REPORTED_LENGTH};

/// What went wrong while a message resolved (section 14.3). It leaves through
/// the parser's report handler and nowhere else.
#[derive(Clone, Debug, PartialEq, Eq)]
#[non_exhaustive]
pub struct Report {
    /// The condition, as section 14.3 names it.
    pub code: Code,
    /// Whose defect it is: [`Code::origin`].
    pub origin: Origin,
    /// The id the caller gave the resolution.
    pub id: Option<String>,
    /// The limit reached, for the codes that name one.
    pub limit: Option<usize>,
    /// The placeholder that reported, as the message spells it, or the
    /// message itself: never a payload value. It is cut to
    /// [`MAX_REPORTED_LENGTH`] bytes and escaped as a JSON string's contents,
    /// every line terminator included, so a message cannot forge a line
    /// wherever a report is written.
    pub text: String,
    /// The condition, in English.
    pub message: &'static str,
}

/// The conditions a resolution reports (section 14.3).
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum Code {
    /// A placeholder named a modifier the parser does not know.
    UnknownModifier,
    /// A modifier could not produce a result.
    FailedModifier,
    /// A selection was given no options to select from.
    MissingOptions,
    /// A value could not become text.
    UnserializableValue,
    /// A modifier that depends on a locale was given none.
    MissingLocale,
    /// A placeholder was nested deeper than [`MAX_NESTING`] levels.
    NestingLimit,
    /// A placeholder would have carried the output past
    /// [`MAX_OUTPUT_LENGTH`] bytes.
    OutputLimit,
    /// Resolution read more than [`MAX_READ_LENGTH`] bytes of value text.
    ReadLimit,
}

/// Whose defect a report describes.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum Origin {
    /// The message was written wrong.
    Message,
    /// The payload, the props or a host-defined modifier could not serve it.
    Payload,
    /// A limit of section 13 stopped it.
    Limit,
}

impl Code {
    /// The code as section 14.3 spells it.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::UnknownModifier => "unknown-modifier",
            Self::FailedModifier => "failed-modifier",
            Self::MissingOptions => "missing-options",
            Self::UnserializableValue => "unserializable-value",
            Self::MissingLocale => "missing-locale",
            Self::NestingLimit => "nesting-limit",
            Self::OutputLimit => "output-limit",
            Self::ReadLimit => "read-limit",
        }
    }

    /// Whose defect the code describes. It is a property of the code rather
    /// than of the site that reported it.
    pub fn origin(self) -> Origin {
        match self {
            Self::UnknownModifier | Self::MissingOptions | Self::NestingLimit => Origin::Message,
            Self::FailedModifier | Self::UnserializableValue | Self::MissingLocale => {
                Origin::Payload
            }
            Self::OutputLimit | Self::ReadLimit => Origin::Limit,
        }
    }

    fn limit(self) -> Option<usize> {
        match self {
            Self::NestingLimit => Some(MAX_NESTING),
            Self::OutputLimit => Some(MAX_OUTPUT_LENGTH),
            Self::ReadLimit => Some(MAX_READ_LENGTH),
            _ => None,
        }
    }

    fn message(self) -> &'static str {
        match self {
            Self::UnknownModifier => "A placeholder named a modifier this parser does not know.",
            Self::FailedModifier => {
                "A modifier could not produce a result, so the placeholder took its fallback chain."
            }
            Self::MissingOptions => {
                "A selection was given no options to select from, so the placeholder took its fallback chain."
            }
            Self::UnserializableValue => {
                "A value could not become text, so resolution read it as missing."
            }
            Self::MissingLocale => {
                "A modifier that depends on a locale was given none, so the placeholder resolved to the empty string."
            }
            Self::NestingLimit => {
                "A placeholder was nested deeper than 8 levels, so it took its fallback chain."
            }
            Self::OutputLimit => {
                "A placeholder resolved to text this resolution has no room for, and would have carried the output past 100000 bytes, so it resolved to the empty string."
            }
            Self::ReadLimit => {
                "Resolution read more than 100000 bytes of value text, so this placeholder resolved to the empty string."
            }
        }
    }
}

impl Origin {
    /// The origin as section 14.3 spells it.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Message => "message",
            Self::Payload => "payload",
            Self::Limit => "limit",
        }
    }
}

impl Report {
    pub(crate) fn new(code: Code, reported: &str, id: Option<&str>) -> Self {
        Self {
            code,
            origin: code.origin(),
            id: id.map(str::to_owned),
            limit: code.limit(),
            text: excerpt(reported),
            message: code.message(),
        }
    }
}

// The JSON escape leaves raw every terminator it has no short escape for, so
// each one the format holds is escaped again on top of it; the ones it did
// escape are two characters by then and no longer match.
fn excerpt(text: &str) -> String {
    let mut cut = text.len().min(MAX_REPORTED_LENGTH);

    while !text.is_char_boundary(cut) {
        cut = cut.saturating_sub(1);
    }

    let mut escaped = String::with_capacity(cut.saturating_add(8));

    let _ = escape_json(text.get(..cut).unwrap_or_default(), |run| {
        escaped.push_str(run);

        Some(())
    });

    if cut < text.len() {
        escaped.push_str("...");
    }

    if !escaped.contains(LINE_TERM) {
        return escaped;
    }

    escaped.chars().fold(
        String::with_capacity(escaped.len()),
        |mut output, character| {
            if LINE_TERM.contains(&character) {
                output.push_str(&format!("\\u{:04x}", u32::from(character)));
            } else {
                output.push(character);
            }

            output
        },
    )
}

#[cfg(test)]
mod tests {
    #![allow(clippy::unwrap_used, clippy::indexing_slicing)]

    use super::excerpt;

    // A placeholder holds no line terminator, so the resolution reaches none
    // of these; the excerpt is held to them all the same.
    #[test]
    fn every_line_terminator_leaves_escaped() {
        assert_eq!(excerpt("a\u{2028}b\u{2029}c"), "a\\u2028b\\u2029c");
        assert_eq!(excerpt("a\nb\rc"), "a\\nb\\rc");
        assert_eq!(excerpt("\u{2027}\u{85}"), "\u{2027}\u{85}");

        let long = format!("{}\u{2028}", "a".repeat(118));

        assert_eq!(excerpt(&long), format!("{}...", "a".repeat(118)));
    }
}
