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
* Diagnostics leave through the `onReport` option instead of `console.warn`.
  Reaching either interpolation limit hands the caller a structured `Report`
  and, where no handler is set, reports nowhere at all. Resolution still fails
  soft either way. A report's excerpt of the unsettled text arrives truncated
  and with every line terminator escaped, U+2028 and U+2029 included, so a
  payload cannot forge a line wherever the caller writes it.
* Resolution never raises. A modifier that cannot produce a result now resolves
  its placeholder to the fallback chain instead of propagating out of
  `resolve`: `{{price:currency}}` with no currency code configured, and any
  formatting modifier under a locale the host rejects, used to throw.
* A formatting modifier reads its value as the host does, and falls back when
  that conversion fails. `{{when:date}}` over `'tomorrow'` used to render the
  Unix epoch and `{{count:number; default:99;}}` over `0` used to render `99`;
  both now resolve to the fallback chain only when the value is not a number,
  and zero formats as zero.
