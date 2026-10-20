package curly

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func convertOne(v any) (string, bool) {
	w := &walk{p: New(Options{})}

	return w.convert(v, noSite)
}

// Section 3.3 of the design takes ECMAScript's Number::toString as the
// ordinary conversion of a float; the vectors are Node's own answers.
func TestFloatVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/floats.json")

	if err != nil {
		t.Fatal(err)
	}

	var vectors struct {
		Float64 [][2]string
		Float32 [][2]string
	}

	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}

	if len(vectors.Float64) < 100 || len(vectors.Float32) < 50 {
		t.Fatalf("only %d and %d vectors", len(vectors.Float64), len(vectors.Float32))
	}

	bits := func(s string) uint64 {
		b, err := hex.DecodeString(s)

		if err != nil {
			t.Fatal(err)
		}

		var n uint64

		for _, c := range b {
			n = n<<8 | uint64(c)
		}

		return n
	}

	for _, v := range vectors.Float64 {
		x := math.Float64frombits(bits(v[0]))

		if got := string(appendFloat(nil, x, 64)); got != v[1] {
			t.Errorf("float64 %s (%g): got %q, want %q", v[0], x, got, v[1])
		}
	}

	for _, v := range vectors.Float32 {
		x := math.Float32frombits(uint32(bits(v[0])))

		if got, _ := convertOne(x); got != v[1] {
			t.Errorf("float32 %s (%g): got %q, want %q", v[0], x, got, v[1])
		}
	}
}

type hostChain []hostChain

// A value handed to encoding/json is measured first, and one nested deeper
// than hostDepth is never handed over: encoding/json recurses on the
// goroutine's stack, and 16 MB holds what it needs at hostDepth.
func TestHostDepth(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(16 << 20))

	build := func(levels int) hostChain {
		c := hostChain{}

		for range levels - 1 {
			c = hostChain{c}
		}

		return c
	}

	text, ok := convertOne([]any{build(hostDepth)})

	if want := "[" + strings.Repeat("[", hostDepth) + strings.Repeat("]", hostDepth) + "]"; !ok || text != want {
		t.Errorf("at hostDepth: got %t, %d bytes", ok, len(text))
	}

	if _, ok := convertOne([]any{build(hostDepth + 1)}); ok {
		t.Error("past hostDepth: described")
	}
}

type dag []any

// A value of shared references visits each one again at every reference,
// so twenty-five levels visit 2^26 - 1 nodes; each writer stops at the
// budget rather than past it.
func TestConversionStopsAtItsBudget(t *testing.T) {
	var plain any = "leaf"

	var named any = "leaf"

	for range 25 {
		plain = []any{plain, plain}
		named = dag{named, named}
	}

	type holder struct{ D any }

	for what, run := range map[string]func(c *conversion) bool{
		"json":    func(c *conversion) bool { return c.json(reflect.ValueOf(plain)) },
		"fmt":     func(c *conversion) bool { return c.print(named) },
		"prewalk": func(c *conversion) bool { return c.json(reflect.ValueOf([]any{holder{named}})) },
	} {
		c := &conversion{w: &walk{p: New(Options{})}}

		if run(c) || c.nodes > MaxConversionNodes+1 {
			t.Errorf("%s: visited %d nodes", what, c.nodes)
		}
	}
}

