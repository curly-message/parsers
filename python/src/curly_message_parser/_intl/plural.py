"""``Intl.PluralRules`` over the supplier's plural rules."""

from __future__ import annotations

import decimal
from decimal import Decimal

from .digits import PRECISION, Digits, format_numeric, read_digits, selection_operand
from .locale import babel_locale
from .validate import LOCALE_MATCHERS, Options, get_string


class PluralSpec:
    """A ``PluralRules`` request, read as ECMA-402's constructor reads it."""

    __slots__ = ("digits", "kind")

    def __init__(self, options: Options) -> None:
        get_string(options, "localeMatcher", LOCALE_MATCHERS, "best fit")
        self.kind = get_string(options, "type", ("cardinal", "ordinal"), "cardinal")
        self.digits: Digits = read_digits(options, 0, 3, "standard")


def select(tag: object, spec: PluralSpec, value: float) -> str:
    """``new Intl.PluralRules(tag, options).select(value)``, the options read into ``spec``.

    The category is asked of the number as the digit options show it, so
    ``1`` shown as ``1.0`` takes the category ``1.0`` does.
    """
    locale, _ = babel_locale(tag)
    with decimal.localcontext(PRECISION):
        operand = selection_operand(format_numeric(spec.digits, Decimal(repr(float(value)))))
        rule = locale.ordinal_form if spec.kind == "ordinal" else locale.plural_form
        return str(rule(operand))
