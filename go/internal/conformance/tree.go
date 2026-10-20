package conformance

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// units are the units CST.md section 4 lets an implementation count its
// spans in.
var units = []SpanUnit{UTF8, UTF16, CodePoint}

// nodeTypes are the node kinds of CST.md section 6.
var nodeTypes = []string{"message", "placeholder", "open", "close", "separator", "space", "key", "modifier", "option-key", "option-value", "text", "escape"}

// named are the kinds that carry a name (CST.md section 8). An option-value
// is not one of them: it answers to nobody, so it carries the subtree the
// message spells and no string to compare. A field a kind does not take is
// not policed, because that table describes the interchange encoding and an
// adapter hands back a host value.
var named = []string{"key", "modifier", "option-key"}

func width(r rune, size int, unit SpanUnit) int {
	switch unit {
	case CodePoint:
		return 1
	case UTF16:
		if r >= 0x10000 {
			return 2
		}

		return 1
	}

	return size
}

// boundaries is where each boundary of the message falls, as a byte offset
// into the message, indexed by the unit the adapter declared. A boundary
// inside a code point has no offset, and is -1: a character wider than one
// unit contributes one entry for its start and -1 for each position within
// it, which is what CST.md section 4 forbids a span from landing on.
func boundaries(message string, unit SpanUnit) []int {
	var offsets []int

	for at := 0; at < len(message); {
		r, size := utf8.DecodeRuneInString(message[at:])
		offsets = append(offsets, at)

		for within := 1; within < width(r, size, unit); within++ {
			offsets = append(offsets, -1)
		}

		at += size
	}

	return append(offsets, len(message))
}

// treeBudget bounds the walk: a tree describes the message, and the densest
// construction, a run of empty segments, holds two nodes per character. A
// structure that holds itself would be walked forever, so the walk stops at
// more than any tree of the message can hold, and a cyclic answer fails the
// case instead of hanging the run.
func treeBudget(units int) int {
	return units*4 + 64
}

type walked struct {
	node  *Node
	text  string
	depth int
}

// walk is every node of the tree in document order, each with the text its
// span covers, or the first thing wrong with one of them in the order
// CST.md states them: the kinds of section 6, the spans of section 4.
func walk(root *Node, message string, offsets []int) ([]walked, *Outcome) {
	type visit struct {
		node   *Node
		depth  int
		within *Node
	}

	units := len(offsets) - 1
	budget := treeBudget(units)
	var nodes []walked
	stack := []visit{{root, 0, nil}}
	fail := func(reason string, expected, actual any) ([]walked, *Outcome) {
		outcome := failure(reason, expected, actual)

		return nil, &outcome
	}

	for len(stack) > 0 {
		next := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node := next.node

		if len(nodes) > budget {
			return fail("The tree holds more nodes than the message can spell.", fmt.Sprintf("at most %d nodes", budget), "more")
		}

		if node == nil {
			return fail("The tree holds something that is not a node.", "a node", nil)
		}

		if !slices.Contains(nodeTypes, node.Type) {
			return fail("The tree holds a node of no kind this document names.", strings.Join(nodeTypes, ", "), node.Type)
		}

		span := []int{node.Start, node.End}

		if node.End < node.Start {
			return fail(fmt.Sprintf("The %s node ends before it starts.", node.Type), "end at or after start", span)
		}

		if node.Start < 0 || node.End > units {
			return fail(fmt.Sprintf("The %s node lies outside the message.", node.Type), fmt.Sprintf("within [0,%d)", units), span)
		}

		if within := next.within; within != nil && (node.Start < within.Start || node.End > within.End) {
			return fail(fmt.Sprintf("The %s node lies outside the %s node that holds it.", node.Type, within.Type), []int{within.Start, within.End}, span)
		}

		from, to := offsets[node.Start], offsets[node.End]

		if from < 0 || to < 0 {
			return fail(fmt.Sprintf("The span of the %s node falls inside a code point.", node.Type), "a boundary between code points", span)
		}

		if slices.Contains(named, node.Type) && node.Name == nil {
			return fail(fmt.Sprintf("The %s node carries no name.", node.Type), "the span unescaped", nil)
		}

		if node.Type == "escape" && node.Cancels == nil {
			return fail("The escape node does not say which reading it takes.", "true or false", nil)
		}

		nodes = append(nodes, walked{node, message[from:to], next.depth})

		for i := len(node.Nodes) - 1; i >= 0; i-- {
			stack = append(stack, visit{node.Nodes[i], next.depth + 1, node})
		}
	}

	return nodes, nil
}

