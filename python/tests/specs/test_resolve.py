"""Resolution, driven through ``create_parser(...).resolve``."""

from __future__ import annotations

import collections
import dataclasses
import enum
import subprocess
import sys
import threading
import traceback
import types
import unicodedata
from typing import Any, NamedTuple

import pytest

from curly_message import (
    MAX_NESTING,
    MAX_OUTPUT_LENGTH,
    MAX_READ_LENGTH,
    UNDEFINED,
    Modifier,
    ModifierInput,
    Option,
    Report,
    create_parser,
    levels,
)
from curly_message._utils import WHITESPACE


def resolve(message: object, payload: Any = None, **options: Any) -> tuple[str, list[Report]]:
    reports: list[Report] = []
    output = create_parser(on_report=reports.append, **options).resolve(
        message, payload, message_id="id"
    )
    return output, reports


def codes(reports: list[Report]) -> list[str]:
    return [report.code for report in reports]


class Name(enum.StrEnum):
    NAME = "name"
    VALUE = "value"


def test_plain_substitution_and_the_fallback_chain() -> None:
    message = "Hello, {{name; default:Guest;}}!"
    assert resolve(message, {"name": "Ann", "default": "Friend"})[0] == "Hello, Ann!"
    assert resolve(message, {"name": "", "default": "Friend"})[0] == "Hello, !"
    assert resolve(message, {"name": {"default": "You"}, "default": "Friend"})[0] == "Hello, You!"
    assert resolve(message, {"default": "Friend"})[0] == "Hello, Friend!"
    assert resolve(message, {})[0] == "Hello, Guest!"
    assert resolve(message)[0] == "Hello, Guest!"


def test_a_message_with_no_placeholder_is_its_text_unescaped() -> None:
    assert resolve("a\\{{b\\c") == ("a{{b\\c", [])


@pytest.mark.parametrize(
    ("message", "expected"),
    [
        (UNDEFINED, ""),
        (None, "None"),
        (0, "0"),
        (False, "False"),
        ({"a": "{{v}}"}, '{"a":"X"}'),
        (["{{v}}"], '["X"]'),
    ],
)
def test_a_message_that_is_not_text_is_converted_first(message: object, expected: str) -> None:
    assert resolve(message, {"v": "X"})[0] == expected


def test_a_message_no_conversion_describes_is_nothing_to_resolve() -> None:
    class Refuses:
        def __str__(self) -> str:
            raise ValueError("no text")

    assert resolve(Refuses()) == ("", [])


def test_a_message_held_in_a_str_subclass_is_its_text() -> None:
    class Loud(str):
        def __str__(self) -> str:
            return Loud("{{v}}!")

    output, _ = resolve(Loud("ignored"), {"v": "X"})
    assert output == "X!"
    assert type(output) is str


@pytest.mark.parametrize("name", ["__class__", "items", "__dict__", "get", "keys", "__proto__"])
def test_a_name_a_dict_inherits_is_no_entry(name: str) -> None:
    message = f"{{{{{name}; default:absent}}}}"
    assert resolve(message, {}) == ("absent", [])
    assert resolve(message, {name: "own"}) == ("own", [])


def test_a_defaultdict_payload_answers_only_what_it_stores() -> None:
    payload: collections.defaultdict[str, str] = collections.defaultdict(lambda: "made up")
    payload["kept"] = "K"
    assert resolve("{{kept}} {{other; default:D}}", payload)[0] == "K D"
    assert dict(payload) == {"kept": "K"}


def test_a_dict_subclass_is_read_through_its_storage() -> None:
    class Overriding(dict[str, object]):
        def __getitem__(self, key: str) -> object:
            raise AssertionError("an override answered")

        def get(self, key: str, default: object = None) -> object:
            raise AssertionError("an override answered")

    assert resolve("{{a}}", Overriding(a="A")) == ("A", [])


def test_a_mapping_that_is_not_a_dict_holds_no_entries() -> None:
    reads: list[object] = []

    class Counting(dict[str, object]):
        def __getitem__(self, key: str) -> object:
            reads.append(key)
            return "read"

    chained = collections.ChainMap({}, {"role": "admin"})
    assert resolve("{{role; default:guest}}", chained) == ("guest", [])
    proxy = types.MappingProxyType(Counting(role="admin"))
    assert resolve("{{role; default:guest}}", proxy) == ("guest", [])
    assert reads == []


def test_a_str_subclass_key_names_its_entry_by_its_text() -> None:
    assert resolve("{{name}}", {Name.NAME: "by enum"})[0] == "by enum"
    assert resolve("{{name}}", {"name": {Name.VALUE: "wrapped"}})[0] == "wrapped"


def test_a_key_that_is_not_a_str_names_nothing() -> None:
    assert resolve("{{v}}", {"v": {1: "x"}})[0] == '{"1":"x"}'
    assert resolve("{{1; default:D}}", {1: "int key"})[0] == "D"


