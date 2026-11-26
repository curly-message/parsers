"""The Intl modifiers (sections 11.2 and 11.5), driven through ``create_parser(...).resolve``.

The texts are this package's renderings, each checked once against what
Node 22.22.0 (ICU 77.1, CLDR 47) wrote for the request the placeholder
makes. They claim no parity with every release of Node or ICU, whose data
moves: a few already differ on another release of Node 22. A text the
supplier's data writes otherwise is left out here and stated in the README
as a reading of section 11.2.
"""

from __future__ import annotations

import copy
import importlib
import importlib.metadata
import os
import subprocess
import sys
import time
from collections.abc import Callable
from pathlib import Path
from typing import Any
from zoneinfo import ZoneInfo

import pytest

from curly_message import Report, create_parser
from curly_message._requests import UNEXPRESSIBLE, record


@pytest.fixture(autouse=True)
def _supplier(expect_intl: bool) -> None:
    if not expect_intl:
        pytest.skip("this installation is not meant to carry the Intl supplier")
    # Imported here rather than through a modifier, which would contain a
    # failure to import as a failed modifier.
    importlib.import_module("curly_message._intl")


def resolve(
    message: str, payload: dict[str, Any], locale: object = "en", **props: dict[str, Any]
) -> tuple[str, list[Report]]:
    reports: list[Report] = []
    output = create_parser(on_report=reports.append).resolve(
        message, payload, props=props, locale=locale
    )
    return output, reports


def codes(reports: list[Report]) -> list[str]:
    return [report.code for report in reports]


UTC = {"timeZone": "UTC"}

MARCH_5 = "1709650800000"
"""2024-03-05T15:00:00Z, a Tuesday afternoon."""

