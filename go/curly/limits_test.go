package curly_test

import (
	"fmt"
	"math"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/curly-message/parsers/go/curly"
)

func expect(t *testing.T, what string, got resolved, out string, codes ...curly.Code) {
	t.Helper()

	if got.out != out || fmt.Sprint(got.codes) != fmt.Sprint(codes) {
		t.Errorf("%s: got %.40q (%d bytes) %v, want %.40q (%d bytes) %v", what, got.out, len(got.out), got.codes, out, len(out), codes)
	}
}

// The constructions of the conformance set's generated cases, at the limits
// the package declares.
func TestLimitsOutput(t *testing.T) {
	at := strings.Repeat("x", curly.MaxOutputLength)
	expect(t, "at", resolve("{{v; a:"+at+";}}", map[string]any{"v": "a"}), at)

	over := "{{v; a:" + at + "x;}}"
	got, reports := resolveWith(curly.Options{}, "A"+over+"B", map[string]any{"v": "a"})
	expect(t, "over", got, "AB", curly.OutputLimit)

	if reports[0].Limit != curly.MaxOutputLength || reports[0].Origin != curly.OriginLimit {
		t.Errorf("over: got %+v", reports[0])
	}

	expect(t, "continues", resolve(over+"{{w}}", map[string]any{"v": "a", "w": "tail"}), "tail", curly.OutputLimit)

	// A result nested in another reaches the output through it, and is
	// charged once, there.
	half := strings.Repeat("x", curly.MaxOutputLength/2)
	expect(t, "nested", resolve("{{v; a:{{v; a:"+half+"}}"+half+"}}", map[string]any{"v": "a"}), half+half)

	// The message's own text is the caller's.
	expect(t, "own text", resolve(at+"x{{v}}", map[string]any{"v": "a"}), at+"xa")
}

func TestLimitsRead(t *testing.T) {
	const message = "{{v:eq; nomatch:X; default:ok;}}"

	at := strings.Repeat("x", curly.MaxReadLength)
	expect(t, "at", resolve(message+"{{w}}", map[string]any{"v": at, "w": "tail"}), "oktail")

	got, reports := resolveWith(curly.Options{}, message+"{{w}}", map[string]any{"v": at + "x", "w": "tail"})
	expect(t, "over", got, "ok", curly.ReadLimit)

	if reports[0].Limit != curly.MaxReadLength || reports[0].Text != "{{w}}" {
		t.Errorf("over: got %+v", reports[0])
	}

	// A link of the chain is read too.
	expect(t, "a link", resolve("{{v}}{{w}}", map[string]any{"default": at + "x", "w": "tail"}), "", curly.OutputLimit, curly.ReadLimit)
}

// tree is a value of shared references visiting at least n nodes: each
// level an array naming the level below twice.
func tree(n int) any {
	var below any = "leaf"

	for visits := 1; visits < n; visits = 2*visits + 1 {
		below = []any{below, below}
	}

	return below
}

