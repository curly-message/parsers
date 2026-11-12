"""The public types and constants, with the documentation a caller reads."""

from __future__ import annotations

import enum
from collections.abc import Callable, Mapping
from types import MappingProxyType
from typing import TYPE_CHECKING, Any, Final, Literal, Protocol, TypedDict, final

from . import _reads


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

MAX_REENTRY: Final[int] = _reads.MAX_REENTRY
"""The most resolutions in progress at once on one thread (section 13).

A resolution host code asks for while another is in progress on its thread
takes a place, and so does a read of an option's value or of a default by any
code but the modifier it was handed to, while that modifier is being asked: a
read of what a modifier kept or handed on, for as long as it renders.
"""

_NO_PROPS: Final[Mapping[str, Any]] = MappingProxyType({})


class ReentryLimitError(RecursionError):
    """Raised by ``resolve`` asked for while ``MAX_REENTRY`` resolutions are in progress.

    It is raised into the host code that asked, on the thread it asked on, and
    the resolution it refuses reads nothing. Host code that lets it pass fails
    as host code that raises does: a modifier's placeholder takes its fallback
    chain. It is a ``RecursionError`` because the bound stands in for the
    interpreter's own limit on a resolution that reaches itself.
    """


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
    so an option the modifier passes over costs nothing. Kept past the
    resolution, it answers what it rendered there, and one first read after the
    resolution returned is the empty string and renders nothing.

    Read during the resolution by any code but the modifier it was handed to,
    while that modifier is being asked, the first read counts as a resolution
    in progress on its thread while it renders. One that would count past
    ``MAX_REENTRY`` renders nothing and is the empty string, the modifier
    being asked on that thread fails whatever it answers, and the next read
    renders afresh. So does a read of a value its own thread is already
    rendering, whatever is in progress, since it asks for what it is part of;
    one of a value another thread is rendering waits for it and answers what
    that render gave. ``ModifierInput.default`` is read the same way.

    A ``value`` given as a callable is asked on the first read; where it
    answers ``None`` the value is the empty string and the next read asks
    again.
    """

    __slots__ = ("_busy", "_call", "_key", "_produce", "_text")

    def __init__(self, key: str, value: str | Callable[[], str | None]) -> None:
        self._key = key
        self._call: object = None
        self._busy: object = None
        if isinstance(value, str):
            self._text: str | None = value
            self._produce: Callable[[], object] | None = None
        else:
            self._text = None
            self._produce = value

    @property
    def key(self) -> str:
        """The option key, unescaped."""
        return self._key

    if TYPE_CHECKING:

        @property
        def value(self) -> str:
            """The option value, rendered where it is first read."""
    else:
        value = property(_reads.read, doc="The option value, rendered where it is first read.")

    def __repr__(self) -> str:
        return f"Option(key={self._key!r})"


@final
class ModifierInput:
    """What a modifier is handed (section 11).

    ``value`` is text. ``options`` are the placeholder's options in source
    order, duplicates kept and the inline default left out. ``props`` is what
    the layers configure under the modifier's own name, composed per property:
    the parser's ``modifier_defaults``, then the call's ``props``, then the
    wrapper's own (section 11.3). It is read-only, and one mapping answers
    every placeholder of a resolution that names the modifier over the same
    layers: a modifier that writes to it raises, and so fails. A resolution
    reads each layer once for each modifier name and wrapper, so a layer
    changed while it runs is read as it was.
    ``locale`` is exactly what the caller passed, ``None`` where it passed
    none. ``default`` walks the fallback chain (section 10) where it is first
    read, so a modifier that answers without it leaves the chain unread. An
    input kept past the resolution answers what the resolution read of it, and
    a ``default`` first read after the resolution returned is the empty string:
    it resolves, reports and runs nothing.
    """

    __slots__ = ("_busy", "_call", "_produce", "_text", "locale", "options", "props", "value")

    value: str
    options: tuple[Option, ...]
    props: Mapping[str, Any]
    locale: object

    def __init__(
        self,
        *,
        value: str,
        options: tuple[Option, ...] = (),
        props: Mapping[str, Any] | None = None,
        locale: object = None,
        default: str | Callable[[], str | None] = "",
    ) -> None:
        self.value = value
        self.options = options
        self.props = _NO_PROPS if props is None else props
        self.locale = locale
        self._call: object = None
        self._busy: object = None
        if isinstance(default, str):
            self._text: str | None = default
            self._produce: Callable[[], object] | None = None
        else:
            self._text = None
            self._produce = default

    if TYPE_CHECKING:

        @property
        def default(self) -> str:
            """The placeholder's default: the first link of its fallback chain that yields text."""
    else:
        default = property(
            _reads.read,
            doc="The placeholder's default: the first link of its fallback chain that yields text.",
        )


Modifier = Callable[[ModifierInput], object]
"""A host-defined modifier (section 11.3).

It answers with a host value, which becomes text the way a payload value does.
``None`` or ``UNDEFINED`` is no answer, and neither is a raise, so the
placeholder takes its fallback chain; a raise is reported as well.
"""


class Parser(Protocol):
    """What ``create_parser`` returns."""

    def resolve(
        self,
        message: object,
        payload: dict[str, Any] | None = None,
        *,
        props: dict[str, dict[str, Any]] | None = None,
        locale: object = None,
        message_id: object = None,
    ) -> str:
        """Resolve a message over a payload. A defect resolves and is reported.

        Never raises where no other resolution is in progress on the same call
        stack. One asked for by host code while ``MAX_REENTRY`` are in progress
        on its thread resolves nothing and raises ``ReentryLimitError`` to
        that code, and one asked for while another is in progress lets pass,
        to that code, the ``RecursionError`` or ``MemoryError`` its walk ran
        out in; either way the placeholder that called the code takes its
        fallback chain where the code lets it pass.

        ``props`` configures modifiers by name, over the parser's
        ``modifier_defaults`` and under a wrapper's own props. ``locale``
        reaches every modifier exactly as passed; ``None`` or ``UNDEFINED``
        is no locale, and ``""`` is one supplied empty. ``message_id`` is
        read by no step, and every report carries it.
        """
        ...
