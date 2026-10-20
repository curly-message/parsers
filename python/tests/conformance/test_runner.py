"""The runner held to RUNNER.md, through toy adapters over in-memory sets.

None of these tests reads ``curly_message``: what the runner answers is a
function of the set and of what an adapter answers, so each test states both.
"""

from __future__ import annotations

import json
import sys
from collections.abc import Callable, Mapping, Sequence
from dataclasses import replace
from pathlib import Path
from typing import Any

import fetch
import pytest
import runner
from runner import (
    OMIT,
    Adapter,
    Call,
    ClaimError,
    Cst,
    Failed,
    Fixture,
    Passed,
    Resolution,
    Resolved,
    SetError,
    archive,
    audit,
    behaviours,
    cases,
    decode,
    plan,
    run,
    summarize,
    tree,
)
from runner.decode import nodes
from runner.defects import MUTATIONS

FORMAT = "curly-message-3"

LIMITS = {"output": 10, "read": 4, "conversion": 7, "nesting": 3}

Answer = Resolved | Callable[[Resolution], object]


def canned(answers: Mapping[str, Answer], **claims: Any) -> Adapter:
    """An adapter answering each message from a table: correct exactly where the table is."""

    def resolve(resolution: Resolution) -> object:
        answer = answers[str(resolution.message)]
        return answer(resolution) if callable(answer) else answer

    fields: dict[str, Any] = {"levels": ("core",), "limits": LIMITS, "resolve": resolve, **claims}
    return Adapter(**fields)


def resolution_file(level: str, *written: Mapping[str, object]) -> Fixture:
    return Fixture(
        f"{level}.json", {"format": FORMAT, "level": level, "section": "8", "cases": list(written)}
    )


def tree_file(*written: Mapping[str, object]) -> Fixture:
    return Fixture(
        "cst.json", {"format": FORMAT, "kind": "tree", "section": "6", "cases": list(written)}
    )


def case(
    name: str, message: object, expected: Mapping[str, object], **fields: object
) -> dict[str, object]:
    return {
        "id": f"toy/{name}",
        "description": f"The toy case {name}.",
        "message": message,
        "expected": expected,
        **fields,
    }


def outcome(written: Mapping[str, object], answer: Answer, **claims: Any) -> object:
    return cases.execute_concrete(canned({str(written["message"]): answer}, **claims), written)


# The set as released.


def test_the_pinned_archive_holds_the_set_as_released() -> None:
    shipped = runner.fixtures()
    names = [fixture.name for fixture in shipped]
    assert names == sorted(names)
    assert len(names) == 18
    counted = {fixture.name: len(fixture.file["cases"]) for fixture in shipped}  # type: ignore[arg-type]
    assert sum(counted.values()) == 771
    assert counted["cst.json"] == 44


def test_an_archive_whose_digest_is_not_the_pinned_one_is_refused(tmp_path: Path) -> None:
    data = bytearray(archive.ARCHIVE.read_bytes())
    data[-1] ^= 1
    altered = tmp_path / archive.ARCHIVE.name
    altered.write_bytes(bytes(data))
    with pytest.raises(SetError, match="not the pinned"):
        archive._members(altered)


def test_a_file_of_another_format_is_refused_before_its_schema_is_read() -> None:
    with pytest.raises(SetError, match=r"targets the format \"curly-message-2\""):
        archive.validated("old.json", {"format": "curly-message-2"})


def test_a_file_that_breaks_the_schema_is_refused() -> None:
    broken = {
        "format": FORMAT,
        "level": "core",
        "section": "8",
        "cases": [{"id": "toy/a", "message": ""}],
    }
    with pytest.raises(SetError, match=r"does not match fixture\.schema\.json"):
        archive.validated("broken.json", broken)


def test_a_set_under_development_is_read_sorted_and_validated(tmp_path: Path) -> None:
    written = case("a", "a", {"output": "a"})
    for name in ("b.json", "a.json"):
        (tmp_path / name).write_text(
            json.dumps(resolution_file("core", written).file), encoding="utf-8"
        )
    assert [fixture.name for fixture in runner.load(tmp_path)] == ["a.json", "b.json"]
    (tmp_path / "c.json").write_text(
        json.dumps({"format": FORMAT, "level": "core"}), encoding="utf-8"
    )
    with pytest.raises(SetError, match=r"c\.json"):
        runner.load(tmp_path)


def test_a_plan_refuses_a_file_of_another_format_handed_to_it() -> None:
    other = Fixture(
        "old.json", {"format": "curly-message-2", "level": "core", "section": "8", "cases": []}
    )
    with pytest.raises(SetError, match=r"old\.json targets the format"):
        plan(canned({}), files=[other])


