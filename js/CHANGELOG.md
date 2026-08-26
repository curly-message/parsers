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
* A modifier the parser does not know is a defect in the message, not a
  comparison. `{{count:plural; 1:message; default:messages}}` used to run `eq`
  in its place and render `'message'`, answering a question the message never
  asked; it now resolves to the fallback chain and reports `unknown-modifier`.
  Registering the name through `customModifiers` makes it known — a caller's
  table overrides the built-in one, so a host can supply what its messages
  need.
* A payload's own `default` outranks the `default` a placeholder declares.
  The message carries the fallback a translator wrote; the payload is where a
  caller overrides it for one call, so `{{count; default:no}}` under
  `{ default: '-' }` now resolves to `'-'` rather than `'no'`. The inline
  default still applies wherever the payload does not own a `default`, and a
  present value still outranks both.
* Resolution never raises. A modifier that cannot produce a result now resolves
  its placeholder to the fallback chain instead of propagating out of
  `resolve`: `{{price:currency}}` with no currency code configured, and any
  formatting modifier under a locale the host rejects, used to throw.
* A formatting modifier reads its value as the host does, and falls back when
  that conversion fails. `{{when:date}}` over `'tomorrow'` used to render the
  Unix epoch and `{{count:number; default:99;}}` over `0` used to render `99`;
  both now resolve to the fallback chain only when the value is not a number,
  and zero formats as zero.
* Resolution always returns a string. A message that is not one is converted
  rather than handed back as it arrived — `42` resolves to `'42'` and `null` to
  `'null'` — and a resolution that reaches the end of the fallback chain with
  nothing left to fall back to resolves to the empty string.
* An own `default` entry counts as present whatever its value — zero, empty,
  `false`, `null` and `NaN` included — at the placeholder level and at the
  message level alike. Only a key the payload does not own falls further down
  the chain: `{{count}}` under the payload `{ default: 0 }` used to resolve to
  the empty string, and a message that does not exist under that same payload
  used to resolve to its own key; both now resolve to `'0'`. A `default` that
  is not a string renders as text like any other value, so `null` renders as
  `'null'`.
* No value can raise out of resolution. A payload value, a declared `default`,
  a modifier's result or the message itself that no conversion turns into text
  — an object with a null prototype, a `toString` that raises — resolves to the
  fallback chain instead, and a payload member whose own getter raises when it
  is read counts as missing.
