"""The value model of section 4: what a host value is as text.

Everything the format carries is text. A plain ``dict`` and a plain ``list``
become JSON, so a host-defined modifier can read them back, and every other
value becomes what ``str`` makes of it.
"""

from __future__ import annotations

import json.encoder
import math
from collections.abc import Iterator
from typing import Any, Final, cast

from ._types import MAX_CONVERSION_NODES, UNDEFINED
from ._utils import MISSING, WHITESPACE_CHARS

Conversions = dict[int, tuple[object, str | None]]
"""What each value a resolution read came out as, by identity.

The record holds the value itself beside its text, so an identity cannot be
reused by another value while the resolution that recorded it runs.
"""

WrapperParts = tuple[object, object, object]
"""A wrapper's own ``value``, ``default`` and ``props``, each ``MISSING`` where it holds none."""

Wrappers = dict[int, tuple[object, WrapperParts | None]]
"""Which entries a resolution asked about are wrappers, by identity."""

_encode: Final = json.encoder.encode_basestring

_END: Final = object()

MAX_CONVERSION_LENGTH: Final = 10_000_000
"""Code points one serialization may write before it is abandoned.

The node limit bounds what a serialization visits and not what it writes: a
list naming one long string a thousand times visits a thousand and one nodes.
This bound is a hundred code points for each node the limit allows, where
JavaScript's serialization gives up at its engine's longest string.
"""


_REMEMBERED_LENGTH: Final = 64
"""The longest string encoded where it is met: one looked up costs about what encoding it does."""

_REMEMBERED_BITS: Final = 256
"""The widest integer written where it is met, as for a string."""


class _UnserializableError(ValueError):
    pass


def _key(key: object) -> str:
    """A member's name as ``json`` spells it, which coerces a scalar and refuses the rest."""
    if issubclass(type(key), str):
        return _encode(str.__str__(key))
    if issubclass(type(key), float):
        return _encode(_float(key))  # type: ignore[arg-type]
    if key is True:
        return '"true"'
    if key is False:
        return '"false"'
    if key is None:
        return '"null"'
    if issubclass(type(key), int):
        return _encode(int.__repr__(key))
    raise _UnserializableError("a key json cannot name")


def _float(value: float) -> str:
    if not math.isfinite(value):
        raise _UnserializableError("JSON holds no such number")
    return float.__repr__(value)


def _serialize(root: object) -> str:
    """``json.dumps(root, ensure_ascii=False, separators=(",", ":"), allow_nan=False)``.

    Written out rather than called for two reasons: ``json`` cannot count the
    nodes it visits, and its encoder recurses, so a value nested deep raises
    ``RecursionError`` instead of serializing. Containers are read through
    their base types' own storage, so no override runs. Every value visited,
    the root and each member at each time it is reached, spends a node
    (section 13), and every code point written spends a place of
    ``MAX_CONVERSION_LENGTH``. A long string or a wide integer met again is
    written from its first encoding rather than encoded again, so a leaf named
    many times costs its encoding once.
    """
    parts: list[str] = []
    room = MAX_CONVERSION_LENGTH
    # By identity, beside the value itself, as a resolution records its
    # conversions.
    encoded: dict[int, tuple[object, str]] = {}
    ancestors: set[int] = set()
    # Each open container: its members still to write, whether it is an
    # object, its identity, and whether it has written a member yet.
    frames: list[list[Any]] = []
    budget = MAX_CONVERSION_NODES
    value = root
    while True:
        budget -= 1
        if budget < 0:
            raise _UnserializableError("the value visits more nodes than a conversion may")
        kind = type(value)
        piece: str | None = None
        if issubclass(kind, str):
            if str.__len__(value) <= _REMEMBERED_LENGTH:  # type: ignore[arg-type]
                piece = _encode(str.__str__(value))
            else:
                known = encoded.get(id(value))
                if known is not None:
                    piece = known[1]
                else:
                    # The encoding is never shorter than the text and its
                    # quotes, so a text with no room is not encoded at all.
                    if str.__len__(value) + 2 > room:  # type: ignore[arg-type]
                        raise _UnserializableError("the value writes more than a conversion may")
                    piece = _encode(str.__str__(value))
                    encoded[id(value)] = (value, piece)
        elif value is True:
            piece = "true"
        elif value is False:
            piece = "false"
        elif issubclass(kind, int):
            if int.bit_length(value) <= _REMEMBERED_BITS:  # type: ignore[arg-type]
                piece = int.__repr__(value)
            else:
                known = encoded.get(id(value))
                if known is not None:
                    piece = known[1]
                else:
                    piece = int.__repr__(value)
                    encoded[id(value)] = (value, piece)
        elif value is None:
            piece = "null"
        elif issubclass(kind, float):
            piece = _float(value)  # type: ignore[arg-type]
        if piece is not None:
            room -= len(piece)
            parts.append(piece)
        elif issubclass(kind, (list, tuple, dict)):
            identity = id(value)
            if identity in ancestors:
                raise _UnserializableError("the value holds itself")
            ancestors.add(identity)
            members: Iterator[Any]
            if issubclass(kind, dict):
                members = iter(dict.items(value))  # type: ignore[arg-type]
                parts.append("{")
                room -= 1
            else:
                members = (
                    list.__iter__(value)  # type: ignore[arg-type]
                    if issubclass(kind, list)
                    else tuple.__iter__(value)  # type: ignore[arg-type]
                )
                parts.append("[")
                room -= 1
            frames.append([members, issubclass(kind, dict), identity, False])
        else:
            raise _UnserializableError("a value json cannot describe")
        while frames:
            if room < 0:
                raise _UnserializableError("the value writes more than a conversion may")
            frame = frames[-1]
            member = next(frame[0], _END)
            if member is _END:
                frames.pop()
                ancestors.discard(frame[2])
                parts.append("}" if frame[1] else "]")
                room -= 1
                continue
            if frame[3]:
                parts.append(",")
                room -= 1
            frame[3] = True
            if frame[1]:
                name, value = cast(tuple[object, object], member)
                # A name writes at least itself, its quotes and its colon.
                if issubclass(type(name), str) and str.__len__(name) + 3 > room:  # type: ignore[arg-type]
                    raise _UnserializableError("the value writes more than a conversion may")
                key = _key(name)
                parts.append(key)
                parts.append(":")
                room -= len(key) + 1
            else:
                value = member
            break
        else:
            if room < 0:
                raise _UnserializableError("the value writes more than a conversion may")
            return "".join(parts)


