"""Written-out and generated cases: their inputs built, their expectations computed, compared."""

from __future__ import annotations

import inspect
import json
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from typing import Final, cast

from .behaviours import behaviours
from .decode import decode, nodes
from .model import (
    CATEGORIES,
    OMIT,
    Adapter,
    Failed,
    Outcome,
    Passed,
    Resolution,
    Resolved,
    SetError,
)

Request = Mapping[str, object]

ID: Final = "limits"
"""The message id every generated case reports under."""


@dataclass(frozen=True)
class Expectation:
    """What a case expects: an output, or a request whose text is read only where it is needed."""

    output: str | None
    reports: Sequence[Mapping[str, object]]
    format: Request | None = None
    categories: Mapping[str, str] | None = None
    locale: object = OMIT

    def shown(self) -> object:
        return self.output if self.output is not None else self.format


@dataclass(frozen=True)
class Prepared:
    input: Resolution
    expected: Expectation


def _field(case: Mapping[str, object], name: str, undefined: object) -> object:
    return decode(case[name], undefined) if name in case else OMIT


def _categories(case: Mapping[str, object]) -> Mapping[str, str]:
    # A selection case names the output of every category. One that leaves a
    # category out is the set's defect, whichever reading it is measured on and
    # whichever category a host answers, so it ends the run before anything
    # is compared.
    expected = cast("Mapping[str, object]", case["expected"])
    named = expected.get("categories")
    missing = [
        name
        for name in CATEGORIES
        if not isinstance(named, dict) or not isinstance(named.get(name), str)
    ]
    if missing:
        raise SetError(f"The case {case['id']} names no output for the category {missing[0]!r}.")
    return cast("Mapping[str, str]", named)


def concrete(case: Mapping[str, object], adapter: Adapter) -> Prepared:
    """A written-out case, its inputs decoded for the adapter's host."""
    undefined = adapter.undefined
    expected = cast("Mapping[str, object]", case["expected"])
    request = cast("Request | None", expected.get("format"))
    categories = None
    if request is not None and request.get("api") == "PluralRules":
        categories = _categories(case)
    output = expected.get("output")
    if request is None and not isinstance(output, str):
        raise SetError(f"The case {case['id']} expects neither an output nor a format.")
    table = behaviours(undefined)
    named = cast("Mapping[str, str] | None", case.get("modifiers"))
    modifiers = None if named is None else {name: table[kind] for name, kind in named.items()}
    locale = case.get("locale", OMIT)
    return Prepared(
        Resolution(
            message=_field(case, "message", undefined),
            payload=_field(case, "payload", undefined),
            props=_field(case, "props", undefined),
            locale=locale,
            id=_field(case, "messageId", undefined),
            modifiers=modifiers,
            defaults=_field(case, "defaults", undefined),
        ),
        Expectation(
            output=cast("str", output) if request is None else None,
            reports=cast("Sequence[Mapping[str, object]]", expected.get("reports", [])),
            format=request,
            categories=categories,
            locale=locale,
        ),
    )


def _writes(length: int) -> str:
    # An output of the stated length written entirely by the message: an option
    # value is the message's own text, so the payload is read for one character.
    return "{{v; a:" + "x" * length + ";}}"


_READS: Final = "{{v:eq; nomatch:X; default:ok;}}"


def _nest(levels: int) -> str:
    # Each level selects the option holding the next; the innermost declares
    # a fallback, so a level the nesting limit refuses shows as its chain.
    return "{{v; a:" * (levels - 1) + "{{v; a:settled; default:fallback;}}" + ";}}" * (levels - 1)


def _limit(code: str, origin: str, declared: int) -> Mapping[str, object]:
    return {"code": code, "origin": origin, "id": ID, "limit": declared}