func TestLimitsConversion(t *testing.T) {
	expect(t, "over", resolve("{{v; default:D}}", map[string]any{"v": tree(curly.MaxConversionNodes + 1)}), "D", curly.UnserializableValue)

	// The root and its elements are the nodes a flat array visits.
	flat := make([]any, curly.MaxConversionNodes-1)

	for i := range flat {
		flat[i] = 0
	}

	expect(t, "at", resolve("{{v; x:X; default:D}}", map[string]any{"v": flat}), "D")
	expect(t, "one over", resolve("{{v; x:X; default:D}}", map[string]any{"v": append(flat, 0)}), "D", curly.UnserializableValue)

	// Twenty-five levels of a doubling tree visit 2^26 - 1 nodes, which the
	// budget stops at long before.
	expect(t, "a deep tree of shared references", resolve("{{v; default:D}}", map[string]any{"v": tree(1 << 26)}), "D", curly.UnserializableValue)

	// The elements of a map's keys are counted however many an int can
	// count: a key of zero-size arrays holds more than that.
	type huge struct {
		Z1, Z2 [math.MaxInt]struct{}
		A      int
	}

	expect(t, "keys holding more elements than an int counts", resolve("{{v; default:D}}", map[string]any{"v": map[huge]int{{A: 1}: 1, {A: 2}: 2}}), "D", curly.UnserializableValue)

	// Members below a value with no JSON form of its own are counted too.
	type wide struct{ Items []int }

	expect(t, "counted below a struct", resolve("{{v; default:D}}", map[string]any{"v": []any{wide{make([]int, curly.MaxConversionNodes)}}}), "D", curly.UnserializableValue)

	// Bytes are one node to encoding/json, which writes them in base64,
	// unless their element type marshals itself: each is then a call.
	type bytes struct{ Items []byte }

	type octets struct{ Items []octet }

	type texts struct{ Items []textOctet }

	expect(t, "near miss: bytes below a struct", resolve("{{v; x:X; default:D}}", map[string]any{"v": []any{bytes{make([]byte, curly.MaxConversionNodes)}}}), "D")

	octetCalls = 0

	expect(t, "byte-kind elements that marshal", resolve("{{v; default:D}}", map[string]any{"v": []any{octets{make([]octet, curly.MaxConversionNodes)}}}), "D", curly.UnserializableValue)
	expect(t, "byte-kind elements whose pointer marshals", resolve("{{v; default:D}}", map[string]any{"v": []any{texts{make([]textOctet, curly.MaxConversionNodes)}}}), "D", curly.UnserializableValue)
	expect(t, "a few of them", resolve("{{v}}", map[string]any{"v": []any{octets{make([]octet, 2)}}}), `[{"Items":[0,0]}]`)

	if octetCalls != 2 {
		t.Errorf("MarshalJSON called %d times", octetCalls)
	}
}

// large is a value whose String method writes n bytes.
type large int

func (n large) String() string { return strings.Repeat("s", int(n)) }

type named []any

// A conversion writes at most MaxSerializedLength bytes, as JSON or as fmt
// prints a value, however few nodes it visits: a value whose text would be
// longer is one no conversion describes.
func TestLimitsSerializedLength(t *testing.T) {
	const message = "{{v; x:X; default:D}}"

	// A plain array of one string writes it between two quotes and two
	// brackets.
	at := strings.Repeat("x", curly.MaxSerializedLength-4)
	expect(t, "JSON at", resolve(message, map[string]any{"v": []any{at}}), "D")
	expect(t, "JSON over", resolve(message, map[string]any{"v": []any{at + "x"}}), "D", curly.UnserializableValue)

	// Shared references write their text again at each one.
	ref := strings.Repeat("x", curly.MaxSerializedLength/10)
	expect(t, "JSON over by references", resolve(message, map[string]any{"v": []any{ref, ref, ref, ref, ref, ref, ref, ref, ref, ref}}), "D", curly.UnserializableValue)

	// fmt writes a named slice of one string between two brackets.
	expect(t, "fmt at", resolve(message, map[string]any{"v": named{at + "xx"}}), "D")
	expect(t, "fmt over", resolve(message, map[string]any{"v": named{at + "xxx"}}), "D", curly.UnserializableValue)
	expect(t, "a method's text at", resolve(message, map[string]any{"v": large(curly.MaxSerializedLength)}), "D")
	expect(t, "a method's text over", resolve(message, map[string]any{"v": large(curly.MaxSerializedLength + 1)}), "D", curly.UnserializableValue)

	// A value no conversion describes spends none of the read budget.
	expect(t, "a refused value reads nothing", resolve("{{v; default:D}}{{w}}", map[string]any{"v": []any{at + "x"}, "w": "tail"}), "Dtail", curly.UnserializableValue)
}

// sharedNamed is a named slice of levels levels, each naming the level below
// twice, over an empty one: fmt prints 5 * 2^levels - 3 bytes of it.
func sharedNamed(levels int) any {
	var v any = named{}

	for range levels {
		v = named{v, v}
	}

	return v
}

