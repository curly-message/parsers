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
* A report says who fixes it. `Report.origin` accompanies `code` — `'message'`
  for a defect in the message that was written, `'payload'` for one in what the
  caller passed, `'limit'` for a bound this parser set. Every code declares
  one, so the axis is the code's own and never the reporting site's. It ranks
  nothing: a report is no graver for coming from one of the three than from
  another.
* A modifier the parser does not know is a defect in the message, not a
  comparison. `{{count:plural; 1:message; default:messages}}` used to run `eq`
  in its place and render `'message'`, answering a question the message never
  asked; it now resolves to the fallback chain and reports `unknown-modifier`.
  Registering the name through `customModifiers` makes it known — a caller's
  table overrides the built-in one, so a host can supply what its messages
  need. What it registers has to be a modifier: an entry that cannot be called
  registers none, so it takes no name of its own and does not shadow the
  built-in it names. The parser's own exports are the registry a caller's table
  composes with, and the `ago` unit ladder was among them, so `{{v:agoMap}}`
  used to answer to a private data table and resolve to the fallback chain in
  silence where `{{v:nosuch}}` reports.
* A modifier that cannot produce a result is reported as `failed-modifier`.
  Containment was already the behavior — a locale the host rejects, a currency
  style with no currency code, a `customModifiers` entry that throws all
  resolve the placeholder to its fallback chain rather than raising out of
  `resolve` — but the caller heard nothing, so a message that quietly rendered
  its default read exactly like one that had no value to render. The
  specification asks for both halves: the failure is contained and it is
  reported.
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
  a `Date` resolve to the fallback chain — the three formatting ones given a
  locale, because with none they resolve to the empty string whatever the value
  is; pass `getTime()` where a placeholder needs the number.
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
* A modifier reads that chain rather than being handed it. Its `defaultValue`
  resolves at the moment of the read, so a modifier that never asks leaves the
  chain unresolved and a link nobody consulted is never reported. A generic
  copy of the config is such a read — a rest destructure, a spread,
  `JSON.stringify` — so a modifier with no use for the default takes the keys
  it needs by name.
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
* A modifier reads the slice of `props` its own name holds. The three layers
  compose as they did, and what a modifier is handed is what that composition
  holds under the name the placeholder wrote: an object of its own properties,
  empty where nobody configured it, rather than the whole modifier-keyed table.
  That is how `modifierDefaults` reaches a host-defined modifier at all — one
  registered as `x-temp` read `props['x-temp']` for its per-call layer and
  could reach the defaults the parser was built with only by walking
  `parserOptions` itself. The slice is the parser's own copy and carries no
  prototype, so a modifier writing into it reaches nothing else and a name
  somebody wrote onto `Object.prototype` configures nothing. Effective
  formatting options are unchanged: the built-in modifiers composed these same
  layers for themselves.
* A wrapper's `props` is checked like the two layers beneath it. The factory's
  props type parameter reaches the wrapper through the payload, so a wrapper
  carries the same modifier-keyed table `modifierDefaults` and a call's `props`
  carry — the built-in modifiers' options plus whatever the host registered.
  A wrapper's `props` of `{ number: { maximumFractionDigits: 'lots' } }` used
  to compile, and then resolved to the fallback chain at runtime.
* A `Modifier.Wrapper` owns at least one wrapper key. All three of `value`,
  `default` and `props` are optional, so the type admitted the empty object,
  which resolution reads as a value and renders as the JSON text `{}` — the
  opposite of what the annotation promised. It is now the union of its three
  branches, each requiring one key and leaving the other two optional. The
  constraint bites where a wrapper is written out or the payload is typed; an
  entry of an untyped payload still collapses to `any`, as it always has.
* Resolution never raises. A modifier that cannot produce a result now resolves
  its placeholder to the fallback chain instead of propagating out of
  `resolve`: `{{price:currency}}` with no currency code configured, and any
  formatting modifier under a locale the host rejects, used to throw.
* A formatting modifier reads its value as the host does, and falls back when
  that conversion fails. `{{when:date}}` over `'tomorrow'` used to render the
  Unix epoch and `{{count:number; default:99;}}` over `0` used to render `99`;
  given a locale, both now resolve to the fallback chain only when the value is
  not a number, and zero formats as zero.
* Blank text is not a number. `+''` is `0`, so `{{v:number}}` over `''`
  rendered `'0'` and `{{v:date}}` over `''` rendered the Unix epoch, formatting
  a value nobody wrote; given a locale, text that is empty, or made only of the
  whitespace the specification enumerates, now resolves to the fallback chain
  in `number`, `date`, `ago` and `currency`. `currency` applied its `ratio`
  before it read the value, which turned blank text into a `0` its guard never
  saw; it reads the value first, then applies the ratio.