def test_fetch_writes_the_archive_only_once_its_digest_is_the_stated_one(tmp_path: Path) -> None:
    source = archive.ARCHIVE.as_uri()
    version, digest = archive.PIN
    with pytest.raises(ValueError, match="not the stated"):
        fetch.fetch(version, "0" * 64, source=source, into=tmp_path)
    assert list(tmp_path.iterdir()) == []
    written = fetch.fetch(version, digest, source=source, into=tmp_path)
    assert written == tmp_path / archive.ARCHIVE.name
    assert written.read_bytes() == archive.ARCHIVE.read_bytes()


@pytest.mark.parametrize(
    ("version", "digest", "source"),
    [
        ("4.2", "0" * 64, None),
        ("4.2.0", "0" * 63, None),
        ("4.2.0", "A" * 64, None),
        ("4.2.0", "0" * 64, "http://example.invalid/conformance-4.2.0.zip"),
    ],
)
def test_fetch_refuses_what_it_was_not_asked_for_in_its_own_terms(
    version: str, digest: str, source: str | None
) -> None:
    with pytest.raises(ValueError):
        fetch.fetch(version, digest, source=source)


def test_fetch_addresses_the_release_asset() -> None:
    assert fetch.address("4.1.0") == (
        "https://github.com/curly-message/spec/releases/download/conformance-v4.1.0/conformance-4.1.0.zip"
    )


# Decoding the tags.


def test_the_undefined_tag_is_what_the_adapter_states_or_left_out() -> None:
    absent = object()
    tagged = {"a": {"$curly": "undefined"}, "b": [1, {"$curly": "undefined"}, 2]}
    assert decode(tagged, absent) == {"a": absent, "b": [1, absent, 2]}
    assert decode(tagged) == {"b": [1, 2]}
    assert decode({"$curly": "undefined"}) is OMIT
    assert decode({"$curly": "undefined"}, absent) is absent


def test_the_unserializable_tag_holds_itself() -> None:
    value = decode({"v": {"$curly": "unserializable"}})
    held = value["v"]  # type: ignore[index]
    assert held["self"] is held
    with pytest.raises(ValueError, match="Circular"):
        json.dumps(held)


def _visits(value: object) -> int:
    count, stack = 0, [value]
    while stack:
        current = stack.pop()
        count += 1
        if isinstance(current, list):
            stack.extend(current)
    return count


@pytest.mark.parametrize("count", [1, 2, 7, 8, 100001])
def test_the_nodes_tag_visits_at_least_its_count_and_less_than_twice_it(count: int) -> None:
    built = decode({"$curly": "nodes", "count": count})
    assert built == nodes(count)
    assert count <= _visits(built) < 2 * count + 1


@pytest.mark.parametrize(
    "tag", [{"$curly": "nothing"}, {"$curly": "nodes"}, {"$curly": "nodes", "count": True}]
)
def test_a_tag_the_set_does_not_define_ends_the_run(tag: dict[str, object]) -> None:
    with pytest.raises(SetError):
        decode({"v": tag})


def test_decoding_walks_input_deeper_than_the_host_stack() -> None:
    depth = sys.getrecursionlimit() * 3
    deep: list[object] = []
    for _ in range(depth):
        deep = [deep]
    decoded = decode(deep)
    reached = 0
    while decoded:
        decoded = decoded[0]  # type: ignore[index]
        reached += 1
    assert reached == depth


# The catalogue of behaviours.


class _Option:
    def __init__(self, key: str, value: str) -> None:
        self.key = key
        self.value = value


def _call(**fields: Any) -> Call:
    given: dict[str, Any] = {
        "value": "ab\xe9",
        "options": (),
        "props": {},
        "locale": None,
        "default": lambda: "D",
    }
    return Call(**{**given, **fields})


def test_each_behaviour_answers_as_the_set_s_readme_states() -> None:
    absent = object()
    table = behaviours(absent)
    assert table["upper"](_call()) == "AB\xe9"
    assert table["echo"](_call()) == "ab\xe9"
    assert table["empty"](_call()) == ""
    assert table["nothing"](_call()) is absent
    assert behaviours()["nothing"](_call()) is None
    assert table["default"](_call()) == "D"
    assert table["options"](_call(options=(_Option("a", "1"), _Option("a", "2")))) == "a=1,a=2"
    assert table["props"](_call(props={"\ue000": 1, "\U0001f600": [True], "b": "x"})) == (
        'b="x",\U0001f600=[true],\ue000=1'
    )
    assert table["locale"](_call()) == "none"
    assert table["locale"](_call(locale=absent)) == "none"
    assert table["locale"](_call(locale="cs")) == "cs"
    assert table["object"](_call()) == {"answer": "ab\xe9"}
    with pytest.raises(RuntimeError):
        table["raise"](_call())


