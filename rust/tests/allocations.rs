#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]
// A counting allocator is the one way to see allocations from outside the
// crate, and installing one takes an unsafe impl.
#![allow(unsafe_code)]

use std::alloc::{GlobalAlloc, Layout, System};
use std::cell::Cell;

use curly_message_parser::{
    Category, Context, Intl, MAX_SERIALIZED_LENGTH, ModifierError, ModifierInput, Object, Parser,
    Refusal, Request, Value,
};

struct Counting;

thread_local! {
    static ALLOCATIONS: Cell<usize> = const { Cell::new(0) };
    static BYTES: Cell<usize> = const { Cell::new(0) };
    static LARGE: Cell<(usize, usize)> = const { Cell::new((usize::MAX, 0)) };
    static LARGEST: Cell<usize> = const { Cell::new(0) };
    static LIVE: Cell<isize> = const { Cell::new(0) };
}

// The bytes the allocator holds for this thread, a size at most `isize::MAX`
// as a `Layout` keeps it.
fn live(change: isize) {
    let _ = LIVE.try_with(|live| live.set(live.get() + change));
}

fn count(bytes: usize) {
    let _ = ALLOCATIONS.try_with(|count| count.set(count.get() + 1));
    let _ = BYTES.try_with(|count| count.set(count.get() + bytes));
    let _ = LARGEST.try_with(|largest| largest.set(largest.get().max(bytes)));
}

// SAFETY: every call is forwarded to the system allocator unchanged; the count
// is a thread-local `Cell` of a const initializer, which allocates nothing.
unsafe impl GlobalAlloc for Counting {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        count(layout.size());
        live(layout.size().cast_signed());
        let _ = LARGE.try_with(|large| {
            let (from, count) = large.get();

            if layout.size() >= from {
                large.set((from, count + 1));
            }
        });
        // SAFETY: the caller's contract is passed on as it is.
        unsafe { System.alloc(layout) }
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        live(-layout.size().cast_signed());
        // SAFETY: the caller's contract is passed on as it is.
        unsafe { System.dealloc(ptr, layout) }
    }

    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        count(new_size);
        live(new_size.cast_signed() - layout.size().cast_signed());
        // SAFETY: the caller's contract is passed on as it is.
        unsafe { System.realloc(ptr, layout, new_size) }
    }
}

#[global_allocator]
static GLOBAL: Counting = Counting;

fn allocations<T>(run: impl FnOnce() -> T) -> (usize, T) {
    let before = ALLOCATIONS.with(Cell::get);
    let output = run();

    (ALLOCATIONS.with(Cell::get) - before, output)
}

fn bytes<T>(run: impl FnOnce() -> T) -> (usize, T) {
    let before = BYTES.with(Cell::get);
    let output = run();

    (BYTES.with(Cell::get) - before, output)
}

// The fresh allocations of at least `from` bytes a run makes: what a buffer
// that already holds text grows by is a reallocation, so each one counted is
// a second buffer.
fn large<T>(from: usize, run: impl FnOnce() -> T) -> (usize, T) {
    LARGE.with(|large| large.set((from, 0)));
    let output = run();

    (LARGE.with(|large| large.replace((usize::MAX, 0)).1), output)
}

// The most bytes one allocation of a run asks for, a buffer's growth included.
fn largest<T>(run: impl FnOnce() -> T) -> (usize, T) {
    LARGEST.with(|largest| largest.set(0));
    let output = run();

    (LARGEST.with(Cell::get), output)
}

// The bytes resolving a message of `count` placeholders allocates, with props
// holding, under `name`, a string of `size` bytes no placeholder reads.
fn props_bytes(
    parser: &Parser,
    placeholder: &str,
    count: usize,
    name: &str,
    size: usize,
) -> (usize, String) {
    let message = placeholder.repeat(count);
    let payload = Object::new().with("a", 1);
    let props = Object::new().with(name, Object::new().with("unread", "x".repeat(size)));
    let context = Context::new().payload(&payload).props(&props).locale("en");

    bytes(|| parser.resolve(message.as_str(), &context))
}

