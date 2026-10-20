package curly_test

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"github.com/curly-message/parsers/go/curly"
)

type span struct {
	kind       curly.Kind
	start, end int
}

func (s span) String() string {
	return fmt.Sprintf("%s[%d,%d)", s.kind, s.start, s.end)
}

// walk visits every node of a tree in the order the message writes them,
// parents first, on a stack of its own.
func walk(root curly.Node, visit func(n *curly.Node)) {
	stack := []*curly.Node{&root}

	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		visit(n)

		for i := len(n.Nodes) - 1; i >= 0; i-- {
			stack = append(stack, &n.Nodes[i])
		}
	}
}

func leaves(root curly.Node) []span {
	var out []span

	walk(root, func(n *curly.Node) {
		if len(n.Nodes) == 0 {
			out = append(out, span{n.Kind, n.Start, n.End})
		}
	})

	return out
}

func ofKind(root curly.Node, kind curly.Kind) []curly.Node {
	var out []curly.Node

	walk(root, func(n *curly.Node) {
		if n.Kind == kind {
			out = append(out, *n)
		}
	})

	return out
}

func spans(nodes []curly.Node) []span {
	out := make([]span, len(nodes))

	for i, n := range nodes {
		out[i] = span{n.Kind, n.Start, n.End}
	}

	return out
}

func sameSpans(t *testing.T, what string, got, want []span) {
	t.Helper()

	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s:\n got %v\nwant %v", what, got, want)
	}
}

// checkProperties holds a tree to section 5 of CST.md: every parent's
// children tile its span in order, so the leaves tile the message and spell
// it back, and no boundary falls inside a UTF-8 sequence.
func checkProperties(t *testing.T, message string, root curly.Node) {
	t.Helper()

	boundary := map[int]bool{len(message): true}

	for i := range message {
		boundary[i] = true
	}

	if root.Kind != curly.KindMessage || root.Start != 0 || root.End != len(message) {
		t.Fatalf("root %v of %q", span{root.Kind, root.Start, root.End}, message)
	}

	var failure string

	walk(root, func(n *curly.Node) {
		if failure != "" {
			return
		}

		if n.End < n.Start || !boundary[n.Start] || !boundary[n.End] {
			failure = fmt.Sprintf("span %v", span{n.Kind, n.Start, n.End})

			return
		}

		if len(n.Nodes) == 0 {
			return
		}

		at := n.Start

		for _, c := range n.Nodes {
			if c.Start != at {
				failure = fmt.Sprintf("child %v of %v starts away from %d", span{c.Kind, c.Start, c.End}, span{n.Kind, n.Start, n.End}, at)

				return
			}

			at = c.End
		}

		if at != n.End {
			failure = fmt.Sprintf("children of %v end at %d", span{n.Kind, n.Start, n.End}, at)
		}
	})

	if failure != "" {
		t.Fatalf("%s in %q", failure, message)
	}

	var spelled strings.Builder

	for _, l := range leaves(root) {
		spelled.WriteString(message[l.start:l.end])
	}

	if spelled.String() != message {
		t.Fatalf("leaves spell %q, not %q", spelled.String(), message)
	}
}

func TestCSTInterchangeExample(t *testing.T) {
	got := string(curly.CST(`\{world`).AppendJSON(nil))
	want := `{"type":"message","start":0,"end":7,"nodes":[` +
		`{"type":"escape","start":0,"end":2,"cancels":true},` +
		`{"type":"text","start":2,"end":7}]}`

	if got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}