@pytest.mark.parametrize("key", [1, (1,), None, 1.5])
def test_a_key_that_is_not_a_str_is_an_own_key_no_wrapper_holds(key: object) -> None:
    # Section 4.1: a wrapper is a dict whose every own key is one of the
    # three; a key naming nothing is still a key, so the dict is a value.
    assert resolve("{{v}}", {"v": {"value": "Ann"}})[0] == "Ann"
    near_miss = {"value": "Ann", key: "x"}
    assert resolve("{{v}}", {"v": near_miss}) == resolve(
        "{{v}}", {"v": near_miss}, recognize_wrappers=False
    )
    assert resolve("{{v}}", {"v": {key: "x", "value": "Ann"}})[0] != "Ann"


def test_a_stored_key_that_raises_on_comparison_is_absent_and_reported() -> None:
    class Colliding:
        def __hash__(self) -> int:
            return hash("name")

        def __eq__(self, other: object) -> bool:
            raise ValueError("no comparison")

    payload = {Colliding(): "hidden", "other": "O"}
    output, reports = resolve("{{name; default:D}} {{other}}", payload)
    assert output == "D O"
    assert codes(reports) == ["unserializable-value"]


def test_none_is_a_present_value_and_undefined_is_absent() -> None:
    assert resolve("{{v; default:D}}", {"v": None}) == ("None", [])
    assert resolve("{{v; default:D}}", {"v": UNDEFINED}) == ("D", [])
    assert resolve("{{v; default:D}}", {"v": {"value": UNDEFINED}}) == ("D", [])


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        (True, "True"),
        (1, "1"),
        (1.5, "1.5"),
        ({"a": 1, "b": "x"}, '{"a":1,"b":"x"}'),
        ([1, "a", None, True], '[1,"a",null,true]'),
        ({"t": (1, 2)}, '{"t":[1,2]}'),
        ({2: "a", 1.5: "b", True: "c", None: "d"}, '{"2":"a","1.5":"b","true":"c","null":"d"}'),
        ({"k": collections.OrderedDict(a=1)}, '{"k":{"a":1}}'),
        ({"s": '\u2028\\"'}, '{"s":"\u2028\\\\\\""}'),
        ((1, 2), "(1, 2)"),
        ({1}, "{1}"),
    ],
)
def test_a_plain_dict_or_list_serializes_and_anything_else_converts_by_str(
    value: object, expected: str
) -> None:
    assert resolve("{{v}}", {"v": value}) == (expected, [])


class Point(NamedTuple):
    x: int


@dataclasses.dataclass
class Box:
    x: int


def test_records_and_tuples_convert_as_the_host_describes_them() -> None:
    assert resolve("{{a}} {{b}}", {"a": Point(1), "b": Box(2)})[0] == "Point(x=1) Box(x=2)"


@pytest.mark.parametrize(
    "value",
    [
        {"n": float("nan")},
        {"n": float("inf")},
        {"o": object()},
        {"u": UNDEFINED},
        {(1, 2): "tuple key"},
        pytest.param(10**5000, id="int-past-the-digit-limit"),
    ],
)
def test_a_value_no_conversion_describes_is_absent_and_reported(value: object) -> None:
    output, reports = resolve("{{v; default:D}}", {"v": value})
    assert output == "D"
    assert codes(reports) == ["unserializable-value"]
    assert reports[0].origin == "payload"


def test_a_value_that_holds_itself_is_absent_and_reported() -> None:
    held: dict[str, object] = {}
    held["self"] = held
    assert codes(resolve("{{v; default:D}}", {"v": held})[1]) == ["unserializable-value"]


def test_a_value_nested_past_the_host_stack_still_serializes() -> None:
    nested: list[object] = []
    for _ in range(40000):
        nested = [nested]
    output, reports = resolve("{{v}}", {"v": nested})
    assert output == "[" * 40001 + "]" * 40001
    assert reports == []


def test_a_conversion_that_raises_a_base_exception_is_not_contained() -> None:
    class Interrupting:
        def __str__(self) -> str:
            raise KeyboardInterrupt

    with pytest.raises(KeyboardInterrupt):
        create_parser().resolve("{{v}}", {"v": Interrupting()})


def test_a_value_that_resolves_itself_until_the_stack_runs_out_takes_its_chain() -> None:
    message = "A{{v; default:F}}B"

    class Again:
        def __str__(self) -> str:
            return "<" + parser.resolve(message, {"v": Again()}) + ">"

    def deeper(frames: int) -> str:
        return parser.resolve(message, {"v": Again()}) if frames == 0 else deeper(frames - 1)

    reports: list[Report] = []
    parser = create_parser(on_report=reports.append)
    # Which frame the stack runs out in depends on how deep the caller
    # starts, so the chain is started from a few depths.
    for frames in range(4):
        output = deeper(frames)
        assert (output[:2], output[-2:]) == ("A<", ">B")
        assert "A<AFB>B" in output
        assert "<>" not in output
    # Where it runs out in the conversion's own frame, the report is made a
    # frame or two from the limit, where it may be lost on any version; so
    # whether it arrives is not held.
    assert set(codes(reports)) <= {"unserializable-value"}


SWEEP = """
import traceback
from curly_message import create_parser

parser = create_parser()

def descend():
    try:
        return descend() + 1
    except RecursionError:
        return 0

def at(n):
    return at(n - 1) if n else parser.resolve("a{{x}}b", {"x": 1})

depth = descend()
for n in range(depth + 2, depth - 40, -1):
    try:
        at(n)
    except RecursionError as failure:
        for frame, line in traceback.walk_tb(failure.__traceback__):
            if frame.f_code is type(parser).resolve.__code__:
                print(n, line)
"""


