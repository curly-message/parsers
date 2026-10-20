"""``create_parser`` and the walk a message resolves in (sections 5, 9, 10 and 13)."""

from __future__ import annotations

import contextlib
from collections.abc import Callable, Generator
from types import FrameType
from typing import Any, Final, cast

from . import _utils
from ._convert import Conversions, WrapperParts, Wrappers, text, wrapper_parts
from ._modifiers import BUILTINS, SELECTIONS, ModifierFailure
from ._types import (
    MAX_NESTING,
    MAX_OUTPUT_LENGTH,
    MAX_READ_LENGTH,
    UNDEFINED,
    ModifierInput,
    Option,
    Parser,
    Report,
    ReportCode,
    ReportOrigin,
)
from ._utils import MISSING, UNFINISHED, Scanner, Segment, own_value, parse_placeholder, unesc

_Job = Generator["_Job", "str | _Text", "str | _Text"]
"""A step of the walk that renders nested text: it yields the job it waits on and gets its text."""

_Result = str | _Job
"""A placeholder's text, or the job that will produce it."""

_Modifier = Callable[[ModifierInput], object]

_MESSAGES: Final[dict[ReportCode, str]] = {
    "unknown-modifier": "A placeholder named a modifier this parser does not know.",
    "failed-modifier": (
        "A modifier could not produce a result, so the placeholder took its fallback chain."
    ),
    "missing-options": (
        "A selection was given no options to select from, so the placeholder took its"
        " fallback chain."
    ),
    "unserializable-value": "A value could not become text, so resolution read it as missing.",
    "missing-locale": (
        "A modifier that depends on a locale was given none, so the placeholder resolved to"
        " the empty string."
    ),
    "nesting-limit": (
        f"A placeholder was nested deeper than {MAX_NESTING} levels, so it took its fallback chain."
    ),
    "output-limit": (
        "A placeholder resolved to text this resolution has no room for, and would have"
        f" carried the output past {MAX_OUTPUT_LENGTH} characters, so it resolved to the empty"
        " string."
    ),
    "read-limit": (
        f"Resolution read more than {MAX_READ_LENGTH} characters of value text, so this"
        " placeholder resolved to the empty string."
    ),
}

_LIMITS: Final[dict[ReportCode, int | None]] = {
    "unknown-modifier": None,
    "failed-modifier": None,
    "missing-options": None,
    "unserializable-value": None,
    "missing-locale": None,
    "nesting-limit": MAX_NESTING,
    "output-limit": MAX_OUTPUT_LENGTH,
    "read-limit": MAX_READ_LENGTH,
}

# The axis is a property of the code rather than of the site that reported it.
# A modifier that could not produce a result was handed the caller's value, or
# is the caller's own, so what it reports is the payload's.
_ORIGINS: Final[dict[ReportCode, ReportOrigin]] = {
    "unknown-modifier": "message",
    "failed-modifier": "payload",
    "missing-options": "message",
    "unserializable-value": "payload",
    "missing-locale": "payload",
    "nesting-limit": "message",
    "output-limit": "limit",
    "read-limit": "limit",
}


class _Text:
    """Rendered text held as the pieces it is made of, and made a ``str`` where it is read.

    A level that only passes a nested result on would otherwise copy it, so a
    message nested n levels deep would copy its innermost text n times.
    """

    __slots__ = ("length", "pieces")

    def __init__(self, pieces: list[str | _Text], length: int) -> None:
        self.pieces = pieces
        self.length = length


def _drive(top: _Job) -> str | _Text:
    """Run a job and every job it waits on, on a stack of suspended generators.

    However deep a message nests, the host's call stack holds this loop and
    the one generator it is resuming: a nested render is a job pushed here,
    never a call.
    """
    stack = [top]
    sent: Any = None
    while True:
        try:
            job = stack[-1].send(sent)
        except StopIteration as finished:
            stack.pop()
            if not stack:
                result: str | _Text = finished.value
                return result
            sent = finished.value
            continue
        stack.append(job)
        sent = None