* `date` reads a date string, not only a timestamp. `+'2024-03-05T10:00:00Z'`
  is `NaN`, so a date written as text resolved to the fallback chain. Numeric
  text is still a timestamp, and anything else is now left to the host's own
  `Date` parsing, so an ISO string and the form `String(new Date())` writes
  both format. Text that is no date at all — `'tomorrow'`, `''` — still
  resolves to the fallback chain given a locale, and `number`, `ago` and
  `currency` stay numeric.
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
* `ago` accepts the units it can actually resolve to. `format` was typed as the
  whole `Intl` vocabulary, so `quarter` — a unit `Intl` knows and this ladder
  does not climb — compiled and then rendered as `year`, and a typo did the
  same thing silently. The accepted units are now read off the ladder itself,
  so the two cannot drift and an unclimbable unit is a compile error rather
  than a wrong answer.
* Turning a value into text is bounded. A plain object or an array became
  JSON with nothing watching the walk, and both interpolation guards measure a
  string that already exists, so they could not see the cost of building one: a
  payload of twenty-five objects, each of twenty-four levels naming the same
  child twice, holds no cycle for `JSON.stringify` to refuse and describes
  sixteen million leaves. Named by a placeholder it spent over a second before
  reporting `output-limit`; passed as the message itself it never reached the
  interpolation loop at all, because the eighty-eight-megabyte result carries no
  placeholder, and the caller got every character of it with no report. The walk
  now has a node budget of its own — what one conversion may spend, written as
  the output limit's own number — and a value that exhausts it is a value no
  conversion can describe: the placeholder takes the fallback chain and reports
  `unserializable-value`.
* A value converts once per resolution, however many placeholders read it. Each
  placeholder that named a key paid for its own walk of that key's value, and a
  value whose text carries the placeholder that read it paid again on every pass
  and twice as often on each. A payload object of twenty thousand keys valued
  `undefined` — assembled in JavaScript, because `JSON.parse` carries no such
  value — serializes to twenty-two characters after 20002 node visits, so the
  output bound never saw it coming: named by `{{ v }}` while carrying
  `{{ v }}{{ v }}` it was walked 1023 times and spent six seconds, and it still
  ended in an ordinary `pass-limit` report. The text each value came out as is
  now recorded for the length of the `resolve` call, the answer that a value has
  no text included, so what a resolution spends converting is bounded by the
  distinct values it reaches. That covers the host's own string conversion as
  well as the JSON walk: a value the host describes through its own `toString`
  runs it once for the resolution, and one whose `toString` raises is not asked
  a second time, so a value cannot be absent at one placeholder and present at
  the next. A `toString` that answers differently on each call therefore answers
  every placeholder with the text the first read produced. A primitive is not
  recorded — its conversion runs no host code and cannot answer twice over. A
  value built afresh on each read — by a payload getter, or by a custom
  modifier — is a new value every time and is converted every time. The report
  is not recorded with the text: each link that finds none is a defect of its
  own and still reports.
* The call's own inputs are read as own properties. `payload`, `props`,
  `locale` and `key` were read off the context by plain member access, so a
  polluted `Object.prototype` supplied a payload to a call that passed none —
  the own-property rule that protects every payload key, undone one level above
  the payload. They are read like the payload now, and a context of `null` is a
  context nobody passed rather than a `TypeError`: `resolve('a{{v}}b', null)`
  renders `ab`.
* A `props` layer is composed by its own entries, not by its prototype. The
  layer test asked whether an object's prototype was `Object.prototype`, so a
  `props` carried by a class instance — or by anything else with a prototype of
  its own — was discarded whole and the layer beneath it stood alone. A layer is
  now anything carrying entries to read, and its own entries are what compose.
* A polluted prototype configures no formatter. The layers were read as own
  entries and then handed to `Intl` on an ordinary object literal, and a
  formatter reads its options by name — so through that object's prototype:
  with `Object.prototype.style` set to `'percent'`, `{{v:number}}` over
  `1.23456789` rendered `123.46%` instead of `1.23`, `{{v:date}}` took a
  `dateStyle` nobody passed, `{{v:currency}}` took a `currencyDisplay`, and a
  `minimumFractionDigits` above the modifier's own maximum made `Intl` raise, so
  the placeholder reported `failed-modifier` and rendered its fallback. What
  carries the composed layers to a formatter now owns every entry it is
  configured with and answers for no prototype.
* A polluted prototype names no limit in a report either. `Report.limit` was
  read out of a table holding the two codes that reach a limit, so the three
  that reach none read through that table's prototype: with
  `Object.prototype['unknown-modifier']` set, an `unknown-modifier` report
  carried that value as the limit this parser had reached, in a field typed as
  a number and given none of the truncation and terminator escaping `text`
  gets. The table now names all five codes, so no read of it leaves the table.
* A polluted prototype closes no placeholder. Both delimiters are two
  characters, so each scan read one character ahead, and past the end of the
  message that read left the string: with `Object.prototype['4']` set to `'}'`,
  the four-character message `{{v}` was read as a placeholder somebody had
  closed, resolved through the fallback chain and rendered what the payload's
  `default` held instead of the text a translator wrote. The scans read one
  ahead through `charAt` now, which stops at the end of the message.
