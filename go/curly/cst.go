package curly

import "strconv"

// Kind is the kind of a Node, spelled as section 6 of CST.md spells it.
type Kind string

// The twelve kinds of CST.md section 6.
const (
	KindMessage     Kind = "message"
	KindText        Kind = "text"
	KindEscape      Kind = "escape"
	KindPlaceholder Kind = "placeholder"
	KindOpen        Kind = "open"
	KindClose       Kind = "close"
	KindSeparator   Kind = "separator"
	KindSpace       Kind = "space"
	KindKey         Kind = "key"
	KindModifier    Kind = "modifier"
	KindOptionKey   Kind = "option-key"
	KindOptionValue Kind = "option-value"
)

// Node is a node of the concrete syntax tree of CST.md: a kind and a span
// over the message, in bytes of UTF-8, with the children of the kinds that
// take them and the fields of the kinds that carry them.
type Node struct {
	Kind Kind
	// Start and End are the half-open span [Start, End) as byte offsets into
	// the message. No boundary falls inside a UTF-8 sequence; a byte that
	// starts no valid sequence is one character, and a boundary may fall on
	// either side of it.
	Start, End int
	// Nodes are the children, in the order the message writes them. The
	// message, a placeholder, a name kind and an option value take them.
	Nodes []Node
	// Name is the span unescaped, on the name kinds: key, modifier and
	// option-key.
	Name string
	// Cancels is set on an escape whose backslash cancels a structural
	// meaning, so that removing the sequence leaves the character alone.
	Cancels bool
}

func (k Kind) hasNodes() bool {
	switch k {
	case KindMessage, KindPlaceholder, KindKey, KindModifier, KindOptionKey, KindOptionValue:
		return true
	}

	return false
}

func (k Kind) isName() bool {
	return k == KindKey || k == KindModifier || k == KindOptionKey
}

// rawNode is a node while the tree is built: the option values are filled
// after the placeholder holding them, so children are a linked list into one
// slice rather than slices that would have to grow in place.
type rawNode struct {
	kind              Kind
	start, end        int
	name              string
	cancels           bool
	first, last, next int
}

type treeBuilder struct {
	s       string
	sc      *scanner
	nodes   []rawNode
	pending [][3]int
}

func (b *treeBuilder) add(parent int, n rawNode) int {
	n.first, n.last, n.next = -1, -1, -1
	b.nodes = append(b.nodes, n)
	index := len(b.nodes) - 1

	if parent >= 0 {
		p := &b.nodes[parent]

		if p.last < 0 {
			p.first = index
		} else {
			b.nodes[p.last].next = index
		}

		p.last = index
	}

	return index
}

// spelled divides text into text and escape nodes. A backslash with nothing
// left in the span to consume denotes itself and is text.
func (b *treeBuilder) spelled(parent, from, to int) {
	s := b.s
	start := from

	for i := from; i < to; i++ {
		if s[i] != '\\' || i+1 >= to {
			continue
		}

		if i > start {
			b.add(parent, rawNode{kind: KindText, start: start, end: i})
		}

		end := escapeEnd(s, i, to)
		r, width := runeAt(s[:end], i+1)
		b.add(parent, rawNode{kind: KindEscape, start: i, end: end, cancels: cancels(r, width)})
		i = end - 1
		start = end
	}

	if to > start {
		b.add(parent, rawNode{kind: KindText, start: start, end: to})
	}
}

// contents describes a span of the message itself: the message, or an
// option value.
func (b *treeBuilder) contents(parent, from, to int) {
	at := from

	for {
		open, close, ok := b.sc.next(at, to)

		if !ok {
			break
		}

		b.spelled(parent, at, open)
		b.placeholder(parent, open, close)
		at = close
	}

	b.spelled(parent, at, to)
}

// named describes a name and the padding it is read without, as nodes of
// their own so that the leaves still tile the message.
func (b *treeBuilder) named(parent int, kind Kind, from, to int) {
	start, end := trimmed(b.s, from, to)

	if start > from {
		b.add(parent, rawNode{kind: KindSpace, start: from, end: start})
	}

	name := b.add(parent, rawNode{kind: kind, start: start, end: end, name: unesc(b.s[start:end])})
	b.spelled(name, start, end)

	if to > end {
		b.add(parent, rawNode{kind: KindSpace, start: end, end: to})
	}
}

// valued describes an option value, leaving what it holds to be filled from
// pending, which keeps the build off the goroutine's stack however deep a
// message nests.
func (b *treeBuilder) valued(parent, from, to int) {
	start, end := trimmed(b.s, from, to)

	if start > from {
		b.add(parent, rawNode{kind: KindSpace, start: from, end: start})
	}

	value := b.add(parent, rawNode{kind: KindOptionValue, start: start, end: end})
	b.pending = append(b.pending, [3]int{value, start, end})

	if to > end {
		b.add(parent, rawNode{kind: KindSpace, start: end, end: to})
	}
}