def test_a_resolution_asked_for_where_the_stack_runs_out_answers_and_never_raises() -> None:
    # A fresh interpreter, deepest first: which call runs out depends on what
    # the process has already run, so the sweep is held to no test order.
    swept = subprocess.run(
        [sys.executable, "-c", SWEEP], capture_output=True, text=True, check=True
    ).stdout
    assert swept == ""


def test_wrappers_are_recognized_exactly() -> None:
    assert resolve("{{v}}", {"v": {"value": 1, "unit": "kg"}})[0] == '{"value":1,"unit":"kg"}'
    assert resolve("{{v}}", {"v": {}})[0] == "{}"
    assert resolve("{{v}}", {"v": {"value": {"value": 1}}})[0] == '{"value":1}'
    assert resolve("{{v; default:I}}", {"v": {"props": {}}})[0] == "I"
    assert resolve("{{default}}", {"default": {"value": "x"}})[0] == '{"value":"x"}'


def test_a_wrapper_shaped_dict_subclass_is_a_value() -> None:
    # How the host spells one differs between its versions, so the expectation
    # is what this one makes of it.
    wrapped = collections.OrderedDict(value="x")
    assert resolve("{{v}}", {"v": wrapped}) == (str(wrapped), [])
    ordered = collections.OrderedDict(a=1)
    assert resolve("{{v}}", {"v": ordered}) == (str(ordered), [])


@pytest.mark.parametrize("off", [False])
def test_recognition_can_be_turned_off(off: bool) -> None:
    payload = {"v": {"value": "x"}}
    assert resolve("{{v}}", payload, recognize_wrappers=off)[0] == '{"value":"x"}'


@pytest.mark.parametrize("given", [None, 0, "", "no"])
def test_recognition_given_as_anything_but_a_bool_stays_on(given: object) -> None:
    assert resolve("{{v}}", {"v": {"value": "x"}}, recognize_wrappers=given)[0] == "x"


def test_the_default_entry_is_read_once_as_a_value_and_as_its_own_chain() -> None:
    held: dict[str, object] = {}
    held["self"] = held
    output, reports = resolve("{{default; default:I}}", {"default": held})
    assert output == "I"
    assert codes(reports) == ["unserializable-value"]
    # Read twice, 60 000 characters would spend the budget past the limit and
    # the placeholder after it would pay.
    long = "x" * 60000
    output, reports = resolve("{{default:eq; z:Z}}{{w}}", {"default": long, "w": "W"})
    assert output == long + "W"
    assert reports == []


def test_eq_and_ne_compare_lowercased_text() -> None:
    assert resolve("{{v:eq; STRASSE:X; default:D}}", {"v": "stra\xdfe"})[0] == "D"
    assert resolve("{{v:eq; \u212a:K; default:D}}", {"v": "k"})[0] == "K"
    assert resolve("{{v; A:first; a:second}}", {"v": "a"})[0] == "first"
    assert resolve("{{v:ne; a:A; b:B; default:D}}", {"v": "A"})[0] == "B"
    assert resolve("{{v:eq; 1:one; default:D}}", {"v": 1.0})[0] == "D"


def test_eq_lowercases_by_the_unicode_version_this_host_carries() -> None:
    # U+A7CB lowercases to U+0264 from Unicode 16.0; before it, to itself.
    joined = "\ua7cb".lower() == "\u0264"
    assert joined == (tuple(map(int, unicodedata.unidata_version.split("."))) >= (16, 0))
    output = resolve("{{v:eq; \ua7cb:X; default:D}}", {"v": "\u0264"})[0]
    assert output == ("X" if joined else "D")


def test_lt_and_gt_order_numeric_keys_and_pass_over_the_rest() -> None:
    assert resolve("{{v:lt; 5:A; abc:N; 3:B;}}", {"v": 2})[0] == "B"
    assert resolve("{{v:gt; 1:A; 3:B; default:D}}", {"v": 5})[0] == "B"
    assert resolve("{{v:lt; 2.0:first; 2:second}}", {"v": 1})[0] == "first"
    assert resolve("{{v:gt; 2:first; 2.0:second}}", {"v": 3})[0] == "first"
    assert resolve("{{v:lt; 5:A; default:D}}", {"v": "abc"})[0] == "D"


def test_a_nan_key_is_never_selected_and_freezes_nothing() -> None:
    assert resolve("{{v:lt; 5:A; nan:N; 3:B;}}", {"v": 2})[0] == "B"
    assert resolve("{{v:gt; 1:A; NaN:N; -nan:M; 3:B;}}", {"v": 5})[0] == "B"
    assert resolve("{{v:gte; 1:A; default:D}}", {"v": "nan"})[0] == "D"
    assert resolve("{{v:lt; 5:A; default:D}}", {"v": float("nan")})[0] == "D"


def test_numbers_are_read_by_the_host_conversion_after_the_format_whitespace() -> None:
    assert resolve("{{v:lt; 13:A; default:D}}", {"v": "\ufeff12"})[0] == "A"
    assert resolve("{{v:gt; 999:A; default:D}}", {"v": "1_000"})[0] == "A"
    # Blank text is no number to the host's conversion.
    assert resolve("{{v:lt; 1:A; default:D}}", {"v": " "})[0] == "D"