def _text(found: str | _Text) -> str:
    """The text as one ``str``, its pieces collected on a stack rather than by recursion."""
    if isinstance(found, str):
        return found
    pieces: list[str] = []
    stack = [iter(found.pieces)]
    while stack:
        piece = next(stack[-1], None)
        if piece is None:
            stack.pop()
        elif isinstance(piece, str):
            pieces.append(piece)
        else:
            stack.append(iter(piece.pieces))
    return "".join(pieces)


def _finish(result: _Result) -> str:
    return result if isinstance(result, str) else _text(_drive(result))


class _Chain:
    """What a placeholder's fallback chain reads, kept for where the chain is walked."""

    __slots__ = ("close", "depth", "inline", "key", "open", "parts", "value_text")

    def __init__(  # noqa: PLR0917
        self,
        open_: int,
        close: int,
        depth: int,
        key: str | None,
        parts: WrapperParts | None,
        value_text: str | None,
        inline: tuple[int, int] | None,
    ) -> None:
        self.open = open_
        self.close = close
        self.depth = depth
        self.key = key
        self.parts = parts
        self.value_text = value_text
        self.inline = inline


class _Walk:
    """One resolution: the message, what it reads, and the budgets it spends (section 13).

    Every budget and record belongs to the resolution, so one begun inside
    another, by host code it called, owns its own.
    """

    __slots__ = (
        "closed",
        "conversions",
        "message_id",
        "on_report",
        "output",
        "payload",
        "read",
        "recognize",
        "scan",
        "source",
        "wrappers",
    )

    def __init__(
        self,
        source: str,
        scan: Scanner,
        *,
        payload: object,
        message_id: object,
        on_report: Callable[[Report], object] | None,
        recognize: bool,
        conversions: Conversions,
    ) -> None:
        self.source = source
        self.scan = scan
        self.payload = payload
        self.message_id = message_id
        self.on_report = on_report
        self.recognize = recognize
        self.conversions = conversions
        self.wrappers: Wrappers = {}
        self.output = MAX_OUTPUT_LENGTH
        self.read = MAX_READ_LENGTH
        # A modifier may keep what it was handed and read it after the
        # resolution returned; by then the budgets are spent and the records
        # closed, so the walk answers nothing.
        self.closed = False

    def report(self, code: ReportCode, open_: int, close: int) -> None:
        handler = self.on_report
        if handler is None:
            return
        # Reporting is an observation, not a step of the resolution: a handler
        # that fails loses its report, and the message still renders.
        with contextlib.suppress(Exception):
            handler(
                Report(
                    code=code,
                    origin=_ORIGINS[code],
                    message=_MESSAGES[code],
                    text=_utils.excerpt(self.source, open_, close),
                    message_id=self.message_id,
                    limit=_LIMITS[code],
                )
            )

    def own(self, container: object, name: str, open_: int, close: int) -> object:
        # A lookup runs host code where a stored key collides with the name, and
        # an entry that refuses to be read is no entry, but a reported one.
        try:
            return own_value(container, name)
        except Exception:
            self.report("unserializable-value", open_, close)
            return MISSING

    def payload_text(self, declared: object, open_: int, close: int) -> str | None:
        # Every character of value text is spent whether or not any of it
        # reaches the output.
        found = text(declared, self.conversions)
        if found is None:
            if declared is not MISSING and declared is not UNDEFINED:
                self.report("unserializable-value", open_, close)
            return None
        self.read -= len(found)
        return found

    def modifier_for(self, name: str) -> _Modifier | None:
        return BUILTINS.get(name)

    def _head(self, open_: int, close: int, depth: int) -> _Result:
        """Resolve a placeholder as far as it can without rendering nested text.

        That is the whole of most placeholders. The rest, a modifier call, is a
        job for the trampoline, and so is a fallback chain whose inline default
        holds a placeholder.
        """
        # The budget is tested before this placeholder reads, so one that found
        # a budget and spent it past the limit resolved, and this one pays.
        if self.read < 0:
            self.report("read-limit", open_, close)
            return ""
        key, name, segments, inline = parse_placeholder(self.source, open_, close, self.scan)
        entry = MISSING if key is None else self.own(self.payload, key, open_, close)
        # The payload's root `default` is the fallback itself, never configuration.
        parts = wrapper_parts(entry, self.wrappers) if self.recognize and key != "default" else None
        value_text = self.payload_text(entry if parts is None else parts[0], open_, close)
        if depth > MAX_NESTING:
            self.report("nesting-limit", open_, close)
            return self._fallback(_Chain(open_, close, depth, key, parts, value_text, inline))
        modifier = self.modifier_for(name) if name else None
        if name and modifier is None:
            self.report("unknown-modifier", open_, close)
            return self._fallback(_Chain(open_, close, depth, key, parts, value_text, inline))
        # What the placeholder declares makes this error, so it does not wait
        # on the value; one a host registered under the name is no selection.
        if (
            key is not None
            and not segments
            and name in SELECTIONS
            and modifier is BUILTINS.get(name)
        ):
            self.report("missing-options", open_, close)
            return self._fallback(_Chain(open_, close, depth, key, parts, value_text, inline))
        if value_text is None:
            return self._fallback(_Chain(open_, close, depth, key, parts, value_text, inline))
        if not name and not segments:
            return value_text
        chain = _Chain(open_, close, depth, key, parts, value_text, inline)
        return self._tail(chain, modifier or BUILTINS["eq"], value_text, segments)

    def _fallback(self, chain: _Chain) -> _Result:
        """The first link of the chain that yields text (section 10), read link by link."""
        open_, close = chain.open, chain.close
        if chain.parts is not None:
            found = self.payload_text(chain.parts[1], open_, close)
            if found is not None:
                return found
        # A placeholder naming `default` has already read the entry as its
        # value, and one entry is read once.
        if chain.key == "default":
            found = chain.value_text
        else:
            found = self.payload_text(self.own(self.payload, "default", open_, close), open_, close)
        if found is not None:
            return found
        if chain.inline is None:
            return ""
        return self._range(chain.inline[0], chain.inline[1], chain.depth + 1)

    def _range(self, from_: int, to: int, depth: int) -> _Result:
        """A span of the message's own text, its placeholders resolved one level deeper."""
        found = self.scan.next(from_, to)
        if found is None:
            return unesc(self.source[from_:to])
        return self._render(from_, to, depth, found)

    def _render(self, from_: int, to: int, depth: int, found: tuple[int, int] | None) -> _Job:
        parts: list[str | _Text] = []
        length = 0
        nested = False
        source = self.source
        at = from_
        while found is not None:
            open_, close = found
            between = unesc(source[at:open_])
            parts.append(between)
            length += len(between)
            result = self._head(open_, close, depth)
            if isinstance(result, str):
                resolved: str | _Text = result
            else:
                resolved = yield result
                if not isinstance(resolved, str):
                    nested = True
            size = len(resolved) if isinstance(resolved, str) else resolved.length
            # Only what the output carries is counted, and a nested result
            # reaches it through the one around it, so the charge is made where
            # the walk meets the message's own top level.
            if depth > 1 or size <= self.output:
                if depth == 1:
                    self.output -= size
                parts.append(resolved)
                length += size
            else:
                self.report("output-limit", open_, close)
            at = close
            found = self.scan.next(at, to)
        rest = unesc(source[at:to])
        parts.append(rest)
        # A flat message is made text at once, so it never builds the tree it
        # has no use for. Below the top level nothing has met the output limit
        # yet, so a piece the limit will refuse is not copied there.
        if not nested and depth == 1:
            return "".join(cast("list[str]", parts))
        return _Text(parts, length + len(rest))

    def _rendered(self, from_: int, to: int, depth: int) -> Callable[[], str]:
        # An option value is rendered where a modifier first reads it, on a
        # trampoline of its own: a modifier is host code the walk called, so
        # the render it asks for cannot be a job of the walk around it.
        def render() -> str:
            return "" if self.closed else _finish(self._range(from_, to, depth))

        return render

    def _tail(
        self, chain: _Chain, modifier: _Modifier, value_text: str, segments: list[Segment]
    ) -> _Job:
        """Ask the modifier, and take the chain where it answers nothing (section 11)."""
        read: list[str] = []

        def default() -> str:
            if self.closed:
                return ""
            if not read:
                read.append(_finish(self._fallback(chain)))
            return read[0]

        depth = chain.depth + 1
        options = tuple(Option(k, self._rendered(a, b, depth)) for k, a, b in segments)
        config = ModifierInput(value=value_text, options=options, default=default)
        try:
            try:
                answer = modifier(config)
            except Exception as failure:
                code: ReportCode = (
                    failure.code if type(failure) is ModifierFailure else "failed-modifier"
                )
                self.report(code, chain.open, chain.close)
                # A declared default stands in for a value the modifier cannot
                # read, never for the locale it would have formatted in.
                if code == "missing-locale":
                    return ""
                answered = None
            else:
                answered = self.answer_text(answer, chain.open, chain.close)
        except Exception:
            # The containment's own work failed, which only an exhausted host
            # stack does.
            return ""
        if answered is not None:
            return answered
        if read:
            return read[0]
        result = self._fallback(chain)
        return result if isinstance(result, str) else (yield result)

    def answer_text(self, answer: object, open_: int, close: int) -> str | None:
        # An answer is a host value converted like a payload value; nothing at
        # all is no answer, and neither is one no conversion describes, which
        # is reported. It is the placeholder's result rather than text read, so
        # it spends nothing of the read budget.
        if answer is None or answer is UNDEFINED:
            return None
        found = text(answer, self.conversions)
        if found is None:
            self.report("unserializable-value", open_, close)
        return found