# Claims.


@pytest.mark.parametrize(
    ("claims", "message"),
    [
        ({"levels": "core"}, "must list levels"),
        ({"levels": ("core", "ecmascript")}, 'names the level "ecmascript"'),
        ({"levels": ("intl",)}, "must claim the core level"),
        ({"limits": {**LIMITS, "nesting": True}}, "nesting limit as a positive integer"),
        ({"limits": {**LIMITS, "read": 0}}, "read limit as a positive integer"),
        ({"limits": {"output": 1, "read": 1, "conversion": 1}}, "nesting limit"),
        ({"limits": {**LIMITS, "output": 2.0}}, "output limit"),
        ({"unexpressible": {"Collator": ["usage"]}}, 'names the facility "Collator"'),
        ({"unexpressible": {"NumberFormat": "style"}}, "as a list of property names"),
        ({"cst": Cst("utf-32", lambda message: {})}, 'counts its spans in "utf-32"'),
    ],
)
def test_a_claim_outside_the_vocabulary_refuses_the_run(
    claims: dict[str, Any], message: str
) -> None:
    with pytest.raises(ClaimError, match=message):
        plan(canned({}, **claims), files=[])


def test_the_levels_asked_for_must_be_levels_the_adapter_claims() -> None:
    with pytest.raises(ClaimError, match="does not claim the intl level"):
        plan(canned({}), levels=["intl"], files=[])
    with pytest.raises(ClaimError, match='The levels option names the level "all"'):
        plan(canned({}), levels=["all"], files=[])


# Planning.

_REQUESTED = {
    "format": {"api": "NumberFormat", "options": {"maximumFractionDigits": 2}, "input": 1}
}

_SET = [
    resolution_file("core", case("core", "c", {"output": "c"})),
    resolution_file(
        "intl", case("intl", "{{n:number}}", _REQUESTED, payload={"n": 1}, locale="en")
    ),
    resolution_file("extensions", case("extensions", "e", {"output": "e"})),
    tree_file({"id": "cst/toy", "description": "A toy tree case.", "message": "", "expected": []}),
]


def _left_out(adapter: Adapter, levels: Sequence[str] | None = None) -> list[tuple[str, str]]:
    return [
        (skipped.identity.id, skipped.reason)
        for skipped in plan(adapter, levels=levels, files=_SET).skipped
    ]


def test_every_case_left_out_says_why() -> None:
    assert _left_out(canned({})) == [
        ("toy/intl", "The adapter does not claim the intl level."),
        ("toy/extensions", "The adapter does not claim the extensions level."),
        ("cst/toy", "The adapter offers no concrete syntax tree."),
    ]
    every = canned({}, levels=("core", "intl", "extensions"), cst=Cst("utf-16", lambda message: {}))
    assert _left_out(every) == []
    assert _left_out(every, levels=["core", "extensions"]) == [
        ("toy/intl", "The intl level is not among the levels being run."),
    ]
    cannot = replace(
        every, unexpressible={"NumberFormat": ["useGrouping", "maximumFractionDigits"]}
    )
    assert _left_out(cannot) == [
        (
            "toy/intl",
            "The adapter cannot express the maximumFractionDigits property "
            "of a NumberFormat request.",
        ),
    ]


def test_a_planned_case_names_where_its_sentence_is_written() -> None:
    planned = plan(
        canned({}, levels=("core", "intl", "extensions"), cst=Cst("utf-8", lambda m: {})),
        files=_SET,
    )
    identities = [entry.identity for entry in planned.cases]
    assert [(i.id, i.level, i.document, i.section) for i in identities] == [
        ("toy/core", "core", None, "8"),
        ("toy/intl", "intl", None, "8"),
        ("toy/extensions", "extensions", None, "8"),
        ("cst/toy", None, "CST.md", "6"),
    ]


# Executing a resolution case.


def test_the_output_is_compared_code_point_for_code_point() -> None:
    written = case("exact", "m", {"output": "a\u0301"})
    assert outcome(written, Resolved("a\u0301", reports=[])) == Passed()
    assert outcome(written, Resolved("\xe1")) == Failed("The output differs.", "a\u0301", "\xe1")
    assert outcome(written, Resolved(" a\u0301")) == Failed(
        "The output differs.", "a\u0301", " a\u0301"
    )