def test_a_code_point_the_host_skips_outside_the_class_is_part_of_the_number() -> None:
    message = "{{v:gt; 1:X; default:D}}"
    assert resolve(message, {"v": "\x855"})[0] == "D"
    assert resolve(message, {"v": "5\x85"})[0] == "D"
    assert resolve(message, {"v": "\u3000\x855 "})[0] == "D"
    assert resolve("{{v:lt; \x855:A; default:D}}", {"v": "3"})[0] == "D"
    # The near miss: the class around a number, escaped into a key or not, is not part of it.
    assert resolve(message, {"v": "\u30005\ufeff"})[0] == "X"
    assert resolve("{{v:lt; \\\u30005\\\ufeff:A; default:D}}", {"v": "3"})[0] == "A"


def _reads_as_five(text: str) -> bool:
    try:
        return float(text) == 5
    except ValueError:
        return False


def test_no_code_point_float_skips_outside_the_class_is_skipped_around_a_number() -> None:
    # What float skips is the running CPython's, so it is read off it here.
    skipped = [c for c in map(chr, range(sys.maxunicode + 1)) if _reads_as_five(c + "5" + c)]
    assert "\x85" in skipped
    for character in skipped:
        expected = "X" if character in WHITESPACE else "D"
        for value in (character + "5", "5" + character, character + "5" + character):
            assert resolve("{{v:gt; 1:X; default:D}}", {"v": value})[0] == expected, ascii(value)


def test_lte_and_gte_compare_for_equality_first() -> None:
    assert resolve("{{v:lte; abc:X; default:D}}", {"v": "ABC"})[0] == "X"
    assert resolve("{{v:lte; 5:five; 2.0:two; default:D}}", {"v": "2.0"})[0] == "two"
    assert resolve("{{v:gte; 5:five; 2:two; default:D}}", {"v": 3})[0] == "two"
    assert resolve("{{v:lte; 1:one; default:D}}", {"v": 3})[0] == "D"


def test_a_selection_answers_with_its_option_even_when_empty() -> None:
    assert resolve("{{v; a:; default:D}}", {"v": "a"})[0] == ""
    assert resolve("{{v; a; default:D}}", {"v": "a"})[0] == "a"
    assert resolve("{{v; a:A}}", {"v": "RAW"})[0] == ""


def test_a_value_is_data_and_message_text_is_syntax() -> None:
    payload = {"v": "{{w}}\\;", "w": "W"}
    assert resolve("{{v}} \\{{w}}", payload)[0] == "{{w}}\\; {{w}}"
    assert resolve("{{v; a:\\{\\:{{w}}}}", {"v": "a", "w": "W"})[0] == "{:W"


def test_the_whitespace_class_is_the_format_s_and_not_the_host_s() -> None:
    assert resolve("{{ v\x85}}", {"v\x85": "kept"})[0] == "kept"
    assert resolve("{{\u3000v\xa0}}", {"v": "trimmed"})[0] == "trimmed"


def test_message_errors_take_the_chain_and_are_reported_in_order() -> None:
    output, reports = resolve(
        "{{v:zz; default:A}} {{v:eq; default:B}} {{:zz}} {{v:EQ; x:X; default:C}}", {"v": "x"}
    )
    assert output == "A B  C"
    assert codes(reports) == [
        "unknown-modifier",
        "missing-options",
        "unknown-modifier",
        "unknown-modifier",
    ]
    assert [report.origin for report in reports] == ["message"] * 4
    assert resolve("{{:eq}}") == ("", [])


def test_a_placeholder_nested_too_deep_takes_its_chain() -> None:
    message = "{{v; a:" * MAX_NESTING + "{{v; a:deep; default:fallback}}" + "}}" * MAX_NESTING
    output, reports = resolve(message, {"v": "a"})
    assert output == "fallback"
    assert [(r.code, r.limit) for r in reports] == [("nesting-limit", MAX_NESTING)]


def test_only_the_selected_option_is_rendered() -> None:
    output, reports = resolve("{{v; a:A; b:{{w:zz}}}}", {"v": "a"})
    assert (output, reports) == ("A", [])
    assert codes(resolve("{{v; a:A; b:{{w:zz}}}}", {"v": "b"})[1]) == ["unknown-modifier"]


def test_the_output_limit_charges_the_top_level_once() -> None:
    # The nested result reaches the output once, through the one around it,
    # so the budget left is exactly what the second placeholder writes.
    nested, rest = "x" * 50000, "y" * (MAX_OUTPUT_LENGTH - 50000)
    message = f"A{{{{v; a:{{{{w}}}}}}}}{{{{v; a:{rest}}}}}{{{{v; a:z}}}}B"
    output, reports = resolve(message, {"v": "a", "w": nested})
    assert output == f"A{nested}{rest}B"
    assert [(r.code, r.origin, r.limit) for r in reports] == [
        ("output-limit", "limit", MAX_OUTPUT_LENGTH)
    ]


def test_the_read_limit_is_tested_before_a_placeholder_reads() -> None:
    long = "x" * (MAX_READ_LENGTH + 1)
    output, reports = resolve("{{v:eq; y:Y; default:ok}}{{w}}{{w}}", {"v": long, "w": "W"})
    assert output == "ok"
    assert [(r.code, r.limit) for r in reports] == [
        ("read-limit", MAX_READ_LENGTH),
        ("read-limit", MAX_READ_LENGTH),
    ]


