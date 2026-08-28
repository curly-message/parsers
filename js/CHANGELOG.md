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
* Everything a modifier reads is text. A value used to reach a modifier at the
  type the payload gave it while a default arrived as text; both are converted
  the same way now, so a modifier has one kind of input to read. A plain object
  and an array become JSON — `{ v: { a: 1 } }` reaches a modifier as
  `{"a":1}` — and every other value becomes what the host makes of it, so a
  `Date`, a `RegExp` and a class instance keep the text their own `toString`
  writes, and `{{v:date}}` over a `Date` still formats.
* A `Date` reaches a modifier at second precision, because that is what
  `String(date)` writes. The offset survives, so the instant does, but the
  milliseconds do not: `{{v:date}}` over `new Date('2024-03-05T10:00:00.123Z')`
  renders `10:00:00.000` where the same instant written as an ISO string or as
  a timestamp renders `10:00:00.123`. That text is not numeric either, so
  `{{v:number}}`, `{{v:currency}}`, `{{v:ago}}`, `{{v:lt}}` and `{{v:gt}}` over
  a `Date` resolve to the fallback chain; pass `getTime()` where a placeholder
  needs the number.
* A value that is present but cannot become text is reported as
  `unserializable-value`. Resolution reads it as missing and falls through to
  the fallback chain, as it already did, and now says so. The report fires for
  a placeholder's value and for each default link the chain reads and cannot
  convert, never for a key nobody passed or a default nothing consulted —
  neither is a defect.
* A payload entry may carry the value's own configuration instead of the value.
  An entry is that configuration — a wrapper — when it is a plain object owning
  at least one key, every one of them among `value`, `default` and `props`, so
  `{ count: { value: 1, default: 'none' } }` resolves `{{count}}` to `'1'` and
  falls back to `'none'` wherever it carries no value. An entry owning anything
  else is a value, wrapper-shaped or not: `{ value: 1, unit: 'kg' }` and `{}`
  are data and become JSON. Unwrapping happens once, so a wrapper's `value` is
  never read as a wrapper of its own, and the payload's own `default` is always
  a value.
* A placeholder falls back through the payload before it falls back to the
  message. The first link that yields text answers: the wrapper's `default`,
  then the payload's `default`, then the `default` the placeholder declares,
  then the empty string. The message carries the fallback a translator wrote
  and the payload is where a caller overrides it for one call, so
  `{{count; default:no}}` under `{ default: '-' }` now resolves to `'-'` rather
  than `'no'`, and under `{ count: { default: 'none' }, default: '-' }` to
  `'none'`. A link that is missing, or that no conversion turns into text, is
  skipped, and a present value still outranks the whole chain.
* A layer of formatting options cannot reset the layer beneath it. The parser's
  `modifierDefaults` and the `props` a call passes keep the same modifier-keyed
  shape and compose per property: each layer overrides only the properties it
  names, so a property set to `undefined` leaves the layer beneath it standing
  rather than erasing it. A `customModifiers` table composes with the built-in
  one the same way, and one the host will not describe reads as no custom
  modifiers at all rather than raising.
* A wrapper's `props` layer over the `props` the call passes, property by
  property. They keep the same modifier-keyed shape and compose the way the
  parser's `modifierDefaults` and a call's `props` already do: each layer
  overrides only the properties it names and cannot reset an earlier one.
  Under `modifierDefaults` of `{ number: { maximumFractionDigits: 4 } }`, a
  call's `props` of `{ number: { useGrouping: false } }` and a wrapper's
  `props` of `{ number: { maximumFractionDigits: 1 } }`, `{{v:number}}` over
  `1234.56` renders `'1234.6'`.
* Resolution never raises. A modifier that cannot produce a result now resolves
  its placeholder to the fallback chain instead of propagating out of
  `resolve`: `{{price:currency}}` with no currency code configured, and any
  formatting modifier under a locale the host rejects, used to throw.
* A formatting modifier reads its value as the host does, and falls back when
  that conversion fails. `{{when:date}}` over `'tomorrow'` used to render the
  Unix epoch and `{{count:number; default:99;}}` over `0` used to render `99`;
  both now resolve to the fallback chain only when the value is not a number,
  and zero formats as zero.
* Blank text is not a number. `+''` is `0`, so `{{v:number}}` over `''`
  rendered `'0'` and `{{v:date}}` over `''` rendered the Unix epoch, formatting
  a value nobody wrote; text that is empty, or made only of the whitespace the
  specification enumerates, now resolves to the fallback chain in `number`,
  `date`, `ago` and `currency`. `currency`
  applied its `ratio` before it read the value, which turned blank text into a
  `0` its guard never saw; it reads the value first, then applies the ratio.
* `date` reads a date string, not only a timestamp. `+'2024-03-05T10:00:00Z'`
  is `NaN`, so a date written as text resolved to the fallback chain. Numeric
  text is still a timestamp, and anything else is now left to the host's own
  `Date` parsing, so an ISO string and the form `String(new Date())` writes
  both format. Text that is no date at all — `'tomorrow'`, `''` — still
  resolves to the fallback chain, and `number`, `ago` and `currency` stay
  numeric.
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
  — a structure that references itself, a `toString` that raises — resolves to
  the fallback chain instead, and a payload member whose own getter raises when
  it is read counts as missing.
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
  The whitespace it reserves is the class the specification enumerates, not
  the host's own — a host's notion is defined over a Unicode category that
  has changed membership before, which would leave the same message resolving
  two ways on two engines.
