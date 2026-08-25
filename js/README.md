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
package itself has no runtime dependencies.

## Status

**Unreleased, and the public surface is unstable.**

These sources are a verbatim move of
[`@sveltekit-i18n/parser-curly`](https://github.com/sveltekit-i18n/parsers/tree/master/parser-curly),
brought over unchanged so that the move itself is reviewable. They still carry
that library's shape: the entry point is a parser factory whose `parse` takes
base's argument tuple, and `@sveltekit-i18n/base` is still a peer dependency.

Reshaping the surface around the format's own resolution inputs — message,
payload, props, locale, key — is the next step, after which nothing here will
reference a host framework and the adapter for `@sveltekit-i18n/base` will live
in that ecosystem's repository. Documentation of the public API lands with that
change; until then the specification and the test suite describe the behavior.

## Development

```bash
npm install
npm test        # builds, typechecks, then runs vitest
npm run lint
```

Requires Node.js 22 or newer.

## License

[MIT](./LICENSE)