def test_a_report_says_what_and_where_and_names_the_message_id() -> None:
    reports: list[Report] = []
    parser = create_parser(on_report=reports.append)
    parser.resolve("a {{v:zz}} b", {}, message_id=["ids", "are", "any", "value"])
    (report,) = reports
    assert report.code == "unknown-modifier"
    assert report.origin == "message"
    assert report.message == "A placeholder named a modifier this parser does not know."
    assert report.text == "{{v:zz}}"
    assert report.message_id == ["ids", "are", "any", "value"]
    assert report.limit is None
    parser.resolve("{{v:zz}}")
    assert reports[1].message_id is None
    parser.resolve("{{v:zz}}", message_id=UNDEFINED)
    assert reports[2].message_id is None


def test_a_report_is_read_only_and_compares_by_its_fields() -> None:
    first = resolve("{{v:zz}}")[1][0]
    second = resolve("{{v:zz}}")[1][0]
    assert first == second
    assert hash(first) == hash(second)
    assert first != resolve("{{w:zz}}")[1][0]
    assert repr(first).startswith("Report(code='unknown-modifier', origin='message'")
    with pytest.raises(AttributeError):
        first.code = "read-limit"
    with pytest.raises(AttributeError):
        del first.text


def test_a_report_text_is_cut_and_escaped() -> None:
    name = '\x0b\x0c\x1c\x85\ud800"\\' + "y" * 200
    report = resolve(f"{{{{{name}:zz}}}}")[1][0]
    assert report.text == ('{{\\u000b\\f\\u001c\\u0085\\ud800\\"\\\\' + "y" * (120 - 9) + "...")


def test_no_report_text_holds_anything_the_host_splits_lines_at() -> None:
    # Every code point a modifier name can hold: a line terminator ends a
    # placeholder, and a reserved character divides one.
    held = (*map(ord, "{}\\:;"), 0x0A, 0x0D, 0x2028, 0x2029)
    every = "".join(chr(code) for code in range(0x3000) if code not in held)
    for start in range(0, len(every), 100):
        report = resolve("{{v" + every[start : start + 100] + ":zz}}")[1]
        assert len(report[0].text.splitlines()) == 1
        report[0].text.encode()


def test_a_report_handler_that_raises_loses_only_its_report() -> None:
    def failing(report: Report) -> None:
        raise RuntimeError("logger down")

    assert create_parser(on_report=failing).resolve("{{v:zz; default:D}}!") == "D!"


@pytest.mark.parametrize("handler", ["not callable", 1])
def test_a_handler_that_cannot_be_called_is_no_handler(handler: object) -> None:
    assert create_parser(on_report=handler).resolve("{{v:zz; default:D}}") == "D"  # type: ignore[arg-type]


def test_this_installation_satisfies_the_core_and_extensions_levels() -> None:
    assert levels() == frozenset({"core", "extensions"})


def test_importing_the_package_loads_no_optional_dependency() -> None:
    probe = "import sys, curly_message; print(sorted({'babel', 'dataclasses'} & set(sys.modules)))"
    loaded = subprocess.run(
        [sys.executable, "-c", probe], capture_output=True, text=True, check=True
    ).stdout
    assert loaded.strip() == "[]"


# Host-defined modifiers (section 11.3).


def props_of(config: ModifierInput) -> str:
    return ",".join(f"{name}={config.props[name]!r}" for name in sorted(config.props))


def with_modifiers(
    message: object, payload: Any = None, modifiers: Any = None, **options: Any
) -> tuple[str, list[Report]]:
    return resolve(message, payload, custom_modifiers=modifiers, **options)


def test_a_host_modifier_is_handed_the_value_the_options_and_the_default() -> None:
    def show(config: ModifierInput) -> str:
        options = ",".join(f"{o.key}={o.value}" for o in config.options)
        return f"{config.value}|{options}|{config.default}"

    message = "{{v:show; a:A{{w}}; b:; a:again; default:D}}"
    assert with_modifiers(message, {"v": "x", "w": "W"}, {"show": show}) == (
        "x|a=AW,b=,a=again|D",
        [],
    )


def test_a_host_modifier_answers_with_a_value_converted_like_a_payload_value() -> None:
    answers: dict[str, Modifier] = {
        "number": lambda config: 1,
        "object": lambda config: {"answer": config.value},
        "list": lambda config: [config.value, None],
        "empty": lambda config: "",
    }
    message = "{{v:number}} {{v:object}} {{v:list}} [{{v:empty; default:D}}]"
    assert with_modifiers(message, {"v": "x"}, answers) == (
        '1 {"answer":"x"} ["x",null] []',
        [],
    )


@pytest.mark.parametrize("nothing", [None, UNDEFINED])
def test_a_host_modifier_answering_nothing_takes_the_chain(nothing: object) -> None:
    assert with_modifiers("{{v:none; default:D}}", {"v": "x"}, {"none": lambda c: nothing}) == (
        "D",
        [],
    )


