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
* A modifier reads a payload `default` at the type the payload gave it. A
  value already reached a modifier unconverted while its default arrived as
  text, so a modifier could not tell `0` from `'0'` on one of its two inputs;
  both now arrive as they were written. A default declared in the message is
  text by nature and is unaffected, and what a placeholder resolves to is
  unchanged — the parser converts it on the way out, as it always did.
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
* An option declares its value at its colon, empty included. `{{count; 1:;
  default:some}}` used to answer with the option's own key and render `'1'`,
  and `{{count; 1: ; default:some}}` — the same intent one space apart — used
  to drop the option and render `'some'`; both now render the empty string the
  option declares. An option that names no value at all is unchanged and still
  stands for itself, so `{{count; 1}}` renders `'1'`.
* Whitespace around an option's value and around an inline `default` is
  layout, not content. `{{count; 1 : one ; default : none }}` rendered
  `'one '` and `'none '`, so a message spaced out for readability leaked that
  spacing into what it resolved to, and only the leading side was ever
  dropped; both sides are now trimmed. Whitespace inside a value is untouched.
* An option's value starts at its first colon and keeps every colon after it.
  `{{shift; 1:10:30; 2:22:00}}` rendered `'30'`, discarding everything but the
  last segment, so a time, a URL or a namespaced identifier had to be escaped
  to survive being written down. Escaping still works and is no longer
  required.
* A backslash escapes every character the syntax reserves, anywhere in a
  message. The escape set was `:`, `;`, `{` and `}`, and only inside a
  placeholder, so a backslash in the surrounding text — or one before
  whitespace or another backslash — had no way to say what it meant. It now
  covers a backslash and whitespace as well and applies to the whole message:
  `a\ b` resolves to `a b`, `\\` to a single backslash, `{{v\ x}}` names the
  payload key `v x`, and `{{count; 1:ONE\ }}` keeps its trailing space instead
  of having it trimmed away. A backslash before anything the syntax does not
  reserve is text, as it was, so `\d+` and `C:\Users\name` survive as typed.
* A payload value is unescaped by that same rule. It always went through the
  unescaping pass — a value is rescanned so that it may carry a placeholder of
  its own — but under the narrower set the difference only showed inside a
  placeholder. A value holding `\\server\share` now resolves to
  `\server\share` and one holding `a\ b` to `a b`, so a value that has to keep
  a backslash before a reserved character doubles it; `\d+` is unaffected.
* An inline `default` whose value starts with a colon declares that value.
  `{{count; default::x}}` used to drop the default and resolve to the empty
  string, because the inline default was read by a rule of its own instead of
  being split at its first colon like every other option; it now resolves to
  `':x'`.