def _generated(name: str, limits: Mapping[str, int]) -> Prepared:
    output, read = limits["output"], limits["read"]
    conversion, nesting = limits["conversion"], limits["nesting"]

    def case(
        message: str, payload: dict[str, object], text: str, *reports: Mapping[str, object]
    ) -> Prepared:
        return Prepared(
            Resolution(message=message, payload=payload, id=ID),
            Expectation(output=text, reports=reports),
        )

    builders: dict[str, Callable[[], Prepared]] = {
        "output-at-limit": lambda: case(_writes(output), {"v": "a"}, "x" * output),
        "output-over-limit": lambda: case(
            "A" + _writes(output + 1) + "B",
            {"v": "a"},
            "AB",
            _limit("output-limit", "limit", output),
        ),
        "output-over-limit-continues": lambda: case(
            _writes(output + 1) + "{{w}}",
            {"v": "a", "w": "tail"},
            "tail",
            _limit("output-limit", "limit", output),
        ),
        "read-at-limit": lambda: case(_READS, {"v": "x" * read}, "ok"),
        "read-over-limit": lambda: case(
            _READS + "{{w}}",
            {"v": "x" * (read + 1), "w": "tail"},
            "ok",
            _limit("read-limit", "limit", read),
        ),
        "conversion-over-limit": lambda: case(
            "{{v; default:D}}",
            {"v": nodes(conversion + 1)},
            "D",
            {"code": "unserializable-value", "origin": "payload", "id": ID},
        ),
        "nesting-at-limit": lambda: case(_nest(nesting), {"v": "a"}, "settled"),
        "nesting-over-limit": lambda: case(
            _nest(nesting + 1),
            {"v": "a"},
            "fallback",
            _limit("nesting-limit", "message", nesting),
        ),
    }
    if name not in builders:
        raise SetError(f"No construction is named {name!r}.")
    return builders[name]()


def _number(value: object) -> bool:
    return isinstance(value, int | float) and not isinstance(value, bool)


def same(left: object, right: object) -> bool:
    """Whether two JSON-shaped values hold the same entries, whatever order a host wrote them in.

    A number is compared by its value, since JSON tells no integer from a
    float, and a boolean is never a number. The walk is driven by ``right``,
    the expectation, which is finite JSON: a cyclic ``left`` ends it.
    """
    stack = [(left, right)]
    while stack:
        a, b = stack.pop()
        if isinstance(b, Mapping):
            if not isinstance(a, Mapping) or set(a.keys()) != set(b.keys()):
                return False
            stack.extend((a[key], b[key]) for key in b)
        elif isinstance(b, list | tuple):
            if not isinstance(a, list | tuple) or len(a) != len(b):
                return False
            stack.extend(zip(a, b, strict=True))
        elif _number(b):
            if not _number(a) or a != b:
                return False
        elif isinstance(b, bool) or b is None:
            if a is not b:
                return False
        elif not (isinstance(a, str) and isinstance(b, str) and a == b):
            return False
    return True


def _requests(expected: Request, actual: object) -> Failed | None:
    # A case that states a request writes the placeholder alone, so the
    # resolution it describes makes exactly one.
    if not isinstance(actual, list | tuple):
        return Failed("The formatting requests are not a list.", [expected], actual)
    if len(actual) != 1:
        return Failed(f"Expected one formatting request, got {len(actual)}.", [expected], actual)
    request = actual[0]
    if not isinstance(request, Mapping) or request.get("api") != expected["api"]:
        return Failed("The facility of the request differs.", expected, request)
    options = request.get("options")
    if not same({} if options is None else options, expected.get("options") or {}):
        return Failed("The properties of the request differ.", expected, request)
    if "input" not in request or not same(request["input"], expected["input"]):
        return Failed("The input of the request differs.", expected, request)
    return None


def _selection(expected: Expectation, request: Request, output: object) -> Failed | None:
    # An exposed selection says what was asked; the category it carries is what
    # the host answered, and the output is held to the one the case names for it.
    category = request.get("category")
    if not isinstance(category, str) or category not in CATEGORIES:
        return Failed(
            "The selection request carries no category the plural rules name.",
            list(CATEGORIES),
            category,
        )
    chosen = cast("Mapping[str, str]", expected.categories)[category]
    if output != chosen:
        return Failed(
            f"The output differs from the one the category {category} selects.", chosen, output
        )
    return None


