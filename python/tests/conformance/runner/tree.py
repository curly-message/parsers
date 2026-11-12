"""Tree cases: the tree an implementation describes a message by, held to CST.md."""

from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Final, cast

from .cases import describe
from .model import OMIT, UNITS, Adapter, Cst, Failed, Outcome, Passed, Resolution, Resolved

TYPES: Final = (
    "message",
    "placeholder",
    "open",
    "close",
    "separator",
    "space",
    "key",
    "modifier",
    "option-key",
    "option-value",
    "text",
    "escape",
)

NAMED: Final = ("key", "modifier", "option-key")
"""The kinds that carry a name (CST.md section 8).

An ``option-value`` is not one of them: it answers to nobody, so it carries
the subtree the message spells and no string to compare. A field a kind does
not take is not policed, because that table describes the interchange
encoding and an adapter hands back a host value.
"""

Node = Mapping[str, object]

__all__ = ["NAMED", "TYPES", "UNITS", "boundaries", "execute_tree", "spelling"]


def _width(character: str, unit: str) -> int:
    if unit == "code-point":
        return 1
    if unit == "utf-16":
        return 2 if ord(character) > 0xFFFF else 1
    return len(character.encode("utf-8", "surrogatepass"))


def boundaries(message: str, unit: str) -> list[int | None]:
    """Where each boundary of the message falls, as an index into the ``str``, by the declared unit.

    A boundary inside a code point has no index: a character wider than one
    unit contributes one entry for its start and ``None`` for each position
    within it, which is what CST.md section 4 forbids a span from landing on.
    """
    offsets: list[int | None] = []
    for at, character in enumerate(message):
        offsets.append(at)
        offsets.extend([None] * (_width(character, unit) - 1))
    offsets.append(len(message))
    return offsets


def spelling(message: str, unit: str, node: Node) -> str | None:
    """The text a span covers in the declared unit: what a name reads as before it is unescaped."""
    offsets = boundaries(message, unit)
    start, end = node.get("start"), node.get("end")
    if not (_offset(start) and _offset(end)):
        return None
    start, end = cast("int", start), cast("int", end)
    if not (0 <= start < len(offsets) and 0 <= end < len(offsets)):
        return None
    begin, finish = offsets[start], offsets[end]
    return None if begin is None or finish is None else message[begin:finish]


def _offset(value: object) -> bool:
    return isinstance(value, int) and not isinstance(value, bool)


def _budget(units: int) -> int:
    # A tree describes the message, so what it can hold follows from the
    # message's length: the densest construction, a run of empty segments,
    # contributes two nodes per character. A structure that holds itself would
    # be walked forever, so the walk is bounded by more than any tree of the
    # message can hold, and a cyclic answer fails the case instead of hanging
    # the run.
    return units * 4 + 64


@dataclass(frozen=True)
class _Walked:
    node: Node
    text: str
    depth: int


def _check(
    node: object, within: Node | None, message: str, offsets: Sequence[int | None]
) -> Failed | str:
    """The text a node spans, or the first thing wrong with it in the order CST.md states them."""
    units = len(offsets) - 1
    if not isinstance(node, Mapping):
        return Failed("The tree holds something that is not a node.", "a node", node)
    kind = node.get("type")
    if kind not in TYPES:
        return Failed(
            "The tree holds a node of no kind this document names.", ", ".join(TYPES), kind
        )
    start, end = node.get("start"), node.get("end")
    if not (_offset(start) and _offset(end)):
        return Failed(
            f"The span of the {kind} node is not a pair of offsets.", "two integers", [start, end]
        )
    start, end = cast("int", start), cast("int", end)
    if end < start:
        return Failed(
            f"The {kind} node ends before it starts.", "end at or after start", [start, end]
        )
    if start < 0 or end > units:
        return Failed(
            f"The {kind} node lies outside the message.", f"within [0,{units})", [start, end]
        )
    if within is not None and (
        start < cast("int", within["start"]) or end > cast("int", within["end"])
    ):
        return Failed(
            f"The {kind} node lies outside the {within['type']} node that holds it.",
            [within["start"], within["end"]],
            [start, end],
        )
    begin, finish = offsets[start], offsets[end]
    if begin is None or finish is None:
        return Failed(
            f"The span of the {kind} node falls inside a code point.",
            "a boundary between code points",
            [start, end],
        )
    if kind in NAMED and not isinstance(node.get("name"), str):
        return Failed(f"The {kind} node carries no name.", "the span unescaped", node.get("name"))
    if kind == "escape" and not isinstance(node.get("cancels"), bool):
        return Failed(
            "The escape node does not say which reading it takes.",
            "true or false",
            node.get("cancels"),
        )
    return message[begin:finish]