* A modifier's answer that no conversion describes is reported. Such an answer
  already took the fallback chain, the way a payload value that cannot become
  text does, but only half of that treatment reached it: the placeholder
  rendered its fallback and the caller heard nothing, so a custom modifier
  returning a structure that references itself, or one whose `toString` raises,
  failed invisibly. It now reports `unserializable-value` like the payload value
  it is converted as. An answer that is nothing at all is an absent answer
  rather than one that could not be described, so it still takes the chain
  silently. The code's message reads `A value could not become text, so
  resolution read it as missing.`, which is what it now covers.
* A fallback chain link that refuses to be read is reported. A payload entry, a
  wrapper's `value` or `default`, or the payload's own `default` could be an
  accessor that raises; the read answered "absent" and moved on silently, while
  a raise one level deeper — inside the conversion — reported. Every link now
  reports `unserializable-value`, so a chain that ends early says why.
* The chain a message resolves through reports like the one a placeholder
  resolves through. A message no conversion describes steps to the payload's
  `default` and then to the key, and that step read the payload entry with
  nothing watching: an entry that could not become text, or a getter that
  raised, passed for an entry nobody wrote, and the caller saw its key echoed
  with no report. The link is a payload value like any other and now reports
  `unserializable-value`; the report names no placeholder, because at this level
  there is none, so its `text` is empty and its `key` is what says which message
  went looking. Stepping past the message itself stays unreported: a message
  nothing describes is one nobody wrote, not a defect in the payload.
* A key is echoed, not resolved. Where the chain ran out, the key was handed to
  interpolation like a message, so a key shaped like a placeholder rendered what
  the payload said — `{{name}}` over `{ name: 'Alice' }` resolved to `Alice`,
  and over nothing at all to the empty string — a key carrying `\;` lost the
  backslash to the final unescape pass, and a key naming a value the payload
  could not describe reported a second time. The key echo is what the format
  says when there is no message, not text the format resolves over: it now
  leaves as the caller spelled it. It still becomes text, because `resolve`
  answers with text, so a caller that named a key the host wrote as something
  else keeps the echo it had.
* A host-defined modifier can be given implementation defaults.
  `modifierDefaults` named only the built-in modifiers, so an `x-` modifier the
  factory declares could be handed props per call but never configured at the
  bottom layer without a cast. It now names the same modifiers the call's own
  `props` does. The position does not decide the factory's props parameter —
  `customModifiers` and the explicit argument do — so a parser configured only
  through `modifierDefaults` keeps the context type it had.
* The published types ship the documentation written against them. The build's
  tsconfig asked for comments to be removed and tsup's declaration build reads
  that tsconfig, so every JSDoc block was stripped from `dist/index.d.ts`, and
  `dist` is the whole published package: a consumer installing
  `@curly-message/parser` got a type surface with nothing written on it. The
  option never reached the runtime bundle, which esbuild writes and which comes
  out byte-identical either way — it only deleted documentation.
* `Parser.Options` is the option bag, not the bag or nothing. The published
  type closed with `| undefined`, a union the `?` on the factory's own
  parameter already carried, so a consumer who indexed the type for one option
  — `Parser.Options['onReport']` — or took the bag as a parameter got errors
  about a type that might be absent. Assigning a bag to it and intersecting it
  were unaffected, and whether the factory takes an argument at all is
  unchanged.
* A modifier declares the props it reads. `Modifier.T` and
  `Modifier.CustomModifiers` defaulted their props type parameter to `any`
  where `CommonProps`, `Modifier.Wrapper`, `Parser.PayloadEntry`,
  `Parser.Payload` and `Parser.Context` all default theirs to
  `Modifier.DefaultProps`, so a modifier written down without one read its
  `props` unchecked: `props?.['x-own']?.width` compiled against a layer
  carrying no such name, in the modifier and in a table of them alike. Both
  now default to that same table, and a modifier reading props of its own
  names them — `Modifier.T<MyProps>`, `Modifier.CustomModifiers<Key, MyProps>`
  — as the factory's own props parameter already had to. Nothing about
  resolution changes; a parser that names its props type is unaffected.
* The option bag declares the props its modifiers read. `Parser.Options` kept
  defaulting its props type parameter to `any` after the entries it holds
  stopped, so a bag written down without one typed nothing it carried:
  `customModifiers` took a modifier reading `props?.['x-own']?.width` and
  `modifierDefaults` took a layer named `x-own`, both against a built-in table
  naming neither. It defaults to `Modifier.DefaultProps` now, like every other
  position, and a bag carrying props of its own names them —
  `Parser.Options<Key, MyProps>`. The `parserOptions` a modifier receives is
  typed by the same table that modifier reads, so a host-defined modifier
  reaches its own `modifierDefaults` entry without a cast. The factory supplies
  the parameter itself, so a parser built through `createParser` is unaffected.
