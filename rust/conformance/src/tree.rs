use std::collections::HashMap;

use crate::RunError;
use crate::adapter::{Adapter, RNode, Resolution, TreeAnswer};
use crate::cases::contained;
use crate::js;
use crate::plan::{Failure, Outcome};
use crate::set::{ExpectedNode, TreeCase};
use crate::value::Value;

/// The units `CST.md` section 4 lets an implementation count its spans in.
pub(crate) const UNITS: [&str; 3] = ["utf-8", "utf-16", "code-point"];

const KINDS: [&str; 12] = [
    "message",
    "placeholder",
    "open",
    "close",
    "separator",
    "space",
    "key",
    "modifier",
    "option-key",
    "option-value",
    "text",
    "escape",
];

// The kinds that carry a name, and the one that carries `cancels` (CST.md
// section 8). A kind that must carry a field and does not has answered less
// than the document requires; a field a kind does not take is not policed.
pub(crate) const NAMED: [&str; 3] = ["key", "modifier", "option-key"];

fn failure(reason: impl Into<String>, expected: String, actual: String) -> Failure {
    Failure {
        reason: reason.into(),
        expected,
        actual,
    }
}

fn pair(start: i64, end: i64) -> String {
    format!("[{start},{end}]")
}

fn width(character: char, unit: &str) -> usize {
    match unit {
        "utf-8" => character.len_utf8(),
        "utf-16" => character.len_utf16(),
        _ => 1,
    }
}

/// Where each boundary of the message falls, as a byte offset into the
/// message, indexed by the unit the adapter declared. A boundary inside a code
/// point has no offset: a character wider than one unit contributes one entry
/// for its start and nothing for the positions within it, which is what
/// `CST.md` section 4 forbids a span from landing on.
pub(crate) fn boundaries(message: &str, unit: &str) -> Vec<Option<usize>> {
    let mut offsets = Vec::with_capacity(message.len() + 1);

    for (at, character) in message.char_indices() {
        offsets.push(Some(at));

        for _ in 1..width(character, unit) {
            offsets.push(None);
        }
    }

    offsets.push(Some(message.len()));
    offsets
}

/// The text a span covers, where it falls on boundaries.
pub(crate) fn spelled<'m>(
    message: &'m str,
    offsets: &[Option<usize>],
    start: i64,
    end: i64,
) -> Option<&'m str> {
    let from = (*offsets.get(usize::try_from(start).ok()?)?)?;
    let to = (*offsets.get(usize::try_from(end).ok()?)?)?;

    message.get(from..to)
}

// A tree describes the message, so what it can hold follows from the message's
// length: the densest construction, a run of empty segments, contributes two
// nodes per character. A structure that holds itself would be walked forever,
// so the walk is bounded by more than any tree of the message can hold, and a
// cyclic answer fails the case instead of hanging the run.
fn budget(units: usize) -> usize {
    units.saturating_mul(4).saturating_add(64)
}

struct Walked<'m> {
    node: usize,
    text: &'m str,
    depth: usize,
}

// Every node of the tree, in document order, each with the text its span
// covers, or the first thing wrong with one of them, in the order CST.md
// states them: the kinds of section 6, the spans of section 4. The walk keeps
// its own stack, so a deep answer costs memory and not the call stack.
fn walk<'m>(
    tree: &TreeAnswer,
    message: &'m str,
    offsets: &[Option<usize>],
) -> Result<Vec<Walked<'m>>, Failure> {
    let units = offsets.len() - 1;
    let limit = budget(units);
    let mut walked = Vec::new();
    let mut stack: Vec<(usize, usize, Option<usize>)> = vec![(0, 0, None)];

    while let Some((index, depth, within)) = stack.pop() {
        if walked.len() > limit {
            return Err(failure(
                "The tree holds more nodes than the message can spell.",
                js::quote(&format!("at most {limit} nodes")),
                js::quote("more"),
            ));
        }

        let Some(node) = tree.nodes.get(index) else {
            return Err(failure(
                "The tree holds something that is not a node.",
                js::quote("a node"),
                format!("the index {index} of {} nodes", tree.nodes.len()),
            ));
        };
        let kind = node.kind.as_str();

        if !KINDS.contains(&kind) {
            return Err(failure(
                "The tree holds a node of no kind this document names.",
                js::quote(&KINDS.join(", ")),
                js::quote(kind),
            ));
        }

        let span = pair(node.start, node.end);

        if node.end < node.start {
            return Err(failure(
                format!("The {kind} node ends before it starts."),
                js::quote("end at or after start"),
                span,
            ));
        }

        if node.start < 0 || usize::try_from(node.end).map_or(true, |end| end > units) {
            return Err(failure(
                format!("The {kind} node lies outside the message."),
                js::quote(&format!("within [0,{units})")),
                span,
            ));
        }

        if let Some(parent) = within.and_then(|parent| tree.nodes.get(parent))
            && (node.start < parent.start || node.end > parent.end)
        {
            return Err(failure(
                format!(
                    "The {kind} node lies outside the {} node that holds it.",
                    parent.kind
                ),
                pair(parent.start, parent.end),
                span,
            ));
        }

        let Some(text) = spelled(message, offsets, node.start, node.end) else {
            return Err(failure(
                format!("The span of the {kind} node falls inside a code point."),
                js::quote("a boundary between code points"),
                span,
            ));
        };

        if NAMED.contains(&kind) && node.name.is_none() {
            return Err(failure(
                format!("The {kind} node carries no name."),
                js::quote("the span unescaped"),
                "undefined".to_owned(),
            ));
        }

        if kind == "escape" && node.cancels.is_none() {
            return Err(failure(
                "The escape node does not say which reading it takes.",
                js::quote("true or false"),
                "undefined".to_owned(),
            ));
        }

        walked.push(Walked {
            node: index,
            text,
            depth,
        });

        for &child in node.children.iter().rev() {
            stack.push((child, depth + 1, Some(index)));
        }
    }

    Ok(walked)
}

