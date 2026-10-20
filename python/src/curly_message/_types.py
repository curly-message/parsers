"""The public types and constants, with the documentation a caller reads."""

from __future__ import annotations

import enum
from collections.abc import Callable
from typing import Any, Final, Literal, Protocol, TypedDict, final


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


Level = Literal["core", "intl", "extensions"]
"""A conformance level of section 2."""

ReportCode = Literal[
    "unknown-modifier",
    "failed-modifier",
    "missing-options",
    "unserializable-value",
    "missing-locale",
    "nesting-limit",
    "output-limit",
    "read-limit",
]
"""What a report names: the closed vocabulary of section 14.2."""

ReportOrigin = Literal["message", "payload", "limit"]
"""Which input a report holds responsible: the message, the payload or a limit."""


@final
class Report:
    """A condition a resolution met and resolved past (section 14.3).

    Reports leave through ``on_report`` and nowhere else. ``text`` is the
    placeholder that reported, cut at 120 code points and escaped, so no line
    terminator a message holds reaches whatever the handler writes to; it is
    message text, never a payload value. ``message_id`` is the id the
    resolution was given, ``None`` where it was given none, and ``limit`` is the
    limit a limit code reached.
    """

    __slots__ = ("code", "limit", "message", "message_id", "origin", "text")

    code: ReportCode
    origin: ReportOrigin
    message: str
    text: str
    message_id: object
    limit: int | None

    def __init__(
        self,
        *,
        code: ReportCode,
        origin: ReportOrigin,
        message: str,
        text: str,
        message_id: object = None,
        limit: int | None = None,
    ) -> None:
        object.__setattr__(self, "code", code)
        object.__setattr__(self, "origin", origin)
        object.__setattr__(self, "message", message)
        object.__setattr__(self, "text", text)
        object.__setattr__(self, "message_id", message_id)
        object.__setattr__(self, "limit", limit)

    def __setattr__(self, name: str, value: object) -> None:
        raise AttributeError(f"Report is read-only: cannot set {name!r}")

    def __delattr__(self, name: str) -> None:
        raise AttributeError(f"Report is read-only: cannot delete {name!r}")

    def _fields(self) -> tuple[object, ...]:
        return (self.code, self.origin, self.message, self.text, self.message_id, self.limit)

    def __eq__(self, other: object) -> bool:
        if type(other) is not Report:
            return NotImplemented
        return self._fields() == other._fields()

    def __hash__(self) -> int:
        return hash(self._fields())

    def __repr__(self) -> str:
        return (
            f"Report(code={self.code!r}, origin={self.origin!r}, message={self.message!r}, "
            f"text={self.text!r}, message_id={self.message_id!r}, limit={self.limit!r})"
        )


@final
class Option:
    """An option a placeholder declares, as a modifier receives it (section 9.4).

    ``value`` is message text, and a placeholder written in it resolves only
    where something reads it: the value is rendered on the first read and kept,
    so an option the modifier passes over costs nothing.
    """

    __slots__ = ("_key", "_render", "_value")

    def __init__(self, key: str, value: str | Callable[[], str]) -> None:
        self._key = key
        self._value: str | None = None
        self._render: Callable[[], str] | None = None
        if isinstance(value, str):
            self._value = value
        else:
            self._render = value

    @property
    def key(self) -> str:
        """The option key, unescaped."""
        return self._key

    @property
    def value(self) -> str:
        """The option value, rendered where it is first read."""
        if self._value is None:
            render = self._render
            self._value = render() if render is not None else ""
            self._render = None
        return self._value

    def __repr__(self) -> str:
        return f"Option(key={self._key!r})"


@final
class ModifierInput:
    """What a modifier is handed (section 11).

    ``value`` is text. ``options`` are the placeholder's options in source
    order, duplicates kept and the inline default left out. ``default`` walks
    the fallback chain (section 10) where it is first read, so a modifier that
    answers without it leaves the chain unread.
    """

    __slots__ = ("_default", "_fallback", "locale", "options", "props", "value")

    value: str
    options: tuple[Option, ...]
    props: dict[str, Any]
    locale: object

    def __init__(
        self,
        *,
        value: str,
        options: tuple[Option, ...] = (),
        props: dict[str, Any] | None = None,
        locale: object = None,
        default: str | Callable[[], str] = "",
    ) -> None:
        self.value = value
        self.options = options
        self.props = {} if props is None else props
        self.locale = locale
        self._default: str | None = None
        self._fallback: Callable[[], str] | None = None
        if isinstance(default, str):
            self._default = default
        else:
            self._fallback = default

    @property
    def default(self) -> str:
        """The placeholder's default: the first link of its fallback chain that yields text."""
        if self._default is None:
            fallback = self._fallback
            self._default = fallback() if fallback is not None else ""
            self._fallback = None
        return self._default


class Parser(Protocol):
    """What ``create_parser`` returns."""

    def resolve(
        self,
        message: object,
        payload: dict[str, Any] | None = None,
        *,
        message_id: object = None,
    ) -> str:
        """Resolve a message over a payload. Never raises: a defect resolves and is reported."""
        ...
