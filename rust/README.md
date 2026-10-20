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

The parser claims the Core level of the format: the syntax, the resolution
and the comparisons `eq`, `ne`, `lt`, `lte`, `gt` and `gte`. A placeholder
naming any other modifier resolves to its fallback chain and reports
`unknown-modifier`; it is never run as a comparison instead.

## Resolving a message

`Parser::resolve` takes a message and a `Context` holding what the
resolution reads besides it: the payload and the id the caller knows the
message by. Both are optional. A message is text, or a [`Value`] that
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

Containment catches a panic as it unwinds, so it needs the default
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
value nested any depth convert. What does reach the thread's stack is a
resolution started from inside another one, by a report handler or by a
value's `Display`: each holds a walk activation, and one that would take the
thread past `MAX_HOST_DEPTH` does not start. It answers the empty string, and
the call it was started from is treated as one that did not come back.

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

## License

[MIT](LICENSE)

[`Value`]: https://docs.rs/curly-message-parser/latest/curly_message_parser/enum.Value.html
[`Object`]: https://docs.rs/curly-message-parser/latest/curly_message_parser/struct.Object.html
