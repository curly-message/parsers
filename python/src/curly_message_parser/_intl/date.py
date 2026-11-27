"""``Intl.DateTimeFormat`` over the supplier's locale data, and the time a value names."""

from __future__ import annotations

import functools
import math
import sys
import time as host_time
import zoneinfo
from collections.abc import Iterator
from datetime import UTC, datetime, timedelta, timezone, tzinfo
from typing import Final, Literal

from babel import Locale
from babel.core import get_global
from babel.dates import (
    NO_INHERITANCE_MARKER,
    format_datetime,
    get_period_id,
    get_timezone_name,
    tokenize_pattern,
    untokenize_pattern,
)
from babel.localtime import get_localzone

from .._convert import to_number
from .locale import babel_locale
from .skeleton import DAY_PERIOD, HOUR, best_pattern, skeleton
from .validate import (
    LOCALE_MATCHERS,
    Options,
    default_number,
    get_boolean,
    get_string,
    get_type_sequence,
    given,
    refuse,
)

EPOCH: Final = datetime(1970, 1, 1, tzinfo=UTC)

_MAX_TIME: Final = 8.64e15

_STYLES: Final = ("full", "long", "medium", "short")

_TEXT_WIDTHS: Final = ("narrow", "short", "long")
_NUMERIC: Final = ("2-digit", "numeric")

# Each component option: its values, and the skeleton field each value asks for.
_COMPONENTS: Final = {
    "weekday": {"narrow": "EEEEE", "short": "EEE", "long": "EEEE"},
    "era": {"narrow": "GGGGG", "short": "G", "long": "GGGG"},
    "year": {"2-digit": "yy", "numeric": "y"},
    "month": {"2-digit": "MM", "numeric": "M", "narrow": "MMMMM", "short": "MMM", "long": "MMMM"},
    "day": {"2-digit": "dd", "numeric": "d"},
    "dayPeriod": {"narrow": "BBBBB", "short": "B", "long": "BBBB"},
    "hour": {"2-digit": "hh", "numeric": "h"},
    "minute": {"2-digit": "mm", "numeric": "m"},
    "second": {"2-digit": "ss", "numeric": "s"},
}

_ZONE_NAMES: Final = ("short", "long", "shortOffset", "longOffset", "shortGeneric", "longGeneric")

_DATE_FIELDS: Final = ("era", "year", "month", "weekday", "day")

# The components whose presence keeps ECMA-402 from adding its default date.
_SHOWN: Final = ("weekday", "year", "month", "day", "dayPeriod", "hour", "minute", "second")

_HOUR_LETTERS: Final = {"h11": "K", "h12": "h", "h23": "H", "h24": "k"}
_TWELVE: Final = frozenset(("h11", "h12"))

# The zone names ECMA-402 reads as UTC, which need no zone database.
_UTC_NAMES: Final = frozenset(("utc", "etc/utc", "etc/gmt", "gmt", "gmt0", "gmt+0", "gmt-0"))


def _supplier_zone() -> tzinfo | None:
    try:
        zone = get_localzone()
    except (LookupError, ValueError, OSError):
        return None
    if isinstance(zone, zoneinfo.ZoneInfo | timezone):
        return zone
    # Where pytz is installed the supplier answers one of its zones, whose
    # offsets are wrong under ``replace``; the zone it names is read again
    # here.
    key = getattr(zone, "zone", None)
    if isinstance(key, str):
        try:
            return zoneinfo.ZoneInfo(key)
        except (OSError, ValueError, zoneinfo.ZoneInfoNotFoundError):
            pass
    return None


def _standard_zone() -> tzinfo:
    # The standard offset the C runtime reads from the registry, where the
    # supplier maps the registry's zone to none.
    offset = timedelta(seconds=-host_time.timezone)
    return timezone(offset, "") if abs(offset) < timedelta(days=1) else UTC


def _system_zone() -> tzinfo | None:
    if sys.platform == "win32":
        return _supplier_zone() or _standard_zone()
    return None


@functools.cache
def _cached_system_zone() -> tzinfo | None:
    return _system_zone()