// Properties 1, 2 and 3 of CST.md section 5, which are what a consumer walks a
// tree by: the leaves tile the message in order and spell it back.
fn tiling(
    tree: &TreeAnswer,
    walked: &[Walked<'_>],
    message: &str,
    units: usize,
) -> Option<Failure> {
    let mut at = 0;
    let mut spelled = String::with_capacity(message.len());

    for entry in walked {
        let node = &tree.nodes[entry.node];

        if !node.children.is_empty() {
            continue;
        }

        if node.start != at {
            return Some(failure(
                format!(
                    "The leaves do not tile the message: a {} node starts where the one before it did not end.",
                    node.kind
                ),
                at.to_string(),
                node.start.to_string(),
            ));
        }

        at = node.end;
        spelled.push_str(entry.text);
    }

    if usize::try_from(at).ok() != Some(units) {
        return Some(failure(
            "The leaves do not reach the end of the message.",
            units.to_string(),
            at.to_string(),
        ));
    }

    (spelled != message).then(|| {
        failure(
            "The leaves do not spell the message back.",
            js::quote(message),
            js::quote(&spelled),
        )
    })
}

fn optional_text(value: Option<&str>) -> String {
    value.map_or_else(|| "undefined".to_owned(), js::quote)
}

fn optional_flag(value: Option<bool>) -> String {
    value.map_or_else(|| "undefined".to_owned(), |flag| flag.to_string())
}

// The tree as the runner reads it, for comparing one parse with another.
fn shape(tree: &TreeAnswer, walked: &[Walked<'_>]) -> String {
    walked
        .iter()
        .map(|entry| {
            let node = &tree.nodes[entry.node];

            format!(
                "{}:{}:{}:{}:{}",
                entry.depth,
                node.kind,
                js::quote(entry.text),
                optional_text(node.name.as_deref()),
                optional_flag(node.cancels)
            )
        })
        .collect::<Vec<_>>()
        .join("|")
}

fn kinds<'t>(tree: &'t TreeAnswer, children: &[usize]) -> String {
    js::show(&Value::array(
        children
            .iter()
            .map(|&child| {
                tree.nodes
                    .get(child)
                    .map_or(Value::Undefined, |node: &'t RNode| {
                        node.kind.as_str().into()
                    })
            })
            .collect(),
    ))
}

// What a case expects of each node, against what the implementation answered,
// depth first as the case writes them, on a stack of its own.
fn compare(
    tree: &TreeAnswer,
    expected: &[ExpectedNode],
    texts: &HashMap<usize, &str>,
) -> Option<Failure> {
    struct Frame<'e> {
        expected: &'e [ExpectedNode],
        actual: Vec<usize>,
        place: String,
        next: usize,
    }

    let enter = |expected: &[ExpectedNode], actual: &[usize], place: &str| {
        (expected.len() != actual.len()).then(|| {
            failure(
                format!(
                    "The children of {place} are {} nodes rather than {}.",
                    actual.len(),
                    expected.len()
                ),
                js::show(&Value::array(
                    expected
                        .iter()
                        .map(|node| node.kind.as_str().into())
                        .collect(),
                )),
                kinds(tree, actual),
            )
        })
    };
    let root = &tree.nodes[0].children;

    if let Some(wrong) = enter(expected, root, "the message") {
        return Some(wrong);
    }

    let mut stack = vec![Frame {
        expected,
        actual: root.clone(),
        place: "the message".to_owned(),
        next: 0,
    }];

    while let Some(frame) = stack.last_mut() {
        let at = frame.next;
        let (Some(wanted), Some(&index)) = (frame.expected.get(at), frame.actual.get(at)) else {
            stack.pop();
            continue;
        };

        frame.next += 1;

        let position = format!("Node {} of {}", at + 1, frame.place);
        let node = tree.nodes.get(index)?;

        if node.kind != wanted.kind {
            return Some(failure(
                format!("{position} is of another kind."),
                js::quote(&wanted.kind),
                js::quote(&node.kind),
            ));
        }

        let text = texts.get(&index).copied();

        if text != Some(wanted.text.as_str()) {
            return Some(failure(
                format!("{position} spans other text."),
                js::quote(&wanted.text),
                optional_text(text),
            ));
        }

        if let Some(name) = &wanted.name
            && node.name.as_ref() != Some(name)
        {
            return Some(failure(
                format!("{position} is unescaped to something else."),
                js::quote(name),
                optional_text(node.name.as_deref()),
            ));
        }

        if let Some(cancels) = wanted.cancels
            && node.cancels != Some(cancels)
        {
            return Some(failure(
                format!("{position} takes the other reading of section 7."),
                cancels.to_string(),
                optional_flag(node.cancels),
            ));
        }

        let Some(nodes) = &wanted.nodes else {
            continue;
        };
        let place = format!("the {} node", node.kind);

        if let Some(wrong) = enter(nodes, &node.children, &place) {
            return Some(wrong);
        }

        stack.push(Frame {
            expected: nodes,
            actual: node.children.clone(),
            place,
            next: 0,
        });
    }

    None
}