def test_what_an_adapter_answers_must_be_a_resolution_at_once() -> None:
    written = case("shape", "m", {"output": "a"})

    def raising(resolution: Resolution) -> object:
        raise LookupError("boom")

    assert outcome(written, raising) == Failed("The adapter raised.", "a", "boom")
    assert outcome(written, lambda resolution: "a") == Failed(
        "The adapter answered with no resolution.", "a", "a"
    )
    assert isinstance(outcome(written, Resolved(output=1)), Failed)

    made: list[Any] = []

    async def later() -> Resolved:
        return Resolved("a")

    def asynchronous(resolution: Resolution) -> object:
        made.append(later())
        return made[-1]

    answered = outcome(written, asynchronous)
    assert isinstance(answered, Failed)
    assert answered.reason == "The adapter answered asynchronously; resolve must answer at once."
    assert made[0].cr_frame is None


def test_an_adapter_is_handed_the_decoded_inputs_and_nothing_a_case_leaves_out() -> None:
    seen: list[Resolution] = []

    def resolve(resolution: Resolution) -> Resolved:
        seen.append(resolution)
        return Resolved("a")

    written = case(
        "inputs", "m", {"output": "a"}, payload={"v": {"$curly": "undefined"}}, messageId=None
    )
    outcome(written, resolve)
    assert seen[0].payload == {}
    assert seen[0].id is None
    assert (seen[0].props, seen[0].locale, seen[0].defaults, seen[0].modifiers) == (
        OMIT,
        OMIT,
        OMIT,
        None,
    )
    absent = object()
    outcome(written, resolve, undefined=absent)
    assert seen[1].payload == {"v": absent}
    named = case("modifiers", "m", {"output": "a"}, modifiers={"shout": "upper"})
    outcome(named, resolve)
    assert seen[2].modifiers is not None
    assert seen[2].modifiers["shout"](_call()) == "AB\xe9"


def test_a_request_is_compared_by_its_entries() -> None:
    request = {"api": "NumberFormat", "options": {"a": 1, "b": True}, "input": 21}
    written = case("request", "m", {"format": request})
    same = {"api": "NumberFormat", "options": {"b": True, "a": 1.0}, "input": 21.0}
    assert outcome(written, Resolved("text", formats=[same])) == Passed(unobserved=True)
    assert isinstance(
        outcome(written, Resolved("text", formats=[{**same, "options": {"a": 1, "b": 1}}])), Failed
    )
    assert isinstance(outcome(written, Resolved("text", formats=[{**same, "input": "21"}])), Failed)
    assert isinstance(
        outcome(written, Resolved("text", formats=[{**same, "api": "PluralRules"}])), Failed
    )
    assert outcome(written, Resolved("text", formats=[same, same])) == Failed(
        "Expected one formatting request, got 2.", [request], [same, same]
    )
    bare = case("bare", "m", {"format": {"api": "NumberFormat", "input": 1}})
    assert outcome(
        bare, Resolved("text", formats=[{"api": "NumberFormat", "options": None, "input": 1}])
    ) == Passed(unobserved=True)


_CATEGORIES = {"zero": "", "one": "ONE", "two": "", "few": "FEW", "many": "", "other": "OTHER"}


def test_a_selection_is_held_to_the_category_its_request_carries() -> None:
    request = {"api": "PluralRules", "options": {"type": "cardinal"}, "input": 3}
    written = case("select", "m", {"format": request, "categories": _CATEGORIES})
    answered = {**request, "category": "few"}
    assert outcome(written, Resolved("FEW", formats=[answered])) == Passed(unobserved=True)
    assert outcome(written, Resolved("OTHER", formats=[answered])) == Failed(
        "The output differs from the one the category few selects.", "FEW", "OTHER"
    )
    assert isinstance(outcome(written, Resolved("FEW", formats=[request])), Failed)
    assert isinstance(
        outcome(written, Resolved("FEW", formats=[{**request, "category": "several"}])), Failed
    )


def test_a_selection_case_naming_too_few_categories_ends_the_run() -> None:
    request = {"api": "PluralRules", "options": {}, "input": 3}
    written = case(
        "select", "m", {"format": request, "categories": {"one": "ONE", "other": "OTHER"}}
    )
    with pytest.raises(SetError, match="'zero'"):
        outcome(written, Resolved("OTHER"))


