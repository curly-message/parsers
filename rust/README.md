# curly-message-parser

The Rust implementation of the
[Curly Message Format](https://github.com/curly-message/spec).

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
scan reads a message in time linear in its length.

## License

[MIT](LICENSE)

[`Object`]: https://docs.rs/curly-message-parser/latest/curly_message_parser/struct.Object.html
