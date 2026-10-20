"""What the walk costs, asserted by count and never by time."""

from __future__ import annotations

import itertools
import sys
import tracemalloc
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from types import FrameType
from typing import Any

import pytest

from curly_message import (
    MAX_CONVERSION_NODES,
    MAX_NESTING,
    MAX_OUTPUT_LENGTH,
    MAX_REENTRY,
    UNDEFINED,
    ModifierInput,
    Option,
    Report,
    _convert,
    _resolve,
    _utils,
    create_parser,
    levels,
)
from curly_message._utils import Scanner


def test_the_scanner_settles_each_opening_brace_once() -> None:
    message = "{{a; c:" * 1000
    scan = Scanner(message)
    assert scan.next(0, len(message)) is None
    assert len(scan.verdict) <= message.count("{")
    assert all(verdict == -1 for verdict in scan.verdict.values())


def test_a_settled_verdict_is_answered_from_the_record() -> None:
    message = "{{a; b:" * 50 + "x" + "}}" * 50
    scan = Scanner(message)
    assert scan.end(0) == len(message)
    settled = dict(scan.verdict)
    assert len(settled) == 50
    for open_, close in settled.items():
        assert scan.end(open_) == close
    assert scan.verdict == settled


def _frames() -> int:
    count = 0
    frame: FrameType | None = sys._getframe()
    while frame is not None:
        count += 1
        frame = frame.f_back
    return count


@contextmanager
def _shallow_stack() -> Iterator[None]:
    """A host stack with room for what the walk needs at a constant depth, and no more."""
    limit = sys.getrecursionlimit()
    sys.setrecursionlimit(_frames() + 120)
    try:
        yield
    finally:
        sys.setrecursionlimit(limit)


@pytest.mark.parametrize("depth", [2000, 100000])
def test_inline_defaults_nested_past_the_host_stack_resolve(
    depth: int, monkeypatch: pytest.MonkeyPatch
) -> None:
    spans: list[tuple[int, int]] = []
    excerpt = _utils.excerpt

    def spy(source: str, open_: int, close: int) -> str:
        spans.append((open_, close))
        return excerpt(source, open_, close)

    monkeypatch.setattr(_utils, "excerpt", spy)
    message = "{{a; default:" * depth + "x" + "}}" * depth
    reports: list[Report] = []
    parser = create_parser(on_report=reports.append)
    with _shallow_stack():
        output = parser.resolve(message, {})
    assert output == "x"
    assert len(reports) == depth - MAX_NESTING
    assert {report.code for report in reports} == {"nesting-limit"}
    assert len(spans) == len(reports)


def test_selections_nested_past_the_host_stack_resolve() -> None:
    # Each level a modifier selects re-enters the walk from host code, which
    # the nesting limit bounds: deeper levels are refused before any is asked.
    depth = 100000
    message = "{{v; a:" * depth + "x" + "}}" * depth
    reports: list[Report] = []
    parser = create_parser(on_report=reports.append)
    with _shallow_stack():
        output = parser.resolve(message, {"v": "a"})
    assert output == ""
    assert [report.code for report in reports] == ["nesting-limit"]


def test_a_resolution_begun_by_a_modifier_at_the_deepest_level_resolves() -> None:
    # The deepest a walk calls host code from is a modifier at the nesting
    # limit, every level above it a selection on the stack; one that resolves
    # a deep message there walks it on a trampoline of its own.
    depth = 100000
    inner = "{{a; default:" * depth + "x" + "}}" * depth
    reports: list[Report] = []

    def deep(config: ModifierInput) -> str:
        return parser.resolve(inner, {}, message_id="inner")

    parser = create_parser(on_report=reports.append, custom_modifiers={"deep": deep})
    message = "{{v; a:" * (MAX_NESTING - 1) + "{{v:deep}}" + "}}" * (MAX_NESTING - 1)
    with _shallow_stack():
        output = parser.resolve(message, {"v": "a"}, message_id="outer")
    assert output == "x"
    assert len(reports) == depth - MAX_NESTING
    assert {(report.code, report.message_id) for report in reports} == {("nesting-limit", "inner")}


def _levels(selection: str, body: str, count: int) -> str:
    return selection * count + body + "}}" * count


