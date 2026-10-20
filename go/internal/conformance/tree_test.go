package conformance

import (
	"maps"
	"reflect"
	"testing"
)

func TestBoundaries(t *testing.T) {
	message := "a\u00e9\U0001F600"

	for unit, want := range map[SpanUnit][]int{
		UTF8:      {0, 1, -1, 3, -1, -1, -1, 7},
		UTF16:     {0, 1, 3, -1, 7},
		CodePoint: {0, 1, 3, 7},
	} {
		if got := boundaries(message, unit); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", unit, got, want)
		}
	}

	if got := boundaries("", UTF16); !reflect.DeepEqual(got, []int{0}) {
		t.Errorf("the empty message: got %v", got)
	}
}

func leaf(kind string, start, end int) *Node {
	return &Node{Type: kind, Start: start, End: end}
}

func message(start, end int, nodes ...*Node) *Node {
	return &Node{Type: "message", Start: start, End: end, Nodes: nodes}
}

// treeOutcome runs a tree case over "ab", expecting one text node, against
// an adapter whose tree is the one parse answers.
func treeOutcome(t testing.TB, parse func(string) *Node, resolve func(Resolution) *Resolved, over string) Outcome {
	t.Helper()

	c := obj(t, `{"id": "tree/case", "description": "Pins a tree.", "message": "ab", "expected": [{"type": "text", "text": "ab"}]}`)

	if over != "" {
		maps.Copy(c, obj(t, over))
	}

	if resolve == nil {
		resolve = echo
	}

	adapter := adapterOf(resolve)
	adapter.CST = &CST{Unit: UTF8, Parse: parse}
	outcome, err := executeTree(adapter, c)

	if err != nil {
		t.Fatal(err)
	}

	return outcome
}

func constant(tree func() *Node) func(string) *Node {
	return func(string) *Node { return tree() }
}

