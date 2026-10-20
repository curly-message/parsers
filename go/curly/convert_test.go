package curly

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"math/bits"
	"os"
	"reflect"
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

type (
	hostChain []hostChain
	hostPtr   *hostPtr
	hostMap   map[string]hostMap
	callJSON  struct{}
	callText  struct{}
	callZero  struct{}
	callKey   int
)

func (callJSON) MarshalJSON() ([]byte, error) { return []byte(`"j"`), nil }
func (callText) MarshalText() ([]byte, error) { return []byte("t"), nil }
func (callZero) IsZero() bool                 { return false }
func (callKey) MarshalText() ([]byte, error)  { return []byte("k"), nil }

// nestCall holds leaf below levels structs, so encoding/json meets it at
// depth levels+1 of the value the parser hands it.
func nestCall(levels int, leaf any) any {
	v := reflect.ValueOf(leaf)

	for range levels {
		s := reflect.New(reflect.StructOf([]reflect.StructField{{Name: "A", Type: v.Type()}})).Elem()
		s.Field(0).Set(v)
		v = s
	}

	return v.Interface()
}

// omitZeroOf is a struct of one field Z of type T holding v.
func omitZeroOf[T any](tag string, v T) any {
	s := reflect.New(reflect.StructOf([]reflect.StructField{{Name: "Z", Type: reflect.TypeFor[T](), Tag: reflect.StructTag(tag)}})).Elem()
	s.Field(0).Set(reflect.ValueOf(&v).Elem())

	return s.Interface()
}

// chainOf builds a methodless value of levels levels by nesting leaf, one
// level deep, in wrap levels-1 times.
func chainOf[T any](levels int, leaf T, wrap func(T) T) T {
	for range levels - 1 {
		leaf = wrap(leaf)
	}

	return leaf
}

// leafAt nests leaf, which spans own levels, below structs, so the value is
// levels deep.
func leafAt(leaf any, own int) func(int) any {
	return func(levels int) any { return nestCall(levels-own, leaf) }
}

// atHostDepth checks a value build nests levels deep: at hostDepth the
// parser writes it as encoding/json writes it, and a level deeper refuses it
// without handing it over.
func atHostDepth(t *testing.T, what string, build func(levels int) any) {
	t.Helper()

	v := build(hostDepth)
	want, err := json.Marshal([]any{v})

	if text, ok := convertOne([]any{v}); err != nil || !ok || text != string(want) {
		t.Errorf("%s at hostDepth: got %t %.60q, encoding/json %.60q %v", what, ok, text, want, err)
	}

	if text, ok := convertOne([]any{build(hostDepth + 1)}); ok {
		t.Errorf("%s past hostDepth: described as %.60q", what, text)
	}
}

// A value handed to encoding/json is measured first, and one nested deeper
// than hostDepth is never handed over, whatever it nests and whatever it
// holds at the bottom: encoding/json recurses on the goroutine's stack.
func TestHostDepth(t *testing.T) {
	for what, build := range map[string]func(int) any{
		"a named slice": func(levels int) any {
			return chainOf(levels, hostChain{}, func(c hostChain) hostChain { return hostChain{c} })
		},
		"a pointer chain": func(levels int) any {
			return chainOf(levels, hostPtr(nil), func(p hostPtr) hostPtr { return &p })
		},
		"a struct chain": leafAt(0, 1),
		"pointers to interfaces": func(levels int) any {
			var leaf any

			if levels%2 == 1 {
				leaf = 0
			}

			for range levels / 2 {
				p := new(any)
				*p = leaf
				leaf = p
			}

			return leaf
		},
		"a typed map": func(levels int) any {
			return chainOf(levels, hostMap{}, func(m hostMap) hostMap { return hostMap{"a": m} })
		},
		"an array chain": func(levels int) any {
			v := reflect.ValueOf(0)

			for range levels - 1 {
				a := reflect.New(reflect.ArrayOf(1, v.Type())).Elem()
				a.Index(0).Set(v)
				v = a
			}

			return v.Interface()
		},
		"a MarshalJSON leaf":                              leafAt(callJSON{}, 1),
		"a MarshalText leaf":                              leafAt(callText{}, 1),
		"a nil pointer of a Marshaler type":               leafAt((*callJSON)(nil), 1),
		"an empty map keyed by a TextMarshaler":           leafAt(map[callKey]int{}, 1),
		"a nil pointer of an IsZero type tagged omitzero": leafAt(omitZeroOf[*callZero](`json:",omitzero"`, nil), 2),
	} {
		atHostDepth(t, what, build)
	}

	// Near miss: a plain array or object is the parser's own to write, at
	// any depth.
	const deep = 100000

	if text, ok := convertOne(chainOf[any](deep, []any{}, func(v any) any { return []any{v} })); !ok || text != strings.Repeat("[", deep)+strings.Repeat("]", deep) {
		t.Errorf("a plain array %d deep: got %t, %d bytes", deep, ok, len(text))
	}

	if text, ok := convertOne(chainOf[any](deep, map[string]any{}, func(v any) any { return map[string]any{"a": v} })); !ok || text != strings.Repeat(`{"a":`, deep-1)+"{}"+strings.Repeat("}", deep-1) {
		t.Errorf("a plain object %d deep: got %t, %d bytes", deep, ok, len(text))
	}
}