def test_reports_are_compared_in_order_and_by_the_fields_both_sides_carry() -> None:
    expected: list[dict[str, object]] = [
        {"code": "unknown-modifier", "origin": "message", "id": "x"},
        {"code": "read-limit", "limit": 4},
    ]
    written = case("reports", "m", {"output": "", "reports": expected})
    assert (
        outcome(
            written, Resolved("", reports=[{"code": "unknown-modifier"}, {"code": "read-limit"}])
        )
        == Passed()
    )
    assert outcome(written, Resolved("", reports=list(reversed(expected)))) == Failed(
        "The code of report 1 differs.", expected[0], expected[1]
    )
    wrong = [{**expected[0], "origin": "payload"}, expected[1]]
    assert outcome(written, Resolved("", reports=wrong)) == Failed(
        "The origin of report 1 differs.", expected[0], wrong[0]
    )
    shifted = [expected[0], {**expected[1], "limit": 5}]
    assert outcome(written, Resolved("", reports=shifted)) == Failed(
        "The limit of report 2 differs.", expected[1], shifted[1]
    )
    assert outcome(written, Resolved("", reports=[])) == Failed(
        "Expected 2 reports, got 0 reports.", expected, []
    )
    assert outcome(written, Resolved("")) == Passed(unobserved=True)
    assert isinstance(outcome(written, Resolved("", reports="none")), Failed)  # type: ignore[arg-type]


def test_a_case_expecting_no_reports_fails_on_one() -> None:
    written = case("none", "m", {"output": ""})
    assert outcome(written, Resolved("", reports=[{"code": "unknown-modifier"}])) == Failed(
        "Expected 0 reports, got 1 report.", [], [{"code": "unknown-modifier"}]
    )


# The generated constructions, from the adapter's own numbers.


def test_the_eight_constructions_are_built_from_the_declared_limits() -> None:
    def built(name: str) -> tuple[object, object, object, object]:
        prepared = cases._generated(name, LIMITS)
        return (
            prepared.input.message,
            prepared.input.payload,
            prepared.expected.output,
            list(prepared.expected.reports),
        )

    limit = {"origin": "limit", "id": "limits"}
    assert built("output-at-limit") == ("{{v; a:xxxxxxxxxx;}}", {"v": "a"}, "x" * 10, [])
    assert built("output-over-limit") == (
        "A{{v; a:xxxxxxxxxxx;}}B",
        {"v": "a"},
        "AB",
        [{"code": "output-limit", **limit, "limit": 10}],
    )
    assert built("output-over-limit-continues") == (
        "{{v; a:xxxxxxxxxxx;}}{{w}}",
        {"v": "a", "w": "tail"},
        "tail",
        [{"code": "output-limit", **limit, "limit": 10}],
    )
    assert built("read-at-limit") == ("{{v:eq; nomatch:X; default:ok;}}", {"v": "xxxx"}, "ok", [])
    assert built("read-over-limit") == (
        "{{v:eq; nomatch:X; default:ok;}}{{w}}",
        {"v": "xxxxx", "w": "tail"},
        "ok",
        [{"code": "read-limit", **limit, "limit": 4}],
    )
    message, payload, output, reports = built("conversion-over-limit")
    assert (message, output, reports) == (
        "{{v; default:D}}",
        "D",
        [{"code": "unserializable-value", "origin": "payload", "id": "limits"}],
    )
    assert _visits(payload["v"]) >= 8  # type: ignore[index]
    assert built("nesting-at-limit") == (
        "{{v; a:{{v; a:{{v; a:settled; default:fallback;}};}};}}",
        {"v": "a"},
        "settled",
        [],
    )
    deeper = built("nesting-over-limit")
    assert str(deeper[0]).count("{{") == 4
    assert deeper[2:] == (
        "fallback",
        [{"code": "nesting-limit", "origin": "message", "id": "limits", "limit": 3}],
    )
    with pytest.raises(SetError):
        cases._generated("output-under-limit", LIMITS)


def test_a_generated_case_runs_through_the_adapter_with_its_id() -> None:
    seen: list[Resolution] = []

    def resolve(resolution: Resolution) -> Resolved:
        seen.append(resolution)
        return Resolved("x" * 10, reports=[])

    adapter = canned({"{{v; a:xxxxxxxxxx;}}": resolve})
    generated = {"id": "limits/output-at-limit", "description": "", "generate": "output-at-limit"}
    assert cases.execute_generated(adapter, generated) == Passed()
    assert seen[0].id == "limits"


# Tree cases.


def test_boundaries_count_the_declared_unit() -> None:
    message = "a\U0001f600\xe9"
    assert tree.boundaries(message, "code-point") == [0, 1, 2, 3]
    assert tree.boundaries(message, "utf-16") == [0, 1, None, 2, 3]
    assert tree.boundaries(message, "utf-8") == [0, 1, None, None, None, 2, None, 3]


