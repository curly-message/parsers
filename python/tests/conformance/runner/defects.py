"""The defect catalogue, and each defect as this host carries it.

RUNNER.md states what each defect pins. Each realization wraps a conforming
adapter into one that is wrong in exactly one way, and records whether the
defect reached the runner at all: one that altered nothing measured nothing.
"""

from __future__ import annotations

import dataclasses
from collections.abc import Callable, Mapping, Sequence
from typing import Final, cast

from .archive import member, validated
from .model import Adapter, Cst, DefectEntry, Mutation, Resolution, Resolved, Verdict
from .tree import NAMED, spelling

Node = Mapping[str, object]
Report = Mapping[str, object]


def defects() -> list[DefectEntry]:
    """The catalogue shipped in the pinned archive, validated against its schema."""
    file = validated("defects.json", member("defects.json"), schema="defect.schema.json")
    entries = cast("Sequence[Mapping[str, object]]", file["defects"])
    return [
        DefectEntry(
            id=cast("str", entry["id"]),
            description=cast("str", entry["description"]),
            expects=cast("Verdict", entry["expects"]),
            section=cast("str | None", entry.get("section")),
            document=cast("str | None", entry.get("document")),
        )
        for entry in entries
    ]


class _Flag:
    """Whether a defect altered anything yet."""

    def __init__(self) -> None:
        self.reached = False

    def __call__(self) -> bool:
        return self.reached


def _answering(adapter: Adapter, change: Callable[[Resolved], Resolved | None]) -> Mutation:
    # A defect of what the adapter answers. ``change`` answers None where this
    # resolution held nothing to alter, which is what ``reached`` is read from.
    flag = _Flag()

    def resolve(resolution: Resolution) -> object:
        resolved = adapter.resolve(resolution)
        if not isinstance(resolved, Resolved):
            return resolved
        changed = change(resolved)
        if changed is None:
            return resolved
        flag.reached = True
        return changed

    return Mutation(dataclasses.replace(adapter, resolve=resolve), flag)


def _reporting(adapter: Adapter, change: Callable[[list[Report]], list[Report] | None]) -> Mutation:
    # A defect of the reports, which an implementation need not observe at all.
    def changed(resolved: Resolved) -> Resolved | None:
        if not isinstance(resolved.reports, list | tuple):
            return None
        reports = change(list(resolved.reports))
        return None if reports is None else dataclasses.replace(resolved, reports=reports)

    return _answering(adapter, changed)


def _instead(resolve: Callable[[Resolution], object]) -> Callable[[Adapter], Mutation]:
    # A defect that answers nothing of what the implementation produced.
    return lambda adapter: Mutation(dataclasses.replace(adapter, resolve=resolve), lambda: True)


def _claiming(
    adapter: Adapter, reached: Callable[[], bool] = lambda: True, **claims: object
) -> Mutation:
    # A defect of what the adapter claims about itself. The claims are read
    # before anything runs, so such a defect reaches the runner wherever the
    # claim it alters was one the adapter made.
    return Mutation(dataclasses.replace(adapter, **claims), reached)  # type: ignore[arg-type]


def _parsing(adapter: Adapter, change: Callable[[Node, str], Node | None]) -> Mutation:
    # A defect of the tree, which an implementation need not offer at all: an
    # adapter that offers none carries nothing to alter, so the defect is
    # unreachable rather than missed.
    cst = adapter.cst
    if cst is None:
        return Mutation(adapter, lambda: False)
    flag = _Flag()

    def parse(message: str) -> object:
        tree = cst.parse(message)
        if not isinstance(tree, Mapping):
            return tree
        changed = change(tree, message)
        if changed is None:
            return tree
        flag.reached = True
        return changed

    return Mutation(dataclasses.replace(adapter, cst=Cst(cst.unit, parse)), flag)


