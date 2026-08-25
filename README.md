# Parsers

Official implementations of the [Curly Message Format](https://github.com/curly-message/spec)
— a small message syntax for software translations, in which values are
interpolated through double-curly placeholders that may carry a modifier, a set
of options and a fallback.

```json
{
  "greeting": "Hello, {{name; default:Guest;}}!",
  "inbox": "You have {{count:number;}} {{count; 1:message; default:messages;}}."
}
```

## Status

This repository is being seeded. **Nothing here is published yet.**

The specification is a working draft, and the JavaScript implementation is
currently a verbatim copy of
[`@sveltekit-i18n/parser-curly`](https://github.com/sveltekit-i18n/parsers/tree/master/parser-curly),
still shaped around the host library it grew up in. Reshaping it around the
format's own resolution inputs is the next step; until that lands, treat the
public surface as unstable.

## Implementations

| Path | Package | State |
| --- | --- | --- |
| [`js/`](./js) | `@curly-message/parser` | Unreleased |

One directory per language, each a standalone package with its own build,
tests and version line. Release tags are namespaced by directory
(`js-v1.0.0`), so an implementation can leave for its own repository without
that being a breaking change for the others.

## Specification

The format is defined in [`curly-message/spec`](https://github.com/curly-message/spec).
Its machine-readable identifier is `curly-message`; versioned references use
`curly-message-1`, and so on.

## License

[MIT](./LICENSE)
