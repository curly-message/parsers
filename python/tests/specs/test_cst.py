"""The tree a message is described by, driven through ``cst``."""

from __future__ import annotations

import json
import sys
from collections.abc import Iterator
from types import FrameType
from typing import Any, cast

import pytest

from curly_message import CstMessage, CstNode, cst


def _fields(node: CstNode) -> dict[str, Any]:
    return cast(dict[str, Any], node)


def _leaves(tree: CstNode) -> Iterator[CstNode]:
    stack: list[CstNode] = [tree]
    while stack:
        node = stack.pop()
        children: list[CstNode] = _fields(node).get("nodes", [])
        if children:
            stack.extend(reversed(children))
        else:
            yield node


def _spelled_back(message: str, tree: CstMessage) -> str:
    return "".join(message[leaf["start"] : leaf["end"]] for leaf in _leaves(tree))


def _shape(message: str, node: CstNode) -> object:
    """A node as the conformance set writes it: what it spells, never where."""
    fields = _fields(node)
    shape: dict[str, object] = {"type": node["type"], "text": message[node["start"] : node["end"]]}
    for field in ("name", "cancels"):
        if field in fields:
            shape[field] = fields[field]
    if "nodes" in fields:
        shape["nodes"] = [_shape(message, child) for child in fields["nodes"]]
    return shape


def _texts(message: str) -> list[object]:
    return [message[node["start"] : node["end"]] for node in cst(message)["nodes"]]


def test_text_only_message_is_one_text_node() -> None:
    assert cst("Hello") == {
        "type": "message",
        "start": 0,
        "end": 5,
        "nodes": [{"type": "text", "start": 0, "end": 5}],
    }


def test_empty_message_has_no_parts() -> None:
    assert cst("") == {"type": "message", "start": 0, "end": 0, "nodes": []}


@pytest.mark.parametrize("message", [None, 1, 1.5, True, b"{{a}}", ["{{a}}"], {"a": 1}])
def test_a_message_that_is_not_text_has_no_parts(message: object) -> None:
    assert cst(message) == {"type": "message", "start": 0, "end": 0, "nodes": []}


def test_worked_example_with_its_spans() -> None:
    message = "Hi {{name; default:you;}}"
    assert cst(message)["nodes"] == [
        {"type": "text", "start": 0, "end": 3},
        {
            "type": "placeholder",
            "start": 3,
            "end": 25,
            "nodes": [
                {"type": "open", "start": 3, "end": 5},
                {
                    "type": "key",
                    "start": 5,
                    "end": 9,
                    "name": "name",
                    "nodes": [{"type": "text", "start": 5, "end": 9}],
                },
                {"type": "separator", "start": 9, "end": 10},
                {"type": "space", "start": 10, "end": 11},
                {
                    "type": "option-key",
                    "start": 11,
                    "end": 18,
                    "name": "default",
                    "nodes": [{"type": "text", "start": 11, "end": 18}],
                },
                {"type": "separator", "start": 18, "end": 19},
                {
                    "type": "option-value",
                    "start": 19,
                    "end": 22,
                    "nodes": [{"type": "text", "start": 19, "end": 22}],
                },
                {"type": "separator", "start": 22, "end": 23},
                {"type": "option-key", "start": 23, "end": 23, "name": "", "nodes": []},
                {"type": "close", "start": 23, "end": 25},
            ],
        },
    ]


def test_placeholder_in_an_option_value_is_described_there() -> None:
    message = "{{n:gt; 0:{{n:number}}; default:no}}"
    (outer,) = cst(message)["nodes"]
    assert outer["type"] == "placeholder"
    value = next(node for node in outer["nodes"] if node["type"] == "option-value")
    assert _shape(message, value) == {
        "type": "option-value",
        "text": "{{n:number}}",
        "nodes": [
            {
                "type": "placeholder",
                "text": "{{n:number}}",
                "nodes": [
                    {"type": "open", "text": "{{"},
                    {
                        "type": "key",
                        "text": "n",
                        "name": "n",
                        "nodes": [{"type": "text", "text": "n"}],
                    },
                    {"type": "separator", "text": ":"},
                    {
                        "type": "modifier",
                        "text": "number",
                        "name": "number",
                        "nodes": [{"type": "text", "text": "number"}],
                    },
                    {"type": "close", "text": "}}"},
                ],
            }
        ],
    }