_SELECTIONS = {
    "host": "{{v:own; a:",
    "eq": "{{v:eq; a:",
    "ne": "{{v:ne; b:",
    "lt": "{{n:lt; 5:",
    "gt": "{{n:gt; 0:",
    "lte": "{{n:lte; 1:",
    "lte, its strict leg": "{{n:lte; 5:",
    "gte": "{{n:gte; 1:",
    "gte, its strict leg": "{{n:gte; 0:",
    "a default": "{{v:eq; b:no; default:",
    "plural": "{{n:plural; one:",
    "ordinal": "{{n:ordinal; one:",
}

FRAMES_PER_READ = 3
"""The modifier that reads, the read, and the step of the walk the read resumes."""


@pytest.mark.parametrize("name", list(_SELECTIONS))
def test_a_read_in_progress_stands_three_frames_on_the_stack(name: str) -> None:
    # Reads stand one inside another to the depth the bounds allow, and the
    # interpreter's stack is what they stand on, so the frames of each are
    # counted: a helper that read for a modifier, or a call between the read
    # and the walk, would stand under every one of them.
    if name in {"plural", "ordinal"} and "intl" not in levels():
        pytest.skip("this installation defines no plural selections")
    depths: list[int] = []

    def probe(config: ModifierInput) -> str:
        depths.append(_frames())
        return "p"

    parser = create_parser(
        custom_modifiers={"probe": probe, "own": lambda config: config.options[0].value}
    )
    payload = {"v": "a", "n": 1}
    for count in (1, 1, 2, 3):
        message = _levels(_SELECTIONS[name], "{{v:probe}}", count)
        assert parser.resolve(message, payload, locale="en") == "p"
    # The first resolution paid whatever the first Intl call imports.
    _, one, two, three = depths
    assert (two - one, three - two) == (FRAMES_PER_READ, FRAMES_PER_READ)


def test_a_kept_read_in_progress_stands_three_frames_on_the_stack() -> None:
    depths: list[int] = []
    kept: list[Any] = []

    def keep(config: ModifierInput) -> str:
        kept.append(config.options[0] if config.value == "o" else config)
        return "k"

    def pop(config: ModifierInput) -> str:
        depths.append(_frames())
        if not kept:
            return "e"
        last = kept.pop()
        return str(last.default if isinstance(last, ModifierInput) else last.value)

    parser = create_parser(custom_modifiers={"k": keep, "m": pop})
    message = "{{o:k; o:{{v:m}}}}{{d:k; default:{{v:m}}}}{{o:k; o:{{v:m}}}}{{v:m}}"
    assert parser.resolve(message, {"v": 1, "o": "o", "d": "d"}) == "kkke"
    assert [after - before for before, after in itertools.pairwise(depths)] == [FRAMES_PER_READ] * 3


STACK = 600
"""The frames the deepest chain of reads the bounds allow is promised to fit in."""


def _at_headroom(headroom: int, call: Callable[[], Any]) -> Any:
    """Call from so deep that the interpreter has ``headroom`` frames left, as it counts them."""
    spare = sys.getrecursionlimit() - _frames() - headroom
    if spare < 0:
        pytest.skip(f"the interpreter has no {headroom} frames left here")

    def descend(count: int) -> Any:
        return descend(count - 1) if count else call()

    return descend(spare)


def _deepest(shape: str, selection: str) -> tuple[Callable[[], tuple[str, list[str]]], str]:
    """The chain that stands the most reads the bounds allow, and what it resolves to.

    Every resolution in progress but the first is a read of a kept option, or
    a resolution a modifier asks for, and each is reached through a modifier
    reading its own options at every level the nesting limit leaves.
    """
    kept: list[Option] = []
    asked: list[None] = []
    reports: list[Report] = []

    def keep(config: ModifierInput) -> str:
        kept.append(config.options[0])
        return "k"

    def pop(config: ModifierInput) -> str:
        return kept.pop().value if kept else "e"

    def again(config: ModifierInput) -> str:
        asked.append(None)
        if len(asked) == MAX_REENTRY:
            return "e"
        return parser.resolve(inner, payload, locale="en")

    parser = create_parser(
        custom_modifiers={
            "k": keep,
            "m": pop,
            "again": again,
            "own": lambda config: config.options[0].value,
        },
        on_report=reports.append,
    )
    payload = {"v": "a", "n": 1}
    inner = _levels(selection, "{{v:again}}", MAX_NESTING - 1)
    if shape == "kept":
        # A kept option is written at the second level.
        slot = "{{v:k; o:" + _levels(selection, "{{v:m}}", MAX_NESTING - 2) + "}}"
        message = slot * (MAX_REENTRY - 1) + _levels(selection, "{{v:m}}", MAX_NESTING - 1)
        expected = "k" * (MAX_REENTRY - 1) + "e"
    else:
        message, expected = inner, "e"

    def run() -> tuple[str, list[str]]:
        kept.clear()
        asked.clear()
        reports.clear()
        output = parser.resolve(message, payload, locale="en")
        return output, [report.code for report in reports]

    return run, expected


