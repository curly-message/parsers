"""The host values a case's tagged inputs stand for.

A case's inputs are JSON, and three constructions of the format's data model
cannot be written as JSON, so a case writes each as an object carrying a
``$curly`` key. Inputs are untrusted JSON, so the walk runs on an explicit
stack rather than on the host's.

The fourth, ``refusing``, is a mapping whose entries raise where they are
read. This runner builds it as a ``dict`` whose keys each compare by raising:
``dict`` storage runs host code only where a lookup by a name meets a stored
key that collides with it, so such an entry raises where it is looked up by
its name, and never where the storage is enumerated. The plan leaves out a
case whose refusing value the implementation enumerates rather than looks up
(RUNNER.md, *Planning*).
"""

from __future__ import annotations

from collections.abc import Iterable
from typing import Final, cast

from .model import OMIT, SetError

TAG: Final = "$curly"

REFUSING: Final = "refusing"


class Refused(str):
    """A name stored as a key whose comparison raises, so a lookup by that name raises."""

    __slots__ = ()

    __hash__ = str.__hash__

    def __eq__(self, other: object) -> bool:
        raise LookupError(f"The entry {str.__str__(self)!r} refuses to be read.")


def refusing(members: object) -> dict[str, object]:
    """A mapping holding each of ``members`` as an entry, where a lookup of any of them raises."""
    if type(members) is not list or not all(type(member) is str for member in members):
        raise SetError("A refusing tag needs a list of names as its members.")
    # A name written twice is one entry, and its second key would meet the first.
    return {Refused(member): None for member in dict.fromkeys(members)}


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
    if name == REFUSING:
        return refusing(tag.get("members"))
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


def is_refusing(value: object) -> bool:
    """Whether a fixture's JSON is itself a ``refusing`` tag."""
    return type(value) is dict and value.get(TAG) == REFUSING


def refuses(value: object) -> bool:
    """Whether a fixture's JSON holds a ``refusing`` tag, at any depth."""
    stack = [value]
    while stack:
        current = stack.pop()
        if type(current) is dict:
            if is_refusing(current):
                return True
            stack.extend(current.values())
        elif type(current) is list:
            stack.extend(current)
    return False


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