def _text(start: int, end: int) -> dict[str, object]:
    return {"type": "text", "start": start, "end": end}


def _root(end: int, *children: object) -> dict[str, object]:
    return {"type": "message", "start": 0, "end": end, "nodes": list(children)}


def _tree_case(message: str, expected: list[object], **fields: object) -> dict[str, object]:
    return {
        "id": "cst/toy",
        "description": "A toy tree case.",
        "message": message,
        "expected": expected,
        **fields,
    }


def _tree_outcome(
    written: Mapping[str, object], parse: Callable[[str], object], unit: str = "code-point"
) -> object:
    adapter = canned(
        {str(written["message"]): Resolved(str(written.get("resolves", "")))}, cst=Cst(unit, parse)
    )
    return tree.execute_tree(adapter, written)


def test_a_tree_that_reads_the_message_passes_every_step() -> None:
    written = _tree_case("ab", [{"type": "text", "text": "ab"}], resolves="ab")
    assert _tree_outcome(written, lambda message: _root(2, _text(0, 2))) == Passed()


@pytest.mark.parametrize(
    ("answer", "reason"),
    [
        (_root(1, _text(0, 1)), "The tree does not span the whole message."),
        (_root(2, _text(0, 1)), "The leaves do not reach the end of the message."),
        (_root(2, _text(1, 2), _text(0, 1)), "The leaves do not tile the message"),
        (_root(2, {"type": "word", "start": 0, "end": 2}), "no kind this document names"),
        (_root(2, {**_text(0, 2), "start": False}), "is not a pair of offsets"),
        (_root(2, {**_text(0, 2), "nodes": "ab"}), "are not a list"),
        (_root(2, {"type": "key", "start": 0, "end": 2}), "carries no name"),
        (
            _root(2, {"type": "escape", "start": 0, "end": 2, "cancels": 1}),
            "does not say which reading",
        ),
        (_root(2, _text(0, 3)), "lies outside the message"),
        ({"type": "text", "start": 0, "end": 2}, "answered with no tree"),
        (
            _root(2, _text(0, 1), {"type": "space", "start": 1, "end": 2}),
            "Node 2 of the message is of another kind.",
        ),
    ],
)
def test_a_tree_is_failed_at_the_first_thing_wrong_with_it(answer: object, reason: str) -> None:
    written = _tree_case("ab", [{"type": "text", "text": "a"}, {"type": "text", "text": "b"}])
    failed = _tree_outcome(written, lambda message: answer)
    assert isinstance(failed, Failed)
    assert reason in failed.reason


def test_a_span_inside_a_code_point_of_the_declared_unit_fails() -> None:
    written = _tree_case("\U0001f600", [{"type": "text", "text": "\U0001f600"}])
    failed = _tree_outcome(
        written, lambda message: _root(2, _text(0, 1), _text(1, 2)), unit="utf-16"
    )
    assert isinstance(failed, Failed)
    assert failed.reason == "The span of the text node falls inside a code point."
    assert _tree_outcome(written, lambda message: _root(2, _text(0, 2)), unit="utf-16") == Passed()


def test_a_tree_that_holds_itself_fails_instead_of_hanging_the_run() -> None:
    cyclic = _root(2)
    cyclic["nodes"] = [cyclic]
    failed = _tree_outcome(_tree_case("ab", []), lambda message: cyclic)
    assert failed == Failed(
        "The tree holds more nodes than the message can spell.", "at most 72 nodes", "more"
    )


def test_a_tree_deeper_than_the_host_stack_is_walked() -> None:
    depth = sys.getrecursionlimit() * 3
    message = "a" * depth
    deep: dict[str, object] = _text(0, depth)
    for _ in range(depth):
        deep = {"type": "placeholder", "start": 0, "end": depth, "nodes": [deep]}
    failed = _tree_outcome(_tree_case(message, []), lambda text: {**deep, "type": "message"})
    assert failed == Failed(
        "The children of the message are 1 nodes rather than 0.", [], ["placeholder"]
    )


def test_a_tree_is_a_function_of_the_message_alone() -> None:
    answers = iter([_root(2, _text(0, 2)), _root(2, _text(0, 1), _text(1, 2))])
    failed = _tree_outcome(
        _tree_case("ab", [{"type": "text", "text": "ab"}]), lambda message: next(answers)
    )
    assert isinstance(failed, Failed)
    assert (
        failed.reason == "The tree is not a function of the message alone: two parses of it differ."
    )


