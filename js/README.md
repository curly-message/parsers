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

Placeholders may carry a modifier (`:number`, `:date`, `:ago`, `:currency`, or
one of the comparisons `eq`, `ne`, `lt`, `lte`, `gt`, `gte`), a set of options,
and a `default`. Locale-dependent formatting is delegated to `Intl`; the
package itself has no runtime dependencies. A modifier that cannot produce a
result — a locale the host rejects, a custom modifier that throws — resolves
the placeholder to its `default` rather than raising, and so does a value that
no conversion turns into text. A placeholder naming a modifier the parser does
not know resolves to its `default` and is reported; it is never run as a
comparison instead. Given no locale at all, the formatting
modifiers resolve to the empty string.

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
| `key` | The message's identifier. A missing message resolves to it. |

`options` carries `customModifiers`, `modifierDefaults` and `onReport`. Nothing
else is read, and the package has no runtime dependencies — locale-dependent
formatting is delegated to `Intl`.

`onReport` is where diagnostics go. The parser writes to no channel of its own,
so unset it reports nowhere; resolution still fails soft, it just does so
silently. It is called with a `Report` describing what stopped resolution —
`code`, an English `message` carrying nothing from the payload, the `limit`
reached where the report is about one, the message's `key` where one was
passed, and `text`, the excerpt that never settled. Only `text` derives from the payload, and it arrives truncated
with its line terminators escaped, so a report is safe to write anywhere as-is.

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
`{{v:gt}}` over a `Date` resolve to the fallback chain. Pass a timestamp or an
ISO string where a placeholder needs either.

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

Formatting options are keyed by modifier name, and their layers compose per
property: the parser's `modifierDefaults`, then the `props` the call passes,
then the wrapper's own `props`. Each layer overrides only the properties it
names, so a layer cannot reset an earlier one.

```
modifierDefaults  { number: { maximumFractionDigits: 4, useGrouping: false } }
call props        { number: { useGrouping: true } }
wrapper props     { number: { maximumFractionDigits: 1 } }
effective         { maximumFractionDigits: 1, useGrouping: true }  ->  1,234.6
```

## Escaping

The syntax reserves a colon, a semicolon, either brace, a backslash and
whitespace. A backslash takes the structural meaning away from the character
that follows it, and the rule is the same everywhere in a message — inside a
placeholder and in the text around it alike.

```
Braces are written \{\{ like this \}\}      ->  "Braces are written {{ like this }}"
Hello, {{first\ name; default:Guest}}!      ->  names the payload key "first name"
{{count; 1:one\ ; default:none}}            ->  keeps the trailing space
C:\\temp                                    ->  "C:\temp"
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

The specification is normative — where this implementation and the
specification disagree, this implementation is wrong.

## Development

```bash
npm install
npm test        # builds, typechecks, then runs vitest
npm run lint
```

Requires Node.js 22 or newer.

## License

[MIT](./LICENSE)
