"""The catalogue of host-defined modifiers the set's README names, each a function of a ``Call``."""

from __future__ import annotations

import json
from collections.abc import Mapping

from .model import OMIT, Behaviour, Call

_UPPER = str.maketrans("abcdefghijklmnopqrstuvwxyz", "ABCDEFGHIJKLMNOPQRSTUVWXYZ")


def _units(name: str) -> bytes:
    # The README sorts names as ECMAScript sorts strings, by UTF-16 code
    # units, which orders a character outside the basic plane before U+E000.
    return name.encode("utf-16-be", "surrogatepass")


def _spelled(value: object) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), allow_nan=False)


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
            f"{name}={_spelled(call.props[name])}" for name in sorted(call.props, key=_units)
        ),
        "locale": locale,
        "object": lambda call: {"answer": call.value},
    }
