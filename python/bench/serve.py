"""The Python side of ``bench/rows.mjs``: the rows measured inside the interpreter.

``rows.mjs`` starts this file under ``.bench-venv``'s interpreter with ``-I``,
so neither the source tree nor the environment's variables reach the import:
``curly_message`` is the wheel that venv installed. Given ``--list``, it
prints its rows by name and kind. Otherwise each line it reads asks for a row
(``{"row": name}``), which a count or a size answers with its value and a time
with readiness, or for one run of the time row asked for last
(``{"run": true}``). A time row runs a fixed batch per request, the size its
name states, so the round trip stays a small part of a sample. The process
ends when its input does.
"""

from __future__ import annotations

import json
import sys
from collections.abc import Callable
from datetime import UTC, datetime
from pathlib import Path
from types import FrameType
from typing import Any

import curly_message
from curly_message import _resolve, create_parser

MESSAGES = (
    "Hello, {{name}}!",
    "{{name}} has {{count:number}} new {{count:plural; one:message; other:messages}}.",
    "{{gender; male:He; female:She; default:They}} replied to {{thread}}.",
    "{{count:plural; one:{{count}} file; few:{{count}} files; other:{{count}} files}}"
    " in {{folder; :the root; default:{{folder}}}}.",
    "Total: {{amount:number}}, due {{due:date}}.",
)

PAYLOAD: dict[str, object] = {
    "name": "Ada",
    "count": 3,
    "gender": "female",
    "thread": "Plans",
    "folder": "docs",
    "amount": 1234.5,
    "due": datetime(2026, 1, 31, tzinfo=UTC),
}

ZONED_DATE = {
    "value": datetime(2026, 1, 31, tzinfo=UTC),
    "props": {"date": {"timeZone": "Europe/Dublin", "timeStyle": "full"}},
}
"""A winter date in a zone that keeps daylight time, named in full.

Its saving is zero in the database's rearguard form and negative in its main
form, so either form a system database holds reads the zone around it to
name it.
"""

parser = create_parser()
wrapped = create_parser(custom_modifiers={"test": lambda given: given.value})


def nested(depth: int) -> str:
    return "{{v; a:" * depth + "x" + "}}" * depth


def flat(count: int) -> str:
    return " ".join(f"{{{{p{index}}}}}" for index in range(count))


class Counted:
    """A value converted by its ``__str__``, which counts its calls."""

    calls = 0

    def __init__(self, text: str) -> None:
        self.text = text

    def __str__(self) -> str:
        Counted.calls += 1
        return self.text


def conversions_of_the_catalogue() -> int:
    Counted.calls = 0
    payload = {key: Counted(str(value)) for key, value in PAYLOAD.items()}
    for message in MESSAGES:
        parser.resolve(message, payload, locale="en")
    return Counted.calls


def conversions_of_one_value() -> int:
    Counted.calls = 0
    parser.resolve(" ".join(["{{v}}"] * 100), {"v": Counted("a")}, locale="en")
    return Counted.calls


def tails_of_the_catalogue() -> int:
    tail = _resolve._Walk._tail
    started = 0

    def spy(*args: Any) -> Any:
        nonlocal started
        started += 1
        return tail(*args)

    setattr(_resolve._Walk, "_tail", spy)  # noqa: B010
    try:
        for message in MESSAGES:
            parser.resolve(message, PAYLOAD, locale="en")
    finally:
        setattr(_resolve._Walk, "_tail", tail)  # noqa: B010
    return started


def trees_of_the_catalogue() -> int:
    text = _resolve._Text
    built = 0

    class Counted(_resolve._Text):
        __slots__ = ()

        def __init__(self, pieces: list[str | _resolve._Text], length: int) -> None:
            nonlocal built
            built += 1
            super().__init__(pieces, length)

    setattr(_resolve, "_Text", Counted)  # noqa: B010
    try:
        for message in MESSAGES:
            parser.resolve(message, PAYLOAD, locale="en")
    finally:
        setattr(_resolve, "_Text", text)  # noqa: B010
    return built


def deep_text() -> str:
    return "{{a; default:" * 10000 + "x" * 100000 + "}}" * 10000


def joins_of_deep_text() -> int:
    joins = 0

    def profile(frame: FrameType, event: str, arg: Any) -> None:
        nonlocal joins
        if (
            event == "c_call"
            and getattr(arg, "__name__", None) == "join"
            and isinstance(getattr(arg, "__self__", None), str)
        ):
            joins += 1

    message = deep_text()
    sys.setprofile(profile)
    try:
        parser.resolve(message, {}, locale="en")
    finally:
        sys.setprofile(None)
    return joins


def installed_bytes() -> int:
    package = Path(curly_message.__file__).parent
    return sum(
        path.stat().st_size
        for path in package.rglob("*")
        if path.is_file() and "__pycache__" not in path.parts
    )


def catalogue() -> object:
    for message in MESSAGES:
        parser.resolve(message, PAYLOAD, locale="en")
    return None


def trees() -> object:
    for message in MESSAGES:
        curly_message.cst(message)
    return None


def resolving(text: str, payload: dict[str, object], locale: str = "en") -> Callable[[], object]:
    return lambda: parser.resolve(text, payload, locale=locale)


def describing(text: str) -> Callable[[], object]:
    return lambda: curly_message.cst(text)


def wrapper_props() -> Callable[[], object]:
    props = {f"p{index}": index for index in range(5000)}
    payload = {"v": {"value": "a", "props": {"test": props}}}
    return lambda: wrapped.resolve("{{v:test}}", payload, props={"test": {"q": 1}}, locale="en")