@pytest.mark.parametrize(
    "message",
    [
        "{{a",
        "{{a\n}}",
        "{{a\u2028}}",
        "{{a\\\n}}",
        "{{a; b:{{c}",
        "}}{{",
    ],
)
def test_a_construct_that_derives_nothing_is_text(message: str) -> None:
    tree = cst(message)
    assert all(node["type"] in ("text", "escape") for node in tree["nodes"])
    assert _spelled_back(message, tree) == message


def test_the_scan_resumes_one_code_point_past_a_refused_brace() -> None:
    message = "{{{{a}}"
    assert _texts(message) == ["{", "{{{a}}"]


def test_a_placeholder_opens_in_a_value_and_nowhere_else() -> None:
    assert [node["type"] for node in cst("{{a; b:{{c}}}}")["nodes"]] == ["placeholder"]
    assert _texts("{{a; b {{c}}}}") == ["{{a; b ", "{{c}}", "}}"]


def test_escapes_are_marked_with_the_reading_they_take() -> None:
    message = "\\{a\\b\\"
    assert [_shape(message, node) for node in cst(message)["nodes"]] == [
        {"type": "escape", "text": "\\{", "cancels": True},
        {"type": "text", "text": "a"},
        {"type": "escape", "text": "\\b", "cancels": False},
        {"type": "text", "text": "\\"},
    ]


def test_spans_count_code_points() -> None:
    message = "\\\U0001f600{{a}}"
    assert cst(message)["nodes"] == [
        {"type": "escape", "start": 0, "end": 2, "cancels": False},
        {
            "type": "placeholder",
            "start": 2,
            "end": 7,
            "nodes": [
                {"type": "open", "start": 2, "end": 4},
                {
                    "type": "key",
                    "start": 4,
                    "end": 5,
                    "name": "a",
                    "nodes": [{"type": "text", "start": 4, "end": 5}],
                },
                {"type": "close", "start": 5, "end": 7},
            ],
        },
    ]


@pytest.mark.parametrize(
    "message",
    [
        "",
        "plain",
        "a {{ b : eq ; x : {{c; d:e}} ; default : f }} g",
        "{{ \\ a \\ : m \\; ; k\\:k : v\\; ;}}",
        "\\\\{{a}}\\",
        "{{a;;;b;c:}}",
        "{{:}}{{;}}{{}}",
        "\ufeff{{\xa0a\u3000}}",
    ],
)
def test_the_leaves_tile_the_message(message: str) -> None:
    tree = cst(message)
    assert _spelled_back(message, tree) == message
    assert cst(message) == tree


def test_a_str_subclass_is_read_through_str_itself() -> None:
    class Loud(str):
        def __getitem__(self, index: object) -> str:
            raise AssertionError("read through an override")

        def find(self, *args: object) -> int:
            raise AssertionError("read through an override")

    assert cst(Loud("{{a}}")) == cst("{{a}}")


def test_the_tree_is_its_interchange_encoding() -> None:
    message = "{{a:eq; x:{{b}};}}"
    tree = cst(message)
    assert json.loads(json.dumps(tree)) == tree


def test_deep_nesting_is_described_without_the_host_stack() -> None:
    depth = 100000
    message = "{{a; b:" * depth + "x" + "}}" * depth
    limit = sys.getrecursionlimit()
    sys.setrecursionlimit(len(_frames()) + 120)
    try:
        tree = cst(message)
    finally:
        sys.setrecursionlimit(limit)
    reached = 0
    node: CstNode = tree["nodes"][0]
    while node["type"] == "placeholder":
        reached += 1
        value = node["nodes"][-2]
        assert value["type"] == "option-value"
        node = value["nodes"][0]
    assert reached == depth
    assert node == {"type": "text", "start": 7 * depth, "end": 7 * depth + 1}


def _frames() -> list[object]:
    frames: list[object] = []
    frame: FrameType | None = sys._getframe()
    while frame is not None:
        frames.append(frame)
        frame = frame.f_back
    return frames