def local_zone() -> tzinfo | None:
    """The zone a time is read and shown in where nothing names one: the system's.

    On Windows it is the zone the supplier maps the registry's to, or the
    registry's standard offset. Elsewhere it is ``None``: the local time
    Python's ``datetime`` reads through the C library, which follows ``TZ``
    and ``time.tzset()``, and which names no zone.
    """
    return _cached_system_zone()


def _local(naive: datetime) -> datetime:
    try:
        return naive.astimezone()
    except (OverflowError, OSError, ValueError):
        # Python reads a naive time against the offsets a day either side,
        # which the first and last days it holds lack.
        inward = naive + timedelta(days=1) if naive.year == 1 else naive - timedelta(days=1)
        return naive.replace(tzinfo=inward.astimezone().tzinfo)


def time_value(text: str) -> float | None:
    """The time a value names, in milliseconds since the epoch, or ``None`` (section 11.2).

    Numeric text is a timestamp; anything else is read as an ISO 8601 date or
    date and time, a naive one in the local zone.
    """
    number = to_number(text)
    if number is not None:
        return number if math.isfinite(number) else None
    try:
        parsed = datetime.fromisoformat(text)
    except ValueError:
        return None
    try:
        if parsed.tzinfo is None:
            zone = local_zone()
            parsed = _local(parsed) if zone is None else parsed.replace(tzinfo=zone)
        return (parsed - EPOCH) // timedelta(milliseconds=1)
    except (OverflowError, OSError, ValueError):
        return None


@functools.cache
def _zone_names() -> dict[str, str]:
    return {name.lower(): name for name in zoneinfo.available_timezones()}


def _offset_zone(text: str) -> timezone | None:
    # ECMA-402's offset time zones: a sign, two digits of hours and,
    # optionally, two of minutes, with or without a colon.
    if len(text) < 3 or text[0] not in "+-":
        return None
    digits = text[1:]
    if len(digits) == 5 and digits[2] == ":":
        digits = digits[:2] + digits[3:]
    if len(digits) not in (2, 4) or not all("0" <= c <= "9" for c in digits):
        return None
    hours = int(digits[:2])
    minutes = int(digits[2:] or "0")
    if hours > 23 or minutes > 59:
        return None
    offset = timedelta(hours=hours, minutes=minutes)
    if not offset:
        # ICU names the zero offset as it names Etc/GMT, not as UTC.
        return timezone(offset, "Etc/GMT")
    return timezone(-offset if text[0] == "-" else offset)


def _zone(name: str) -> tzinfo:
    if name.lower() in _UTC_NAMES:
        return UTC
    fixed = _offset_zone(name)
    if fixed is not None:
        return fixed
    key = _zone_names().get(name.lower())
    if key is None:
        refuse()
    try:
        return zoneinfo.ZoneInfo(key)
    except (ValueError, zoneinfo.ZoneInfoNotFoundError):
        refuse()


class DateSpec:
    """A ``DateTimeFormat`` request, read as ECMA-402's constructor reads it."""

    __slots__ = ("components", "date_style", "hour12", "hour_cycle", "time_style", "zone")

    def __init__(self, options: Options) -> None:
        get_string(options, "localeMatcher", LOCALE_MATCHERS, "best fit")
        get_type_sequence(options, "calendar")
        get_type_sequence(options, "numberingSystem")
        self.hour12 = get_boolean(options, "hour12")
        hour_cycle = get_string(options, "hourCycle", tuple(_HOUR_LETTERS), None)
        self.hour_cycle = None if self.hour12 is not None else hour_cycle
        zone = get_string(options, "timeZone", None, None)
        self.zone = None if zone is None else _zone(zone)
        components: dict[str, str] = {}
        for name, fields in _COMPONENTS.items():
            value = get_string(options, name, tuple(fields), None)
            if value is not None:
                components[name] = value
        fraction = default_number(given(options, "fractionalSecondDigits"), 1, 3, None)
        zone_name = get_string(options, "timeZoneName", _ZONE_NAMES, None)
        get_string(options, "formatMatcher", ("basic", "best fit"), "best fit")
        self.date_style = get_string(options, "dateStyle", _STYLES, None)
        self.time_style = get_string(options, "timeStyle", _STYLES, None)
        styled = self.date_style is not None or self.time_style is not None
        if styled and (components or fraction is not None or zone_name is not None):
            refuse()
        if not styled and fraction is None and not any(name in components for name in _SHOWN):
            # ECMA-402's defaults: with nothing a date or a time is shown
            # with, the numeric date.
            components.update(year="numeric", month="numeric", day="numeric")
        self.components = components


