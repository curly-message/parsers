"""The public types and constants, with the documentation a caller reads."""

from __future__ import annotations

import enum
from typing import Final, Literal, TypedDict


class Undefined(enum.Enum):
    """The host's undefined (specification, section 3).

    ``None`` is the host's null: it is a value, present, and it converts to
    ``"None"``. A value nobody supplied is spelled by leaving the entry out, or
    by ``UNDEFINED``, which means the same: an entry holding it is not named, a
    wrapper whose ``value`` is ``UNDEFINED`` has no value, and a modifier
    answering ``UNDEFINED`` answered nothing.
    """

    UNDEFINED = "UNDEFINED"

    def __repr__(self) -> str:
        return "UNDEFINED"

    def __bool__(self) -> bool:
        return False


UNDEFINED: Final = Undefined.UNDEFINED
"""The one member of :class:`Undefined`."""

MAX_OUTPUT_LENGTH: Final[int] = 100000
"""The most code points a resolution writes from placeholders (section 13)."""

MAX_READ_LENGTH: Final[int] = 100000
"""The most code points of value text a resolution reads (section 13)."""

MAX_CONVERSION_NODES: Final[int] = 100000
"""The most nodes one conversion of a structured value visits (section 13)."""

MAX_NESTING: Final[int] = 8
"""The deepest level at which a placeholder resolves (section 13)."""


class CstText(TypedDict):
    """Characters that carry no structural meaning where they stand.

    Every span of the tree is ``[start, end)`` in code points of the message.
    """

    type: Literal["text"]
    start: int
    end: int


class CstEscape(TypedDict):
    """A backslash and the code point it consumes (section 7).

    ``cancels`` says which reading the pair takes: a structural meaning
    cancelled, so removing the sequence leaves the character alone, or a
    backslash that denotes itself, so both characters stand.
    """

    type: Literal["escape"]
    start: int
    end: int
    cancels: bool


class CstSpace(TypedDict):
    """Blank padding a name is read without (section 8)."""

    type: Literal["space"]
    start: int
    end: int


class CstPunctuation(TypedDict):
    """A delimiter: the pair that opens or closes, or a ``:`` or ``;`` that divides."""

    type: Literal["open", "close", "separator"]
    start: int
    end: int


class CstName(TypedDict):
    """A key, a modifier name or an option key: ``name`` is the span unescaped.

    A name holds no placeholder, and an empty one still has a position: a
    placeholder that names no key carries a ``key`` node of no width.
    """

    type: Literal["key", "modifier", "option-key"]
    start: int
    end: int
    name: str
    nodes: list[CstText | CstEscape]


class CstOptionValue(TypedDict):
    """An option's value: message text, so a placeholder written in it derives."""

    type: Literal["option-value"]
    start: int
    end: int
    nodes: list[CstText | CstEscape | CstPlaceholder]


class CstPlaceholder(TypedDict):
    """A construct section 6 derives as a placeholder, and what it is made of."""

    type: Literal["placeholder"]
    start: int
    end: int
    nodes: list[CstPunctuation | CstSpace | CstName | CstOptionValue]


class CstMessage(TypedDict):
    """A whole message: text and placeholders, and nothing else (section 9.1).

    A ``{{ ... }}`` construct the grammar does not derive is text here, exactly
    as it is to resolution. A message that is not text carries no parts.
    """

    type: Literal["message"]
    start: int
    end: int
    nodes: list[CstText | CstEscape | CstPlaceholder]


CstNode = (
    CstMessage
    | CstPlaceholder
    | CstName
    | CstOptionValue
    | CstText
    | CstEscape
    | CstSpace
    | CstPunctuation
)
"""Any node of the tree."""
