"""The formatting modifiers and the plural selections, over Babel (sections 11.2 and 11.5).

Imported on the first call of one of them, never by ``import curly_message``.
Each modifier tests the caller's locale first, then reads its value and the
properties the layers compose, builds its request in ECMA-402's vocabulary,
offers it to the request seam, and only then performs it: a request
ECMA-402 refuses fails here as it fails there.
"""

from __future__ import annotations

import math
from collections.abc import Callable, Mapping
from typing import Final, TypeVar, cast

from .._convert import to_number
from .._modifiers import ModifierFailure
from .._requests import offer
from .._types import UNDEFINED, ModifierInput, Option
from .._utils import MISSING
from .date import DateSpec, format_date, time_value
from .number import NumberSpec, format_number
from .plural import PluralSpec, select
from .relative import format_relative
from .validate import Options, given, refuse
from .validate import to_number as js_number

__all__ = ["ago", "currency", "date", "number", "ordinal", "plural"]

DEFAULT_MAXIMUM: Final = 2
"""The fraction digits ``number`` shows where no layer names a maximum: a default, not a cap."""

_Spec = TypeVar("_Spec")
_Read = TypeVar("_Read")


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


def _shown_digits(props: Mapping[str, object]) -> dict[str, object]:
    # The default maximum widens to a larger named minimum, as ECMA-402's own
    # default does: held at two under a minimum of four it would contradict
    # it, and the request would be refused.
    if "maximumFractionDigits" in props:
        return dict(props)
    minimum = js_number(given(props, "minimumFractionDigits"))
    widened = max(0.0 if math.isnan(minimum) else minimum, float(DEFAULT_MAXIMUM))
    return {**props, "maximumFractionDigits": _js_value(widened)}


def _prepared(config: ModifierInput, read: Callable[[Mapping[str, object]], _Read]) -> _Read:
    # What a request reads of the props is decided by them alone, and the walk
    # hands every placeholder over the same layers one composition of them,
    # so it is read once for that composition and kept with it.
    kept = config._prepared
    if kept is None:
        return read(config.props)
    found = kept.get(read, MISSING)
    if found is MISSING:
        found = kept[read] = read(config.props)
    return cast("_Read", found)


def _request(
    spec: Callable[[Options], _Spec], options: dict[str, object]
) -> tuple[dict[str, object], _Spec | None]:
    # A request ECMA-402 refuses is still made, and refused once it is.
    try:
        return options, spec(options)
    except ModifierFailure:
        return options, None


def _number_request(props: Mapping[str, object]) -> tuple[dict[str, object], NumberSpec | None]:
    return _request(NumberSpec, _shown_digits(props))


def number(config: ModifierInput) -> str:
    """Format a number (section 11.2)."""
    locale = _locale(config)
    value = _formattable(config.value)
    options, spec = _prepared(config, _number_request)
    offer("NumberFormat", options, value)
    if spec is None:
        refuse()
    return format_number(locale, spec, value)


def _currency_request(
    props: Mapping[str, object],
) -> tuple[float, dict[str, object], NumberSpec | None]:
    ratio = given(props, "ratio")
    # The style is what this modifier is, pinned over every layer; the ratio
    # is already in the amount.
    options = {name: value for name, value in props.items() if name != "ratio"}
    options["style"] = "currency"
    return (1.0 if ratio is MISSING else js_number(ratio)), *_request(NumberSpec, options)


def currency(config: ModifierInput) -> str:
    """Format an amount of money, the value scaled by the ``ratio`` property (section 11.2)."""
    locale = _locale(config)
    amount = _formattable(config.value)
    ratio, options, spec = _prepared(config, _currency_request)
    product = amount * ratio
    if not math.isfinite(product):
        refuse()
    offer("NumberFormat", options, product)
    if spec is None:
        refuse()
    return format_number(locale, spec, product)


def _date_request(props: Mapping[str, object]) -> tuple[dict[str, object], DateSpec | None]:
    return _request(DateSpec, dict(props))


def date(config: ModifierInput) -> str:
    """Format a time: a timestamp, or ISO 8601 text (section 11.2)."""
    locale = _locale(config)
    time = time_value(config.value)
    if time is None:
        refuse()
    options, spec = _prepared(config, _date_request)
    offer("DateTimeFormat", options, time)
    if spec is None:
        refuse()
    return format_date(locale, spec, time)


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


def _selected(
    config: ModifierInput,
    kind: str,
    read: Callable[[Mapping[str, object]], tuple[dict[str, object], PluralSpec | None]],
) -> Option | None:
    # The option is selected here and read by the modifier itself, so a read
    # in progress stands no frame of this helper (``MAX_REENTRY``).
    locale = _locale(config)
    value = _formattable(config.value)
    if kind == "ordinal" and not value.is_integer():
        # An ordinal of a fraction names no position, and the ordinal rules
        # are written for whole numbers.
        refuse()
    for option in config.options:
        key = to_number(option.key)
        if key is not None and math.isfinite(key) and key == value:
            return option
    options, spec = _prepared(config, read)
    request = offer("PluralRules", options, value)
    if spec is None:
        refuse()
    category = select(locale, spec, value)
    if request is not None:
        request["category"] = category
    return next((option for option in config.options if option.key == category), None)


def _plural_request(props: Mapping[str, object]) -> tuple[dict[str, object], PluralSpec | None]:
    return _request(PluralSpec, {**_shown_digits(props), "type": "cardinal"})


def plural(config: ModifierInput) -> str:
    """Select by the plural category of the number as ``number`` shows it (section 11.5)."""
    option = _selected(config, "cardinal", _plural_request)
    return option.value if option is not None else config.default


def _ordinal_request(props: Mapping[str, object]) -> tuple[dict[str, object], PluralSpec | None]:
    return _request(PluralSpec, {**props, "type": "ordinal"})


def ordinal(config: ModifierInput) -> str:
    """Select by the ordinal category of a whole number (section 11.5)."""
    option = _selected(config, "ordinal", _ordinal_request)
    return option.value if option is not None else config.default
