"""The defect catalogue, and each defect as this host carries it.

RUNNER.md states what each defect pins. Each realization wraps a conforming
adapter into one that is wrong in exactly one way, and records whether the
defect reached the runner at all: one that altered nothing measured nothing.
"""

from __future__ import annotations

import dataclasses
from collections.abc import Callable, Mapping, Sequence
from typing import Final, cast

from .archive import fixtures, member, validated
from .cases import FRAMED, RESOLUTIONS, UNDEFINED_COUNTED, covers, renested
from .model import (
    OMIT,
    Adapter,
    Cst,
    DefectEntry,
    Mutation,
    Resolution,
    Resolved,
    Run,
    Unexpressible,
    Verdict,
)
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


def _cases(run: Run, generated: bool) -> list[Mapping[str, object]]:
    # The cases of a run's set at the levels it runs, written or generated.
    return [
        case
        for fixture in run.set
        if fixture.file.get("kind") != "tree" and fixture.file.get("level") in run.levels
        for case in cast("Sequence[Mapping[str, object]]", fixture.file["cases"])
        if ("generate" in case) == generated
    ]


def _declaring(entry: Unexpressible) -> Callable[[Adapter, Run], Mutation]:
    # A defect of what the adapter declares it cannot express: one entry more
    # than the adapter declared itself, so the gaps it documents stay left
    # out. It reaches the runner where a case of the run states a request the
    # entry covers and no entry of the adapter's own covers.
    def realized(adapter: Adapter, run: Run) -> Mutation:
        own = list((adapter.unexpressible or {}).get("NumberFormat", ()))

        def leaves(case: Mapping[str, object]) -> bool:
            request = cast("Mapping[str, object]", case["expected"]).get("format")
            if not isinstance(request, Mapping) or request.get("api") != "NumberFormat":
                return False
            options = cast("Mapping[str, object]", request.get("options") or {})
            return covers(entry, "NumberFormat", options) is not None and not any(
                covers(declared, "NumberFormat", options) is not None for declared in own
            )

        return _claiming(
            adapter,
            lambda: any(leaves(case) for case in _cases(run, generated=False)),
            unexpressible={**(adapter.unexpressible or {}), "NumberFormat": [*own, entry]},
        )

    return realized


def _built(adapter: Adapter, run: Run, constructions: Sequence[str]) -> bool:
    # Whether the run builds one of the constructions, where the plan leaves
    # none of them out for want of a stand-in for undefined.
    return any(
        case["generate"] in constructions
        and not (case["generate"] in UNDEFINED_COUNTED and adapter.undefined is OMIT)
        for case in _cases(run, generated=True)
    )


def _converting(by: int, constructions: Sequence[str]) -> Callable[[Adapter, Run], Mutation]:
    # A defect of the conversion limit the adapter declares, which only the
    # constructions that build from it read.
    def realized(adapter: Adapter, run: Run) -> Mutation:
        conversion = cast("int", adapter.limits["conversion"])
        return _claiming(
            adapter,
            lambda: _built(adapter, run, constructions),
            limits={**adapter.limits, "conversion": conversion + by},
        )

    return realized


DEFINED: Final = "reentry-over-limit-beneath-a-defined-modifier"
"""The construction whose read is refused beneath a modifier of the format."""


def _reentering(by: int, constructions: Sequence[str]) -> Callable[[Adapter, Run], Mutation]:
    # A defect of the bound on counted reads the adapter states. An adapter
    # that states none, or states 0 where the defect would state one lower,
    # carries nothing to alter. Otherwise only the constructions built from
    # it read it. Those built from kept reads run where a modifier can keep
    # an input; the two built inside a resolution a modifier asks for run at
    # a bound above 0, and from resolutions, whatever the adapter states of
    # keeping, where it states that its bound counts them. The one whose read
    # is refused shows a bound stated one lower however it is built, and one
    # stated one higher only built from resolutions: built from kept reads it
    # stands a place short where the bound counts reads alone. The other
    # answers the same text where the resolution is refused, so it shows only
    # a bound stated one higher, built from resolutions, by the report of the
    # modifier that asked, which reaches the runner at a stated bound of 1
    # where the adapter observes reports.
    def realized(adapter: Adapter, run: Run) -> Mutation:
        declared = -1 if adapter.reentry is None else adapter.reentry + by
        if declared < 0:
            return Mutation(adapter, lambda: False)
        counts = adapter.resolutions is True
        observes = _Flag()

        def resolve(resolution: Resolution) -> object:
            resolved = adapter.resolve(resolution)
            if isinstance(resolved, Resolved) and resolved.reports is not None:
                observes.reached = True
            return resolved

        def shows(generate: object) -> bool:
            if generate not in constructions:
                return False
            if generate not in FRAMED:
                return adapter.keeps is not False
            if declared == 0:
                return False
            if generate == DEFINED:
                return counts or (by < 0 and adapter.keeps is not False)
            return counts and by > 0 and declared == 1 and observes()

        return _claiming(
            dataclasses.replace(adapter, resolve=resolve),
            lambda: any(shows(case["generate"]) for case in _cases(run, generated=True)),
            reentry=declared,
        )

    return realized


