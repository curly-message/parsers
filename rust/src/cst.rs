use std::fmt;
use std::ops::Range;

use crate::scan::{Scanner, char_at, escape_end};
use crate::syntax::{cancels, find, named, trimmed};

/// What a node of the tree is (`CST.md`, section 6).
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
#[non_exhaustive]
pub enum NodeKind {
    /// The root, spanning the whole message.
    Message,
    /// Characters with no structural meaning where they stand.
    Text,
    /// A backslash and the character it consumes.
    Escape,
    /// A placeholder, from its `{{` to its `}}`.
    Placeholder,
    /// The `{{` that opens a placeholder.
    Open,
    /// The `}}` that closes a placeholder.
    Close,
    /// A `:` or a `;` that divides a placeholder.
    Separator,
    /// Padding dropped from a name or an option value.
    Space,
    /// The payload key a placeholder names.
    Key,
    /// The modifier a placeholder names.
    Modifier,
    /// The key of an option.
    OptionKey,
    /// The value of an option: message text.
    OptionValue,
}

impl NodeKind {
    /// The kind as `CST.md` spells it.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Message => "message",
            Self::Text => "text",
            Self::Escape => "escape",
            Self::Placeholder => "placeholder",
            Self::Open => "open",
            Self::Close => "close",
            Self::Separator => "separator",
            Self::Space => "space",
            Self::Key => "key",
            Self::Modifier => "modifier",
            Self::OptionKey => "option-key",
            Self::OptionValue => "option-value",
        }
    }
}

#[derive(Clone, Debug)]
struct NodeData {
    kind: NodeKind,
    start: usize,
    end: usize,
    children: Range<usize>,
    name: Option<Range<usize>>,
    cancels: Option<bool>,
}

/// The concrete syntax tree of a message (`CST.md`).
///
/// Spans are in bytes of UTF-8, the unit a `str` is indexed by, so
/// `&message[node.span()]` is the text a node covers. The nodes live in one
/// arena, so a tree nested any depth is built and dropped without a call per
/// level.
#[derive(Clone, Debug)]
pub struct Tree {
    nodes: Vec<NodeData>,
    names: String,
}

impl Tree {
    /// The `message` node.
    pub fn root(&self) -> NodeRef<'_> {
        NodeRef {
            tree: self,
            index: 0,
        }
    }
}

/// A node of a [`Tree`].
#[derive(Clone, Copy)]
pub struct NodeRef<'t> {
    tree: &'t Tree,
    index: usize,
}

impl<'t> NodeRef<'t> {
    fn data(&self) -> Option<&'t NodeData> {
        self.tree.nodes.get(self.index)
    }

    /// What the node is.
    pub fn kind(&self) -> NodeKind {
        self.data().map_or(NodeKind::Message, |node| node.kind)
    }

    /// The bytes of the message the node covers.
    pub fn span(&self) -> Range<usize> {
        self.data().map_or(0..0, |node| node.start..node.end)
    }

    /// The name a `key`, a `modifier` or an `option-key` answers to: its span
    /// with every escape sequence resolved. Every other kind carries none.
    pub fn name(&self) -> Option<&'t str> {
        self.data()
            .and_then(|node| node.name.clone())
            .and_then(|range| self.tree.names.get(range))
    }

    /// Whether an `escape` cancels a structural meaning (section 7). Every
    /// other kind carries none.
    pub fn cancels(&self) -> Option<bool> {
        self.data().and_then(|node| node.cancels)
    }

    /// The node's children, in the order the message writes them.
    pub fn children(&self) -> Children<'t> {
        Children {
            tree: self.tree,
            indices: self.data().map_or(0..0, |node| node.children.clone()),
        }
    }
}

impl fmt::Debug for NodeRef<'_> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let mut node = f.debug_struct("NodeRef");

        node.field("kind", &self.kind()).field("span", &self.span());

        if let Some(name) = self.name() {
            node.field("name", &name);
        }

        if let Some(cancels) = self.cancels() {
            node.field("cancels", &cancels);
        }

        node.finish_non_exhaustive()
    }
}