def test_a_tree_and_its_resolution_read_the_same_placeholders() -> None:
    written = _tree_case("ab", [{"type": "text", "text": "ab"}], resolves="ab")
    adapter = canned(
        {"ab": Resolved("a")}, cst=Cst("code-point", lambda message: _root(2, _text(0, 2)))
    )
    assert tree.execute_tree(adapter, written) == Failed(
        "The tree and the resolution do not read the same placeholders.", "ab", "a"
    )


def test_a_tree_the_adapter_raised_over_fails_the_case() -> None:
    def parse(message: str) -> object:
        raise ValueError("no tree")

    assert _tree_outcome(_tree_case("ab", []), parse) == Failed(
        "The adapter raised rather than answering with a tree.", "a message node", "no tree"
    )


# The result and its summary.


def test_the_summary_lists_each_failure_and_each_case_left_out_then_the_counts() -> None:
    fixtures = [
        resolution_file(
            "core", case("good", "g", {"output": "g"}), case("bad", "b", {"output": "b"})
        ),
        resolution_file("intl", case("intl", "i", {"output": "i"})),
    ]
    adapter = canned({"g": Resolved("g"), "b": Resolved("x" * 300, reports=[])})
    result = run(adapter, files=fixtures)
    assert [entry.identity.id for entry in result.passed] == ["toy/good"]
    assert [entry.identity.id for entry in result.unobserved] == ["toy/good"]
    shown = json.dumps("x" * 300)[:200] + "..."
    assert summarize(result).splitlines() == [
        "FAIL toy/bad (section 8): The output differs. "
        f'Expected "b", actual {shown}. The toy case bad.',
        "SKIP toy/intl (section 8): The adapter does not claim the intl level.",
        "1 passed, 1 failed, 1 skipped, 1 passed with reports unobserved",
    ]
    with pytest.raises(AssertionError, match="1 of 2 conformance cases failed"):
        runner.check(adapter, files=fixtures)


# The audit, against an adapter that is right about a toy set.

_ESCAPED_MESSAGE = "\U0001f600{{a\\;b; c:d}}\\{{"

_AUDITED = [
    resolution_file(
        "core",
        case("plain", " a ", {"output": " a "}),
        case(
            "reported",
            "{{v:nope}}",
            {
                "output": "",
                "reports": [
                    {"code": "unknown-modifier", "origin": "message", "id": "m"},
                    {"code": "output-limit", "origin": "limit", "id": "m", "limit": 10},
                ],
            },
            messageId="m",
        ),
    ),
    tree_file(
        {
            "id": "cst/escaped",
            "description": "A toy tree case.",
            "message": _ESCAPED_MESSAGE,
            "expected": [
                {"type": "text", "text": "\U0001f600"},
                {
                    "type": "placeholder",
                    "text": "{{a\\;b; c:d}}",
                    "nodes": [
                        {"type": "open", "text": "{{"},
                        {
                            "type": "key",
                            "text": "a\\;b",
                            "name": "a;b",
                            "nodes": [
                                {"type": "text", "text": "a"},
                                {"type": "escape", "text": "\\;", "cancels": True},
                                {"type": "text", "text": "b"},
                            ],
                        },
                        {"type": "separator", "text": ";"},
                        {"type": "space", "text": " "},
                        {"type": "option-key", "text": "c", "name": "c"},
                        {"type": "separator", "text": ":"},
                        {"type": "option-value", "text": "d"},
                        {"type": "close", "text": "}}"},
                    ],
                },
                {"type": "escape", "text": "\\{", "cancels": True},
                {"type": "text", "text": "{"},
            ],
            "resolves": "\U0001f600{{",
        }
    ),
]


def _named(kind: str, start: int, end: int, name: str, *children: object) -> dict[str, object]:
    return {"type": kind, "start": start, "end": end, "name": name, "nodes": list(children)}


def _mark(kind: str, start: int, end: int) -> dict[str, object]:
    return {"type": kind, "start": start, "end": end}


# The tree of the toy message in UTF-16 code units: the face before the
# placeholder is two of them.
_ESCAPED = _root(
    18,
    _text(0, 2),
    {
        "type": "placeholder",
        "start": 2,
        "end": 15,
        "nodes": [
            _mark("open", 2, 4),
            _named(
                "key",
                4,
                8,
                "a;b",
                _text(4, 5),
                {**_mark("escape", 5, 7), "cancels": True},
                _text(7, 8),
            ),
            _mark("separator", 8, 9),
            _mark("space", 9, 10),
            _named("option-key", 10, 11, "c", _text(10, 11)),
            _mark("separator", 11, 12),
            {**_mark("option-value", 12, 13), "nodes": [_text(12, 13)]},
            _mark("close", 13, 15),
        ],
    },
    {**_mark("escape", 15, 17), "cancels": True},
    _text(17, 18),
)


