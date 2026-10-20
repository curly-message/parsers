#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

// Messages written at random over the characters the syntax reads, resolved
// against payloads and modifiers that answer, fail, panic and resolve again.
// A panic inside the crate is a defect even where the backstop of `resolve`
// swallows it, so the hook records every panic raised from the crate's own
// source, and every one this file raises but did not mean to.

use std::fmt;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex, OnceLock};

use curly_message_parser::{
    Array, Context, MAX_OUTPUT_LENGTH, ModifierError, ModifierInput, NodeKind, Object, Parser,
    Value, cst,
};

const MESSAGES: usize = 20_000;

const EXPECTED: [&str; 2] = ["a host value that panics", "a modifier that panics"];

// The characters, and words written in them that reach the placeholders a
// payload holds and the modifiers the parser registers.
const ALPHABET: [&str; 21] = [
    "{",
    "}",
    ";",
    ":",
    "\\",
    " ",
    "x",
    "\u{e9}",
    "\u{2028}",
    "{{",
    "}}",
    "{{x",
    "{{xx",
    ":x",
    ":xx",
    ":xxx",
    ":\u{e9}",
    ":default",
    "; x:",
    "; default:",
    "default",
];

struct Xorshift(u64);

impl Xorshift {
    fn next(&mut self) -> u64 {
        self.0 ^= self.0 << 13;
        self.0 ^= self.0 >> 7;
        self.0 ^= self.0 << 17;
        self.0
    }

    fn below(&mut self, bound: usize) -> usize {
        (self.next() % bound as u64) as usize
    }
}

struct Panics;

impl fmt::Display for Panics {
    fn fmt(&self, _: &mut fmt::Formatter<'_>) -> fmt::Result {
        panic!("a host value that panics");
    }
}

fn payloads() -> Vec<Object> {
    let nested = (0..50).fold(Value::from("x"), |inner, _| {
        Value::from(Array::from(vec![inner, Value::Null]))
    });
    let wrapper = Object::new()
        .with("value", "x")
        .with("default", "\u{e9}")
        .with("props", Object::new().with("x", Object::new().with("x", 1)));

    vec![
        Object::new(),
        Object::new().with("x", "x").with("default", "d"),
        Object::new()
            .with("x", 1.5)
            .with("\u{e9}", -0.0)
            .with("xx", u64::MAX),
        Object::new().with("x", wrapper.clone()).with("xx", wrapper),
        Object::new().with("x", nested).with(
            "xx",
            Object::new().with("x", vec![Value::Undefined, Value::Null]),
        ),
        Object::new()
            .with("x", Value::host(Panics))
            .with("default", Value::Undefined)
            .with("xx", "{{x}}"),
    ]
}

static PARSER: OnceLock<Parser> = OnceLock::new();

// How often each modifier was called, and how many reports were made: the
// run has to reach every one of them to show anything.
static CALLS: [AtomicUsize; 6] = [const { AtomicUsize::new(0) }; 6];

fn called(at: usize) {
    CALLS[at].fetch_add(1, Ordering::Relaxed);
}

fn parser() -> &'static Parser {
    PARSER.get_or_init(|| {
        Parser::builder()
            .on_report(|report| {
                called(5);
                assert!(
                    !report.text.contains(['\n', '\r', '\u{2028}', '\u{2029}']),
                    "{report:?}"
                );
            })
            .modifier("x", |input: &ModifierInput<'_>| {
                called(0);
                let first = input.option_value(0).unwrap_or_default();

                Ok(Some(Value::from(format!(
                    "{}{first}{}",
                    input.value(),
                    input.default()
                ))))
            })
            .modifier("xx", |_: &ModifierInput<'_>| {
                called(1);
                Err(ModifierError::new("fails"))
            })
            .modifier(
                "xxx",
                |input: &ModifierInput<'_>| -> Result<Option<Value>, ModifierError> {
                    called(2);
                    let _ = input.option_value(1);
                    panic!("a modifier that panics");
                },
            )
            .modifier("\u{e9}", |input: &ModifierInput<'_>| {
                called(3);
                let message = input.option_value(0).unwrap_or(input.value()).to_owned();
                let payload = Object::new().with("x", message.as_str());

                Ok(Some(Value::from(parser().resolve(
                    message.as_str(),
                    &Context::new().payload(&payload),
                ))))
            })
            .modifier("default", |input: &ModifierInput<'_>| {
                called(4);
                Ok(Some(Value::from(input.props().to_object())))
            })
            .build()
    })
}

fn assert_tiles(message: &str) {
    let tree = cst(message);
    let mut at = 0;
    let mut stack = vec![tree.root()];

    assert_eq!(tree.root().span(), 0..message.len(), "{message:?}");

    while let Some(node) = stack.pop() {
        let span = node.span();

        assert!(
            message.is_char_boundary(span.start) && message.is_char_boundary(span.end),
            "{message:?}"
        );

        if node.children().len() == 0 && node.kind() != NodeKind::Message {
            assert_eq!(span.start, at, "{message:?}");
            at = span.end;
        }

        stack.extend(node.children().rev());
    }

    assert_eq!(at, message.len(), "{message:?}");
}

#[test]
fn no_message_panics_inside_the_crate() {
    let defects: Arc<Mutex<Vec<String>>> = Arc::default();
    let sink = Arc::clone(&defects);

    std::panic::set_hook(Box::new(move |info| {
        let inside = info.location().is_some_and(|location| {
            let file = location.file().replace('\\', "/");

            file.starts_with("src/") || file.contains("/rust/src/")
        });
        let payload = info.payload();
        let message = payload
            .downcast_ref::<&str>()
            .copied()
            .or_else(|| payload.downcast_ref::<String>().map(String::as_str))
            .unwrap_or_default();
        let expected = EXPECTED.contains(&message);

        if inside || !expected {
            sink.lock().unwrap().push(info.to_string());
        }
    }));

    let outcome = std::thread::Builder::new()
        .stack_size(8 * 1024 * 1024)
        .spawn(|| {
            let payloads = payloads();
            let mut random = Xorshift(0x9e37_79b9_7f4a_7c15);

            for _ in 0..MESSAGES {
                let length = random.below(40);
                let message: String = (0..length)
                    .map(|_| ALPHABET[random.below(ALPHABET.len())])
                    .collect();
                let payload = &payloads[random.below(payloads.len())];
                let props = Object::new().with("x", Object::new().with("y", 2));
                let context = Context::new()
                    .payload(payload)
                    .props(&props)
                    .locale(["", "x", "\u{e9}"][random.below(3)])
                    .id("fuzz");
                let first = parser().resolve(message.as_str(), &context);

                assert!(first.len() <= MAX_OUTPUT_LENGTH, "{message:?}");
                assert_eq!(
                    parser().resolve(message.as_str(), &context),
                    first,
                    "{message:?}"
                );
                assert_tiles(&message);
            }
        })
        .unwrap()
        .join();

    let _ = std::panic::take_hook();

    assert_eq!(*defects.lock().unwrap(), Vec::<String>::new());
    assert!(outcome.is_ok());

    for (at, calls) in CALLS.iter().enumerate() {
        assert!(calls.load(Ordering::Relaxed) > 0, "{at}");
    }
}