// What the adapter answered, checked before it is read: a tree it raised over,
// or one that is not a message node, fails the case rather than ending the run.
fn answer(adapter: &dyn Adapter, message: &str) -> Result<TreeAnswer, Failure> {
    let tree = contained(|| adapter.parse(message)).map_err(|raised| {
        failure(
            "The adapter raised rather than answering with a tree.",
            js::quote("a message node"),
            js::quote(&raised),
        )
    })?;

    match tree.nodes.first() {
        Some(root) if root.kind == "message" => Ok(tree),
        root => Err(failure(
            "The adapter answered with no tree.",
            js::quote("a message node"),
            root.map_or_else(|| "undefined".to_owned(), |root| js::quote(&root.kind)),
        )),
    }
}

/// Runs a tree case. The properties of `CST.md` section 5 are checked on the
/// tree the implementation answered with, in the unit it declared, before the
/// case's own expectation is compared: a tree whose leaves do not tile the
/// message is wrong about the message whatever the case says of its nodes.
pub(crate) fn execute(adapter: &dyn Adapter, case: &TreeCase) -> Result<Outcome, RunError> {
    let Some(claim) = adapter.cst() else {
        return Err(RunError::new(format!(
            "The case {} was planned against an adapter that offers no tree.",
            case.id
        )));
    };

    if !UNITS.contains(&claim.unit.as_str()) {
        return Err(RunError::new(format!(
            "The adapter counts its spans in {}; the units are {}.",
            js::quote(&claim.unit),
            UNITS.join(", ")
        )));
    }

    Ok(match tree_outcome(adapter, case, &claim.unit) {
        Ok(()) => Outcome::Passed,
        Err(failure) => Outcome::Failed(failure),
    })
}

fn tree_outcome(adapter: &dyn Adapter, case: &TreeCase, unit: &str) -> Result<(), Failure> {
    let message = case.message.as_str();
    let offsets = boundaries(message, unit);
    let units = offsets.len() - 1;
    let tree = answer(adapter, message)?;
    let root = &tree.nodes[0];

    if root.start != 0 || usize::try_from(root.end).ok() != Some(units) {
        return Err(failure(
            "The tree does not span the whole message.",
            format!("[0,{units}]"),
            pair(root.start, root.end),
        ));
    }

    let walked = walk(&tree, message, &offsets)?;

    if let Some(wrong) = tiling(&tree, &walked, message, units) {
        return Err(wrong);
    }

    let texts: HashMap<usize, &str> = walked
        .iter()
        .map(|entry| (entry.node, entry.text))
        .collect();

    if let Some(wrong) = compare(&tree, &case.expected, &texts) {
        return Err(wrong);
    }

    let again = answer(adapter, message)?;
    let twice = walk(&again, message, &offsets)?;
    let (first, second) = (shape(&tree, &walked), shape(&again, &twice));

    if first != second {
        return Err(failure(
            "The tree is not a function of the message alone: two parses of it differ.",
            js::quote(&first),
            js::quote(&second),
        ));
    }

    let Some(resolves) = &case.resolves else {
        return Ok(());
    };
    let resolved =
        contained(|| adapter.resolve(&Resolution::message(message))).map_err(|raised| {
            failure(
                "The adapter raised resolving the message the tree describes.",
                js::quote(resolves),
                js::quote(&raised),
            )
        })?;
    let output = resolved.as_ref().map(|resolved| resolved.output.as_str());

    if output == Some(resolves.as_str()) {
        Ok(())
    } else {
        Err(failure(
            "The tree and the resolution do not read the same placeholders.",
            js::quote(resolves),
            optional_text(output),
        ))
    }
}