// tiling checks properties 1, 2 and 3 of CST.md section 5, which are what a
// consumer walks a tree by: the leaves tile the message in order and spell
// it back.
func tiling(nodes []walked, message string, units int) *Outcome {
	at := 0
	var spelled strings.Builder

	for _, w := range nodes {
		if len(w.node.Nodes) > 0 {
			continue
		}

		if w.node.Start != at {
			outcome := failure(fmt.Sprintf("The leaves do not tile the message: a %s node starts where the one before it did not end.", w.node.Type), at, w.node.Start)

			return &outcome
		}

		at = w.node.End
		spelled.WriteString(w.text)
	}

	if at != units {
		outcome := failure("The leaves do not reach the end of the message.", units, at)

		return &outcome
	}

	if spelled.String() != message {
		outcome := failure("The leaves do not spell the message back.", message, spelled.String())

		return &outcome
	}

	return nil
}

// shape is the tree as the runner reads it, for comparing one parse with
// another.
func shape(nodes []walked) string {
	written := make([]string, len(nodes))

	for i, w := range nodes {
		name, cancels := "undefined", "undefined"

		if w.node.Name != nil {
			name = shown(*w.node.Name)
		}

		if w.node.Cancels != nil {
			cancels = fmt.Sprint(*w.node.Cancels)
		}

		text, _ := stringify(w.text)
		written[i] = fmt.Sprintf("%d:%s:%s:%s:%s", w.depth, w.node.Type, text, name, cancels)
	}

	return strings.Join(written, "|")
}

func expectedTypes(expected []any) []any {
	types := make([]any, len(expected))

	for i, node := range expected {
		types[i] = node.(map[string]any)["type"]
	}

	return types
}

func actualTypes(actual []*Node) []any {
	types := make([]any, len(actual))

	for i, node := range actual {
		if node != nil {
			types[i] = node.Type
		}
	}

	return types
}

// compareTree holds the children of the root to what the case expects of
// them, node by node, each node's own children before the node after it.
func compareTree(expected []any, root *Node, texts map[*Node]string) *Outcome {
	type frame struct {
		expected []any
		actual   []*Node
		where    string
		next     int
	}

	var stack []frame
	enter := func(expected []any, actual []*Node, where string) *Outcome {
		if len(expected) != len(actual) {
			outcome := failure(fmt.Sprintf("The children of %s are %d nodes rather than %d.", where, len(actual), len(expected)), expectedTypes(expected), actualTypes(actual))

			return &outcome
		}

		stack = append(stack, frame{expected, actual, where, 0})

		return nil
	}

	if wrong := enter(expected, root.Nodes, "the message"); wrong != nil {
		return wrong
	}

	for len(stack) > 0 {
		top := &stack[len(stack)-1]

		if top.next == len(top.expected) {
			stack = stack[:len(stack)-1]

			continue
		}

		i := top.next
		top.next++
		wanted := top.expected[i].(map[string]any)
		node := top.actual[i]
		position := fmt.Sprintf("Node %d of %s", i+1, top.where)
		var reason string
		var want, got any

		switch name, hasName := wanted["name"].(string); {
		case node.Type != wanted["type"]:
			reason, want, got = position+" is of another kind.", wanted["type"], node.Type
		case texts[node] != wanted["text"]:
			reason, want, got = position+" spans other text.", wanted["text"], texts[node]
		case hasName && (node.Name == nil || *node.Name != name):
			reason, want, got = position+" is unescaped to something else.", name, node.Name
		}

		if cancels, ok := wanted["cancels"].(bool); ok && reason == "" && (node.Cancels == nil || *node.Cancels != cancels) {
			reason, want, got = position+" takes the other reading of section 7.", cancels, node.Cancels
		}

		if reason != "" {
			outcome := failure(reason, want, got)

			return &outcome
		}

		if children, ok := wanted["nodes"].([]any); ok {
			if wrong := enter(children, node.Nodes, "the "+node.Type+" node"); wrong != nil {
				return wrong
			}
		}
	}

	return nil
}

