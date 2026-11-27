"""A reading of section 13 for the messages the cases of kept reads write.

RUNNER.md has a port check that ``reentry-over-limit-beneath-a-defined-modifier``
fails over an implementation in which a modifier the format defines is not
current, against a reading that does not take section 13's permission to count
an own read while an option value or default of another placeholder, begun
beneath the modifier's call, is in production. ``curly_message_parser`` takes it, and
a reading that takes it answers ``<empty>`` whichever modifier it holds
current, so the runner's own tests check against this reading instead, as the
JavaScript runner's do against the one its tests carry.

It reads placeholders of a key, a host-defined modifier or the format's own
``ne``, and options, and nothing else the format holds: what a counted read
is, the bound it meets, the refusal of a read whose own production is in
progress, and which modifier is current where a read is made.
"""

from __future__ import annotations

from collections.abc import Callable, Mapping
from dataclasses import dataclass, field

from runner import Call, Resolution, Resolved


@dataclass(eq=False)
class Placeholder:
    key: str
    modifier: str | None
    options: list[tuple[str, list[str | Placeholder]]] = field(default_factory=list)
    fallback: list[str | Placeholder] = field(default_factory=list)


@dataclass(frozen=True)
class _Option:
    key: str
    value: str


def _close(text: str, start: int) -> int:
    # The end of the placeholder opening at ``start``, past its closing braces.
    depth = 0
    at = start
    while at < len(text):
        if text.startswith("{{", at):
            depth += 1
            at += 2
        elif text.startswith("}}", at):
            depth -= 1
            at += 2
            if depth == 0:
                return at
        else:
            at += 1
    raise ValueError(f"An unclosed placeholder in {text}.")


def _segments(text: str) -> list[str]:
    # The text split at each ``;`` no nested placeholder holds.
    found: list[str] = []
    start = at = 0
    while at < len(text):
        if text.startswith("{{", at):
            at = _close(text, at)
        else:
            if text[at] == ";":
                found.append(text[start:at])
                start = at + 1
            at += 1
    return [*found, text[start:]]


def parse(text: str) -> list[str | Placeholder]:
    parts: list[str | Placeholder] = []
    start = 0
    at = text.find("{{")
    while at != -1:
        end = _close(text, at)
        selector, *rest = _segments(text[at + 2 : end - 2])
        key, _, modifier = selector.strip().partition(":")
        written = [
            (segment[: segment.index(":")].strip(), parse(segment[segment.index(":") + 1 :]))
            for segment in rest
            if segment.strip()
        ]
        fallback = next((value for name, value in written if name == "default"), [])
        options = [(name, value) for name, value in written if name != "default"]
        parts += [text[start:at], Placeholder(key, modifier or None, options, fallback)]
        start = end
        at = text.find("{{", start)
    return [*parts, text[start:]]


class Reading:
    """A resolver keeping ``bound`` counted reads, its count the thread's.

    ``resolutions`` has the bound count a resolution asked for inside another
    as well, as section 13 lets it; ``defined_current`` left false takes only
    a host-defined modifier as current, the defect the case is held to
    telling apart.
    """

    def __init__(
        self, bound: int, *, resolutions: bool = False, defined_current: bool = True
    ) -> None:
        self._limit = bound
        self._resolutions = resolutions
        self._defined_current = defined_current
        self._counted = 0
        self._active = 0
        self._current: list[Placeholder] = []

    def resolve(self, resolution: Resolution) -> Resolved:
        inside = self._resolutions and self._active > 0
        if inside and self._counted + 1 > self._limit:
            raise RecursionError("A resolution past the bound.")
        self._active += 1
        self._counted += 1 if inside else 0
        try:
            return self._walk(resolution)
        finally:
            self._active -= 1
            self._counted -= 1 if inside else 0

    def _walk(self, resolution: Resolution) -> Resolved:
        reports: list[Mapping[str, object]] = []
        values = resolution.payload if isinstance(resolution.payload, dict) else {}
        modifiers = resolution.modifiers or {}

        def render(parts: list[str | Placeholder]) -> str:
            return "".join(part if isinstance(part, str) else resolve(part) for part in parts)

        def reader(placeholder: Placeholder) -> Callable[[], str]:
            # A read of a placeholder's default: the walk's where that
            # placeholder's modifier is current and the default is not being
            # produced, and counted otherwise.
            text: list[str] = []
            producing = 0

            def read() -> str:
                nonlocal producing
                if text:
                    return text[0]
                if producing:
                    return ""
                own = bool(self._current) and self._current[-1] is placeholder
                if not own and self._counted + 1 > self._limit:
                    return ""
                self._counted += 0 if own else 1
                producing += 1
                try:
                    text.append(render(placeholder.fallback))
                finally:
                    producing -= 1
                    self._counted -= 0 if own else 1
                return text[0]

            return read

        def resolve(placeholder: Placeholder) -> str:
            key, modifier = placeholder.key, placeholder.modifier
            # The format's own comparison, current while it produces the
            # option it selected, as a host-defined modifier is while it runs.
            if modifier == "ne":
                selected = next(
                    (value for name, value in placeholder.options if name != str(values.get(key))),
                    None,
                )
                if key not in values or selected is None:
                    return render(placeholder.fallback)
                if self._defined_current:
                    self._current.append(placeholder)
                try:
                    return render(selected)
                finally:
                    if self._defined_current:
                        self._current.pop()
            if modifier is not None and modifier not in modifiers:
                reports.append(
                    {"code": "unknown-modifier", "origin": "message", "id": resolution.id}
                )
                return render(placeholder.fallback)
            if key not in values:
                return render(placeholder.fallback)
            if modifier is None:
                return str(values[key])
            answer: object = None
            failed = False
            self._current.append(placeholder)
            try:
                options = [_Option(name, render(value)) for name, value in placeholder.options]
                answer = modifiers[modifier](
                    Call(str(values[key]), options, {}, None, reader(placeholder))
                )
            except Exception:
                failed = True
            finally:
                self._current.pop()
            if failed:
                reports.append(
                    {"code": "failed-modifier", "origin": "payload", "id": resolution.id}
                )
                return render(placeholder.fallback)
            return str(answer)

        return Resolved(render(parse(str(resolution.message))), reports=reports)