def _walk(root: object, message: str, offsets: Sequence[int | None]) -> list[_Walked] | Failed:
    # Every node in document order, each with the text its span covers, on a
    # stack of its own: the tree is the adapter's answer, however deep.
    budget = _budget(len(offsets) - 1)
    walked: list[_Walked] = []
    stack: list[tuple[object, int, Node | None]] = [(root, 0, None)]
    while stack:
        if len(walked) > budget:
            return Failed(
                "The tree holds more nodes than the message can spell.",
                f"at most {budget} nodes",
                "more",
            )
        value, depth, within = stack.pop()
        text = _check(value, within, message, offsets)
        if isinstance(text, Failed):
            return text
        node = cast("Node", value)
        walked.append(_Walked(node, text, depth))
        if "nodes" not in node:
            continue
        nodes = node["nodes"]
        if not isinstance(nodes, list | tuple):
            return Failed(
                f"The children of the {node['type']} node are not a list.", "a list of nodes", nodes
            )
        stack.extend((child, depth + 1, node) for child in reversed(nodes))
    return walked


def _children(node: Node) -> Sequence[Node]:
    return cast("Sequence[Node]", node.get("nodes") or ())


def _tiling(walked: Sequence[_Walked], message: str, units: int) -> Failed | None:
    # Properties 1, 2 and 3 of CST.md section 5, which are what a consumer
    # walks a tree by: the leaves tile the message in order and spell it back.
    at = 0
    spelled: list[str] = []
    for entry in walked:
        if _children(entry.node):
            continue
        if entry.node["start"] != at:
            return Failed(
                f"The leaves do not tile the message: a {entry.node['type']} node starts where "
                "the one before it did not end.",
                at,
                entry.node["start"],
            )
        at = cast("int", entry.node["end"])
        spelled.append(entry.text)
    if at != units:
        return Failed("The leaves do not reach the end of the message.", units, at)
    if "".join(spelled) != message:
        return Failed("The leaves do not spell the message back.", message, "".join(spelled))
    return None


def _spelled(node: Node, name: str) -> str:
    if name not in node:
        return "undefined"
    try:
        return json.dumps(node[name], ensure_ascii=False)
    except (TypeError, ValueError):
        return type(node[name]).__name__


def _shape(walked: Sequence[_Walked]) -> str:
    # The tree as the runner reads it, for comparing one parse with another.
    return "|".join(
        f"{entry.depth}:{entry.node['type']}:{json.dumps(entry.text, ensure_ascii=False)}:"
        f"{_spelled(entry.node, 'name')}:{_spelled(entry.node, 'cancels')}"
        for entry in walked
    )


def _strictly(left: object, right: object) -> bool:
    return type(left) is type(right) and left == right


def _lengths(expected: Sequence[Node], actual: Sequence[Node], where: str) -> Failed | None:
    if len(expected) == len(actual):
        return None
    return Failed(
        f"The children of {where} are {len(actual)} nodes rather than {len(expected)}.",
        [wanted["type"] for wanted in expected],
        [node.get("type") for node in actual],
    )