@pytest.mark.parametrize("shape", ["kept", "resolved"])
@pytest.mark.parametrize("name", ["host", "lte, its strict leg", "plural"])
def test_the_deepest_chain_the_bounds_allow_fits_the_stack_it_is_promised(
    shape: str, name: str
) -> None:
    # `MAX_REENTRY` resolutions in progress, each under the `MAX_NESTING - 1`
    # reads a modifier makes of its own options, are the most reads that stand
    # at once. Each stands three frames, and four of the interpreter's limit
    # on CPython 3.11, which also counts the call that resumes the walk; a
    # resolution asked for by a modifier costs at most seven more than a read
    # does. That fits `STACK`, so a caller with that much room is answered by
    # the bounds and never by the interpreter's limit.
    if name == "plural" and "intl" not in levels():
        pytest.skip("this installation defines no plural selections")
    assert MAX_REENTRY * ((MAX_NESTING - 1) * (FRAMES_PER_READ + 1) + 7) <= STACK
    run, expected = _deepest(shape, _SELECTIONS[name])
    # The first Intl call imports its supplier, which is no part of the chain.
    assert run() == (expected, [])
    assert _at_headroom(STACK, run) == (expected, [])
    # Near miss: with a sixth of that room the interpreter's limit arrives
    # first, and is contained as the failure of a modifier.
    output, reports = _at_headroom(STACK // 6, run)
    assert output != expected
    assert set(reports) <= {"failed-modifier"}


def test_a_chain_of_kept_reads_alone_fits_a_caller_near_the_limit_of_its_stack() -> None:
    # With nothing nested under them, the reads the reentry bound allows stand
    # four frames each at most, so the bound answers a caller this close to
    # the interpreter's limit as it answers any other.
    kept = "{{v:k; o:{{v:m}}}}"

    def chain(count: int) -> Callable[[], tuple[str, list[str]]]:
        held: list[Option] = []
        found: list[Report] = []

        def keep(config: ModifierInput) -> str:
            held.append(config.options[0])
            return "k"

        def pop(config: ModifierInput) -> str:
            return held.pop().value if held else "e"

        parser = create_parser(custom_modifiers={"k": keep, "m": pop}, on_report=found.append)

        def run() -> tuple[str, list[str]]:
            held.clear()
            found.clear()
            output = parser.resolve(kept * count + "{{v:m}}", {"v": 1})
            return output, [report.code for report in found]

        return run

    room = MAX_REENTRY * (FRAMES_PER_READ + 1) + 40
    within = MAX_REENTRY - 1
    assert _at_headroom(room, chain(within)) == ("k" * within + "e", [])
    assert _at_headroom(room, chain(MAX_REENTRY)) == ("k" * MAX_REENTRY, ["failed-modifier"])


@pytest.fixture
def enumerated(monkeypatch: pytest.MonkeyPatch) -> list[int]:
    """How many entries each enumeration of a props layer handed the walk."""
    entries: list[int] = []
    own_items = _utils.own_items

    def spy(container: object) -> list[tuple[str, object]]:
        found = own_items(container)
        entries.append(len(found))
        return found

    monkeypatch.setattr(_resolve, "own_items", spy)
    return entries


@pytest.mark.parametrize("name", ["eq", "host"])
def test_a_props_layer_is_enumerated_once_however_many_placeholders_name_its_modifier(
    name: str, enumerated: list[int]
) -> None:
    # The width of a layer is the payload's and the count of placeholders the
    # message's: composing per placeholder would cost their product.
    width, placeholders = 40, 50
    wide = {f"k{index}": index for index in range(width)}
    parser = create_parser(
        custom_modifiers={"host": lambda config: str(len(config.props))},
        modifier_defaults={name: dict(wide)},
    )
    payload = {"w": {"value": 1, "props": {name: dict(wide)}}, "v": 1}
    message = f"{{{{w:{name}; 1:a}}}}{{{{v:{name}; 1:a}}}}" * placeholders
    expected = "aa" if name == "eq" else str(width) * 2
    enumerated.clear()
    assert parser.resolve(message, payload, props={name: dict(wide)}) == expected * placeholders
    # The registry's one entry, then each of the three layers once: the
    # wrapper's props are laid over what the other two compose.
    assert sum(enumerated) == 1 + 3 * width
    # The record is the resolution's: the next one reads the layers again.
    enumerated.clear()
    parser.resolve(message, payload, props={name: dict(wide)})
    assert sum(enumerated) == 1 + 3 * width


def test_each_wrapper_and_each_name_composes_its_own_props(enumerated: list[int]) -> None:
    def show(config: ModifierInput) -> str:
        return ",".join(f"{key}={value}" for key, value in sorted(config.props.items()))

    parser = create_parser(
        custom_modifiers={"show": show, "also": show}, modifier_defaults={"show": {"d": 0}}
    )
    payload = {
        "a": {"value": 1, "props": {"show": {"p": "a"}, "also": {"q": "a"}}},
        "b": {"value": 1, "props": {"show": {"p": "b"}}},
        "c": 1,
    }
    message = "{{a:show}}|{{b:show}}|{{c:show}}|{{a:also}}|{{b:also}}|{{a:show}}"
    enumerated.clear()
    assert parser.resolve(message, payload) == "d=0,p=a|d=0,p=b|d=0|q=a||d=0,p=a"
    # `a:show` is composed once for its two placeholders: the registry, then
    # the one entry of the defaults, read once for all three, one entry of
    # each of the two wrappers under `show`, and `also`.
    assert sum(enumerated) == 2 + 1 + 2 + 1


@pytest.mark.parametrize("name", ["eq", "host"])
def test_wrappers_that_each_configure_a_modifier_copy_nothing_beneath_them(
    name: str, enumerated: list[int]
) -> None:
    # What the parser's defaults and the call's props compose is shared by
    # every wrapper: one that adds a property lays it over that composition,
    # so many wrappers over wide props hold what each adds and not that many
    # copies of the width beneath.
    width = 2000
    wide = {f"k{index}": index for index in range(width)}

    def resolved(wrappers: int) -> tuple[str, int, int]:
        parser = create_parser(
            custom_modifiers={"host": lambda config: f"{len(config.props)},"},
            modifier_defaults={name: dict(wide)},
        )
        payload = {
            f"w{index}": {"value": 1, "props": {name: {"own": index, "k0": "over"}}}
            for index in range(wrappers)
        }
        message = "".join(f"{{{{w{index}:{name}; 1:a}}}}" for index in range(wrappers)) * 2
        enumerated.clear()
        tracemalloc.start()
        try:
            output = parser.resolve(message, payload)
            _, peak = tracemalloc.get_traced_memory()
        finally:
            tracemalloc.stop()
        return output, sum(enumerated), peak

    expected = "a" if name == "eq" else f"{width + 1},"
    output, entries, one = resolved(1)
    assert (output, entries) == (expected * 2, 1 + width + 2)
    output, entries, many = resolved(200)
    assert (output, entries) == (expected * 400, 1 + width + 2 * 200)
    assert many - one < 10 * sys.getsizeof(wide)


def test_a_layer_changed_while_a_resolution_runs_is_read_as_it_was_composed() -> None:
    layer = {"a": 1}

    def change(config: ModifierInput) -> str:
        shown = str(config.props["a"])
        layer["a"] += 1
        return shown

    parser = create_parser(custom_modifiers={"change": change})
    message = "{{v:change}}{{v:change}}"
    assert parser.resolve(message, {"v": 1}, props={"change": layer}) == "11"
    # The next resolution composes anew.
    assert parser.resolve(message, {"v": 1}, props={"change": layer}) == "33"


def test_a_report_copies_no_more_of_the_message_than_its_cut() -> None:
    copied: list[int] = []

    class Recording(str):
        def __getitem__(self, index: Any) -> str:
            part = str.__getitem__(self, index)
            copied.append(len(part))
            return part

    depth = 1000
    message = Recording("{{a; default:" * depth + "x" + "}}" * depth)
    text = _utils.excerpt(message, 0, len(message))
    assert text.endswith("...")
    assert copied == [_utils.MAX_REPORTED_LENGTH + 1]


def test_a_placeholder_that_renders_nothing_nested_starts_no_tail(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[object] = []
    tail = _resolve._Walk._tail

    def spy(*args: Any) -> Any:
        calls.append(args)
        return tail(*args)

    monkeypatch.setattr(_resolve._Walk, "_tail", spy)
    parser = create_parser()
    message = "Hi {{name}}, {{missing; default:you}} {{v:zz; default:D}} {{n:eq; default:E}}"
    assert parser.resolve(message, {"name": "Ann", "n": 1}) == "Hi Ann, you D E"
    assert calls == []
    assert parser.resolve("{{n; 1:one}}", {"n": 1}) == "one"
    assert len(calls) == 1


def test_a_selection_answering_text_at_the_top_level_builds_no_tree(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    built: list[int] = []

    class Counted(_resolve._Text):
        __slots__ = ()

        def __init__(self, pieces: list[str | _resolve._Text], length: int) -> None:
            built.append(length)
            super().__init__(pieces, length)

    monkeypatch.setattr(_resolve, "_Text", Counted)
    parser = create_parser()
    message = "{{gender; male:He; female:She; default:They}} replied to {{thread}}."
    output = parser.resolve(message, {"gender": "female", "thread": "Plans"})
    assert output == "She replied to Plans."
    assert built == []
    # Below the top level a span is still held as pieces: the option the
    # modifier read, and an inline default the top level is handed.
    message = "{{count; 1:{{count}} file; other:{{count}} files}} in {{folder; default:{{folder}}}}"
    assert parser.resolve(message, {"count": 1, "folder": "docs"}) == "1 file in docs"
    assert built == [len("1 file")]
    assert parser.resolve("{{a; default:x{{b}}y}}", {"b": "B"}) == "xBy"
    assert built[1:] == [len("xBy"), len("xBy")]


class _Counted:
    def __init__(self, conversions: list[str], name: str) -> None:
        self._conversions = conversions
        self._name = name

    def __str__(self) -> str:
        self._conversions.append(self._name)
        return self._name


def test_a_value_converts_once_per_resolution_however_often_it_is_named() -> None:
    conversions: list[str] = []
    value = _Counted(conversions, "v")
    parser = create_parser()
    message = "{{v}}" * 25 + "{{v:eq; v:{{v}}; default:D}}" * 25
    assert parser.resolve(message, {"v": value}) == "v" * 50
    assert conversions == ["v"]
    parser.resolve(message, {"v": value})
    assert conversions == ["v", "v"]


def test_an_option_the_modifier_passes_over_reads_nothing() -> None:
    conversions: list[str] = []
    payload = {name: _Counted(conversions, name) for name in ("a", "b", "c", "d")}
    message = "{{a; a:{{b}}; x:{{c}}; default:{{d}}}}"
    assert create_parser().resolve(message, payload) == "b"
    assert conversions == ["a", "b"]


def test_a_conversion_is_abandoned_past_the_node_limit() -> None:
    assert _convert.serialize([0] * (MAX_CONVERSION_NODES - 1)) is not None
    assert _convert.serialize([0] * MAX_CONVERSION_NODES) is None
    shared: list[object] = []
    for _ in range(24):
        shared = [shared, shared]
    assert _convert.serialize(shared) is None


def test_a_member_left_out_still_spends_a_node() -> None:
    def members(count: int) -> dict[str, object]:
        return {f"k{index}": UNDEFINED for index in range(count)}

    reports: list[Report] = []
    parser = create_parser(on_report=reports.append)
    message = "{{v:ne; nomatch:ok; default:refused;}}"
    assert parser.resolve(message, {"v": members(MAX_CONVERSION_NODES - 1)}) == "ok"
    assert reports == []
    assert parser.resolve("{{v; default:D}}", {"v": members(MAX_CONVERSION_NODES)}) == "D"
    assert [(report.code, report.origin) for report in reports] == [
        ("unserializable-value", "payload")
    ]


def test_undefined_is_written_null_only_as_an_element() -> None:
    assert _convert.serialize([UNDEFINED]) == "[null]"
    assert _convert.serialize(UNDEFINED) is None


def _counting_serializer(monkeypatch: pytest.MonkeyPatch) -> list[object]:
    calls: list[object] = []
    serialize: Callable[[object], str | None] = _convert.serialize

    def spy(value: object) -> str | None:
        calls.append(value)
        return serialize(value)

    monkeypatch.setattr(_convert, "serialize", spy)
    return calls


def test_a_structured_value_is_serialized_once_per_resolution(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls = _counting_serializer(monkeypatch)
    held: dict[str, object] = {}
    held["self"] = held
    reports: list[Report] = []
    parser = create_parser(on_report=reports.append)
    assert parser.resolve("{{v; default:D}}" * 10, {"v": held}) == "D" * 10
    assert len(calls) == 1
    assert [report.code for report in reports] == ["unserializable-value"] * 10


@pytest.mark.parametrize(
    ("innermost", "payload"),
    [
        ("x" * 1000, {}),
        ("{{v}}", {"v": {"default": "x" * 1000}}),
        (".{{a; default:" * 20 + "x" * 1000 + "}}." * 20, {}),
    ],
)
def test_a_nested_result_is_copied_where_it_becomes_text_and_not_per_level(
    innermost: str, payload: dict[str, Any]
) -> None:
    def joins(depth: int) -> int:
        calls: list[None] = []

        def profile(frame: FrameType, event: str, arg: Any) -> None:
            if (
                event == "c_call"
                and getattr(arg, "__name__", None) == "join"
                and isinstance(getattr(arg, "__self__", None), str)
            ):
                calls.append(None)

        message = "{{a; default:" * depth + innermost + "}}" * depth
        parser = create_parser()
        with _shallow_stack():
            sys.setprofile(profile)
            try:
                output = parser.resolve(message, payload)
            finally:
                sys.setprofile(None)
        assert output.strip(".") == "x" * 1000
        return len(calls)

    assert joins(2000) == joins(20000) <= 2


@pytest.mark.parametrize("message", ["{{a; default:{{b}}}}", "{{a; default:x{{b}}y}}"])
def test_a_value_the_output_refuses_is_not_copied_below_the_top_level(message: str) -> None:
    value = "x" * (40 * MAX_OUTPUT_LENGTH)
    codes: list[str] = []
    parser = create_parser(on_report=lambda report: codes.append(report.code))
    # What the first report builds once per process is no copy of the value.
    parser.resolve(message, {"b": value})
    codes.clear()
    tracemalloc.start()
    try:
        output = parser.resolve(message, {"b": value})
        _, peak = tracemalloc.get_traced_memory()
    finally:
        tracemalloc.stop()
    assert (output, codes) == ("", ["output-limit"])
    assert peak < len(value) // 2


def test_a_conversion_is_abandoned_past_the_length_it_may_write() -> None:
    # Each member writes its 100 000 characters, two quotes and a separator.
    shared = "x" * 100000
    assert _convert.serialize([shared] * 99) is not None
    assert _convert.serialize([shared] * 100) is None
    assert _convert.serialize([int("7" * 4000)] * (MAX_CONVERSION_NODES - 1)) is None
    reports: list[Report] = []
    parser = create_parser(on_report=reports.append)
    assert parser.resolve("{{v:eq; x:y; default:d}}", {"v": [shared] * 200}) == "d"
    assert [report.code for report in reports] == ["unserializable-value"]


def test_a_member_name_with_no_room_is_not_encoded(monkeypatch: pytest.MonkeyPatch) -> None:
    encoded: list[int] = []
    encode = _convert._encode

    def spy(value: str) -> str:
        encoded.append(len(value))
        return encode(value)

    monkeypatch.setattr(_convert, "_encode", spy)
    # `{`, the name in its quotes, `:`, the member and `}` fill the room.
    room = _convert.MAX_CONVERSION_LENGTH
    assert _convert.serialize({"x" * (room - 6): 1}) is not None
    encoded.clear()
    # A name writes at least itself, its quotes and the colon after it.
    assert _convert.serialize({"\x00" * (room - 3): 1}) is None
    assert _convert.serialize({"v": {"\x00" * (room - 7): 1}}) is None
    assert encoded == [1]


def test_a_member_named_again_is_written_from_its_first_encoding(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    encoded: list[str] = []
    encode = _convert._encode

    def spy(value: str) -> str:
        encoded.append(value)
        return encode(value)

    monkeypatch.setattr(_convert, "_encode", spy)
    shared = "x" * 1000
    assert _convert.serialize([shared, [shared], shared, "".join(["x"] * 1000)]) is not None
    assert len(encoded) == 2
    # The near miss: a short member costs no more to encode than to look up,
    # so it is encoded where it is met rather than remembered.
    encoded.clear()
    assert _convert.serialize(["ab", "ab", ["ab"]]) == '["ab","ab",["ab"]]'
    assert encoded == ["ab", "ab", "ab"]
