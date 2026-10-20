"""The pattern a skeleton asks for, chosen as ICU's date-time pattern generator chooses it.

ECMA-402 leaves the choice to the implementation, and every engine the set
is recorded on makes it with ICU: the locale's style patterns, the single
fields and its available formats are candidates, each at a distance from the
request that counts the fields it lacks or adds and how far each width is
from the one asked for. The nearest wins, the earliest of equals; its fields
are then given the requested widths where its own skeleton does not already
settle them. The supplier's own matcher weighs widths differently, so it
answers another pattern where several are near.
"""

from __future__ import annotations

import functools
from typing import Final

from babel import Locale
from babel.dates import tokenize_pattern, untokenize_pattern

_DELTA: Final = 0x10
_NUMERIC: Final = 0x100
_NARROW: Final = -0x101
_SHORTER: Final = -0x102
_SHORT: Final = -0x103
_LONG: Final = -0x104

_EXTRA_FIELD: Final = 0x10000
_MISSING_FIELD: Final = 0x1000

ERA, YEAR, QUARTER, MONTH, WEEK, WEEK_OF_MONTH, WEEKDAY = 0, 1, 2, 3, 4, 5, 6
DAY_OF_YEAR, WEEKDAY_IN_MONTH, DAY, DAY_PERIOD, HOUR, MINUTE, SECOND = 7, 8, 9, 10, 11, 12, 13
FRACTION, ZONE = 14, 15

_DATE_MASK: Final = (1 << DAY_PERIOD) - 1
_TIME_MASK: Final = (1 << (ZONE + 1)) - 1 - _DATE_MASK


def _text(field: int, offset: int) -> tuple[tuple[int, int, int], ...]:
    return (
        (1, field, _SHORT - offset),
        (4, field, _LONG - offset),
        (5, field, _NARROW - offset),
    )


# ICU's table of pattern letters: for each, from which width on a row
# applies, the field it writes and the kind of value it is. A numeric kind
# is positive and grows with the width; a textual one is one value for every
# width of the row.
_ROWS: Final[dict[str, tuple[tuple[int, int, int], ...]]] = {
    "G": _text(ERA, 0),
    "y": ((1, YEAR, _NUMERIC),),
    "Y": ((1, YEAR, _NUMERIC + _DELTA),),
    "u": ((1, YEAR, _NUMERIC + 2 * _DELTA),),
    "r": ((1, YEAR, _NUMERIC + 3 * _DELTA),),
    "U": _text(YEAR, 0),
    "Q": ((1, QUARTER, _NUMERIC), (3, QUARTER, _SHORT), (4, QUARTER, _LONG), (5, QUARTER, _NARROW)),
    "q": (
        (1, QUARTER, _NUMERIC + _DELTA),
        (3, QUARTER, _SHORT - _DELTA),
        (4, QUARTER, _LONG - _DELTA),
        (5, QUARTER, _NARROW - _DELTA),
    ),
    "M": ((1, MONTH, _NUMERIC), (3, MONTH, _SHORT), (4, MONTH, _LONG), (5, MONTH, _NARROW)),
    "L": (
        (1, MONTH, _NUMERIC + _DELTA),
        (3, MONTH, _SHORT - _DELTA),
        (4, MONTH, _LONG - _DELTA),
        (5, MONTH, _NARROW - _DELTA),
    ),
    "l": ((1, MONTH, _NUMERIC + _DELTA),),
    "w": ((1, WEEK, _NUMERIC),),
    "W": ((1, WEEK_OF_MONTH, _NUMERIC),),
    "E": (
        (1, WEEKDAY, _SHORT),
        (4, WEEKDAY, _LONG),
        (5, WEEKDAY, _NARROW),
        (6, WEEKDAY, _SHORTER),
    ),
    "c": (
        (1, WEEKDAY, _NUMERIC + 2 * _DELTA),
        (3, WEEKDAY, _SHORT - 2 * _DELTA),
        (4, WEEKDAY, _LONG - 2 * _DELTA),
        (5, WEEKDAY, _NARROW - 2 * _DELTA),
        (6, WEEKDAY, _SHORTER - 2 * _DELTA),
    ),
    "e": (
        (1, WEEKDAY, _NUMERIC + _DELTA),
        (3, WEEKDAY, _SHORT - _DELTA),
        (4, WEEKDAY, _LONG - _DELTA),
        (5, WEEKDAY, _NARROW - _DELTA),
        (6, WEEKDAY, _SHORTER - _DELTA),
    ),
    "d": ((1, DAY, _NUMERIC),),
    "g": ((1, DAY, _NUMERIC + _DELTA),),
    "D": ((1, DAY_OF_YEAR, _NUMERIC),),
    "F": ((1, WEEKDAY_IN_MONTH, _NUMERIC),),
    "a": _text(DAY_PERIOD, 0),
    "b": _text(DAY_PERIOD, _DELTA),
    "B": _text(DAY_PERIOD, 3 * _DELTA),
    "H": ((1, HOUR, _NUMERIC + 10 * _DELTA),),
    "k": ((1, HOUR, _NUMERIC + 11 * _DELTA),),
    "h": ((1, HOUR, _NUMERIC),),
    "K": ((1, HOUR, _NUMERIC + _DELTA),),
    "m": ((1, MINUTE, _NUMERIC),),
    "s": ((1, SECOND, _NUMERIC),),
    "A": ((1, SECOND, _NUMERIC + _DELTA),),
    "S": ((1, FRACTION, _NUMERIC),),
    "v": ((1, ZONE, _SHORT - 2 * _DELTA), (4, ZONE, _LONG - 2 * _DELTA)),
    "z": ((1, ZONE, _SHORT), (4, ZONE, _LONG)),
    "Z": ((1, ZONE, _NARROW - _DELTA), (4, ZONE, _LONG - _DELTA), (5, ZONE, _SHORT - _DELTA)),
    "O": ((1, ZONE, _SHORT - 2 * _DELTA), (4, ZONE, _LONG - 2 * _DELTA)),
    "V": (
        (1, ZONE, _SHORT - _DELTA),
        (2, ZONE, _LONG - _DELTA),
        (3, ZONE, _LONG - 1 - _DELTA),
        (4, ZONE, _LONG - 2 - _DELTA),
    ),
    "X": ((1, ZONE, _NARROW - _DELTA), (2, ZONE, _SHORT - _DELTA), (4, ZONE, _LONG - _DELTA)),
    "x": ((1, ZONE, _NARROW - _DELTA), (2, ZONE, _SHORT - _DELTA), (4, ZONE, _LONG - _DELTA)),
}

