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

This repository is being seeded. **It has no implementations in it yet.**

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