def test_a_host_modifier_answering_what_no_conversion_describes_takes_the_chain() -> None:
    cycle: dict[str, object] = {}
    cycle["self"] = cycle
    modifiers = {"cycle": lambda config: cycle}
    output, reports = with_modifiers("{{v:cycle; default:D}}", {"v": "x"}, modifiers)
    assert (output, codes(reports)) == ("D", ["unserializable-value"])


def test_a_host_modifier_that_raises_is_contained_and_reported() -> None:
    def failing(config: ModifierInput) -> str:
        raise RuntimeError("down")

    output, reports = with_modifiers("{{v:fail; default:D}}!", {"v": "x"}, {"fail": failing})
    assert (output, codes(reports)) == ("D!", ["failed-modifier"])
    assert reports[0].origin == "payload"
    assert reports[0].text == "{{v:fail; default:D}}"


def test_a_default_read_before_a_raise_is_read_once() -> None:
    reads: list[str] = []

    def failing(config: ModifierInput) -> str:
        reads.append(config.default)
        raise RuntimeError("down")

    def counted(config: ModifierInput) -> str:
        reads.append("chain")
        return "C"

    message = "{{v:fail; default:{{w:count}}}}"
    output, reports = with_modifiers(
        message, {"v": "x", "w": "y"}, {"fail": failing, "count": counted}
    )
    assert (output, codes(reports), reads) == ("C", ["failed-modifier"], ["chain", "C"])


def test_an_interrupt_a_host_modifier_raises_is_not_contained() -> None:
    def interrupted(config: ModifierInput) -> str:
        raise KeyboardInterrupt

    with pytest.raises(KeyboardInterrupt):
        with_modifiers("{{v:stop}}", {"v": "x"}, {"stop": interrupted})


def test_a_host_modifier_takes_its_name_from_the_format_s_own() -> None:
    def echo(config: ModifierInput) -> str:
        return f"<{config.value}>"

    modifiers = {"eq": echo, "lt": echo}
    # A placeholder declaring options and naming no modifier asks `eq`, and a
    # selection a host replaced is no selection, so it needs no options.
    message = "{{v:eq; x:X}} {{v; x:X}} {{v:lt}} {{v:gt; 1:G; default:D}}"
    assert with_modifiers(message, {"v": "x"}, modifiers) == ("<x> <x> <x> D", [])


@pytest.mark.parametrize("entry", [3, "eq", None, UNDEFINED])
def test_an_entry_that_cannot_be_called_registers_nothing(entry: object) -> None:
    output, reports = with_modifiers(
        "{{v:eq; x:X; default:D}} {{v:own; default:O}}", {"v": "x"}, {"eq": entry, "own": entry}
    )
    assert (output, codes(reports)) == ("X O", ["unknown-modifier"])


def test_a_registered_modifier_is_called_whatever_it_answers_to_truth() -> None:
    class Untruthful:
        def __call__(self, config: ModifierInput) -> str:
            return "called"

        def __bool__(self) -> bool:
            raise RuntimeError("asked")

        def __len__(self) -> int:
            return 0

    modifiers = {"own": Untruthful(), "eq": Untruthful()}
    assert with_modifiers("{{v:own}} {{v; x:X}}", {"v": "x"}, modifiers) == ("called called", [])


def test_a_registry_holds_only_its_own_string_keyed_entries() -> None:
    class Counting(dict[Any, Any]):
        reads = 0

        def __getitem__(self, key: object) -> Any:
            Counting.reads += 1
            return super().__getitem__(key)

        def get(self, key: object, default: Any = None) -> Any:
            Counting.reads += 1
            return super().get(key, default)

    def upper(config: ModifierInput) -> str:
        return config.value.upper()

    registry = Counting({Name.NAME: upper, 1: upper, "__proto__": upper})
    message = "{{v:name}} {{v:__proto__}} {{v:1; default:D}}"
    output, reports = with_modifiers(message, {"v": "x"}, registry)
    assert (output, codes(reports), Counting.reads) == ("X X D", ["unknown-modifier"], 0)
    for other in (types.MappingProxyType({"name": upper}), collections.ChainMap({"name": upper})):
        output, reports = with_modifiers("{{v:name; default:D}}", {"v": "x"}, other)
        assert (output, codes(reports)) == ("D", ["unknown-modifier"])


@pytest.mark.parametrize("name", ["__class__", "__init__", "get", "items", "keys", "__getitem__"])
def test_a_class_member_is_no_registered_modifier(name: str) -> None:
    output, reports = with_modifiers(f"{{{{v:{name}; default:D}}}}", {"v": "x"}, {})
    assert (output, codes(reports)) == ("D", ["unknown-modifier"])


def test_the_registry_and_the_defaults_are_read_again_on_each_resolution() -> None:
    registry: dict[str, Modifier] = {}
    defaults: dict[str, dict[str, Any]] = {}
    parser = create_parser(custom_modifiers=registry, modifier_defaults=defaults)
    message = "{{v:show; default:D}}"
    assert parser.resolve(message, {"v": "x"}) == "D"
    registry["show"] = props_of
    defaults["show"] = {"k": 1}
    assert parser.resolve(message, {"v": "x"}) == "k=1"