struct Fixed;

impl Intl for Fixed {
    fn format(&self, _: &str, _: &Request) -> Result<String, Refusal> {
        Ok("n".to_owned())
    }

    fn select(&self, _: &str, _: &Request) -> Result<Category, Refusal> {
        Ok(Category::Other)
    }
}

#[test]
fn an_object_of_n_keys_costs_n_plus_two_allocations() {
    let keys: Vec<String> = (0..1000).map(|n| format!("key{n}")).collect();

    for size in [1, 8, 9, 100, 1000] {
        let entries: Vec<(&str, i32)> = keys
            .iter()
            .take(size)
            .map(|key| (key.as_str(), 0))
            .collect();
        let (count, object) = allocations(|| entries.iter().copied().collect::<Object>());
        let expected = if size > 8 { size + 2 } else { size + 1 };

        assert_eq!(count, expected, "{size} keys");
        assert_eq!(object.len(), size);
    }
}

#[test]
fn a_modifier_reading_props_costs_nothing_per_byte_of_a_string_they_hold() {
    let parser = Parser::builder()
        .modifier(
            "peek",
            |input: &ModifierInput<'_>| -> Result<Option<Value>, ModifierError> {
                Ok(Some(Value::from(input.props().len())))
            },
        )
        .build();
    let (small, output) = props_bytes(&parser, "{{a:peek}}", 100, "peek", 10);
    let (large, _) = props_bytes(&parser, "{{a:peek}}", 100, "peek", 100_000);

    assert_eq!(output, "1".repeat(100));
    assert_eq!(large, small);
}

// A formatting modifier hands the host its props composed into one request,
// which a resolution builds once and reuses for each placeholder it serves:
// what a string the props hold costs is paid once, never per placeholder.
#[test]
fn a_formatting_modifier_costs_nothing_per_placeholder_for_a_string_its_props_hold() {
    let parser = Parser::builder().intl(Fixed).build();
    let per_placeholder = |size| {
        let (fewer, output) = props_bytes(&parser, "{{a:number}}", 100, "number", size);
        let (more, _) = props_bytes(&parser, "{{a:number}}", 200, "number", size);

        assert_eq!(output, "n".repeat(100));

        more - fewer
    };

    assert_eq!(per_placeholder(100_000), per_placeholder(10));
}

// What a hundred more of a message's units allocate, where the payload and
// the props `inputs` makes hold a string of 10 bytes, then of 100 000.
fn growth(parser: &Parser, unit: &str, inputs: impl Fn(usize) -> (Object, Object)) -> [usize; 2] {
    let fewer = unit.repeat(100);
    let more = unit.repeat(200);

    [10, 100_000].map(|size| {
        let (payload, props) = inputs(size);
        let context = Context::new().payload(&payload).props(&props).locale("en");
        // What a resolution sets up once a process, the first time it is
        // read, is not what a placeholder costs.
        parser.resolve(unit, &context);
        let (before, _) = bytes(|| parser.resolve(fewer.as_str(), &context));
        let (after, _) = bytes(|| parser.resolve(more.as_str(), &context));

        after - before
    })
}

fn unread(size: usize) -> Object {
    Object::new().with("unread", "x".repeat(size))
}

// Placeholders that take turns, one formatting modifier after another, are
// each configured from the layers as they lie, never from a copy of them.
#[test]
fn formatting_modifiers_taking_turns_cost_nothing_per_byte_of_a_string_their_props_hold() {
    let parser = Parser::builder().intl(Fixed).build();
    let payload = || Object::new().with("a", 1);

    for (unit, other) in [
        ("{{a:number}}{{a:date}}", "date"),
        ("{{a:number}}{{a:plural; other:x}}", "plural"),
    ] {
        let [small, large] = growth(&parser, unit, |size| {
            let props = Object::new()
                .with("number", unread(size))
                .with(other, unread(size));

            (payload(), props)
        });

        assert_eq!(large, small, "{unit}");
    }
}

