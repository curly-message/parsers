use std::cell::RefCell;
use std::collections::HashMap;

/// The format's `line-term` production (section 6, note 1): the four code
/// points a placeholder holds in no position. Every rule that reads the set
/// reads it from here, so an amendment moves one list.
pub(crate) const LINE_TERM: [char; 4] = ['\u{a}', '\u{d}', '\u{2028}', '\u{2029}'];

// The whitespace class section 6 enumerates: the line terminators and
// twenty-one code points besides.
const WHITESPACE: [char; 25] = [
    LINE_TERM[0],
    LINE_TERM[1],
    LINE_TERM[2],
    LINE_TERM[3],
    '\u{9}',
    '\u{b}',
    '\u{c}',
    '\u{20}',
    '\u{a0}',
    '\u{1680}',
    '\u{2000}',
    '\u{2001}',
    '\u{2002}',
    '\u{2003}',
    '\u{2004}',
    '\u{2005}',
    '\u{2006}',
    '\u{2007}',
    '\u{2008}',
    '\u{2009}',
    '\u{200a}',
    '\u{202f}',
    '\u{205f}',
    '\u{3000}',
    '\u{feff}',
];

/// Whether a character is in the whitespace class of section 6.
pub(crate) fn is_blank(character: char) -> bool {
    WHITESPACE.contains(&character)
}

fn is_terminator(character: char) -> bool {
    LINE_TERM.contains(&character)
}

/// The character starting at byte `at`, where one starts there.
pub(crate) fn char_at(text: &str, at: usize) -> Option<char> {
    text.get(at..).and_then(|rest| rest.chars().next())
}

/// Where the escape sequence opening at `at` ends. A backslash consumes the
/// whole code point after it (section 7), so the sequence is claimed whole
/// rather than cut inside a character. With nothing left before `to` it
/// consumes nothing, and the end falls past `to`.
pub(crate) fn escape_end(text: &str, at: usize, to: usize) -> usize {
    let next = at.saturating_add(1);
    let width = if next < to {
        char_at(text, next).map_or(1, char::len_utf8)
    } else {
        1
    };

    next.saturating_add(width)
}

// Which part of a placeholder the scan is inside. Only a value admits a nested
// placeholder (section 6, note 10).
#[derive(Clone, Copy, PartialEq, Eq)]
enum Part {
    Key,
    Modifier,
    OptionKey,
    OptionValue,
}

struct Frame {
    open: usize,
    at: usize,
    part: Part,
}

/// Reads one message for the placeholders it derives.
///
/// A verdict per opening position is final (section 6, note 11), so it is
/// recorded and never recomputed. That record is what keeps the scan linear in
/// the length of the message: the scan resumes one code point past a brace
/// that opened nothing (note 7), so attempts overlap, and without the record a
/// message that nests deeply costs time growing faster than any polynomial.
pub(crate) struct Scanner<'m> {
    text: &'m str,
    verdicts: RefCell<HashMap<usize, Option<usize>>>,
    #[cfg(test)]
    steps: std::cell::Cell<usize>,
}

impl<'m> Scanner<'m> {
    pub(crate) fn new(text: &'m str) -> Self {
        Self {
            text,
            verdicts: RefCell::new(HashMap::new()),
            #[cfg(test)]
            steps: std::cell::Cell::new(0),
        }
    }