def _audit_adapter() -> Adapter:
    reports: list[dict[str, object]] = [
        {"code": "unknown-modifier", "origin": "message", "id": "m"},
        {"code": "output-limit", "origin": "limit", "id": "m", "limit": 10},
    ]
    return canned(
        {
            " a ": Resolved(" a ", reports=[]),
            "{{v:nope}}": Resolved("", reports=reports),
            _ESCAPED_MESSAGE: Resolved("\U0001f600{{"),
        },
        cst=Cst("utf-16", lambda message: _ESCAPED),
    )


def test_the_audit_catches_each_defect_a_toy_set_can_reach() -> None:
    audited = {
        entry.entry.id: (entry.observed, entry.outcome)
        for entry in audit(_audit_adapter(), files=_AUDITED)
    }
    caught = {name for name, (_, result) in audited.items() if result == "caught"}
    unreachable = {name for name, (_, result) in audited.items() if result == "unreachable"}
    assert unreachable == {"claims-core-only", "formats-wrong", "unexpressible-declared"}
    assert caught == set(audited) - unreachable
    assert audited["tree-unit-changed"] == ("fail", "caught")
    assert audited["claims-no-limits"] == ("error", "caught")


def test_the_audit_answers_unreachable_where_reports_go_unobserved_and_no_tree_is_offered() -> None:
    bare = canned({" a ": Resolved(" a "), "{{v:nope}}": Resolved("")})
    audited = {entry.entry.id: entry.outcome for entry in audit(bare, files=_AUDITED[:1])}
    assert {name for name, result in audited.items() if result == "unreachable"} == {
        "reports-dropped",
        "reports-extra",
        "reports-reversed",
        "report-code-changed",
        "report-origin-changed",
        "report-id-changed",
        "report-limit-changed",
        "reports-unobserved",
        "formats-wrong",
        "claims-core-only",
        "unexpressible-declared",
        "tree-unoffered",
        "tree-node-dropped",
        "tree-node-retyped",
        "tree-span-shifted",
        "tree-name-raw",
        "tree-cancels-inverted",
        "tree-unit-changed",
        "tree-unit-unknown",
    }
    assert {name for name, result in audited.items() if result == "missed"} == set()


def test_a_code_point_tree_recounted_in_utf_16_reaches_the_runner_past_the_basic_plane() -> None:
    adapter = canned(
        {}, cst=Cst("code-point", lambda message: _root(len(message), _text(0, len(message))))
    )
    mutation = MUTATIONS["tree-unit-changed"](adapter)
    assert mutation.adapter.cst is not None
    assert mutation.adapter.cst.parse("ab") == _root(2, _text(0, 2))
    assert not mutation.reached()
    assert mutation.adapter.cst.parse("\U0001f600") == _root(2, _text(0, 2))
    assert mutation.reached()


def test_the_audit_needs_an_adapter_that_passes() -> None:
    wrong = canned({" a ": Resolved("a"), "{{v:nope}}": Resolved("")})
    with pytest.raises(SetError, match="fails 1 of 2 cases"):
        audit(wrong, files=_AUDITED[:1])


# The text reading, where an adapter withholds its requests.


def test_the_text_reading_performs_the_request_on_this_host() -> None:
    pytest.importorskip("babel")
    request = {"api": "NumberFormat", "options": {"maximumFractionDigits": 2}, "input": 1234.5678}
    written = case("text", "m", {"format": request}, locale="en")
    claims = {"levels": ("core", "intl")}
    assert outcome(written, Resolved("1,234.57"), **claims) == Passed(unobserved=True)
    assert outcome(written, Resolved("1234.57"), **claims) == Failed(
        "The output differs.", "1,234.57", "1234.57"
    )
    selection = {"api": "PluralRules", "options": {"type": "ordinal"}, "input": 22}
    chosen = case(
        "ordinal",
        "m",
        {"format": selection, "categories": {**_CATEGORIES, "two": "ND"}},
        locale="en",
    )
    assert outcome(chosen, Resolved("ND"), **claims) == Passed(unobserved=True)
    beyond = case(
        "beyond", "m", {"format": {**request, "options": {"notation": "compact"}}}, locale="en"
    )
    with pytest.raises(SetError, match="notation"):
        outcome(beyond, Resolved("1.2K"), **claims)
