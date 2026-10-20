"""ECMA-402's reading of a request's options.

A request is written in ECMA-402's vocabulary, so ECMA-402 decides which
requests are well formed: a request its constructor refuses is refused here,
before the supplier is asked, however lenient the supplier would be. Each
option is read the way ECMA-402 reads it, through JavaScript's own
conversions, so ``"2"`` and ``2`` name the same number of digits as they do
for the facility the request is measured against.
"""

from __future__ import annotations

import math
from collections.abc import Collection, Mapping
from decimal import Decimal
from typing import Final, NoReturn

from .._modifiers import ModifierFailure
from .._types import UNDEFINED
from .._utils import MISSING, WHITESPACE_CHARS

Options = Mapping[str, object]


def refuse() -> NoReturn:
    """A request ECMA-402 refuses, which the placeholder answers with its fallback chain."""
    raise ModifierFailure("failed-modifier")


def given(options: Options, name: str) -> object:
    """The value a request names under ``name``, or ``MISSING`` where it names none."""
    value = options.get(name, MISSING)
    return MISSING if value is UNDEFINED else value


def _decimal_digits(text: str, start: int) -> int:
    end = start
    while end < len(text) and "0" <= text[end] <= "9":
        end += 1
    return end


def _unsigned_decimal(text: str) -> bool:
    # StrUnsignedDecimalLiteral: digits with an optional fraction, or a
    # fraction alone, then an optional exponent; no separators.
    integer = _decimal_digits(text, 0)
    end = integer
    fraction = 0
    if end < len(text) and text[end] == ".":
        fraction_end = _decimal_digits(text, end + 1)
        fraction = fraction_end - end - 1
        end = fraction_end
    if integer == 0 and fraction == 0:
        return False
    if end < len(text) and text[end] in "eE":
        end += 1
        if end < len(text) and text[end] in "+-":
            end += 1
        exponent = _decimal_digits(text, end)
        if exponent == end:
            return False
        end = exponent
    return end == len(text)


_RADIXES: Final = {"x": (16, "0123456789abcdefABCDEF"), "o": (8, "01234567"), "b": (2, "01")}


def _string_to_number(value: str) -> float:
    # StringToNumber, written out: Python's float() reads underscores, "nan"
    # and "inf", none of which JavaScript reads, and reads no radix prefix.
    text = value.strip(WHITESPACE_CHARS)
    if not text:
        return 0.0
    if len(text) > 2 and text[0] == "0" and text[1].lower() in _RADIXES:
        radix, alphabet = _RADIXES[text[1].lower()]
        digits = text[2:]
        if all(character in alphabet for character in digits):
            return _float(int(digits, radix))
        return math.nan
    sign = 1.0
    body = text
    if body[0] in "+-":
        sign = -1.0 if body[0] == "-" else 1.0
        body = body[1:]
    if body == "Infinity":
        return sign * math.inf
    if not _unsigned_decimal(body):
        return math.nan
    return sign * float(body)


def _float(value: int) -> float:
    try:
        return float(value)
    except OverflowError:
        return math.inf if value > 0 else -math.inf


def to_number(value: object) -> float:
    """ECMAScript's ``ToNumber`` over the host values a layer can hold."""
    if value is None:
        return 0.0
    if value is True:
        return 1.0
    if value is False:
        return 0.0
    if issubclass(type(value), str):
        return _string_to_number(str.__str__(value))
    if issubclass(type(value), float):
        return float.__float__(value)  # type: ignore[arg-type]
    if issubclass(type(value), int):
        return _float(int.__int__(value))  # type: ignore[arg-type]
    return math.nan