def test_props_compose_per_property_under_the_modifier_s_own_name() -> None:
    def show(config: ModifierInput) -> str:
        return f"{config.value}|{props_of(config)}"

    defaults: dict[str, dict[str, Any]] = {
        "show": {"a": "default", "b": "default", "c": "default"},
        "other": {"z": 1},
    }
    props: dict[str, dict[str, Any]] = {"show": {"b": "call", "c": "call"}, "other": {"y": 1}}
    payload = {"v": {"value": "x", "props": {"show": {"c": "wrapper"}, "other": {"x": 1}}}}
    parser = create_parser(custom_modifiers={"show": show}, modifier_defaults=defaults)
    assert parser.resolve("{{v:show}}", payload, props=props) == (
        "x|a='default',b='call',c='wrapper'"
    )
    # An entry read as a value configures nothing.
    off = create_parser(
        custom_modifiers={"show": show}, modifier_defaults=defaults, recognize_wrappers=False
    )
    assert off.resolve("{{v:show}}", {"v": {"value": "x"}}, props=props) == (
        """{"value":"x"}|a='default',b='call',c='call'"""
    )


def test_a_property_holding_undefined_names_nothing_and_none_is_named() -> None:
    parser = create_parser(
        custom_modifiers={"show": props_of}, modifier_defaults={"show": {"a": 1, "b": 1}}
    )
    props = {"show": {"a": UNDEFINED, "b": None, "c": UNDEFINED}}
    assert parser.resolve("{{v:show}}", {"v": "x"}, props=props) == "a=1,b=None"


@pytest.mark.parametrize(
    "layer", ["text", 1, None, UNDEFINED, ["a"], types.MappingProxyType({"a": 1})]
)
def test_an_entry_that_is_no_layer_configures_nothing(layer: object) -> None:
    parser = create_parser(
        custom_modifiers={"show": props_of}, modifier_defaults={"show": {"a": 1}}
    )
    payload = {"v": {"value": "x", "props": {"show": layer}}}
    assert parser.resolve("{{v:show}}", payload, props={"show": layer}) == "a=1"  # type: ignore[dict-item]
    assert parser.resolve("{{v:show}}", payload, props=layer) == "a=1"  # type: ignore[arg-type]


def test_props_layers_read_their_own_string_keyed_entries() -> None:
    class Overriding(dict[Any, Any]):
        def __getitem__(self, key: object) -> Any:
            raise AssertionError("read through an override")

        def items(self) -> Any:
            raise AssertionError("read through an override")

    parser = create_parser(custom_modifiers={"show": props_of})
    props = Overriding({Name.NAME: "ignored", "show": Overriding({Name.VALUE: 1, 2: 2})})
    assert parser.resolve("{{v:show}}", {"v": "x"}, props=props) == "value=1"


def test_a_layer_lookup_that_raises_is_reported_and_names_nothing() -> None:
    class Colliding(str):
        __slots__ = ()

        def __hash__(self) -> int:
            return hash("show")

        def __eq__(self, other: object) -> bool:
            raise RuntimeError("compared")

    parser_reports: list[Report] = []
    parser = create_parser(
        custom_modifiers={"show": props_of},
        modifier_defaults={"show": {"a": 1}},
        on_report=parser_reports.append,
    )
    props: Any = {Colliding("other"): {"b": 2}}
    assert parser.resolve("{{v:show}}|{{w}}", {"v": "x", "w": "W"}, props=props) == "a=1|W"
    assert codes(parser_reports) == ["unserializable-value"]


def test_the_props_a_modifier_is_handed_are_its_own_copy() -> None:
    def scribble(config: ModifierInput) -> str:
        shown = props_of(config)
        config.props["a"] = "written"
        config.props["new"] = 1
        return shown

    props = {"scribble": {"a": 1}}
    parser = create_parser(custom_modifiers={"scribble": scribble})
    assert parser.resolve("{{v:scribble}} {{v:scribble}}", {"v": "x"}, props=props) == "a=1 a=1"
    assert props == {"scribble": {"a": 1}}


_LOCALE = object()


@pytest.mark.parametrize(
    ("locale", "expected"),
    [(None, None), (UNDEFINED, None), ("", ""), ("cs-CZ", "cs-CZ"), (_LOCALE, _LOCALE)],
)
def test_a_host_modifier_is_handed_the_locale_as_the_caller_passed_it(
    locale: object, expected: object
) -> None:
    seen: list[object] = []

    def record(config: ModifierInput) -> str:
        seen.append(config.locale)
        return "ok"

    parser = create_parser(custom_modifiers={"record": record})
    assert parser.resolve("{{v:record}}", {"v": "x"}, locale=locale) == "ok"
    assert seen == [expected]


def test_what_a_modifier_keeps_answers_nothing_after_the_resolution() -> None:
    kept: list[ModifierInput] = []

    def keep(config: ModifierInput) -> str:
        kept.append(config)
        return "now"

    reports: list[Report] = []
    parser = create_parser(custom_modifiers={"keep": keep}, on_report=reports.append)
    assert parser.resolve("{{v:keep; a:{{w:zz}}; default:{{w:zz}}}}", {"v": "x"}) == "now"
    assert (kept[0].options[0].value, kept[0].default, reports) == ("", "", [])

    converted: list[None] = []

    class Fallback:
        def __str__(self) -> str:
            converted.append(None)
            return "F"

    assert parser.resolve("{{v:keep}}", {"v": "x", "default": Fallback()}) == "now"
    assert (kept[1].default, converted, reports) == ("", [], [])