// A value fmt prints is the host's ordinary conversion, not a serialization:
// shared references are walked again at each one, past the nodes a
// serialization may visit, and such a value is refused once its text is
// longer than MaxSerializedLength.
func TestLimitsPrintedValues(t *testing.T) {
	for _, levels := range []int{16, 20} {
		expect(t, fmt.Sprint(levels, " levels past the output"), resolve("{{v; default:D}}", map[string]any{"v": sharedNamed(levels)}), "", curly.OutputLimit)
	}

	expect(t, "near miss: 21 levels write more than MaxSerializedLength", resolve("{{v; default:D}}", map[string]any{"v": sharedNamed(21)}), "D", curly.UnserializableValue)
	expect(t, "near miss: a plain array of shared references is serialized", resolve("{{v; default:D}}", map[string]any{"v": tree(1 << 17)}), "D", curly.UnserializableValue)
}

// Sorting a map's keys spends none of what the conversion may visit: a map
// is described as far as its text, beside any other value, however many
// pairs its sort compares. The keys are counted apart, each value and each
// string's byte in them, which keys printed as fmt prints them reach only
// past the text bound: keys a String method prints in less, a million
// elements or a long string each, are refused at the count.
func TestLimitsPrintedMaps(t *testing.T) {
	m := make(map[[256]int]int, 10000)

	for i := range 10000 {
		m[[256]int{i}] = i
	}

	v := map[string]any{"v": m}

	expect(t, "10 000 keys past the output", resolve("{{v; default:D}}", v), "", curly.OutputLimit)

	numbers := make(map[int]int, 1000000)

	for i := range 1000000 {
		numbers[i] = 0
	}

	v = map[string]any{"v": numbers}

	expect(t, "1 000 000 numbers past the output", resolve("{{v; default:D}}", v), "", curly.OutputLimit)

	pairs := make(map[[2]int]int, 300000)

	for i := range 300000 {
		pairs[[2]int{0, i}] = 0
	}

	empty := make(named, 3000000)

	for i := range empty {
		empty[i] = ""
	}

	v = map[string]any{"v": named{pairs, empty}}

	expect(t, "a sorted map beside 3 000 000 values past the output", resolve("{{v; default:D}}", v), "", curly.OutputLimit)

	expect(t, "19 keys of a million elements", resolve("{{v; default:D}}", millionKeys(19)), fmt.Sprint(millionKeys(19)["v"]))
	expect(t, "near miss: 20 keys of a million elements", resolve("{{v; default:D}}", millionKeys(20)), "D", curly.UnserializableValue)

	// Each key counts the struct, its field and the string's bytes.
	const n = curly.MaxSerializedLength - 1
	long := strings.Repeat("s", n) + "tt"
	expect(t, "two long keys at the count", resolve("{{v; default:D}}", map[string]any{"v": map[quiet]int{{long[:n]}: 0, {long[1 : n+1]}: 1}}), "map[:0 :1]")
	expect(t, "near miss: three long keys", resolve("{{v; default:D}}", map[string]any{"v": map[quiet]int{{long[:n]}: 0, {long[1 : n+1]}: 1, {long[2:]}: 2}}), "D", curly.UnserializableValue)
}

// quiet is a key a String method prints in no bytes, however long its
// string.
type quiet struct{ S string }

func (quiet) String() string { return "" }

// millionKey is the README's K: comparing two of it reads every element
// before the field that tells them apart.
type millionKey struct {
	A [1000000]struct{}
	I int
}

func (millionKey) String() string { return "" }

func millionKeys(n int) map[string]any {
	m := make(map[millionKey]int, n)

	for i := range n {
		m[millionKey{I: i}] = i
	}

	return map[string]any{"v": m}
}

var octetCalls int

type octet uint8

func (o octet) MarshalJSON() ([]byte, error) {
	octetCalls++

	return []byte{'0' + byte(o)}, nil
}

type textOctet uint8