def _compare(
    expected: Sequence[Node], actual: Sequence[Node], texts: Mapping[int, str]
) -> Failed | None:
    # What a case expects of each node, against what the implementation
    # answered, in document order: a node's own fields, then its children,
    # then its next sibling.
    wrong = _lengths(expected, actual, "the message")
    if wrong is not None:
        return wrong
    stack: list[tuple[Sequence[Node], Sequence[Node], str, int]] = [
        (expected, actual, "the message", 0)
    ]
    while stack:
        wanted_nodes, nodes, where, index = stack.pop()
        if index == len(wanted_nodes):
            continue
        stack.append((wanted_nodes, nodes, where, index + 1))
        wanted, node = wanted_nodes[index], nodes[index]
        position = f"Node {index + 1} of {where}"
        if node["type"] != wanted["type"]:
            return Failed(f"{position} is of another kind.", wanted["type"], node["type"])
        text = texts.get(id(node))
        if text != wanted["text"]:
            return Failed(f"{position} spans other text.", wanted["text"], text)
        if "name" in wanted and not _strictly(node.get("name", OMIT), wanted["name"]):
            return Failed(
                f"{position} is unescaped to something else.", wanted["name"], node.get("name")
            )
        if "cancels" in wanted and not _strictly(node.get("cancels", OMIT), wanted["cancels"]):
            return Failed(
                f"{position} takes the other reading of section 7.",
                wanted["cancels"],
                node.get("cancels"),
            )
        if "nodes" not in wanted:
            continue
        kids = cast("Sequence[Node]", wanted["nodes"])
        inside = f"the {node['type']} node"
        wrong = _lengths(kids, _children(node), inside)
        if wrong is not None:
            return wrong
        stack.append((kids, _children(node), inside, 0))
    return None


def _answer(cst: Cst, message: str) -> Node | Failed:
    # A tree the adapter raised over, or one that is not a message node, fails
    # the case rather than ending the run.
    try:
        tree = cst.parse(message)
    except Exception as error:
        return Failed(
            "The adapter raised rather than answering with a tree.",
            "a message node",
            describe(error),
        )
    if not isinstance(tree, Mapping) or tree.get("type") != "message":
        return Failed("The adapter answered with no tree.", "a message node", tree)
    return cast("Node", tree)


def _agrees(adapter: Adapter, message: str, resolves: str) -> Outcome:
    try:
        resolved = adapter.resolve(Resolution(message=message))
    except Exception as error:
        return Failed(
            "The adapter raised resolving the message the tree describes.",
            resolves,
            describe(error),
        )
    output = resolved.output if isinstance(resolved, Resolved) else None
    if output == resolves and isinstance(output, str):
        return Passed()
    return Failed(
        "The tree and the resolution do not read the same placeholders.", resolves, output
    )


def execute_tree(adapter: Adapter, case: Mapping[str, object]) -> Outcome:
    """Runs a tree case.

    The properties of CST.md section 5 are checked on the tree the
    implementation answered with, in the unit it declared, before the case's
    own expectation is compared: a tree whose leaves do not tile the message
    is wrong about the message whatever the case says of its nodes.
    """
    cst = adapter.cst
    if cst is None:
        raise RuntimeError(
            f"The case {case['id']} was planned against an adapter that offers no tree."
        )
    message = cast("str", case["message"])
    offsets = boundaries(message, cst.unit)
    units = len(offsets) - 1
    tree = _answer(cst, message)
    if isinstance(tree, Failed):
        return tree
    start, end = tree.get("start"), tree.get("end")
    if not (_offset(start) and _offset(end) and start == 0 and end == units):
        return Failed("The tree does not span the whole message.", [0, units], [start, end])
    walked = _walk(tree, message, offsets)
    if isinstance(walked, Failed):
        return walked
    tiled = _tiling(walked, message, units)
    if tiled is not None:
        return tiled
    texts = {id(entry.node): entry.text for entry in walked}
    compared = _compare(cast("Sequence[Node]", case["expected"]), _children(tree), texts)
    if compared is not None:
        return compared
    again = _answer(cst, message)
    twice = again if isinstance(again, Failed) else _walk(again, message, offsets)
    if isinstance(twice, Failed):
        return twice
    if _shape(twice) != _shape(walked):
        return Failed(
            "The tree is not a function of the message alone: two parses of it differ.",
            _shape(walked),
            _shape(twice),
        )
    resolves = case.get("resolves")
    if resolves is None:
        return Passed()
    return _agrees(adapter, message, cast("str", resolves))