TEXTS = [
    ("number", "1234.5", {}, "en", "1,234.5"),
    ("number", "1234.5", {}, "de", "1.234,5"),
    ("number", "1234.5", {}, "cs", "1\u00a0234,5"),
    ("number", "1234.5", {}, "fr", "1\u202f234,5"),
    ("number", "-1234.5678", {"maximumFractionDigits": 3}, "en", "-1,234.568"),
    ("number", "0.125", {"style": "percent"}, "de", "12,5\u00a0%"),
    ("number", "0.125", {"style": "percent", "maximumFractionDigits": 1}, "fr", "12,5\u00a0%"),
    ("number", "1.005", {"maximumFractionDigits": 2}, "en", "1.01"),
    ("number", "2.5", {"maximumFractionDigits": 0, "roundingMode": "halfEven"}, "en", "2"),
    ("number", "-2.5", {"maximumFractionDigits": 0, "roundingMode": "floor"}, "en", "-3"),
    ("number", "1e28", {}, "en", "10,000,000,000,000,000,000,000,000,000"),
    ("number", "-0", {"signDisplay": "negative"}, "en", "0"),
    ("number", "-0", {}, "en", "-0"),
    ("number", "5", {"signDisplay": "always"}, "en", "+5"),
    ("number", "5", {"minimumIntegerDigits": 3}, "en", "005"),
    ("number", "\u30005\ufeff", {}, "en", "5"),
    ("number", "1.255", {"maximumFractionDigits": "\u30001\ufeff"}, "en", "1.3"),
    ("number", "12345.678", {"useGrouping": False}, "en", "12345.68"),
    ("number", "1234567", {"notation": "compact"}, "en", "1.23M"),
    (
        "number",
        "1234567",
        {"notation": "compact", "compactDisplay": "long"},
        "de",
        "1,23 Millionen",
    ),
    ("number", "1234567", {"notation": "compact", "compactDisplay": "long"}, "ja", "123.46\u4e07"),
    ("number", "1234567", {"notation": "scientific"}, "en", "1.23E6"),
    ("number", "1234567", {"notation": "engineering"}, "fr", "1,23E6"),
    ("number", "3", {"style": "unit", "unit": "meter", "unitDisplay": "long"}, "de", "3 Meter"),
    ("number", "1", {"style": "unit", "unit": "meter", "unitDisplay": "long"}, "de", "1 Meter"),
    # A length that holds neither the category nor `other`, but `per` alone
    # or nothing, gives the unit's pattern to the next length.
    (
        "number",
        "5",
        {"style": "unit", "unit": "hour", "unitDisplay": "long"},
        "ko",
        "5\uc2dc\uac04",
    ),
    (
        "number",
        "999999",
        {"style": "unit", "unit": "byte", "unitDisplay": "long"},
        "it",
        "999.999 byte",
    ),
    ("number", "5", {"style": "unit", "unit": "hour", "unitDisplay": "long"}, "en", "5 hours"),
    (
        "number",
        "5",
        {"style": "unit", "unit": "petabyte", "unitDisplay": "long"},
        "ast",
        "5 PB",
    ),
    (
        "number",
        "5",
        {"style": "unit", "unit": "hour", "unitDisplay": "narrow"},
        "ko",
        "5\uc2dc\uac04",
    ),
    # A `per` the long length lacks is the next length's, not one built from
    # the two units; English's long length holds its own.
    (
        "number",
        "5",
        {"style": "unit", "unit": "kilometer-per-day", "unitDisplay": "long"},
        "hu",
        "5 kilom\u00e9ter/nap",
    ),
    (
        "number",
        "5",
        {"style": "unit", "unit": "kilometer-per-day", "unitDisplay": "long"},
        "en",
        "5 kilometers per day",
    ),
    ("number", "3", {"style": "unit", "unit": "kilometer-per-hour"}, "en", "3 km/h"),
    (
        "number",
        "3",
        {"style": "unit", "unit": "kilometer-per-hour", "unitDisplay": "narrow"},
        "cs",
        "3 km/h",
    ),
    ("number", "50", {"style": "unit", "unit": "percent"}, "de", "50\u00a0%"),
    (
        "number",
        "50",
        {"style": "unit", "unit": "percent", "unitDisplay": "long"},
        "en",
        "50 percent",
    ),
    # ECMA-402 reads `unit` only for the unit style; any other style
    # validates it and leaves it out.
    ("number", "12.5", {"unit": "kilogram"}, "en", "12.5"),
    ("number", "12.5", {"unit": "percent"}, "en", "12.5"),
    ("number", "12.5", {"unit": "kilogram", "notation": "compact"}, "en", "12.5"),
    ("number", "12.5", {"style": "percent", "unit": "kilogram"}, "en", "1,250%"),
    ("number", "3", {"unit": "kilometer-per-hour", "unitDisplay": "long"}, "cs", "3"),
    ("currency", "12.5", {"currency": "USD", "unit": "kilogram"}, "en", "$12.50"),
    ("number", "1234.5", {}, "zh-TW", "1,234.5"),
    ("number", "1234.5", {}, "iw", "1,234.5"),
    ("number", "1234.5", {}, "und", "1,234.5"),
    ("currency", "12.5", {"currency": "USD"}, "en", "$12.50"),
    ("currency", "12.5", {"currency": "usd"}, "de", "12,50\u00a0$"),
    ("currency", "12.5", {"currency": "EUR", "currencyDisplay": "name"}, "en", "12.50 euros"),
    ("currency", "12.5", {"currency": "EUR"}, "en", "\u20ac12.50"),
    ("currency", "12.5", {"currency": "EUR", "currencyDisplay": "code"}, "en", "EUR12.50"),
    (
        "currency",
        "1",
        {"currency": "CZK", "currencyDisplay": "name"},
        "cs",
        "1,00 \u010desk\u00e9 koruny",
    ),
    ("currency", "1250", {"currency": "JPY"}, "ja", "\uffe51,250"),
    ("currency", "1250", {"currency": "USD", "ratio": 0.01}, "en", "$12.50"),
    ("currency", "-5", {"currency": "USD", "currencySign": "accounting"}, "en", "($5.00)"),
    ("date", "0", {"timeZone": "UTC"}, "en", "1/1/1970"),
    ("date", "0\u3000", {"timeZone": "UTC"}, "en", "1/1/1970"),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "dateStyle": "full"},
        "en",
        "Tuesday, March 5, 2024",
    ),
    ("date", "1709650800000", {"timeZone": "UTC", "dateStyle": "long"}, "de", "5. M\u00e4rz 2024"),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "dateStyle": "medium", "timeStyle": "short"},
        "en",
        "Mar 5, 2024, 3:00 PM",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "dateStyle": "short", "timeStyle": "medium"},
        "fr",
        "05/03/2024 15:00:00",
    ),
    ("date", "1709650800000", {"timeZone": "UTC", "timeStyle": "short"}, "en-GB", "15:00"),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "timeStyle": "short", "hour12": True},
        "de",
        "03:00 PM",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "timeStyle": "medium"},
        "de-u-hc-h12",
        "3:00:00 PM",
    ),
    (
        "date",
        "1709650800000",
        {
            "timeZone": "UTC",
            "weekday": "long",
            "year": "numeric",
            "month": "long",
            "day": "numeric",
        },
        "fr",
        "mardi 5 mars 2024",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "weekday": "short", "month": "long", "day": "numeric"},
        "ja",
        "3\u67085\u65e5(\u706b)",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "weekday": "short", "month": "long", "day": "numeric"},
        "ru",
        "\u0432\u0442, 5 \u043c\u0430\u0440\u0442\u0430",
    ),
    ("date", "1709650800000", {"timeZone": "UTC", "month": "long"}, "es", "marzo"),
    ("date", "1709650800000", {"timeZone": "UTC", "month": "long"}, "ja", "3\u6708"),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "year": "2-digit", "month": "2-digit", "day": "2-digit"},
        "en",
        "03/05/24",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "hour": "numeric", "minute": "2-digit"},
        "en-GB",
        "15:00",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "hour": "numeric", "minute": "numeric"},
        "en-GB",
        "15:00",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "hour": "2-digit", "minute": "2-digit"},
        "en",
        "03:00 PM",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "hour": "numeric", "minute": "2-digit"},
        "zh-TW",
        "\u4e0b\u53483:00",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "hour": "numeric", "minute": "2-digit"},
        "ko",
        "\uc624\ud6c4 3:00",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "hour": "numeric", "hourCycle": "h11"},
        "ja",
        "\u5348\u5f8c3\u6642",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "hour": "numeric", "hour12": True},
        "de",
        "3 Uhr PM",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "hour": "numeric", "minute": "2-digit", "hour12": True},
        "ja-u-hc-h11",
        "\u5348\u5f8c3:00",
    ),
    ("date", "1709650800000", {"timeZone": "UTC", "hour": "numeric"}, "en-GB-u-hc-h12", "3 pm"),
    ("date", "1709650800000", {"timeZone": "UTC", "hour": "numeric"}, "en-GB-u-hc-h12-abc", "15"),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "hour": "numeric"},
        "en-GB-u-hc-h12-hc-h23",
        "3 pm",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "hour": "numeric"},
        "en-GB-u-hc-h12-abc-hc-h23",
        "15",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "hour": "numeric", "dayPeriod": "long"},
        "en",
        "3 in the afternoon",
    ),
    (
        "date",
        "0",
        {"timeZone": "UTC", "hour": "numeric", "dayPeriod": "short"},
        "de-u-hc-h12",
        "12 nachts",
    ),
    (
        "date",
        "0",
        {"timeZone": "UTC", "hour": "numeric", "dayPeriod": "short"},
        "zh-TW",
        "\u51cc\u666812\u6642",
    ),
    ("date", "1709650800000", {"timeZone": "UTC", "second": "2-digit"}, "en", "0"),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "era": "short", "year": "numeric"},
        "en",
        "2024 AD",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "Asia/Kolkata", "hour": "numeric", "minute": "numeric"},
        "en",
        "8:30 PM",
    ),
    ("date", "1709650800000", {"timeZone": "+05:30", "timeStyle": "short"}, "en", "8:30 PM"),
    ("date", "1709650800000", {"timeZone": "europe/prague", "timeStyle": "short"}, "en", "4:00 PM"),
    (
        "date",
        "1709650800000",
        {"timeZone": "Europe/Prague", "timeStyle": "long"},
        "en",
        "4:00:00 PM GMT+1",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "Asia/Kolkata", "timeStyle": "long"},
        "en",
        "8:30:00 PM GMT+5:30",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "America/St_Johns", "timeStyle": "long"},
        "en",
        "11:30:00 AM GMT-3:30",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "Asia/Tokyo", "timeStyle": "long"},
        "fr",
        "00:00:00 UTC+9",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "Europe/London", "timeStyle": "long"},
        "ja",
        "15:00:00 GMT",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "Asia/Tokyo", "timeStyle": "long"},
        "ar",
        "12:00:00 \u0635 \u063a\u0631\u064a\u0646\u062a\u0634+9",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "+05:30", "timeStyle": "full"},
        "en",
        "8:30:00 PM GMT+05:30",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "-03:30", "timeStyle": "full"},
        "en",
        "11:30:00 AM GMT-03:30",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "+00:00", "timeStyle": "full"},
        "en",
        "3:00:00 PM Greenwich Mean Time",
    ),
    ("date", "1709650800000", {"timeZone": "-0000", "timeStyle": "long"}, "en", "3:00:00 PM GMT"),
    ("date", "-3000000000000", {"timeZone": "+00", "timeStyle": "full"}, "en", "6:40:00 PM GMT"),
    (
        "date",
        "1709650800000",
        {"timeZone": "Europe/London", "timeStyle": "full"},
        "en",
        "3:00:00 PM Greenwich Mean Time",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "America/New_York", "timeStyle": "long"},
        "en",
        "10:00:00 AM EST",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "America/New_York", "timeStyle": "long"},
        "en-GB",
        "10:00:00 GMT-5",
    ),
    (
        "date",
        "1720000000000",
        {"timeZone": "America/New_York", "timeStyle": "long"},
        "en-GB",
        "05:46:40 GMT-4",
    ),
    (
        "date",
        "1720000000000",
        {"timeZone": "America/New_York", "timeStyle": "full"},
        "en-GB",
        "05:46:40 Eastern Daylight Time",
    ),
    (
        "date",
        "1720000000000",
        {"timeZone": "Europe/London", "timeStyle": "long"},
        "en-GB",
        "10:46:40 BST",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "Europe/Berlin", "timeStyle": "long"},
        "pt-GW",
        "16:00:00 GMT+1",
    ),
    (
        "date",
        "1720000000000",
        {"timeZone": "Europe/Berlin", "timeStyle": "long"},
        "pt-GW",
        "11:46:40 GMT+2",
    ),
    (
        "date",
        "1720000000000",
        {"timeZone": "Europe/Berlin", "timeStyle": "full"},
        "pt-GW",
        "11:46:40 Hora de ver\u00e3o da Europa Central",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "America/Los_Angeles", "timeStyle": "long"},
        "en-IN",
        "7:00:00 am GMT-8",
    ),
    (
        "date",
        MARCH_5,
        {"timeZone": "GMT", "timeStyle": "full"},
        "en",
        "3:00:00 PM Coordinated Universal Time",
    ),
    ("date", MARCH_5, {"timeZone": "gmt0", "timeStyle": "long"}, "en", "3:00:00 PM UTC"),
    ("date", MARCH_5, {"timeZone": "GMT+0", "timeStyle": "long"}, "en", "3:00:00 PM UTC"),
    ("date", MARCH_5, {"timeZone": "GMT-0", "timeStyle": "long"}, "en", "3:00:00 PM UTC"),
    (
        "date",
        MARCH_5,
        {"timeZone": "Etc/GMT0", "timeStyle": "full"},
        "en",
        "3:00:00 PM Greenwich Mean Time",
    ),
    ("date", MARCH_5, {"timeZone": "Greenwich", "timeStyle": "long"}, "en", "3:00:00 PM GMT"),
    (
        "date",
        "-3000000000000",
        {"timeZone": "America/New_York", "timeStyle": "long"},
        "en",
        "1:43:58 PM GMT-4:56:02",
    ),
    (
        "date",
        "-3000000000000",
        {"timeZone": "Europe/Prague", "timeStyle": "long"},
        "de",
        "19:37:44 GMT+0:57:44",
    ),
    (
        "date",
        "-3000000000000",
        {"timeZone": "America/St_Johns", "timeStyle": "full"},
        "en",
        "3:09:08 PM GMT-03:30:52",
    ),
    (
        "date",
        "-631152000000",
        {"timeZone": "America/New_York", "timeStyle": "long"},
        "en",
        "7:00:00 PM GMT-5",
    ),
    (
        "date",
        "-631152000000",
        {"timeZone": "Asia/Tokyo", "timeStyle": "full"},
        "en",
        "9:00:00 AM GMT+09:00",
    ),
    (
        "date",
        "-1",
        {"timeZone": "America/New_York", "timeStyle": "full"},
        "en",
        "6:59:59 PM GMT-05:00",
    ),
    (
        "date",
        "0",
        {"timeZone": "America/New_York", "timeStyle": "full"},
        "en",
        "7:00:00 PM Eastern Standard Time",
    ),
    (
        "date",
        "530000000000",
        {"timeZone": "America/Sao_Paulo", "timeStyle": "full"},
        "en",
        "3:13:20 AM Brasilia Standard Time",
    ),
    (
        "date",
        "-3000000000000",
        {"timeZone": "UTC", "timeStyle": "full"},
        "en",
        "6:40:00 PM Coordinated Universal Time",
    ),
    (
        "date",
        "-3000000000000",
        {"timeZone": "Zulu", "timeStyle": "full"},
        "en",
        "6:40:00 PM Coordinated Universal Time",
    ),
    (
        "date",
        "-3000000000000",
        {"timeZone": "Universal", "timeStyle": "long"},
        "en",
        "6:40:00 PM UTC",
    ),
    (
        "date",
        "-1000000000000",
        {"timeZone": "Etc/UCT", "timeStyle": "full"},
        "fr",
        "22:13:20 temps universel coordonn\u00e9",
    ),
    (
        "date",
        "-1000000000000",
        {"timeZone": "Pacific/Honolulu", "timeStyle": "long"},
        "en",
        "11:43:20 AM HST",
    ),
    (
        "date",
        "-1000000000000",
        {"timeZone": "Europe/London", "timeStyle": "full"},
        "en",
        "11:13:20 PM British Summer Time",
    ),
    (
        "date",
        "-200000000000",
        {"timeZone": "Europe/Dublin", "timeStyle": "full"},
        "de",
        "05:26:40 Irische Sommerzeit",
    ),
    (
        "date",
        "-200000000000",
        {"timeZone": "Pacific/Honolulu", "timeStyle": "full"},
        "en",
        "6:26:40 PM GMT-10:00",
    ),
    (
        "date",
        "2024-03-05T15:00:00Z",
        {"timeZone": "UTC", "dateStyle": "medium", "timeStyle": "short"},
        "en",
        "Mar 5, 2024, 3:00 PM",
    ),
    ("ago", "-90000", {}, "en", "2 minutes ago"),
    ("ago", "90000", {}, "en", "in 2 minutes"),
    ("ago", "-86400000", {"numeric": "always"}, "de", "vor 1 Tag"),
    ("ago", "-5000", {"style": "narrow"}, "cs", "p\u0159ed 5 s"),
    ("ago", "60000", {"format": "second"}, "en", "in 60 seconds"),
    ("ago", "7200000", {"format": "minutes", "style": "short"}, "fr", "dans 120\u00a0min"),
    ("number", "1e300", {"maximumSignificantDigits": 3}, "en", "1" + ",000" * 100),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "dateStyle": "medium", "timeStyle": "short", "hourCycle": "h24"},
        "zh-TW",
        "2024/3/5 15:00",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "timeStyle": "short", "hour12": True},
        "en-u-hc-h23",
        "03:00 PM",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "timeStyle": "short", "hour12": False},
        "en",
        "15:00",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "timeStyle": "short", "hour12": True},
        "en-GB",
        "03:00 pm",
    ),
    (
        "date",
        "1720000000000",
        {"timeZone": "America/Indiana/Indianapolis", "timeStyle": "long"},
        "en",
        "5:46:40 AM EDT",
    ),
    (
        "date",
        "1720000000000",
        {"timeZone": "America/Indiana/Indianapolis", "timeStyle": "full"},
        "en",
        "5:46:40 AM Eastern Daylight Time",
    ),
    (
        "date",
        "1720000000000",
        {"timeZone": "America/Fort_Wayne", "timeStyle": "full"},
        "en",
        "5:46:40 AM Eastern Daylight Time",
    ),
    (
        "date",
        "1720000000000",
        {"timeZone": "Europe/Kyiv", "timeStyle": "long"},
        "en-GB",
        "12:46:40 EEST",
    ),
    (
        "date",
        "1720000000000",
        {"timeZone": "Europe/Kiev", "timeStyle": "full"},
        "en",
        "12:46:40 PM Eastern European Summer Time",
    ),
    (
        "date",
        "1720000000000",
        {"timeZone": "Asia/Kolkata", "timeStyle": "long"},
        "en-IN",
        "3:16:40 pm IST",
    ),
    (
        "date",
        "1709650800000",
        {"timeZone": "UTC", "month": "short", "day": "numeric", "weekday": "short"},
        "ja",
        "3\u67085\u65e5(\u706b)",
    ),
    ("date", "1709650800000", {"timeZone": "UTC", "weekday": "short"}, "ru", "\u0432\u0442"),
]