func TestCSTPlaceholderInsideAnother(t *testing.T) {
	message := `{{count:gt; 0:{{count:number;}}; default:no;}}`
	tree := curly.CST(message)
	checkProperties(t, message, tree)

	placeholders := ofKind(tree, curly.KindPlaceholder)
	sameSpans(t, "placeholders", spans(placeholders), []span{
		{curly.KindPlaceholder, 0, 46},
		{curly.KindPlaceholder, 14, 31},
	})

	outer := placeholders[0]

	var optionKeys []curly.Node

	for _, c := range outer.Nodes {
		if c.Kind == curly.KindOptionKey {
			optionKeys = append(optionKeys, c)
		}
	}

	if len(optionKeys) != 3 || optionKeys[0].Name != "0" || optionKeys[0].Start != 12 || optionKeys[0].End != 13 {
		t.Fatalf("option keys %v", spans(optionKeys))
	}

	if names := []string{optionKeys[1].Name, optionKeys[2].Name}; fmt.Sprint(names) != "[default ]" {
		t.Errorf("option key names %q", names)
	}

	value := ofKind(outer, curly.KindOptionValue)[0]
	sameSpans(t, "first value", spans([]curly.Node{value}), []span{{curly.KindOptionValue, 14, 31}})
	sameSpans(t, "its children", spans(value.Nodes), []span{{curly.KindPlaceholder, 14, 31}})

	if key := ofKind(outer, curly.KindKey)[0]; key.Name != "count" {
		t.Errorf("key %q", key.Name)
	}

	if modifier := ofKind(outer, curly.KindModifier)[0]; modifier.Name != "gt" {
		t.Errorf("modifier %q", modifier.Name)
	}
}

func TestCSTBraceABackslashConsumed(t *testing.T) {
	for _, c := range []struct {
		message string
		leaves  []span
	}{
		{`\{{v}}`, []span{{curly.KindEscape, 0, 2}, {curly.KindText, 2, 6}}},
		{`{{v\}}`, []span{{curly.KindText, 0, 3}, {curly.KindEscape, 3, 5}, {curly.KindText, 5, 6}}},
	} {
		tree := curly.CST(c.message)
		checkProperties(t, c.message, tree)
		sameSpans(t, c.message, leaves(tree), c.leaves)
	}

	message := `{{v\}}}`
	tree := curly.CST(message)
	checkProperties(t, message, tree)
	sameSpans(t, message, spans(ofKind(tree, curly.KindPlaceholder)), []span{{curly.KindPlaceholder, 0, 7}})

	if key := ofKind(tree, curly.KindKey)[0]; key.Name != "v}" {
		t.Errorf("key %q", key.Name)
	}
}

func TestCSTColonThatIsContent(t *testing.T) {
	message := `{{a:b:c; d:e:f;}}`
	tree := curly.CST(message)
	checkProperties(t, message, tree)

	if key := ofKind(tree, curly.KindKey)[0]; key.Name != "a" {
		t.Errorf("key %q", key.Name)
	}

	if modifier := ofKind(tree, curly.KindModifier)[0]; modifier.Name != "b:c" {
		t.Errorf("modifier %q", modifier.Name)
	}

	optionKeys := ofKind(tree, curly.KindOptionKey)

	if len(optionKeys) != 2 || optionKeys[0].Name != "d" || optionKeys[1].Name != "" {
		t.Fatalf("option keys %v", spans(optionKeys))
	}

	value := ofKind(tree, curly.KindOptionValue)[0]

	if got := message[value.Start:value.End]; got != "e:f" {
		t.Errorf("value %q", got)
	}
}

func TestCSTPadding(t *testing.T) {
	message := `{{ v : number ;}}`
	tree := curly.CST(message)
	checkProperties(t, message, tree)

	p := ofKind(tree, curly.KindPlaceholder)[0]
	sameSpans(t, "placeholder", spans(p.Nodes), []span{
		{curly.KindOpen, 0, 2},
		{curly.KindSpace, 2, 3},
		{curly.KindKey, 3, 4},
		{curly.KindSpace, 4, 5},
		{curly.KindSeparator, 5, 6},
		{curly.KindSpace, 6, 7},
		{curly.KindModifier, 7, 13},
		{curly.KindSpace, 13, 14},
		{curly.KindSeparator, 14, 15},
		{curly.KindOptionKey, 15, 15},
		{curly.KindClose, 15, 17},
	})

	if p.Nodes[2].Name != "v" || p.Nodes[6].Name != "number" {
		t.Errorf("names %q %q", p.Nodes[2].Name, p.Nodes[6].Name)
	}
}

func TestCSTNameThatIsAllPadding(t *testing.T) {
	message := `{{  }}`
	tree := curly.CST(message)
	checkProperties(t, message, tree)

	p := ofKind(tree, curly.KindPlaceholder)[0]
	sameSpans(t, "placeholder", spans(p.Nodes), []span{
		{curly.KindOpen, 0, 2},
		{curly.KindKey, 2, 2},
		{curly.KindSpace, 2, 4},
		{curly.KindClose, 4, 6},
	})

	if p.Nodes[1].Name != "" || p.Nodes[1].Nodes == nil || len(p.Nodes[1].Nodes) != 0 {
		t.Errorf("key %+v", p.Nodes[1])
	}
}

