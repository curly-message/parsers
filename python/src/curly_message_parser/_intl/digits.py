"""Which digits a number is shown with: ECMA-402's digit options and its rounding.

``SetNumberFormatDigitOptions`` decides the digits a request asks for, and
``FormatNumericToString`` rounds a number to them, here over ``Decimal`` so
that the digits are the decimal ones ECMA-402 rounds rather than a binary
approximation of them. The rounded number is the one both formatted and asked
for its plural category: its exponent carries the fraction digits shown.
"""

from __future__ import annotations

import decimal
from decimal import Decimal
from typing import Final

from .._utils import MISSING
from .validate import Options, default_number, get_number, get_string, given, refuse

PRECISION: Final = decimal.Context(
    prec=450,
    rounding=decimal.ROUND_HALF_EVEN,
    Emin=decimal.MIN_EMIN,
    Emax=decimal.MAX_EMAX,
    capitals=1,
    clamp=0,
    flags=[],
    traps=[decimal.InvalidOperation, decimal.DivisionByZero, decimal.Overflow],
)
"""Enough digits for every number a request rounds exactly.

The widest is the largest double scaled for a percentage and shown with a
hundred fraction digits, a little over 410 digits. Every field is spelled out:
a field left out is copied from ``decimal.DefaultContext``, which is the
host's to configure, and the digits a message shows are not.
"""

_INCREMENTS: Final = frozenset(
    (1, 2, 5, 10, 20, 25, 50, 100, 200, 250, 500, 1000, 2000, 2500, 5000)
)

ROUNDING_MODES: Final = (
    "ceil",
    "floor",
    "expand",
    "trunc",
    "halfCeil",
    "halfFloor",
    "halfExpand",
    "halfTrunc",
    "halfEven",
)

# ``GetUnsignedRoundingMode``: what each mode does to a magnitude, for a
# positive number and for a negative one.
_UNSIGNED: Final = {
    "ceil": (decimal.ROUND_UP, decimal.ROUND_DOWN),
    "floor": (decimal.ROUND_DOWN, decimal.ROUND_UP),
    "expand": (decimal.ROUND_UP, decimal.ROUND_UP),
    "trunc": (decimal.ROUND_DOWN, decimal.ROUND_DOWN),
    "halfCeil": (decimal.ROUND_HALF_UP, decimal.ROUND_HALF_DOWN),
    "halfFloor": (decimal.ROUND_HALF_DOWN, decimal.ROUND_HALF_UP),
    "halfExpand": (decimal.ROUND_HALF_UP, decimal.ROUND_HALF_UP),
    "halfTrunc": (decimal.ROUND_HALF_DOWN, decimal.ROUND_HALF_DOWN),
    "halfEven": (decimal.ROUND_HALF_EVEN, decimal.ROUND_HALF_EVEN),
}

_SIGNIFICANT: Final = "significantDigits"
_FRACTION: Final = "fractionDigits"
_MORE: Final = "morePrecision"
_LESS: Final = "lessPrecision"


class Digits:
    """The digit options a request resolves to."""

    __slots__ = (
        "increment",
        "maximum_fraction",
        "maximum_significant",
        "minimum_fraction",
        "minimum_integer",
        "minimum_significant",
        "mode",
        "rounding",
        "strip_if_integer",
    )

    def __init__(
        self,
        *,
        minimum_integer: int,
        minimum_fraction: int,
        maximum_fraction: int,
        minimum_significant: int,
        maximum_significant: int,
        increment: int,
        mode: str,
        rounding: str,
        strip_if_integer: bool,
    ) -> None:
        self.minimum_integer = minimum_integer
        self.minimum_fraction = minimum_fraction
        self.maximum_fraction = maximum_fraction
        self.minimum_significant = minimum_significant
        self.maximum_significant = maximum_significant
        self.increment = increment
        self.mode = mode
        self.rounding = rounding
        self.strip_if_integer = strip_if_integer


def read_digits(options: Options, mnfd_default: int, mxfd_default: int, notation: str) -> Digits:
    """``SetNumberFormatDigitOptions``, refusing what it refuses."""
    minimum_integer = get_number(options, "minimumIntegerDigits", 1, 21, 1)
    mnfd = given(options, "minimumFractionDigits")
    mxfd = given(options, "maximumFractionDigits")
    mnsd = given(options, "minimumSignificantDigits")
    mxsd = given(options, "maximumSignificantDigits")
    increment = get_number(options, "roundingIncrement", 1, 5000, 1)
    if increment not in _INCREMENTS:
        refuse()
    mode = get_string(options, "roundingMode", ROUNDING_MODES, "halfExpand") or "halfExpand"
    priority = get_string(options, "roundingPriority", ("auto", _MORE, _LESS), "auto") or "auto"
    trailing = get_string(options, "trailingZeroDisplay", ("auto", "stripIfInteger"), "auto")
    if increment != 1:
        mxfd_default = mnfd_default
    has_sd = mnsd is not MISSING or mxsd is not MISSING
    has_fd = mnfd is not MISSING or mxfd is not MISSING
    need_sd = True
    need_fd = True
    if priority == "auto":
        need_sd = has_sd
        if need_sd or (not has_fd and notation == "compact"):
            need_fd = False
    minimum_significant, maximum_significant = 1, 21
    if need_sd and has_sd:
        minimum_significant = default_number(mnsd, 1, 21, 1) or 1
        maximum_significant = default_number(mxsd, minimum_significant, 21, 21) or 21
    minimum_fraction, maximum_fraction = mnfd_default, mxfd_default
    if need_fd and has_fd:
        low = default_number(mnfd, 0, 100, None)
        high = default_number(mxfd, 0, 100, None)
        if low is None:
            low = min(mnfd_default, 0 if high is None else high)
        elif high is None:
            high = max(mxfd_default, low)
        elif low > high:
            refuse()
        minimum_fraction, maximum_fraction = low, high if high is not None else mxfd_default
    if not need_sd and not need_fd:
        minimum_fraction, maximum_fraction = 0, 0
        minimum_significant, maximum_significant = 1, 2
        rounding = _MORE
    elif priority == "auto":
        rounding = _SIGNIFICANT if has_sd else _FRACTION
    else:
        rounding = priority
    if increment != 1 and (rounding != _FRACTION or maximum_fraction != minimum_fraction):
        refuse()
    return Digits(
        minimum_integer=minimum_integer,
        minimum_fraction=minimum_fraction,
        maximum_fraction=maximum_fraction,
        minimum_significant=minimum_significant,
        maximum_significant=maximum_significant,
        increment=increment,
        mode=mode,
        rounding=rounding,
        strip_if_integer=trailing == "stripIfInteger",
    )