func (o *textOctet) MarshalText() ([]byte, error) {
	octetCalls++

	return []byte{'0' + byte(*o)}, nil
}

func nested(levels int, innermost string) string {
	message := innermost

	for range levels - 1 {
		message = "{{v; a:" + message + "}}"
	}

	return message
}

func TestLimitsNesting(t *testing.T) {
	const innermost = "{{v; a:settled; default:fallback;}}"

	payload := map[string]any{"v": "a"}
	expect(t, "at", resolve(nested(curly.MaxNesting, innermost), payload), "settled")

	got, reports := resolveWith(curly.Options{}, nested(curly.MaxNesting+1, innermost), payload)
	expect(t, "over", got, "fallback", curly.NestingLimit)

	if reports[0].Limit != curly.MaxNesting || reports[0].Text != innermost {
		t.Errorf("over: got %+v", reports[0])
	}

	// A refused placeholder reads its value and its links all the same.
	expect(t, "a refused placeholder reads", resolve(nested(curly.MaxNesting+1, "{{w; default:{{x; default:I}}}}"), map[string]any{"v": "a", "w": cyclic(), "default": "P"}), "P",
		curly.UnserializableValue, curly.NestingLimit)
	expect(t, "a refused wrapper", resolve(nested(curly.MaxNesting+1, "{{w; default:I}}"), map[string]any{"v": "a", "w": map[string]any{"value": "V", "default": "W"}}), "W", curly.NestingLimit)
	expect(t, "a refused default placeholder", resolve(nested(curly.MaxNesting+1, "{{default; default:I}}"), map[string]any{"v": "a", "default": cyclic()}), "I",
		curly.UnserializableValue, curly.NestingLimit)
	expect(t, "a refused placeholder past the read limit", resolve("{{r:eq; x:X; default:}}"+nested(curly.MaxNesting+1, "{{w; default:I}}"), map[string]any{"v": "a", "r": strings.Repeat("r", curly.MaxReadLength+1)}), "",
		curly.ReadLimit)
}

// A message of inline defaults nested 100 000 deep resolves on a stack of
// its own: under a goroutine stack of 1 MB a walk that recursed per level
// would overflow, which is a fatal error rather than a failing test.
func TestLimitsNestingDeep(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(1 << 20))

	for _, n := range []int{1000, 100000} {
		message := strings.Repeat("{{a; default:", n) + "deep" + strings.Repeat("}}", n)
		got, reports := resolveWith(curly.Options{}, message, nil)

		if got.out != "deep" || len(reports) != n-curly.MaxNesting {
			t.Errorf("%d levels: got %q and %d reports, want %d", n, got.out, len(reports), n-curly.MaxNesting)
		}

		for _, r := range reports {
			if r.Code != curly.NestingLimit {
				t.Fatalf("%d levels: got %+v", n, r)
			}
		}
	}
}

type chain []chain

func deepSlice(levels int) any {
	var v any = []any{}

	for range levels - 1 {
		v = []any{v}
	}

	return v
}

func deepChain(levels int) chain {
	c := chain{}

	for range levels - 1 {
		c = chain{c}
	}

	return c
}

// A plain value is described at any depth the conversion budget allows, and
// so is a value printed as fmt prints it, each on a stack of its own.
func TestLimitsDeepValues(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(1 << 20))

	brackets := func(n int) string { return strings.Repeat("[", n) + strings.Repeat("]", n) }

	expect(t, "JSON", resolve("{{v}}", map[string]any{"v": deepSlice(40000)}), brackets(40000))
	expect(t, "fmt", resolve("{{v}}", map[string]any{"v": deepChain(40000)}), brackets(40000))

	// Deeper than the output can carry, so only the selection shows the
	// value was described.
	expect(t, "JSON at 99 999", resolve("{{v; x:X; default:D}}", map[string]any{"v": deepSlice(99999)}), "D")
	expect(t, "fmt at 99 999", resolve("{{v; x:X; default:D}}", map[string]any{"v": deepChain(99999)}), "D")
}
