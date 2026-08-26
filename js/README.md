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
result — a rejected locale, a custom modifier that throws — resolves the
placeholder to its `default` rather than raising.

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
| `payload` | The values the placeholders name. Its `default` key is the message-wide fallback. |
| `props` | Per-call formatting options handed to the modifiers, keyed by modifier name. |
| `locale` | The locale the locale-dependent modifiers format for. |
| `key` | The message's identifier. A missing message resolves to it. |

`options` carries `customModifiers`, `modifierDefaults` and `onReport`. Nothing
else is read, and the package has no runtime dependencies — locale-dependent
formatting is delegated to `Intl`.

`onReport` is where diagnostics go. The parser writes to no channel of its own,
so unset it reports nowhere; resolution still fails soft, it just does so
silently. It is called with a `Report` describing what stopped resolution —
`code`, an English `message` carrying nothing from the payload, the `limit`
reached, the message's `key` where one was passed, and `text`, the excerpt that
never settled. Only `text` derives from the payload, and it arrives truncated
with its line terminators escaped, so a report is safe to write anywhere as-is.

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