class Rounded:
    """A number rounded to its digits: the value, and the digits ECMA-402 writes for it."""

    __slots__ = ("integer_digits", "magnitude", "number", "text")

    def __init__(self, number: Decimal, text: str, integer_digits: int, magnitude: int) -> None:
        self.number = number
        self.text = text
        self.integer_digits = integer_digits
        self.magnitude = magnitude


def _unit(exponent: int) -> Decimal:
    return Decimal((0, (1,), exponent))


def _plain(value: Decimal) -> str:
    return format(value, "f")


def _cut(text: str, cut: int) -> str:
    # Trailing zeros of the fraction are dropped down to the minimum, and a
    # point left with nothing after it goes too.
    if "." in text:
        while cut > 0 and text.endswith("0"):
            text = text[:-1]
            cut -= 1
        if text.endswith("."):
            text = text[:-1]
    return text


def _raw_precision(x: Decimal, minimum: int, maximum: int, mode: str) -> Rounded:
    p = maximum
    if not x:
        digits = "0" * p
        e = 0
        rounded = Decimal(0)
    else:
        e = x.adjusted()
        rounded = x.quantize(_unit(e - p + 1), rounding=mode)
        if rounded.adjusted() > e:
            # Rounding carried into the next magnitude: the same digits, one
            # place further left.
            e = rounded.adjusted()
            rounded = rounded.quantize(_unit(e - p + 1))
        digits = "".join(map(str, rounded.as_tuple().digits))
    if e >= p - 1:
        text = digits + "0" * (e - p + 1)
        integer = e + 1
    elif e >= 0:
        text = digits[: e + 1] + "." + digits[e + 1 :]
        integer = e + 1
    else:
        text = "0." + "0" * -(e + 1) + digits
        integer = 1
    if maximum > minimum:
        text = _cut(text, maximum - minimum)
    return Rounded(rounded, text, integer, e - p + 1)


def _raw_fixed(x: Decimal, minimum: int, maximum: int, increment: int, mode: str) -> Rounded:
    f = maximum
    steps = (x.scaleb(f) / increment).quantize(_unit(0), rounding=mode)
    n = (steps * increment).quantize(_unit(0))
    rounded = n.scaleb(-f)
    text = _plain(n)
    if f:
        k = len(text)
        if k <= f:
            text = "0" * (f + 1 - k) + text
            k = f + 1
        text = text[: k - f] + "." + text[k - f :]
        integer = k - f
    else:
        integer = len(text)
    return Rounded(rounded, _cut(text, maximum - minimum), integer, -f)


def format_numeric(digits: Digits, x: Decimal) -> Rounded:
    """``FormatNumericToString``: ``x`` rounded, its sign kept, and its digits unsigned.

    The caller runs it inside :data:`PRECISION`.
    """
    negative = x.is_signed()
    magnitude = x.copy_abs()
    mode = _UNSIGNED[digits.mode][1 if negative else 0]
    if digits.rounding == _SIGNIFICANT:
        result = _raw_precision(
            magnitude, digits.minimum_significant, digits.maximum_significant, mode
        )
    elif digits.rounding == _FRACTION:
        result = _raw_fixed(
            magnitude, digits.minimum_fraction, digits.maximum_fraction, digits.increment, mode
        )
    else:
        significant = _raw_precision(
            magnitude, digits.minimum_significant, digits.maximum_significant, mode
        )
        fixed = _raw_fixed(
            magnitude, digits.minimum_fraction, digits.maximum_fraction, digits.increment, mode
        )
        more = significant.magnitude <= fixed.magnitude
        result = significant if more == (digits.rounding == _MORE) else fixed
    text = result.text
    if digits.strip_if_integer and result.number == result.number.to_integral_value():
        text = text.partition(".")[0]
    if result.integer_digits < digits.minimum_integer:
        text = "0" * (digits.minimum_integer - result.integer_digits) + text
    number = result.number.copy_negate() if negative else result.number
    return Rounded(
        number, text, max(result.integer_digits, digits.minimum_integer), result.magnitude
    )


def selection_operand(rounded: Rounded) -> Decimal:
    """The number a plural rule reads: the digits as shown, fraction digits and all."""
    return Decimal(rounded.text)