type (
	hidI struct {
		X any `json:"b"`
	}
	hidO struct {
		X int `json:"a"`
		hidI
	}
	hidU struct{ Y any }
	hidP struct{ hidU }
	hidN struct {
		hidI `json:"n"`
	}
	hidD struct {
		C any `json:"-"`
		N int
	}
	eA struct{ *eB }
	eB struct {
		*eA
		X int
	}
)

// A field encoding/json writes is measured though a field of the same Go
// name further out hides it from reflect.VisibleFields: encoding/json names
// a field by its JSON name, and writes both.
func TestHostHiddenField(t *testing.T) {
	chain := func(levels int) hostChain {
		return chainOf(levels, hostChain{}, func(c hostChain) hostChain { return hostChain{c} })
	}

	atHostDepth(t, "a hidden field", func(levels int) any { return hidO{1, hidI{chain(levels - 2)}} })

	if text, _, _ := describedAsEncodingJSON(hidO{1, hidI{chain(hostDepth - 2)}}); !strings.HasPrefix(text, `[{"a":1,"b":[`) {
		t.Errorf("a hidden field: written as %.40q", text)
	}

	_, hidden := convertOne([]any{hidO{1, hidI{chain(hostDepth - 1)}}})
	_, visible := convertOne([]any{hidI{chain(hostDepth - 1)}})

	if hidden || hidden != visible {
		t.Errorf("past hostDepth: hidden field described %t, visible %t", hidden, visible)
	}

	// Near misses: a field promoted through an unexported embedded struct,
	// and an embedded struct with a name, written as a value, are measured;
	// a field tagged "-" is not, as encoding/json does not write it.
	atHostDepth(t, "a field promoted through an unexported struct", func(levels int) any { return hidP{hidU{chain(levels - 2)}} })
	atHostDepth(t, "an embedded struct with a name", func(levels int) any { return hidN{hidI{chain(levels - 3)}} })

	cycle := []any{nil}
	cycle[0] = cycle

	if text, want, ok := describedAsEncodingJSON(hidD{cycle, 1}); !ok || want != `[{"N":1}]` {
		t.Errorf("a cycle in a field tagged \"-\": got %q, encoding/json %q", text, want)
	}

	// A cycle of embedded types is expanded once.
	if text, want, ok := describedAsEncodingJSON(eA{&eB{&eA{&eB{nil, 2}}, 1}}); !ok || want != `[{"X":1}]` {
		t.Errorf("a cycle of embedded types: got %q, encoding/json %q", text, want)
	}
}