VALUES: dict[str, tuple[str, Callable[[], int]]] = {
    "installed curly_message/": ("size", installed_bytes),
    "host conversions: resolving five catalogue messages": (
        "count",
        conversions_of_the_catalogue,
    ),
    "host conversions: one value named 100 times": ("count", conversions_of_one_value),
    "tails started: resolving five catalogue messages": ("count", tails_of_the_catalogue),
    "text trees built: resolving five catalogue messages": ("count", trees_of_the_catalogue),
    "str.join calls: 100 000 characters under 10 000 inline defaults": (
        "count",
        joins_of_deep_text,
    ),
}

# Each time row: the runs of its batch, and what builds the one run it times.
# The builder runs while the row is set up, so a message is built outside the
# samples.
TIMES: dict[str, tuple[int, Callable[[], Callable[[], object]]]] = {
    "resolve: five catalogue messages": (20, lambda: catalogue),
    "resolve: a flat message": (200, lambda: resolving("Hello, {{name}}!", {"name": "Ada"})),
    "resolve: a selection": (100, lambda: resolving(MESSAGES[2], PAYLOAD)),
    "resolve: selections holding placeholders": (
        100,
        lambda: resolving(
            "{{count; 1:{{count}} file; other:{{count}} files}}"
            " in {{folder; :the root; default:{{folder}}}}.",
            PAYLOAD,
        ),
    ),
    "resolve: a list of 1 000 distinct leaves": (
        5,
        lambda: resolving("{{items}}", {"items": [f"item {index}" for index in range(1000)]}),
    ),
    "resolve: 5 000 placeholders": (1, lambda: resolving(flat(5000), {})),
    "resolve: nested 10 000 levels": (1, lambda: resolving(nested(10000), {"v": "a"})),
    "resolve: inline defaults nested 10 000 levels": (
        1,
        lambda: resolving("{{a; default:" * 10000 + "x" + "}}" * 10000, {}),
    ),
    "resolve: 100 000 characters under 10 000 inline defaults": (
        1,
        lambda: resolving(deep_text(), {}),
    ),
    "resolve: a placeholder of 10 000 blanks": (
        1,
        lambda: resolving("{{" + " " * 10000 + "}}", {}),
    ),
    "resolve: a key with 10 000 blanks inside": (
        1,
        lambda: resolving("{{a" + " " * 10000 + "b}}", {}),
    ),
    "resolve: 1 000 placeholders that never close": (
        5,
        lambda: resolving("{{a; c:" * 1000, {}),
    ),
    "resolve: 5 000 options": (1, lambda: resolving("{{a" + "; a" * 5000 + "}}", {"a": "a"})),
    "resolve: an option key of 50 000 characters": (
        1,
        lambda: resolving("{{a; " + "x" * 50000 + ":v}}", {"a": "a"}),
    ),
    "resolve: wrapper props of 5 000 entries": (1, wrapper_props),
    "resolve: a number": (50, lambda: resolving("{{n:number}}", {"n": 1234.5})),
    "resolve: a plural": (
        50,
        lambda: resolving("{{n:plural; one:file; other:files}}", {"n": 1.5}),
    ),
    "resolve: an ago at a tag with extensions": (
        50,
        lambda: resolving("{{n:ago}}", {"n": -259200000}, "ja-JP-u-ca-japanese-hc-h23"),
    ),
    "resolve: a date": (
        50,
        lambda: resolving("{{d:date}}", {"d": datetime(2026, 1, 31, tzinfo=UTC)}),
    ),
    "resolve: a date named in its zone": (
        50,
        lambda: resolving("{{d:date}}", {"d": ZONED_DATE}),
    ),
    "cst: five catalogue messages": (20, lambda: trees),
    "cst: 5 000 placeholders": (1, lambda: describing(flat(5000))),
    "cst: nested 250 levels": (1, lambda: describing(nested(250))),
}


def named(name: str, times: int) -> str:
    """A time row's name states the runs of its batch, since a sample times the batch."""
    return name if times == 1 else f"{name} \N{MULTIPLICATION SIGN}{times}"


TIMED = {named(name, times): (times, build) for name, (times, build) in TIMES.items()}


def listing() -> list[dict[str, str]]:
    return [
        *({"name": name, "kind": kind} for name, (kind, _) in VALUES.items()),
        *({"name": name, "kind": "time"} for name in TIMED),
    ]


def batch(times: int, once: Callable[[], object]) -> Callable[[], None]:
    def run() -> None:
        for _ in range(times):
            once()

    return run


def answer(request: object, prepared: list[Callable[[], None]]) -> dict[str, object]:
    if not isinstance(request, dict):
        raise TypeError(f"a request is an object, not {request!r}")
    if request.get("run"):
        if not prepared:
            raise RuntimeError("no time row is set up")
        prepared[0]()
        return {}
    name = request.get("row")
    if isinstance(name, str) and name in VALUES:
        return {"value": VALUES[name][1]()}
    if isinstance(name, str) and name in TIMED:
        times, build = TIMED[name]
        prepared[:] = [batch(times, build())]
        return {"ready": True}
    raise KeyError(f"no row is named {name!r}")


def main() -> None:
    if sys.argv[1:] == ["--list"]:
        sys.stdout.write(json.dumps(listing()) + "\n")
        return
    prepared: list[Callable[[], None]] = []
    for line in sys.stdin:
        try:
            reply = answer(json.loads(line), prepared)
        except Exception as error:
            reply = {"error": f"{type(error).__name__}: {error}"}
        sys.stdout.write(json.dumps(reply) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
