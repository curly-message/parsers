"""A runner for the conformance set of the Curly Message Format, in Python.

RUNNER.md states what a runner in another language is held to: read the set
as released, refuse what it cannot read, execute each case through an
adapter (section 14.3), and answer for every case left out why it was. The
defect catalogue holds the runner to that: ``audit`` runs it against an
adapter that passes, and answers what the runner made of each defect.
"""

from __future__ import annotations

import functools
import json
from collections.abc import Mapping, Sequence
from typing import cast

from .archive import fixtures, load
from .behaviours import behaviours, descending, keeping, renesting
from .cases import (
    COUNTED,
    FRAMED,
    RESOLUTIONS,
    UNDEFINED_COUNTED,
    covers,
    execute_concrete,
    execute_generated,
    renested,
    shown,
)
from .decode import decode, is_refusing, refuses
from .defects import MUTATIONS, defects, realize
from .model import (
    APIS,
    LEVELS,
    LIMITS,
    OMIT,
    UNITS,
    Adapter,
    Audited,
    Call,
    Caught,
    ClaimError,
    Cst,
    Failed,
    Fixture,
    Identity,
    Observed,
    Outcome,
    Passed,
    Plan,
    Planned,
    Resolution,
    Resolved,
    Result,
    Run,
    SetError,
    Skipped,
)
from .tree import execute_tree

__all__ = [
    "OMIT",
    "Adapter",
    "Audited",
    "Call",
    "ClaimError",
    "Cst",
    "Failed",
    "Fixture",
    "Identity",
    "Outcome",
    "Passed",
    "Plan",
    "Planned",
    "Resolution",
    "Resolved",
    "Result",
    "SetError",
    "Skipped",
    "audit",
    "behaviours",
    "check",
    "decode",
    "defects",
    "descending",
    "fixtures",
    "keeping",
    "load",
    "plan",
    "renesting",
    "run",
    "summarize",
]

FORMAT = "curly-message-3"


def _levels(levels: object, where: str) -> Sequence[str]:
    if isinstance(levels, str) or not isinstance(levels, Sequence):
        raise ClaimError(f"{where} must list levels.")
    for level in levels:
        if level not in LEVELS:
            raise ClaimError(
                f"{where} names the level {json.dumps(level, default=repr)}; "
                "the levels are core, intl and extensions."
            )
    return cast("Sequence[str]", levels)


def _unexpressible(adapter: Adapter) -> None:
    # The third statement an adapter makes about itself (section 11.2), held to
    # the same vocabulary as the other two before anything runs.
    declared = adapter.unexpressible
    if declared is None:
        return
    if not isinstance(declared, Mapping):
        raise ClaimError(
            "The adapter must name the properties it cannot express by the request that reads them."
        )
    for api, names in declared.items():
        if api not in APIS:
            raise ClaimError(
                f"The adapter names the facility {json.dumps(api, default=repr)}; "
                f"the facilities are {', '.join(APIS)}."
            )
        if (
            isinstance(names, str)
            or not isinstance(names, Sequence)
            or not all(_entry(entry) for entry in names)
        ):
            raise ClaimError(
                f"The adapter must name what it cannot express of a {api} request "
                "as a list of property names, or of entries each naming a property "
                "with the values or the request it cannot be expressed in."
            )


_ENTRY_KEYS = frozenset({"property", "values", "when"})


def _entry(entry: object) -> bool:
    # An entry is a property name, or an object naming one and narrowing it to
    # values or to a condition, neither of them empty: an empty list of values
    # would claim nothing, and an empty condition would read as one while
    # narrowing nothing.
    if isinstance(entry, str):
        return True
    if not isinstance(entry, Mapping) or not set(entry) <= _ENTRY_KEYS:
        return False
    values, when = entry.get("values"), entry.get("when")
    return (
        isinstance(entry.get("property"), str)
        and (values is None or (isinstance(values, list | tuple) and len(values) > 0))
        and (when is None or (isinstance(when, Mapping) and len(when) > 0))
    )


def _kept_claims(adapter: Adapter) -> None:
    # The statements an adapter makes about kept inputs and the bound on
    # them (sections 11.3, 13). Section 13 sets no minimum for the bound on
    # counted reads, so 0 is one an implementation may keep; a bound below
    # it, or not a count, is no bound.
    reentry, keeps, resolutions = adapter.reentry, adapter.keeps, adapter.resolutions
    if reentry is not None and (
        isinstance(reentry, bool) or not isinstance(reentry, int) or reentry < 0
    ):
        raise ClaimError(
            "The adapter must state its bound on counted reads, reentry, as a non-negative integer."
        )
    if keeps is not None and not isinstance(keeps, bool):
        raise ClaimError(
            "The adapter must state whether a modifier can keep an input past its call, "
            "keeps, as true or false."
        )
    if resolutions is not None and not isinstance(resolutions, bool):
        raise ClaimError(
            "The adapter must state whether its bound counts a resolution asked for inside "
            "another, resolutions, as true or false."
        )


