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
modifier could not produce:

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

## Usage

```js
import { createParser } from '@curly-message/parser';

const { resolve } = createParser();

resolve('Hello, {{name; default:Guest;}}!', { payload: { name: 'Alice' }, locale: 'en' });
// -> 'Hello, Alice!'
```

`createParser(options?)` returns a parser whose `resolve(message, context?)`
takes the four inputs resolution is defined over, plus the message's own key:

| Context | Meaning |
| --- | --- |
| `payload` | The values the placeholders name, or the configuration of one — see [Payload](#payload). Its `default` key is the message-wide fallback. |
| `props` | Per-call formatting options handed to the modifiers, keyed by modifier name. A payload entry layers over them. |
| `locale` | The locale the locale-dependent modifiers format for. |
| `key` | The message's identifier. A missing message resolves to the payload's `default`, and to this where the payload carries none, echoed as the caller spelled it. |

A message that no conversion describes is a message that does not exist:
resolution steps past it to the payload's `default` and then to the key, and
reports nothing, because a message nobody wrote is not a defect. The link it
steps to is a payload value like any other, so one that is read and cannot be
described is reported.

The key is where that chain ends, and it is not text the format resolves over.
A key shaped like a placeholder is echoed rather than resolved, an escape
sequence inside one stays as it was spelled, and nothing behind the key is read
a second time. It still becomes text, because `resolve` answers with text.

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
so unset it reports nowhere; resolution still fails soft, it just does so
silently. It is called with a `Report` describing what the parser could not do
— `code`, the `origin` that code declares, an English `message` carrying
nothing from the payload, the `limit` reached where the report is about one,
the message's `key` where one was passed, and `text`, the source of the
trouble: the placeholder that named it for `unknown-modifier`,
`failed-modifier`, `missing-locale` and `unserializable-value`, the output that
would not settle for `pass-limit` and `output-limit`, and nothing at all where
what could not be described is the chain the message itself resolves through,
which names no placeholder. `origin` says who fixes what `code` names — `'message'` for a
defect in the message that was written, `'payload'` for one in what the caller
passed, and `'limit'` for a bound this parser set. Every code declares one, and
it ranks nothing: a report is no graver for coming from one of the three than
from another. Only `text` derives from the payload. It is cut to 120 characters
of what reached it, marked with a trailing `...` of the parser's own where the
cut took something, and escaped after that — quotes, backslashes, and every
line terminator. So a cut excerpt arrives at 123 characters at the shortest,
one carrying something to escape arrives longer still, and no payload can forge
a line where a report is written.

Two guards bound resolution, and reaching either is what the two limit codes
report. A payload value may name another placeholder, so interpolation runs
again over what the last pass produced — at most **10 passes**, after which the
output is returned with its remaining placeholders unresolved. And the output
may not exceed **100000 characters**; a pass that would carry it past that
stops, and the last output under the bound is what resolves. Both are what make
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
each placeholder, and one that raises is not asked a second time. A value built
afresh on each read — by a payload getter, or by a custom modifier — is a new
value every time and is converted every time.

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
names, so a layer cannot reset an earlier one. Only what a layer owns composes,
and the object a modifier is handed owns every entry it is configured with and
carries no prototype, so a prototype somebody else wrote to configures nothing.

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

## Status

**Unreleased, and the public surface is unstable.**

Nothing here references a host framework: `resolve` takes the format's own
inputs, and an adapter that presents this parser to a host library belongs in
that host's own repository.

This implementation satisfies every conformance level the specification
defines: **Core**, **Intl** and **Extensions**. Section 2 asks an
implementation to say so, because a level it does not satisfy changes what a
message resolves to rather than merely what it can do — without Intl, `number`,
`date`, `ago` and `currency` are modifier names nobody registered.

The specification is normative — where this implementation and the
specification disagree, this implementation is wrong.

## Development

```bash
npm install
npm test         # builds, typechecks, lints, then runs vitest
npm run lint:fix # applies what the lint step only reports
```

Requires Node.js 22 or newer.

## License

[MIT](./LICENSE)