@pytest.mark.parametrize(("modifier", "value", "props", "locale", "text"), TEXTS)
def test_formats_as_ecma_402_does(
    modifier: str, value: str, props: dict[str, Any], locale: str, text: str
) -> None:
    assert resolve("{{v:" + modifier + "}}", {"v": value}, locale, **{modifier: props}) == (
        text,
        [],
    )


def test_one_locale_reading_its_data_leaves_another_locale_alone() -> None:
    # The supplier's own locale objects share the dicts an alias resolves
    # into, so ``he`` read first once gave ``es`` its stand-alone months. A
    # fresh process, since either may have been read already.
    probe = """
from curly_message import create_parser
parser = create_parser()
props = {"date": {"timeZone": "UTC", "month": "long"}}
print(ascii([parser.resolve("{{v:date}}", {"v": "0"}, props=props, locale=tag)
             for tag in ("he", "es", "ja")]))
"""
    output = subprocess.run(
        [sys.executable, "-c", probe], capture_output=True, text=True, check=True
    ).stdout
    assert output.strip() == ascii(["\u05d9\u05e0\u05d5\u05d0\u05e8", "enero", "1\u6708"])


def test_a_locale_read_through_the_supplier_first_leaves_the_package_s_alone() -> None:
    # Reading ``he`` through the supplier's own locale objects writes what
    # its alias resolves to into data ``es`` shares, so the supplier's ``es``
    # then holds Hebrew stand-alone months; the package reads its locales'
    # data apart from those objects.
    probe = """
import babel
babel.Locale.parse("he").months["stand-alone"]["wide"][1]
from curly_message import create_parser
props = {"date": {"timeZone": "UTC", "month": "long"}}
print(ascii(create_parser().resolve("{{v:date}}", {"v": "0"}, props=props, locale="es")))
"""
    output = subprocess.run(
        [sys.executable, "-c", probe], capture_output=True, text=True, check=True
    ).stdout
    assert output.strip() == ascii("enero")