def test_what_a_modifier_read_and_keeps_answers_the_same_after_the_resolution() -> None:
    kept: list[ModifierInput] = []

    def keep(config: ModifierInput) -> str:
        kept.append(config)
        return config.default + "|" + config.options[0].value

    reports: list[Report] = []
    parser = create_parser(custom_modifiers={"keep": keep}, on_report=reports.append)
    message = "{{v:keep; a:{{w:zz; default:A}}; default:{{w:zz; default:D}}}}"
    assert parser.resolve(message, {"v": "x"}) == "D|A"
    assert len(reports) == 2
    assert (kept[0].options[0].value, kept[0].default, len(reports)) == ("A", "D", 2)


def test_a_resolution_begun_by_a_modifier_owns_its_budgets_and_its_id() -> None:
    reports: list[Report] = []

    def nested(config: ModifierInput) -> str:
        return parser.resolve("{{w:zz; default:in}}", {"w": "y"}, message_id="inner")

    parser = create_parser(custom_modifiers={"nested": nested}, on_report=reports.append)
    output = parser.resolve("{{v:nested}}|{{w:zz; default:out}}", {"v": "x"}, message_id="outer")
    assert output == "in|out"
    assert [(r.code, r.message_id) for r in reports] == [
        ("unknown-modifier", "inner"),
        ("unknown-modifier", "outer"),
    ]


def test_a_chain_of_kept_options_ends_at_the_interpreter_s_limit_contained() -> None:
    kept: list[Option] = []

    def keep(config: ModifierInput) -> str:
        kept.append(config.options[0])
        return "k"

    def pop(config: ModifierInput) -> str:
        return kept.pop().value if kept else "e"

    reports: list[Report] = []
    parser = create_parser(custom_modifiers={"k": keep, "m": pop}, on_report=reports.append)
    # A report is built on the exhausted stack, where it may be lost, so how
    # many arrive is not held; the first of a process also builds the escape
    # table there, so one is made before.
    parser.resolve("{{x:zz}}", {"x": 1})
    reports.clear()
    n = 2000
    output = parser.resolve("{{x:k; o:{{x:m}}}}" * n + "{{x:m}}", {"x": 1})
    assert output == "k" * n
    assert codes(reports)
    assert set(codes(reports)) == {"failed-modifier"}


def test_a_kept_option_that_reads_itself_takes_its_fallback_chain() -> None:
    kept: list[ModifierInput] = []

    def keep(config: ModifierInput) -> str:
        kept.append(config)
        return "k"

    def reread(config: ModifierInput) -> str:
        return kept[-1].options[0].value if kept else config.default

    reports: list[Report] = []
    parser = create_parser(custom_modifiers={"k": keep, "r": reread}, on_report=reports.append)
    parser.resolve("{{x:zz}}", {"x": 1})
    reports.clear()
    output = parser.resolve("{{x:k; o:{{x:r; default:D}}}}{{x:r}}", {"x": 1})
    assert output == "kD"
    assert codes(reports)
    assert set(codes(reports)) == {"failed-modifier"}


def test_a_modifier_that_resolves_itself_until_the_stack_runs_out_takes_its_chain() -> None:
    message = "A{{x:again; default:F}}B"

    def again(config: ModifierInput) -> str:
        return "<" + parser.resolve(message, {"x": 1}) + ">"

    reports: list[Report] = []
    parser = create_parser(custom_modifiers={"again": again}, on_report=reports.append)
    parser.resolve("{{x:zz}}", {"x": 1})
    reports.clear()
    output = parser.resolve(message, {"x": 1})
    assert (output[:2], output[-2:]) == ("A<", ">B")
    assert "A<AFB>B" in output
    assert "<>" not in output
    assert codes(reports)
    assert set(codes(reports)) == {"failed-modifier"}


def test_a_resolution_on_a_thread_a_modifier_starts_answers_where_its_walk_runs_out() -> None:
    # A thread runs on a call stack of its own, so the resolution in progress
    # on the thread that started it is not in progress there (section 13):
    # asked for at every depth near the limit, it answers and never raises.
    message = "a{{x}}b{{y; default:{{z; default:D}}}}"
    answers: set[str] = set()
    raised: list[str] = []

    def descend() -> int:
        try:
            return descend() + 1
        except RecursionError:
            return 0

    def at(n: int) -> str:
        return at(n - 1) if n else parser.resolve(message, {"x": 1})

    def sweep() -> None:
        depth = descend()
        for back in range(1, 60):
            try:
                answers.add(at(depth - back))
            except RecursionError as failure:
                # A call that could not enter `resolve` at all is the host's.
                walked = traceback.walk_tb(failure.__traceback__)
                if any(frame.f_code is type(parser).resolve.__code__ for frame, _ in walked):
                    raised.append(traceback.format_exc())

    def spawn(config: ModifierInput) -> str:
        thread = threading.Thread(target=sweep)
        thread.start()
        thread.join()
        return "s"

    parser = create_parser(custom_modifiers={"spawn": spawn})
    assert parser.resolve("{{x:spawn}}", {"x": 1}) == "s"
    assert raised == []
    assert answers == {"", "a1bD"}
