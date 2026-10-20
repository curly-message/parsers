# curly-message-parser

The Rust implementation of the
[Curly Message Format](https://github.com/curly-message/spec).

```rust
use curly_message_parser::{Context, Object, Parser};

let parser = Parser::new();
let greeting = "Hello, {{name; default:Guest;}}!";
let payload = Object::new().with("name", "Alice");

assert_eq!(parser.resolve(greeting, &Context::new().payload(&payload)), "Hello, Alice!");
assert_eq!(parser.resolve(greeting, &Context::new()), "Hello, Guest!");
```

The parser claims the Core level of the format, the syntax, the resolution
and the comparisons `eq`, `ne`, `lt`, `lte`, `gt` and `gte`, and the
Extensions level: the modifiers a host registers. It claims the Intl level
too where it is handed a facility to format with (see [Intl](#intl)). A
placeholder naming a modifier nobody registered resolves to its fallback chain
and reports `unknown-modifier`; it is never run as a comparison instead.

## Resolving a message

`Parser::resolve` takes a message and a `Context` holding what the
resolution reads besides it: the payload, the props and the locale handed to
modifiers, and the id the caller knows the message by. Each is optional. A message is text, or a [`Value`] that
converts to text before anything reads it; an absent message, or one no
conversion describes, resolves to the empty string.

Resolution never fails. A placeholder the payload holds no value for takes
its fallback chain: the wrapper's `default`, then the payload's own
`default` entry, then the `default` the placeholder writes inline, then the
empty string. A `Parser` is cheap to clone and may be shared across threads.

A comparison reads the value and the option keys as text: `eq` and `ne`
compare them ignoring case, and `lt`, `lte`, `gt` and `gte` read both as
numbers the way ECMAScript reads text as a number, so ` 12 ` and `0x10` are
numbers and `12px` and `inf` are not. A comparison that selects nothing takes
the fallback chain. One that declares no options was asked to select from
nothing, and reports `missing-options`.

## Modifiers

A modifier is a function of what a placeholder hands it, registered under the
name placeholders spell it with:

```rust
use curly_message_parser::{Context, ModifierError, ModifierInput, Object, Parser, Value};

let parser = Parser::builder()
    .modifier("initial", |input: &ModifierInput<'_>| {
        match input.value().chars().next() {
            Some(first) => Ok(Some(Value::from(first.to_string()))),
            None => Err(ModifierError::new("nothing to take an initial of")),
        }
    })
    .build();
let payload = Object::new().with("name", "Ada").with("empty", "");
let context = Context::new().payload(&payload);

assert_eq!(parser.resolve("{{name:initial}}.", &context), "A.");
assert_eq!(parser.resolve("{{empty:initial; default:?}}", &context), "?");
```

A modifier is called only for a value the payload holds, and is handed it as
text, with:

* the options in the order the placeholder writes them, a key written twice
  kept twice and the inline default none of them. An option's value is
  message text, rendered when the modifier first asks for it and not again;
  one it never asks for is never rendered.
* the default: the placeholder's fallback chain, walked when the modifier
  first asks for it. A link it never asks for is never read, and never
  reported.
* the props composed under its own name (section 11.2): those the parser was
  built with by `modifier_defaults`, then the context's, then the wrapper's,
  each overriding the properties it names. `Undefined` names nothing, and
  `Null` is a property like any other. They are a [`Props`], read where the
  layers hold them and never copied, so what they hold costs a placeholder
  nothing; `to_object` makes an object of them.
* the locale, as the caller wrote it.

Every accessor borrows the input alone, so a modifier may hold any number of
their answers at once.

`Ok(Some(value))` is the answer, and converts to text as a payload value
does, a structure included. `Ok(None)`, or `Undefined`, is no answer: the
placeholder takes its fallback chain and nothing is reported. `Err(_)` and a
panic are failures: the placeholder takes its fallback chain and reports
`failed-modifier`. A modifier registered under a name the format defines,
`eq` included, replaces the comparison: it is called with whatever options
the placeholder declares, none included.

## Intl

The formatting modifiers `number`, `currency`, `date` and `ago`, and the
plural selections `plural` and `ordinal` (sections 11.2 and 11.5), delegate
to a facility the parser is built with: an [`Intl`], handed a request in the
vocabulary of ECMA-402, the facility, the options it is constructed with and
the input. The crate's own is `IcuIntl`, over [ICU4X](https://github.com/unicode-org/icu4x),
behind the `intl` feature:

```sh
cargo add curly-message-parser --features intl
```

```rust
# #[cfg(feature = "intl")]
# {
use curly_message_parser::{Context, IcuIntl, Object, Parser};

let parser = Parser::builder().intl(IcuIntl::new()).build();
let payload = Object::new().with("n", 1234.5678).with("files", 3);
let context = Context::new().payload(&payload).locale("en");

assert_eq!(parser.resolve("{{n:number}}", &context), "1,234.57");
assert_eq!(
    parser.resolve("{{files:plural; one:# file; other:# files}}", &context),
    "# files"
);
# }
```

The feature configures nothing. A parser claims the Intl level only where
its builder is handed a facility, so one crate enabling `intl` never changes
what another crate's parser resolves: without `.intl(..)`, each of the six
names is a modifier nobody registered. A host may hand a facility of its own,
with or without the feature, and a modifier a host registers under one of the
names still replaces the format's. A facility that refuses a request, or
panics, is a modifier that could not answer: the placeholder takes its
fallback chain and reports `failed-modifier`. With no locale, or an empty
one, a formatting modifier renders the empty string and reports
`missing-locale`.

`IcuIntl` reads the options as ECMA-402 reads them, coercions included: a
property ECMA-402 raises a `RangeError` or a `TypeError` for is a refusal,
and so is a locale ICU4X cannot read (`en_US`). It reads a property as
ECMAScript's ToNumber and ToString would, except that an array or an object
reads as no number and as no text, where ECMAScript would read the text its
`toString` gives.

* **Locale data** is ICU4X 2.3's, compiled into the binary: no file to ship,
  and the linker drops what is not used. It adds about 7 MB to a stripped
  release binary (x86_64 Linux: 0.5 MB without the feature, 7.4 MB with every
  modifier used), most of it time zones and their names. ICU4X 2.3 is a
  locked dependency: with `intl`, an application's graph must hold ICU4X
  2.3.x. What ICU4X has to say about data it lacks goes to the
  [`log`](https://docs.rs/log) facade, which the application routes.
* **Dates** are read as a timestamp first, as section 11.2 orders it, and
  otherwise by this grammar: `YYYY[-MM[-DD]]`, or `+YYYYYY[-MM[-DD]]` or
  `-YYYYYY[-MM[-DD]]`; then optionally `T`, `t` or one space and
  `HH:mm[:ss[.fraction]]`; then, after at most one space, `Z`, `z`, `UTC`,
  or an offset of `+` or `-` and `HH:mm`, `HHmm` or `HH:mm:ss`; then any
  number of bracketed annotations, read and ignored. That reads RFC 3339,
  `Value::Date`'s own text, jiff's `Timestamp` and `Zoned`, chrono's
  `DateTime` and `NaiveDateTime` and the `time` crate's `OffsetDateTime` as
  their `Display` writes them. What names no offset is in UTC.
* **Time zones.** A request naming no `timeZone` formats in UTC: there is no
  host zone to read, and reading the system's would make a message's text
  depend on the machine it is resolved on. `UTC`, `Etc/UTC`, `Etc/GMT` and
  `GMT` are UTC; `+` or `-` and `HH`, `HHmm` or `HH:mm` is a fixed offset;
  and an IANA name is read from
  [jiff](https://github.com/BurntSushi/jiff)'s bundled time zone database at
  the instant formatted. Past the last instant jiff represents (year 9999),
  the offset is the one at the instant whole 400-year Gregorian cycles
  earlier, on which a zone's yearly rule repeats; before the first, it is
  the one there.
* **What `IcuIntl` cannot express** is listed in `UNEXPRESSIBLE`. A request
  naming such a property formats with what is expressed, as section 11.2 has
  it, and never takes the chain on its account:

  | Request | Properties | When |
  | --- | --- | --- |
  | `NumberFormat` | `notation`, `compactDisplay`, `unit`, `unitDisplay`, `localeMatcher` | always |
  | `NumberFormat` | `minimumIntegerDigits`, `minimumFractionDigits`, `maximumFractionDigits`, `minimumSignificantDigits`, `maximumSignificantDigits`, `roundingIncrement`, `roundingMode`, `roundingPriority`, `trailingZeroDisplay`, `useGrouping` | `style` is `currency`: ICU4X formats an amount with the currency's own digits and the locale's grouping |
  | `DateTimeFormat` | `weekday`, `era`, `year`, `month`, `day`, `dayPeriod`, `hour`, `minute`, `second`, `fractionalSecondDigits`, `timeZoneName`, `hour12`, `hourCycle`, `formatMatcher`, `localeMatcher` | always |
  | `RelativeTimeFormat` | `localeMatcher` | always |
  | `PluralRules` | `localeMatcher` | always |

* **Text that differs from Node's.** Where both express a request, the text
  is CLDR's through ICU4X's patterns, and a test holds it to Node 22's
  answers. Those it knows to differ:
  * a time writes CLDR's narrow no-break space before its day period
    (`10:00\u{202f}AM`), where V8 writes a space;
  * with no style, ICU4X pads day and month outside English: `05.03.2024`
    (de), `05. 03. 2024` (cs), `2024/03/05` (ja), `05-03-2024` (nl),
    `05.03.2024` (pl), where Node writes `5.3.2024`, `5. 3. 2024`,
    `2024/3/5`, `5-3-2024`, `5.03.2024`;
  * before year 1, ICU4X shows the era (`1/1/2 BC`), which Node omits;
  * with `dateStyle: "short"`, a year outside 1950 to 2049 is written in full
    (`6/15/2050`, `15.06.2050` in de), where Node writes two digits
    (`6/15/50`, `15.06.50`);
  * Czech `timeStyle: "full"` has no comma before the zone name;
  * English digits of the `arab` numbering system keep the Latin grouping
    separator.

The ICU4X crates are licensed under
[Unicode-3.0](https://www.unicode.org/license.txt), and jiff and its time
zone database under MIT or the Unlicense; enabling `intl` brings their terms
into the binary.

## Reports

What a resolution could not resolve leaves through the handler a parser is
built with, and nowhere else:

```rust
use std::sync::{Arc, Mutex};

use curly_message_parser::{Code, Context, Parser};

let codes = Arc::new(Mutex::new(Vec::new()));
let sink = Arc::clone(&codes);
let parser = Parser::builder()
    .on_report(move |report| sink.lock().unwrap().push(report.code))
    .build();

assert_eq!(parser.resolve("{{count:sum; default:none}}", &Context::new()), "none");
assert_eq!(*codes.lock().unwrap(), [Code::UnknownModifier]);
```

A report carries its code, the origin the code declares, the id of the
resolution, the limit it reached where it reached one, and the placeholder
that reported as the message spells it: never a payload value. That text is
cut to `MAX_REPORTED_LENGTH` bytes on a character boundary and escaped as the
contents of a JSON string, every line terminator included, so a message
cannot forge a line wherever a report is written. Reports are emitted in walk
order. A handler that panics is contained, and the resolution carries on.

Containment catches a panic as it unwinds, a modifier's or a `Display`'s as
much as a handler's, so it needs the default
`panic = "unwind"`; under `panic = "abort"` a panic ends the process wherever
it happens. The process's panic hook still runs for a contained panic, and
prints what it prints for any other.

## Limits

| Constant | Value | Bounds |
| --- | --- | --- |
| `MAX_OUTPUT_LENGTH` | 100 000 | bytes of UTF-8 of placeholder results the output carries; the message's own text is not counted and always renders |
| `MAX_READ_LENGTH` | 100 000 | bytes of UTF-8 of value text a resolution reads |
| `MAX_CONVERSION_NODES` | 100 000 | nodes one conversion of a value visits |
| `MAX_SERIALIZED_LENGTH` | 536 870 888 | code units of UTF-16 one serialization of a value writes |
| `MAX_NESTING` | 8 | levels a placeholder may nest and still resolve |
| `MAX_HOST_DEPTH` | 16 | walk activations one thread holds at once |

A placeholder nested past `MAX_NESTING` takes its fallback chain and reports
`nesting-limit`. The walk keeps what it nests on a stack of its own, so a
message nested any depth resolves without a call per level, and so does a
value nested any depth convert. What does reach the thread's stack is what a host's
code starts from inside a resolution: an option or the default a modifier
asks for, and a resolution a modifier, a report handler or a value's `Display`
starts. Each holds a walk activation, and one that would take the thread past
`MAX_HOST_DEPTH` does not start. It answers the empty string, and the call it
was started from is treated as one that did not come back: a modifier's
reports `failed-modifier`. A test reaches `MAX_HOST_DEPTH` along each of
these ways on a thread of 512 KiB, in a debug build. On x86-64 Linux the
deepest takes about 240 KiB there, a `Display` of a date's option that the
`intl` feature reads, and about 70 KiB in a release build, where the deepest
is a `Display` of an option of `plural`.

## Values

A payload is an [`Object`]: its own entries, in the order they were first
inserted. Every key is a name and nothing else, `__proto__` included.

```rust
use curly_message_parser::{Object, Value};

let payload = Object::new()
    .with("name", "Ada")
    .with("count", 3)
    .with("nickname", None::<&str>);

assert_eq!(payload.get("name").and_then(Value::as_str), Some("Ada"));
assert!(payload.get("nickname").is_some_and(Value::is_undefined));
assert!(payload.get("__proto__").is_none());
```

| Value | Holds |
| --- | --- |
| `Undefined` | nothing: an entry nobody passed; `None` converts to it |
| `Null` | the null value |
| `Bool` | a boolean |
| `Number` | a double |
| `Integer`, `Unsigned` | an integer of up to 64 bits |
| `String` | text |
| `Date` | an instant authored as a date, in milliseconds since the epoch |
| `Array`, `Object` | the plain shapes |
| `Host` | a value of the application's own type, read by its `Display` |

A value converts to text as section 4 of the format has it: `Null`, `Bool`
and the numbers as ECMAScript writes them (`1e+21`, `0.5`), an `Integer` or
`Unsigned` in all its digits, a `Date` as an ISO 8601 instant in UTC or
`Invalid Date`, an `Array` or `Object` as JSON with its members in insertion
order, and a `Host` value through its `Display`. An `Undefined` member is left
out of an object and written as `null` in an array, as is a number that is not
finite. A `Display` that fails or panics describes nothing, and neither does a
container whose serialization visits more than `MAX_CONVERSION_NODES` nodes,
nor a `Display` or a serialization that would write more than
`MAX_SERIALIZED_LENGTH` code units of UTF-16, the most a string holds in V8,
where the JavaScript implementation meets the same bound: the placeholder
takes its fallback chain and reports `unserializable-value`. What would pass
the bound is refused there, never written first.
The second bound is what a container naming one large child many times meets,
which visits few nodes. A value
is converted once per resolution however many placeholders read it.

A payload entry holding only `value`, `default` and `props` is a wrapper: it
configures the value it holds rather than being one (section 4.1). A caller
whose payload holds entries it did not write turns that off with
`Parser::builder().recognize_wrappers(false)`.

A value nested a million levels deep is dropped without a call per level.

## The tree

`cst` describes a message as it is written: where its placeholders are, what
each is made of and where every escape sequence falls, as the format's
[concrete syntax tree](https://github.com/curly-message/spec/blob/main/CST.md)
spells it.

```rust
use curly_message_parser::{NodeKind, cst};

let message = "Hi {{name; default:you}}";
let tree = cst(message);
let placeholder = tree.root().children().nth(1).unwrap();

assert_eq!(placeholder.kind(), NodeKind::Placeholder);
assert_eq!(&message[placeholder.span()], "{{name; default:you}}");
```

Spans are in bytes of UTF-8, the unit a `str` is indexed by, so a span slices
the message it came from and never falls inside a character. A message nested
any depth is described, and its tree dropped, without a call per level; the
scan reads a message in time linear in its length. The tree and the resolution
share that scan, so the placeholders the tree describes are the ones a
resolution resolves.

## Conformance

The specification is normative: where this implementation and the
specification disagree, this implementation is wrong. The specification's
conformance set holds it to that. Release 4.1.0 of the set is vendored in the
repository, checked against its digest before it is read, and
`cargo test -p curly-message-parser` runs every case it ships through the
adapter of section 14.3 (`tests/conformance/adapter.rs`), one test per case on each reading below:

| Level | Cases | Without `intl` | With `intl` |
| --- | --- | --- | --- |
| Core | 459, 8 of them built from the limits | 459 passed | 459 passed |
| Extensions | 53 | 53 passed | 53 passed |
| Intl | 215 | left out: the parser claims no Intl level | 215 passed |
| The tree of `CST.md` | 44 | 44 passed | 44 passed |

A case that states a formatting request rather than its text is read two
ways with `intl`, and passes on both. On the first, the parser exposes the
requests it made and each is compared with the one the case states: the
facility, the properties and the input. On the second, the output is held to
the text Node 22.22.0 (ICU 77.1, CLDR 47.0) made of the case's request,
recorded in `conformance/set/golden-4.1.0.json` by `conformance/golden.mjs`.
One of the set's requests is spelled otherwise by ICU4X than by V8: in
`intl/date-layers-compose`, CLDR writes a narrow no-break space before `AM`,
which V8 writes as a space. That case is measured on its request on both
readings, and a test holds that list to exactly the requests whose text
differs.

The runner is audited too. Each of the 26 defects of the set's catalogue,
wrapped around the adapter, must fail a case: with `intl` all 26 do, on both
readings; without it, 24 do, and the two that concern formatting requests
(an extra request, a property declared unexpressible) reach no case.

## Benchmark

The benchmark measures the release build. From `rust/`, `sh bench/prepare.sh`
builds it into `bench/dist/`, and `node bench/harness.mjs` measures the rows
of `bench/rows.mjs` on it; `--compare <dir>` sets each row beside the same row
of the crate checked out and prepared at `<dir>`. The harness is the family's,
the same file in every package, and runs on Node; the rows read the build
through `curly-message-bench`, a program of this workspace that runs a row's
workload a fixed number of times per request, named in the row.

* **Sizes:** the `.crate` a release ships, and a stripped program resolving
  one message, without `intl` and with every Intl modifier and `timeStyle`
  used.
* **Counts**, which a change must not grow: the allocations of resolving and
  describing catalogue messages, the `Display` calls of a value many
  placeholders read, and the crates in the parser's dependency tree, without
  features (the parser alone) and with `intl`. No public type computes over
  what a caller passes, so nothing costs a consumer's compiler more as their
  code grows; the dependency tree is what the crate adds to their build.
* **Times**, per batch, of the workloads every implementation of the family
  is measured on: catalogue messages, thousands of placeholders and options,
  long blanks and keys, deep nesting.

## License

[MIT](LICENSE)

[`Intl`]: https://docs.rs/curly-message-parser/latest/curly_message_parser/trait.Intl.html
[`Value`]: https://docs.rs/curly-message-parser/latest/curly_message_parser/enum.Value.html
[`Object`]: https://docs.rs/curly-message-parser/latest/curly_message_parser/struct.Object.html
[`Props`]: https://docs.rs/curly-message-parser/latest/curly_message_parser/struct.Props.html
