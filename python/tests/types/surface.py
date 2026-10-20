"""The public surface as a consumer's checker sees it.

``mypy --strict`` checks this file twice: against ``src/``, and against the
wheel installed in ``.venv-dist``, where the package is found through its own
``py.typed``. A ``# type: ignore[code]`` marks a call the checker must refuse:
``warn_unused_ignores`` fails the run where it does not. pytest runs it too,
so every function declared here is called.
"""

from __future__ import annotations

from typing import Literal, assert_type

import pytest

import curly_message
from curly_message import (
    MAX_CONVERSION_NODES,
    MAX_NESTING,
    MAX_OUTPUT_LENGTH,
    MAX_READ_LENGTH,
    UNDEFINED,
    CstMessage,
    CstNode,
    Modifier,
    ModifierInput,
    Option,
    Parser,
    Report,
    ReportCode,
    ReportOrigin,
    Undefined,
    create_parser,
    cst,
    levels,
)

EXPORTED = {
    "MAX_CONVERSION_NODES",
    "MAX_NESTING",
    "MAX_OUTPUT_LENGTH",
    "MAX_READ_LENGTH",
    "UNDEFINED",
    "CstEscape",
    "CstMessage",
    "CstName",
    "CstNode",
    "CstOptionValue",
    "CstPlaceholder",
    "CstPunctuation",
    "CstSpace",
    "CstText",
    "Modifier",
    "ModifierInput",
    "Option",
    "Parser",
    "Report",
    "ReportCode",
    "ReportOrigin",
    "Undefined",
    "create_parser",
    "cst",
    "levels",
}


def test_the_package_exports_exactly_its_surface() -> None:
    assert set(curly_message.__all__) == EXPORTED
    assert all(hasattr(curly_message, name) for name in EXPORTED)


def test_a_parser_resolves_to_text() -> None:
    parser = create_parser()
    assert_type(parser, Parser)
    assert_type(parser.resolve("Hi {{name}}", {"name": "Ann"}, locale="en"), str)
    assert_type(parser.resolve(UNDEFINED), str)
    assert_type(
        parser.resolve("{{n}}", payload={"n": 1}, props={"number": {}}, message_id=("app", 3)),
        str,
    )
    assert parser.resolve("Hi {{name}}", {"name": "Ann"}) == "Hi Ann"


def test_options_are_keyword_only_and_typed() -> None:
    reports: list[Report] = []

    def upper(given: ModifierInput) -> object:
        assert_type(given.value, str)
        assert_type(given.options, tuple[Option, ...])
        assert_type(given.default, str)
        assert_type(given.locale, object)
        return given.value.upper()

    registered: dict[str, Modifier] = {"upper": upper}
    parser = create_parser(
        custom_modifiers=registered,
        modifier_defaults={"upper": {"strict": True}},
        on_report=reports.append,
        recognize_wrappers=False,
    )
    assert parser.resolve("{{v:upper}} {{w:zz}}", {"v": "a", "w": "b"}) == "A "
    (report,) = reports
    assert_type(report.code, ReportCode)
    assert_type(report.origin, ReportOrigin)
    assert_type(report.text, str)
    assert_type(report.message, str)
    assert_type(report.message_id, object)
    assert_type(report.limit, int | None)
    assert report.code == "unknown-modifier"


def test_the_checker_refuses_misuse() -> None:
    parser = create_parser(on_report=3)  # type: ignore[arg-type]
    parser = create_parser(custom_modifiers={"a": 1})  # type: ignore[dict-item]
    with pytest.raises(TypeError):
        create_parser(True)  # type: ignore[call-arg, arg-type]
    assert parser.resolve("x", "payload") == "x"  # type: ignore[arg-type]
    with pytest.raises(TypeError):
        parser.resolve("x", None, None)  # type: ignore[call-arg]
    assert parser.resolve("x", props={"number": 1}) == "x"  # type: ignore[dict-item]


def test_a_tree_is_typed_by_its_kind() -> None:
    tree = cst("a {{b}}")
    assert_type(tree, CstMessage)
    assert_type(tree["nodes"][0]["start"], int)
    node: CstNode = tree
    assert node["type"] == "message"
    assert [child["type"] for child in tree["nodes"]] == ["text", "placeholder"]


def test_constants_and_the_sentinel() -> None:
    assert_type(MAX_OUTPUT_LENGTH, int)
    assert_type(MAX_READ_LENGTH, int)
    assert_type(MAX_CONVERSION_NODES, int)
    assert_type(MAX_NESTING, int)
    assert_type(UNDEFINED, Literal[Undefined.UNDEFINED])
    assert_type(levels(), frozenset[Literal["core", "intl", "extensions"]])
    assert {"core", "extensions"} <= levels()
    assert not UNDEFINED