_CANONICAL_ITEMS: Final = "GyQMwWEDFdaHmsSv"

# The fields whose letter the found pattern keeps rather than the request's.
_PATTERN_LETTER: Final = frozenset((HOUR, MONTH, WEEKDAY))

Field = tuple[str, int, int]
"""A field of a skeleton: its letter, its width and its kind."""

Skeleton = dict[int, Field]


def _row(letter: str, width: int) -> tuple[int, int, int] | None:
    rows = _ROWS.get(letter)
    if rows is None:
        return None
    found = rows[0]
    for row in rows:
        if row[0] <= width:
            found = row
    return found


def _kind(letter: str, width: int) -> int:
    row = _row(letter, width)
    if row is None:
        return 0
    return row[2] + width if row[2] > 0 else row[2]


def skeleton(text: str) -> Skeleton:
    """The fields a skeleton or a pattern names, as ICU's matcher reads them.

    A twelve-hour hour is given the plain day period where none is named,
    and a day period beside any other hour is dropped.
    """
    fields: Skeleton = {}
    for kind, value in tokenize_pattern(text):
        if kind == "field" and isinstance(value, tuple):
            letter, width = value
            row = _row(letter, width)
            if row is not None:
                fields[row[1]] = (letter, width, _kind(letter, width))
    hour = fields.get(HOUR)
    if hour is not None:
        if hour[0] in "hK":
            fields.setdefault(DAY_PERIOD, ("a", 1, _SHORT))
        else:
            fields.pop(DAY_PERIOD, None)
    return fields


def _base(fields: Skeleton) -> tuple[tuple[int, str, int], ...]:
    # The skeleton with every width at its row's least: what decides whether
    # two patterns say the same thing.
    base = []
    for field, (letter, width, _) in sorted(fields.items()):
        row = _row(letter, width)
        base.append((field, letter, row[0] if row is not None else width))
    return tuple(base)


def _bucket(fields: Skeleton) -> int:
    # ICU keeps its candidates under the first letter of each base, upper
    # case before lower, and reads them in that order.
    if not fields:
        return 0
    letter = fields[min(fields)][0]
    return ord(letter) - ord("A") if letter <= "Z" else ord(letter) - ord("a") + 26


class Candidate:
    """A pattern the generator may answer, with the skeleton it was given, if any."""

    __slots__ = ("base", "fields", "pattern", "specified")

    def __init__(self, fields: Skeleton, pattern: str, specified: Skeleton | None) -> None:
        self.fields = fields
        self.base = _base(fields)
        self.pattern = pattern
        self.specified = specified


def _add(entries: list[Candidate], fields: Skeleton, pattern: str, specified: bool) -> None:
    original = {field: value[:2] for field, value in fields.items()}
    if not specified and any(entry.base == _base(fields) for entry in entries):
        return
    for entry in entries:
        if {field: value[:2] for field, value in entry.fields.items()} == original:
            if specified and entry.specified is None:
                entry.pattern = pattern
                entry.specified = fields
            return
    entries.append(Candidate(fields, pattern, fields if specified else None))


@functools.lru_cache(maxsize=256)
def candidates(locale: Locale) -> tuple[Candidate, ...]:
    """The locale's candidates, in the order ICU reads them."""
    entries: list[Candidate] = []
    for letter in _CANONICAL_ITEMS:
        _add(entries, skeleton(letter), letter, specified=False)
    for style in ("full", "long", "medium", "short"):
        for formats in (locale.date_formats, locale.time_formats):
            pattern = str(formats[style].pattern)
            _add(entries, skeleton(pattern), pattern, specified=False)
    available = locale.datetime_skeletons
    for key in sorted(available):
        _add(entries, skeleton(key), str(available[key]), specified=True)
    return tuple(sorted(entries, key=lambda entry: _bucket(entry.fields)))