/// The children of a node.
#[derive(Clone, Debug)]
pub struct Children<'t> {
    tree: &'t Tree,
    indices: Range<usize>,
}

impl<'t> Iterator for Children<'t> {
    type Item = NodeRef<'t>;

    fn next(&mut self) -> Option<NodeRef<'t>> {
        self.indices.next().map(|index| NodeRef {
            tree: self.tree,
            index,
        })
    }

    fn size_hint(&self) -> (usize, Option<usize>) {
        self.indices.size_hint()
    }
}

impl DoubleEndedIterator for Children<'_> {
    fn next_back(&mut self) -> Option<Self::Item> {
        self.indices.next_back().map(|index| NodeRef {
            tree: self.tree,
            index,
        })
    }
}

impl ExactSizeIterator for Children<'_> {}

// The nodes still to be filled, each with the span it is filled from. Nesting
// is not otherwise limited (section 6, note 10), so the tree is filled off
// this list rather than by a call per level.
enum Work {
    Contents {
        parent: usize,
        from: usize,
        to: usize,
    },
    Placeholder {
        parent: usize,
    },
    Spelled {
        parent: usize,
    },
}

struct Builder<'m> {
    scan: Scanner<'m>,
    nodes: Vec<NodeData>,
    names: String,
    work: Vec<Work>,
}

impl Builder<'_> {
    fn push(&mut self, kind: NodeKind, start: usize, end: usize) -> usize {
        let index = self.nodes.len();

        self.nodes.push(NodeData {
            kind,
            start,
            end,
            children: 0..0,
            name: None,
            cancels: None,
        });

        index
    }

    fn adopt(&mut self, parent: usize, first: usize) {
        let last = self.nodes.len();

        if let Some(node) = self.nodes.get_mut(parent) {
            node.children = first..last;
        }
    }

    fn span(&self, index: usize) -> (usize, usize) {
        self.nodes
            .get(index)
            .map_or((0, 0), |node| (node.start, node.end))
    }

    // Text with every escape sequence in it marked. A backslash with nothing
    // left to consume denotes itself and is text.
    fn spelled(&mut self, from: usize, to: usize) {
        let text = self.scan.text();
        let bytes = text.as_bytes();
        let mut start = from;
        let mut at = from;

        while at < to {
            if bytes.get(at) != Some(&b'\\') || at.saturating_add(1) >= to {
                at = at.saturating_add(1);
                continue;
            }

            if at > start {
                self.push(NodeKind::Text, start, at);
            }

            let end = escape_end(text, at, to);
            let escape = self.push(NodeKind::Escape, at, end);

            if let Some(node) = self.nodes.get_mut(escape) {
                node.cancels = Some(char_at(text, at.saturating_add(1)).is_some_and(cancels));
            }

            at = end;
            start = end;
        }

        if to > start {
            self.push(NodeKind::Text, start, to);
        }
    }

    fn contents(&mut self, parent: usize, from: usize, to: usize) {
        let first = self.nodes.len();
        let mut placeholders = Vec::new();
        let mut at = from;

        while let Some((open, close)) = self.scan.next(at, to) {
            self.spelled(at, open);
            placeholders.push(self.push(NodeKind::Placeholder, open, close));
            at = close;
        }

        self.spelled(at, to);
        self.adopt(parent, first);
        self.work.extend(
            placeholders
                .into_iter()
                .map(|parent| Work::Placeholder { parent }),
        );
    }

    // A name and the padding it is read without. The padding is nodes of its
    // own, so the parts still tile the message.
    fn named(&mut self, kind: NodeKind, from: usize, to: usize) {
        let text = self.scan.text();
        let name = named(text, from, to);

        if name.start > from {
            self.push(NodeKind::Space, from, name.start);
        }

        let node = self.push(kind, name.start, name.end);
        let begin = self.names.len();

        self.names.push_str(&name.name);

        let range = begin..self.names.len();

        if let Some(data) = self.nodes.get_mut(node) {
            data.name = Some(range);
        }

        self.work.push(Work::Spelled { parent: node });

        if to > name.end {
            self.push(NodeKind::Space, name.end, to);
        }
    }

    fn valued(&mut self, from: usize, to: usize) {
        let (start, end) = trimmed(self.scan.text(), from, to);

        if start > from {
            self.push(NodeKind::Space, from, start);
        }

        let node = self.push(NodeKind::OptionValue, start, end);

        self.work.push(Work::Contents {
            parent: node,
            from: start,
            to: end,
        });

        if to > end {
            self.push(NodeKind::Space, end, to);
        }
    }

    fn placeholder(&mut self, parent: usize) {
        let text = self.scan.text();
        let (open, close) = self.span(parent);
        let (inner, last) = (open.saturating_add(2), close.saturating_sub(2));
        let first = self.nodes.len();

        self.push(NodeKind::Open, open, inner);

        // A `;` a nested placeholder encloses divides that placeholder and not
        // this one (note 5), so the segments are cut around what each option
        // value holds.
        let declaration = find(text, inner, last, b';', Some(&self.scan)).unwrap_or(last);

        // The selector's colon is the first one no escape sequence claims, and
        // everything after it is the modifier name (note 3).
        match find(text, inner, declaration, b':', None) {
            Some(colon) => {
                self.named(NodeKind::Key, inner, colon);
                self.push(NodeKind::Separator, colon, colon.saturating_add(1));
                self.named(NodeKind::Modifier, colon.saturating_add(1), declaration);
            }
            None => self.named(NodeKind::Key, inner, declaration),
        }

        let mut separator = declaration;

        while separator < last {
            let from = separator.saturating_add(1);
            let to = find(text, from, last, b';', Some(&self.scan)).unwrap_or(last);

            self.push(NodeKind::Separator, separator, from);

            // A segment that states no value stands for its own key
            // (section 9.4), so there is no value to describe.
            match find(text, from, to, b':', Some(&self.scan)) {
                Some(colon) => {
                    self.named(NodeKind::OptionKey, from, colon);
                    self.push(NodeKind::Separator, colon, colon.saturating_add(1));
                    self.valued(colon.saturating_add(1), to);
                }
                None => self.named(NodeKind::OptionKey, from, to),
            }

            separator = to;
        }

        self.push(NodeKind::Close, last, close);
        self.adopt(parent, first);
    }
}

