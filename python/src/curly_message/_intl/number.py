"""``Intl.NumberFormat`` over the supplier's locale data.

The digits are this package's own (``digits.py``): the supplier's renderers
round again on their own terms, so a number reaches them already rounded and
written out, and only their data is read here: the patterns' affixes and
grouping, the symbols, the currency and unit names.
"""

from __future__ import annotations

import decimal
import functools
from collections.abc import Callable, Mapping
from decimal import Decimal
from typing import Any, Final

from babel import Locale
from babel.numbers import (
    NumberPattern,
    get_currency_name,
    get_currency_precision,
    get_currency_unit_pattern,
)

from .._utils import MISSING
from .digits import PRECISION, Digits, Rounded, format_numeric, read_digits, selection_operand
from .locale import babel_locale
from .validate import (
    LOCALE_MATCHERS,
    Options,
    get_string,
    get_type_sequence,
    given,
    is_currency_code,
    refuse,
    to_boolean,
    to_string,
)

# The units ECMA-402 sanctions (its table of simple units), each under the
# category its locale data files it in.
UNITS: Final = {
    "acre": "area",
    "bit": "digital",
    "byte": "digital",
    "celsius": "temperature",
    "centimeter": "length",
    "day": "duration",
    "degree": "angle",
    "fahrenheit": "temperature",
    "fluid-ounce": "volume",
    "foot": "length",
    "gallon": "volume",
    "gigabit": "digital",
    "gigabyte": "digital",
    "gram": "mass",
    "hectare": "area",
    "hour": "duration",
    "inch": "length",
    "kilobit": "digital",
    "kilobyte": "digital",
    "kilogram": "mass",
    "kilometer": "length",
    "liter": "volume",
    "megabit": "digital",
    "megabyte": "digital",
    "meter": "length",
    "microsecond": "duration",
    "mile": "length",
    "mile-scandinavian": "length",
    "milliliter": "volume",
    "millimeter": "length",
    "millisecond": "duration",
    "minute": "duration",
    "month": "duration",
    "nanosecond": "duration",
    "ounce": "mass",
    "percent": "concentr",
    "petabyte": "digital",
    "pound": "mass",
    "second": "duration",
    "stone": "mass",
    "terabit": "digital",
    "terabyte": "digital",
    "week": "duration",
    "yard": "length",
    "year": "duration",
}

_PER: Final = "-per-"

STYLES: Final = ("decimal", "percent", "currency", "unit")
NOTATIONS: Final = ("standard", "scientific", "engineering", "compact")
SIGN_DISPLAYS: Final = ("auto", "never", "always", "exceptZero", "negative")
_GROUPINGS: Final = ("min2", "auto", "always", "true", "false")


def _unit_parts(unit: str) -> tuple[str, str | None] | None:
    # ``IsWellFormedUnitIdentifier``: a sanctioned unit, or two joined by
    # ``-per-``, spelled exactly.
    if unit in UNITS:
        return unit, None
    numerator, per, denominator = unit.partition(_PER)
    if per and numerator in UNITS and denominator in UNITS:
        return numerator, denominator
    return None