// A func, a channel or a complex number is encoding/json's to refuse, or to
// leave out where a field holding it is tagged omitzero and it is nil.
func TestHostOmitzeroLeaves(t *testing.T) {
	type leaves struct {
		F func()   `json:",omitzero"`
		C chan int `json:",omitzero"`
		N int
	}

	if text, want, ok := describedAsEncodingJSON(leaves{N: 1}); !ok || want != `[{"N":1}]` {
		t.Errorf("nil leaves tagged omitzero: got %q, encoding/json %q", text, want)
	}

	// Near misses: encoding/json refuses a func it is not told to leave
	// out, and the parser with it.
	for what, v := range map[string]any{
		"a func tagged omitzero":    leaves{F: func() {}},
		"a nil func not tagged":     struct{ F func() }{},
		"a complex number":          struct{ C complex128 }{},
		"a channel tagged omitzero": leaves{C: make(chan int)},
	} {
		if text, want, ok := describedAsEncodingJSON(v); !ok || want != "" {
			t.Errorf("%s: got %q, encoding/json %q", what, text, want)
		}
	}
}

type dag []any

// A value of shared references visits each one again at every reference,
// so twenty-five levels visit 2^26 - 1 nodes; each writer stops at its
// budget rather than past it, a value fmt prints at its text.
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
		"fmt":     func(c *conversion) bool { c.limit = printLimit; return c.print(named) },
		"prewalk": func(c *conversion) bool { return c.json(reflect.ValueOf([]any{holder{named}})) },
	} {
		c := &conversion{w: &walk{p: New(Options{})}, limit: MaxConversionNodes}

		if run(c) || c.nodes > c.limit+1 || len(c.buf) > MaxSerializedLength+len("leaf") {
			t.Errorf("%s: visited %d nodes, wrote %d bytes", what, c.nodes, len(c.buf))
		}
	}
}