// parse puts the message to the adapter's tree. A panic, or an answer that
// is not a message node, fails the case rather than ending the run.
func parse(cst *CST, message string) (tree *Node, outcome *Outcome) {
	defer func() {
		if value := recover(); value != nil {
			failed := failure("The adapter raised rather than answering with a tree.", "a message node", raised(value))
			tree, outcome = nil, &failed
		}
	}()

	tree = cst.Parse(message)

	if tree == nil || tree.Type != "message" {
		failed := failure("The adapter answered with no tree.", "a message node", tree)

		return nil, &failed
	}

	return tree, nil
}

// resolveTree resolves the message the tree describes over no payload.
func resolveTree(adapter Adapter, message string) (output any, outcome *Outcome) {
	defer func() {
		if value := recover(); value != nil {
			output, outcome = nil, &Outcome{Actual: raised(value)}
		}
	}()

	resolved := adapter.Resolve(Resolution{Message: message, ID: adapter.Undefined})

	if resolved == nil {
		return nil, nil
	}

	return resolved.Output, nil
}

// executeTree runs a tree case. The properties of CST.md section 5 are
// checked on the tree the implementation answered with, in the unit it
// declared, before the case's own expectation is compared: a tree whose
// leaves do not tile the message is wrong about the message whatever the
// case says of its nodes.
func executeTree(adapter Adapter, c map[string]any) (Outcome, error) {
	cst := adapter.CST

	if cst == nil {
		return Outcome{}, fmt.Errorf("The case %s was planned against an adapter that offers no tree.", c["id"])
	}

	message, _ := c["message"].(string)
	expected, _ := c["expected"].([]any)
	offsets := boundaries(message, cst.Unit)
	units := len(offsets) - 1
	tree, failed := parse(cst, message)

	if failed != nil {
		return *failed, nil
	}

	if tree.Start != 0 || tree.End != units {
		return failure("The tree does not span the whole message.", []int{0, units}, []int{tree.Start, tree.End}), nil
	}

	nodes, failed := walk(tree, message, offsets)

	if failed != nil {
		return *failed, nil
	}

	if failed := tiling(nodes, message, units); failed != nil {
		return *failed, nil
	}

	texts := make(map[*Node]string, len(nodes))

	for _, w := range nodes {
		texts[w.node] = w.text
	}

	if failed := compareTree(expected, tree, texts); failed != nil {
		return *failed, nil
	}

	again, failed := parse(cst, message)

	if failed != nil {
		return *failed, nil
	}

	twice, failed := walk(again, message, offsets)

	if failed != nil {
		return *failed, nil
	}

	if shape(twice) != shape(nodes) {
		return failure("The tree is not a function of the message alone: two parses of it differ.", shape(nodes), shape(twice)), nil
	}

	resolves, ok := c["resolves"].(string)

	if !ok {
		return Outcome{OK: true}, nil
	}

	output, raisedOutcome := resolveTree(adapter, message)

	if raisedOutcome != nil {
		return failure("The adapter raised resolving the message the tree describes.", resolves, raisedOutcome.Actual), nil
	}

	if output != resolves {
		return failure("The tree and the resolution do not read the same placeholders.", resolves, output), nil
	}

	return Outcome{OK: true}, nil
}