@pytest.mark.parametrize(
    ("message", "payload", "props"),
    [
        ("{{v:currency}}", {"v": "1"}, {}),
        ("{{v:currency}}", {"v": "1"}, {"currency": {"currency": ""}}),
        ("{{v:currency}}", {"v": "1"}, {"currency": {"currency": "US"}}),
        (
            "{{v:number}}",
            {"v": "1"},
            {"number": {"minimumFractionDigits": 3, "maximumFractionDigits": 1}},
        ),
        ("{{v:number}}", {"v": "1"}, {"number": {"style": "unit", "unit": "furlong"}}),
        ("{{v:number}}", {"v": "1"}, {"number": {"unit": "furlong"}}),
        ("{{v:number}}", {"v": "1"}, {"number": {"roundingIncrement": 3}}),
        ("{{v:number}}", {"v": "one"}, {}),
        ("{{v:number}}", {"v": "\x855"}, {}),
        ("{{v:number}}", {"v": "1"}, {"number": {"maximumFractionDigits": "\x852"}}),
        ("{{v:date}}", {"v": "not a date"}, {}),
        ("{{v:date}}", {"v": "0\x85"}, {}),
        ("{{v:date}}", {"v": "0"}, {"date": {"timeZone": "Mars/Olympus"}}),
        ("{{v:ago}}", {"v": "1"}, {"ago": {"format": "fortnight"}}),
        ("{{v:ordinal; one: st; other: th}}", {"v": "1.5"}, {}),
    ],
)
def test_a_request_ecma_402_refuses_fails_the_modifier(
    message: str, payload: dict[str, Any], props: dict[str, Any]
) -> None:
    output, reports = resolve(message, payload, **props)
    assert (output, codes(reports)) == ("", ["failed-modifier"])


@pytest.mark.parametrize("locale", [None, ""])
def test_without_a_locale_nothing_is_formatted(locale: object) -> None:
    output, reports = resolve("{{v:number}} {{v:plural; other: x}}", {"v": "1"}, locale)
    assert (output, codes(reports)) == (" ", ["missing-locale", "missing-locale"])


@pytest.mark.parametrize(
    "locale",
    [
        "en_US",
        "e",
        "en--US",
        5,
        b"en",
        "en-u-c1",
        "en-u-c1-gregory",
        "en-t-a1",
        "en-t-en-a1",
        "en-t-a1-abc-b2",
        "en-t-en-abc",
        "en-t-root",
        "en-t-en-fonipa-fonipa",
        "ab-t-1996",
    ],
)
def test_a_locale_that_is_no_language_tag_fails_the_modifier(locale: object) -> None:
    output, reports = resolve("{{v:number}}", {"v": "1"}, locale)
    assert (output, codes(reports)) == ("", ["failed-modifier"])


@pytest.mark.parametrize(
    "locale",
    ["en-u-1a-abc", "en-u-abc-ca-gregory-def", "en-t-a1-abc", "en-t-en", "en-t-en-us-1996-a1-abc"],
)
def test_an_extension_as_its_grammar_reads_it_is_a_language_tag(locale: str) -> None:
    assert resolve("{{v:number}}", {"v": "1234.5"}, locale) == ("1,234.5", [])


def test_a_tag_the_supplier_has_no_data_for_is_looked_up_by_truncation() -> None:
    assert resolve("{{v:number}}", {"v": "1234.5"}, "de-CH-x-private")[0] == "1\u2019234.5"
    output, reports = resolve("{{v:number}}", {"v": "1234.5"}, "zz")
    assert (output, codes(reports)) == ("", ["failed-modifier"])


def test_the_locale_lookup_keeps_what_the_supplier_reads_and_not_the_tag() -> None:
    locale = importlib.import_module("curly_message._intl.locale")

    locale._lookup.cache_clear()
    for index in range(300):
        tag = "de-CH-x-" + "abcdefgh-" * 1000 + str(index)
        assert resolve("{{v:number}}", {"v": "1234.5"}, tag)[0] == "1\u2019234.5"
    for index in range(300):
        output, reports = resolve("{{v:number}}", {"v": "1"}, "de-CH-" + "-" * index)
        assert (output, codes(reports)) == ("", ["failed-modifier"])
    assert locale._lookup.cache_info().currsize == 1


