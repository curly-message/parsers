"""Written-out and generated cases: their inputs built, their expectations computed, compared."""

from __future__ import annotations

import inspect
import json
import math
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from typing import Final, cast

from .behaviours import DESCEND, RENESTED, behaviours, descending, keeping, renesting
from .decode import decode, unserializable
from .model import (
    CATEGORIES,
    OMIT,
    Adapter,
    Behaviour,
    Failed,
    Outcome,
    Passed,
    Resolution,
    Resolved,
    SetError,
    Unexpressible,
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


def _register(
    modifiers: Mapping[str, str] | None, keepers: Mapping[str, str] | None, adapter: Adapter
) -> Mapping[str, Behaviour] | None:
    # The behaviours over a list of what was kept share one for the resolution
    # they are registered for, so each registration builds them afresh, and a
    # ``nest`` among them asks the adapter the case is run against, so that in
    # an audit it asks the adapter the defect was applied to.
    if modifiers is None and keepers is None:
        return None
    table = behaviours(adapter.undefined)
    kept = keeping(adapter)
    return {
        **{name: table[kind] for name, kind in (modifiers or {}).items()},
        **{name: kept[kind] for name, kind in (keepers or {}).items()},
    }


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
    named = cast("Mapping[str, str] | None", case.get("modifiers"))
    keepers = cast("Mapping[str, str] | None", case.get("keeping"))
    modifiers = _register(named, keepers, adapter)
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

_CONVERTS: Final = "{{v:ne; nomatch:ok; default:refused;}}"
"""A value converted whole and compared against a key its text cannot equal: the
option is chosen only where the value was converted, so a conversion abandoned
shows as the default, and none of the value's text reaches the output."""


def _visiting(count: int, undefined: object) -> dict[str, object]:
    # A value whose serialization visits ``count`` nodes and writes little text.
    # Section 13 counts a member the walk visits and then omits, and a value
    # reached twice twice: the root holds ``side`` references to one object of
    # ``side - 1`` members holding the host's undefined, each reference
    # visiting ``side`` nodes, and enough members holding undefined itself to
    # make up the rest.
    side = math.isqrt(count - 1)
    shared = {f"k{index}": undefined for index in range(side - 1)}
    return {
        **{f"s{index}": shared for index in range(side)},
        **{f"u{index}": undefined for index in range(count - 1 - side * side)},
    }


def _nest(levels: int, innermost: str = "{{v; a:settled; default:fallback;}}") -> str:
    # Each level selects the option holding the next; the innermost declares
    # a fallback, so a level the nesting limit refuses shows as its chain.
    return "{{v; a:" * (levels - 1) + innermost + ";}}" * (levels - 1)


CHAIN: Final = 20000
"""How many levels past the nesting limit the chain construction nests."""


def _chain(levels: int) -> str:
    # Each level the inline default of the one around it, built without
    # recursion: the payload names none of the keys, so every level takes its
    # chain down to the next, and the innermost writes the text.
    return "{{v; default:" * levels + "x" + "}}" * levels


def _limit(code: str, origin: str, declared: int) -> Mapping[str, object]:
    return {"code": code, "origin": origin, "id": ID, "limit": declared}


COUNTED: Final = (
    "reentry-at-limit",
    "reentry-over-limit",
    "reentry-refused-not-recorded",
    "reentry-own-reads-free",
    "reentry-over-limit-by-a-conversion",
    "reentry-own-read-in-a-nested-resolution",
    "reentry-over-limit-beneath-a-defined-modifier",
)
"""The constructions of kept reads, built from the bound the adapter states on counted reads."""

FRAMED: Final = (
    "reentry-own-read-in-a-nested-resolution",
    "reentry-over-limit-beneath-a-defined-modifier",
)
"""Those of them that make their read in a resolution a modifier asks for with the bound
reached, built from resolutions one inside another where the adapter states that its
bound counts them, which keeps no input past the call it was handed to."""

RESOLUTIONS: Final = "reentry-resolutions-over-limit"
"""The construction built from that bound where the adapter states that it counts a
resolution asked for inside another, which keeps no input."""


def renested(bound: int) -> str:
    """That construction's output: ``bound + 1`` resolutions stand, each writing its brackets."""
    return "[" * (bound + 1) + "F" + "]" * (bound + 1)


UNDEFINED_COUNTED: Final = ("conversion-at-limit", "conversion-over-limit")
"""The constructions that count their nodes in members holding the host's undefined."""

_TAKE: Final = "{{x:take; default:F}}"

_TAKING: Final = "{{x:taking; default:F}}"

_KEEP_NEST: Final = "{{x:keep-nest; default:z}}"
"""A placeholder whose modifier keeps its default unread and asks for a resolution in
which the conversion of an answer reads it."""

_KEEP_NEST_NE: Final = "<{{x:keep-nest-ne; default:{{x:keep-nest-ne; default:z}}}}>"
"""One whose modifier does the same, of a resolution in which that conversion runs while
the format's own ``ne`` produces the option it selected, between brackets. Where the
bound counts the resolution, the read of the first default is one past it; where it
counts reads alone, that read takes the last place and produces the second
placeholder, whose default is then read one past the bound. Either read is refused,
and the text is the same."""


def _keeps(count: int, body: str = _TAKE) -> str:
    # Placeholders that each keep their default unread, the default taking the
    # input kept before it, so the reads a ``take`` after them makes stand one
    # inside another as deep as the placeholders kept.
    return ("{{x:keep; default:" + body + "}}") * count


def _refusing(bound: int, reader: str = _TAKE) -> str:
    # One input more than the bound lets be read at once. The read past it is
    # made by the placeholder between brackets in the second input's default,
    # or, where the bound is 0, by the one after them.
    if not bound:
        return _keeps(1) + reader
    return _keeps(1) + _keeps(1, "[" + reader + "]") + _keeps(bound - 1) + _TAKE


def _own(levels: int) -> str:
    # Levels of own reads: each placeholder's modifier reads its own default,
    # which holds the next, and the innermost answers ``z``.
    return "{{x:read; default:" * levels + "z" + "}}" * levels


def _generated(name: str, adapter: Adapter) -> Prepared:
    limits = cast("Mapping[str, int]", adapter.limits)
    output, read = limits["output"], limits["read"]
    conversion, nesting = limits["conversion"], limits["nesting"]
    bound = adapter.reentry or 0

    def case(
        message: str, payload: dict[str, object], text: str, *reports: Mapping[str, object]
    ) -> Prepared:
        return Prepared(
            Resolution(message=message, payload=payload, id=ID),
            Expectation(output=text, reports=reports),
        )

    def reentering(message: str, text: str, *more: str) -> Prepared:
        keepers = {name: name for name in ("keep", "take", "first", *more)}
        return Prepared(
            Resolution(
                message=message,
                payload={"x": ""},
                id=ID,
                modifiers=_register({"read": "default"}, keepers, adapter),
            ),
            Expectation(output=text, reports=()),
        )

    def standing(inner: str, keeper: str, text: str) -> Prepared:
        # ``inner`` resolved with one place fewer taken than the bound lets
        # stand, so that the resolution its modifier asks for takes the last
        # where the bound counts resolutions. Where the adapter states that it
        # does, the places are resolutions one inside another, the innermost
        # of ``inner``; otherwise they are counted reads, the innermost
        # producing ``inner``, and the bound may count the resolution or not.
        if adapter.resolutions is True:
            return Prepared(
                Resolution(
                    message=DESCEND if bound > 1 else inner,
                    payload={"x": ""},
                    id=ID,
                    modifiers=descending(adapter, bound - 1, inner),
                ),
                Expectation(output=text, reports=()),
            )
        return reentering(
            _keeps(1, inner) + _keeps(bound - 2) + _TAKE if bound > 1 else inner,
            text,
            "taking",
            keeper,
        )

    def visiting(count: int) -> dict[str, object]:
        if adapter.undefined is OMIT:
            raise SetError(f"The construction {name} needs a stand-in for undefined.")
        return _visiting(count, adapter.undefined)

    unserializable_value = {"code": "unserializable-value", "origin": "payload", "id": ID}
    failed_modifier = {"code": "failed-modifier", "origin": "payload", "id": ID}
    nesting_limit = _limit("nesting-limit", "message", nesting)
    read_limit = _limit("read-limit", "limit", read)
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
            _READS + "{{w}}", {"v": "x" * (read + 1), "w": "tail"}, "ok", read_limit
        ),
        "conversion-at-limit": lambda: case(_CONVERTS, {"v": visiting(conversion)}, "ok"),
        "conversion-over-limit": lambda: case(
            "{{v; default:D}}", {"v": visiting(conversion + 1)}, "D", unserializable_value
        ),
        # One node past the limit, each element long enough that the text a
        # finished serialization would write runs past the read budget.
        "conversion-over-limit-continues": lambda: case(
            "{{v; default:D}}{{w}}",
            {"v": ["x" * (-(-read // conversion) + 1)] * conversion, "w": "tail"},
            "Dtail",
            unserializable_value,
        ),
        "nesting-at-limit": lambda: case(_nest(nesting), {"v": "a"}, "settled"),
        "nesting-over-limit": lambda: case(
            _nest(nesting + 1), {"v": "a"}, "fallback", nesting_limit
        ),
        "nesting-over-limit-converts": lambda: case(
            _nest(nesting + 1, "{{w; default:fallback;}}"),
            {"v": "a", "w": unserializable()},
            "fallback",
            unserializable_value,
            nesting_limit,
        ),
        "nesting-over-limit-reads": lambda: case(
            _nest(nesting + 1, "{{w; default:fallback;}}") + "{{v}}",
            {"v": "a", "w": "x" * (read + 1)},
            "fallback",
            nesting_limit,
            read_limit,
        ),
        "nesting-over-limit-read-spent": lambda: case(
            _nest(nesting + 1, "{{w:eq; nomatch:X;}}{{v; default:fallback;}}"),
            {"v": "a", "w": "x" * (read + 1)},
            "",
            nesting_limit,
            read_limit,
        ),
        "nesting-over-limit-unknown-modifier": lambda: case(
            _nest(nesting + 1, "{{w:zz; default:fallback;}}"),
            {"v": "a", "w": "x"},
            "fallback",
            nesting_limit,
        ),
        "nesting-over-limit-missing-options": lambda: case(
            _nest(nesting + 1, "{{w:eq; default:fallback;}}"),
            {"v": "a", "w": "x"},
            "fallback",
            nesting_limit,
        ),
        "nesting-chain-over-limit": lambda: case(
            _chain(nesting + CHAIN), {}, "x", *[nesting_limit] * CHAIN
        ),
        "reentry-at-limit": lambda: reentering(_keeps(bound) + _TAKE, "none"),
        "reentry-over-limit": lambda: reentering(_refusing(bound), "[empty]" if bound else "empty"),
        "reentry-refused-not-recorded": lambda: reentering(
            _refusing(bound) + "{{x:first; default:F}}", "[empty]none" if bound else "emptyempty"
        ),
        "reentry-own-reads-free": lambda: reentering(
            _keeps(1, _own(max(nesting - 1, 1))) + _keeps(bound - 1) + _TAKE, "z"
        ),
        "reentry-over-limit-by-a-conversion": lambda: reentering(
            _refusing(bound, _TAKING), "[empty]" if bound else "empty", "taking"
        ),
        # With the bound reached, a modifier keeps its default and asks for a
        # resolution, where the conversion of an answer reads that default:
        # the modifier is current there, so the read is its own.
        "reentry-own-read-in-a-nested-resolution": lambda: standing(_KEEP_NEST, "keep-nest", "z"),
        # The same, with that conversion run while a modifier the format
        # defines produces the option it selected: that modifier is current,
        # so the read is no modifier's own, is counted one past the bound and
        # is refused, and the conversion goes on with the empty string.
        "reentry-over-limit-beneath-a-defined-modifier": lambda: standing(
            _KEEP_NEST_NE, "keep-nest-ne", "<empty>"
        ),
        # One resolution more than the bound lets stand inside the one asked
        # for: it is refused, and the modifier that asked fails in the
        # innermost resolution standing, which reports there; only where the
        # bound is 0 is that the resolution the case asks for.
        RESOLUTIONS: lambda: Prepared(
            Resolution(
                message=RENESTED,
                payload={"x": ""},
                id=ID,
                modifiers={"renest": renesting(adapter, bound)},
            ),
            Expectation(
                output=renested(bound),
                reports=() if bound else (failed_modifier,),
            ),
        ),
    }
    if name not in builders:
        raise SetError(f"No construction is named {name!r}.")
    if (name in COUNTED or name == RESOLUTIONS) and adapter.reentry is None:
        raise SetError(f"The construction {name} is built from a bound the adapter does not state.")
    return builders[name]()


def covers(entry: Unexpressible, api: str, options: Mapping[str, object]) -> str | None:
    """The reason an entry of ``unexpressible`` leaves out a request, or None where it covers none.

    The property must be one the request reads, among the values the entry
    lists, in a request whose properties hold what its condition states; a
    value is compared as a request's are.
    """
    if isinstance(entry, str):
        prop, values, when = entry, None, None
    else:
        prop = cast("str", entry["property"])
        values = cast("Sequence[object] | None", entry.get("values"))
        when = cast("Mapping[str, object] | None", entry.get("when"))
    if prop not in options:
        return None
    if values and not any(same(options[prop], value) for value in values):
        return None
    if when and not all(
        name in options and same(options[name], held) for name, held in when.items()
    ):
        return None
    value = f"the value {_written(options[prop])} of " if values else ""
    condition = (
        " whose " + " and ".join(f"{name} is {_written(held)}" for name, held in when.items())
        if when
        else ""
    )
    return f"The adapter cannot express {value}the {prop} property of a {api} request{condition}."


def _written(value: object) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)


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
    return execute(adapter, _generated(cast("str", case["generate"]), adapter))


def shown(value: object, at_most: int = 200) -> str:
    """A value as a failure line shows it, bounded: an output can be as long as the output limit."""
    try:
        text = json.dumps(value, ensure_ascii=False)
    except (TypeError, ValueError):
        text = repr(value)
    return text if len(text) <= at_most else text[:at_most] + "..."
