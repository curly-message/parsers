#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

use curly_message_parser::{NodeKind, NodeRef, cst};

// One line per node, in the order the message writes them: its depth, kind,
// span and whatever field it carries.
fn dump(message: &str) -> Vec<String> {
    let tree = cst(message);
    let mut lines = Vec::new();
    let mut stack = vec![(0, tree.root())];

    while let Some((depth, node)) = stack.pop() {
        lines.push(line(depth, node));
        stack.extend(node.children().rev().map(|child| (depth + 1, child)));
    }

    lines
}

fn line(depth: usize, node: NodeRef<'_>) -> String {
    let span = node.span();
    let mut line = format!(
        "{depth} {} {}..{}",
        node.kind().as_str(),
        span.start,
        span.end
    );

    if let Some(name) = node.name() {
        line.push_str(&format!(" {name:?}"));
    }

    if let Some(cancels) = node.cancels() {
        line.push_str(&format!(" cancels={cancels}"));
    }

    line
}

// The leaves in order, each checked to start where the one before it ended.
fn assert_tiles(message: &str) {
    let tree = cst(message);
    let mut at = 0;
    let mut stack = vec![tree.root()];

    assert_eq!(tree.root().span(), 0..message.len());

    while let Some(node) = stack.pop() {
        let span = node.span();

        assert!(span.start <= span.end, "{message:?}: {node:?}");
        assert!(
            message.is_char_boundary(span.start) && message.is_char_boundary(span.end),
            "{message:?}: {node:?}"
        );

        if node.children().len() == 0 && node.kind() != NodeKind::Message {
            assert_eq!(span.start, at, "{message:?}: {node:?}");
            at = span.end;
        }

        stack.extend(node.children().rev());
    }

    assert_eq!(at, message.len(), "{message:?}");
}

#[test]
fn the_interchange_example() {
    assert_eq!(
        dump("\\{world"),
        [
            "0 message 0..7",
            "1 escape 0..2 cancels=true",
            "1 text 2..7"
        ]
    );
}

#[test]
fn an_empty_message_is_a_root_alone() {
    assert_eq!(dump(""), ["0 message 0..0"]);
}

#[test]
fn a_placeholder_inside_another() {
    let lines = dump("{{count:gt; 0:{{count:number;}}; default:no;}}");

    assert_eq!(lines[1], "1 placeholder 0..46");
    assert!(lines.contains(&"2 key 2..7 \"count\"".to_owned()));
    assert!(lines.contains(&"2 modifier 8..10 \"gt\"".to_owned()));
    assert!(lines.contains(&"2 option-key 12..13 \"0\"".to_owned()));
    assert!(lines.contains(&"2 option-value 14..31".to_owned()));
    assert!(lines.contains(&"3 placeholder 14..31".to_owned()));
    assert!(lines.contains(&"2 separator 31..32".to_owned()));
    assert!(lines.contains(&"2 separator 43..44".to_owned()));
    assert!(lines.contains(&"2 option-key 44..44 \"\"".to_owned()));
    assert_eq!(
        lines
            .iter()
            .filter(|line| line.contains(" placeholder "))
            .count(),
        2
    );
}

#[test]
fn a_brace_a_backslash_consumed() {
    assert_eq!(
        dump("\\{{v}}"),
        [
            "0 message 0..6",
            "1 escape 0..2 cancels=true",
            "1 text 2..6"
        ]
    );
    assert_eq!(
        dump("{{v\\}}"),
        [
            "0 message 0..6",
            "1 text 0..3",
            "1 escape 3..5 cancels=true",
            "1 text 5..6"
        ]
    );

    let lines = dump("{{v\\}}}");

    assert_eq!(lines[1], "1 placeholder 0..7");
    assert!(lines.contains(&"2 key 2..5 \"v}\"".to_owned()));
}

#[test]
fn a_colon_that_is_content() {
    let lines = dump("{{a:b:c; d:e:f;}}");

    assert!(lines.contains(&"2 key 2..3 \"a\"".to_owned()));
    assert!(lines.contains(&"2 modifier 4..7 \"b:c\"".to_owned()));
    assert!(lines.contains(&"2 option-key 9..10 \"d\"".to_owned()));
    assert!(lines.contains(&"2 option-value 11..14".to_owned()));
    assert!(lines.contains(&"3 text 11..14".to_owned()));
}

#[test]
fn padding_is_described_beside_the_name() {
    let tree = cst("{{ v : number ;}}");
    let placeholder = tree.root().children().next().unwrap();
    let parts: Vec<String> = placeholder
        .children()
        .map(|node| {
            format!(
                "{} {:?} {:?}",
                node.kind().as_str(),
                node.span(),
                node.name()
            )
        })
        .collect();

    assert_eq!(
        parts,
        [
            "open 0..2 None",
            "space 2..3 None",
            "key 3..4 Some(\"v\")",
            "space 4..5 None",
            "separator 5..6 None",
            "space 6..7 None",
            "modifier 7..13 Some(\"number\")",
            "space 13..14 None",
            "separator 14..15 None",
            "option-key 15..15 Some(\"\")",
            "close 15..17 None",
        ]
    );
}