def _overstating(adapter: Adapter, run: Run) -> Mutation:
    # The construction of resolutions one inside another then runs over an
    # implementation that refuses none of them, and the one whose read is
    # refused beneath a modifier of the format is built from resolutions that
    # take no place, so above a bound of 1 its read is let through. An adapter
    # that states the claim itself carries nothing to alter; the first is left
    # out all the same where it states no bound or its output limit does not
    # hold it, and the second shows nothing where it states none, 0 or 1.
    reentry = adapter.reentry

    def shows(generate: object) -> bool:
        if reentry is None:
            return False
        if generate == RESOLUTIONS:
            return len(renested(reentry)) <= cast("int", adapter.limits["output"])
        return generate == DEFINED and reentry > 1

    return _claiming(
        adapter,
        lambda: (
            adapter.resolutions is not True
            and any(shows(case["generate"]) for case in _cases(run, generated=True))
        ),
        resolutions=True,
    )


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


_IN_RUN: Final[Mapping[str, Callable[[Adapter, Run], Mutation]]] = {
    "unexpressible-declared": _declaring("maximumFractionDigits"),
    "unexpressible-conditional-declared": _declaring(
        {"property": "maximumFractionDigits", "values": [2], "when": {"useGrouping": False}}
    ),
    # Only the construction at the limit visits a node more than the limit the
    # implementation imposes; past it, either limit refuses.
    "conversion-limit-overstated": _converting(1, ("conversion-at-limit",)),
    # Only the constructions past the limit then visit no node more than the
    # implementation's limit; at it, either limit lets the value through.
    "conversion-limit-understated": _converting(
        -1, ("conversion-over-limit", "conversion-over-limit-continues")
    ),
    # Every construction that stands at the bound then asks for one read more
    # than the implementation lets stand.
    "reentry-limit-overstated": _reentering(
        1,
        (
            "reentry-at-limit",
            "reentry-over-limit",
            "reentry-refused-not-recorded",
            "reentry-own-reads-free",
            "reentry-over-limit-by-a-conversion",
            "reentry-own-read-in-a-nested-resolution",
            DEFINED,
        ),
    ),
    # Only the constructions past the bound then let one read through that the
    # case expects refused.
    "reentry-limit-understated": _reentering(
        -1,
        (
            "reentry-over-limit",
            "reentry-refused-not-recorded",
            "reentry-over-limit-by-a-conversion",
            DEFINED,
        ),
    ),
    "resolutions-overstated": _overstating,
}
"""The defects whose reach depends on the run: the set it reads and the levels it runs."""

_ALONE: Final[Mapping[str, Callable[[Adapter], Mutation]]] = {
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
    "claims-no-core": lambda adapter: _claiming(
        adapter, levels=tuple(level for level in adapter.levels if level != "core")
    ),
    "claims-unknown-level": lambda adapter: _claiming(
        adapter, levels=(*adapter.levels, "ecmascript")
    ),
    "claims-no-limits": lambda adapter: _claiming(adapter, limits={**adapter.limits, "nesting": 0}),
    "claims-negative-reentry": lambda adapter: _claiming(adapter, reentry=-1),
    "claims-non-boolean-resolutions": lambda adapter: _claiming(adapter, resolutions="true"),
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


def realize(name: str, adapter: Adapter, run: Run) -> Mutation:
    """A defect realized for the run an audit makes, which may read another set or fewer levels."""
    if name in _IN_RUN:
        return _IN_RUN[name](adapter, run)
    return _ALONE[name](adapter)


def _shipped(name: str) -> Callable[[Adapter], Mutation]:
    return lambda adapter: _IN_RUN[name](adapter, Run(fixtures(), tuple(adapter.levels)))


MUTATIONS: Final[Mapping[str, Callable[[Adapter], Mutation]]] = {
    **_ALONE,
    **{name: _shipped(name) for name in _IN_RUN},
}
"""The catalogue's defects as this host carries them, each over the shipped set at the
levels the adapter claims: each wraps a conforming adapter into one wrong in exactly one way."""