// negativeZero is what the parser writes where encoding/json writes -0.
func negativeZero(v any) any {
	switch x := v.(type) {
	case float64:
		if x == 0 {
			return 0.0
		}
	case []any:
		for i := range x {
			x[i] = negativeZero(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = negativeZero(x[k])
		}
	}

	return v
}

// The JSON writer writes what encoding/json writes with HTML escaping off,
// for every plain value, but for the form of a byte that is not UTF-8.
func FuzzJSONWriter(f *testing.F) {
	for _, seed := range []string{`{"a":[1,"x",null,true,{"b":-0.5e-7}]}`, `[1e21,1e-7,123456789,0.1]`, `{"\u2028<&>":"\u0000\""}`, `[]`, `{}`} {
		f.Add(seed, "a")
		f.Add(seed, "\xff\xfe a")
	}

	f.Fuzz(func(t *testing.T, data string, text string) {
		var decoded any

		if json.Unmarshal([]byte(data), &decoded) != nil {
			return
		}

		v := []any{negativeZero(decoded), text, map[string]any{text: text}}
		got, ok := convertOne(v)

		var want bytes.Buffer

		enc := json.NewEncoder(&want)
		enc.SetEscapeHTML(false)

		if err := enc.Encode(v); err != nil {
			t.Fatal(err)
		}

		if !ok {
			t.Fatalf("undescribed, where encoding/json wrote %q", want.String())
		}

		wrote := strings.TrimSuffix(want.String(), "\n")

		// The parser writes a byte that starts no valid sequence as the
		// escape encoding/json wrote up to Go 1.26; from 1.27 encoding/json
		// writes the character itself. Both say the same text, and nothing
		// else differs.
		if !utf8.ValidString(text) {
			got = strings.ReplaceAll(got, `\ufffd`, "\ufffd")
			wrote = strings.ReplaceAll(wrote, `\ufffd`, "\ufffd")
		}

		if got != wrote {
			t.Errorf("got  %q\nwant %q", got, wrote)
		}
	})
}

type (
	namedMap   map[string]any
	intMap     map[int]any
	floatMap   map[float64]any
	boolMap    map[bool]any
	structMap  map[pair]any
	ifaceMap   map[any]any
	pair       struct{ A, B any }
	namedSlice []any
	array      [2]any
	pointerBox struct{ P *pair }
)

// varied rebuilds a decoded JSON value as Go types fmt prints in each of its
// ways, picking one by the bytes of choice in turn. None has a method.
func varied(v any, choice *[]byte) any {
	pick := func() byte {
		if len(*choice) == 0 {
			return 0
		}

		b := (*choice)[0]
		*choice = (*choice)[1:]

		return b
	}

	switch x := v.(type) {
	case map[string]any:
		// The choices are read in key order, so one input builds one value.
		keys := slices.Sorted(maps.Keys(x))
		sorted := func(yield func(string, any) bool) {
			for _, k := range keys {
				if !yield(k, x[k]) {
					return
				}
			}
		}

		switch pick() % 7 {
		case 0:
			m := namedMap{}

			for k, e := range sorted {
				m[k] = varied(e, choice)
			}

			return m
		case 1:
			m := intMap{}

			for k, e := range sorted {
				m[len(k)-3] = varied(e, choice)
			}

			return m
		case 2:
			m := floatMap{}

			for k, e := range sorted {
				key := float64(len(k)) / 2

				// One NaN key at most: fmt orders keys that compare equal
				// as the map happens to range over them.
				if k == "" {
					key = math.NaN()
				}

				m[key] = varied(e, choice)
			}

			return m
		case 3:
			m := ifaceMap{}

			for k, e := range sorted {
				if len(k)%2 == 0 {
					m[len(k)] = varied(e, choice)
				} else {
					m[k] = varied(e, choice)
				}
			}

			return m
		case 4:
			m := structMap{}

			for k, e := range sorted {
				m[pair{len(k), k}] = varied(e, choice)
			}

			return m
		case 5:
			m := boolMap{}

			for k, e := range sorted {
				m[len(k)%2 == 0] = varied(e, choice)
			}

			return m
		}

		m := map[string]any{}

		for k, e := range sorted {
			m[k] = varied(e, choice)
		}

		return m
	case []any:
		out := make([]any, len(x))

		for i, e := range x {
			out[i] = varied(e, choice)
		}

		switch pick() % 6 {
		case 0:
			return namedSlice(out)
		case 1:
			var a array

			copy(a[:], out)

			return a
		case 2:
			var p pair

			if len(out) > 0 {
				p.A = out[0]
			}

			if len(out) > 1 {
				p.B = out[1]
			}

			return &p
		case 3:
			return pointerBox{&pair{A: out}}
		case 4:
			return []byte(fmt.Sprint(len(out)))
		}

		return out
	case float64:
		switch pick() % 5 {
		case 0:
			return float32(x)
		case 1:
			return int64(x)
		case 2:
			return complex(x, -x)
		case 3:
			return uint8(x)
		}
	case string:
		if pick()%3 == 0 {
			return []string{x, x}
		}
	case nil:
		switch pick() % 4 {
		case 0:
			return (*pair)(nil)
		case 1:
			return namedMap(nil)
		case 2:
			return []any(nil)
		}
	}

	return v
}

// The %v writer writes what fmt.Sprint writes, for every value without
// methods and without cycles.
func FuzzFmtWriter(f *testing.F) {
	for _, seed := range []string{`{"a":[1,"x",null,true,{"bb":0.5}],"ccc":{}}`, `[1e21,1e-7,[null,[]],0.1]`, `{"a":1,"bb":2,"ccc":3,"dddd":4}`, `[[],[[]],null]`} {
		f.Add(seed, []byte{0, 1, 2, 3, 4, 5, 6, 7})
		f.Add(seed, []byte{6, 5, 4, 3, 2, 1, 0})
	}

	f.Fuzz(func(t *testing.T, data string, choice []byte) {
		var decoded any

		if json.Unmarshal([]byte(data), &decoded) != nil {
			return
		}

		v := struct{ V any }{varied(decoded, &choice)}
		got, ok := convertOne(v)

		if want := fmt.Sprint(v); !ok || got != want {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})
}

// A placeholder at the top level of a message resolves as it does alone,
// and the text around the placeholders the tree reads is the message's own:
// the tree and the resolution read the same placeholders.
func FuzzResolve(f *testing.F) {
	for _, seed := range []string{
		"Hello, {{name; default:Guest;}}!", "{{n; 1:one; default:{{n}} many}}", "\\{{a}} {{a\\;b}} {{ {{a}} }}",
		"{{v:lt; 10:A; NaN:N; 5:B}}", "{{a{{b}}}}", "{{a\n}}{{b}}", "\\", "{{{{a}}}}", "{{a:nope}}{{:eq}}{{default}}",
	} {
		f.Add(seed, `{"name":"Alice","n":"1","v":3,"a":{"value":"x","default":"y"},"b":[1,{"c":null}],"default":"D"}`)
	}

	f.Fuzz(func(t *testing.T, message, data string) {
		var payload map[string]any

		_ = json.Unmarshal([]byte(data), &payload)

		p := New(Options{OnReport: func(Report) {}})
		context := Context{Payload: payload}
		whole := p.Resolve(message, context)

		if len(message)+len(data) > 10000 {
			return
		}

		var pieces strings.Builder

		for _, n := range CST(message).Nodes {
			if n.Kind == KindPlaceholder {
				pieces.WriteString(p.Resolve(message[n.Start:n.End], context))
			} else {
				pieces.WriteString(unesc(message[n.Start:n.End]))
			}
		}

		if whole != pieces.String() {
			t.Errorf("%q: whole %q, by the tree %q", message, whole, pieces.String())
		}
	})
}

type wideKey [50000]int32

func (wideKey) String() string { return "k" }

type sharedMaps []map[wideKey]int

// Sorting a map's keys reads every value inside them, which the conversion
// is charged for even where a method prints each key in a byte, so a map
// reached through any number of references is sorted within one budget.
func TestMapKeysSortWithinTheBudget(t *testing.T) {
	m := map[wideKey]int{{}: 1}

	steps := func(n int) int {
		refs := make(sharedMaps, n)

		for i := range refs {
			refs[i] = m
		}

		c := &conversion{w: &walk{p: New(Options{})}}

		if c.print(refs) {
			t.Errorf("%d references: described", n)
		}

		return c.keys.steps
	}

	if small, large := steps(10), steps(20); large > small || large > 2*MaxConversionNodes {
		t.Errorf("%d steps sorting the keys of 10 references, %d of 20", small, large)
	}
}

// A map's keys are visited once, though the sort reads them before they
// print: the conversion visits the map, each key with its two elements and
// each value, whatever reads them first.
func TestMapKeysAreVisitedOnce(t *testing.T) {
	keyed := func(n int) map[[2]int]int {
		m := make(map[[2]int]int, n)

		for i := range n {
			m[[2]int{i, -i}] = i
		}

		return m
	}

	for _, n := range []int{(MaxConversionNodes - 1) / 4, (MaxConversionNodes-1)/4 + 1} {
		m := keyed(n)
		c := &conversion{w: &walk{p: New(Options{})}}
		ok := c.print(m)

		if within := 1+4*n <= MaxConversionNodes; ok != within || within && (c.nodes != 1+4*n || string(c.buf) != fmt.Sprint(m)) {
			t.Errorf("%d entries: described %v after %d nodes", n, ok, c.nodes)
		}
	}
}
