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

// Sorting a map's keys reads every node of every key, so a map whose keys
// hold more nodes than one conversion visits is refused before the sort:
// what it costs stops growing at the budget. A map within the budget is
// sorted on one stack, not an allocation per comparison.
func TestMapKeysAreChargedBeforeTheSort(t *testing.T) {
	if raceEnabled {
		t.Skip("allocations are not the parser's under the race detector")
	}

	type wide [256]int

	keyed := func(n int) any {
		m := make(map[wide]int, n)

		for i := range n {
			m[wide{i}] = i
		}

		return m
	}

	boxed := func(n int) any {
		m := make(map[any]int, n)

		for i := range n {
			m[wide{i}] = i
		}

		return m
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

	for what, build := range map[string]func(int) any{"keys": keyed, "keys in interfaces": boxed} {
		var allocs [2]float64

		for i, n := range []int{400, 4000} {
			out, codes, a := resolve(build(n))

			if out != "D" || len(codes) != 1 || codes[0] != curly.UnserializableValue {
				t.Errorf("%s, %d of them: %q %v", what, n, out, codes)
			}

			allocs[i] = a
		}

		if allocs[1] > allocs[0] {
			t.Errorf("%s: %v allocations for 400, %v for 4000", what, allocs[0], allocs[1])
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
