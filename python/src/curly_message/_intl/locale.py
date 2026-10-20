"""The caller's locale, as the supplier's locale data.

The caller's tag is a BCP 47 language tag, read as ECMA-402 reads one. A tag
that is not well formed is a locale the facility cannot use (section 11.2).
One the supplier has no data for is looked up as RFC 4647 looks up a tag,
shortening it from the end, rather than replaced by a default locale: the
process's would depend on the environment it runs in.
"""

from __future__ import annotations

import functools
import pickle
from collections.abc import Mapping, MutableMapping
from typing import Any, Final, cast

from babel import Locale, UnknownLocaleError
from babel.core import get_global, parse_locale
from babel.localedata import Alias, LocaleDataDict, merge, resolve_locale_filename

from .validate import refuse

_HOUR_CYCLES: Final = frozenset(("h11", "h12", "h23", "h24"))


def _alpha(part: str) -> bool:
    return all("a" <= c <= "z" for c in part)


def _digits(part: str) -> bool:
    return all("0" <= c <= "9" for c in part)


def _alphanumeric(part: str) -> bool:
    return all("a" <= c <= "z" or "0" <= c <= "9" for c in part)


def _variant(part: str) -> bool:
    return (5 <= len(part) <= 8 and _alphanumeric(part)) or (
        len(part) == 4 and _digits(part[0]) and _alphanumeric(part[1:])
    )


def _extension_subtag(part: str) -> bool:
    return 2 <= len(part) <= 8 and _alphanumeric(part)


class Tag:
    """A well-formed tag, split into what the supplier reads."""

    __slots__ = ("hour_cycle", "language", "region", "script", "variants")

    def __init__(
        self,
        language: str,
        script: str | None,
        region: str | None,
        variants: tuple[str, ...],
        hour_cycle: str | None,
    ) -> None:
        self.language = language
        self.script = script
        self.region = region
        self.variants = variants
        self.hour_cycle = hour_cycle


def _language_id(
    parts: list[str], index: int
) -> tuple[str, str | None, str | None, tuple[str, ...], int] | None:
    # A ``unicode_language_id`` from ``parts[index]``, as ECMA-402 reads one,
    # and the index after it.
    language = parts[index]
    if not (_alpha(language) and (2 <= len(language) <= 3 or 5 <= len(language) <= 8)):
        return None
    index += 1
    script = region = None
    if index < len(parts) and len(parts[index]) == 4 and _alpha(parts[index]):
        script = parts[index]
        index += 1
    if index < len(parts) and (
        (len(parts[index]) == 2 and _alpha(parts[index]))
        or (len(parts[index]) == 3 and _digits(parts[index]))
    ):
        region = parts[index]
        index += 1
    variants: list[str] = []
    seen: set[str] = set()
    while index < len(parts) and _variant(parts[index]):
        if parts[index] in seen:
            return None
        seen.add(parts[index])
        variants.append(parts[index])
        index += 1
    return language, script, region, tuple(variants), index


def _unicode_extension(subtags: list[str]) -> bool:
    # Attributes, then keywords: every subtag of three to eight characters
    # is an attribute or a type wherever it stands, and a key is two, the
    # second a letter.
    return all(len(subtag) > 2 or _alpha(subtag[1]) for subtag in subtags)


def _transformed_extension(subtags: list[str]) -> bool:
    # An optional ``tlang``, then fields, each a key of a letter and a digit
    # with one or more values of three to eight characters.
    index = 0
    if _alpha(subtags[0]):
        found = _language_id(subtags, 0)
        if found is None:
            return False
        index = found[4]
    while index < len(subtags):
        key = subtags[index]
        if not (len(key) == 2 and _alpha(key[0]) and _digits(key[1])):
            return False
        index += 1
        start = index
        while index < len(subtags) and len(subtags[index]) > 2:
            index += 1
        if index == start:
            return False
    return True


def parse_tag(tag: str) -> Tag | None:
    """``IsStructurallyValidLanguageTag``, and the parts of a tag that passes it."""
    if not tag or not tag.isascii():
        return None
    parts = tag.lower().split("-")
    found = _language_id(parts, 0)
    if found is None:
        return None
    language, script, region, variants, index = found
    singletons: set[str] = set()
    hour_cycle = None
    while index < len(parts):
        singleton = parts[index]
        if len(singleton) != 1 or not _alphanumeric(singleton):
            return None
        index += 1
        if singleton == "x":
            if index == len(parts):
                return None
            if not all(1 <= len(part) <= 8 and _alphanumeric(part) for part in parts[index:]):
                return None
            break
        if singleton in singletons:
            return None
        singletons.add(singleton)
        start = index
        while index < len(parts) and len(parts[index]) > 1:
            if not _extension_subtag(parts[index]):
                return None
            index += 1
        subtags = parts[start:index]
        if not subtags:
            return None
        if singleton == "u":
            if not _unicode_extension(subtags):
                return None
            hour_cycle = _hour_cycle(subtags) or hour_cycle
        elif singleton == "t" and not _transformed_extension(subtags):
            return None
    return Tag(language, script, region, variants, hour_cycle)


