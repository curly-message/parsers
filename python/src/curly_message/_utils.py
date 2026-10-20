"""Pure helpers: the character classes, the placeholder scanner and the reads of a span.

Resolution and the tree read a message through the same scanner, so what one
calls a placeholder is what the other does (CST.md, section 2). No regular
expression touches message text: every scan here is a loop over indices.
"""

from __future__ import annotations

import enum
import functools
import json
from typing import Final, cast

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


class _Missing(enum.Enum):
    MISSING = "MISSING"

    def __repr__(self) -> str:
        return "MISSING"


MISSING: Final = _Missing.MISSING
"""No entry at all, which a container answers apart from an entry holding ``UNDEFINED``."""


def own_value(container: object, name: str) -> object:
    """The entry a container holds under ``name``, or ``MISSING``.

    Only ``dict`` storage holds entries, read through ``dict``'s own lookup: a
    subclass's ``__getitem__``, ``get`` or ``__missing__`` never answers, and a
    mapping of any other type holds nothing (section 9.2). The lookup itself
    can still run host code, a colliding stored key's ``__eq__``, so the
    caller contains it.
    """
    if not issubclass(type(container), dict):
        return MISSING
    return dict.get(cast(dict[str, object], container), name, MISSING)


def own_items(container: object) -> list[tuple[str, object]]:
    """The entries a container holds under a name, in its order.

    A stored key names an entry when it is a ``str``, read through ``str``'s own
    storage so no override runs; any other key names nothing. Enumerating
    ``dict`` storage runs no host code.
    """
    if not issubclass(type(container), dict):
        return []
    return [
        (str.__str__(key), value)
        for key, value in dict.items(cast(dict[object, object], container))
        if issubclass(type(key), str)
    ]


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


MAX_REPORTED_LENGTH: Final = 120


@functools.cache
def _escape_table() -> dict[int, str]:
    """Every code point a report must not carry raw, mapped to its ``\\u`` escape.

    The format's own terminators come from ``LINE_TERM``. A consumer that
    splits a report into lines may use the host's notion instead, so every code
    point ``str.splitlines`` breaks on is escaped too, read off the host rather
    than spelled out; a lone surrogate is escaped so that a handler encoding to
    UTF-8 cannot fail on one.
    """
    breaking = [chr(code) for code in range(0x3000) if len(f"a{chr(code)}b".splitlines()) > 1]
    surrogates = map(chr, range(0xD800, 0xE000))
    return {ord(c): f"\\u{ord(c):04x}" for c in (*LINE_TERM, *breaking, *surrogates)}


def excerpt(source: str, open_: int, close: int) -> str:
    """The text a report carries for the span ``[open_, close)`` of a message.

    Only as much of the span is copied as the cut keeps: a message nested deep
    holds every inner placeholder inside each outer one, and copying whole
    spans would cost the square of its depth. The cut falls at 120 code points,
    which never splits a character, and the result is escaped so that it holds
    no line terminator.
    """
    span = source[open_ : min(close, open_ + MAX_REPORTED_LENGTH + 1)]
    if len(span) > MAX_REPORTED_LENGTH:
        span = span[:MAX_REPORTED_LENGTH] + "..."
    return json.dumps(span, ensure_ascii=False)[1:-1].translate(_escape_table())