// encoding/json writes a value into a buffer of its own, so the text of a
// value handed to it is measured first, from the strings, keys and bytes it
// holds: shared references to one long string are refused where the measure
// passes MaxSerializedLength, not once encoding/json has written each of
// them, and what that costs does not grow with the references.
func TestHostTextIsMeasuredFirst(t *testing.T) {
	type texts struct{ S []string }

	type keys struct{ M []map[string]int }

	type octets struct{ B [][]byte }

	long := strings.Repeat("x", 1<<20)
	shapes := map[string]struct {
		build  func(n int) any
		within int
	}{
		"strings": {func(n int) any { return texts{slices.Repeat([]string{long}, n)} }, 9},
		"keys":    {func(n int) any { return keys{slices.Repeat([]map[string]int{{long: 0}}, n)} }, 9},
		"bytes":   {func(n int) any { return octets{slices.Repeat([][]byte{[]byte(long)}, n)} }, 7},
	}

	var codes []Code

	p := New(Options{OnReport: func(r Report) { codes = append(codes, r.Code) }})

	for what, shape := range shapes {
		var allocated [2]uint64

		for i, n := range []int{16, 64} {
			v := shape.build(n)

			allocated[i] = bytesAllocated(1, func() {
				codes = nil

				if out := p.Resolve("{{v; default:D}}", Context{Payload: map[string]any{"v": []any{v}}}); out != "D" || !slices.Equal(codes, []Code{UnserializableValue}) {
					t.Errorf("%s, %d of them: %q %v", what, n, out, codes)
				}
			})
		}

		if allocated[1] > allocated[0]+1<<20 {
			t.Errorf("%s: %d bytes allocated for 16, %d for 64", what, allocated[0], allocated[1])
		}

		v := []any{shape.build(shape.within)}
		want, _ := json.Marshal(v)

		if text, ok := convertOne(v); !ok || text != string(want) {
			t.Errorf("near miss: %s, %d of them: %t, %d bytes", what, shape.within, ok, len(text))
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

// A map's keys are counted at each reference it is reached through, so a
// map reached through any number of references is sorted within one
// budget, however few bytes a method prints each key in.
func TestMapKeysCountWithinTheBudget(t *testing.T) {
	var last wideKey

	last[len(last)-1] = 1
	m := map[wideKey]int{{}: 1, last: 2}

	counted := func(n int) (bool, int) {
		refs := make(sharedMaps, n)

		for i := range refs {
			refs[i] = m
		}

		c := &conversion{w: &walk{p: New(Options{})}, limit: printLimit}
		ok := c.print(refs)

		if ok && string(c.buf) != fmt.Sprint(refs) {
			t.Errorf("%d references: %d bytes", n, len(c.buf))
		}

		return ok, c.keys.nodes
	}

	if ok, _ := counted(10); !ok {
		t.Error("10 references: refused")
	}

	if ok, small := counted(1000); ok || small != printLimit+1 {
		t.Errorf("1 000 references: described %v after %d nodes", ok, small)
	} else if _, large := counted(2000); large != small {
		t.Errorf("%d nodes counting the keys of 1 000 references, %d of 2 000", small, large)
	}
}

// zeroKey holds zero-size elements before the field that tells two keys
// apart, so that comparing two of them reads every node of both: the field
// holding them, each element, and the field after.
type zeroKey[A any] struct {
	A A
	I int
}

func (zeroKey[A]) String() string { return "" }

// A map's keys are counted apart from what the conversion visits, a node
// for each key, the field holding the elements, each element and the field
// after, and a map of one key is not counted. The sort reads at most that
// count in each of its passes: a merge sort compares at least once for each
// key but the first and at most once for each key in each pass.
func TestMapSortReadsWithinTheCount(t *testing.T) {
	for _, n := range []int{1, 2, 3, 5, 17, 300} {
		m := make(map[zeroKey[[1000]struct{}]]int, n)

		for i := range n {
			m[zeroKey[[1000]struct{}]{I: i}] = i
		}

		c := &conversion{w: &walk{p: New(Options{})}, limit: printLimit}
		count := 1003 * n

		if n == 1 {
			count = 0
		}

		if !c.print(m) || string(c.buf) != fmt.Sprint(m) || c.nodes != 1+2*n || c.keys.nodes != count {
			t.Errorf("%d keys: %d bytes after %d nodes, %d counted", n, len(c.buf), c.nodes, c.keys.nodes)
		}

		if reads := c.keys.reads; reads%1003 != 0 || reads < (n-1)*1003 || reads > bits.Len(uint(n-1))*count {
			t.Errorf("%d keys: %d reads", n, reads)
		}
	}
}

// The count stops at the node past the budget: a map of two keys whose
// nodes take the whole budget is sorted, and one whose key is a node wider
// is refused there, before any comparison.
func TestMapKeysCountStopsAtTheBudget(t *testing.T) {
	type edge = zeroKey[[printLimit/2 - 3]struct{}]

	type past = zeroKey[[printLimit/2 - 2]struct{}]

	c := &conversion{w: &walk{p: New(Options{})}, limit: printLimit}

	if !c.print(map[edge]int{{I: 0}: 0, {I: 1}: 1}) || string(c.buf) != "map[:0 :1]" || c.keys.nodes != printLimit {
		t.Errorf("at the budget: %q after %d nodes", c.buf, c.keys.nodes)
	}

	c = &conversion{w: &walk{p: New(Options{})}, limit: printLimit}

	if c.print(map[past]int{{I: 0}: 0, {I: 1}: 1}) || c.keys.nodes != printLimit+1 || c.keys.reads != 0 {
		t.Errorf("past the budget: %q after %d nodes and %d reads", c.buf, c.keys.nodes, c.keys.reads)
	}
}

// Counting and comparing keys hold a frame for each level a key nests, not
// one for each element, so what the sort allocates does not grow with the
// zero-size elements a key holds. A map of one key is neither counted nor
// sorted.
func TestMapKeysAreWalkedLevelByLevel(t *testing.T) {
	resolve := func(v any) (string, uint64) {
		var out string

		p := New(Options{})
		n := bytesAllocated(1, func() {
			out = p.Resolve("{{v; default:D}}", Context{Payload: map[string]any{"v": v}})
		})

		return out, n
	}

	narrow, a := resolve(map[zeroKey[[90000]struct{}]]int{{I: 0}: 0, {I: 1}: 1})
	wide, b := resolve(map[zeroKey[[4000000]struct{}]]int{{I: 0}: 0, {I: 1}: 1})

	if narrow != "map[:0 :1]" || wide != narrow || b > a+1<<20 {
		t.Errorf("%q in %d bytes, %q in %d", narrow, a, wide, b)
	}

	if one, n := resolve(map[zeroKey[[1 << 30]struct{}]]int{{}: 0}); one != "map[:0]" || n > 1<<20 {
		t.Errorf("one key: %q in %d bytes", one, n)
	}
}

type (
	textOf   struct{ F func() }
	appendOf struct{ F func() }
	appendB  byte
)

func (textOf) MarshalText() ([]byte, error)          { return []byte("t"), nil }
func (appendOf) AppendText(b []byte) ([]byte, error) { return append(b, 'a'), nil }
func (appendB) AppendText(b []byte) ([]byte, error)  { return append(b, 'b'), nil }

// describedAsEncodingJSON reports whether the parser writes v inside a plain
// array as encoding/json writes it there, and refuses it where encoding/json
// does.
func describedAsEncodingJSON(v any) (string, string, bool) {
	want, err := json.Marshal([]any{v})
	text, ok := convertOne([]any{v})

	return text, string(want), ok == (err == nil) && (!ok || text == string(want))
}

// A value encoding/json describes by a method of its own is the method's to
// describe, whatever it holds, a func among it: the parser hands it over and
// writes what encoding/json writes, by the methods the toolchain's
// encoding/json calls. Built on encoding/json/v2, from Go 1.27, it calls
// AppendText too, and writes a slice of a byte type that has one as an
// array; before, it calls none, refuses the func and writes the slice in
// base64.
func TestHostMethodsDescribeTheirValue(t *testing.T) {
	for what, v := range map[string]any{
		"MarshalText":                     textOf{func() {}},
		"AppendText":                      appendOf{func() {}},
		"AppendText of a byte's slice":    []appendB{1},
		"AppendText of a byte's array":    [1]appendB{1},
		"the bytes of a byte type":        []byte{1},
		"a func with no method around it": struct{ F func() }{func() {}},
	} {
		if text, want, ok := describedAsEncodingJSON(v); !ok {
			t.Errorf("%s: got %q, encoding/json %q", what, text, want)
		}
	}

	// A slice of a byte type is a plain array where encoding/json writes it
	// as one, and fmt prints it otherwise.
	want := "[1]"

	if out, _ := json.Marshal([]appendB{1}); string(out) == `["b"]` {
		want = `["b"]`
	}

	if got := New(Options{}).Resolve("{{v}}", Context{Payload: map[string]any{"v": []appendB{1}}}); got != want {
		t.Errorf("a slice of a byte type: got %q, want %q", got, want)
	}
}

type floatKey float64

// A map keyed by neither a string nor an integer is encoding/json's to
// write or refuse, as the toolchain's encoding/json does. Built on
// encoding/json/v2, from Go 1.27, it writes one keyed by a float, by a
// pointer or by an interface where each key writes a name, and an empty one
// whatever its key; before, it refuses each.
func TestHostMapKeysOfEveryKind(t *testing.T) {
	s, i := "x", 7
	ps := &s

	for what, v := range map[string]any{
		"a float key":                       map[float64]int{1.5: 1},
		"a float32 key":                     map[float32]int{2: 1},
		"a named float key":                 map[floatKey]int{2.5: 1},
		"a NaN key":                         map[float64]int{math.NaN(): 1},
		"an interface key holding a string": map[any]int{"a": 1},
		"an interface key holding an int":   map[any]int{1: 1},
		"an interface key holding a float":  map[any]int{1.5: 1},
		"a nil interface key":               map[any]int{nil: 1},
		"a pointer key to a string":         map[*string]int{&s: 1},
		"a pointer key to an int":           map[*int]int{&i: 1},
		"a pointer key to a pointer":        map[**string]int{&ps: 1},
		"a nil pointer key":                 map[*string]int{nil: 1},
		"a bool key":                        map[bool]int{true: 1},
		"an array key":                      map[[1]int]int{{1}: 1},
		"a complex key":                     map[complex128]int{1: 1},
		"an empty map keyed by a float":     map[float64]int{},
		"an empty map keyed by a bool":      map[bool]int{},
		"an empty map keyed by a chan":      map[chan int]int{},
	} {
		if text, want, ok := describedAsEncodingJSON(v); !ok {
			t.Errorf("%s: got %q, encoding/json %q", what, text, want)
		}
	}
}
