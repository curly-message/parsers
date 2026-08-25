# Changelog

## Unreleased

Initial version line for `@curly-message/parser`.

* Resolution takes named inputs. `resolve(message, context)` reads `payload`,
  `props`, `locale` and `key` off the context, replacing the host library's
  positional argument tuple, and the factory is the named export
  `createParser` — there is no default export. The payload type is the
  factory's first type parameter.
* No dependency on a host library, in any form. The types the parser needs are
  its own.