def number_to_string(value: float) -> str:
    """ECMAScript's ``Number::toString``: the shortest digits, in its notation."""
    if math.isnan(value):
        return "NaN"
    if math.isinf(value):
        return "Infinity" if value > 0 else "-Infinity"
    if value == 0:
        return "0"
    # The digits are read off the shortest spelling without arithmetic, which
    # would round in whatever decimal context the host has set.
    shortest = Decimal(repr(abs(value))).as_tuple()
    written = "".join(map(str, shortest.digits))
    digits = written.rstrip("0")
    k = len(digits)
    # Where the decimal point falls, counted in digits from the first.
    n = len(written) + int(shortest.exponent)
    sign = "-" if value < 0 else ""
    if k <= n <= 21:
        return sign + digits + "0" * (n - k)
    if 0 < n <= 21:
        return sign + digits[:n] + "." + digits[n:]
    if -6 < n <= 0:
        return sign + "0." + "0" * -n + digits
    exponent = ("+" if n >= 1 else "-") + str(abs(n - 1))
    if k == 1:
        return sign + digits + "e" + exponent
    return sign + digits[0] + "." + digits[1:] + "e" + exponent


def to_string(value: object) -> str:
    """ECMAScript's ``ToString`` over the scalars a layer can hold; anything else is refused."""
    if issubclass(type(value), str):
        return str.__str__(value)
    if value is True:
        return "true"
    if value is False:
        return "false"
    if value is None:
        return "null"
    if issubclass(type(value), (int, float)):
        return number_to_string(to_number(value))
    refuse()


def to_boolean(value: object) -> bool:
    """ECMAScript's ``ToBoolean``: every object is true, whatever it holds."""
    if value is None or value is False:
        return False
    if value is True:
        return True
    if issubclass(type(value), str):
        return bool(str.__len__(value))  # type: ignore[arg-type]
    if issubclass(type(value), (int, float)):
        number = to_number(value)
        return not (number == 0 or math.isnan(number))
    return True


def get_string(
    options: Options, name: str, values: Collection[str] | None, fallback: str | None
) -> str | None:
    """``GetOption`` of a string: converted, then held to its list where it has one."""
    value = given(options, name)
    if value is MISSING:
        return fallback
    text = to_string(value)
    if values is not None and text not in values:
        refuse()
    return text


def get_boolean(options: Options, name: str) -> bool | None:
    """``GetOption`` of a boolean."""
    value = given(options, name)
    return None if value is MISSING else to_boolean(value)


def default_number(
    value: object, minimum: float, maximum: float, fallback: int | None
) -> int | None:
    """``DefaultNumberOption``: a number within the range, floored, or refused."""
    if value is MISSING:
        return fallback
    number = to_number(value)
    if math.isnan(number) or number < minimum or number > maximum:
        refuse()
    return math.floor(number)


def get_number(options: Options, name: str, minimum: float, maximum: float, fallback: int) -> int:
    """``GetNumberOption`` with a fallback, so it answers a number."""
    found = default_number(given(options, name), minimum, maximum, fallback)
    return fallback if found is None else found


def _alphanumeric(text: str) -> bool:
    return bool(text) and all(
        "a" <= character <= "z" or "A" <= character <= "Z" or "0" <= character <= "9"
        for character in text
    )


def is_type_sequence(text: str) -> bool:
    """Unicode's ``type`` production, which a ``numberingSystem`` or a ``calendar`` must match."""
    return all(3 <= len(part) <= 8 and _alphanumeric(part) for part in text.split("-"))


def get_type_sequence(options: Options, name: str) -> str | None:
    """A ``numberingSystem`` or ``calendar`` option, refused where it is not well formed."""
    text = get_string(options, name, None, None)
    if text is not None and not is_type_sequence(text):
        refuse()
    return text


def _alpha(text: str) -> bool:
    return bool(text) and all("a" <= c <= "z" or "A" <= c <= "Z" for c in text)


def is_currency_code(text: str) -> bool:
    """``IsWellFormedCurrencyCode``: three ASCII letters, in either case."""
    return len(text) == 3 and _alpha(text)


LOCALE_MATCHERS: Final = ("lookup", "best fit")