    pub(crate) fn text(&self) -> &'m str {
        self.text
    }

    fn byte(&self, at: usize) -> Option<u8> {
        self.text.as_bytes().get(at).copied()
    }

    fn pair(&self, at: usize, brace: u8) -> bool {
        self.byte(at) == Some(brace) && self.byte(at.saturating_add(1)) == Some(brace)
    }

    fn step(&self) {
        #[cfg(test)]
        self.steps.set(self.steps.get().saturating_add(1));
    }

    fn known(&self, open: usize) -> Option<Option<usize>> {
        self.verdicts.borrow().get(&open).copied()
    }

    /// Where a complete placeholder opening at `open` ends, or nothing.
    pub(crate) fn end(&self, open: usize) -> Option<usize> {
        if let Some(known) = self.known(open) {
            return known;
        }

        // An explicit stack rather than recursion: nesting is not otherwise
        // limited (note 10), and a message a hundred thousand levels deep must
        // not take the host's call stack down with it.
        let mut stack = vec![Frame {
            open,
            at: open.saturating_add(2),
            part: Part::Key,
        }];
        let length = self.text.len();

        while let Some(frame) = stack.last_mut() {
            self.step();

            let at = frame.at;
            let Some(byte) = self.byte(at) else {
                break;
            };

            match byte {
                b'\\' => {
                    if char_at(self.text, at.saturating_add(1)).is_some_and(is_terminator) {
                        break;
                    }

                    frame.at = escape_end(self.text, at, length);
                }
                b';' => {
                    frame.part = Part::OptionKey;
                    frame.at = at.saturating_add(1);
                }
                b':' => {
                    // The first colon of a part is its separator and a later
                    // one is content (notes 3 and 4), so a part only moves on.
                    frame.part = match frame.part {
                        Part::Key => Part::Modifier,
                        Part::OptionKey => Part::OptionValue,
                        part => part,
                    };
                    frame.at = at.saturating_add(1);
                }
                b'{' if self.pair(at, b'{') => {
                    // A `{{` in a value must open a complete placeholder or the
                    // construct around it does not derive; anywhere else it
                    // opens none, so the construct does not derive either.
                    if frame.part != Part::OptionValue {
                        break;
                    }

                    match self.known(at) {
                        Some(Some(close)) => frame.at = close,
                        Some(None) => break,
                        None => stack.push(Frame {
                            open: at,
                            at: at.saturating_add(2),
                            part: Part::Key,
                        }),
                    }
                }
                b'}' if self.pair(at, b'}') => {
                    let close = at.saturating_add(2);

                    self.verdicts.borrow_mut().insert(frame.open, Some(close));
                    stack.pop();

                    match stack.last_mut() {
                        Some(outer) => outer.at = close,
                        None => return Some(close),
                    }
                }
                _ if byte.is_ascii() => {
                    if is_terminator(char::from(byte)) {
                        break;
                    }

                    frame.at = at.saturating_add(1);
                }
                _ => {
                    let character = char_at(self.text, at);

                    if character.is_some_and(is_terminator) {
                        break;
                    }

                    frame.at = at.saturating_add(character.map_or(1, char::len_utf8));
                }
            }
        }

        // Every construct still open when the scan dies is one whose closing
        // pair never arrived, so each of them derives nothing.
        let mut verdicts = self.verdicts.borrow_mut();

        for frame in stack {
            verdicts.insert(frame.open, None);
        }

        None
    }

    /// Where the next placeholder opening at or after `from` opens and ends.
    /// The search stops at `to`, so a span is read for what it holds rather
    /// than for what follows it.
    pub(crate) fn next(&self, from: usize, to: usize) -> Option<(usize, usize)> {
        let mut at = from;

        while at < to {
            self.step();

            match self.byte(at) {
                Some(b'\\') => {
                    at = escape_end(self.text, at, to);
                    continue;
                }
                // Both braces belong to the span: a placeholder that derives
                // inside one closes inside it, so a lone brace at the end
                // opens nothing.
                Some(b'{') if at.saturating_add(1) < to && self.pair(at, b'{') => {
                    // The braces a refused construct opened are text, and the
                    // scan resumes at the very next code point (note 7).
                    if let Some(close) = self.end(at) {
                        return Some((at, close));
                    }
                }
                _ => {}
            }

            at = at.saturating_add(1);
        }

        None
    }

    #[cfg(test)]
    pub(crate) fn steps(&self) -> usize {
        self.steps.get()
    }
}

#[cfg(test)]
mod tests {
    #![allow(
        clippy::unwrap_used,
        clippy::indexing_slicing,
        clippy::arithmetic_side_effects
    )]

    use super::Scanner;

    // Every placeholder a message derives, found the way a walk finds them:
    // the message, then each span a nested placeholder could open in.
    fn steps(message: &str) -> usize {
        let scan = Scanner::new(message);
        let mut at = 0;

        while let Some((open, close)) = scan.next(at, message.len()) {
            scan.end(open);
            at = close;
        }

        scan.steps()
    }

    #[test]
    fn the_scan_is_linear_in_the_message() {
        let shapes: [fn(usize) -> String; 5] = [
            |n| "{{a".repeat(n),
            |n| format!("{}x{}", "{{a; b:".repeat(n), "}}".repeat(n)),
            |n| format!("{}x", "{{a; b:".repeat(n)),
            |n| "{{a; b:{{".repeat(n),
            |n| "{{\\}".repeat(n),
        ];

        for shape in shapes {
            let small = steps(&shape(1000));
            let large = steps(&shape(2000));

            assert!(large <= 2 * small + 16, "{small} then {large}");
        }
    }

    #[test]
    fn a_verdict_is_reached_once() {
        let message = format!("{}x{}", "{{a; b:".repeat(100), "}}".repeat(100));
        let scan = Scanner::new(&message);

        assert_eq!(scan.end(0), Some(message.len()));

        let first = scan.steps();

        assert_eq!(scan.end(0), Some(message.len()));
        assert_eq!(scan.end(7), Some(message.len() - 2));
        assert_eq!(scan.steps(), first);
    }
}
