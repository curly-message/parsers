"""Pure helpers: the character classes, the placeholder scanner and the reads of a span.

Resolution and the tree read a message through the same scanner, so what one
calls a placeholder is what the other does (CST.md, section 2). No regular
expression touches message text: every scan here is a loop over indices.
"""

from __future__ import annotations

from typing import Final

LINE_TERM: Final = ("\n", "\r", "\u2028", "\u2029")
"""The format's ``line-term`` production (section 6, note 1).

The four code points a placeholder holds in no position. The whitespace
class, the scanner and the report excerpt all read the set from here, so an
amendment moves one list.
"""

WHITESPACE: Final = frozenset(
    (
        *LINE_TERM,
        "\t",
        "\x0b",
        "\x0c",
        " ",
        "\xa0",
        "\u1680",
        "\u2000",
        "\u2001",
        "\u2002",
        "\u2003",
        "\u2004",
        "\u2005",
        "\u2006",
        "\u2007",
        "\u2008",
        "\u2009",
        "\u200a",
        "\u202f",
        "\u205f",
        "\u3000",
        "\ufeff",
    )
)
"""The whitespace class section 6 enumerates: the line terminators and twenty-one more.

The host's own notion (``str.isspace``) is no substitute: it follows a Unicode
general category, which has changed membership before.
"""

WHITESPACE_CHARS: Final = "".join(sorted(WHITESPACE))

_TERMINATORS: Final = frozenset(LINE_TERM)

_RESERVED: Final = frozenset(":;{}\\")

# The parts of a placeholder the scan can be inside. Only a value admits a
# nested placeholder (section 6, note 10).
_KEY: Final = 0
_MODIFIER: Final = 1
_OPTION_KEY: Final = 2
_OPTION_VALUE: Final = 3


def is_blank(value: str) -> bool:
    """Whether text holds nothing outside the format's whitespace class."""
    return not value.strip(WHITESPACE_CHARS)


def cancels(character: str) -> bool:
    """Whether a backslash before this character cancels a structural meaning (section 7).

    Both readings are escape sequences; they differ in what removing one leaves
    behind.
    """
    return character in _RESERVED or character in WHITESPACE


def unesc(value: str) -> str:
    """Text as the format reads it, with every escape sequence it claims resolved.

    A backslash consumes the code point after it; before a reserved character
    or whitespace the backslash is dropped, before anything else both stand. A
    backslash with nothing after it is text.
    """
    at = value.find("\\")
    if at < 0:
        return value
    last = len(value) - 1
    parts: list[str] = []
    start = 0
    while 0 <= at < last:
        if cancels(value[at + 1]):
            parts.append(value[start:at])
            start = at + 1
        at = value.find("\\", at + 2)
    parts.append(value[start:])
    return "".join(parts)


class Scanner:
    """Reads one message for the placeholders it derives.

    A verdict is final (section 6, note 11): whether a placeholder derives at a
    position is a function of the message and the position alone, so each is
    recorded in ``verdict`` and never recomputed. The record is what keeps the
    scan linear: note 7 resumes the scan one code point past a brace that
    opened nothing, so attempts overlap, and without it a deeply nested
    message costs time growing faster than any polynomial in its length.
    """

    __slots__ = ("_value", "verdict")

    def __init__(self, value: str) -> None:
        self._value = value
        self.verdict: dict[int, int] = {}
        """Where the placeholder opening at a position closes, or ``-1``: it derives none."""

    def end(self, open_: int) -> int | None:
        """Where a complete placeholder opening at ``open_`` ends, or ``None``."""
        verdict = self.verdict
        known = verdict.get(open_)
        if known is not None:
            return None if known < 0 else known
        value = self._value
        length = len(value)
        terminators = _TERMINATORS
        # The constructs this one is nested in, outermost first. An explicit
        # stack, since nesting is not otherwise limited (note 10) and must not
        # take the host's call stack down with it. Only a value nests, so a
        # construct resumes in its value when the one inside it closes.
        enclosing: list[int] = []
        current = open_
        index = open_ + 2
        part = _KEY
        while True:
            if index >= length:
                break
            character = value[index]
            if character == "\\":
                if index + 1 < length and value[index + 1] in terminators:
                    break
                index += 2
                continue
            if character in terminators:
                break
            if character == ";":
                part = _OPTION_KEY
                index += 1
                continue
            if character == ":":
                # The first colon of a part is its separator and a later one
                # is content (notes 3 and 4), so a part only ever moves on.
                if part == _KEY:
                    part = _MODIFIER
                elif part == _OPTION_KEY:
                    part = _OPTION_VALUE
                index += 1
                continue
            if character == "{" and index + 1 < length and value[index + 1] == "{":
                # A `{{` in a value opens a complete placeholder or the
                # construct around it derives nothing; anywhere else it opens
                # none, and the construct around it derives nothing either.
                if part != _OPTION_VALUE:
                    break
                nested = verdict.get(index)
                if nested is not None:
                    if nested < 0:
                        break
                    index = nested
                    continue
                enclosing.append(current)
                current = index
                index += 2
                part = _KEY
                continue
            if character == "}" and index + 1 < length and value[index + 1] == "}":
                close = index + 2
                verdict[current] = close
                if not enclosing:
                    return close
                current = enclosing.pop()
                index = close
                part = _OPTION_VALUE
                continue
            index += 1
        # Every construct still open when the scan dies is one whose closing
        # pair never arrived, so none of them derives.
        verdict[current] = -1
        for opened in enclosing:
            verdict[opened] = -1
        return None

    def next(self, from_: int, to: int) -> tuple[int, int] | None:
        """Where the next placeholder opening in ``[from_, to)`` opens and ends, or ``None``.

        Both braces of the opening pair lie inside the range, so a lone brace
        at its end opens nothing.
        """
        value = self._value
        brace = value.find("{{", from_, to)
        at = from_
        while brace >= 0:
            escape = value.find("\\", at, brace)
            if escape >= 0:
                # The backslash consumes the code point after it: a brace it
                # claims is text, and a pair beyond it is still the next one.
                at = escape + 2
                if brace < at:
                    brace = value.find("{{", at, to)
                continue
            close = self.end(brace)
            if close is not None:
                return brace, close
            # The braces a refused construct opened are text, and the scan
            # resumes at the very next code point (note 7).
            at = brace + 1
            brace = value.find("{{", at, to)
        return None