func TestCSTLineTerminator(t *testing.T) {
	for _, message := range []string{
		"{{v\n}}", "{{v\r}}", "{{v\u2028}}", "{{v\u2029}}",
		"{{v; x:\n}}", "{{v; x:{{w\u2029}}}}",
	} {
		tree := curly.CST(message)
		checkProperties(t, message, tree)
		sameSpans(t, strconv.Quote(message), leaves(tree), []span{{curly.KindText, 0, len(message)}})
	}

	// Escaping the terminator does not make the placeholder: the escape
	// sequence is text's, and cancels, as a backslash before any blank does.
	for _, message := range []string{"{{v\\\n}}", "{{v\\\u2028}}"} {
		tree := curly.CST(message)
		checkProperties(t, message, tree)

		if p := ofKind(tree, curly.KindPlaceholder); len(p) != 0 {
			t.Errorf("%q: %v", message, spans(p))
		}

		if e := ofKind(tree, curly.KindEscape); len(e) != 1 || e[0].Start != 3 || !e[0].Cancels {
			t.Errorf("%q: %v", message, spans(e))
		}
	}

	// The near miss: a terminator outside the placeholder is text beside it.
	message := "{{v}}\n"
	tree := curly.CST(message)
	sameSpans(t, strconv.Quote(message), spans(tree.Nodes), []span{
		{curly.KindPlaceholder, 0, 5},
		{curly.KindText, 5, 6},
	})
}