func TestExecuteTree(t *testing.T) {
	right := func() *Node { return message(0, 2, leaf("text", 0, 2)) }
	yes := true

	t.Run("passes the tree the case expects, parsed alike twice", func(t *testing.T) {
		if outcome := treeOutcome(t, constant(right), nil, ""); !outcome.OK {
			t.Errorf("got %+v", outcome)
		}
	})

	for _, c := range []struct {
		name   string
		tree   func() *Node
		over   string
		reason string
	}{
		{"a panic", func() *Node { panic("boom") }, "", "The adapter raised rather than answering with a tree."},
		{"no tree", func() *Node { return nil }, "", "The adapter answered with no tree."},
		{"a root of another kind", func() *Node { return leaf("text", 0, 2) }, "", "The adapter answered with no tree."},
		{"a root short of the message", func() *Node { return message(0, 1, leaf("text", 0, 1)) }, "", "The tree does not span the whole message."},
		{"a nil child", func() *Node { return message(0, 2, nil) }, "", "The tree holds something that is not a node."},
		{"an unknown kind", func() *Node { return message(0, 2, leaf("word", 0, 2)) }, "", "The tree holds a node of no kind this document names."},
		{"a span ending before it starts", func() *Node { return message(0, 2, leaf("text", 2, 1)) }, "", "The text node ends before it starts."},
		{"a span past the message", func() *Node { return message(0, 2, leaf("text", 0, 3)) }, "", "The text node lies outside the message."},
		{"a span outside its parent", func() *Node {
			return message(0, 2, &Node{Type: "placeholder", Start: 0, End: 1, Nodes: []*Node{leaf("text", 0, 2)}})
		}, "", "The text node lies outside the placeholder node that holds it."},
		{"a span inside a code point", func() *Node { return message(0, 3, leaf("text", 0, 1), leaf("text", 1, 3)) }, `{"message": "\u00e9b"}`, "The span of the text node falls inside a code point."},
		{"a key with no name", func() *Node { return message(0, 2, leaf("key", 0, 2)) }, "", "The key node carries no name."},
		{"an escape with no reading", func() *Node { return message(0, 2, leaf("escape", 0, 2)) }, "", "The escape node does not say which reading it takes."},
		{"a gap between leaves", func() *Node { return message(0, 2, leaf("text", 1, 2)) }, "", "The leaves do not tile the message: a text node starts where the one before it did not end."},
		{"leaves short of the end", func() *Node { return message(0, 2, leaf("text", 0, 1)) }, "", "The leaves do not reach the end of the message."},
		{"another count of children", func() *Node { return message(0, 2, leaf("text", 0, 1), leaf("text", 1, 2)) }, "", "The children of the message are 2 nodes rather than 1."},
		{"another kind", func() *Node { return message(0, 2, leaf("space", 0, 2)) }, "", "Node 1 of the message is of another kind."},
		{"other text", func() *Node { return message(0, 2, leaf("text", 0, 1), leaf("text", 1, 2)) }, `{"expected": [{"type": "text", "text": "a"}, {"type": "text", "text": "a"}]}`, "Node 2 of the message spans other text."},
		{"another name", func() *Node { return message(0, 2, &Node{Type: "key", Start: 0, End: 2, Name: ptr("b")}) }, `{"expected": [{"type": "key", "text": "ab", "name": "ab"}]}`, "Node 1 of the message is unescaped to something else."},
		{"the other reading", func() *Node { return message(0, 2, &Node{Type: "escape", Start: 0, End: 2, Cancels: &yes}) }, `{"expected": [{"type": "escape", "text": "ab", "cancels": false}]}`, "Node 1 of the message takes the other reading of section 7."},
		{"children of a child", func() *Node {
			return message(0, 2, &Node{Type: "placeholder", Start: 0, End: 2, Nodes: []*Node{leaf("open", 0, 2)}})
		}, `{"expected": [{"type": "placeholder", "text": "ab", "nodes": [{"type": "open", "text": "ab"}, {"type": "close", "text": ""}]}]}`, "The children of the placeholder node are 1 nodes rather than 2."},
	} {
		t.Run("fails "+c.name, func(t *testing.T) {
			expectReason(t, treeOutcome(t, constant(c.tree), nil, c.over), c.reason)
		})
	}

	t.Run("fails a tree that holds itself within its budget", func(t *testing.T) {
		cyclic := func() *Node {
			root := message(0, 2)
			root.Nodes = []*Node{root}

			return root
		}

		expectReason(t, treeOutcome(t, constant(cyclic), nil, ""), "The tree holds more nodes than the message can spell.")
	})

	t.Run("fails two parses that differ", func(t *testing.T) {
		calls := 0
		parse := func(string) *Node {
			calls++

			if calls == 1 {
				return right()
			}

			return message(0, 2, &Node{Type: "text", Start: 0, End: 2, Name: ptr("x")})
		}

		expectReason(t, treeOutcome(t, parse, nil, ""), "The tree is not a function of the message alone: two parses of it differ.")
	})

	t.Run("holds the tree to the resolution where the case names one", func(t *testing.T) {
		says := func(output string) func(Resolution) *Resolved {
			return func(Resolution) *Resolved { return &Resolved{Output: output} }
		}

		if outcome := treeOutcome(t, constant(right), says("ab"), `{"resolves": "ab"}`); !outcome.OK {
			t.Errorf("got %+v", outcome)
		}

		expectReason(t, treeOutcome(t, constant(right), says("a"), `{"resolves": "ab"}`), "The tree and the resolution do not read the same placeholders.")
		expectReason(t, treeOutcome(t, constant(right), func(Resolution) *Resolved { panic("boom") }, `{"resolves": "ab"}`), "The adapter raised resolving the message the tree describes.")
		expectReason(t, treeOutcome(t, constant(right), func(Resolution) *Resolved { return nil }, `{"resolves": "ab"}`), "The tree and the resolution do not read the same placeholders.")
	})

	t.Run("counts spans in the unit the adapter declares", func(t *testing.T) {
		c := obj(t, `{"id": "tree/astral", "description": "Pins a tree.", "message": "\ud83d\ude00", "expected": [{"type": "text", "text": "\ud83d\ude00"}]}`)

		for unit, end := range map[SpanUnit]int{UTF8: 4, UTF16: 2, CodePoint: 1} {
			adapter := adapterOf(echo)
			adapter.CST = &CST{Unit: unit, Parse: constant(func() *Node { return message(0, end, leaf("text", 0, end)) })}

			if outcome, err := executeTree(adapter, c); err != nil || !outcome.OK {
				t.Errorf("%s: got %+v, %v", unit, outcome, err)
			}
		}
	})
}
