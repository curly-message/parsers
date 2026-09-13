# @curly-message/parser

The JavaScript implementation of the
[Curly Message Format](https://github.com/curly-message/spec).

```json
{
  "greeting": "Hello, {{name; default:Guest;}}!",
  "inbox": "You have {{count:number;}} {{count; 1:message; default:messages;}}."
}
```

```
greeting  { name: 'Alice' }  ->  "Hello, Alice!"
greeting  {}                 ->  "Hello, Guest!"
inbox     { count: 1 }       ->  "You have 1 message."
inbox     { count: 1234 }    ->  "You have 1,234 messages."
```

A placeholder is written on one line: a `{{` and a `}}` with a line terminator
anywhere between them are text rather than a placeholder, and escaping the
terminator does not make them one. A placeholder that names no key — `{{}}`,
`{{ }}` — is a placeholder still, and resolves through the fallback chain.

Placeholders may carry a modifier (`:number`, `:date`, `:ago`, `:currency`, or
one of the comparisons `eq`, `ne`, `lt`, `lte`, `gt`, `gte`), a set of options,
and a `default`. Locale-dependent formatting is delegated to `Intl`; the
package itself has no runtime dependencies. A modifier that cannot produce a
result — a locale the host rejects, a custom modifier that throws — resolves
the placeholder to its `default` rather than raising, and so does a value that
no conversion turns into text. Neither is silent: containment keeps the failure
out of the caller's render path, and a report is how the caller hears about it
anyway. A placeholder naming a modifier the parser does not know resolves to
its `default` and is reported as well; it is never run as a comparison instead.
A comparison that declares no options has been asked to select from nothing —
the inline `default` is the fallback itself rather than something to select, so
`{{v:eq; default:D}}` declares none either. It resolves to the fallback chain,
as it always did, and reports `missing-options`. That is a defect in how the
placeholder was written, so it reports whether or not the payload carries the
key. The six names are the format's comparisons while the format's own
modifier answers to them: a host that registers its own `eq` has replaced the
comparison, and whether its modifier needs options is that modifier's business,
so `{{v:eq}}` over that registration reports nothing. A placeholder naming no
key has nothing to compare, so `{{:eq}}` is no selection and reports nothing;
`{{:zz}}` still reports the modifier nobody registered.
Given no locale the formatting modifiers resolve to the empty string, not to
the fallback chain: a declared default does not stand in for a locale nobody
supplied. A caller that passes none and a caller that passes the empty string
resolve alike; one that passes a locale the host then rejects has supplied one,
and takes the fallback chain like any other formatting failure. The empty
string is reported as `missing-locale`, whose origin is the payload: a locale
nobody supplied is a defect in what the caller passed rather than in the
message that was written.

Given a locale, each formatting modifier reads its value as a particular kind
of number, and a value that is not one resolves the placeholder to its
`default` and is reported as `failed-modifier`, like any other result a
modifier could not produce. Its origin is the payload too: the value, the props
and the locale a modifier is handed are the caller's, and so is a
`customModifiers` entry that raised, so none of it is a defect in the message
that was written. The kind each reads:

| Modifier | Value |
| --- | --- |
| `number` | a number |
| `date` | milliseconds since the Unix epoch, or text the host can parse as a date |
| `ago` | a signed millisecond delta relative to now, negative for the past |
| `currency` | a number, multiplied by the `ratio` property below |

Empty text and text that is only whitespace are none of these, whatever the
host's own numeric conversion makes of them: `{{v:number}}` over `{ v: '' }`
takes the fallback chain rather than formatting a zero, and `{{v:date}}` over
the same value takes it rather than formatting the epoch.

## Installation

```bash
npm install @curly-message/parser
```

## Usage

```js
import { createParser } from '@curly-message/parser';

const { resolve } = createParser();

resolve('Hello, {{name; default:Guest;}}!', { payload: { name: 'Alice' }, locale: 'en' });
// -> 'Hello, Alice!'
```

`createParser(options?)` returns a parser whose `resolve(message, context?)`
takes the four inputs resolution is defined over, plus the message's own id:

| Context | Meaning |
| --- | --- |
| `payload` | The values the placeholders name, or the configuration of one — see [Payload](#payload). Its `default` key is the message-wide fallback. |
| `props` | Per-call formatting options handed to the modifiers, keyed by modifier name. A payload entry layers over them. |
| `locale` | The locale the locale-dependent modifiers format for. |
| `id` | The message's identifier. No step of resolution reads it; a report names it, so that a report says which message went looking. |

A caller that supplies no message — `undefined`, or a message no conversion
describes — has supplied nothing to resolve, and the resolution is the empty
string. Nothing stands in for it: the payload's `default` is there for the
placeholders of a message, and this message has none. Nothing behind it is read
and nothing is reported, because a message nobody wrote is not a defect. A
message that exists resolves as it is, even where it is empty, and one that is
`0`, `false` or `null` resolves as the text it converts to.

The id is not text the format resolves over and not text it falls back to. One
shaped like a placeholder is neither resolved nor echoed: nothing reads it on
the way to an output, and a report is the one place it is named.

`options` carries `customModifiers`, `modifierDefaults` and `onReport`. Nothing
else is read, and the package has no runtime dependencies — locale-dependent
formatting is delegated to `Intl`.

`customModifiers` registers modifiers by name, over the built-in ones, so a
name it carries is a name a message may write, and so is a name the parser
holds a modifier under already. What it registers has to be a modifier: an
entry that cannot be called registers none, so it takes no name of its own and
does not shadow the built-in it names. Where nothing else answers to the name,
a message writing it reads it as one nobody registered — `unknown-modifier`,
the fallback chain; where a built-in answers to it, that built-in answers as it
did, so `{ eq: 'text' }` costs a caller the name it wrote and nothing further.
The types say as much, but a JavaScript caller reaches the table regardless.

`onReport` is where diagnostics go. The parser writes to no channel of its own,
so unset, or set to `null`, it reports nowhere; resolution still fails soft, it
just does so silently. `null` is for a host that states the silence rather than
omits it. It is called with a `Report` describing what the parser could not do
— `code`, the `origin` that code declares, an English `message` carrying
nothing from the payload, the `limit` reached where the report is about one,
the message's `id` where one was passed, and `text`, the source of the
trouble: the placeholder that named it for `unknown-modifier`,
`failed-modifier`, `missing-options`, `missing-locale` and
`unserializable-value`, the output that would not settle for `pass-limit` and
`output-limit`, the message as it was passed where the read that refused is of
the call's own structure — a context entry, an entry of the option bag. That
one names no placeholder, and a message that is not text carries none of itself
either. `origin` says who fixes what `code`
names — `'message'` for a defect in the message that was written, `'payload'`
for one in what the caller passed, and `'limit'` for a bound this parser set.
Every code declares one, and it ranks nothing: a report is no graver for coming
from one of the three than from another. Only `text` derives from the payload.
It is cut to 120 UTF-16 code units of what reached it — what a string's
`length` counts — or one fewer where the last of them is the high half of a
surrogate pair, so the cut never severs a character. A cut is marked with a
trailing `...` of the parser's own, and the excerpt is escaped after that —
quotes, backslashes, and every line terminator. So a cut excerpt arrives at
122 code units at the shortest, one carrying something to escape arrives
longer still, and no payload can forge a line where a report is written.

Two guards bound resolution, and reaching either is what the two limit codes
report. A payload value may name another placeholder, so interpolation runs
again over what the last pass produced — at most **10 passes**, after which the
output is returned with its remaining placeholders unresolved. And the output
may not exceed **100000 UTF-16 code units** — what a string's `length` counts,
so a character outside the Basic Multilingual Plane counts twice; a pass that
would carry it past that stops, and the last output under the bound is what
resolves. Both are what make
a payload value that references or multiplies its own placeholder terminate
rather than hang the caller.

A third bound holds the conversion that feeds them. Turning a value into JSON
follows a shared reference again every time it meets one, so a value naming the
same child twice at each of twenty-four levels holds twenty-five objects and
describes sixteen million leaves — no cycle anywhere, and nothing an output
bound measured after the fact can prevent. So the walk stops after **100000
nodes**, and the value is read as one no conversion describes: it falls through
its fallback chain and reports as `unserializable-value`. That is what a single
conversion may spend, and a resolution converts one value once however many
placeholders name it, so what a resolution spends converting is bounded by the
values it reaches rather than by the reads it makes of them. That covers the
host's own conversion as well as the JSON walk, so a class instance whose
`toString` runs host code runs it once for the resolution rather than once for
each placeholder, and one that raises is not asked a second time. It covers a
bigint as well, which runs no host code at all but converts to digits the
engine works out afresh on every read. A value built afresh on each read — by
a payload getter, or by a custom modifier — is a new value every time and is
converted every time. Whether an entry is a wrapper is asked the same way:
recognizing one enumerates the entry's own names, work that grows with the
entry, so a resolution asks one entry once however many placeholders name it,
and an entry that refuses the question is not asked again — though it reports
at every placeholder that reads it.

All three bounds belong to the call rather than to the parser, and a call is
what a host begins by calling `resolve` again while one is running — from a
custom modifier, from an `onReport` handler writing its diagnostic into a
translated string, from a payload accessor, or from a value's own `toString`.
Such a resolution counts its own passes, spends its own output and keeps its
own record of what it has converted, so neither it nor the one around it can
reach a bound the other owns, each report names the message its own call was
resolving, and a value the two share is converted once for each of them.
Nothing bounds how deep that goes: a modifier resolving a message that names
it again recurses until the host's own stack runs out, which is contained like
any other failure — the placeholder takes its fallback chain and `resolve`
still answers with text.

## Payload

Everything the format carries is text. A payload value reaches a modifier, and
the output, as text whatever type it was written at: a plain object and an
array become JSON, and every other value becomes what the host makes of it, so
a `Date`, a `RegExp` or a class instance reads as its own `toString` writes it.

```
{ v: 1234.5 }      ->  1234.5
{ v: [1, 2] }      ->  [1,2]
{ v: { a: 1 } }    ->  {"a":1}
{ v: /re/g }       ->  /re/g
```

That conversion costs a `Date` its sub-second precision, because `String(date)`
writes seconds: `{{v:date}}` over `new Date('2024-03-05T10:00:00.123Z')`
renders the same instant with its milliseconds zeroed. The text is not numeric
either, so `{{v:number}}`, `{{v:currency}}`, `{{v:ago}}`, `{{v:lt}}` and
`{{v:gt}}` over a `Date` resolve to the fallback chain — the three formatting
ones given a locale, because with none they resolve to the empty string
whatever the value is. Pass a timestamp or an ISO string where a placeholder
needs either.

A payload entry may carry the value's own configuration — a wrapper — instead
of the value itself:

```js
resolve('You have {{count:number}} points.', {
  payload: {
    count: { value: 1234.56, props: { number: { maximumFractionDigits: 1 } } },
  },
  locale: 'en',
});
// -> 'You have 1,234.6 points.'
```

An entry is a wrapper when it is a plain object that owns at least one key and
every key it owns is `value`, `default` or `props`. An entry owning anything
else is a value, wrapper-shaped or not: `{ value: 1, unit: 'kg' }` and `{}` are
data and become JSON. Unwrapping happens once, so a wrapper's `value` is never
read as a wrapper of its own, and a wrapper carrying no `value` falls back like
a key the payload does not carry. The payload's own `default` is always a
value.

A placeholder resolves to its value wherever the payload carries one, and
otherwise to the first of these that yields text:

1. the wrapper's `default`
2. the payload's `default`
3. the `default` the placeholder declares
4. the empty string

A value no conversion describes — a structure that references itself, a getter
that raises — is read as missing and falls through this chain, and so does any
link in it. Anything the chain reads and could not convert is reported as
`unserializable-value`; a link nothing reaches is never read, so it is never
reported either.

Every entry the parser reads is read as an own **enumerable** property: a
payload key, a wrapper's, a `props` layer's, an entry of the modifier registry,
and the call's own `payload`, `props`, `locale` and `id`. That is the rule the
value conversion has always followed, so a property `Object.defineProperty`
left hidden is not one the format carries anywhere — after
`Object.defineProperty(payload, 'v', { value: 'V' })`, `{{v}}` resolves to its
fallback chain, and a hidden `onReport` reports nowhere. A prototype somebody
else wrote to supplies nothing at any of those reads either.

A read that raises is an absence and a report wherever it sits, not only at a
value. A `props` entry that refuses to be read leaves the layer beneath it
standing and reports `unserializable-value` at the placeholder that needed it;
`customModifiers`, `modifierDefaults` and the context's own entries are read
once for the call, so a report about one names the text that went looking
rather than a placeholder, and an `id` that is itself the entry that raised
leaves the report naming no id.

A modifier's answer becomes text by that same conversion, so an answer no
conversion describes is read as missing and reported the same way, and the
placeholder takes the chain. An answer that is nothing at all is an absent
answer rather than one that could not be described: it takes the chain too, and
says nothing.

A modifier reaches the chain by reading its own `defaultValue`, which resolves
at the moment of that read — running whatever host code the chain carries and
reporting a link it cannot describe. A generic copy of a modifier's config is
such a read, so a rest destructure, a spread or `JSON.stringify` walks a chain
that a modifier taking the keys it needs by name leaves alone.

Formatting options are keyed by modifier name, and their layers compose per
property: the parser's `modifierDefaults`, then the `props` the call passes,
then the wrapper's own `props`. Each layer overrides only the properties it
names, so a layer cannot reset an earlier one: a property set to `undefined`
names nothing, where one set to the host's null is named and null — a value,
like zero — is what the modifier is handed. Only what a layer owns composes,
and only under the name the placeholder wrote: what a layer holds under other
names is not read for it. The object a modifier is handed owns every entry it
is configured with and carries no prototype, so a prototype somebody else wrote
to configures nothing.

```
modifierDefaults  { number: { maximumFractionDigits: 4, useGrouping: false } }
call props        { number: { useGrouping: true } }
wrapper props     { number: { maximumFractionDigits: 1 } }
effective         { maximumFractionDigits: 1, useGrouping: true }  ->  1,234.6
```

A modifier is handed that composition under its own name and nothing else — the
`effective` line is what `number` reads — so what one modifier is configured
with never reaches the next, and a modifier nobody configured is handed an empty
object rather than nothing. A modifier a host registers reads its properties the
same way, `modifierDefaults` included, and the object it holds is the parser's
own copy: writing into it reaches neither the next placeholder nor the caller.

`number` formats at most two fraction digits when no layer names a maximum.
That two is a default rather than a cap: a layer naming a
`minimumFractionDigits` above it widens the default to reach it, the way
`Intl.NumberFormat` widens its own.

`currency` formats in the currency style whatever a layer names as the style:
that style is the modifier rather than one of the options it layers. It
multiplies its value by a `ratio` property first, defaulting to 1, so a payload
carrying minor units renders as major ones.

`ago` takes the unit to count in from a `format` property holding a unit name,
in the singular or the plural: `second`, `minute`, `hour`, `day`, `week`,
`month` and `year` are the rungs of the ladder it climbs. Its `auto`, which is
what a layer naming none leaves in place, selects the unit from the magnitude
of the delta instead. A `format` naming anything else — a unit `Intl` knows and
this ladder does not climb, a rung spelled in another case — is a property the
modifier cannot process: the placeholder takes its fallback chain and reports
`failed-modifier`.

`ratio` and `format` are the format's own properties rather than `Intl`'s, and
both are read from the layers like every other property — a message cannot
write either as an option.

```
{ v: 2 }           { currency: { currency: 'USD', ratio: 100 } }  ->  $200.00
{ v: -172800000 }  { ago: {} }                                    ->  2 days ago
{ v: -172800000 }  { ago: { format: 'hour' } }                    ->  48 hours ago
```

## Escaping

The syntax reserves a colon, a semicolon, either brace, a backslash and
whitespace. A backslash takes the structural meaning away from the character
that follows it, and the rule is the same everywhere in a message — inside a
placeholder and in the text around it alike.

Whitespace means the twenty-five code points the specification enumerates, not
whatever the host calls whitespace: a host's own class is defined over a
Unicode category that has changed membership before.

```
Braces are written \{\{ like this \}\}      ->  "Braces are written {{ like this }}"
Hello, {{first\ name; default:Guest}}!      ->  names the payload key "first name"
{{count; 1:one\ ; default:none}}            ->  keeps the trailing space
C:\\temp                                    ->  "C:\temp"
```

The rule reaches the braces themselves: a brace a backslash consumed is text,
so it cannot be half of a delimiter. That is what lets a key end in a closing
brace; one that starts no pair needs no escape.

```
\{{v}}      ->  "{{v}}"      text, whatever the payload carries
\\{{v}}     ->  a backslash, then the placeholder {{v}}
{{v\}}      ->  "{{v}}"      no closing pair, so the whole run is text
{{v\}}}     ->  names the payload key "v}"
{{v\\}}     ->  names the payload key "v\"
{{a}b}}     ->  names the payload key "a}b"
```

Before anything the syntax does not reserve, a backslash is text itself, so a
regular expression or a Windows path survives as typed: `\d+` resolves to
`\d+`, and `C:\Users\name` to `C:\Users\name`.

A payload value is read the same way, because a value may carry a placeholder
of its own. A value that has to keep a backslash in front of a reserved
character doubles it — `\\server\share` resolves to `\server\share`.

Escape sequences are removed once, from the finished text, so the removal
reaches the text a conversion produced as readily as the text a message was
written in. The two characters JSON writes for a backslash are an escape
sequence, and the removal takes one of them, so the JSON a plain object
serializes to does not necessarily reach the output parsable as JSON.

```
{ v: { a: 'C:\U' } }    serializes to  {"a":"C:\\U"}  and renders  {"a":"C:\U"}
```

A modifier is unaffected, because it reads its value before the removal runs:
one that parses a serialized object back reads the serialization the conversion
produced. A caller that needs the result itself to parse passes the text it
wants as an ordinary string value, with every backslash doubled.

## Extracting parameters

What a message expects of its payload is fixed when the message is written, so
it can be read off the catalogue instead of discovered at render time.
`createExtractor` is that reader. It is a named export beside `createParser`,
over the same scanner, so the two can never disagree about what the syntax is;
a message scanner is of no use while rendering, and the package declares
`sideEffects: false` so a bundle that never reaches it drops it.

```js
import { createExtractor } from '@curly-message/parser';

const extract = createExtractor();

extract('You have {{count:number;}} {{count; 1:message; default:messages;}}.');
// -> [{ name: 'count', kind: 'number', values: ['1'], optional: true }]
```

Each parameter is reported once, in the order the message first names it, and
says what every placeholder naming it says together.

| Field | Meaning |
| --- | --- |
| `name` | The payload key, already unescaped. A key is arbitrary text rather than an identifier, so whatever writes it down quotes it. |
| `kind` | What the parameter accepts, or several kinds where the message reads it in several ways and any of them is valid. |
| `values` | The values the message names explicitly. Absent where it names none. |
| `optional` | Whether the message states a fallback for the parameter. |

`kind` is what the modifier narrows the value to. Every value arrives as text,
so a modifier that reads it as text narrows nothing and the parameter accepts
`unknown` — the top of the lattice rather than a kind of its own.

| Modifier | `kind` |
| --- | --- |
| none, `eq`, `ne` | `'unknown'` |
| `lt`, `gt` | `'number'` |
| `lte`, `gte` | `['number', 'string']` — the equality leg selects on text before the numeric one is reached |
| `number`, `currency` | `'number'` |
| `ago` | `'number'` — a signed millisecond delta relative to now, not a point in time |
| `date` | `['date', 'string']` — milliseconds since the epoch, and failing that text the host reads as a date |

A parameter several placeholders name accepts what all of them say together,
and `unknown` is the top of that lattice rather than a member of it: it is what
a parameter accepts while nothing has narrowed it, and it drops out the moment
something does. So `{{count}}` alone reports `unknown`, and the example above —
where a second placeholder formats the same key with `number` — reports
`'number'` rather than `['unknown', 'number']`.

`values` lists the option keys of an `eq` selection, which is the one
comparison whose keys are values of the parameter: `ne`'s are what the value
must differ from, and an inequality's are thresholds it is ordered against. It
is a hint and never a closed set — a value none of them matches resolves
through the fallback chain rather than failing.

`optional` reports what the message says rather than what resolution tolerates.
Every placeholder renders without its value, an absent one taking the fallback
chain, so a placeholder declaring an inline `default` is the message saying the
value may be missing, and one declaring none is the message saying it is
expected.

An extractor is built from the same options the parser beside it is built from:
a host's own modifier registered under a name this format defines changes what
a message naming it says about its value, and a message naming a replaced
modifier narrows nothing. `modifierDefaults` and `onReport` reach nothing —
extraction formats nothing and reports nothing.

Only the text of a message is scanned. A message that is not text names no
parameters rather than raising, a catalogue leaf being arbitrary data, and a
placeholder a payload value carries into a later pass is not one the message
itself names.

## Status

**Stable.** This package implements **`curly-message-1`**, version 1 of the
[Curly Message Format](https://github.com/curly-message/spec), which the
specification states is stable: within that version, what a message resolves to
does not change.

Nothing here references a host framework: `resolve` takes the format's own
inputs, and an adapter that presents this parser to a host library belongs in
that host's own repository.

This implementation satisfies every conformance level the specification
defines: **Core**, **Intl** and **Extensions**. Section 2 asks an
implementation to say so, because a level it does not satisfy changes what a
message resolves to rather than merely what it can do — without Intl, `number`,
`date`, `ago` and `currency` are modifier names nobody registered.

The specification is normative — where this implementation and the
specification disagree, this implementation is wrong. The specification's
conformance set (`@curly-message/conformance`) holds it to that: the adapter of
section 14.3 lives in `tests/conformance/adapter.ts`, and `npm test` runs
every case the set ships against it, at every level.

## Development

```bash
npm install
npm test         # builds, typechecks, lints, then runs vitest
npm run lint:fix # applies what the lint step only reports
```

Requires Node.js 22 or newer.

## License

[MIT](./LICENSE)
