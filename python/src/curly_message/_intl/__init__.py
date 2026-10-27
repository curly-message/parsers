"""The formatting modifiers and the plural selections, over Babel (sections 11.2 and 11.5).

Imported on the first call of one of them, never by ``import curly_message``.
Each modifier tests the caller's locale first, then reads its value and the
properties the layers compose, builds its request in ECMA-402's vocabulary,
offers it to the request seam, and only then performs it: a request
ECMA-402 refuses fails here as it fails there.
"""

from __future__ import annotations

import math
from collections.abc import Callable
from typing import Final

from .._convert import to_number
from .._modifiers import ModifierFailure
from .._requests import offer
from .._types import UNDEFINED, ModifierInput
from .._utils import MISSING
from .date import format_date, time_value
from .number import format_number
from .plural import select
from .relative import format_relative
from .validate import given, refuse
from .validate import to_number as js_number

__all__ = ["ago", "currency", "date", "number", "ordinal", "plural"]

DEFAULT_MAXIMUM: Final = 2
"""The fraction digits ``number`` shows where no layer names a maximum: a default, not a cap."""


def _locale(config: ModifierInput) -> object:
    # The locale is tested before anything else is read: without one, no
    # value and no link of the chain is formatted (section 11.2).
    locale = config.locale
    if (
        locale is None
        or locale is UNDEFINED
        or (
            issubclass(type(locale), str) and not str.__len__(locale)  # type: ignore[arg-type]
        )
    ):
        raise ModifierFailure("missing-locale")
    return locale


def _formattable(value: str) -> float:
    number = to_number(value)
    if number is None or not math.isfinite(number):
        refuse()
    return number


def _js_value(number: float) -> float | int:
    # A count a request carries is a number, written as JSON writes one.
    return int(number) if math.isfinite(number) and number.is_integer() else number


def _shown_digits(props: dict[str, object]) -> dict[str, object]:
    # The default maximum widens to a larger named minimum, as ECMA-402's own
    # default does: held at two under a minimum of four it would contradict
    # it, and the request would be refused.
    if "maximumFractionDigits" in props:
        return dict(props)
    minimum = js_number(given(props, "minimumFractionDigits"))
    widened = max(0.0 if math.isnan(minimum) else minimum, float(DEFAULT_MAXIMUM))
    return {**props, "maximumFractionDigits": _js_value(widened)}


def number(config: ModifierInput) -> str:
    """Format a number (section 11.2)."""
    locale = _locale(config)
    value = _formattable(config.value)
    options = _shown_digits(config.props)
    offer("NumberFormat", options, value)
    return format_number(locale, options, value)


def currency(config: ModifierInput) -> str:
    """Format an amount of money, the value scaled by the ``ratio`` property (section 11.2)."""
    locale = _locale(config)
    amount = _formattable(config.value)
    ratio = given(config.props, "ratio")
    product = amount * (1.0 if ratio is MISSING else js_number(ratio))
    if not math.isfinite(product):
        refuse()
    # The style is what this modifier is, pinned over every layer; the ratio
    # is already in the amount.
    options = {name: value for name, value in config.props.items() if name != "ratio"}
    options["style"] = "currency"
    offer("NumberFormat", options, product)
    return format_number(locale, options, product)


def date(config: ModifierInput) -> str:
    """Format a time: a timestamp, or ISO 8601 text (section 11.2)."""
    locale = _locale(config)
    time = time_value(config.value)
    if time is None:
        refuse()
    options = dict(config.props)
    offer("DateTimeFormat", options, time)
    return format_date(locale, options, time)


AGO_LADDER: Final = (
    ("second", 1000.0),
    ("minute", 60.0),
    ("hour", 60.0),
    ("day", 24.0),
    ("week", 7.0),
    ("month", 13 / 3),
    ("year", 12.0),
)
"""The units ``ago`` climbs, each with the count of the one below it it takes."""


def _names(key: str, text: str) -> bool:
    # A unit is named by its singular or its plural, spelled exactly.
    return text in (key, key + "s")


def _step(value: float) -> float:
    # Rounded on the magnitude, then given its sign back, so a delta and its
    # negation climb alike; half rounds away from zero.
    magnitude = abs(value)
    whole = math.floor(magnitude)
    rounded = float(whole + (1 if magnitude - whole >= 0.5 else 0))
    return math.copysign(rounded, value)


def _ladder(millis: float, resolution: str) -> tuple[float, str]:
    value, current = millis, ""
    for index, (key, multiplier) in enumerate(AGO_LADDER):
        if current and _names(current, resolution):
            continue
        current_index = next((i for i, (k, _) in enumerate(AGO_LADDER) if k == current), -1)
        if not current or index == current_index + 1:
            output = _step(value / multiplier)
            if not current or abs(output) >= 1 or resolution != "auto":
                value, current = output, key
    return value, current


def ago(config: ModifierInput) -> str:
    """Format a duration as a relative time (section 11.2)."""
    locale = _locale(config)
    millis = _formattable(config.value)
    props = config.props
    resolution = given(props, "format")
    resolution = "auto" if resolution is MISSING else resolution
    if not issubclass(type(resolution), str):
        refuse()
    unit_name = str.__str__(resolution)
    if unit_name != "auto" and not any(_names(key, unit_name) for key, _ in AGO_LADDER):
        refuse()
    value, unit = _ladder(millis, unit_name)
    options = {name: value_ for name, value_ in props.items() if name != "format"}
    if "numeric" not in options:
        options["numeric"] = "auto"
    offer("RelativeTimeFormat", options, [_js_value(value), unit])
    return format_relative(locale, options, value, unit)


def _selection(
    config: ModifierInput, kind: str, digits: Callable[[dict[str, object]], dict[str, object]]
) -> str:
    locale = _locale(config)
    value = _formattable(config.value)
    if kind == "ordinal" and not value.is_integer():
        # An ordinal of a fraction names no position, and the ordinal rules
        # are written for whole numbers.
        refuse()
    for option in config.options:
        key = to_number(option.key)
        if key is not None and math.isfinite(key) and key == value:
            return option.value
    options = {**digits(config.props), "type": kind}
    request = offer("PluralRules", options, value)
    category = select(locale, options, value)
    if request is not None:
        request["category"] = category
    found = next((option for option in config.options if option.key == category), None)
    return found.value if found is not None else config.default


def plural(config: ModifierInput) -> str:
    """Select by the plural category of the number as ``number`` shows it (section 11.5)."""
    return _selection(config, "cardinal", _shown_digits)


def ordinal(config: ModifierInput) -> str:
    """Select by the ordinal category of a whole number (section 11.5)."""
    return _selection(config, "ordinal", dict)