def serialize(value: object) -> str | None:
    """A plain value's JSON, or ``None`` where no serialization describes it."""
    try:
        return _serialize(value)
    except Exception:
        return None


def _string(value: object) -> str | None:
    """The host's ordinary string conversion, or ``None`` where it raises."""
    try:
        converted = str(value)
    except Exception:
        return None
    # `str` hands back a subclass where `__str__` answers one, and the text is
    # what the format carries rather than the type it came in.
    return converted if type(converted) is str else str.__str__(converted)


def text(value: object, conversions: Conversions) -> str | None:
    """The text a value resolves to, or ``None``: no value, or one no conversion describes.

    A conversion that runs host code, or costs a walk, is recorded for the
    resolution, so a value is converted once however many placeholders read it
    and every later read answers with the first answer (section 13). A
    built-in scalar runs no host code and answers the same every time, so it is
    not recorded.
    """
    if value is UNDEFINED or value is MISSING:
        return None
    kind = type(value)
    if kind is str:
        return value  # type: ignore[return-value]
    if kind is int:
        try:
            return str(value)
        except ValueError:
            # Past the host's limit on the digits it converts.
            return None
    if kind is float or kind is bool or value is None:
        return str(value)
    identity = id(value)
    known = conversions.get(identity)
    if known is not None:
        return known[1]
    converted = serialize(value) if kind is dict or kind is list else _string(value)
    conversions[identity] = (value, converted)
    return converted


def wrapper_parts(entry: object, wrappers: Wrappers) -> WrapperParts | None:
    """An entry's own ``value``, ``default`` and ``props`` where it is a wrapper (section 4.1).

    A wrapper is a plain ``dict`` whose every own key names one of those three,
    and which holds at least one. A key that is not a ``str`` names none of
    them, so a ``dict`` holding one is a value. Enumerating costs what the
    entry holds, so the verdict is recorded for the resolution the way a
    conversion is.
    """
    if type(entry) is not dict:
        return None
    identity = id(entry)
    known = wrappers.get(identity)
    if known is not None:
        return known[1]
    parts: list[object] = [MISSING, MISSING, MISSING]
    named = False
    verdict: WrapperParts | None = None
    for key, member in dict.items(entry):
        slot = _WRAPPED.get(str.__str__(key)) if issubclass(type(key), str) else None
        if slot is None:
            named = False
            break
        if parts[slot] is MISSING:
            parts[slot] = member
        named = True
    if named:
        verdict = (parts[0], parts[1], parts[2])
    wrappers[identity] = (entry, verdict)
    return verdict


_WRAPPED: Final = {"value": 0, "default": 1, "props": 2}


def to_number(value: str) -> float | None:
    """The host's ordinary numeric conversion of text, or ``None`` where it yields no number.

    The format's whitespace class is stripped first, since it is not part of a
    number and ``float`` strips only its own. What ``float`` would skip beyond
    the class, ``U+0085`` among it, is part of the text, so text that begins
    or ends with it is no number; every code point ``float`` skips is one
    ``str.isspace`` holds.
    NaN is no number: left in, it compares false to everything, and an
    ordering would freeze around it.
    """
    text = value.strip(WHITESPACE_CHARS)
    try:
        number = float(text)
    except ValueError:
        return None
    if math.isnan(number) or text[0].isspace() or text[-1].isspace():
        return None
    return number