// A wrapper's props are a layer of their own over the caller's: placeholders
// of different wrappers, or of one wrapper holding the string, cost the same
// whatever the string's size.
#[test]
fn wrapped_values_cost_nothing_per_byte_of_a_string_the_props_hold() {
    let parser = Parser::builder().intl(Fixed).build();
    let wrapper = |value, props: Object| {
        Object::new()
            .with("value", value)
            .with("props", Object::new().with("number", props))
    };
    let [small, large] = growth(&parser, "{{a:number}}{{b:number}}", |size| {
        let payload = Object::new()
            .with("a", wrapper(1, Object::new().with("x", 1)))
            .with("b", wrapper(2, Object::new().with("y", 2)));

        (payload, Object::new().with("number", unread(size)))
    });

    assert_eq!(large, small, "two wrappers");

    let [small, large] = growth(&parser, "{{w:number}}{{w:plural; other:x}}", |size| {
        (
            Object::new().with("w", wrapper(1, unread(size))),
            Object::new(),
        )
    });

    assert_eq!(large, small, "a wrapper holding the string");
}

// A host's modifier reads its props where they lie in every layer that holds
// them: the parser's defaults, the caller's props and the wrapper's.
#[test]
fn a_modifier_reading_props_in_several_layers_costs_nothing_per_byte_of_a_string_they_hold() {
    let peek = |input: &ModifierInput<'_>| -> Result<Option<Value>, ModifierError> {
        Ok(Some(Value::from(input.props().len())))
    };
    let defaults = Object::new().with("peek", Object::new().with("y", 1));
    let parser = Parser::builder()
        .modifier("peek", peek)
        .modifier_defaults(defaults)
        .build();
    let [small, large] = growth(&parser, "{{a:peek}}", |size| {
        (
            Object::new().with("a", 1),
            Object::new().with("peek", unread(size)),
        )
    });

    assert_eq!(large, small, "defaults and props");

    let parser = Parser::builder().modifier("peek", peek).build();
    let [small, large] = growth(&parser, "{{a:peek}}", |size| {
        let wrapped = Object::new().with("value", 1).with(
            "props",
            Object::new().with("peek", Object::new().with("z", 1)),
        );

        (
            Object::new().with("a", wrapped),
            Object::new().with("peek", unread(size)),
        )
    });

    assert_eq!(large, small, "props and a wrapper");
}

// The text a value serializes to is kept in the buffer it was written into: a
// copy would hold the text twice at once, and take its room from an allocator
// that cannot refuse it.
#[test]
fn a_serialization_is_kept_where_it_was_written() {
    let size = 90_000;
    let parser = Parser::new();
    let payload = Object::new().with("v", vec![Value::from("x".repeat(size))]);
    let context = Context::new().payload(&payload);
    let (count, output) = large(size, || parser.resolve("<{{v}}>", &context));

    assert_eq!(output.len(), size + 6);
    assert_eq!(count, 0);
}

// A serialization that cannot fit is refused before the text that would pass
// the bound is copied: the copy is the very allocation the bound is there to
// spare.
#[test]
fn a_string_past_the_serialization_bound_is_refused_before_it_is_copied() {
    let parser = Parser::new();
    let payload = Object::new().with(
        "v",
        vec![Value::from("x".repeat(MAX_SERIALIZED_LENGTH + 1))],
    );
    let context = Context::new().payload(&payload);
    let (most, output) = largest(|| parser.resolve("<{{v}}>", &context));

    assert_eq!(output, "<>");
    assert!(most < 1 << 20, "{most} bytes in one allocation");
}