func TestCSTEscapeSequences(t *testing.T) {
	for _, c := range []struct {
		message string
		end     int
		cancels bool
	}{
		{"\\\U0001F600", 5, false},
		{"\\\u00e9", 3, false},
		{"\\\u3000", 4, true},
		{"\\\ufeff", 4, true},
		{"\\\u2028", 4, true},
		{"\\ ", 2, true},
		{"\\:", 2, true},
		{"\\;", 2, true},
		{"\\{", 2, true},
		{"\\}", 2, true},
		{"\\\\", 2, true},
		{"\\n", 2, false},
		{"\\\xff", 2, false},
		{"\\\ufffd", 4, false},
	} {
		tree := curly.CST(c.message)
		checkProperties(t, c.message, tree)

		escape := tree.Nodes[0]

		if escape.Kind != curly.KindEscape || escape.End != c.end || escape.Cancels != c.cancels {
			t.Errorf("%q: %+v", c.message, escape)
		}
	}

	// A backslash with nothing after it denotes itself, so it is text.
	message := `a\`
	sameSpans(t, message, leaves(curly.CST(message)), []span{{curly.KindText, 0, 2}})
}

func TestCSTValueMoreThanAPlaceholder(t *testing.T) {
	message := `{{n:gt; 0:you have {{n}} left; default:none;}}`
	tree := curly.CST(message)
	checkProperties(t, message, tree)

	value := ofKind(tree, curly.KindOptionValue)[0]
	sameSpans(t, "value", spans([]curly.Node{value}), []span{{curly.KindOptionValue, 10, 29}})
	sameSpans(t, "its children", spans(value.Nodes), []span{
		{curly.KindText, 10, 19},
		{curly.KindPlaceholder, 19, 24},
		{curly.KindText, 24, 29},
	})
}

func TestCSTOpeningPairInAValueThatOpensNothing(t *testing.T) {
	message := `{{a; x:{{b}}`
	tree := curly.CST(message)
	checkProperties(t, message, tree)
	sameSpans(t, message, spans(tree.Nodes), []span{
		{curly.KindText, 0, 7},
		{curly.KindPlaceholder, 7, 12},
	})

	// A "{{" outside a value opens nothing and refuses the construct.
	message = `{{a{{b}}`
	sameSpans(t, message, spans(curly.CST(message).Nodes), []span{
		{curly.KindText, 0, 3},
		{curly.KindPlaceholder, 3, 8},
	})
}

func TestCSTInvalidUTF8(t *testing.T) {
	for _, message := range []string{
		"\xff", "{{\xff}}", "{{v:\xfe; \xff:\xc3}}", "\xe2\x80{{v}}", "{{v; x:\xe2\x80}}", "\\\xe2\x80\xa8",
	} {
		checkProperties(t, message, curly.CST(message))
	}

	// A truncated U+2028 is not a line terminator, so the placeholder holds.
	message := "{{v\xe2\x80}}"
	tree := curly.CST(message)

	if key := ofKind(tree, curly.KindKey); len(key) != 1 || key[0].Name != "v\xe2\x80" {
		t.Errorf("key %v", key)
	}
}

func TestCSTProperties(t *testing.T) {
	for _, message := range []string{
		"", "plain", "{{}}", "{{:}}", "{{;}}", "{{v}}", "{{ v }}", "{{v:}}", "{{v;}}",
		"{{v; :x}}", "{{v; a}}", "{{v; a: }}", "{{v; a:b; a:c}}", "{{v; default: {{w}} }}",
		"{{v\\ }}", "{{ \\ v\\  }}", "{{v\\\\ }}", "{{v\\\\\\ }}", "{{a}}{{b}}", "{{{v}}}",
		"{{v}}}}", "{{{{v}}", "}}{{", "{{v; x:a\\;b; y:{{w; z:{{u}}}}}}", "\u3000{{\u3000v\u3000}}\u3000",
		"{{v:eq; a:{{b}}c{{d}}; e:f}}", "{{v; x:{{a{{b}}}}", "{{v; x:{{w; y:{{u}}}}",
	} {
		tree := curly.CST(message)
		checkProperties(t, message, tree)

		var decoded any

		if err := json.Unmarshal(tree.AppendJSON(nil), &decoded); err != nil {
			t.Errorf("%q: %v", message, err)
		}

		marshalled, err := json.Marshal(tree)

		if err != nil || string(marshalled) != string(tree.AppendJSON(nil)) {
			t.Errorf("%q: Marshal %s, %v", message, marshalled, err)
		}
	}
}

func TestCSTNameFields(t *testing.T) {
	tree := curly.CST(`{{v; x:y}}`)
	got := string(tree.AppendJSON(nil))
	want := `{"type":"message","start":0,"end":10,"nodes":[` +
		`{"type":"placeholder","start":0,"end":10,"nodes":[` +
		`{"type":"open","start":0,"end":2},` +
		`{"type":"key","start":2,"end":3,"name":"v","nodes":[{"type":"text","start":2,"end":3}]},` +
		`{"type":"separator","start":3,"end":4},` +
		`{"type":"space","start":4,"end":5},` +
		`{"type":"option-key","start":5,"end":6,"name":"x","nodes":[{"type":"text","start":5,"end":6}]},` +
		`{"type":"separator","start":6,"end":7},` +
		`{"type":"option-value","start":7,"end":8,"nodes":[{"type":"text","start":7,"end":8}]},` +
		`{"type":"close","start":8,"end":10}]}]}`

	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// deep is a message nesting n placeholders, each in the one option value of
// the placeholder around it.
func deep(n int) string {
	return strings.Repeat("{{a;x:", n) + "v" + strings.Repeat("}}", n)
}

// deepJSON is the encoding of deep(n), spelled out level by level.
func deepJSON(n int) string {
	length := 8*n + 1
	at := func(kind string, start, end int) string {
		return `{"type":"` + kind + `","start":` + strconv.Itoa(start) + `,"end":` + strconv.Itoa(end) + `}`
	}
	name := func(kind, name string, start int) string {
		return `{"type":"` + kind + `","start":` + strconv.Itoa(start) + `,"end":` + strconv.Itoa(start+1) +
			`,"name":"` + name + `","nodes":[` + at("text", start, start+1) + `]}`
	}

	var b strings.Builder

	b.WriteString(`{"type":"message","start":0,"end":` + strconv.Itoa(length) + `,"nodes":[`)

	for i := range n {
		start, end := 6*i, length-2*i
		b.WriteString(`{"type":"placeholder","start":` + strconv.Itoa(start) + `,"end":` + strconv.Itoa(end) + `,"nodes":[`)
		b.WriteString(at("open", start, start+2) + ",")
		b.WriteString(name("key", "a", start+2) + ",")
		b.WriteString(at("separator", start+3, start+4) + ",")
		b.WriteString(name("option-key", "x", start+4) + ",")
		b.WriteString(at("separator", start+5, start+6) + ",")
		b.WriteString(`{"type":"option-value","start":` + strconv.Itoa(start+6) + `,"end":` + strconv.Itoa(end-2) + `,"nodes":[`)
	}

	b.WriteString(at("text", 6*n, 6*n+1))

	for i := n - 1; i >= 0; i-- {
		end := length - 2*i
		b.WriteString(`]},` + at("close", end-2, end) + `]}`)
	}

	b.WriteString(`]}`)

	return b.String()
}

// TestCSTDeep describes and encodes a message nested a hundred thousand
// levels deep with the goroutine's stack held to a megabyte, so a walk that
// recursed per level would fail here rather than pass on Go's default.
func TestCSTDeep(t *testing.T) {
	const n = 100_000

	message := deep(n)
	want := deepJSON(n)

	defer debug.SetMaxStack(debug.SetMaxStack(1 << 20))

	tree := curly.CST(message)
	got := tree.AppendJSON(nil)

	if string(got) != want {
		t.Fatalf("encoding differs: %d bytes, want %d", len(got), len(want))
	}

	if count := len(ofKind(tree, curly.KindPlaceholder)); count != n {
		t.Errorf("%d placeholders", count)
	}

	checkProperties(t, message, tree)

	// The near miss: one closing pair short, so the outermost level is text
	// and every level inside it still derives.
	short := curly.CST(message[:len(message)-2])
	sameSpans(t, "one pair short", spans(short.Nodes), []span{
		{curly.KindText, 0, 6},
		{curly.KindPlaceholder, 6, len(message) - 2},
	})
}

func TestCSTMarshalJSONDepth(t *testing.T) {
	// encoding/json refuses what a Marshaler writes past 10000 levels of
	// nesting; AppendJSON is the encoding at any depth.
	tree := curly.CST(deep(6000))

	if _, err := json.Marshal(tree); err == nil {
		t.Errorf("json.Marshal took a tree nested past its limit")
	}

	if string(tree.AppendJSON(nil)) != deepJSON(6000) {
		t.Errorf("AppendJSON differs")
	}

	shallow := curly.CST(deep(100))
	marshalled, err := json.Marshal(shallow)

	if err != nil || string(marshalled) != deepJSON(100) {
		t.Errorf("json.Marshal of a shallow tree: %v", err)
	}
}

// encoding/json escapes HTML in what a Marshaler writes, so json.Marshal
// writes a name holding < > & escaped, where an Encoder with HTML escaping
// off writes AppendJSON's bytes.
func TestCSTMarshalJSONEscapesHTML(t *testing.T) {
	tree := curly.CST("{{a&b}}{{<x>}}")
	want := string(tree.AppendJSON(nil))

	var out strings.Builder

	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(tree); err != nil || out.String() != want+"\n" {
		t.Errorf("an Encoder without HTML escaping: %v\n%s", err, out.String())
	}

	marshalled, err := json.Marshal(tree)

	if err != nil || string(marshalled) == want || !strings.Contains(string(marshalled), `"a\u0026b"`) {
		t.Errorf("json.Marshal: %v\n%s", err, marshalled)
	}

	var a, b any

	if json.Unmarshal(marshalled, &a) != nil || json.Unmarshal([]byte(want), &b) != nil || fmt.Sprint(a) != fmt.Sprint(b) {
		t.Errorf("json.Marshal writes other JSON:\n%s\n%s", marshalled, want)
	}
}

func TestCSTAppendsToDst(t *testing.T) {
	got := string(curly.CST("x").AppendJSON([]byte("prefix:")))

	if got != `prefix:{"type":"message","start":0,"end":1,"nodes":[{"type":"text","start":0,"end":1}]}` {
		t.Errorf("got %s", got)
	}
}

func FuzzCST(f *testing.F) {
	for _, seed := range []string{
		"", "{{v}}", `{{count:gt; 0:{{count:number;}}; default:no;}}`, `{{a; x:{{b}}`,
		"{{v\\}}}", "{{ v : number ;}}", "{{v\n}}", "\\\U0001F600", "{{v\xe2\x80}}", deep(50),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, message string) {
		tree := curly.CST(message)
		checkProperties(t, message, tree)

		if !json.Valid(tree.AppendJSON(nil)) {
			t.Fatalf("invalid JSON for %q", message)
		}
	})
}