Tokens = list[tuple[str, "str | tuple[str, int]"]]


def _locale_cycle(locale: Locale) -> str:
    # The hour cycle a locale's own short time is written in.
    for kind, value in tokenize_pattern(locale.time_formats["short"].pattern):
        if kind == "field" and isinstance(value, tuple) and value[0] in "hHKk":
            return {"h": "h12", "H": "h23", "K": "h11", "k": "h24"}[value[0]]
    return "h23"


def _hour_cycle(spec: DateSpec, locale: Locale, tag_cycle: str | None) -> str:
    # ``hour12`` reads the cycle the tag asks for as the default it departs
    # from, as V8 resolves it: twelve hours are ``h12`` unless that default
    # is ``h11``, and twenty-four ``h23`` unless it is ``h24``.
    default = tag_cycle or _locale_cycle(locale)
    if spec.hour12 is True:
        return "h11" if default == "h11" else "h12"
    if spec.hour12 is False:
        return "h24" if default == "h24" else "h23"
    return spec.hour_cycle or default


def _pattern_cycle(pattern: str) -> str | None:
    for kind, value in tokenize_pattern(pattern):
        if kind == "field" and isinstance(value, tuple) and value[0] in "hHKk":
            return {"h": "h12", "H": "h23", "K": "h11", "k": "h24"}[value[0]]
    return None


def _with_hour(pattern: str, letter: str) -> str:
    return str(
        untokenize_pattern(
            [
                ("field", (letter, value[1]))
                if kind == "field" and isinstance(value, tuple) and value[0] in "hHKk"
                else (kind, value)
                for kind, value in tokenize_pattern(pattern)
            ]
        )
    )


def _regenerated(pattern: str, letter: str) -> str:
    # A style written in another cycle is asked for again as ICU and V8 ask: its
    # skeleton, with the cycle's hour and without a day period.
    fields = skeleton(pattern)
    return "".join(
        (letter if field == HOUR else found) * width
        for field, (found, width, _) in sorted(fields.items())
        if field != DAY_PERIOD
    )


def _requested(components: dict[str, str], letter: str, twelve: bool) -> str:
    text = "".join(
        _COMPONENTS[name][components[name]] for name in _DATE_FIELDS if name in components
    )
    if "hour" in components:
        if twelve and "dayPeriod" in components:
            text += _COMPONENTS["dayPeriod"][components["dayPeriod"]]
        text += letter * len(_COMPONENTS["hour"][components["hour"]])
    elif "dayPeriod" in components and not ("minute" in components or "second" in components):
        text += _COMPONENTS["dayPeriod"][components["dayPeriod"]]
    for name in ("minute", "second"):
        if name in components:
            text += _COMPONENTS[name][components[name]]
    return text


def date_pattern(spec: DateSpec, locale: Locale, tag_cycle: str | None) -> str:
    """The pattern a request shows a time in."""
    cycle = _hour_cycle(spec, locale, tag_cycle)
    letter = _HOUR_LETTERS[cycle]
    if spec.date_style is None and spec.time_style is None:
        request = skeleton(_requested(spec.components, letter, cycle in _TWELVE))
        return _with_hour(best_pattern(locale, request), letter)
    date = None if spec.date_style is None else str(locale.date_formats[spec.date_style].pattern)
    time = None if spec.time_style is None else str(locale.time_formats[spec.time_style].pattern)
    if time is not None and tag_cycle is not None and _pattern_cycle(time) != tag_cycle:
        # ICU asks for a style's time again in the cycle the tag names, the
        # hour as wide as the locale writes it, and keeps its date.
        tag_letter = _HOUR_LETTERS[tag_cycle]
        again = skeleton(_regenerated(time, tag_letter))
        time = _with_hour(best_pattern(locale, again, hour_length=False), tag_letter)
    pattern = date or time or ""
    if date is not None and time is not None:
        glue = str(locale.datetime_formats[spec.date_style or "short"])
        pattern = glue.replace("{1}", date).replace("{0}", time)
    if time is not None and _pattern_cycle(time) != cycle:
        # V8 then asks for the whole pattern again in the cycle the options
        # request, the hour as wide as the style writes it.
        return _with_hour(best_pattern(locale, skeleton(_regenerated(pattern, letter))), letter)
    return pattern


