"""The host values a case's tagged inputs stand for.

A case's inputs are JSON, and three constructions of the format's data model
cannot be written as JSON, so a case writes each as an object carrying a
``$curly`` key. Inputs are untrusted JSON, so the walk runs on an explicit
stack rather than on the host's.
"""

from __future__ import annotations

from collections.abc import Iterable
from typing import Final, cast

from .model import OMIT, SetError

TAG: Final = "$curly"


def nodes(count: int | float) -> list[object]:
    """A value whose serialization visits at least ``count`` nodes, holding no cycle.

    Serialization follows a shared reference again every time it meets one,
    so each level of ``[below, below]`` spends one node on itself and twice
    what ``below`` spends: the visits double per level.
    """
    tree: list[object] = []
    visits = 1
    while visits < count:
        tree = [tree, tree]
        visits = visits * 2 + 1
    return tree


def unserializable() -> dict[str, object]:
    """An object holding itself, which this host's serialization refuses as a cycle."""
    value: dict[str, object] = {}
    value["self"] = value
    return value


def _tagged(tag: dict[str, object], undefined: object) -> object:
    name = tag[TAG]
    if name == "undefined":
        return undefined
    if name == "unserializable":
        return unserializable()
    if name == "nodes":
        count = tag.get("count")
        if isinstance(count, bool) or not isinstance(count, int | float):
            raise SetError("A nodes tag needs a numeric count.")
        return nodes(count)
    raise SetError(f"Unknown tag {name!r}.")


def _one(value: object, undefined: object) -> tuple[object, bool]:
    """The value itself where it is a leaf or a tag, and whether its members are still to come."""
    if type(value) is dict:
        if TAG in value:
            return _tagged(value, undefined), False
        return {}, True
    if type(value) is list:
        return [], True
    return value, False


def decode(value: object, undefined: object = OMIT) -> object:
    """The host value a fixture's JSON stands for: tagged objects replaced, all else as written.

    ``undefined`` is what the tagged undefined becomes. Where it is ``OMIT``,
    the adapter states no value for it, and a member holding one is left out
    of its container; a top-level value so tagged is ``OMIT`` itself.
    """
    root, open_ = _one(value, undefined)
    stack: list[tuple[object, object]] = [(value, root)] if open_ else []
    while stack:
        source, target = stack.pop()
        members: Iterable[tuple[object, object]] = (
            source.items() if isinstance(source, dict) else enumerate(cast("list[object]", source))
        )
        pending: list[tuple[object, object]] = []
        for key, entry in members:
            decoded, more = _one(entry, undefined)
            if decoded is OMIT:
                continue
            if isinstance(target, dict):
                target[key] = decoded
            elif isinstance(target, list):
                target.append(decoded)
            if more:
                pending.append((entry, decoded))
        stack.extend(reversed(pending))
    return root
