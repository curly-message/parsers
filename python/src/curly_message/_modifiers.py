"""The modifiers the format defines (section 11)."""

from __future__ import annotations

import importlib.util
from collections.abc import Callable
from types import MappingProxyType, ModuleType
from typing import Final

from ._convert import to_number
from ._types import Level, ModifierInput, Option, ReportCode
from ._utils import UNFINISHED

Builtin = Callable[[ModifierInput], str]


class ModifierFailure(Exception):  # noqa: N818
    """Raised by a built-in modifier that cannot answer, naming the report it makes."""

    def __init__(self, code: ReportCode) -> None:
        super().__init__(code)
        self.code: ReportCode = code


# A selection reads the option it selected, or the default, in its own frame:
# each read in progress stands on the interpreter's stack, and a helper that
# read for it would stand there too (``MAX_REENTRY``).


def _equal(config: ModifierInput) -> Option | None:
    # The value is lowered once, ahead of the options, so the cost adds the two
    # rather than multiplying them.
    needle = config.value.lower()
    return next((o for o in config.options if o.key.lower() == needle), None)


def eq(config: ModifierInput) -> str:
    # A selection that matched answers with its own value, empty or not; only
    # one that matched nothing reads the chain.
    option = _equal(config)
    return option.value if option is not None else config.default


def ne(config: ModifierInput) -> str:
    needle = config.value.lower()
    option = next((o for o in config.options if o.key.lower() != needle), None)
    return option.value if option is not None else config.default


def _ordered(config: ModifierInput, descending: bool) -> Option | None:
    # Only a key that is a number can be ordered; one that is not is never
    # selected. The sort is stable in either direction, so a tie keeps source
    # order, and it orders a copy: the tuple is the caller's.
    value = to_number(config.value)
    if value is None:
        return None
    keyed = [
        (number, option)
        for option in config.options
        if (number := to_number(option.key)) is not None
    ]
    keyed.sort(key=lambda pair: pair[0], reverse=descending)
    for key, option in keyed:
        if value > key if descending else value < key:
            return option
    return None


def lt(config: ModifierInput) -> str:
    option = _ordered(config, descending=False)
    return option.value if option is not None else config.default


def gt(config: ModifierInput) -> str:
    option = _ordered(config, descending=True)
    return option.value if option is not None else config.default


def lte(config: ModifierInput) -> str:
    # The equality leg answers first, and the strict leg is read only where
    # equality selects nothing.
    option = _equal(config) or _ordered(config, descending=False)
    return option.value if option is not None else config.default


def gte(config: ModifierInput) -> str:
    option = _equal(config) or _ordered(config, descending=True)
    return option.value if option is not None else config.default


CORE: Final[MappingProxyType[str, Builtin]] = MappingProxyType(
    {"eq": eq, "ne": ne, "lt": lt, "gt": gt, "lte": lte, "gte": gte}
)
"""The comparisons, which every installation defines (section 11.1)."""

SELECTIONS: Final = frozenset(("eq", "ne", "lt", "gt", "lte", "gte", "plural", "ordinal"))
"""The names the format defines as selections, which a placeholder must give options."""

SUPPLIER_FLOOR: Final = (2, 14)
"""The oldest Babel release the Intl modifiers are tested against."""


def _release(version: str) -> tuple[int, ...]:
    # The leading integers of a release, so a pre-release or a local suffix
    # compares as the release it precedes rather than failing to parse.
    numbers: list[int] = []
    for part in version.split("."):
        digits = ""
        for character in part:
            if not character.isdigit():
                break
            digits += character
        if not digits:
            break
        numbers.append(int(digits))
        if len(digits) != len(part):
            break
    return tuple(numbers)


_SUPPLIED: Final[list[bool]] = []
"""Whether the supplier is installed, once the question was answered."""

_LOADED: Final[list[ModuleType | None]] = []
"""The Intl modifiers, or ``None`` for a supplier they cannot import, once known."""


def supplier() -> bool:
    """Whether the Intl supplier is installed, asked without importing it.

    Babel turns the Intl modifiers on wherever it is installed, at the floor or
    above, whoever installed it. Importing it costs more than everything else
    the package loads, so the answer is read off the import system and the
    distribution's metadata, once per process.
    """
    if _SUPPLIED:
        return _SUPPLIED[0]
    try:
        installed = importlib.util.find_spec("babel") is not None
        if installed:
            # Imported here: it costs more to import than the rest of the package.
            from importlib.metadata import PackageNotFoundError, version  # noqa: PLC0415

            try:
                installed = _release(version("babel")) >= SUPPLIER_FLOOR
            except PackageNotFoundError:
                installed = False
    except UNFINISHED:
        # Asking failed rather than answered, on a stack or a memory the
        # caller had all but spent: this call goes without the supplier, and
        # the next one asks again.
        return False
    except Exception:
        # Metadata that cannot be read answers as surely as metadata that
        # says no, and asking again would read it again on every call.
        installed = False
    _SUPPLIED.append(installed)
    return installed


def _intl() -> ModuleType | None:
    # The first Intl call imports the modifiers and the supplier with them. A
    # supplier that cannot be imported, lacking what they import or broken
    # in itself, is not imported again on each call; an import that failed
    # to finish is tried again by the next call.
    if _LOADED:
        return _LOADED[0]
    try:
        module = importlib.import_module("curly_message._intl")
    except UNFINISHED:
        return None
    except Exception:
        _LOADED.append(None)
        return None
    _LOADED.append(module)
    return module


def _deferred(name: str) -> Builtin:
    def modifier(config: ModifierInput) -> str:
        module = _intl()
        if module is None:
            raise ModifierFailure("failed-modifier")
        answer: str = getattr(module, name)(config)
        return answer

    modifier.__name__ = modifier.__qualname__ = name
    return modifier


INTL: Final[MappingProxyType[str, Builtin]] = MappingProxyType(
    {name: _deferred(name) for name in ("number", "currency", "date", "ago", "plural", "ordinal")}
)
"""The formatting modifiers and the plural selections, defined where the supplier is installed."""


def _intl_satisfied() -> bool:
    # Until a call finds the supplier cannot be imported, an installed one is
    # taken at its word.
    return supplier() and not (_LOADED and _LOADED[0] is None)


def builtin(name: str) -> Builtin | None:
    """The modifier this installation defines under a name, or ``None``.

    The Intl modifiers are defined only while the installation satisfies Intl,
    so where it does not their names are unknown (section 2).
    """
    found = CORE.get(name)
    if found is not None:
        return found
    found = INTL.get(name)
    if found is None or not supplier():
        return None
    if not _LOADED:
        return found
    # Once imported, the modifier answers for itself: the import's wrapper
    # would stand a frame under every read it makes (``MAX_REENTRY``).
    module = _LOADED[0]
    return None if module is None else getattr(module, name)


def levels() -> frozenset[Level]:
    """The conformance levels this installation satisfies (section 2).

    ``"intl"`` is among them where the supplier is installed: Babel, at
    release 2.14 or later, which ``curly-message[intl]`` installs. It leaves
    once an Intl call has found a supplier the modifiers cannot import.
    """
    if _intl_satisfied():
        return frozenset(("core", "intl", "extensions"))
    return frozenset(("core", "extensions"))