def _hour_cycle(subtags: list[str]) -> str | None:
    # The first ``hc`` keyword's value, every type up to the next key joined,
    # where it is a cycle the format reads; a key is two characters, the
    # second a letter, and a type longer.
    for position, subtag in enumerate(subtags):
        if subtag == "hc":
            end = position + 1
            while end < len(subtags) and len(subtags[end]) > 2:
                end += 1
            found = "-".join(subtags[position + 1 : end])
            return found if found in _HOUR_CYCLES else None
    return None


class _Unshared(LocaleDataDict):
    """The supplier's alias-resolving view of its data, writing nothing back.

    The supplier's own view stores each alias it resolves into the dict it
    read it from, and that dict is shared with every locale inheriting it:
    once ``ja`` resolves its stand-alone month names, ``es`` reads them too.
    """

    def __getitem__(self, key: str | int | None) -> Any:
        found = self._data[key]
        # An alias is followed through this view rather than the raw data,
        # where a child's entries merged over an aliased parent's are
        # waiting to be merged in turn.
        if isinstance(found, Alias):
            found = found.resolve(self._view())
        if isinstance(found, tuple):
            alias, others = found
            found = dict(alias.resolve(self._view()))
            merge(found, others)
        if isinstance(found, dict):
            return _Unshared(found, base=self.base)
        return found

    def _view(self) -> _Unshared:
        return _Unshared(cast("MutableMapping[str | int | None, Any]", self.base), base=self.base)

    def copy(self) -> LocaleDataDict:
        return _Unshared(dict(self._data), base=self.base)


def _parent(name: str) -> str:
    # The supplier's inheritance: the exceptions CLDR lists, a language
    # written in a script other than its likely one, then truncation.
    exception: str | None = get_global("parent_exceptions").get(name)
    if exception:
        return exception
    parts = name.split("_")
    if len(parts) == 1:
        return "root"
    if len(parts) == 2 and len(parts[1]) == 4:
        likely = parse_locale(get_global("likely_subtags").get(parts[0], parts[0]))
        if likely[2] != parts[1]:
            return "root"
    return "_".join(parts[:-1])


@functools.cache
def _merged(name: str) -> Mapping[str | int | None, Any]:
    # Read from the supplier's files rather than its cache, whose dicts
    # another view may have written resolved aliases into. Nothing is
    # written into what is cached here: a child copies each dict it merges
    # into, as the supplier's own merge does.
    with open(resolve_locale_filename(name), "rb") as file:
        own: dict[str | int | None, Any] = pickle.load(file)  # noqa: S301 - the supplier's own data files
    if name == "root":
        return own
    data: MutableMapping[str | int | None, Any] = dict(_merged(_parent(name)))
    merge(data, own)
    return data


def _isolated(found: Locale) -> Locale:
    locale = Locale(found.language, found.territory, found.script, found.variant)
    if "_Locale__data" in vars(locale):
        locale._Locale__data = _Unshared(dict(_merged(str(locale))))  # type: ignore[attr-defined]
    return locale


def _parse(parts: list[str]) -> Locale | None:
    try:
        return Locale.parse("-".join(parts), sep="-")
    except UnknownLocaleError:
        return None
    except ValueError:
        # An identifier the supplier's grammar does not read: its last
        # subtag is dropped, as a lookup would drop it.
        return None


@functools.lru_cache(maxsize=256)
def _lookup(
    language: str, script: str | None, region: str | None, variant: str | None
) -> Locale | None:
    # Keyed by what the supplier reads rather than by the tag, so a tag's
    # extensions and private use are not held. A tag the supplier has no
    # data for is cached as well: probing its data costs more than
    # everything else a placeholder does.
    parts = [language]
    if script:
        parts.append(script.title())
    if region:
        parts.append(region.upper())
    if variant:
        parts.append(variant)
    if parts == ["und"]:
        # The supplier reads ``und`` through its likely subtags as English;
        # the root locale is what the tag names.
        return _isolated(Locale.parse("root"))
    while parts:
        found = _parse(parts)
        if found is not None:
            return _isolated(found)
        parts.pop()
    return None


SPELLED_TAG: Final = 64
"""The longest tag whose reading is kept, in code points.

Every Intl placeholder reads its locale, so a tag is read once and kept; a
longer one is read again on each call rather than held, so what the process
keeps of the caller's tags stays bounded.
"""


def _read(tag: str) -> tuple[Locale, str | None] | None:
    parsed = parse_tag(tag)
    if parsed is None:
        return None
    # The supplier's identifiers hold one variant, so a lookup over more
    # would drop each of the others before trying it.
    variant = parsed.variants[0] if parsed.variants else None
    found = _lookup(parsed.language, parsed.script, parsed.region, variant)
    return None if found is None else (found, parsed.hour_cycle)


_kept: Final = functools.lru_cache(maxsize=256)(_read)


def babel_locale(tag: object) -> tuple[Locale, str | None]:
    """The supplier's locale for a tag, and the hour cycle the tag asks for."""
    if not issubclass(type(tag), str):
        refuse()
    text = str.__str__(tag)
    found = (_kept if len(text) <= SPELLED_TAG else _read)(text)
    if found is None:
        refuse()
    return found