/// Describes a message: where its placeholders are, what each is made of, and
/// where every escape sequence falls (`CST.md`).
///
/// The placeholders are the ones resolution finds, because the same scan finds
/// them: a placeholder written in an option value is described there, and a
/// construct that derives nothing is text.
///
/// ```
/// use curly_message_parser::{NodeKind, cst};
///
/// let message = "Hi {{name}}";
/// let tree = cst(message);
/// let placeholder = tree.root().children().nth(1).unwrap();
///
/// assert_eq!(placeholder.kind(), NodeKind::Placeholder);
/// assert_eq!(&message[placeholder.span()], "{{name}}");
/// assert_eq!(placeholder.children().nth(1).unwrap().name(), Some("name"));
/// ```
pub fn cst(message: &str) -> Tree {
    let mut builder = Builder {
        scan: Scanner::new(message),
        nodes: Vec::new(),
        names: String::new(),
        work: Vec::new(),
    };

    builder.push(NodeKind::Message, 0, message.len());
    builder.work.push(Work::Contents {
        parent: 0,
        from: 0,
        to: message.len(),
    });

    while let Some(work) = builder.work.pop() {
        match work {
            Work::Contents { parent, from, to } => builder.contents(parent, from, to),
            Work::Placeholder { parent } => builder.placeholder(parent),
            Work::Spelled { parent } => {
                let first = builder.nodes.len();
                let (from, to) = builder.span(parent);

                builder.spelled(from, to);
                builder.adopt(parent, first);
            }
        }
    }

    Tree {
        nodes: builder.nodes,
        names: builder.names,
    }
}