def _tree(adapter: Adapter) -> None:
    # CST.md section 4: a tree whose unit is unstated says nothing about where
    # anything is, so one offered without it is refused as an unknown level is.
    cst = adapter.cst
    if cst is None:
        return
    if not isinstance(cst, Cst) or not callable(cst.parse):
        raise ClaimError(
            "An adapter that offers a tree must supply the call that produces one as cst.parse."
        )
    if cst.unit not in UNITS:
        raise ClaimError(
            f"The adapter counts its spans in {json.dumps(cst.unit, default=repr)}; "
            f"the units are {', '.join(UNITS)}."
        )


def _claims(adapter: Adapter, levels: Sequence[str] | None) -> None:
    # The adapter's statements about itself, and the levels asked for, are
    # held to the vocabulary of sections 2, 11.2, 11.5 and 13 before anything
    # runs: a level that is not one of the three, or a limit that is not a
    # count, is an error, not a skip.
    claimed = _levels(adapter.levels, "The adapter")
    if "core" not in claimed:
        raise ClaimError("The adapter must claim the core level.")
    if levels is not None:
        for level in _levels(levels, "The levels option"):
            if level not in claimed:
                raise ClaimError(f"The adapter does not claim the {level} level.")
    declared = adapter.limits if isinstance(adapter.limits, Mapping) else {}
    for name in LIMITS:
        limit = declared.get(name)
        # A bool is an int in Python, and True would claim a limit of 1.
        if isinstance(limit, bool) or not isinstance(limit, int) or limit < 1:
            raise ClaimError(f"The adapter must declare its {name} limit as a positive integer.")
    _kept_claims(adapter)
    _unexpressible(adapter)
    _tree(adapter)


def _cannot(case: Mapping[str, object], adapter: Adapter) -> str | None:
    # A case whose request reads what the adapter documented it cannot
    # express (section 11.2) measures the host's facility rather than the
    # implementation, so it is left out the way an unclaimed level is.
    if "generate" in case or adapter.unexpressible is None:
        return None
    request = cast("Mapping[str, object]", case["expected"]).get("format")
    if not isinstance(request, Mapping):
        return None
    api = cast("str", request["api"])
    options = cast("Mapping[str, object]", request.get("options") or {})
    for entry in adapter.unexpressible.get(api, ()):
        found = covers(entry, api, options)
        if found is not None:
            return found
    return None


_INPUTS = ("message", "payload", "props", "messageId", "defaults")

REFUSING = (
    "The case hands over a value whose entries raise where they are read, where this "
    "implementation enumerates it rather than looks its entries up by name: a dict entry "
    "raises only on a lookup by a name its key collides with, never where its storage is "
    "enumerated, so this host has no such value there."
)

# The inputs the implementation reads by name: the payload by a placeholder's
# key, and the props and the defaults by a modifier's name. What they hold, a
# modifier's layer among it, it enumerates, but for the digit entries `plural`
# looks up by name in the `number` layer, and so it does a message it converts.
_BY_NAME = ("payload", "props", "defaults")

_STAND_IN = (
    "The adapter states no stand-in for undefined, in which the construction counts its nodes."
)


def _host(case: Mapping[str, object], adapter: Adapter) -> str | None:
    # What this host builds no value for: a refusing tag where the
    # implementation enumerates it (RUNNER.md, *Decoding*), and the host's
    # undefined where the adapter states none.
    if any(
        refuses(case[name]) and not (name in _BY_NAME and is_refusing(case[name]))
        for name in _INPUTS
        if name in case
    ):
        return REFUSING
    if case.get("generate") in UNDEFINED_COUNTED and adapter.undefined is OMIT:
        return _STAND_IN
    return None


# The behaviours that read a kept input only after the modifier that kept it
# returned: ``keep`` reads nothing it keeps, so only a later modifier does,
# and ``first`` reads only what an earlier one kept.
_LATER = ("keep", "first")

_KEEPS = "The adapter states that a modifier cannot keep an input past its call."

# The behaviours that ask for a resolution inside another.
_NESTS = ("nest", "keep-nest", "keep-nest-ne")

