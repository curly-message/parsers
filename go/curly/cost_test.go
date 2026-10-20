package curly_test

import (
	"fmt"
	"testing"

	"github.com/curly-message/parsers/go/curly"
)

// What a resolution allocates on the shapes a catalogue writes most, held
// to a ceiling so a change that adds to it is seen. A ceiling holds on every
// Go the module declares it builds with: the compiler decides some of what
// escapes, and Go 1.26.0 to 1.26.2 allocate once more for a comparison.
func TestResolveAllocations(t *testing.T) {
	if raceEnabled {
		t.Skip("allocations are not the parser's under the race detector")
	}

	p := curly.New(curly.Options{})

	for _, c := range []struct {
		message string
		payload map[string]any
		ceiling float64
	}{
		{"Hello!", nil, 3},
		{"Hello, {{name}}!", map[string]any{"name": "Alice"}, 11},
		{"{{n}} {{n; 1:file; default:files;}}", map[string]any{"n": 5.0}, 23},
		{"{{v:lt; 10:A; 5:B; default:D}}", map[string]any{"v": 3.0}, 24},
		{"{{o}}", map[string]any{"o": map[string]any{"a": 1.0, "b": []any{"x"}}}, 25},
	} {
		context := curly.Context{Payload: c.payload}

		if n := testing.AllocsPerRun(100, func() { p.Resolve(c.message, context) }); n > c.ceiling {
			t.Errorf("%q: %v allocations, ceiling %v", c.message, n, c.ceiling)
		}
	}
}

// widened holds its zero-size elements before the field that tells two of
// it apart, so that comparing two reads them all.
type widened[Z comparable] struct {
	Z Z
	A int
}

func widenedKeys[Z comparable](boxed bool, n int) any {
	if boxed {
		m := make(map[any]int, n)

		for i := range n {
			m[widened[Z]{A: i}] = i
		}

		return m
	}

	m := make(map[widened[Z]]int, n)

	for i := range n {
		m[widened[Z]{A: i}] = i
	}

	return m
}

// A map's keys are counted before they are sorted, and the count stops at
// the node past the budget: what refusing a map costs stops growing there,
// however many nodes each key holds. A key of zero-size elements holds them
// without memory, and the budget of a value fmt prints, twice
// MaxSerializedLength, is spent within the tenth of the narrower keys here
// and within the first of the wider. A map within the budget is sorted on
// one stack, not an allocation per comparison.
func TestMapSortCostStopsAtTheBudget(t *testing.T) {
	if raceEnabled {
		t.Skip("allocations are not the parser's under the race detector")
	}

	resolve := func(v any) (string, []curly.Code, float64) {
		var out string

		var codes []curly.Code

		p := curly.New(curly.Options{OnReport: func(r curly.Report) { codes = append(codes, r.Code) }})
		allocs := testing.AllocsPerRun(1, func() {
			codes = nil
			out = p.Resolve("{{v; default:D}}", curly.Context{Payload: map[string]any{"v": v}})
		})

		return out, codes, allocs
	}

	for _, boxed := range []bool{false, true} {
		var allocs [2]float64

		for i, v := range []any{
			widenedKeys[[curly.MaxSerializedLength / 5]struct{}](boxed, 40),
			widenedKeys[[100 * curly.MaxSerializedLength]struct{}](boxed, 40),
		} {
			out, codes, a := resolve(v)

			if out != "D" || len(codes) != 1 || codes[0] != curly.UnserializableValue {
				t.Errorf("keys in interfaces %v, %d: %q %v", boxed, i, out, codes)
			}

			allocs[i] = a
		}

		if allocs[1] > allocs[0] {
			t.Errorf("keys in interfaces %v: %v allocations for the narrower keys, %v for the wider", boxed, allocs[0], allocs[1])
		}
	}

	type pair [2]int

	within := make(map[pair]int, 5000)

	for i := range 5000 {
		within[pair{i % 7, -i}] = i
	}

	out, codes, allocs := resolve(within)

	if out != fmt.Sprint(within) || codes != nil {
		t.Errorf("within the budget: %d bytes, %v", len(out), codes)
	}

	if allocs > 3*5000 {
		t.Errorf("within the budget: %v allocations", allocs)
	}
}
