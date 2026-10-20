"""The modifiers the format defines (section 11)."""

from __future__ import annotations

from collections.abc import Callable
from types import MappingProxyType
from typing import Final

from ._convert import to_number
from ._types import Level, ModifierInput, Option, ReportCode

Builtin = Callable[[ModifierInput], str]


class ModifierFailure(Exception):  # noqa: N818
    """Raised by a built-in modifier that cannot answer, naming the report it makes."""

    def __init__(self, code: ReportCode) -> None:
        super().__init__(code)
        self.code: ReportCode = code


def _selected(option: Option | None, config: ModifierInput) -> str:
    # A selection that matched answers with its own value, empty or not; only
    # one that matched nothing reads the chain.
    return option.value if option is not None else config.default


def eq(config: ModifierInput) -> str:
    # The value is lowered once, ahead of the options, so the cost adds the two
    # rather than multiplying them.
    needle = config.value.lower()
    return _selected(next((o for o in config.options if o.key.lower() == needle), None), config)


def ne(config: ModifierInput) -> str:
    needle = config.value.lower()
    return _selected(next((o for o in config.options if o.key.lower() != needle), None), config)


def _ordered(options: tuple[Option, ...], descending: bool) -> list[tuple[float, Option]]:
    # Only a key that is a number can be ordered; one that is not is never
    # selected. The sort is stable in either direction, so a tie keeps source
    # order, and it orders a copy: the list is the caller's.
    keyed = [
        (number, option) for option in options if (number := to_number(option.key)) is not None
    ]
    keyed.sort(key=lambda pair: pair[0], reverse=descending)
    return keyed


def lt(config: ModifierInput) -> str:
    value = to_number(config.value)
    if value is not None:
        for key, option in _ordered(config.options, descending=False):
            if value < key:
                return option.value
    return config.default


def gt(config: ModifierInput) -> str:
    value = to_number(config.value)
    if value is not None:
        for key, option in _ordered(config.options, descending=True):
            if value > key:
                return option.value
    return config.default


def _equal_first(config: ModifierInput, strict: Builtin) -> str:
    # The equality leg answers first, and the strict leg is its default, so it
    # is read only where equality selects nothing.
    return eq(
        ModifierInput(
            value=config.value,
            options=config.options,
            props=config.props,
            locale=config.locale,
            default=lambda: strict(config),
        )
    )


def lte(config: ModifierInput) -> str:
    return _equal_first(config, lt)


def gte(config: ModifierInput) -> str:
    return _equal_first(config, gt)


BUILTINS: Final[MappingProxyType[str, Builtin]] = MappingProxyType(
    {"eq": eq, "ne": ne, "lt": lt, "gt": gt, "lte": lte, "gte": gte}
)
"""The modifiers this installation defines, by name."""

SELECTIONS: Final = frozenset(("eq", "ne", "lt", "gt", "lte", "gte", "plural", "ordinal"))
"""The names the format defines as selections, which a placeholder must give options."""


def levels() -> frozenset[Level]:
    """The conformance levels this installation satisfies (section 2)."""
    return frozenset(("core", "extensions"))