_NESTED = (
    "The adapter states a bound of 0 on counted reads, so the resolution "
    "a modifier asks for inside another may be refused."
)


def _counting(adapter: Adapter) -> str | None:
    # The construction of resolutions one inside another keeps no input, so
    # what the adapter states of keeping does not reach it. It measures a
    # bound that counts resolutions, and writes two brackets for each that
    # stands, which the output limit has to hold.
    if adapter.reentry is None:
        return "The adapter states no bound on counted reads."
    if adapter.resolutions is not True:
        return (
            "The adapter does not state that its bound counts a resolution asked for "
            "inside another."
        )
    if len(renested(adapter.reentry)) > cast("int", adapter.limits["output"]):
        return (
            "The output limit the adapter states does not hold the brackets of as many "
            "resolutions as its bound lets stand."
        )
    return None


def _kept(case: Mapping[str, object], adapter: Adapter) -> str | None:
    # A case built from the bound on counted reads, or one that reads a kept
    # input after the modifier that kept it returned, measures what the adapter
    # stated it does not do. So is a resolution asked for inside another at a
    # bound of 0, which an implementation whose bound counts resolutions refuses.
    if "generate" not in case:
        registered = list(cast("Mapping[str, str]", case.get("keeping") or {}).values())
        if adapter.keeps is False and any(name in _LATER for name in registered):
            return _KEEPS
        if adapter.reentry == 0 and any(name in _NESTS for name in registered):
            return _NESTED
        return None
    if case["generate"] == RESOLUTIONS:
        return _counting(adapter)
    if case["generate"] not in COUNTED:
        return None
    framed = case["generate"] in FRAMED
    if adapter.keeps is False and not (framed and adapter.resolutions is True):
        return _KEEPS
    if adapter.reentry is None:
        return "The adapter states no bound on counted reads."
    if case["generate"] == "reentry-own-reads-free" and adapter.reentry == 0:
        return (
            "The adapter states a bound of 0 on counted reads, "
            "so no counted read stands for own reads to be made inside."
        )
    if framed and adapter.reentry == 0:
        return _NESTED
    return None


def _reason(
    case: Mapping[str, object], level: str, adapter: Adapter, levels: Sequence[str] | None
) -> str | None:
    if level not in adapter.levels:
        return f"The adapter does not claim the {level} level."
    if levels is not None and level not in levels:
        return f"The {level} level is not among the levels being run."
    return _cannot(case, adapter) or _host(case, adapter) or _kept(case, adapter)


def _readable(chosen: Sequence[Fixture]) -> None:
    # A file whose format this runner does not read is refused rather than
    # skipped: what a runner could not read is not what an implementation passed.
    for fixture in chosen:
        if fixture.file.get("format") != FORMAT:
            raise SetError(
                f"The fixture file {fixture.name} targets the format "
                f"{json.dumps(fixture.file.get('format'), default=repr)}; "
                f"this runner reads {FORMAT}."
            )


def _entries(
    adapter: Adapter, levels: Sequence[str] | None, fixture: Fixture
) -> list[tuple[Identity, functools.partial[Outcome], str | None]]:
    file = fixture.file
    cases = cast("Sequence[Mapping[str, object]]", file["cases"])
    section = cast("str", file.get("section", ""))
    if file.get("kind") == "tree":
        # A tree file declares no level, because CST.md is not one of them: its
        # cases run where the adapter offers a tree.
        offered = None if adapter.cst is not None else "The adapter offers no concrete syntax tree."
        return [
            (
                Identity(
                    id=cast("str", case["id"]),
                    file=fixture.name,
                    section=cast("str", case.get("section", section)),
                    description=cast("str", case["description"]),
                    document="CST.md",
                ),
                functools.partial(execute_tree, adapter, case),
                offered,
            )
            for case in cases
        ]
    level = cast("str", file["level"])
    return [
        (
            Identity(
                id=cast("str", case["id"]),
                file=fixture.name,
                section=cast("str", case.get("section", section)),
                description=cast("str", case["description"]),
                level=level,
            ),
            functools.partial(
                execute_generated if "generate" in case else execute_concrete, adapter, case
            ),
            _reason(case, level, adapter, levels),
        )
        for case in cases
    ]