#[test]
fn a_name_that_is_all_padding() {
    assert_eq!(
        dump("{{  }}"),
        [
            "0 message 0..6",
            "1 placeholder 0..6",
            "2 open 0..2",
            "2 key 2..2 \"\"",
            "2 space 2..4",
            "2 close 4..6"
        ]
    );
}

#[test]
fn a_line_terminator_leaves_text() {
    for terminator in ["\n", "\r", "\u{2028}", "\u{2029}"] {
        let message = format!("{{{{v{terminator}}}}}");

        assert_eq!(
            dump(&message),
            [
                "0 message 0..".to_owned() + &message.len().to_string(),
                "1 text 0..".to_owned() + &message.len().to_string()
            ]
        );

        let escaped = format!("{{{{v\\{terminator}}}}}");

        assert_eq!(dump(&escaped).len(), 4, "{escaped:?}");
    }

    assert_eq!(dump("{{v\u{2027}}}")[1], "1 placeholder 0..8");
}

#[test]
fn an_escape_before_an_astral_character_spans_the_whole_code_point() {
    assert_eq!(
        dump("\\\u{1f600}x"),
        [
            "0 message 0..6",
            "1 escape 0..5 cancels=false",
            "1 text 5..6"
        ]
    );
    assert_eq!(dump("\\\u{3000}")[1], "1 escape 0..4 cancels=true");
    assert_eq!(dump("a\\")[1], "1 text 0..2");
    assert_eq!(dump("\\d")[1], "1 escape 0..2 cancels=false");
}

#[test]
fn an_option_value_that_is_more_than_a_placeholder() {
    let lines = dump("{{n:gt; 0:you have {{n}} left; default:none;}}");

    assert!(lines.contains(&"2 option-value 10..29".to_owned()));
    assert!(lines.contains(&"3 text 10..19".to_owned()));
    assert!(lines.contains(&"3 placeholder 19..24".to_owned()));
    assert!(lines.contains(&"3 text 24..29".to_owned()));
}

#[test]
fn a_double_brace_in_a_value_that_opens_nothing() {
    let lines = dump("{{a; x:{{b}}");

    assert_eq!(
        lines[..3],
        ["0 message 0..12", "1 text 0..7", "1 placeholder 7..12"]
    );
}

#[test]
fn a_name_keeps_escaped_padding_and_spells_its_escapes() {
    let lines = dump("{{my\\;key}}");

    assert!(lines.contains(&"2 key 2..9 \"my;key\"".to_owned()));
    assert!(lines.contains(&"3 text 2..4".to_owned()));
    assert!(lines.contains(&"3 escape 4..6 cancels=true".to_owned()));
    assert!(lines.contains(&"3 text 6..9".to_owned()));

    let padded = dump("{{ v\\  }}");

    assert!(padded.contains(&"2 key 3..6 \"v \"".to_owned()));
    assert!(padded.contains(&"2 space 6..7".to_owned()));

    let doubled = dump("{{ v\\\\ }}");

    assert!(doubled.contains(&"2 key 3..6 \"v\\\\\"".to_owned()));
}

#[test]
fn a_tree_tiles_its_message() {
    let messages = [
        "",
        "plain",
        "{{a}}{{b}}",
        "{{a; b:{{c; d:{{e}}}}; f}}",
        "{{a:eq; x:\\;; y:\\{{z}}; default:{{d}};}}",
        "{{ \u{a0}k\u{a0} : m ; o : \u{3000}v\u{3000} ;}}",
        "{{{{a}}}}",
        "}}{{",
        "{{a; b:{{c}}x{{d}} ; }}",
        "\\",
        "{{a\\",
        "\u{e9}{{\u{e9}; \u{e9}:\u{e9}}}\u{e9}",
    ];

    for message in messages {
        assert_tiles(message);
    }
}

#[test]
fn a_message_nested_a_hundred_thousand_levels_is_described_without_a_call_per_level() {
    std::thread::Builder::new()
        .stack_size(64 * 1024)
        .spawn(|| {
            let depth = 100_000;
            let message = format!("{}x{}", "{{a; b:".repeat(depth), "}}".repeat(depth));
            let tree = cst(&message);
            let mut placeholders = 0;
            let mut stack = vec![tree.root()];

            while let Some(node) = stack.pop() {
                if node.kind() == NodeKind::Placeholder {
                    placeholders += 1;
                }

                stack.extend(node.children());
            }

            assert_eq!(placeholders, depth);
            assert_tiles(&message);
            drop(tree);
        })
        .unwrap()
        .join()
        .unwrap();
}