func (b *treeBuilder) placeholder(parent, open, close int) {
	s := b.s
	p := b.add(parent, rawNode{kind: KindPlaceholder, start: open, end: close})
	b.add(p, rawNode{kind: KindOpen, start: open, end: open + 2})

	segments := separated(s, open+2, close-2, ';', b.sc)
	selector := separated(s, segments[0][0], segments[0][1], ':', nil)
	key := selector[0]
	b.named(p, KindKey, key[0], key[1])

	if len(selector) > 1 {
		b.add(p, rawNode{kind: KindSeparator, start: key[1], end: key[1] + 1})
		b.named(p, KindModifier, key[1]+1, segments[0][1])
	}

	for _, seg := range segments[1:] {
		b.add(p, rawNode{kind: KindSeparator, start: seg[0] - 1, end: seg[0]})

		parts := separated(s, seg[0], seg[1], ':', b.sc)
		optionKey := parts[0]
		b.named(p, KindOptionKey, optionKey[0], optionKey[1])

		// A segment that states no value stands for its own key (section
		// 9.4), so there is no value to describe.
		if len(parts) > 1 {
			b.add(p, rawNode{kind: KindSeparator, start: optionKey[1], end: optionKey[1] + 1})
			b.valued(p, optionKey[1]+1, seg[1])
		}
	}

	b.add(p, rawNode{kind: KindClose, start: close - 2, end: close})
}

// CST describes a message as it is written: where its placeholders are, what
// each is made of, and where every escape sequence falls (CST.md). The
// placeholders are the ones Resolve finds, because the same scan finds them.
// Spans are byte offsets (CST.md section 4, unit utf-8). The tree is built
// without recursion, at any depth of nesting.
func CST(message string) Node {
	return build(message).tree()
}

func build(message string) *treeBuilder {
	b := &treeBuilder{s: message, sc: newScanner(message)}
	b.add(-1, rawNode{kind: KindMessage, start: 0, end: len(message)})
	b.pending = append(b.pending, [3]int{0, 0, len(message)})

	for len(b.pending) > 0 {
		work := b.pending[len(b.pending)-1]
		b.pending = b.pending[:len(b.pending)-1]
		b.contents(work[0], work[1], work[2])
	}

	return b
}

// tree turns the linked nodes into the Node values a caller holds, each
// parent's children allocated at their final length so that a child's
// address stays where it was taken.
func (b *treeBuilder) tree() Node {
	var root Node

	type fill struct {
		raw    int
		target *Node
	}

	stack := []fill{{0, &root}}

	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		raw := &b.nodes[f.raw]
		*f.target = Node{Kind: raw.kind, Start: raw.start, End: raw.end, Name: raw.name, Cancels: raw.cancels}

		if !raw.kind.hasNodes() {
			continue
		}

		count := 0

		for c := raw.first; c >= 0; c = b.nodes[c].next {
			count++
		}

		f.target.Nodes = make([]Node, count)
		i := 0

		for c := raw.first; c >= 0; c = b.nodes[c].next {
			stack = append(stack, fill{c, &f.target.Nodes[i]})
			i++
		}
	}

	return root
}

// AppendJSON appends the interchange encoding of CST.md section 8 to dst: an
// object per node carrying only the fields its kind takes, "nodes" present,
// possibly empty, on the kinds that hold children. It walks the tree on a
// stack of its own and writes into one buffer, so it is linear in the tree
// at any depth.
func (n Node) AppendJSON(dst []byte) []byte {
	return n.appendJSON(dst, nil)
}

func (n *Node) appendJSON(dst []byte, steps *int) []byte {
	type open struct {
		node *Node
		next int
	}

	dst = appendNodeHead(dst, n)

	if !n.Kind.hasNodes() {
		return append(dst, '}')
	}

	stack := []open{{n, 0}}

	for len(stack) > 0 {
		if steps != nil {
			*steps++
		}

		top := &stack[len(stack)-1]

		if top.next == len(top.node.Nodes) {
			dst = append(dst, ']', '}')
			stack = stack[:len(stack)-1]

			continue
		}

		if top.next > 0 {
			dst = append(dst, ',')
		}

		child := &top.node.Nodes[top.next]
		top.next++
		dst = appendNodeHead(dst, child)

		if child.Kind.hasNodes() {
			stack = append(stack, open{child, 0})
		} else {
			dst = append(dst, '}')
		}
	}

	return dst
}

func appendNodeHead(dst []byte, n *Node) []byte {
	dst = append(dst, `{"type":`...)
	dst = appendJSONString(dst, string(n.Kind))
	dst = append(dst, `,"start":`...)
	dst = strconv.AppendInt(dst, int64(n.Start), 10)
	dst = append(dst, `,"end":`...)
	dst = strconv.AppendInt(dst, int64(n.End), 10)

	if n.Kind.isName() {
		dst = append(dst, `,"name":`...)
		dst = appendJSONString(dst, n.Name)
	}

	if n.Kind == KindEscape {
		dst = append(dst, `,"cancels":`...)
		dst = strconv.AppendBool(dst, n.Cancels)
	}

	if n.Kind.hasNodes() {
		dst = append(dst, `,"nodes":[`...)
	}

	return dst
}

// MarshalJSON is AppendJSON(nil). encoding/json re-reads what a Marshaler
// writes: json.Marshal escapes <, > and & in it, which an Encoder with
// SetEscapeHTML(false) leaves as AppendJSON writes them, and refuses JSON
// nested past its own limit of 10000 levels, so json.Marshal of a tree
// nested past about 5000 placeholders returns an error; AppendJSON writes
// such a tree.
func (n Node) MarshalJSON() ([]byte, error) {
	return n.AppendJSON(nil), nil
}
