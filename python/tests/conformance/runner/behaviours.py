"""The catalogue of host-defined modifiers the set's README names, each a function of a ``Call``."""

from __future__ import annotations

import json
import math
import re
from collections.abc import Callable, Mapping
from typing import Final, Protocol

from .model import OMIT, Behaviour, Call, Resolution, Resolved

_UPPER = str.maketrans("abcdefghijklmnopqrstuvwxyz", "ABCDEFGHIJKLMNOPQRSTUVWXYZ")

_SURROGATE: Final = re.compile("[\ud800-\udfff]")

_INDEX: Final = re.compile("0|[1-9][0-9]*")


def _units(name: str) -> bytes:
    # The README sorts names as ECMAScript sorts strings, by UTF-16 code
    # units, which orders a character outside the basic plane before U+E000.
    return name.encode("utf-16-be", "surrogatepass")


def _number(value: float) -> str:
    # ECMAScript's Number::toString over the shortest digits that round-trip,
    # which are the ones ``repr`` writes: a whole number without a fraction,
    # and an exponent only from 1e21 up or below 1e-6.
    if not math.isfinite(value):
        return "null"
    if value == 0:
        return "0"
    sign = "-" if value < 0 else ""
    mantissa, _, exponent = repr(abs(value)).partition("e")
    whole, _, fraction = mantissa.partition(".")
    digits = (whole + fraction.rstrip("0")).lstrip("0")
    point = len(whole) + int(exponent or 0) - (len(whole + fraction.rstrip("0")) - len(digits))
    if len(digits) <= point <= 21:
        return sign + digits + "0" * (point - len(digits))
    if 0 < point <= 21:
        return sign + digits[:point] + "." + digits[point:]
    if -6 < point <= 0:
        return sign + "0." + "0" * -point + digits
    shown = digits[0] + ("." + digits[1:] if len(digits) > 1 else "")
    return f"{sign}{shown}e{'+' if point > 0 else '-'}{abs(point - 1)}"


def _text(value: str) -> str:
    return _SURROGATE.sub(
        lambda match: f"\\u{ord(match.group()):04x}", json.dumps(value, ensure_ascii=False)
    )


def _keys(value: Mapping[object, object]) -> list[object]:
    # ECMAScript orders an object's own keys that are array indices first,
    # ascending, then the others as they were added.
    indices = [key for key in value if isinstance(key, str) and _INDEX.fullmatch(key)]
    return [
        *sorted(indices, key=lambda key: int(str(key))),
        *(key for key in value if key not in indices),
    ]


def _spelled(value: object, undefined: object, held: frozenset[int] = frozenset()) -> str | None:
    """``value`` as ECMAScript's ``JSON.stringify`` writes it; None where it writes nothing."""
    if value is undefined:
        return None
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, int):
        return str(value)
    if isinstance(value, float):
        return _number(value)
    if isinstance(value, str):
        return _text(value)
    if id(value) in held:
        raise ValueError("The props hold a cycle, which JSON.stringify refuses.")
    inside = held | {id(value)}
    if isinstance(value, list | tuple):
        return "[" + ",".join(_spelled(entry, undefined, inside) or "null" for entry in value) + "]"
    if isinstance(value, Mapping):
        members = ((key, _spelled(value[key], undefined, inside)) for key in _keys(value))
        return (
            "{"
            + ",".join(f"{_text(str(key))}:{spelled}" for key, spelled in members if spelled)
            + "}"
        )
    raise TypeError(f"JSON.stringify writes no {type(value).__name__}.")


def _raise(call: Call) -> object:
    raise RuntimeError("The raise behaviour raised.")


def behaviours(undefined: object = OMIT) -> Mapping[str, Behaviour]:
    """The catalogue, for an adapter whose host reads ``undefined`` as no answer.

    ``nothing`` answers with the adapter's own statement of the host's
    undefined, so the runner never decides what counts as no answer; with no
    statement, it answers ``None``, what a Python function without an answer
    returns.
    """
    nothing = None if undefined is OMIT else undefined

    def locale(call: Call) -> object:
        return "none" if call.locale is None or call.locale is undefined else call.locale

    return {
        "upper": lambda call: call.value.translate(_UPPER),
        "echo": lambda call: call.value,
        "empty": lambda call: "",
        "nothing": lambda call: nothing,
        "raise": _raise,
        "default": lambda call: call.default(),
        "options": lambda call: ",".join(f"{option.key}={option.value}" for option in call.options),
        "props": lambda call: ",".join(
            f"{name}={_spelled(call.props[name], undefined)}"
            for name in sorted(call.props, key=_units)
        ),
        "locale": locale,
        "object": lambda call: {"answer": call.value},
    }


class Resolving(Protocol):
    """What ``nest`` asks: an adapter, or anything that resolves as one does."""

    @property
    def resolve(self) -> Callable[[Resolution], object]: ...


NESTED: Final = "{{x:take; default:F}}"
"""The message ``nest`` asks a resolution of."""

NESTED_TAKING: Final = "{{x:taking; default:F}}"
"""The message ``keep-nest`` asks a resolution of."""