def format_date(tag: object, spec: DateSpec, time: float) -> str:
    """``new Intl.DateTimeFormat(tag, options).format(time)``, the options read into ``spec``."""
    locale, tag_cycle = babel_locale(tag)
    if not math.isfinite(time) or abs(time) > _MAX_TIME:
        refuse()
    zone = spec.zone if spec.zone is not None else local_zone()
    try:
        instant = (EPOCH + timedelta(milliseconds=math.trunc(time))).astimezone(zone)
    except (OverflowError, OSError, ValueError):
        refuse()
    pattern = _as_icu_writes(date_pattern(spec, locale, tag_cycle), instant, locale, zone is None)
    return str(format_datetime(instant, pattern, locale=locale))


def _localized_gmt(instant: datetime, long: bool, locale: Locale) -> str:
    # CLDR's localized GMT format: the long form's hours and minutes always
    # two digits, the short form's hours as they are and its minutes only
    # where it has any. The supplier carries neither the hour format nor the
    # format of the zero offset, so the sign is ASCII and the zero offset is
    # the format without one.
    offset = instant.utcoffset()
    total = 0 if offset is None else int(offset.total_seconds())
    gmt = str(locale.zone_formats["gmt"])
    if total == 0:
        return (gmt % "").strip()
    hours, rest = divmod(abs(total), 3600)
    minutes, seconds = divmod(rest, 60)
    if long:
        text = f"{hours:02d}:{minutes:02d}"
    else:
        text = str(hours) + (f":{minutes:02d}" if minutes or seconds else "")
    if seconds:
        text += f":{seconds:02d}"
    return gmt % (("-" if total < 0 else "+") + text)


@functools.cache
def _zone_identity(canonical: str) -> tuple[tuple[str, ...], str]:
    # The identifiers CLDR may file a zone's names under, and its metazone.
    # The supplier canonicalizes an identifier to the database's spelling
    # (Europe/Kyiv), while CLDR keeps a zone under the one it first filed it
    # by (Europe/Kiev), and maps that one alone to a metazone.
    aliases = get_global("zone_aliases")
    ids = (canonical, *(alias for alias, target in aliases.items() if target == canonical))
    metazones = get_global("meta_zones")
    return ids, next((metazones[key] for key in ids if key in metazones), "")


# How far a zone is looked at either side of a month: over a year, 30 days
# at a time, shorter than any period the database sets a clock back for
# (35 days, in Africa/Casablanca).
_PROBE_STEP: Final = timedelta(days=30)
_PROBE_STEPS: Final = 13


def _probes(
    zone: tzinfo, start: datetime, step: timedelta
) -> Iterator[tuple[timedelta, timedelta]]:
    # The offset and the saving of the zone at each step from ``start``.
    for count in range(1, _PROBE_STEPS + 1):
        try:
            probe = (start + count * step).astimezone(zone)
        except (OverflowError, ValueError):
            return
        offset, saving = probe.utcoffset(), probe.dst()
        if offset is not None and saving is not None:
            yield offset, saving


