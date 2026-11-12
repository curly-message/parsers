"""The text reading: a formatting request performed on this runner's own host.

RUNNER.md has a runner perform a request "on its own host" where an adapter
does not expose it. Python's host has no ECMA-402 facility, so this module
maps the request vocabulary of the set onto Babel's locale data itself. It
does not import the implementation under test: a text reading that asked the
implementation how to format would hold it to itself.

The mapping is exactly as wide as the requests the set states, and a request
outside it raises, which ends the run: a reading this runner cannot perform
is not one an implementation passed.
"""

from __future__ import annotations

import decimal
import math
import re
from collections.abc import Callable, Mapping
from datetime import UTC, datetime, timedelta, tzinfo
from decimal import Decimal
from typing import Final, cast
from zoneinfo import ZoneInfo

import babel
from babel import dates, numbers

from .model import SetError

_EPOCH: Final = datetime(1970, 1, 1, tzinfo=UTC)

_FRACTION: Final = re.compile(r"0(?:\.[0#]+)?")

_PROPERTIES: Final = {
    "NumberFormat": frozenset(
        {
            "minimumFractionDigits",
            "maximumFractionDigits",
            "useGrouping",
            "style",
            "currency",
            "currencyDisplay",
        }
    ),
    "DateTimeFormat": frozenset({"timeZone", "dateStyle", "timeStyle"}),
    "RelativeTimeFormat": frozenset({"numeric", "style"}),
    "PluralRules": frozenset(
        {
            "minimumIntegerDigits",
            "minimumFractionDigits",
            "maximumFractionDigits",
            "minimumSignificantDigits",
            "type",
        }
    ),
}

_STYLES: Final = ("full", "long", "medium", "short")


def _beyond(what: str) -> SetError:
    return SetError(f"The text reading cannot perform {what}.")


def _locale(tag: object) -> babel.Locale:
    if not isinstance(tag, str):
        raise _beyond(f"a request under the locale {tag!r}")
    return babel.Locale.parse(tag.replace("-", "_"))


def _choice(
    options: Mapping[str, object], name: str, allowed: tuple[str, ...], fallback: str
) -> str:
    value = options.get(name, fallback)
    if value not in allowed:
        raise _beyond(f"{name} {value!r}")
    return value


def _digit(options: Mapping[str, object], name: str) -> int | None:
    # A property stated as null is read as the number null converts to, as
    # ECMA-402 reads every digit option that is not undefined.
    if name not in options:
        return None
    value = options[name]
    number = 0 if value is None else value
    if isinstance(number, bool) or not isinstance(number, int | float) or number != int(number):
        raise _beyond(f"{name} {value!r}")
    return int(number)


def _fraction_digits(options: Mapping[str, object], low: int, high: int) -> tuple[int, int]:
    # ECMA-402 SetNumberFormatDigitOptions, for the fraction digits alone.
    least, most = _digit(options, "minimumFractionDigits"), _digit(options, "maximumFractionDigits")
    if least is None and most is None:
        return low, high
    if least is None:
        return min(low, cast("int", most)), cast("int", most)
    if most is None:
        return least, max(high, least)
    if least > most:
        raise _beyond("a minimum above its maximum")
    return least, most


def _rounded(value: object, least: int, most: int) -> Decimal:
    # Halves round away from zero, and a number reads as its shortest
    # spelling, as the host the set was written on reads it.
    if isinstance(value, bool) or not isinstance(value, int | float) or not math.isfinite(value):
        raise _beyond(f"the input {value!r}")
    number = Decimal(repr(value)).quantize(Decimal(1).scaleb(-most), rounding=decimal.ROUND_HALF_UP)
    text = format(number, "f")
    if "." in text:
        whole, fraction = text.split(".")
        fraction = fraction.rstrip("0").ljust(least, "0")
        text = whole + ("." + fraction if fraction else "")
    return Decimal(text)


def _pattern(pattern: str, least: int, most: int) -> str:
    fraction = "." + "0" * least + "#" * (most - least) if most else ""
    return _FRACTION.sub(lambda _: "0" + fraction, pattern)


def _number(options: Mapping[str, object], value: object, tag: object) -> str:
    locale = _locale(tag)
    style = _choice(options, "style", ("decimal", "currency"), "decimal")
    grouping = options.get("useGrouping", True)
    if not isinstance(grouping, bool):
        raise _beyond(f"useGrouping {grouping!r}")
    if style == "decimal":
        least, most = _fraction_digits(options, 0, 3)
        pattern = _pattern(str(locale.decimal_formats[None].pattern), least, most)
        return numbers.format_decimal(
            _rounded(value, least, most), format=pattern, locale=locale, group_separator=grouping
        )
    currency = options.get("currency")
    if not isinstance(currency, str):
        raise _beyond(f"the currency {currency!r}")
    display = _choice(options, "currencyDisplay", ("symbol", "code"), "symbol")
    digits = numbers.get_currency_precision(currency)
    least, most = _fraction_digits(options, digits, digits)
    pattern = _pattern(str(locale.currency_formats["standard"].pattern), least, most)
    if display == "code":
        pattern = pattern.replace("\xa4", "\xa4\xa4")
    return numbers.format_currency(
        _rounded(value, least, most),
        currency,
        format=pattern,
        locale=locale,
        currency_digits=False,
        group_separator=grouping,
    )