def _rebuilt(tree: Node, change: Callable[[Node], Node]) -> Node | None:
    # The tree copied with ``change`` applied to every node, on a stack of its
    # own; None where it changed none.
    altered = False
    pending: list[tuple[Sequence[object], list[object]]] = []

    def one(node: object) -> object:
        nonlocal altered
        if not isinstance(node, Mapping):
            return node
        changed = change(node)
        altered = altered or changed is not node
        nodes = changed.get("nodes")
        if not isinstance(nodes, list | tuple):
            return changed
        copied: list[object] = [None] * len(nodes)
        pending.append((nodes, copied))
        return {**changed, "nodes": copied}

    root = one(tree)
    while pending:
        nodes, copied = pending.pop()
        for index, child in enumerate(nodes):
            copied[index] = one(child)
    return cast("Node", root) if altered else None


def _everywhere(adapter: Adapter, change: Callable[[Node, str], Node]) -> Mutation:
    # A defect applied to every node it recognizes, which reaches the runner
    # where it recognized at least one.
    return _parsing(
        adapter, lambda tree, message: _rebuilt(tree, lambda node: change(node, message))
    )


def _dropped(tree: Node, message: str) -> Node | None:
    # The first node with children, in document order, is the root wherever
    # the root has any: it loses its last one.
    nodes = tree.get("nodes")
    if not isinstance(nodes, list | tuple) or not nodes:
        return None
    return {**tree, "nodes": list(nodes[:-1])}


def _named_raw(adapter: Adapter) -> Callable[[Node, str], Node]:
    unit = adapter.cst.unit if adapter.cst is not None else "code-point"

    def change(node: Node, message: str) -> Node:
        if node.get("type") not in NAMED:
            return node
        spelled = spelling(message, unit, node)
        return node if spelled is None or spelled == node.get("name") else {**node, "name": spelled}

    return change


def _recounted(tree: Node, message: str) -> Node | None:
    # Every span counted in UTF-16 code units though the adapter still
    # declares code points: the literal reading of the catalogue for an
    # adapter that already declares code points.
    table = [0]
    for character in message:
        table.append(table[-1] + (2 if ord(character) > 0xFFFF else 1))

    def at(value: object) -> object:
        if isinstance(value, int) and not isinstance(value, bool) and 0 <= value < len(table):
            return table[value]
        return value

    def change(node: Node) -> Node:
        start, end = at(node.get("start")), at(node.get("end"))
        if start == node.get("start") and end == node.get("end"):
            return node
        return {**node, "start": start, "end": end}

    return _rebuilt(tree, change)


def _unit_changed(adapter: Adapter) -> Mutation:
    cst = adapter.cst
    if cst is None:
        return Mutation(adapter, lambda: False)
    if cst.unit != "code-point":
        return _claiming(adapter, cst=Cst("code-point", cst.parse))
    return _parsing(adapter, _recounted)


def _other_code(code: object) -> str:
    return "failed-modifier" if code == "unknown-modifier" else "unknown-modifier"


def _other_origin(origin: object) -> str:
    return "payload" if origin == "message" else "message"


def _integer(value: object) -> bool:
    return isinstance(value, int) and not isinstance(value, bool)


OTHER_ID: Final = "an id no case names"

OTHER_REQUEST: Final[Mapping[str, object]] = {
    "api": "NumberFormat",
    "options": {"style": "percent"},
    "input": 0,
}
"""A request the set states nowhere, so a case compared on one fails on it."""


def _raise(resolution: Resolution) -> object:
    raise RuntimeError("The raises defect raised.")


def _formats_wrong(adapter: Adapter) -> Mutation:
    # The one defect that answers what the implementation produced and states
    # something else about it: the output stands, and only the request is
    # wrong, so a runner that ignores the request answers nothing at all.
    def resolve(resolution: Resolution) -> object:
        resolved = adapter.resolve(resolution)
        if not isinstance(resolved, Resolved):
            return resolved
        return dataclasses.replace(resolved, formats=[OTHER_REQUEST])

    return Mutation(dataclasses.replace(adapter, resolve=resolve), lambda: "intl" in adapter.levels)


def _strip(resolved: Resolved) -> Resolved | None:
    if not isinstance(resolved.output, str):
        return None
    trimmed = resolved.output.strip()
    return None if trimmed == resolved.output else dataclasses.replace(resolved, output=trimmed)