def trimmed(value: str, from_: int, to: int) -> tuple[int, int]:
    """The span with its blank padding dropped, empty at ``from_`` where it is all padding.

    Only the padding is read, from either end: an option value holds every
    placeholder nested in it, so reading the whole span at each level would
    read a message nested n levels deep n times over. Whitespace an escape
    claims is text, not padding.
    """
    start = from_
    while start < to and value[start] in WHITESPACE:
        start += 1
    if start == to:
        return from_, from_
    end = to
    while end > start and value[end - 1] in WHITESPACE:
        run = 0
        while end - 2 - run >= from_ and value[end - 2 - run] == "\\":
            run += 1
        if run % 2:
            break
        end -= 1
    return start, end


def separated(
    value: str, from_: int, to: int, separator: str, scan: Scanner | None = None
) -> list[tuple[int, int]]:
    """The spans ``separator`` divides ``[from_, to)`` into (section 6, note 5).

    Every occurrence an escape claims is passed over, and with a scanner so is
    every one a nested placeholder writes. A key, a modifier name and an option
    key hold no placeholder (note 10), so a range inside one is divided by
    escapes alone. There is always at least one span.
    """
    parts: list[tuple[int, int]] = []
    start = from_
    index = from_
    while index < to:
        character = value[index]
        if character == "\\":
            index += 2
            continue
        if (
            scan is not None
            and character == "{"
            and index + 1 < len(value)
            and value[index + 1] == "{"
        ):
            close = scan.end(index)
            if close is not None and close <= to:
                index = close
                continue
        if character == separator:
            parts.append((start, index))
            start = index + 1
        index += 1
    parts.append((start, to))
    return parts


def named(value: str, from_: int, to: int) -> tuple[bool, str]:
    """A span with its padding dropped and unescaped: whether it is empty, and the name."""
    start, end = trimmed(value, from_, to)
    return start == end, unesc(value[start:end])


Segment = tuple[str, int, int]
"""An option a placeholder declares: its key, and where its value is written.

The value is a range, padding dropped, because it is message text: reading it
is work a modifier asks for, not work collecting it does (section 9.4). A
segment that states no value stands for its own key, so the range is the key's.
"""

Parsed = tuple[str | None, str, list[Segment], tuple[int, int] | None]
"""What a placeholder declares: its key, its modifier name, its options and its inline default."""


def parse_placeholder(message: str, open_: int, close: int, scan: Scanner) -> Parsed:
    """What the placeholder spanning ``[open_, close)`` declares.

    Every name arrives unescaped, since a name answers to itself rather than to
    the spelling a message needed to write it; every value arrives unread.
    """
    declaration, *segments = separated(message, open_ + 2, close - 2, ";", scan)
    # The selector's colon is the first one no escape claims, and everything
    # after it is the modifier name (note 3).
    declared_key = separated(message, declaration[0], declaration[1], ":")[0]
    key_empty, key = named(message, declared_key[0], declared_key[1])
    modifier = named(message, min(declared_key[1] + 1, declaration[1]), declaration[1])[1]
    options: list[Segment] = []
    inline_default: tuple[int, int] | None = None
    for from_, to in segments:
        option_key = separated(message, from_, to, ":", scan)[0]
        name = named(message, option_key[0], option_key[1])[1]
        if not name:
            continue
        # The first colon is the separator and every later one is value. An
        # option naming no value stands for itself; one ending at its colon
        # declares the empty string.
        value = (
            trimmed(message, option_key[1] + 1, to)
            if option_key[1] < to
            else trimmed(message, option_key[0], option_key[1])
        )
        # `default` is reserved in lowercase, so a segment is either the
        # inline default or an option.
        if name == "default":
            if inline_default is None:
                inline_default = value
            continue
        options.append((name, value[0], value[1]))
    return None if key_empty else key, modifier, options, inline_default