@functools.lru_cache(maxsize=1024)
def _daylight_in(zone: tzinfo, offset: timedelta, saving: timedelta, month: int) -> bool:
    start = datetime(month // 12, month % 12 + 1, 1, tzinfo=UTC)
    sides = (_probes(zone, start, -_PROBE_STEP), _probes(zone, start, _PROBE_STEP))
    if saving:
        # A negative saving read off a file can be an artifact of how the
        # file is read: a daylight time whose offset the standard time
        # before it shared. A clock set back is lower than the standard
        # time on either side of it. A side with no standard time in reach,
        # the end of the calendar among them, shows no such artifact.
        beside = [next((other for other, saved in side if not saved), None) for side in sides]
        return not all(other is None or other > offset for other in beside)
    return all(
        any(saved < timedelta(0) and other - saved == offset for other, saved in side)
        for side in sides
    )


def _is_daylight(instant: datetime) -> bool:
    # ICU reads the database in its rearguard form, which holds no negative
    # saving: where the main form sets a zone's clock back from its standard
    # time for part of each year (Europe/Dublin's winter, read from the
    # tzdata package), the time set back is standard time, and the time
    # between two such periods is daylight time.
    saving, offset = instant.dst(), instant.utcoffset()
    if saving is None or offset is None or saving > timedelta(0):
        return bool(saving)
    if not isinstance(instant.tzinfo, zoneinfo.ZoneInfo):
        return False
    month = instant.year * 12 + instant.month - 1
    return _daylight_in(instant.tzinfo, offset, saving, month)


def _zone_name(instant: datetime, width: int, locale: Locale, by_offset: bool) -> str:
    # ICU names a zone by the name the locale gives that zone itself, then by
    # the name of the metazone it belongs to at that instant, then in the
    # localized GMT format, where the supplier writes an ISO offset. A name
    # the locale marks as not inherited is no name, where the supplier would
    # take the long one for a short one. The supplier's metazones carry no
    # dates, so it would name a zone by today's metazone at any instant;
    # before the epoch, where ICU's mapping of nearly every zone to a
    # metazone begins, no metazone names it. Python's local time has no key
    # to name it by, and the supplier would name it by its abbreviation.
    long = width >= 4
    style: Literal["long", "short"] = "long" if long else "short"
    if by_offset:
        return _localized_gmt(instant, long, locale)
    canonical = str(get_timezone_name(instant, style, locale=locale, return_zone=True))
    ids, metazone = _zone_identity(canonical)
    own = next((names for key in ids if (names := locale.time_zones.get(key))), None)
    sources = [] if own is None else [own]
    if metazone and instant >= EPOCH:
        sources.append(locale.meta_zones.get(metazone, {}))
    if not sources:
        return _localized_gmt(instant, long, locale)
    variant = "daylight" if _is_daylight(instant) else "standard"
    for names in sources:
        name = names.get(style, {}).get(variant)
        if isinstance(name, str) and name and name != NO_INHERITANCE_MARKER:
            return name
    return _localized_gmt(instant, long, locale)


def _as_icu_writes(pattern: str, instant: datetime, locale: Locale, by_offset: bool) -> str:
    # ICU writes the plain day period up to a width of three with the
    # locale's wide name where it has one, and names no flexible period
    # "midnight": that instant takes the period its hour falls in, the one
    # a second later. A specific zone name the locale has no name for is
    # written in the localized GMT format.
    names = locale.day_periods.get("format", {})
    midnight = get_period_id(instant, locale=locale) == "midnight"
    tokens: Tokens = []
    for kind, value in tokenize_pattern(pattern):
        if kind == "field" and isinstance(value, tuple):
            letter, width = value
            if letter == "a" and width <= 3:
                period = "am" if instant.hour < 12 else "pm"
                name = names.get("wide", {}).get(period) or names.get("abbreviated", {}).get(period)
                if name:
                    tokens.append(("chars", str(name)))
                    continue
            elif letter == "B" and midnight:
                later = instant + timedelta(seconds=1)
                tokens.append(("chars", str(format_datetime(later, "B" * width, locale=locale))))
                continue
            elif letter == "z":
                tokens.append(("chars", _zone_name(instant, width, locale, by_offset)))
                continue
        tokens.append((kind, value))
    return _untokenized(tokens)


def _untokenized(tokens: Tokens) -> str:
    # Adjacent literals are joined before they are quoted: the supplier
    # quotes each on its own, and two quoted literals side by side read as
    # one holding a quote. V8 writes a narrow no-break space in a pattern as
    # a space.
    joined: Tokens = []
    for kind, value in tokens:
        if kind == "chars" and isinstance(value, str):
            text = value.replace("\u202f", " ")
            if joined and joined[-1][0] == "chars":
                joined[-1] = ("chars", str(joined[-1][1]) + text)
            else:
                joined.append(("chars", text))
            continue
        joined.append((kind, value))
    return str(untokenize_pattern(joined))