class NumberSpec:
    """A ``NumberFormat`` request, read and resolved as ECMA-402 resolves it."""

    __slots__ = (
        "compact_display",
        "currency",
        "currency_display",
        "currency_sign",
        "digits",
        "grouping",
        "notation",
        "sign_display",
        "style",
        "unit",
        "unit_display",
    )

    def __init__(self, options: Options) -> None:
        get_string(options, "localeMatcher", LOCALE_MATCHERS, "best fit")
        get_type_sequence(options, "numberingSystem")
        self.style = get_string(options, "style", STYLES, "decimal") or "decimal"
        currency = get_string(options, "currency", None, None)
        if currency is not None and not is_currency_code(currency):
            refuse()
        self.currency = None if currency is None else currency.upper()
        self.currency_display = (
            get_string(
                options, "currencyDisplay", ("code", "symbol", "narrowSymbol", "name"), "symbol"
            )
            or "symbol"
        )
        self.currency_sign = (
            get_string(options, "currencySign", ("standard", "accounting"), "standard")
            or "standard"
        )
        unit = get_string(options, "unit", None, None)
        parts = None if unit is None else _unit_parts(unit)
        if unit is not None and parts is None:
            refuse()
        # Every style validates the unit, and only the unit style shows it.
        self.unit = parts if self.style == "unit" else None
        self.unit_display = (
            get_string(options, "unitDisplay", ("short", "narrow", "long"), "short") or "short"
        )
        if (self.style == "currency" and self.currency is None) or (
            self.style == "unit" and self.unit is None
        ):
            refuse()
        self.notation = get_string(options, "notation", NOTATIONS, "standard") or "standard"
        if self.style == "currency" and self.notation == "standard" and self.currency is not None:
            fraction = get_currency_precision(self.currency)
            mnfd_default, mxfd_default = fraction, fraction
        else:
            mnfd_default, mxfd_default = 0, 0 if self.style == "percent" else 3
        self.digits: Digits = read_digits(options, mnfd_default, mxfd_default, self.notation)
        self.compact_display = (
            get_string(options, "compactDisplay", ("short", "long"), "short") or "short"
        )
        self.grouping = _use_grouping(options, "min2" if self.notation == "compact" else "auto")
        self.sign_display = get_string(options, "signDisplay", SIGN_DISPLAYS, "auto") or "auto"


def _use_grouping(options: Options, fallback: str) -> str | None:
    # ``GetBooleanOrStringNumberFormatOption``: ``true`` groups always, a
    # false value never, and the strings ``"true"`` and ``"false"`` name
    # neither, so they take the default.
    value = given(options, "useGrouping")
    if value is MISSING:
        return fallback
    if value is True:
        return "always"
    if not to_boolean(value):
        return None
    text = to_string(value)
    if text not in _GROUPINGS:
        refuse()
    if text in ("true", "false"):
        return fallback
    return text


Symbols = Mapping[str, str]


def _symbols(locale: Locale) -> Symbols:
    found: Symbols = locale.number_symbols["latn"]
    return found


def _affix(text: str, symbols: Symbols, sign: str, currency: Callable[[int], str]) -> str:
    """A pattern's prefix or suffix as text: quotes read, and each special character replaced."""
    out: list[str] = []
    index = 0
    while index < len(text):
        character = text[index]
        if character == "'":
            end = text.find("'", index + 1)
            if end == index + 1:
                out.append("'")
                index += 2
                continue
            if end == -1:
                out.append(text[index + 1 :])
                break
            out.append(text[index + 1 : end])
            index = end + 1
            continue
        if character == "\xa4":
            end = index
            while end < len(text) and text[end] == "\xa4":
                end += 1
            out.append(currency(end - index))
            index = end
            continue
        if character == "%":
            out.append(symbols["percentSign"])
        elif character == "\u2030":
            out.append(symbols["perMille"])
        elif character == "-":
            out.append(sign)
        elif character == "+":
            out.append(symbols["plusSign"])
        else:
            out.append(character)
        index += 1
    return "".join(out)


def _no_currency(width: int) -> str:
    return ""


def _sign_of(display: str, number: Decimal) -> str:
    """Which sign the number shows under ``signDisplay``: ``"-"``, ``"+"`` or none."""
    negative = number.is_signed()
    zero = not number
    if display == "never":
        return ""
    if display == "auto":
        return "-" if negative else ""
    if display == "always":
        return "-" if negative else "+"
    if display == "exceptZero":
        if zero:
            return ""
        return "-" if negative else "+"
    return "-" if negative and not zero else ""


def _wrap(
    pattern: NumberPattern,
    core: str,
    sign: str,
    symbols: Symbols,
    currency: Callable[[int], str] = _no_currency,
) -> str:
    """The number placed in a pattern's affixes, for the sign it shows."""
    prefixes: tuple[str, str] = pattern.prefix
    suffixes: tuple[str, str] = pattern.suffix
    if not sign:
        return (
            _affix(prefixes[0], symbols, "", currency)
            + core
            + _affix(suffixes[0], symbols, "", currency)
        )
    shown = symbols["minusSign"] if sign == "-" else symbols["plusSign"]
    if sign == "+" and "-" not in prefixes[1] + suffixes[1]:
        # A negative pattern that writes no minus sign, the accounting one's
        # parentheses, has no place a plus sign could take.
        return (
            shown
            + _affix(prefixes[0], symbols, "", currency)
            + core
            + _affix(suffixes[0], symbols, "", currency)
        )
    return (
        _affix(prefixes[1], symbols, shown, currency)
        + core
        + _affix(suffixes[1], symbols, shown, currency)
    )


