"""``Intl.PluralRules`` over the supplier's plural rules."""

from __future__ import annotations

import decimal
from decimal import Decimal

from .digits import PRECISION, format_numeric, read_digits, selection_operand
from .locale import babel_locale
from .validate import LOCALE_MATCHERS, Options, get_string


def select(tag: object, options: Options, value: float) -> str:
    """``new Intl.PluralRules(tag, options).select(value)``.

    The category is asked of the number as the digit options show it, so
    ``1`` shown as ``1.0`` takes the category ``1.0`` does.
    """
    get_string(options, "localeMatcher", LOCALE_MATCHERS, "best fit")
    kind = get_string(options, "type", ("cardinal", "ordinal"), "cardinal")
    digits = read_digits(options, 0, 3, "standard")
    locale, _ = babel_locale(tag)
    with decimal.localcontext(PRECISION):
        operand = selection_operand(format_numeric(digits, Decimal(repr(float(value)))))
        rule = locale.ordinal_form if kind == "ordinal" else locale.plural_form
        return str(rule(operand))
