"""The contract between the conformance set and an implementation under test.

Everything here is spelled in the vocabulary of the specification: the inputs
of section 4, the levels of section 2, the limits of section 13, the report
codes of section 14.2. Nothing in it names ``curly_message``; an
implementation meets the set through an ``Adapter`` (section 14.3), which is
where its own calling convention is translated into this one.
"""

from __future__ import annotations

import enum
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass, field
from typing import Literal, Protocol

Level = Literal["core", "intl", "extensions"]

Verdict = Literal["error", "fail", "skip", "unobserved"]

Observed = Literal[Verdict, "none"]
"""What the runner answered for a defect: a verdict, or nothing at all."""

Caught = Literal["caught", "missed", "unreachable"]

LEVELS: tuple[Level, ...] = ("core", "intl", "extensions")

LIMITS: tuple[str, ...] = ("output", "read", "conversion", "nesting")

APIS: tuple[str, ...] = ("NumberFormat", "DateTimeFormat", "RelativeTimeFormat", "PluralRules")

UNITS: tuple[str, ...] = ("utf-8", "utf-16", "code-point")

CATEGORIES: tuple[str, ...] = ("zero", "one", "two", "few", "many", "other")


class Omit(enum.Enum):
    """A field a case gives no value for, which the adapter passes as nothing at all."""

    OMIT = "OMIT"

    def __repr__(self) -> str:
        return "OMIT"


OMIT = Omit.OMIT


class SetError(Exception):
    """The set itself cannot be read: the run ends, because nothing was measured."""


class ClaimError(Exception):
    """An adapter's claim about itself is outside the format's vocabulary: the run is refused."""


class CallOption(Protocol):
    """An option as a modifier receives it: its key, and its value read on demand."""

    @property
    def key(self) -> str: ...

    @property
    def value(self) -> str: ...


@dataclass(frozen=True)
class Call:
    """What a behaviour of the catalogue receives (sections 11, 11.3).

    The default is a call, because section 10 walks the chain only where a
    modifier asks for it.
    """

    value: str
    options: Sequence[CallOption]
    props: Mapping[str, object]
    locale: object
    default: Callable[[], str]


Behaviour = Callable[[Call], object]


@dataclass(frozen=True)
class Resolution:
    """One resolution's inputs, decoded into host values; ``OMIT`` where the case gives none."""

    message: object
    payload: object = OMIT
    props: object = OMIT
    locale: object = OMIT
    id: object = OMIT
    modifiers: Mapping[str, Behaviour] | None = None
    defaults: object = OMIT


@dataclass(frozen=True)
class Resolved:
    """What a resolution produced.

    ``reports`` left ``None`` says the adapter observes no reports, and
    ``formats`` left ``None`` says the same of the formatting requests
    (section 11.2): such a case is measured on its text instead.
    """

    output: object
    reports: Sequence[Mapping[str, object]] | None = None
    formats: Sequence[Mapping[str, object]] | None = None


@dataclass(frozen=True)
class Cst:
    """The tree an implementation offers (CST.md): the unit its spans count, and the call."""

    unit: str
    parse: Callable[[str], object]


@dataclass(frozen=True)
class Adapter:
    """The adapter of section 14.3.

    ``undefined`` is the value the implementation reads as the host's
    undefined. Where it states none, a tagged undefined is omitted: a member
    holding one is left out of its container.
    """

    levels: Sequence[str]
    limits: Mapping[str, object]
    resolve: Callable[[Resolution], object]
    unexpressible: Mapping[str, Sequence[str]] | None = None
    cst: Cst | None = None
    undefined: object = OMIT


@dataclass(frozen=True)
class Fixture:
    """A fixture file together with the name it is shipped under."""

    name: str
    file: Mapping[str, object]


@dataclass(frozen=True)
class Passed:
    """The case matched; ``unobserved`` where its report expectation went unchecked."""

    unobserved: bool = False


@dataclass(frozen=True)
class Failed:
    """The first mismatch, beside what was expected and what came back."""

    reason: str
    expected: object
    actual: object


Outcome = Passed | Failed


@dataclass(frozen=True)
class Identity:
    """What identifies a case: ``document`` names CST.md for a tree case, which has no level."""

    id: str
    file: str
    section: str
    description: str
    level: str | None = None
    document: str | None = None


@dataclass(frozen=True)
class Planned:
    """A case ready to run."""

    identity: Identity
    execute: Callable[[], Outcome] = field(repr=False, compare=False)


@dataclass(frozen=True)
class Skipped:
    """A case the plan left out, and why."""

    identity: Identity
    reason: str


@dataclass(frozen=True)
class Plan:
    cases: tuple[Planned, ...]
    skipped: tuple[Skipped, ...]


@dataclass(frozen=True)
class Result:
    """A plan executed, sorted. ``unobserved`` are the passed cases whose reports went unchecked."""

    passed: tuple[Planned, ...]
    failed: tuple[tuple[Planned, Failed], ...]
    skipped: tuple[Skipped, ...]
    unobserved: tuple[Planned, ...]


@dataclass(frozen=True)
class DefectEntry:
    """One defect of the catalogue, as ``defects.json`` writes it."""

    id: str
    description: str
    expects: Verdict
    section: str | None = None
    document: str | None = None


@dataclass(frozen=True)
class Mutation:
    """An adapter carrying one defect, and whether the defect reached the runner at all."""

    adapter: Adapter
    reached: Callable[[], bool] = field(repr=False, compare=False)


@dataclass(frozen=True)
class Audited:
    """What the audit made of one defect: what the runner answered, and whether that caught it."""

    entry: DefectEntry
    observed: Observed
    outcome: Caught