def _text(expected: Expectation) -> str | None:
    # The text reading: the runner performs the request on its own host. It
    # needs Babel, which an installation at the Core level lacks; nothing else
    # of the runner does.
    from .perform import perform  # noqa: PLC0415

    request = cast("Request", expected.format)
    answered = perform(request, expected.locale)
    if request.get("api") == "PluralRules":
        return cast("Mapping[str, str]", expected.categories).get(answered)
    return answered


def _produced(expected: Expectation, resolved: Resolved) -> Failed | None:
    if expected.format is not None and resolved.formats is not None:
        wrong = _requests(expected.format, resolved.formats)
        if wrong is None and expected.format.get("api") == "PluralRules":
            return _selection(expected, resolved.formats[0], resolved.output)
        return wrong
    wanted = expected.output if expected.format is None else _text(expected)
    if resolved.output != wanted:
        return Failed("The output differs.", wanted, resolved.output)
    return None


_COMPARED: Final = ("origin", "id", "limit")


def _count(reports: Sequence[object]) -> str:
    return f"{len(reports)} {'report' if len(reports) == 1 else 'reports'}"


def _reports(expected: Sequence[Mapping[str, object]], actual: Sequence[object]) -> Failed | None:
    # A field is compared where the adapter's report carries it and the case
    # expects something of it: the specification prescribes a report no shape.
    if len(expected) != len(actual):
        return Failed(f"Expected {_count(expected)}, got {_count(actual)}.", expected, actual)
    for index, (wanted, report) in enumerate(zip(expected, actual, strict=True), start=1):
        if not isinstance(report, Mapping) or report.get("code") != wanted["code"]:
            return Failed(f"The code of report {index} differs.", wanted, report)
        for name in _COMPARED:
            if name in report and name in wanted and not same(report[name], wanted[name]):
                return Failed(f"The {name} of report {index} differs.", wanted, report)
    return None


def describe(error: BaseException) -> str:
    """What an adapter raised with, as text."""
    return str(error) or type(error).__name__


def _answer(adapter: Adapter, prepared: Prepared) -> Resolved | Failed:
    shown = prepared.expected.shown()
    try:
        answered = adapter.resolve(prepared.input)
    except Exception as error:
        # A raise is the outcome of that case alone.
        return Failed("The adapter raised.", shown, describe(error))
    if inspect.isawaitable(answered):
        close = getattr(answered, "close", None)
        if callable(close):
            close()
        return Failed(
            "The adapter answered asynchronously; resolve must answer at once.",
            shown,
            "an awaitable",
        )
    if not isinstance(answered, Resolved) or not isinstance(answered.output, str):
        return Failed("The adapter answered with no resolution.", shown, answered)
    return answered


def execute(adapter: Adapter, prepared: Prepared) -> Outcome:
    """The steps of RUNNER.md, in order: the answer's shape, what it produced, its reports."""
    resolved = _answer(adapter, prepared)
    if isinstance(resolved, Failed):
        return resolved
    produced = _produced(prepared.expected, resolved)
    if produced is not None:
        return produced
    if resolved.reports is None:
        return Passed(unobserved=True)
    if not isinstance(resolved.reports, list | tuple):
        return Failed("The reports are not a list.", prepared.expected.reports, resolved.reports)
    return _reports(prepared.expected.reports, resolved.reports) or Passed()


def execute_concrete(adapter: Adapter, case: Mapping[str, object]) -> Outcome:
    return execute(adapter, concrete(case, adapter))


def execute_generated(adapter: Adapter, case: Mapping[str, object]) -> Outcome:
    limits = cast("Mapping[str, int]", adapter.limits)
    return execute(adapter, _generated(cast("str", case["generate"]), limits))


def shown(value: object, at_most: int = 200) -> str:
    """A value as a failure line shows it, bounded: an output can be as long as the output limit."""
    try:
        text = json.dumps(value, ensure_ascii=False)
    except (TypeError, ValueError):
        text = repr(value)
    return text if len(text) <= at_most else text[:at_most] + "..."