def _zone(options: Mapping[str, object]) -> tzinfo:
    name = options.get("timeZone")
    if not isinstance(name, str):
        raise _beyond("a date in the host's own time zone")
    return UTC if name.upper() == "UTC" else ZoneInfo(name)


def _date(options: Mapping[str, object], value: object, tag: object) -> str:
    locale = _locale(tag)
    if isinstance(value, bool) or not isinstance(value, int | float):
        raise _beyond(f"the time value {value!r}")
    instant = (_EPOCH + timedelta(milliseconds=value)).astimezone(_zone(options))
    date_style, time_style = options.get("dateStyle"), options.get("timeStyle")
    for style in (date_style, time_style):
        if style is not None and style not in _STYLES:
            raise _beyond(f"the style {style!r}")
    zone = instant.tzinfo
    if date_style is None and time_style is None:
        text = dates.format_skeleton("yMd", instant, tzinfo=zone, locale=locale)
    elif time_style is None:
        text = dates.format_date(instant.date(), cast("str", date_style), locale=locale)
    else:
        time = dates.format_time(instant, cast("str", time_style), tzinfo=zone, locale=locale)
        if date_style is None:
            text = time
        else:
            day = dates.format_date(instant.date(), cast("str", date_style), locale=locale)
            glue = str(locale.datetime_formats[cast("str", date_style)])
            text = glue.replace("{1}", day).replace("{0}", time)
    # V8 writes the narrow space CLDR puts before a day period as a space.
    return text.replace("\u202f", " ")


_UNITS: Final = ("second", "minute", "hour", "day", "week", "month", "quarter", "year")

_LENGTHS: Final = {"long": ("",), "short": ("-short", ""), "narrow": ("-narrow", "-short", "")}


def _relative(options: Mapping[str, object], value: object, tag: object) -> str:
    locale = _locale(tag)
    _choice(options, "numeric", ("always", "auto"), "always")
    style = _choice(options, "style", ("long", "short", "narrow"), "long")
    if not isinstance(value, list | tuple) or len(value) != 2:
        raise _beyond(f"the relative time {value!r}")
    count, unit = value
    if isinstance(count, bool) or not isinstance(count, int) or unit not in _UNITS:
        raise _beyond(f"the relative time {value!r}")
    # The patterns are read off Babel's data rather than through
    # format_timedelta, which in Babel 2.14 writes the long phrase whatever the
    # length asked for. A length CLDR leaves as the longer one falls back to
    # it. The data holds no phrase such as "yesterday", so ``numeric: auto``
    # shows the count as ``always`` does.
    fields = locale._data["date_fields"]
    direction = "past" if count < 0 else "future"
    patterns: Mapping[str, object] = {}
    for suffix in _LENGTHS[style]:
        patterns = (fields.get(f"{unit}{suffix}") or {}).get(direction) or {}
        if patterns:
            break
    pattern = patterns.get(str(locale.plural_form(abs(count)))) or patterns.get("other")
    if pattern is None:
        raise _beyond(f"the relative time {value!r} in {locale}")
    return str(pattern).replace("{0}", numbers.format_decimal(abs(count), locale=locale))


def _plural(options: Mapping[str, object], value: object, tag: object) -> str:
    locale = _locale(tag)
    kind = _choice(options, "type", ("cardinal", "ordinal"), "cardinal")
    if isinstance(value, bool) or not isinstance(value, int | float) or not math.isfinite(value):
        raise _beyond(f"the input {value!r}")
    if "minimumSignificantDigits" in options:
        # Significant digits take precedence over fraction digits in ECMA-402,
        # and only a minimum is stated: up to 21 digits, none dropped.
        least = cast("int", _digit(options, "minimumSignificantDigits"))
        operand = Decimal(repr(value))
        shown = len(operand.as_tuple().digits)
        if shown < least:
            operand = operand.quantize(Decimal(1).scaleb(operand.adjusted() - least + 1))
    else:
        least, most = _fraction_digits(options, 0, 3)
        operand = _rounded(value, least, most)
    rule = cast(
        "Callable[[Decimal], str]",
        locale.ordinal_form if kind == "ordinal" else locale.plural_form,
    )
    return str(rule(operand))


_PERFORMED: Final[Mapping[str, Callable[[Mapping[str, object], object, object], str]]] = {
    "NumberFormat": _number,
    "DateTimeFormat": _date,
    "RelativeTimeFormat": _relative,
    "PluralRules": _plural,
}


def perform(request: Mapping[str, object], locale: object) -> str:
    """The text a request reads as on this host, or for ``PluralRules`` the category it selects."""
    api = request.get("api")
    if not isinstance(api, str) or api not in _PERFORMED:
        raise _beyond(f"a request of the facility {api!r}")
    options = cast("Mapping[str, object]", request.get("options") or {})
    beyond = set(options) - _PROPERTIES[api]
    if beyond:
        raise _beyond(f"the {api} properties {sorted(beyond)}")
    return _PERFORMED[api](options, request.get("input"), locale)
