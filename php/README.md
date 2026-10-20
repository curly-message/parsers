# curly-message/parser

The PHP implementation of the
[Curly Message Format](https://github.com/curly-message/spec).

```json
{
  "greeting": "Hello, {{name; default:Guest;}}!",
  "inbox": "You have {{count:number;}} {{count:plural; one:message; other:messages;}}."
}
```

```
greeting  ['name' => 'Alice']  ->  "Hello, Alice!"
greeting  []                   ->  "Hello, Guest!"
inbox     ['count' => 1]       ->  "You have 1 message."
inbox     ['count' => 1234]    ->  "You have 1,234 messages."
```

`inbox` resolves as shown at the locale `en` where ext-intl is loaded (see
[Intl](#intl)). A placeholder may carry a modifier (`:number`, `:date`, `:ago`, `:currency`,
one of the comparisons `eq`, `ne`, `lt`, `lte`, `gt`, `gte`, or one of the
plural selections `plural` and `ordinal`), a set of options, and a `default`.
A modifier that cannot produce a result, a value no conversion turns into text
and a modifier nobody registered all resolve the placeholder to its fallback
chain rather than raising, and each is reported to the caller. The
specification says what every message resolves to; this page says what is
particular to PHP.

## Installation

The package is **unreleased**: nothing is on Packagist yet, and
`composer require curly-message/parser` finds nothing until the first release.
Releases will be served from `curly-message/parser-php`, a read-only mirror of
this directory.

Until then, install it from a checkout of
[`curly-message/parsers`](https://github.com/curly-message/parsers) through a
`path` repository in your project's `composer.json`, which links the checkout's
`php/` directory into `vendor/`:

```json
{
  "repositories": [
    { "type": "path", "url": "../parsers/php" }
  ],
  "require": { "curly-message/parser": "*@dev" }
}
```

It requires PHP 8.2 or newer and no extension beyond those PHP always carries
(`json`, `pcre`, `spl`): no mbstring, no ctype, no iconv. **ext-intl** is
suggested rather than required; with it the package satisfies the Intl level
(see [Intl](#intl)), and without it the six locale-dependent modifiers are not
there at all. PHP 8.2 is supported until its end of life on 2026-12-31; the
first minor release after that date requires PHP 8.3.

## Usage

```php
use CurlyMessage\Parser;

$parser = new Parser();

$parser->resolve('Hello, {{name; default:Guest;}}!', payload: ['name' => 'Alice'], locale: 'en');
// 'Hello, Alice!'
```

A parser is built once with its options and resolves any number of messages.
`resolve` takes the inputs resolution is defined over as named arguments,
each of them optional:

| Argument | Meaning |
| --- | --- |
| `message` | The message. A caller that supplies none, or `Undefined::Value`, resolves to `''`. |
| `payload` | The values the placeholders name, as an `array` or a `stdClass`. Its `default` entry is the message-wide fallback. Anything else is no payload. |
| `props` | Formatting options for the modifiers, keyed by modifier name, as an `array` or a `stdClass`. A payload entry's own props layer over them. |
| `locale` | The locale the locale-dependent modifiers format for: a string, `cs-CZ` or `cs_CZ`. Anything but a string is none. |
| `id` | What each report carries to say which message it came from. No step of resolution reads it. |

Each argument is `mixed`, so nothing a caller passes makes `resolve` throw a
`TypeError`, under `strict_types` or not: a value of the wrong shape reads as
none. `resolve` answers a string, always, with the one exception described
under [Re-entry](#re-entry).

The constructor takes five named options:

| Option | Meaning |
| --- | --- |
| `customModifiers` | Modifiers the host defines, by name: see [Modifiers a host defines](#modifiers-a-host-defines). |
| `modifierDefaults` | The props each modifier is configured with, by its name, beneath every other layer. |
| `onReport` | `Closure(Report): mixed`, called with each report in the order the walk meets it. |
| `onRequest` | `Closure(FormatRequest): mixed`, called with each request an Intl modifier makes, once its facility answered or refused it. |
| `recognizeWrappers` | Whether a payload entry holding only `value`, `default` and `props` configures its value (on by default). |

## Values

Everything the format carries is text, so a payload value reaches a modifier
and the output as text, whatever type it holds:

| PHP value | Text |
| --- | --- |
| `string` | itself, byte for byte |
| `int` | its digits |
| `float` | `(string)`, at the `precision` ini (14 significant digits by default): `0.1 + 0.2` is `0.3`, `1e15` is `1.0E+15` |
| `true`, `false` | `1`, the empty string |
| `null` | the empty string: a value that is present, not a missing one |
| `array` | JSON |
| `stdClass` | JSON |
| any other object | its `__toString`; one with none (a `DateTimeImmutable`, an enum, an `ArrayObject`) has no text |
| `Undefined::Value` | none: the entry is missing |

A float carries no more digits than `precision` writes; a caller that needs
more passes a numeric string. A value that has no text is read as missing: the
placeholder takes its fallback chain, and the condition is reported as
`unserializable-value`.

**Plain data** is an `array`, whatever its keys, and an object whose class is
exactly `stdClass`. Both serialize to JSON as `json_encode` writes them, with
slashes, characters outside ASCII and the two Unicode line terminators written
as themselves: a list is a JSON array, any other array a JSON object, and an
empty array `[]`. So `{}` in PHP is `(object) []`.

```
['v' => [1, 2]]           ->  [1,2]
['v' => ['a' => 1]]       ->  {"a":1}
['v' => []]               ->  []
['v' => (object) []]      ->  {}
['v' => [0 => 'a', 2 => 'b']]  ->  {"0":"a","2":"b"}
```

Inside plain data an object serializes as `json_encode` would serialize it: a
`JsonSerializable` as what its `jsonSerialize()` answers, a backed enum as its
value, a `Closure` as `{}`, and any other object as its public properties, so a
`DateTimeImmutable` nested in an array is `{"date":...,"timezone_type":...,
"timezone":...}`. On PHP 8.4 and newer that includes what `json_encode` runs:
each property hook's `get`, and a lazy object's initializer. That is host code,
and it is contained: one that throws makes the value one with no text. An
entry holding `Undefined::Value` is left out of an object and written `null` in
a list, as JSON writes ECMAScript's `undefined`. A cycle through objects, a pure
enum, a string that is not UTF-8, `INF` and `NAN` have no JSON, so the value has
no text.

A value is converted once per resolution however many placeholders name it, so
an object's `__toString` or `jsonSerialize()` runs once for the resolution, and
the message counts too: `resolve($object, payload: ['x' => $object])` asks
`$object` once. A PHP array has no identity, so an array is converted once for
each place it is read at: each key of the payload, a wrapper's `value` or
`default`, and each host modifier's answers. Read there again, an equal array
answers the text it converted to, and one host code put there since converts,
as far as the resolution compares what it reads again. So an object inside an
array runs its `jsonSerialize()` once for each key that holds the array, where
the JavaScript parser runs it once. A read of a value while its conversion is
under way, from the value's own `__toString` or from a fiber while the one
converting it is suspended, converts nothing: the value has no text there, and
the read reports `unserializable-value`, where the JavaScript parser converts
it again from inside the first. A conversion a fiber was let go in the middle of never
finishes, and the value has no text for the rest of the resolution. A
conversion visits at most 100 000 values, and the JSON it
writes stops at 536 870 888 bytes; past either the value has no text
(see [Limits](#limits)). What an object answers for its text is the object's
text however long, as a string's is.

**Missing and null.** PHP has one `null`, and the format needs two meanings: a
value that is there and empty, and one nobody supplied. `null` is the first:
`['v' => null]` holds `v`, and `{{v; default:D}}` renders the empty string.
`CurlyMessage\Undefined::Value` is the second, and wherever the format reads
the host's undefined it is that: a payload entry holding it is missing, a
wrapper's `value` holding it has no value, a `default` holding it is a link
that is not there, and a layer property holding it is a property the layer
does not name.

**Containers** are read from their own entries only: an `array` by
`array_key_exists` (so a `null` entry is found) and a `stdClass` by its
properties. `__proto__`, `constructor` and `toString` are keys like any other.
No other object is a container, because reading one would run its
`ArrayAccess` or `__get`, so a payload, a layer or a registry of any other
class is none. PHP reads the key `"1"` as `1`, so `{{1}}` finds `['1' => ...]`
and `[1 => ...]` alike.

**Wrappers.** An entry holding nothing but `value`, `default` and `props`
configures its value: `['v' => ['value' => 3, 'props' => ['number' => [...]]]]`.
A caller that passes data it did not write turns that off with
`recognizeWrappers: false`: an entry of that shape is then a value like any
other, and nobody can reconfigure a modifier by writing data in that shape.

## Comparisons

`eq` and `ne` compare the value and each option key in lower case, by
Unicode's own mapping (Default Case Conversion, with the final sigma): PHP's
`strtolower` maps ASCII only, and `mb_strtolower` belongs to an optional
extension and answers differently from one PHP version to the next, so the
package carries the mapping itself, generated from the Unicode Character
Database 17.0.0. No locale is applied and nothing is normalized.

`lt`, `gt`, `lte` and `gte` read the value and the keys as numbers by PHP's
numeric strings: `is_numeric`, so ` 12 `, `1e3` and `.5` are numbers, and
`12px`, `0x1A`, `1_000`, the empty string and `Infinity` are not. A value
that is no number is neither less nor greater than any key; `lte` and `gte`
still select an option whose key equals it, as `eq` would.

## Intl

The locale-dependent modifiers, `number`, `date`, `ago`, `currency`, `plural`
and `ordinal`, are performed by **ext-intl**, which binds ICU.
`Parser::levels()` says which levels this host satisfies: `core`, `intl` and
`extensions` where ext-intl is loaded and ICU's plural rules and relative-time
data are readable, and `core` and `extensions` where they are not. Without
them the six names are not registered at all: `{{n:number}}` is a modifier
nobody registered, reported `unknown-modifier`, and a host may register its own
modifier under any of the six names.

A request is stated as ECMAScript's `Intl` states it, in its vocabulary
(`maximumFractionDigits`, `dateStyle`, `numeric`), and its options are read as
ECMA-402 reads them, with its defaults and its range errors. ext-intl has no
binding for plural rules or relative time, so those two are performed from
ICU's own data, read through `ResourceBundle`, by ICU's own algorithm: plural
operands come from the digits `number` would show, so in Czech `1` is `one`,
and `many` under `minimumFractionDigits: 1`, which shows it as `1.0`; the text
`1.0` is `one`, since `number` shows it as `1`. ECMAScript's default grouping,
which leaves `1234` ungrouped
in Polish where ICU's formatter groups it, is applied from the locale's minimum
grouping digits. `onRequest` observes each request:

```php
use CurlyMessage\FormatRequest;

$parser = new Parser(onRequest: function (FormatRequest $request): void {
    // $request->api, $request->options, $request->input, $request->category
});
```

**Locales.** A locale is checked before any formatter is built. It must be a
Unicode locale identifier (UTS #35), written with `-` or with `_`, so
`cs-CZ`, `cs_CZ` and `de-u-nu-arab` are locales; ICU's own
`de@collation=phonebook` is not, and keywords are written as `-u-` extensions.
A locale that fails the check is one the host rejects: the placeholder takes
its fallback chain and reports `failed-modifier`. A well-formed locale ICU has
no data for formats at PHP's default locale (`Locale::getDefault()`), as
ECMAScript formats at its own. No locale, or the empty string, resolves a
locale-dependent placeholder to the empty string and reports `missing-locale`.

**Dates.** `date` reads a number as milliseconds since the epoch. Text is read
in two steps, the first that reads it deciding: ECMAScript's date time string
format, as `Date.parse` reads it (`2024`, `2024-03`, `2024-03-05` at UTC,
`2024-03-05T10:00` in PHP's default time zone, `2024-03-05T10:00:00.123Z`);
then PHP's own date parser, for text that names a date: `date_parse` reads it
without an error or a warning, with a year, a month and a day, and with nothing
relative but a weekday that is the date's own. So the RFC 2822 form
`Tue, 05 Mar 2024 10:00:00 +0000` is a date, and `now`, `tomorrow`, `+1 day`,
a lone `a` and `Wed, 05 Mar 2024` (a Tuesday) are not.

**What ext-intl cannot express.** `Parser::unexpressible()` lists, by facility,
the properties no value of which can be performed here, and none of them is
read:

| Facility | Properties |
| --- | --- |
| `NumberFormat` | `notation`, `compactDisplay`, `signDisplay`, `unit`, `unitDisplay`, `currencySign`, `localeMatcher` |
| `DateTimeFormat` | `formatMatcher`, `localeMatcher` |
| `RelativeTimeFormat` | `localeMatcher` |
| `PluralRules` | `localeMatcher` |

Below ICU 69, `roundingMode` joins `NumberFormat` and `PluralRules`: the
half-ceiling, half-floor and half-odd modes arrived there.

**What is kept.** Formatters are kept for reuse, at most 256, and what ICU's
data says of a locale, at most 64 locales of each kind, the oldest making room.
A formatter is kept by its locale and by the properties its facility reads,
the ECMA-402 properties it performs; those it cannot express, which
`Parser::unexpressible()` lists, and any other name the props hold are ignored
and keep nothing more. A request whose locale, or the value of a property its
facility reads, is longer than 64 bytes as it is written is built every time
and not kept.
These live in static properties, so under PHP-FPM or mod_php they last one
request, and each request pays for building the first formatter of each locale
and kind it uses.

## Modifiers a host defines

```php
use CurlyMessage\ModifierInput;

function shout(ModifierInput $in): string
{
    return strtoupper($in->value) . '!';
}

$parser = new Parser(customModifiers: [
    'shout' => shout(...),
    'initial' => fn(ModifierInput $in): string => substr($in->value, 0, 1),
]);

$parser->resolve('{{name:shout}} {{name:initial}}', payload: ['name' => 'ada']);
// 'ADA! a'
```

A modifier is a `Closure` or an object with `__invoke`, called with a
`ModifierInput`: the value's text in `value`; the options the placeholder
declares in `options`, each an `Option` whose `key` is its key and whose
`value()` renders its value on the first call; the props the layers hold under
the modifier's name in `props`; the `locale`, or `null`; and `default()`, which
walks the fallback chain on the first call. An option that is never asked for
is never rendered. A text the parser reads itself, the fallback chain of a
modifier that failed or answered nothing, is bounded by the nesting limit and
never throws. A text host code asks for, `value()` or `default()`, from the
modifier's own call or from anywhere else (a modifier that kept it, a fiber, a
report handler, a value's `__toString`), takes a place on its thread of
execution as a `resolve` does (see Re-entry): past 32 there, the read throws
`ReentryLimit` in the host code that asked, which the placeholder that called
that code contains. It takes none of the 512 places the process counts, which
are resolutions': a resolution holds its one place until its `resolve`
returned and no text of it host code asked for is still rendering, in any
fiber, and it renders each of its texts at most once, so what host code reads
of one, in fibers of its own or not, is bounded by its message. A text host
code asks for once nothing of its resolution is in progress takes the place
again, as a `resolve` does. A message nests at most 8 levels, so a
resolution whose host code reads only texts of the input it was handed takes
at most 9 places, its own and one for each level a read renders, and never
meets the bound. Host code that reads, inside one text, a text it kept from
another placeholder re-enters the resolution, as a `resolve` would: each such
read nested inside another on one thread of execution takes a place of its
own there, however shallow the message, so the 32nd of them is refused in a
resolution of any depth. Read each in a fiber of its own, they take no place
but the resolution's, and only the texts the message declares bound them,
each of which renders once (see Memory). The
JavaScript parser bounds such reads by its engine's stack instead, about a
thousand deep under Node's default, so a message that reads more than 31 of
them each inside the one before, or resolutions that resolve each other more
than four messages deep, each nested as deep as a message nests, report
`failed-modifier` here where they resolve there. Host code gets one rendering
of a text at a time: a text it asks for once a rendering of it began and
before one finished, in any fiber, throws `ReentryLimit` too, so a modifier
that reads its kept option again from a placeholder nested in that option, or
a fiber that asks for a text another fiber left suspended mid-render, or let
go of mid-render, reports `failed-modifier` here where the JavaScript parser
renders it again. A text renders once: where a fiber left a rendering of a
fallback chain suspended, or let go of it mid-render, the parser's own read of
that chain resolves to the empty string rather than render it again, which
would double what each level of a message walks; the JavaScript parser cannot
suspend a rendering, so it never meets this. Every read after the one
rendering finished answers its text. A `ModifierInput` is built for one call,
so a modifier may sort `options` or write into `props`; neither reaches
anything else. A function name or an array callable is not a modifier and registers
nothing; `shout(...)` makes a `Closure` of a named function.

A modifier that throws takes the fallback chain and reports `failed-modifier`,
and so does one whose answer, or anything it hangs on its `ModifierInput`,
throws as the parser releases it (a `__destruct`). Whatever a value, a
modifier or a handler throws is released inside the parser too, its own
`__destruct` and those of the arguments its trace holds included, and so is
what the parser read of the payload, the props and the modifiers, which it
holds until the resolution ends: where host code drops its own reference to
an entry while the parser reads it, or the caller hands over what nothing else
holds, the parser lets go of the last one inside the containment. So nothing
they run as they go leaves `resolve`. A `__destruct` PHP's cycle collector
runs is the host's garbage rather than code the parser called, and what it
throws leaves wherever the collector ran, as in any code: out of `resolve`,
or, where the collector ran inside what the parser contains of a placeholder,
as that placeholder's failure.
One that answers `null` or `Undefined::Value` answers nothing, which is PHP's
way for a function to answer nothing (a `return;`, a function that ends without
one), so the placeholder takes its fallback chain and nothing is reported. Any
other answer converts to text as a payload value does. A host modifier under
the name of one of the format's own replaces it.

## Reports

```php
use CurlyMessage\Report;

$parser = new Parser(onReport: function (Report $report): void {
    error_log("{$report->code->value}: {$report->text}");
});

$parser->resolve('{{v:nope; default:D}}', payload: ['v' => 1], id: 'greeting');
// 'D', and onReport is called with unknown-modifier, origin message, id 'greeting'
```

The parser writes nowhere itself: `onReport` is where diagnostics go. A
`Report` carries `code` (a `ReportCode`), the `origin` the code declares
(`message`, `payload` or `limit`), an English `message`, the `id` the
resolution was given, the `limit` for the three codes that reach one (for
`output-limit` below the message, `MAX_TEXT_LENGTH`), and `text`, the
placeholder that reported, as the message spells it. `text` is never payload
text. It is cut to 120 bytes, stepping back over at most three
bytes so the cut never splits a character, marked `...` where cut, and escaped
as a JSON string's contents, with every line terminator the format knows
escaped, so no message can forge a line where a report is written. A handler
that throws is contained, and nothing it answers changes the resolution.

## Limits

| Limit | Constant | Value |
| --- | --- | --- |
| what the placeholders of one resolution place in its output | `Parser::MAX_OUTPUT_LENGTH` | 100 000 bytes |
| value text one resolution reads from its payload | `Parser::MAX_READ_LENGTH` | 100 000 bytes |
| values one conversion visits | `Parser::MAX_CONVERSION_NODES` | 100 000 |
| text the parser writes: one value's JSON, and what a level below the message holds as it places a result | `Parser::MAX_TEXT_LENGTH` | 536 870 888 bytes |
| levels of placeholder resolved, the outermost first | `Parser::MAX_NESTING` | 8 |
| walks in progress on one thread of execution: each resolution, and each option or fallback chain host code asks for, as it renders or after; a fiber is a thread of its own | `Parser::MAX_REENTRY` | 32 |
| resolutions in progress in the process, every fiber's included, suspended or not; a resolution is in progress until its `resolve` returned and no option or fallback chain of it host code asked for is still rendering | `Parser::MAX_IN_PROGRESS` | 512 |

A PHP string is bytes, so the length limits count bytes, what `strlen` counts:
100 000 bytes hold 100 000 ASCII characters and 50 000 two-byte ones.
`MAX_TEXT_LENGTH` is the longest string V8 holds, which V8 counts in UTF-16
code units and this parser in bytes, so a text outside ASCII meets it here at
fewer characters than in the JavaScript parser; section 13 asks a host to hold
at least 100 000 units of text, which it does many times over. A placeholder
whose result would carry the output past its limit resolves to the empty
string and reports `output-limit`, and so does one below the message whose
result would carry its level's text past `MAX_TEXT_LENGTH`; one that begins
after the payload's reading passed its limit resolves to the empty string and
reports `read-limit`; one whose value would serialize to more JSON than
`MAX_TEXT_LENGTH`, or visit more values than a conversion visits, as a list
that holds itself through a reference does, takes its fallback chain and
reports `unserializable-value`; one nested deeper than 8 levels takes its
fallback chain and reports `nesting-limit`. The fallback chain of a placeholder
past that depth is walked on a stack of the parser's own, so a message nested
any deeper resolves on the same number of PHP frames.
A comparison lowers the value only where it is short enough to equal a key, so
`eq` and `ne` over a long value make no text at all.

### Memory

The limits bound what a resolution does by its message, as section 13 asks,
and the text it writes by `MAX_TEXT_LENGTH`; they do not ask how much memory
is free. PHP ends a process that passes `memory_limit` with a fatal error no
`catch` sees, and no code the parser runs can contain it: that is the host
ending the process, as it would for any code, and a host sizes `memory_limit`
for what it resolves. What a resolution holds is bounded by its message, by
`MAX_NESTING`, by `MAX_TEXT_LENGTH` and by the re-entry bounds, and none of it
grows with anything outside the call; within those bounds, these end a process
at PHP's default `memory_limit` of 128M, measured on PHP 8.3 by
[`tools/memory.php`](tools/memory.php) in the shape each row names, which it
runs at another limit too. Another shape holds what its row says it holds, and
ends the process where that comes to 128M:

| What | Ends the process at 128M from about |
| --- | --- |
| a message declaring many options, each held as the placeholder resolves: about 1.6 KB for each | 83 000 options in one placeholder, a message of about 415 KB; less where resolutions re-enter, each holding its own |
| a value whose JSON is long, which the parser writes before it measures what it places: a string shared many times, or a key of characters that escape | 124 MiB of JSON, as one string of 1 MiB shared 124 times, or a key of 13 MiB of control characters, each written as six |
| a long text a level below the message places with text around it, held about three times over as the level writes it (what it places, the level's own text, and the text the level returns) at any depth | an answer, or a value, of about 42 MiB; an answer placed with nothing around it is held once, and ends it from about 124 MiB |
| a recursion that resolves each level in a fiber of its own, stopped at 512 resolutions in progress in the process, each level holding its own message, what its placeholders declare and the options it hands a host modifier | a message that includes itself so: through a host modifier it hands its options to, of about 0.8 KB declaring 158 options; through a value that resolves it again, of about 2.5 KB declaring 496 options; either way, of about 220 KiB of text |
| host code that reads options it kept one inside another, each in a fiber of its own, while the resolution is in progress, which only the texts the message declares bound, each rendering once: about 24 KB for each | about 5 300 options, a message of about 80 KB |
| fibers the host keeps, which the parser does not count unless one is suspended in a resolution or in a text of one host code asked for | about 7 600 fibers, at about 17 KB each, the host's alone |
| a list an Intl modifier's props hold, which it reads as text, as ECMAScript's `String()` joins one, up to `MAX_TEXT_LENGTH` | 124 MiB of text, as one string of 1 MiB shared 124 times |
| a refusal's trace, which refers to the arguments of every frame beneath it unless `zend.exception_ignore_args` is on, as production configurations set it | what the host's frames pass, kept until the refusal is let go |

A thread-safe (ZTS) build keeps each thread's counts apart, as a fiber's are;
it is not tested.

### Re-entry

A host modifier, a report handler or a `__toString` may call `resolve` again,
or ask for an option or a fallback chain (see Modifiers). PHP ends a recursion
that never stops with a fatal error at its memory limit, which no `catch`
sees, so the parser bounds it itself, as section 13 lets an implementation
stand in for the host's own limit: each `resolve`, and each text host code
asks for, takes a place on its thread of execution, and one begun while 32
are taken there throws `CurlyMessage\ReentryLimit` before it reads anything.
A message nests at most 8 levels, so a resolution whose host code reads only
texts of the input it was handed takes at most 9 places, and resolutions that
resolve others beneath them, as an include does, each taking at most 8 to
reach its deepest level, nest four messages deep, each as deep as a message
nests, before the fifth meets it. Host code that reads,
inside one text, texts it kept from another placeholder re-enters the
resolution as such a `resolve` does, so the 32nd such read, each inside the
one before on one thread of execution, meets it in a resolution of any depth.

A fiber has a stack of its own, so it is a thread of execution of its own and
counts its own places: what one fiber holds, suspended or not, takes nothing
from another's 32, and a fiber started or resumed inside a resolution begins
from its own count. So the resolutions in progress in the process are counted
too, and a `resolve` begun while 512 are in progress, every fiber's included,
throws the same `ReentryLimit`. That bounds a recursion that begins each level
in a fiber of its own, as an async host awaiting each call does, which each
fiber's count begins afresh, and what fibers left suspended in resolutions
hold. A resolution is in progress until its `resolve` returned and no text of
it host code asked for is still rendering, so a fiber a modifier left
suspended in an option keeps the resolution's place once it returned. A
resolution renders each of its texts at most once, so it holds one place
however many of them host code reads at once, in fibers of their own or not,
during the resolution or after it, and host code that reads them one inside
another, each in a fiber of its own, nests no deeper than the texts its
message declares (see Memory). A text host code asks for once nothing of its resolution is in progress takes
the place again, as a `resolve` does, so host code that reads texts it kept
from many resolutions, each inside the one before and in a fiber of its own,
meets the bound too. While 512 are in progress, every `resolve` in the process
throws, and so does every such read, in main code and in any fiber, until one
finishes, or until PHP destroys a fiber let go: at
once, as the host lets go of its last reference to it, or, where the host's
own references to it form a cycle, when PHP's cycle collector runs. No code
that does not suspend a fiber in the middle of a resolution, or begin one per
level, reaches it.

`ReentryLimit` is all `resolve` ever throws, but for what a `__destruct` PHP's
cycle collector runs inside it throws (see Modifiers), and a read of an option
or a fallback chain throws it too. Either lands in the host code that called it,
which the placeholder that called that code contains as it contains anything
that code throws: a modifier takes its fallback chain and reports
`failed-modifier`; a value's `__toString` or `jsonSerialize` gives no text, so
its placeholder takes its fallback chain and reports `unserializable-value`;
and a report or request handler's raise changes nothing. The resolution around
it answers as usual.

## Describing a message

`Cst::parse` describes a message as it is written, by the same scan that
resolution reads it with, as the tree
[`CST.md`](https://github.com/curly-message/spec/blob/main/CST.md) specifies.

```php
use CurlyMessage\Cst;

$tree = Cst::parse('Hi {{name}}');
// $tree->type is NodeType::Message, $tree->nodes its text and placeholder

$tree->toArray();
// ['type' => 'message', 'start' => 0, 'end' => 11, 'nodes' => [
//     ['type' => 'text', 'start' => 0, 'end' => 3],
//     ['type' => 'placeholder', 'start' => 3, 'end' => 11, 'nodes' => [...]],
// ]]
```

Every node is a `CurlyMessage\Cst\Node` carrying `type`, `start` and `end`;
`nodes` on a message, a placeholder, a name and an option value; `name` on a
key, a modifier name and an option key; and `cancels` on an escape. Spans are
byte offsets into the PHP string read as UTF-8 (`Cst::UNIT` is `'utf-8'`), so
`substr($message, $node->start, $node->end - $node->start)` is what a node
spans. `toArray()` answers CST.md's interchange encoding as PHP arrays, built
without recursing. `json_encode` of those arrays recurses, and refuses a tree
past its `depth`, 512 by default; give it a depth of `4 * nesting + 8` or
more, where `nesting` is how deep the message nests its placeholders. PHP
frees nested arrays by recursing into them, so the arrays of a tree nested
tens of thousands of levels deep can take the process down when they are
dropped; the `Node`s themselves are released one level at a time, and a tree
that deep is walked through them.

## Conformance

This package implements `curly-message-3`, version 3 of the format, and
satisfies **Core** and **Extensions** on every host, and **Intl** where
ext-intl is loaded: `Parser::levels()` answers which.

Its suite runs the specification's conformance set,
`@curly-message/conformance` 4.1.0, fetched and checked against its published
SHA-256 digest, through a runner of its own, with ext-intl and without it, and
over the set's tree cases. One case is left out:
`comparisons/infinity-compares` reads the text `Infinity` as a number, which is
ECMAScript's numeric conversion; PHP's does not read it, and section 11.2
leaves that conversion to the host.

## Development

Run from this directory:

```bash
composer install       # the package, its suite's dependencies and the tools in tools/qa/
composer test          # fetches the set, lints, analyses, runs the suite, then composer test:dist
composer test:dist     # the archive a release ships, installed by copy and tested there
composer lint          # the formatting contract and the source checks; composer lint:fix fixes the first
composer analyse       # PHPStan at its highest level
composer unicode       # regenerates the case table from the pinned Unicode data
composer bench         # the benchmark; composer bench -- --compare <dir> beside another checkout
php tools/memory.php   # what ends a process at 128M in each shape of Memory; php tools/memory.php <limit> at another
```

The benchmark runs on Node.js 22 or newer, as the family's harness does.

## License

[MIT](./LICENSE)