def _grouped(integer: str, pattern: NumberPattern, grouping: str | None, separator: str) -> str:
    primary, secondary = pattern.grouping
    if grouping is None or primary >= len(integer) or primary >= 1000:
        return integer
    if grouping == "min2" and len(integer) < primary + 2:
        return integer
    groups = [integer[-primary:]]
    rest = integer[:-primary]
    while len(rest) > secondary:
        groups.append(rest[-secondary:])
        rest = rest[:-secondary]
    if rest:
        groups.append(rest)
    return separator.join(reversed(groups))


def _digits_text(
    rounded: Rounded, pattern: NumberPattern, spec: NumberSpec, symbols: Symbols
) -> str:
    integer, point, fraction = rounded.text.partition(".")
    currency = spec.style == "currency" and spec.currency_display != "name"
    group = symbols.get("currencyGroup", symbols["group"]) if currency else symbols["group"]
    decimal_symbol = (
        symbols.get("currencyDecimal", symbols["decimal"]) if currency else symbols["decimal"]
    )
    text = _grouped(integer, pattern, spec.grouping, group)
    return text + decimal_symbol + fraction if point else text


def _exponent_for(spec: NumberSpec, locale: Locale, magnitude: int) -> tuple[int, str | None]:
    """``ComputeExponentForMagnitude``, and the compact pattern's key where one applies."""
    if spec.notation == "scientific":
        return magnitude, None
    if spec.notation == "engineering":
        return (magnitude // 3) * 3, None
    if magnitude < 3:
        return 0, None
    formats = _compact_formats(locale, spec.compact_display)["other"]
    key = None
    for candidate in formats:
        if len(candidate) - 1 <= magnitude and (key is None or len(candidate) > len(key)):
            key = candidate
    if key is None:
        return 0, None
    pattern: NumberPattern = formats[key]
    if pattern.pattern == "0":
        return 0, None
    return len(key) - 1 - (pattern.int_prec[0] - 1), key


def _compact_formats(locale: Locale, display: str) -> Any:
    # A locale without long compact patterns writes the short ones, as ICU
    # falls back.
    formats = locale.compact_decimal_formats
    return formats.get(display) or formats["short"]


def _round(spec: NumberSpec, locale: Locale, value: Decimal) -> tuple[Rounded, int, str | None]:
    """``ComputeExponent`` and the rounding of the scaled number, as one."""
    if spec.notation == "standard" or not value:
        return format_numeric(spec.digits, value), 0, None
    magnitude = value.adjusted()
    exponent, key = _exponent_for(spec, locale, magnitude)
    rounded = format_numeric(spec.digits, value.scaleb(-exponent))
    if rounded.number and rounded.number.adjusted() != magnitude - exponent:
        exponent, key = _exponent_for(spec, locale, magnitude + 1)
        rounded = format_numeric(spec.digits, value.scaleb(-exponent))
    return rounded, exponent, key


def _unit_patterns(locale: Locale) -> Any:
    return locale._data["unit_patterns"]


@functools.lru_cache(maxsize=64)
def _compound_keys(locale: Locale) -> dict[str, str]:
    # The locale's own compound units, ``speed-kilometer-per-hour`` among
    # them, by the identifier ECMA-402 writes for them.
    return {key.partition("-")[2]: key for key in _unit_patterns(locale) if _PER in key}


_LENGTHS: Final = {
    "long": ("long",),
    "short": ("short", "long"),
    "narrow": ("narrow", "short", "long"),
}
"""The lengths a unit's data is read at: CLDR writes a narrow length that
says nothing new as the short one, and a short one as the long."""


def _at_length(lengths: Any, display: str, *names: str) -> str | None:
    # The first length that holds anything is the one read: a category it
    # lacks is its ``other``, not another length's category.
    for length in _LENGTHS[display]:
        patterns = lengths.get(length)
        if patterns:
            return next((str(patterns[name]) for name in names if patterns.get(name)), None)
    return None


def _unit_pattern(locale: Locale, key: str, display: str, category: str) -> str | None:
    return _at_length(_unit_patterns(locale).get(key) or {}, display, category, "other")


def _percent_unit(spec: NumberSpec) -> bool:
    # ICU writes the percent unit at a short or narrow length as the
    # locale's percent pattern, unscaled, rather than as a unit pattern;
    # compact notation and the long length keep the unit's own.
    return (
        spec.unit == ("percent", None)
        and spec.unit_display != "long"
        and spec.notation != "compact"
    )


def _with_unit(locale: Locale, spec: NumberSpec, text: str, operand: Decimal) -> str:
    if spec.unit is None:
        return text
    category = str(locale.plural_form(operand))
    numerator, denominator = spec.unit
    display = spec.unit_display
    numerator_key = f"{UNITS[numerator]}-{numerator}"
    if denominator is None:
        pattern = _unit_pattern(locale, numerator_key, display, category)
        return text if pattern is None else pattern.replace("{0}", text)
    compound = _compound_keys(locale).get(f"{numerator}{_PER}{denominator}")
    if compound is not None:
        pattern = _unit_pattern(locale, compound, display, category)
        if pattern is not None:
            return pattern.replace("{0}", text)
    numerator_pattern = _unit_pattern(locale, numerator_key, display, category)
    shown = text if numerator_pattern is None else numerator_pattern.replace("{0}", text)
    denominator_key = f"{UNITS[denominator]}-{denominator}"
    per = _at_length(_unit_patterns(locale).get(denominator_key) or {}, display, "per")
    if per:
        return per.replace("{0}", shown)
    singular = _unit_pattern(locale, denominator_key, display, "one") or denominator
    name = singular.replace("{0}", "").strip()
    compound_pattern = (
        _at_length(locale._data["compound_unit_patterns"].get("per") or {}, display, "compound")
        or "{0}/{1}"
    )
    return compound_pattern.replace("{0}", shown).replace("{1}", name)


def _currency_text(locale: Locale, code: str, display: str) -> Callable[[int], str]:
    def currency(width: int) -> str:
        if width == 2 or display == "code":
            return code
        symbol: str = locale.currency_symbols.get(code, code)
        return symbol

    return currency


def format_number(tag: object, options: Options, value: float) -> str:
    """``new Intl.NumberFormat(tag, options).format(value)``."""
    spec = NumberSpec(options)
    locale, _ = babel_locale(tag)
    symbols = _symbols(locale)
    with decimal.localcontext(PRECISION):
        number = Decimal(repr(float(value)))
        if spec.style == "percent":
            number = number.scaleb(2)
        rounded, exponent, key = _round(spec, locale, number)
        operand = selection_operand(rounded)
        if spec.style == "currency" and spec.currency_display != "name":
            pattern = locale.currency_formats[spec.currency_sign]
        elif spec.style == "percent" or _percent_unit(spec):
            pattern = locale.percent_formats[None]
        else:
            pattern = locale.decimal_formats[None]
        core = _digits_text(
            rounded, locale.decimal_formats[None] if key else pattern, spec, symbols
        )
        if spec.notation in ("scientific", "engineering"):
            core += symbols["exponential"]
            if exponent < 0:
                core += symbols["minusSign"]
            core += str(abs(exponent))
        elif key is not None:
            formats = _compact_formats(locale, spec.compact_display)
            category = str(locale.plural_form(operand))
            compact: NumberPattern = (formats.get(category) or formats["other"]).get(
                key
            ) or formats["other"][key]
            core = _wrap(compact, core, "", symbols)
        sign = _sign_of(spec.sign_display, rounded.number)
        if spec.style == "currency" and spec.currency is not None:
            if spec.currency_display == "name":
                shown = _wrap(pattern, core, sign, symbols)
                unit_pattern = get_currency_unit_pattern(
                    spec.currency, count=operand, locale=locale
                )
                name = get_currency_name(spec.currency, count=operand, locale=locale)
                return unit_pattern.replace("{0}", shown).replace("{1}", name)
            return _wrap(
                pattern,
                core,
                sign,
                symbols,
                _currency_text(locale, spec.currency, spec.currency_display),
            )
        shown = _wrap(pattern, core, sign, symbols)
        return shown if _percent_unit(spec) else _with_unit(locale, spec, shown, operand)
