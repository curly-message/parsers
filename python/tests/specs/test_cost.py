"""What the walk costs, asserted by count and never by time."""

from __future__ import annotations

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
    ModifierInput,
    Report,
    _convert,
    _resolve,
    _utils,
    create_parser,
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