MUTATIONS: Final[Mapping[str, Callable[[Adapter], Mutation]]] = {
    "output-truncated": lambda adapter: _answering(
        adapter,
        lambda resolved: (
            dataclasses.replace(resolved, output=resolved.output[:-1])
            if isinstance(resolved.output, str) and resolved.output
            else None
        ),
    ),
    "output-trimmed": lambda adapter: _answering(adapter, _strip),
    "answers-nothing": _instead(lambda resolution: None),
    "raises": _instead(_raise),
    "reports-dropped": lambda adapter: _reporting(adapter, lambda reports: [] if reports else None),
    "reports-extra": lambda adapter: _reporting(
        adapter, lambda reports: [*reports, {"code": "unknown-modifier"}]
    ),
    "reports-reversed": lambda adapter: _reporting(
        adapter, lambda reports: None if reports[::-1] == reports else reports[::-1]
    ),
    "report-code-changed": lambda adapter: _reporting(
        adapter,
        lambda reports: (
            [{**report, "code": _other_code(report.get("code"))} for report in reports]
            if reports
            else None
        ),
    ),
    "report-origin-changed": lambda adapter: _reporting(
        adapter,
        lambda reports: (
            [
                {**report, "origin": _other_origin(report["origin"])}
                if "origin" in report
                else report
                for report in reports
            ]
            if any("origin" in report for report in reports)
            else None
        ),
    ),
    "report-id-changed": lambda adapter: _reporting(
        adapter,
        lambda reports: (
            [{**report, "id": OTHER_ID} if "id" in report else report for report in reports]
            if any("id" in report for report in reports)
            else None
        ),
    ),
    "report-limit-changed": lambda adapter: _reporting(
        adapter,
        lambda reports: (
            [
                {**report, "limit": cast("int", report["limit"]) + 1}
                if _integer(report.get("limit"))
                else report
                for report in reports
            ]
            if any(_integer(report.get("limit")) for report in reports)
            else None
        ),
    ),
    "formats-wrong": _formats_wrong,
    "reports-unobserved": lambda adapter: _answering(
        adapter,
        lambda resolved: (
            None if resolved.reports is None else dataclasses.replace(resolved, reports=None)
        ),
    ),
    "claims-core-only": lambda adapter: _claiming(
        adapter, lambda: len(adapter.levels) > 1, levels=("core",)
    ),
    "unexpressible-declared": lambda adapter: _claiming(
        adapter,
        lambda: "intl" in adapter.levels,
        unexpressible={"NumberFormat": ["maximumFractionDigits"]},
    ),
    "claims-no-core": lambda adapter: _claiming(
        adapter, levels=tuple(level for level in adapter.levels if level != "core")
    ),
    "claims-unknown-level": lambda adapter: _claiming(
        adapter, levels=(*adapter.levels, "ecmascript")
    ),
    "claims-no-limits": lambda adapter: _claiming(adapter, limits={**adapter.limits, "nesting": 0}),
    "tree-unoffered": lambda adapter: _claiming(adapter, lambda: adapter.cst is not None, cst=None),
    "tree-node-dropped": lambda adapter: _parsing(adapter, _dropped),
    "tree-node-retyped": lambda adapter: _everywhere(
        adapter,
        lambda node, message: {**node, "type": "text"} if node.get("type") == "separator" else node,
    ),
    "tree-span-shifted": lambda adapter: _everywhere(
        adapter,
        lambda node, message: (
            {**node, "start": cast("int", node["start"]) + 1}
            if node.get("type") == "placeholder" and _integer(node.get("start"))
            else node
        ),
    ),
    "tree-name-raw": lambda adapter: _everywhere(adapter, _named_raw(adapter)),
    "tree-cancels-inverted": lambda adapter: _everywhere(
        adapter,
        lambda node, message: (
            {**node, "cancels": not node.get("cancels")} if node.get("type") == "escape" else node
        ),
    ),
    "tree-unit-changed": _unit_changed,
    "tree-unit-unknown": lambda adapter: _claiming(
        adapter,
        lambda: adapter.cst is not None,
        cst=None if adapter.cst is None else Cst("utf-32", adapter.cst.parse),
    ),
}