NESTED_DEFINED: Final = "{{x:ne; b:{{x:taking; default:F}}}}"
"""The message ``keep-nest-ne`` asks a resolution of: ``ne`` is the format's own, and
the empty string ``x`` holds differs from ``b``, so it selects that option and
produces it during its call."""

DESCEND: Final = "{{x:descend; default:F}}"
"""The message ``descend`` asks a resolution of while it has further to go."""

RENESTED: Final = "[{{x:renest; default:F}}]"
"""The message ``renest`` asks a resolution of: the one it is registered in."""


def _output(adapter: Resolving, resolution: Resolution, asker: str) -> str:
    # The output of a resolution asked of the adapter; one it answered none
    # for, which is how an adapter shows one its implementation refused,
    # raises in the modifier that asked.
    resolved = adapter.resolve(resolution)
    output = resolved.output if isinstance(resolved, Resolved) else None
    if not isinstance(output, str):
        raise RuntimeError(f"The resolution {asker} asked for answered no output.")
    return output


def renesting(adapter: Resolving, bound: int) -> Behaviour:
    """The ``renest`` of the README, which asks ``adapter`` for the message it is registered in.

    It answers that resolution's output, with itself registered. One level
    past what ``bound`` lets stand it asks for nothing and answers
    ``unrefused``, so an implementation that refuses nothing fails the case
    in place of nesting until its host ends it.
    """
    standing = 0

    def renest(call: Call) -> str:
        nonlocal standing
        if standing > bound:
            return "unrefused"
        standing += 1
        try:
            return _output(
                adapter,
                Resolution(message=RENESTED, payload={"x": ""}, modifiers={"renest": renest}),
                "renest",
            )
        finally:
            standing -= 1

    return renest


def _shown(text: str) -> str:
    # A read that answered the empty string, as a modifier that goes on with
    # it answers: never empty, so a modifier that did not go on shows.
    return text or "empty"


class _Taking:
    """What ``taking`` answers: its ordinary string conversion makes the read (section 4)."""

    def __init__(self, read: Callable[[], str]) -> None:
        self._read = read

    def __str__(self) -> str:
        return self._read()


def _over(adapter: Resolving, descent: tuple[int, str] | None = None) -> dict[str, Behaviour]:
    # The behaviours over one list of what was kept, with a ``descend`` beside
    # them where ``descent`` gives its hops and the message it leads to.
    kept: list[Callable[[], str]] = []
    claimed: set[int] = set()

    def claim() -> Callable[[], str] | None:
        # The most recently kept input no ``take`` has claimed, claimed before
        # it is read, so a read that reaches ``take`` again finds the one kept
        # before it.
        for index in range(len(kept) - 1, -1, -1):
            if index not in claimed:
                claimed.add(index)
                return kept[index]
        return None

    def take(call: Call | None = None) -> str:
        read = claim()
        return "none" if read is None else _shown(read())

    def keep(call: Call) -> str:
        kept.append(call.default)
        return ""

    def keep_default(call: Call) -> str:
        kept.append(call.default)
        return call.default()

    def first(call: Call) -> str:
        return _shown(kept[0]()) if kept else "none"

    registered: dict[str, Behaviour] = {}

    def nested(message: str, asker: str) -> str:
        return _output(
            adapter, Resolution(message=message, payload={"x": ""}, modifiers=registered), asker
        )

    def keeping_nest(message: str, asker: str) -> Behaviour:
        def keep_nest(call: Call) -> str:
            kept.append(call.default)
            return nested(message, asker)

        return keep_nest

    registered.update(
        {
            "keep": keep,
            "keep-default": keep_default,
            "take": take,
            "first": first,
            "taking": lambda call: _Taking(take),
            "nest": lambda call: nested(NESTED, "nest"),
            "keep-nest": keeping_nest(NESTED_TAKING, "keep-nest"),
            "keep-nest-ne": keeping_nest(NESTED_DEFINED, "keep-nest-ne"),
        }
    )
    if descent is not None:
        hops, inner = descent
        descending = 0

        # With fewer calls in progress than the descent has hops, the next
        # hop; with as many, the message the descent leads to.
        def descend(call: Call) -> str:
            nonlocal descending
            descending += 1
            try:
                return nested(DESCEND if descending < hops else inner, "descend")
            finally:
                descending -= 1

        registered["descend"] = descend
    return registered


def keeping(adapter: Resolving) -> Mapping[str, Behaviour]:
    """The behaviours of the README over one list of what was kept.

    Each call builds them afresh, so what one resolution kept is read by no
    other but the one a ``nest``, a ``keep-nest`` or a ``keep-nest-ne`` among
    them asks ``adapter`` for while its call is in progress, handing it these
    same behaviours: a runner calls it once per resolution it asks for, with
    the adapter it asks.
    """
    return _over(adapter)


def descending(adapter: Resolving, hops: int, inner: str) -> Mapping[str, Behaviour]:
    """Those behaviours with the README's ``descend`` beside them.

    ``descend`` asks ``adapter`` for ``hops`` resolutions one inside another,
    each handed these same behaviours and ``descend``, the innermost of them
    of ``inner``.
    """
    return _over(adapter, (hops, inner))