def test_a_short_tag_is_parsed_once_and_a_long_one_on_every_call(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    locale = importlib.import_module("curly_message._intl.locale")
    parsed: list[int] = []
    real = locale.parse_tag

    def parse_tag(tag: str) -> object:
        parsed.append(len(tag))
        return real(tag)

    monkeypatch.setattr(locale, "parse_tag", parse_tag)
    message = "{{v:number}} {{v:ago}} {{v:plural; one:A; other:B}} {{v:date}}" * 10
    short = "ja-JP-u-ca-japanese-hc-h23-x-parsed-once"
    output, reports = resolve(message, {"v": "1"}, short)
    assert (len(parsed), reports) == (1, [])
    assert output == resolve(message, {"v": "1"}, short)[0]
    assert len(parsed) == 1
    # A tag past the bound is not held, so a caller's long tags cannot fill
    # what the process keeps; reading it again costs a parse per call.
    long = "ja-JP-x-" + "-".join(["abcdefgh"] * 8)
    assert len(long) > 64
    assert resolve("{{v:number}} {{v:number}}", {"v": "1"}, long) == ("1 1", [])
    assert parsed[1:] == [len(long), len(long)]


def test_variants_past_the_one_the_supplier_holds_are_not_tried(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    locale = importlib.import_module("curly_message._intl.locale")

    tried: list[object] = []
    parse = locale._parse

    def counted(parts: list[str]) -> object:
        tried.append(parts)
        return parse(parts)

    monkeypatch.setattr(locale, "_parse", counted)
    locale._lookup.cache_clear()
    tag = "ca-ES-valencia-" + "-".join(f"v{index:07d}" for index in range(1000))
    assert resolve("{{v:number}}", {"v": "1234.5"}, tag)[0] == "1.234,5"
    assert len(tried) == 1
    output, reports = resolve("{{v:number}}", {"v": "1"}, tag + "-v0000999")
    assert (output, codes(reports)) == ("", ["failed-modifier"])


@pytest.mark.parametrize(
    ("modifier", "props"),
    [
        ("number", {"number": {"minimumFractionDigits": "3"}}),
        ("number", {"number": {"minimumFractionDigits": "300"}}),
        ("plural; one:A; other:B", {"number": {"minimumFractionDigits": "3"}}),
        ("ordinal; two:A; other:B", {"ordinal": {"minimumIntegerDigits": "3"}}),
        ("currency", {"currency": {"currency": "USD", "ratio": "3"}}),
        ("currency", {"currency": {"currency": "USD", "ratio": "3", "maximumFractionDigits": "x"}}),
        ("date", {"date": {"second": "numeric", "fractionalSecondDigits": "3"}}),
    ],
)
def test_the_props_of_a_request_are_read_once_for_the_placeholders_they_compose_for(
    modifier: str, props: dict[str, Any], monkeypatch: pytest.MonkeyPatch
) -> None:
    validate = importlib.import_module("curly_message._intl.validate")
    read: list[str] = []
    convert: Callable[[str], float] = validate._string_to_number

    def spy(text: str) -> float:
        read.append(text)
        return convert(text)

    monkeypatch.setattr(validate, "_string_to_number", spy)

    def resolved(message: str, payload: dict[str, Any], **layers: Any) -> tuple[str, int, int]:
        read.clear()
        output, reports = resolve(message, payload, **layers)
        return output, len(reports), len(read)

    placeholder = f"{{{{v:{modifier}}}}}"
    output, reported, once = resolved(placeholder, {"v": 2}, **props)
    assert once
    # A refused request is refused at each placeholder, from what was read once.
    assert resolved(placeholder * 3, {"v": 2}, **props) == (output * 3, reported * 3, once)
    # Props that differ from one placeholder to the next are read for each.
    wrapped = {name: {"value": 2, "props": copy.deepcopy(props)} for name in ("a", "b", "c")}
    message = "".join(placeholder.replace("v:", f"{name}:") for name in wrapped)
    assert resolved(message, wrapped) == (output * 3, reported * 3, once * 3)
    # A wrapper that composes nothing for the modifier hands on what lies
    # beneath it, read once with it.
    empty = {name: {"value": 2, "props": {"other": {"k": "3"}}} for name in wrapped}
    assert resolved(message, empty, **props) == (output * 3, reported * 3, once)


def requests(message: str, payload: dict[str, Any], **props: dict[str, Any]) -> list[Any]:
    with record() as made:
        resolve(message, payload, **props)
    return made


def test_number_shows_two_fraction_digits_at_most_where_no_layer_names_a_maximum() -> None:
    assert requests("{{v:number}}", {"v": "5"}) == [
        {"api": "NumberFormat", "options": {"maximumFractionDigits": 2}, "input": 5}
    ]
    assert requests("{{v:number}}", {"v": "5"}, number={"minimumFractionDigits": 4}) == [
        {
            "api": "NumberFormat",
            "options": {"minimumFractionDigits": 4, "maximumFractionDigits": 4},
            "input": 5,
        }
    ]


def test_currency_pins_its_style_and_scales_the_value_by_its_ratio() -> None:
    props = {"style": "percent", "currency": "EUR", "ratio": 0.01}
    assert requests("{{v:currency}}", {"v": "1250"}, currency=props) == [
        {"api": "NumberFormat", "options": {"currency": "EUR", "style": "currency"}, "input": 12.5}
    ]


def test_date_requests_the_time_a_value_names() -> None:
    assert requests("{{v:date}}", {"v": "2024-03-05T15:00:00Z"}, date=UTC) == [
        {"api": "DateTimeFormat", "options": UTC, "input": 1709650800000}
    ]


def test_ago_requests_the_unit_it_climbed_to_and_the_count_in_it() -> None:
    assert requests("{{v:ago}}", {"v": "-90000"}) == [
        {"api": "RelativeTimeFormat", "options": {"numeric": "auto"}, "input": [-2, "minute"]}
    ]


def test_a_selection_records_the_category_the_supplier_answered() -> None:
    assert requests("{{v:plural; one: a; other: b}}", {"v": "1"}) == [
        {
            "api": "PluralRules",
            "options": {"maximumFractionDigits": 2, "type": "cardinal"},
            "input": 1,
            "category": "one",
        }
    ]
    assert requests("{{v:ordinal; two: nd; other: th}}", {"v": "2"}) == [
        {"api": "PluralRules", "options": {"type": "ordinal"}, "input": 2, "category": "two"}
    ]


def test_an_exact_match_selects_without_asking_the_supplier() -> None:
    with record() as made:
        output, _ = resolve("{{v:plural; 1: exactly; one: a}}", {"v": "1"})
    assert (output, made) == ("exactly", [])


def test_an_unexpressible_property_is_requested_and_left_out_of_the_text() -> None:
    assert "numberingSystem" in UNEXPRESSIBLE["NumberFormat"]
    props = {"numberingSystem": "arab"}
    with record() as made:
        output, reports = resolve("{{v:number}}", {"v": "12"}, number=props)
    assert (output, reports) == ("12", [])
    assert made == [
        {
            "api": "NumberFormat",
            "options": {"numberingSystem": "arab", "maximumFractionDigits": 2},
            "input": 12,
        }
    ]


@pytest.mark.parametrize(
    ("message", "value", "locale", "props", "text"),
    [
        ("{{v:plural; one: one; many: many; other: other}}", "1", "cs", {}, "one"),
        (
            "{{v:plural; one: one; many: many; other: other}}",
            "1",
            "cs",
            {"plural": {"minimumFractionDigits": 1}},
            "many",
        ),
        (
            "{{v:plural; one: one; many: many; other: other}}",
            "1",
            "cs",
            {"number": {"minimumFractionDigits": 1}},
            "many",
        ),
        ("{{v:plural; one: one; few: few; other: other}}", "21", "ru", {}, "one"),
        ("{{v:plural; one: one; few: few; other: other}}", "1.5", "ru", {}, "other"),
        ("{{v:ordinal; one: st; two: nd; few: rd; other: th}}", "2", "en", {}, "nd"),
        ("{{v:ordinal; one: st; two: nd; few: rd; other: th}}", "13", "en", {}, "th"),
        ("{{v:ordinal; one: st; other: th}}", "3", "en", {}, ""),
    ],
)
def test_a_selection_takes_the_option_under_the_category(
    message: str, value: str, locale: str, props: dict[str, Any], text: str
) -> None:
    assert resolve(message, {"v": value}, locale, **props) == (text, [])


def test_plural_reads_only_the_digits_the_number_layers_name() -> None:
    props = {"number": {"style": "percent", "minimumFractionDigits": 1}}
    assert requests("{{v:plural; other: x}}", {"v": "1"}, **props)[0]["options"] == {
        "minimumFractionDigits": 1,
        "maximumFractionDigits": 2,
        "type": "cardinal",
    }


def test_a_value_without_a_zone_is_read_and_shown_in_the_local_zone(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    date = importlib.import_module("curly_message._intl.date")
    monkeypatch.setattr(date, "local_zone", lambda: ZoneInfo("Europe/Prague"))
    style = {"dateStyle": "short", "timeStyle": "short"}
    # A local time the spring transition skips is read at the offset before
    # it, and one the autumn transition repeats as its first occurrence.
    for local, instant, text in (
        ("2024-03-31T02:30:00", 1711848600000, "3/31/24, 3:30 AM"),
        ("2024-10-27T02:30:00", 1729989000000, "10/27/24, 2:30 AM"),
    ):
        with record() as made:
            output, reports = resolve("{{v:date}}", {"v": local}, date=style)
        assert made[0]["input"] == instant
        assert (output, reports) == (text, [])


@pytest.mark.parametrize(
    ("zone", "text"),
    [
        ("Etc/UTC", "6:40:00 PM Coordinated Universal Time"),
        ("UTC", "6:40:00 PM Coordinated Universal Time"),
        ("America/New_York", "1:43:58 PM GMT-04:56:02"),
    ],
)
def test_a_local_zone_read_as_utc_is_named_so_before_the_epoch(
    zone: str, text: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    date = importlib.import_module("curly_message._intl.date")
    monkeypatch.setattr(date, "local_zone", lambda: ZoneInfo(zone))
    output = resolve("{{v:date}}", {"v": "-3000000000000"}, date={"timeStyle": "full"})
    assert output == (text, [])


def test_a_named_zone_needs_a_zone_database_and_utc_does_not() -> None:
    # Without the system's database or tzdata, UTC and offsets still format.
    probe = """
import importlib.abc, sys
class Block(importlib.abc.MetaPathFinder):
    def find_spec(self, name, path, target=None):
        if name == "tzdata" or name.startswith("tzdata."):
            raise ModuleNotFoundError(name)
sys.meta_path.insert(0, Block())
from curly_message import create_parser
codes = []
parser = create_parser(on_report=lambda report: codes.append(report.code))
for zone in ("UTC", "+01:00", "Europe/Prague"):
    props = {"date": {"timeZone": zone, "timeStyle": "short"}}
    print(repr(parser.resolve("{{v:date}}", {"v": "0"}, props=props, locale="en")))
print(codes)
"""
    env = {**os.environ, "PYTHONTZPATH": ""}
    lines = subprocess.run(
        [sys.executable, "-c", probe], capture_output=True, text=True, check=True, env=env
    ).stdout.splitlines()
    assert lines == ["'12:00 AM'", "'1:00 AM'", "''", "['failed-modifier']"]


# The tzdata package holds Europe/Dublin's winter as a negative saving, as the
# database's main form does; a system database can hold the rearguard form,
# where the summer is the saving, as ICU reads it.
@pytest.mark.parametrize("database", ["system", "tzdata"])
def test_a_zone_that_saves_backwards_is_named_as_icu_reads_it(database: str) -> None:
    probe = """
from curly_message import create_parser
parser = create_parser()
for v in ("1704110400000", "1720000000000", "253399089600000"):
    for locale, style in (("en", "full"), ("en-IE", "long")):
        props = {"date": {"timeZone": "Europe/Dublin", "timeStyle": style}}
        print(parser.resolve("{{v:date}}", {"v": v}, props=props, locale=locale))
"""
    env = {**os.environ, **({"PYTHONTZPATH": ""} if database == "tzdata" else {})}
    lines = subprocess.run(
        [sys.executable, "-c", probe], capture_output=True, text=True, check=True, env=env
    ).stdout.splitlines()
    assert lines == [
        "12:00:00 PM Greenwich Mean Time",
        "12:00:00 GMT",
        "10:46:40 AM Irish Standard Time",
        "10:46:40 IST",
        "8:00:00 PM Greenwich Mean Time",
        "20:00:00 GMT",
    ]


LOCAL_PROBE = """
import importlib, os, time
from curly_message import create_parser
date = importlib.import_module("curly_message._intl.date")
codes = []
parser = create_parser(on_report=lambda report: codes.append(report.code))
payload = {"v": "0", "w": "2024-03-05T15:00:00", "s": "2024-08-15T12:00:00"}
for style in ("long", "full"):
    props = {"date": {"timeStyle": style}}
    print(parser.resolve("{{v:date}}, {{w:date}}, {{s:date}}", payload, props=props, locale="en"))
print(date.time_value("2024-03-31T02:30:00"), date.time_value("2024-10-27T02:30:00"))
os.environ["TZ"] = "JST-9"
time.tzset()
print(parser.resolve("{{v:date}}", payload, props={"date": {"timeStyle": "long"}}, locale="en"))
print(codes)
"""

GMT = "12:00:00 AM GMT, 3:00:00 PM GMT, 12:00:00 PM GMT"


def _babel_before(version: tuple[int, int]) -> bool:
    try:
        found = importlib.metadata.version("babel")
    except importlib.metadata.PackageNotFoundError:
        return False
    return tuple(int(part) for part in found.split(".")[:2]) < version


CET = (
    "1:00:00 AM GMT+1, 3:00:00 PM GMT+1, 12:00:00 PM GMT+2",
    "1:00:00 AM GMT+01:00, 3:00:00 PM GMT+01:00, 12:00:00 PM GMT+02:00",
    # The spring transition skips 2:30, read by 3.11 at the offset after it
    # and by later releases at the one before. The autumn one repeats it,
    # read as its first occurrence.
    ("1711845000000" if sys.version_info < (3, 12) else "1711848600000") + " 1729989000000",
)


@pytest.mark.skipif(sys.platform == "win32", reason="Windows reads its zone from the registry")
@pytest.mark.parametrize(
    ("zone", "long", "full", "transitions"),
    [
        # A zone of the database is named by its offset all the same.
        ("Europe/Prague", *CET),
        ("CET-1CEST,M3.5.0,M10.5.0/3", *CET),
        (
            "<+0530>-5:30",
            "5:30:00 AM GMT+5:30, 3:00:00 PM GMT+5:30, 12:00:00 PM GMT+5:30",
            "5:30:00 AM GMT+05:30, 3:00:00 PM GMT+05:30, 12:00:00 PM GMT+05:30",
            "1711832400000 1729976400000",
        ),
        ("UTC0", GMT, GMT, "1711852200000 1729996200000"),
        ("UTC", GMT, GMT, "1711852200000 1729996200000"),
    ],
)
def test_a_local_zone_is_python_s_local_time_named_by_its_offset(
    zone: str, long: str, full: str, transitions: str
) -> None:
    # A later ``time.tzset()`` is followed.
    env = {**os.environ, "TZ": zone}
    lines = subprocess.run(
        [sys.executable, "-c", LOCAL_PROBE], capture_output=True, text=True, check=True, env=env
    ).stdout.splitlines()
    assert lines == [long, full, transitions, "9:00:00 AM GMT+9", "[]"]


EDGE_PROBE = """
import sys
from curly_message import create_parser
codes = []
parser = create_parser(on_report=lambda report: codes.append(report.code))
props = {"date": {"dateStyle": "medium", "timeStyle": "medium", "timeZone": "UTC"}}
for value in sys.argv[1:]:
    output = parser.resolve("{{v:date; default:never}}", {"v": value}, props=props, locale="en")
    print(output, codes)
    codes.clear()
"""

REFUSED = "never ['failed-modifier']"


@pytest.mark.skipif(sys.platform == "win32", reason="Windows reads its zone from the registry")
@pytest.mark.parametrize(
    ("zone", "texts"),
    [
        (
            "UTC0",
            [
                "Jan 1, 1, 12:00:00 AM []",
                "Jan 1, 1, 12:00:00 PM []",
                "Dec 31, 9999, 12:00:00 AM []",
                "Dec 31, 9999, 11:59:00 PM []",
            ],
        ),
        # Midnight of the first day at Prague's mean time is in year 0.
        (
            "Europe/Prague",
            [
                REFUSED,
                "Jan 1, 1, 11:02:16 AM []",
                "Dec 30, 9999, 11:00:00 PM []",
                "Dec 31, 9999, 10:59:00 PM []",
            ],
        ),
        (
            "Pacific/Kiritimati",
            [
                "Jan 1, 1, 10:29:20 AM []",
                "Jan 1, 1, 10:29:20 PM []",
                "Dec 30, 9999, 10:00:00 AM []",
                "Dec 31, 9999, 9:59:00 AM []",
            ],
        ),
        # The last minute of the calendar twelve hours behind is in year 10000.
        (
            "Etc/GMT+12",
            [
                "Jan 1, 1, 12:00:00 PM []",
                "Jan 2, 1, 12:00:00 AM []",
                "Dec 31, 9999, 12:00:00 PM []",
                REFUSED,
            ],
        ),
    ],
)
def test_a_naive_time_on_the_first_or_last_day_is_placed_in_local_time(
    zone: str, texts: list[str]
) -> None:
    days = ("0001-01-01", "0001-01-01T12:00", "9999-12-31", "9999-12-31T23:59")
    lines = subprocess.run(
        [sys.executable, "-c", EDGE_PROBE, *days],
        capture_output=True,
        text=True,
        check=True,
        env={**os.environ, "TZ": zone},
    ).stdout.splitlines()
    assert lines == texts


@pytest.mark.skipif(sys.platform == "win32", reason="Windows reads its zone from the registry")
def test_a_local_offset_of_a_day_or_more_takes_the_fallback_chain() -> None:
    probe = """
from curly_message import create_parser
codes = []
parser = create_parser(on_report=lambda report: codes.append(report.code))
payload = {"v": "0", "w": "2024-03-05T15:00:00"}
print(parser.resolve("{{v:date; default:-}} {{w:date; default:-}}", payload, locale="en"))
print(codes)
"""
    lines = subprocess.run(
        [sys.executable, "-c", probe],
        capture_output=True,
        text=True,
        check=True,
        env={**os.environ, "TZ": "ABC-24"},
    ).stdout.splitlines()
    assert lines == ["- -", "['failed-modifier', 'failed-modifier']"]


@pytest.mark.parametrize(
    ("tz", "named", "text"),
    [(None, None, "9:00:00 AM GMT+9"), ("EST5", "Europe/Prague", "1:00:00 AM GMT+1")],
)
def test_on_windows_the_local_zone_is_the_registry_s_or_its_standard_offset(
    tz: str | None, named: str | None, text: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    date = importlib.import_module("curly_message._intl.date")

    def get_localzone() -> ZoneInfo:
        if named is None:
            raise LookupError("Can not find any timezone configuration")
        return ZoneInfo(named)

    monkeypatch.setattr(date, "get_localzone", get_localzone)
    monkeypatch.setattr(sys, "platform", "win32")
    monkeypatch.setattr(time, "timezone", -32400)
    if tz is None:
        monkeypatch.delenv("TZ", raising=False)
    else:
        monkeypatch.setenv("TZ", tz)
    monkeypatch.setattr(date, "local_zone", date._system_zone)
    assert resolve("{{v:date}}", {"v": "0"}, date={"timeStyle": "long"}) == (text, [])


@pytest.mark.skipif(sys.platform == "win32", reason="Windows reads its zone from the registry")
@pytest.mark.parametrize(
    "zone",
    [
        "directory",
        "file",
        pytest.param(
            "database directory",
            marks=pytest.mark.skipif(sys.version_info >= (3, 13), reason="Babel reads no zone"),
        ),
        pytest.param(
            "refused key",
            marks=pytest.mark.skipif(not _babel_before((2, 16)), reason="Babel reads no zone"),
        ),
    ],
)
def test_a_local_zone_file_the_supplier_cannot_read_leaves_intl_unsatisfied(
    zone: str, tmp_path: Path
) -> None:
    # The supplier reads the zone a TZ naming a file holds as it is imported,
    # and its locale data imports that reading: where the file holds no zone,
    # no Intl modifier can load, and Core is left as it was.
    probe = """
from curly_message import create_parser, levels
codes = []
parser = create_parser(on_report=lambda report: codes.append(report.code))
for message in ("{{n:number; default:-}}", "{{v:date; default:-}}", "{{n:eq; 1234.5:core}}"):
    print(parser.resolve(message, {"v": "0", "n": "1234.5"}, locale="en"))
print(codes, sorted(levels()))
"""
    path = tmp_path / "zone"
    if zone == "directory":
        path.mkdir()
    elif zone == "file":
        path.write_bytes(b"not a zone")
    named = {"database directory": "Europe", "refused key": "Asia//Tokyo"}
    env = {**os.environ, "TZ": named.get(zone, str(path))}
    lines = subprocess.run(
        [sys.executable, "-c", probe], capture_output=True, text=True, check=True, env=env
    ).stdout.splitlines()
    assert lines == [
        "-",
        "-",
        "core",
        "['failed-modifier', 'unknown-modifier'] ['core', 'extensions']",
    ]


@pytest.mark.parametrize(
    "host",
    [
        "decimal.DefaultContext.traps[decimal.Inexact] = True\n"
        "decimal.DefaultContext.traps[decimal.Rounded] = True",
        "decimal.DefaultContext.Emax = 10\ndecimal.DefaultContext.Emin = -10",
        "decimal.DefaultContext.rounding = decimal.ROUND_FLOOR",
        "decimal.DefaultContext.prec = 1",
        "decimal.getcontext().prec = 1\ndecimal.getcontext().traps[decimal.Inexact] = True",
    ],
    ids=["traps", "exponent", "rounding", "precision", "thread"],
)
def test_the_host_decimal_context_decides_no_digit(host: str) -> None:
    # The host's own decimal arithmetic is configured before the first Intl
    # call, in a process of its own, so nothing built earlier hides it.
    probe = f"""
import decimal
{host}
from curly_message import create_parser
reports = []
parser = create_parser(on_report=reports.append)
number = [
    parser.resolve("{{{{n:number}}}}", {{"n": value}}, locale="en")
    for value in ("1234.5678", "2.5", "1e20", "0.000001234")
]
digits = {{"number": {{"maximumSignificantDigits": 3}}}}
number.append(parser.resolve("{{{{n:number}}}}", {{"n": "1.005"}}, props=digits, locale="en"))
number.append(parser.resolve("{{{{n:plural; one:1; other:O}}}}", {{"n": "1.5"}}, locale="en"))
zone = {{"date": {{"timeZone": -13, "hour": "numeric"}}}}
number.append(parser.resolve("{{{{v:date}}}}", {{"v": "0"}}, props=zone, locale="en"))
print(ascii(number))
print(len(reports))
"""
    lines = subprocess.run(
        [sys.executable, "-c", probe], capture_output=True, text=True, check=True
    ).stdout.splitlines()
    assert lines == [
        ascii(
            [
                "1,234.57",
                "2.5",
                "100,000,000,000,000,000,000",
                "0",
                "1.01",
                "O",
                "11 AM",
            ]
        ),
        "0",
    ]


FLAKY_FIRST_CALL = """
import importlib
import importlib.util
from curly_message import create_parser, levels
real = importlib.{name}
calls = []
def flaky(*args, **kwargs):
    # Only the modifiers' own question is counted: what else imports through
    # the same function depends on the installation.
    if args[0] == {target!r}:
        calls.append(args[0])
        if len(calls) == 1:
            raise {error}("asked too deep")
    return real(*args, **kwargs)
importlib.{name} = flaky
{before}
reports = []
parser = create_parser(on_report=lambda report: reports.append(report.code))
first = parser.resolve("{{{{n:number}}}}", {{"n": "1234.5"}}, locale="en")
again = parser.resolve("{{{{n:number}}}}", {{"n": "1234.5"}}, locale="en")
print(ascii([first, again, reports, sorted(levels()), len(calls)]))
"""


def _first_call_fails(name: str, error: str, before: str = "") -> object:
    target = "babel" if name == "util.find_spec" else "curly_message._intl"
    probe = FLAKY_FIRST_CALL.format(name=name, target=target, error=error, before=before)
    output = subprocess.run(
        [sys.executable, "-c", probe], capture_output=True, text=True, check=True
    ).stdout
    return output.strip()


INTL_LEVELS = ["core", "extensions", "intl"]


def test_a_supplier_check_that_fails_to_answer_is_asked_again() -> None:
    # An exhausted stack or memory makes the question fail rather than
    # answer it, and one unlucky call must not turn the supplier off.
    assert _first_call_fails("util.find_spec", "RecursionError") == ascii(
        ["", "1,234.5", ["unknown-modifier"], INTL_LEVELS, 2]
    )


@pytest.mark.parametrize("before", ["", "levels()"])
def test_an_import_of_the_modifiers_that_fails_to_finish_is_tried_again(before: str) -> None:
    assert _first_call_fails("import_module", "RecursionError", before) == ascii(
        ["", "1,234.5", ["failed-modifier"], INTL_LEVELS, 2]
    )


@pytest.mark.parametrize("error", ["ImportError", "AttributeError", "SyntaxError"])
def test_a_supplier_the_modifiers_cannot_import_is_asked_once_and_leaves_its_names_unknown(
    error: str,
) -> None:
    # The near miss: an import that answers, with a supplier lacking what the
    # modifiers import or broken in itself, is asked once. The call that found
    # it fails; after it, the installation satisfies no Intl, so the six names
    # are names nobody registered (section 2).
    assert _first_call_fails("import_module", error) == ascii(
        ["", "", ["failed-modifier", "unknown-modifier"], ["core", "extensions"], 1]
    )


def test_a_supplier_whose_release_cannot_be_read_is_asked_once() -> None:
    # A distribution whose metadata carries no version answers ``None``.
    probe = """
import importlib.metadata
from curly_message import create_parser, levels
calls = []
def version(name):
    calls.append(name)
    return None
importlib.metadata.version = version
reports = []
parser = create_parser(on_report=lambda report: reports.append(report.code))
outputs = [parser.resolve("{{n:number}}", {"n": "1"}, locale="en") for _ in range(3)]
print(ascii([outputs, reports, sorted(levels()), len(calls)]))
"""
    output = subprocess.run(
        [sys.executable, "-c", probe], capture_output=True, text=True, check=True
    ).stdout.strip()
    assert output == ascii([["", "", ""], ["unknown-modifier"] * 3, ["core", "extensions"], 1])


def test_props_refused_under_number_and_under_plural_are_one_input_reported_once() -> None:
    # The format's ``plural`` reads the digit properties composed under
    # ``number`` beside its own: the props are still one input (section 11.2).
    class Colliding(str):
        __slots__ = ()

        def __hash__(self) -> int:
            return hash(str(self))

        def __eq__(self, other: object) -> bool:
            raise RuntimeError("compared")

    reports: list[Report] = []
    props: Any = {Colliding("number"): {}, Colliding("plural"): {}}
    output = create_parser(on_report=reports.append).resolve(
        "{{n:plural; one:ONE; other:OTHER;}}", {"n": 1}, props=props, locale="en"
    )
    assert (output, codes(reports)) == ("ONE", ["unserializable-value"])
