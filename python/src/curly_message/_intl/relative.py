"""``Intl.RelativeTimeFormat`` over the supplier's locale data."""

from __future__ import annotations

import math
from typing import Any, Final

from .locale import babel_locale
from .number import format_number
from .plural import select
from .validate import LOCALE_MATCHERS, Options, get_string, get_type_sequence, refuse

UNITS: Final = ("second", "minute", "hour", "day", "week", "month", "quarter", "year")

_LENGTHS: Final = {"long": ("",), "short": ("-short", ""), "narrow": ("-narrow", "-short", "")}
"""The lengths a unit's patterns are read at: CLDR writes a narrow length
that says nothing new as the short one, and a short one as the long, and the
supplier keeps no trace of either."""


def _patterns(fields: Any, unit: str, style: str, direction: str) -> Any:
    for suffix in _LENGTHS[style]:
        patterns = (fields.get(unit + suffix) or {}).get(direction)
        if patterns:
            return patterns
    return {}


def format_relative(tag: object, options: Options, value: float, unit: str) -> str:
    """``new Intl.RelativeTimeFormat(tag, options).format(value, unit)``.

    With ``numeric: "auto"`` a phrase such as "yesterday" is used where the
    locale data has one; the supplier's data has none, so the count is
    always shown.
    """
    get_string(options, "localeMatcher", LOCALE_MATCHERS, "best fit")
    get_type_sequence(options, "numberingSystem")
    style = get_string(options, "style", ("long", "short", "narrow"), "long")
    get_string(options, "numeric", ("always", "auto"), "always")
    locale, _ = babel_locale(tag)
    singular = unit[:-1] if unit.endswith("s") else unit
    if singular not in UNITS:
        refuse()
    past = value < 0 or (value == 0 and math.copysign(1.0, value) < 0)
    count = abs(value)
    patterns = _patterns(
        locale._data["date_fields"], singular, style or "long", "past" if past else "future"
    )
    pattern = patterns.get(select(tag, {}, count)) or patterns.get("other")
    if pattern is None:
        refuse()
    return str(pattern).replace("{0}", format_number(tag, {}, count))