def _distance(request: Skeleton, fields: Skeleton, mask: int) -> tuple[int, int, int]:
    distance = missing = extra = 0
    for field in set(request) | set(fields):
        mine = request[field][2] if field in request and mask & (1 << field) else 0
        other = fields[field][2] if field in fields else 0
        if mine == other:
            continue
        if not mine:
            distance += _EXTRA_FIELD
            extra |= 1 << field
        elif not other:
            distance += _MISSING_FIELD
            missing |= 1 << field
        else:
            distance += abs(mine - other)
    return distance, missing, extra


def _best_raw(
    found: tuple[Candidate, ...], request: Skeleton, mask: int
) -> tuple[Candidate, int, int]:
    best = found[0]
    best_distance = best_missing = -1
    best_extra = 0
    for candidate in found:
        distance, missing, extra = _distance(request, candidate.fields, mask)
        if (
            best_distance < 0
            or distance < best_distance
            or (distance == best_distance and best_missing < missing)
        ):
            best, best_distance, best_missing, best_extra = candidate, distance, missing, extra
            if distance == 0:
                break
    return best, best_missing, best_extra


def _adjust(candidate: Candidate, request: Skeleton, hour_length: bool) -> str:
    # The found pattern's fields at the requested widths, as ICU's
    # ``adjustFieldTypes`` gives them: minutes and seconds keep the
    # pattern's, the hour too unless its length is matched, and a width the
    # found pattern's own skeleton settles is kept.
    tokens: list[tuple[str, str | tuple[str, int]]] = []
    for kind, value in tokenize_pattern(candidate.pattern):
        if kind != "field" or not isinstance(value, tuple):
            tokens.append((kind, value))
            continue
        letter, width = value
        row = _row(letter, width)
        if row is None or row[1] not in request:
            tokens.append((kind, value))
            continue
        field = row[1]
        wanted_letter, wanted, _ = request[field]
        if wanted_letter == "E" and wanted < 3:
            wanted = 3
        adjusted = wanted
        if field in (MINUTE, SECOND) or (field == HOUR and not hour_length):
            adjusted = width
        elif candidate.specified is not None and wanted_letter not in "ce":
            own = candidate.specified.get(field)
            own_width = own[1] if own is not None else 0
            own_numeric = own is not None and own[2] > 0
            if own_width == wanted or (row[2] > 0) != own_numeric:
                adjusted = width
        keep = field in _PATTERN_LETTER or (field == YEAR and wanted_letter != "Y")
        tokens.append(("field", (letter if keep else wanted_letter, adjusted)))
    return str(untokenize_pattern(tokens))


_APPEND_PLAIN: Final = frozenset((ERA, YEAR, WEEKDAY, ZONE))


def _best_appending(
    found: tuple[Candidate, ...], request: Skeleton, mask: int, hour_length: bool
) -> str:
    if not mask:
        return ""
    candidate, missing, _ = _best_raw(found, request, mask)
    result = _adjust(candidate, request, hour_length)
    last = -1
    while missing and missing != last:
        last = missing
        candidate, missing, _ = _best_raw(found, request, missing)
        # The supplier carries none of CLDR's append items; the field joins
        # as root's items join a weekday, an era, a year or a zone.
        result = result + " " + _adjust(candidate, request, hour_length)
    return result


def best_pattern(locale: Locale, request: Skeleton, *, hour_length: bool = True) -> str:
    """``getBestPattern``: the pattern a request is shown in at this locale.

    V8 asks with the hour's length matched for component options, and
    without it for a style it asks for again in another hour cycle.
    """
    return _best_pattern(locale, tuple(sorted(request.items())), hour_length)


@functools.lru_cache(maxsize=1024)
def _best_pattern(locale: Locale, fields: tuple[tuple[int, Field], ...], hour_length: bool) -> str:
    # Cached: a choice reads every candidate the locale has, and an
    # application asks for the few it shows over and over.
    request = dict(fields)
    found = candidates(locale)
    if not request:
        return ""
    candidate, missing, extra = _best_raw(found, request, -1)
    if not missing and not extra:
        return _adjust(candidate, request, hour_length)
    mask = 0
    for field in request:
        mask |= 1 << field
    date = _best_appending(found, request, mask & _DATE_MASK, hour_length)
    time = _best_appending(found, request, mask & _TIME_MASK, hour_length)
    if not date or not time:
        return date or time
    month = request.get(MONTH)
    month_width = month[1] if month is not None else 0
    if month_width == 4:
        style = "full" if WEEKDAY in request else "long"
    elif month_width == 3:
        style = "medium"
    else:
        style = "short"
    glue = str(locale.datetime_formats[style])
    return glue.replace("{1}", date).replace("{0}", time)
