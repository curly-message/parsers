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

Resolution falls back rather than failing. A placeholder the payload holds
no value for takes its fallback chain: the wrapper's `default`, then the
payload's own `default` entry, then the `default` the placeholder writes
inline, then the empty string. The one resolution that fails is one asked
for while `MAX_REENTRY` are in progress on its thread: it is refused, and
`resolve` unwinds with [`ReentryLimit`] into the code that asked, unless the
thread already unwinds (see [Limits](#limits)). A `Parser` is cheap to clone
and may be shared across threads.

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
it happens, and so does the refusal of a resolution past `MAX_REENTRY`
(below). The process's panic hook still runs for a contained panic, and
prints what it prints for any other; that refusal runs none.

## Limits

| Constant | Value | Bounds |
| --- | --- | --- |
| `MAX_OUTPUT_LENGTH` | 100 000 | bytes of UTF-8 of placeholder results the output carries; the message's own text is not counted and always renders |
| `MAX_READ_LENGTH` | 100 000 | bytes of UTF-8 of value text a resolution reads |
| `MAX_CONVERSION_NODES` | 100 000 | nodes one conversion of a value visits |
| `MAX_SERIALIZED_LENGTH` | 536 870 888 | code units of UTF-16 one serialization of a value writes |
| `MAX_NESTING` | 8 | levels a placeholder may nest and still resolve |
| `MAX_REENTRY` | 8 | resolutions in progress at once on one thread |

A placeholder nested past `MAX_NESTING` takes its fallback chain and reports
`nesting-limit`. The walk keeps what it nests on a stack of its own, so a
message nested any depth resolves without a call per level, and so does a
value nested any depth convert. What does reach the thread's stack is a
resolution a report handler or a value's `Display` starts from inside
another.

A thread that runs out of stack aborts the process, which nothing can
contain, so `MAX_REENTRY` is the bound that stands in for the host's own
limit on a resolution reaching itself (section 13). It counts the
resolutions in progress on one thread, across every parser, and nothing of
the walk of one, so a resolution is never cut short within its nesting by
the ones around it. A resolution asked for while `MAX_REENTRY` are in
progress on its thread is refused before anything is read: `resolve` unwinds
with [`ReentryLimit`] into the host code that asked, without running the
panic hook. That code may catch it, as it may any panic. One it lets pass is
contained as a panic of host code is: a value's `Display` describes nothing,
and a handler's report goes undelivered. One asked for while the thread
already unwinds, by a `Drop` that resolves, is refused without unwinding,
since an unwind leaving a `Drop` there would abort the process: `resolve`
answers the empty string to its caller, and the host code the parser called
that asked for it counts as failed all the same, as one that lets the
refusal pass does. A resolution asked for where none is in progress on its
thread is never refused, the one a host hands to a thread of its own
included, so a chain that goes on from thread to thread is the host's to
bound.

What the crate keeps for this bound is the operating-system thread's, and
that is where it falls short of section 13 for a host that runs stackful
fibers or coroutines: section 13 counts the resolutions on each thread of
execution, whatever runs on a call stack of its own, while fibers that
share an OS thread share its count, the crate's mark of a refusal made
while the thread unwinds, and whether the thread unwinds at all, which
`std::thread::panicking` answers for the OS thread. A resolution asked for
on a fiber where none is in progress is refused there while `MAX_REENTRY`
are in progress on the other fibers of its OS thread, suspended part way.
While a fiber is suspended in a `Drop` that runs as it unwinds, a refusal
on any fiber of its OS thread answers the empty string rather than
unwinding, and a refusal made there can fail a host call in progress on
another fiber in place of the one that asked, which keeps its answer. A
fiber suspended in the middle of a resolution holds a value on its stack
that is not `Send`, so whatever sends it to another OS thread, a host's
`unsafe impl Send` or a coroutine library's own, claims a `Send` the stack
does not have, and the fiber takes nothing of the count with it: the
resolution stays counted on the thread it started on, while none of it runs
there, and uncounted on any other it runs on, and as it finishes it lowers by
one the count of the thread it finishes on, where that count is above zero,
whatever that thread holds. A thread's count can so stand above or below the
number of resolutions in progress on it.

Tests start resolutions inside one another along each of these ways until
one is refused, on a thread of 512 KiB, in a debug build.

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
[`ReentryLimit`]: https://docs.rs/curly-message-parser/latest/curly_message_parser/struct.ReentryLimit.html