def plan(
    adapter: Adapter, *, levels: Sequence[str] | None = None, files: Sequence[Fixture] | None = None
) -> Plan:
    """One entry per case the adapter's levels require, ready to run, beside the cases left out."""
    _claims(adapter, levels)
    chosen = fixtures() if files is None else files
    _readable(chosen)
    planned = [entry for fixture in chosen for entry in _entries(adapter, levels, fixture)]
    return Plan(
        cases=tuple(
            Planned(identity, execute) for identity, execute, skip in planned if skip is None
        ),
        skipped=tuple(Skipped(identity, skip) for identity, _, skip in planned if skip is not None),
    )


def run(
    adapter: Adapter, *, levels: Sequence[str] | None = None, files: Sequence[Fixture] | None = None
) -> Result:
    """Executes a plan and sorts the outcomes."""
    planned = plan(adapter, levels=levels, files=files)
    executed = [(case, case.execute()) for case in planned.cases]
    return Result(
        passed=tuple(case for case, outcome in executed if isinstance(outcome, Passed)),
        failed=tuple((case, outcome) for case, outcome in executed if isinstance(outcome, Failed)),
        skipped=planned.skipped,
        unobserved=tuple(
            case for case, outcome in executed if isinstance(outcome, Passed) and outcome.unobserved
        ),
    )


def _where(identity: Identity) -> str:
    # The document is named only where it is not the specification.
    return (
        f"{identity.document} section {identity.section}"
        if identity.document
        else f"section {identity.section}"
    )


def _failed(case: Planned, outcome: Failed) -> str:
    identity = case.identity
    return (
        f"FAIL {identity.id} ({_where(identity)}): {outcome.reason} "
        f"Expected {shown(outcome.expected)}, actual {shown(outcome.actual)}. "
        f"{identity.description}"
    )


def _skipped(skipped: Skipped) -> str:
    return f"SKIP {skipped.identity.id} ({_where(skipped.identity)}): {skipped.reason}"


def summarize(result: Result) -> str:
    """One line per failure and per case left out, then the counts."""
    counts = (
        f"{len(result.passed)} passed, {len(result.failed)} failed, {len(result.skipped)} skipped"
    )
    unobserved = (
        f", {len(result.unobserved)} passed with reports unobserved" if result.unobserved else ""
    )
    return "\n".join(
        [
            *(_failed(case, outcome) for case, outcome in result.failed),
            *(_skipped(skipped) for skipped in result.skipped),
            counts + unobserved,
        ]
    )


def check(
    adapter: Adapter, *, levels: Sequence[str] | None = None, files: Sequence[Fixture] | None = None
) -> None:
    """Runs the set and raises where anything failed, listing every failure."""
    result = run(adapter, levels=levels, files=files)
    if result.failed:
        total = len(result.passed) + len(result.failed)
        lines = "\n".join(_failed(case, outcome) for case, outcome in result.failed)
        raise AssertionError(f"{len(result.failed)} of {total} conformance cases failed:\n{lines}")


def _verdict(result: Result, baseline: Result) -> Observed:
    # What the runner answered for a defect, read against what it answered
    # without one.
    if len(result.failed) > len(baseline.failed):
        return "fail"
    if len(result.skipped) > len(baseline.skipped):
        return "skip"
    if len(result.unobserved) > len(baseline.unobserved):
        return "unobserved"
    return "none"


def audit(
    adapter: Adapter, *, levels: Sequence[str] | None = None, files: Sequence[Fixture] | None = None
) -> list[Audited]:
    """Runs the defect catalogue against an adapter that passes the set.

    Answers, for each defect, whether this runner caught it, missed it, or
    never met it. Only a refused claim answers ``error``: anything else an
    altered adapter raises out of the run is a defect of this runner, and
    ends the audit.
    """
    baseline = run(adapter, levels=levels, files=files)
    if baseline.failed:
        total = len(baseline.passed) + len(baseline.failed)
        raise SetError(
            "The audit needs an adapter that passes the set; "
            f"this one fails {len(baseline.failed)} of {total} cases."
        )
    within = Run(
        set=fixtures() if files is None else files,
        levels=tuple(level for level in adapter.levels if levels is None or level in levels),
    )
    audited: list[Audited] = []
    for entry in defects():
        if entry.id not in MUTATIONS:
            raise SetError(
                f"The catalogue names a defect this runner does not carry: {json.dumps(entry.id)}."
            )
        mutation = realize(entry.id, adapter, within)
        observed: Observed
        try:
            observed = _verdict(run(mutation.adapter, levels=levels, files=files), baseline)
        except ClaimError:
            observed = "error"
        outcome: Caught = (
            "unreachable"
            if not mutation.reached()
            else "caught"
            if observed == entry.expects
            else "missed"
        )
        audited.append(Audited(entry, observed, outcome))
    return audited