* A payload value is unescaped by that same rule. It always went through the
  unescaping pass — a value is rescanned so that it may carry a placeholder of
  its own — but under the narrower set the difference only showed inside a
  placeholder. A value holding `\\server\share` now resolves to
  `\server\share` and one holding `a\ b` to `a b`, so a value that has to keep
  a backslash before a reserved character doubles it; `\d+` is unaffected.
* A backslash reaches the braces themselves, so a brace it consumed cannot be
  half of a delimiter. A pair used to open or close a placeholder whatever
  stood in front of it: `\{{v}}` resolved and rendered `\HIT` over the payload
  `{ v: 'HIT' }`, and `{{v\}}` named the key `v\`, which put a closing brace
  out of reach at the end of a key. Both are text now — `\{{v}}` renders
  `{{v}}`, and `{{v\}}` has no closing pair, so it renders `{{v}}` as well —
  while `{{v\}}}` closes at its last two braces and names the key `v}`.
  Doubling the backslash gives each side its own meaning back: `\\{{v}}` is a
  backslash in front of a placeholder, and `{{v\\}}` names the key `v\`. An
  escape that consumed a lone brace is unchanged, so `{{\{v}}` still names
  `{v` and `{{a\}b}}` still names `a}b`.
* A placeholder holds no line terminator, in any position. A terminator beside
  the key was padding to the pass that substitutes placeholders and a
  rejection to the scan that decides whether to run one, so `"{{\nv\n}}"` was
  text on its own and resolved inside `"{{a}} {{\nv\n}}"` — one construct with
  two answers, decided by what else the message carried. Such a construct is
  text unconditionally now, and carriage return, U+2028 and U+2029 count
  exactly as a newline does. Escaping the terminator still does not make a
  placeholder of it.
* A placeholder that names no key is a placeholder still. `{{}}` was text
  while `{{ }}` resolved, so a single space decided whether the braces meant
  anything at all; `{{}}` now resolves through the same fallback chain as its
  spaced twin, rendering the payload's `default` where the payload carries one
  and the empty string otherwise.
* An inline `default` whose value starts with a colon declares that value.
  `{{count; default::x}}` used to drop the default and resolve to the empty
  string, because the inline default was read by a rule of its own instead of
  being split at its first colon like every other option; it now resolves to
  `':x'`.
* The inline `default` is reserved in lowercase only. The scan that claimed it
  folded case while the filter that removed it from the option list compared
  exactly, so `{{v; DEFAULT:UPPER; default:LOWER}}` rendered `'UPPER'` — one
  segment standing as the inline default and as a comparison option at once,
  with the default the message actually declared discarded. Both gates now
  compare the same spelling, so `DEFAULT:x` is an ordinary option and
  `{{v; DEFAULT:X}}` over `{ v: 'DEFAULT' }` renders `'X'` through `eq`.
* An absent value takes the fallback chain under every comparison, `ne`
  included. `ne` was exempt from the guard, so a placeholder whose key the
  payload does not carry still ran the comparison — against the host's own word
  for absence, the text `undefined` — and `{{v:ne; 10:V2; default:D}}` rendered
  `'V2'` because `10` differs from that word. It renders `'D'` now, and
  `{{v:ne; a:A; b:B}}` the empty string. The old answer was a spelling one
  language happens to use rather than anything the format decides.
* A numeric comparison reads only the options it can order. `lt`, `lte`, `gt`
  and `gte` sorted the whole option list with a comparator that answers `NaN`
  for any key that is not a number, and a `NaN` answer sorts as equal, so one
  such key froze the pairs around it and the wrong option won:
  `{{v:lt; 10:TEN; abc:ABC; 5:FIVE; default:D}}` over `1` rendered `'TEN'`
  where the same message without `abc` rendered `'FIVE'` — the order decided by
  an option that can never be selected. Keys that are not numeric are now left
  out of the ordering entirely, which is also the rule that keeps them
  unselectable, and the ordering runs on a copy, so the list `lte` and `gte`
  hand to their `eq` leg keeps the order the message wrote.
* A zero in `modifierDefaults` is a value, not an absence. The built-in
  formatting defaults were read with `||`, so a parser-level
  `{ number: { maximumFractionDigits: 0 } }` or `{ currency: { ratio: 0 } }`
  was discarded and the built-in `2` and `1` won — the bottom layer resetting a
  property the layer above it named, which is exactly what the layering rule
  forbids. The defaults layer is now read for presence, so `0` composes like
  any other value.
* A failing `onReport` no longer fails the resolution. The diagnostic callback
  ran unguarded, so a host whose logger raises — a full disk, a structured
  logger that cannot serialize a field — turned every reported message into an
  application-level exception. Reporting is an observation, not a step of the
  resolution: the callback is dispatched inside a guard and the message still
  comes back.