class _Parser:
    __slots__ = ("_on_report", "_recognize")

    def __init__(self, on_report: object, recognize_wrappers: object) -> None:
        self._on_report: Callable[[Report], object] | None = (
            on_report if callable(on_report) else None
        )
        # Recognition is on where the caller says nothing (section 4.1).
        self._recognize = recognize_wrappers if type(recognize_wrappers) is bool else True

    def resolve(
        self,
        message: object,
        payload: dict[str, Any] | None = None,
        *,
        message_id: object = None,
    ) -> str:
        try:
            return self._resolve(message, payload, message_id)
        except UNFINISHED as failure:
            # Every call into host code is contained where it is made, so what
            # reaches here is the interpreter's stack or memory running out in
            # the walk, or in a containment's own work. A resolution host code
            # asked for while another walks on the same call stack lets it pass
            # to that code, where the placeholder that called it takes its
            # fallback chain (section 13); the one the caller asked for
            # answers nothing. The stack is read here, once a walk has run
            # out, rather than recorded by every walk, and a thread or a fiber
            # runs on a stack of its own. It is read off the failure, since a
            # call made here could run out on the same stack.
            tb = failure.__traceback__
            frame: FrameType | None = None if tb is None else tb.tb_frame.f_back
            while frame is not None:
                if frame.f_code is _WALK:
                    raise
                frame = frame.f_back
            return ""
        except Exception:
            return ""

    def _resolve(self, message: object, payload: object, message_id: object) -> str:
        conversions: Conversions = {}
        # The message becomes text before anything reads it, and a message no
        # conversion describes is nothing to resolve (section 4).
        source = text(message, conversions)
        if source is None:
            return ""
        scan = Scanner(source)
        found = scan.next(0, len(source))
        if found is None:
            return unesc(source)
        walk = _Walk(
            source,
            scan,
            payload=payload,
            message_id=None if message_id is UNDEFINED else message_id,
            on_report=self._on_report,
            recognize=self._recognize,
            conversions=conversions,
        )
        try:
            return _text(_drive(walk._render(0, len(source), 1, found)))
        finally:
            walk.closed = True


_WALK: Final = _Parser._resolve.__code__
"""What a resolution in progress has on its call stack."""


def create_parser(
    *,
    on_report: Callable[[Report], object] | None = None,
    recognize_wrappers: bool = True,
) -> Parser:
    """A parser for the Curly Message Format, version 3.

    ``on_report`` receives every report a resolution makes (section 14.3), and
    is the only channel the parser reports through: a handler that raises loses
    that report and nothing else. ``recognize_wrappers=False`` reads every
    payload entry as a value, which is where a caller holding untrusted data
    passes it (section 4.1). An option of the wrong type is read as not given.
    """
    return _Parser(on_report, recognize_wrappers)
