"""The conformance set, run through the adapter of section 14.3, and the defect audit over it."""

from __future__ import annotations

import collections
import dataclasses
from collections.abc import Iterator

import pytest
import runner
from adapter import adapter
from runner import Adapter, Passed, Planned, Resolution, Resolved
from runner.cases import shown

PLAN = runner.plan(adapter)

INTL = {"intl.json": 126, "plural.json": 87, "fallback-intl.json": 2}
"""The files of the Intl level, and how many cases each holds."""

UNREACHABLE_WITHOUT_INTL = {"formats-wrong", "unexpressible-declared"}

REPORTS = {
    "reports-dropped",
    "reports-extra",
    "reports-reversed",
    "report-code-changed",
    "report-origin-changed",
    "report-id-changed",
    "report-limit-changed",
    "reports-unobserved",
}

TREES = {
    "tree-unoffered",
    "tree-node-dropped",
    "tree-node-retyped",
    "tree-span-shifted",
    "tree-name-raw",
    "tree-cancels-inverted",
    "tree-unit-changed",
    "tree-unit-unknown",
}


def _cases() -> Iterator[object]:
    for planned in PLAN.cases:
        yield pytest.param(planned, id=planned.identity.id)
    for skipped in PLAN.skipped:
        yield pytest.param(
            None, id=skipped.identity.id, marks=pytest.mark.skip(reason=skipped.reason)
        )


@pytest.mark.parametrize("planned", list(_cases()))
def test_case(planned: Planned) -> None:
    outcome = planned.execute()
    assert outcome == Passed(), (
        f"{getattr(outcome, 'reason', '')} Expected {shown(getattr(outcome, 'expected', None))}, "
        f"actual {shown(getattr(outcome, 'actual', None))}. {planned.identity.description}"
    )


def test_the_adapter_claims_what_the_leg_installed(expect_intl: bool) -> None:
    assert tuple(adapter.levels) == (
        ("core", "extensions", "intl") if expect_intl else ("core", "extensions")
    )


def test_the_cases_left_out_are_the_intl_level_s_where_it_is_not_claimed(
    expect_intl: bool,
) -> None:
    trees = [planned for planned in PLAN.cases if planned.identity.document == "CST.md"]
    assert len(trees) == 44
    if expect_intl:
        assert PLAN.skipped == ()
        assert len(PLAN.cases) == 771
        return
    assert collections.Counter(skipped.identity.file for skipped in PLAN.skipped) == INTL
    assert {skipped.reason for skipped in PLAN.skipped} == {
        "The adapter does not claim the intl level."
    }
    assert len(PLAN.cases) == 556


def _audited(audited: Adapter) -> dict[str, str]:
    return {entry.entry.id: entry.outcome for entry in runner.audit(audited)}


def _expected(names: set[str], unreachable: set[str]) -> dict[str, str]:
    return {name: "unreachable" if name in unreachable else "caught" for name in names}


def test_the_audit_catches_every_defect_the_adapter_can_carry(expect_intl: bool) -> None:
    audited = _audited(adapter)
    assert len(audited) == 26
    assert audited == _expected(set(audited), set() if expect_intl else UNREACHABLE_WITHOUT_INTL)


def _without_reports(resolution: Resolution) -> object:
    resolved = adapter.resolve(resolution)
    assert isinstance(resolved, Resolved)
    return dataclasses.replace(resolved, reports=None)


def test_the_report_defects_never_reach_an_adapter_that_observes_no_reports(
    expect_intl: bool,
) -> None:
    audited = _audited(dataclasses.replace(adapter, resolve=_without_reports))
    unreachable = REPORTS | (set() if expect_intl else UNREACHABLE_WITHOUT_INTL)
    assert audited == _expected(set(audited), unreachable)


def test_the_tree_defects_never_reach_an_adapter_that_offers_no_tree(expect_intl: bool) -> None:
    audited = _audited(dataclasses.replace(adapter, cst=None))
    unreachable = TREES | (set() if expect_intl else UNREACHABLE_WITHOUT_INTL)
    assert audited == _expected(set(audited), unreachable)


def _without_requests(resolution: Resolution) -> object:
    resolved = adapter.resolve(resolution)
    assert isinstance(resolved, Resolved)
    return dataclasses.replace(resolved, formats=None)


def test_the_set_s_text_reading_agrees_with_the_requests_the_adapter_makes() -> None:
    result = runner.run(dataclasses.replace(adapter, resolve=_without_requests))
    assert result.failed == (), runner.summarize(result)
    assert len(result.passed) == len(PLAN.cases)