// What escaping writes grows the text up to the bound and no further: a
// string of control characters, each written as six, is refused once its
// escapes reach the bound, with no buffer larger than the bound allows.
#[test]
fn escapes_written_past_the_serialization_bound_are_refused_at_the_bound() {
    let parser = Parser::new();
    let payload = Object::new().with(
        "v",
        vec![Value::from("\u{1}".repeat(MAX_SERIALIZED_LENGTH / 6 + 1))],
    );
    let context = Context::new().payload(&payload);
    let (most, output) = largest(|| parser.resolve("<{{v}}>", &context));

    assert_eq!(output, "<>");
    assert!(
        most <= MAX_SERIALIZED_LENGTH,
        "{most} bytes in one allocation"
    );
}

// What serializing a small container allocates beyond reading a string: its
// text, grown as a `String` grows, from eight bytes and then by doubling, and
// what the resolution keeps of it. A buffer starting smaller reallocates at
// two, four and eight bytes, three more for each container.
#[test]
fn a_small_container_serializes_as_a_string_grows() {
    let parser = Parser::new();
    let cost = |value: Value| {
        let text = Object::new().with("v", "x");
        let payload = Object::new().with("v", value);
        let resolve = |payload: &Object| {
            let context = Context::new().payload(payload);

            parser.resolve("{{v}}", &context);
            allocations(|| parser.resolve("{{v}}", &context))
        };
        let (serialized, output) = resolve(&payload);
        let (read, _) = resolve(&text);

        (serialized - read, output)
    };

    assert_eq!(cost(Value::from(vec![1, 2])), (4, "[1,2]".to_owned()));
    assert_eq!(
        cost(Value::from(Object::new().with("a", 1).with("b", true))),
        (5, r#"{"a":1,"b":true}"#.to_owned())
    );
    assert_eq!(
        cost(Value::from(vec!["ab", "cd", "ef"])),
        (5, r#"["ab","cd","ef"]"#.to_owned())
    );
}

// A description that goes on writing after the bound refused one of its runs,
// each error thrown away: what it writes after is refused as well, with no
// buffer grown again to hold it.
struct Persisting(String);

impl std::fmt::Display for Persisting {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str("x")?;

        for _ in 0..MAX_SERIALIZED_LENGTH / self.0.len() + 4 {
            let _ = formatter.write_str(&self.0);
        }

        Ok(())
    }
}

#[test]
fn a_description_going_on_past_a_refusal_grows_no_buffer() {
    let run = 1 << 20;
    let parser = Parser::new();
    let payload = Object::new().with("v", Value::host(Persisting("x".repeat(run))));
    let context = Context::new().payload(&payload);
    let (count, output) = large(run, || parser.resolve("{{v; default:D}}", &context));

    assert_eq!(output, "D");
    assert_eq!(count, 0);
}

thread_local! {
    static AT_REFUSAL: Cell<Option<isize>> = const { Cell::new(None) };
}

// A description that writes runs until one is refused, and notes the bytes
// still held when it is, against those held as it began.
struct Noting(String);

impl std::fmt::Display for Noting {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let began = LIVE.with(Cell::get);

        loop {
            if formatter.write_str(&self.0).is_err() {
                AT_REFUSAL.with(|at| at.set(Some(LIVE.with(Cell::get) - began)));

                return Ok(());
            }
        }
    }
}

#[test]
fn a_refusal_lets_go_of_the_text_written_before_it() {
    let run = 1 << 16;
    let parser = Parser::new();
    let payload = Object::new().with("v", Value::host(Noting("x".repeat(run))));
    let context = Context::new().payload(&payload);

    AT_REFUSAL.with(|at| at.set(None));
    let output = parser.resolve("{{v; default:D}}", &context);
    let held = AT_REFUSAL.with(Cell::get).unwrap();

    assert_eq!(output, "D");
    assert!(
        held < run.cast_signed(),
        "still held after the refusal: {held}"
    );
}
