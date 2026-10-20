#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

use std::fmt;
use std::sync::Arc;

use curly_message_parser::{Array, Object, Value};

#[test]
fn an_object_keeps_its_entries_in_insertion_order() {
    let mut object = Object::new().with("b", 1).with("a", 2).with("c", 3);

    assert_eq!(
        object.insert("a", 20).map(|old| format!("{old:?}")),
        Some("Integer(2)".to_owned())
    );
    assert_eq!(
        object.iter().map(|(key, _)| key).collect::<Vec<_>>(),
        ["b", "a", "c"]
    );
    assert_eq!(format!("{:?}", object.get("a")), "Some(Integer(20))");
    assert_eq!(object.len(), 3);
}

#[test]
fn prototype_names_are_keys_like_any_other() {
    let object = Object::new()
        .with("__proto__", "p")
        .with("constructor", "c");

    assert_eq!(object.get("__proto__").and_then(Value::as_str), Some("p"));
    assert_eq!(object.get("constructor").and_then(Value::as_str), Some("c"));
    assert!(object.get("toString").is_none());
    assert!(Object::new().get("__proto__").is_none());
}

#[test]
fn an_object_past_its_index_threshold_answers_alike() {
    for size in [0, 1, 8, 9, 10, 100] {
        let mut object: Object = (0..size).map(|n| (format!("k{n}"), n)).collect();

        for n in 0..size {
            assert_eq!(
                format!("{:?}", object.get(&format!("k{n}"))),
                format!("Some(Unsigned({n}))")
            );
        }

        assert!(object.get("missing").is_none());
        assert_eq!(object.insert("k0", "again").is_some(), size > 0);
        assert_eq!(object.len(), size.max(1));
        assert_eq!(
            object.iter().next().map(|(key, _)| key.to_owned()),
            Some("k0".to_owned())
        );
    }

    let mut grown = Object::new();

    for n in 0..20 {
        grown.insert(format!("k{n}"), n);
    }

    assert_eq!(
        grown
            .iter()
            .map(|(key, _)| key.to_owned())
            .collect::<Vec<_>>(),
        (0..20).map(|n| format!("k{n}")).collect::<Vec<_>>()
    );
    assert!(grown.contains_key("k19"));
    assert!(!grown.contains_key("k20"));
}

#[test]
fn duplicate_keys_collected_keep_the_first_place_and_the_last_value() {
    let object: Object = [("a", 1), ("b", 2), ("a", 3)].into_iter().collect();

    assert_eq!(
        format!("{object:?}"),
        r#"{"a": Integer(3), "b": Integer(2)}"#
    );
}

#[test]
fn none_is_undefined_and_some_is_its_value() {
    assert!(Value::from(None::<i32>).is_undefined());
    assert_eq!(format!("{:?}", Value::from(Some("x"))), r#"String("x")"#);
    assert!(Value::default().is_undefined());
}

#[test]
fn primitives_convert_to_their_variant() {
    let cases: [(Value, &str); 12] = [
        (true.into(), "Bool(true)"),
        (1.5f64.into(), "Number(1.5)"),
        (0.5f32.into(), "Number(0.5)"),
        ((-3i8).into(), "Integer(-3)"),
        (i64::MIN.into(), "Integer(-9223372036854775808)"),
        ((-1isize).into(), "Integer(-1)"),
        (7u8.into(), "Unsigned(7)"),
        (u64::MAX.into(), "Unsigned(18446744073709551615)"),
        (2usize.into(), "Unsigned(2)"),
        ("s".into(), r#"String("s")"#),
        (String::from("t").into(), r#"String("t")"#),
        (Value::date(0.0), "Date(0.0)"),
    ];

    for (value, debug) in cases {
        assert_eq!(format!("{value:?}"), debug);
    }

    assert_eq!(
        format!("{:?}", Value::from(&String::from("u"))),
        r#"String("u")"#
    );
    assert_eq!(format!("{:?}", Value::Null), "Null");
    assert_eq!(format!("{:?}", Value::host(42u128)), "Host(..)");
}

#[test]
fn containers_convert_and_answer_what_they_hold() {
    let array = Value::from(vec![1, 2]);

    assert_eq!(array.as_array().map(|items| items.len()), Some(2));
    assert_eq!(format!("{array:?}"), "[Integer(1), Integer(2)]");

    let mut built = Array::new();

    built.push("x");
    built.push(Object::new().with("k", Value::Null));

    let value = Value::from(built);

    assert_eq!(format!("{value:?}"), r#"[String("x"), {"k": Null}]"#);
    assert!(value.as_object().is_none());
    assert!(
        Value::from(Object::new())
            .as_object()
            .is_some_and(Object::is_empty)
    );
    assert!(Value::from("x").as_array().is_none());
    assert_eq!(
        Value::from(Arc::new(Array::from(vec![Value::Null])))
            .as_array()
            .map(|items| items.len()),
        Some(1)
    );
    assert!(Value::from(Arc::new(Object::new())).as_object().is_some());
}

#[test]
fn a_value_is_three_words_and_crosses_threads() {
    fn shared<T: Send + Sync + Clone>() {}

    shared::<Value>();
    shared::<Object>();
    shared::<Array>();

    #[cfg(target_pointer_width = "64")]
    assert_eq!(std::mem::size_of::<Value>(), 24);
}

// Builds a value nested `depth` levels deep, alternating arrays and objects.
fn nested(depth: usize) -> Value {
    let mut value = Value::from("leaf");

    for level in 0..depth {
        value = if level % 2 == 0 {
            Value::from(vec![value])
        } else {
            Value::from(Object::new().with("v", value))
        };
    }

    value
}

#[test]
fn a_deep_value_drops_and_prints_without_a_call_per_level() {
    std::thread::Builder::new()
        .stack_size(64 * 1024)
        .spawn(|| {
            let value = nested(1_000_000);
            let shared = value.clone();

            drop(value);

            let printed = format!("{shared:?}");

            assert!(printed.starts_with(r#"{"v": [{"v": ["#));
            assert!(printed.contains(r#"String("leaf")"#));
            drop(shared);
        })
        .unwrap()
        .join()
        .unwrap();
}

#[test]
fn a_shared_child_survives_its_first_owner() {
    let child = Arc::new(Object::new().with("deep", nested(10)));
    let first = Value::from(Array::from(vec![Value::Object(Arc::clone(&child))]));
    let second = Value::from(Array::from(vec![Value::Object(Arc::clone(&child))]));

    drop(first);

    assert_eq!(Arc::strong_count(&child), 2);
    assert!(format!("{second:?}").contains("deep"));
}

struct Point(i32, i32);

impl fmt::Display for Point {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "({}, {})", self.0, self.1)
    }
}

#[test]
fn a_host_value_keeps_its_own_type() {
    let value = Value::host(Point(1, 2));

    match value {
        Value::Host(point) => assert_eq!(point.to_string(), "(1, 2)"),
        other => panic!("not a host value: {other:?}"),
    }
}
