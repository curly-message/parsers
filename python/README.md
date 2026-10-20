# curly-message

The Python implementation of the
[Curly Message Format](https://github.com/curly-message/spec).

```json
{
  "greeting": "Hello, {{name; default:Guest}}!",
  "inbox": "You have {{count:number}} {{count:plural; one:message; other:messages}}."
}
```

Resolved at the locale `en`:

```
greeting  {"name": "Alice"}  ->  "Hello, Alice!"
greeting  {}                 ->  "Hello, Guest!"
inbox     {"count": 1}       ->  "You have 1 message."
inbox     {"count": 1234}    ->  "You have 1,234 messages."
```

A placeholder may carry a modifier (`:number`, `:date`, `:ago`, `:currency`,
one of the comparisons `eq`, `ne`, `lt`, `lte`, `gt`, `gte`, or one of the
plural selections `plural` and `ordinal`), a set of options, and a `default`.
Resolving a message never raises: a modifier that cannot produce a result, a
value no conversion turns into text, or a modifier nobody registered resolves
the placeholder through its fallback chain, and is reported. The one exception
is a resolution host code asks for while another is in progress on the same
thread of execution, which may pass on the stack running out (*Limits*).

## Installation

```bash
pip install curly-message          # Core and Extensions, with no dependencies
pip install "curly-message[intl]"  # and Intl, over Babel
```

A release candidate installs with `pip install --pre curly-message`.

The `intl` extra installs [Babel](https://babel.pocoo.org/) 2.14 or later,
and `tzdata` for the zone database where the system has none. Intl is on
wherever such a Babel is installed, by this extra or by any other package
that depends on Babel: in an environment that carries one,
`{{n:number}}` formats rather than naming a modifier nobody registered.
`levels()` says which it is.

## Usage

```python
from curly_message import create_parser

parser = create_parser()

parser.resolve("Hello, {{name; default:Guest}}!", {"name": "Alice"})
# -> 'Hello, Alice!'
parser.resolve("You have {{count:number}} new messages.", {"count": 1234}, locale="en")
# -> 'You have 1,234 new messages.'
parser.resolve("{{gender:eq; male:He; female:She; default:They}} replied.", {"gender": "female"})
# -> 'She replied.'
```

`number` here, and `currency` below, are Intl modifiers: they format with the
`intl` extra installed, and without it they are modifiers nobody registered
(see *Levels*).

`parser.resolve(message, payload=None, *, props=None, locale=None,
message_id=None)` takes the inputs resolution is defined over:

| Argument | Meaning |
| --- | --- |
| `payload` | The values the placeholders name, a `dict`. Its `default` entry is the message-wide fallback. |
| `props` | Per-call options for the modifiers, keyed by modifier name: `{"currency": {"currency": "EUR"}}`. A payload entry layers over them. |
| `locale` | The locale the formatting modifiers and the plural selections format for, as a BCP 47 tag. `None` is no locale. |
| `message_id` | The message's identifier. No step reads it; every report carries it. |

A payload entry that is a `dict` whose keys are only `value`, `default` and
`props` is a wrapper: it carries a value with its own default and its own
props. A caller holding untrusted data passes `recognize_wrappers=False`, and
every entry is read as a value.

```python
parser.resolve(
    "{{price:currency}}",
    {"price": {"value": 12.5, "props": {"currency": {"currency": "USD"}}}},
    locale="en",
)
# -> '$12.50'
```

The props of the Intl modifiers are ECMA-402's options, by their ECMA-402
names (`maximumFractionDigits`, `dateStyle`, `timeZone`, and so on).

### Host-defined modifiers

```python
def upper(given):
    return given.value.upper()


parser = create_parser(custom_modifiers={"upper": upper})
parser.resolve("{{name:upper}}", {"name": "ada"})
# -> 'ADA'
```

A modifier receives a `ModifierInput`: `value` (text), `options` (the
placeholder's `Option`s in source order, each with a `key` and a `value`),
`props` (a fresh `dict` composed from `modifier_defaults`, the call's `props`
and the wrapper's own, in that order, property by property), `locale`
(exactly what the caller passed) and `default` (the placeholder's fallback,
computed where it is first read). A `ModifierInput` the modifier keeps
answers, after the resolution returned, what the resolution read of it: a
`default` or an option `value` first read then is the empty string, and
resolves, reports and runs nothing. The modifier's answer becomes text the
way a payload value does. `None` or `UNDEFINED` is no answer, and so is a
raise: the placeholder takes its fallback chain, and a raise is reported as
`failed-modifier`. A name registered here is looked up before the format's
own, so a host may replace `eq` or `number`.

```python
def name(given):
    return (
        "".join(word[0] for word in given.value.split())
        if given.props.get("short")
        else given.value
    )


parser = create_parser(custom_modifiers={"name": name}, modifier_defaults={"name": {"short": True}})
parser.resolve("{{who:name}}", {"who": "Ada Lovelace"})
# -> 'AL'
parser.resolve("{{who:name}}", {"who": "Ada Lovelace"}, props={"name": {"short": False}})
# -> 'Ada Lovelace'
```

### Reports

```python
reports = []
parser = create_parser(on_report=reports.append)
parser.resolve("{{n:shout; default:?}}", {"n": "a"}, message_id="greeting")
# -> '?'
reports[0].code, reports[0].origin, reports[0].text, reports[0].message_id
# -> ('unknown-modifier', 'message', '{{n:shout; default:?}}', 'greeting')
```

`on_report` is the only channel the parser reports through; nothing is
logged. A `Report` is read-only and carries `code` (one of the eight codes of
the specification's section 14.2), `origin` (`"message"`, `"payload"` or
`"limit"`), `message` (an English sentence), `text`, `message_id` and `limit`
(the limit a limit code reached, else `None`). `text` is the placeholder that
reported, escaped as the body of a JSON string (its quotes, backslashes and
C0 control characters too) and, past 120 code points, cut there and ended
with `...`: it is message text, never a payload value. It carries none of
the line terminators of section 6's `line-term` raw, as section 13 asks, and
escapes more than that section asks: also U+0085 and every other code point
the running CPython's `str.splitlines` breaks on, and every lone surrogate,
so a handler that splits lines Python's way or encodes to UTF-8 meets none.
Which further code points end a line is decided by whatever reads what the
handler writes, and escaping those is the handler's. A handler that raises
loses that report and nothing else.

### Describing a message

```python
from curly_message import cst

cst("Hi {{name}}")
# -> {'type': 'message', 'start': 0, 'end': 11, 'nodes': [
#      {'type': 'text', 'start': 0, 'end': 3},
#      {'type': 'placeholder', 'start': 3, 'end': 11, 'nodes': [
#        {'type': 'open', 'start': 3, 'end': 5},
#        {'type': 'key', 'start': 5, 'end': 9, 'name': 'name', 'nodes': [{'type': 'text', 'start': 5, 'end': 9}]},
#        {'type': 'close', 'start': 9, 'end': 11}]}]}
```

`cst` describes a message as it is written: the tree of the specification's
[CST.md](https://github.com/curly-message/spec/blob/main/CST.md), as plain
`dict`s and `list`s, so `json.dumps(cst(message))` is its interchange
encoding. Spans are `[start, end)` in code points, the unit a Python `str`
is indexed in, so they index the `str` the caller holds. The placeholders it
finds are the ones resolution finds, because one scanner finds both.

## Levels

With the `intl` extra installed:

```python
from curly_message import levels

sorted(levels())
# -> ['core', 'extensions', 'intl']
```

This implementation satisfies **Core** and **Extensions** (host-defined
modifiers, props and wrappers) everywhere, and **Intl** where Babel 2.14 or
later is installed. `levels()` answers for the installation it runs in.
Without Intl, `number`, `date`, `ago`, `currency`, `plural` and `ordinal` are
modifier names nobody registered: `{{n:number; default:-}}` resolves to `-`
and reports `unknown-modifier`. `import curly_message` imports no Babel;
the first Intl call does. Where that import fails, as with a Babel missing
what the modifiers import, the placeholder that tried it reports
`failed-modifier` and takes its fallback chain; from then on the installation
satisfies no Intl, so `levels()` leaves it out and the six names report
`unknown-modifier`, as without Babel. A Babel whose installed release cannot
be read counts as not installed. A check or an import that fails to finish,
as on an exhausted stack, answers nothing for later calls, which ask again.

## Limits

| Constant | Value | What it bounds |
| --- | --- | --- |
| `MAX_OUTPUT_LENGTH` | 100000 | code points a resolution writes from placeholders |
| `MAX_READ_LENGTH` | 100000 | code points of value text a resolution reads |
| `MAX_CONVERSION_NODES` | 100000 | nodes one conversion of a structured value visits |
| `MAX_NESTING` | 8 | the deepest level at which a placeholder resolves |

Each keeps what a resolution costs bounded by the message rather than by the
payload, and each reports where it stops (section 13). One more bound is this
implementation's own: a serialization that writes more than 10 000 000 code
points is abandoned as one that reached the conversion limit, and its value
reports `unserializable-value`. The node limit bounds what a serialization
visits, not what it writes, and a list naming one long string many times
visits few nodes. JavaScript's serialization gives up the same way, at its
engine's longest string.

None of them bounds `str()`. A value of any type but `dict` and `list`
converts by its own `str()` (*Values*), which costs what that type's own
code costs, and its text is read whole before the read limit is tested: a
`list` subclass holding the one below it twice, twenty levels deep over the
string `x`, writes 7 340 028 code points. `json.loads` builds no such
value, so only a type the application built reaches it.

Nor does any of them bound a resolution that reaches itself (section 13).
A resolution host code starts while another is running, and an option
`value` or a `default` read from a `ModifierInput` kept from another
placeholder, each go deeper on the interpreter's stack, until CPython raises
`RecursionError`. That is contained where it lands: the placeholder whose
modifier or value it reaches fails as `failed-modifier` or
`unserializable-value` and takes its fallback chain. Where it lands in the
walk of a resolution host code asked for while another is in progress on the
same thread of execution (a thread or a fiber), that resolution lets it pass,
as it does `MemoryError`, to the host code that asked, so the placeholder
that called that code takes its fallback chain; where it lands in the
containment's own work, that placeholder answers `""`. The resolution the
caller asked for, on a thread of execution where none is in progress, never
raises: it answers its text, or `""` where its own walk ran out. The report
is made on the same exhausted stack, so it may not reach the handler. The
recursion limit is the bound, and this implementation keeps none in its
place. A host that raises `sys.setrecursionlimit` past what its C stack
holds has set a limit the stack cannot keep, and the interpreter may then
crash: CPython 3.11 on an 8 MiB stack, at a limit of 60 000, ends the
process with a segmentation fault on a chain of 20 000 kept options.

## Values

The specification reads every value as text (section 4). In Python:

- **`None` is the host's null:** a value, present, which converts to
  `"None"`. A value nobody supplied is spelled by leaving the entry out, or
  by `curly_message.UNDEFINED`, which means the same. Idiomatic Python hands
  over `None` where nothing was meant, so prefer
  `catalogue.get(key, UNDEFINED)` to `catalogue.get(key)`, and leave an entry
  out rather than setting it to `None`:

  ```python
  from curly_message import UNDEFINED

  parser.resolve("{{v; default:none}}", {"v": None})  # -> 'None'
  parser.resolve("{{v; default:none}}", {"v": UNDEFINED})  # -> 'none'

  catalogue = {"greeting": "Hello, {{name; default:Guest}}!"}
  parser.resolve(catalogue.get("farewell"))  # -> 'None'
  parser.resolve(catalogue.get("farewell", UNDEFINED))  # -> ''
  ```

- **Plain objects and arrays are exactly `dict` and `list`**, the types
  `json.loads` produces. One converts to compact JSON
  (`{"a":1,"b":[true,null]}`), its members serialized by `json`'s rules. Any
  other value converts by `str()`: a `tuple`, a `dict` subclass, a
  dataclass, a `datetime`, a `Decimal`. A value whose conversion raises, or a
  structure `json` cannot serialize, is undescribable, and reports
  `unserializable-value`.
- **Only `dict` storage holds entries.** A payload, a props layer, a wrapper
  and the registry are read through `dict`'s own methods, so a subclass's
  `__getitem__`, `get` or `__missing__` never runs, and no entry is ever
  read as an attribute (`{{__class__}}` is a missing key). Any other mapping
  holds no entries: pass `dict(chain_map)`, not a `ChainMap` or a
  `MappingProxyType`.
- **`bool` is not a number**: `True` converts to `"True"`.
- **`eq` and `ne` lowercase both sides with `str.lower()`**, the full
  Unicode mapping of the running CPython's Unicode version: 14.0 on 3.11,
  15.0 on 3.12, 15.1 on 3.13, 16.0 on 3.14, 17.0 on 3.15. A code point
  assigned since compares differently across versions.

## Dates

`date` reads its value as text. Text that is a finite number is
milliseconds since the epoch; any other text is read by the running
CPython's `datetime.fromisoformat`, which reads more than ISO 8601 and does
not read the same on every release:

- **Every supported release reads** what `str()` writes for a `datetime` or
  a `date`, and ISO 8601's calendar and week dates, alone or with a time, in
  the extended or the basic form (`2024-03-05T10:00:00.000Z`,
  `2024-03-05 10:00+01:00`, `20240305T100000Z`, `2024-W10-2`): the time may
  stop at the hour or the minute, the seconds may carry a fraction after a
  point or a comma, and an offset is `Z` or a sign with hours, then
  optionally minutes and seconds, with or without colons, its seconds too
  with a fraction (`+01:00:30.5`). After a time that ends without a
  fraction, a space may stand before the offset: `2024-03-05 10:00:00 +0100`,
  as git writes it, is 10:00 at `+01:00`.
- **Every supported release also reads text that is not ISO 8601**, and not
  always as it looks. Two cases, among others:
  - a date and a time joined by a character other than `T`, a space or a
    digit, a sign among them, so a date followed by a signed offset is a time
    of that day, in the local zone: `2024-03-05x10:00` is 10:00, and
    `2024-03-05+01:00` is 01:00 local time, not midnight at `+01:00` (a date
    followed by `Z` is no date);
  - an extended week date that names its day, followed directly by a digit,
    is that week's Monday, the text from the day on read as its time, and no
    date where that text is no time: `2024-W10-201000` is the Monday at
    20:10, `2024-W10-21` the Monday at 21:00, and `2024-W10-51` no date.
- **Whether other text is a date**, and which instant it is, is the running
  release's to decide, and releases differ on some of it: from Python 3.14
  the hour `24:00` is the next day's midnight; before 3.14 a fraction
  directly after an hour or a minute, of the time or of the offset, is read
  as a fraction of a second (`10.5` is 10:00:00.5, not ISO 8601's 10:30),
  where from 3.14 such text is no date; and a point or a comma before an
  offset (`2024-03-05T10:00:00.+01:00`) is read by some releases and not by
  others, within one minor line too (3.13.0 reads it, 3.13.16 does not).
- **No release reads** a lowercase `z` as an offset (`2024-03-05T10:00z`),
  an ordinal date (`2024-065`), a leap second (`23:59:60`), a month alone
  (`2024-03`) or text whose instant depends on the day it is read
  (`next monday`). A bare date in the basic form (`20240305`) is a
  number, so milliseconds.

A naive date and time, and a bare date, are local time: a bare date is local
midnight, where ECMAScript reads it as UTC. The local zone is the one the C
library reads, and where it comes from is decided once per process, at the
first date: from `TZ` where it is set, a
zone of the database by its name and anything else, a file or a POSIX rule,
by the C library itself, and otherwise from `/etc/localtime`, UTC where
there is none. A zone of the database or of `/etc/localtime` stays the
local zone for the rest of the process; a zone the C library reads in `TZ`
follows the C library's current setting, so a later `time.tzset()` moves it.
Babel's `babel.localtime` names the zone `/etc/localtime` holds where it is
that zone's own file. On Windows the local zone is Babel's reading of the
registry (below), fixed at the first date.
Python's `datetime` spans the years 1 to 9999; a value outside them is one
`date` cannot format, and takes the fallback chain.

## Locales

A locale is read with CLDR's aliases and likely subtags (`zh-TW` is
Traditional Chinese, `iw` is Hebrew), and a tag Babel holds no data for is
cut back one subtag at a time (`en-ZZ` reads as English). `und` reads as the
root locale. A tag nothing of which Babel holds (`zz`) fails the modifier,
and the placeholder takes its fallback chain: there is no fallback to a
default locale, which would make the output depend on the environment.

A request is judged by ECMA-402's rules before Babel is asked, so what
ECMA-402 refuses fails the modifier however lenient Babel would be, and the
placeholder takes its fallback chain: a tag that is not a structurally valid
Unicode BCP 47 locale identifier, UTS #35's `unicode_bcp47_locale_id` as
ECMA-402 reads one (`en_US`, `i-klingon`, `x-private`, `root`), an option an
ECMA-402 constructor refuses (`maximumFractionDigits: 200`, the `unit`
style with no unit, a `timeZone` that names no zone or offset), and, for
`currency`, a currency code that is not three ASCII letters (`US`) or no
code at all, which Babel would write as `US10.00` or with the generic
currency sign. A well-formed code that names no currency is written as itself
(`XYZ10.00`), as ECMA-402 writes it, but without ICU's currency spacing (see
*What Babel cannot express*).

## What Babel cannot express

Section 11.2 lets a host's facility lack a property. Babel lacks these, and
a request that names one formats with the rest:

| Facility | Properties |
| --- | --- |
| `NumberFormat` | `numberingSystem` |
| `RelativeTimeFormat` | `numberingSystem` |
| `DateTimeFormat` | `calendar`, `numberingSystem`, `timeZoneName`, `fractionalSecondDigits` |

Every locale renders Latin digits, a locale whose CLDR default is another
numbering system included: `ar-EG` writes `1,234.5`. Babel's locale data
also differs from ICU's in places, which section 11.2 does not count against
an implementation:

- CLDR's `minimumGroupingDigits` is not applied: `es` writes 1234 as `1.234`,
  where ICU writes `1234`.
- There is no currency spacing: with `currencyDisplay: "code"`, `en` writes
  `EUR12.50` where ICU writes `EUR 12.50`, with a no-break space.
- There are no relative-day phrases: `numeric: "auto"` writes `1 day ago`
  where ICU writes `yesterday`.
- Long and full date styles with a time join them with the plain glue
  (`Tuesday, March 5, 2024, 3:00 PM`), not ICU's `at` form.
- The plural operands of a number at or above 1e28 follow its exact digits.
- A zone the locale names nothing for is written in CLDR's localized GMT
  format, as ICU writes it (`GMT+1`, `GMT+5:30`, `UTC+9` in `fr`), but
  Babel carries no hour format, so every locale writes the offset as an
  ASCII sign and hours and minutes joined by a colon, with no direction
  marks: the minus sign is ASCII where a locale writes another (`fr` writes
  U+2212), the separator is a colon where a locale writes another (`fi`,
  `da`, `id` and `sl` write `+5.30`), a negative offset follows the format's
  text directly where `hr` writes `GMT -1`, and the marks `he` and `fa` put
  around the offset are left out. The zero offset is the format with nothing
  in it (`GMT`).
- Babel's metazones carry no dates, so it names a zone by the metazone it
  belongs to today at any instant. Before 1970 no metazone names a zone, as
  ICU names none there: a zone takes the name the locale gives that zone
  itself where it gives one (`British Summer Time` for London in 1938, `HST`
  for Honolulu, `Coordinated Universal Time` for UTC and each of its links),
  and is written in the localized GMT format otherwise (`GMT-4:56:02` for
  New York in 1874). From 1970 a zone takes the name it has today, which can
  name an offset it did not have: London in 1970, at GMT+1 all year, is
  `Greenwich Mean Time` in the `full` style and `GMT` in the `long` one,
  where ICU writes `GMT+01:00` and `GMT+1`.
- Babel names zones ICU does not: `fr` writes `HNE` for New York where ICU
  writes the localized GMT format, `TU` for UTC where ICU writes `UTC`, and
  `GMT` for London in winter where ICU writes `UTC`.
- A `TZ` naming no zone of the database is read by the C library, as
  Python's `time` module reads it: a file (`/usr/share/zoneinfo/Asia/Tokyo`,
  `Asia//Tokyo`) or a POSIX rule, transitions included.
  `CET-1CEST,M3.5.0,M10.5.0/3` is `GMT+1` in winter and `GMT+2` in summer,
  `JST-9` is `GMT+9`, and a rule naming daylight saving time but no dates
  takes the C library's (`EST5EDT4` is `GMT-5` in winter and `GMT-4` in
  summer, on the dates of the United States). A `TZ` the C library reads
  nothing from is offset zero, written `GMT`, as ICU writes it: an empty
  `TZ`, a misspelled name (`Europe/Pargue`), a file that does not exist, and
  from Python 3.13 a directory of the database (`TZ=Europe`). An offset of a
  day or more, which no `datetime` holds, is offset zero at the instants it
  applies to: `ABC-24` is `GMT`, and `ABC-23DEF,M3.5.0,M10.5.0` is `GMT+23`
  in winter and `GMT` in summer.
- ICU reads `TZ` alike for an offset or a file, but reads a rule naming
  daylight saving time, an offset with minutes (`IST-5:30`) or a name in
  angle brackets (`<+05>-5`) as the zone `/etc/localtime` holds, whatever
  the rule names. It shows an offset of a day or more at that offset with
  no zone name, and names a few abbreviations at offset zero by a zone's
  name: in the `long` style and the `full` one (`PST` is `PST` and
  `Pacific Standard Time`), or in the `full` style alone (`JST` is `Japan
  Standard Time`), where this package writes `GMT`.
- A zone read from a file or a rule, which no identifier names, is written
  in the localized GMT format (`GMT+9`, and `GMT+09:00` in the `full`
  style). With `TZ` unset, an `/etc/localtime` copied from the database
  rather than linked to it is named only where Babel finds that zone's name
  beside it, in `/etc/timezone`; ICU finds the name by searching the
  database (`Japan Standard Time`). An `/etc/localtime` that holds no zone,
  a damaged one included, is UTC, as ICU reads it.
- A `TZ` naming a file that holds no zone or a directory (`TZ=/etc`, and on
  Python 3.11 and 3.12 a directory of the database, `TZ=Europe`), with Babel
  before 2.16 a `TZ` that `zoneinfo` refuses as a key (`Asia//Tokyo`, or an
  absolute path to no file), and, with `TZ` unset, an `/etc/localtime` that
  holds no zone where nothing beside it names one, keep Babel from loading
  any locale data: it reads the local zone as `babel.dates` is imported, and
  its locale data imports `babel.dates`. The first Intl placeholder reports
  `failed-modifier`, and the installation then satisfies no Intl (*Levels*),
  where Node formats as ever: in `GMT` for a directory, and in UTC for a
  file that holds no zone.
- On Windows `TZ` is not read, as Babel reads none, and where Babel maps the
  registry's zone to none, the local zone is the standard offset
  `time.timezone` reads, without daylight saving time.

## Supported Pythons

CPython 3.11, 3.12, 3.13, 3.14 and 3.15, on Linux, macOS and Windows. PyPy
and free-threaded builds are not claimed.

The package ships `py.typed`, and its types are checked against the built
wheel as a consumer's checker sees them. They compute nothing over what a
caller passes, so there is no type-level cost to measure.

## Status

This package implements **`curly-message-3`**, version 3 of the
[Curly Message Format](https://github.com/curly-message/spec), which the
specification states is stable: within that version, what a message resolves
to does not change. The specification is normative: where this
implementation and the specification disagree, this implementation is wrong.
The specification's conformance set holds it to that: the adapter of section
14.3 lives in `tests/conformance/adapter.py`, and the tests run every case the
set ships against it, at every level and over the tree of CST.md.

## Development

From this directory, with [uv](https://docs.astral.sh/uv/):

```bash
uv sync --locked --extra intl
uv run --no-project tasks.py test         # builds, lints, typechecks the source and the wheel, then the suite on both
uv run --no-project tasks.py test-core    # the same suite, without the intl extra
uv run --no-project tasks.py test-lowest  # the same suite, over Babel 2.14.0
uv run --no-project tasks.py test-sdist   # builds, then runs the sdist's own test in the tree it unpacks to
uv run --no-project tasks.py lint-fix     # applies what the lint only reports
uv run --no-project tasks.py bench-venv && node bench/harness.mjs  # measures what the build costs
node bench/harness.mjs --compare ../other/python  # the same, beside another checkout's build
```

The benchmark needs Node.js 22 or newer.

## License

[MIT](https://github.com/curly-message/parsers/blob/main/python/LICENSE)
