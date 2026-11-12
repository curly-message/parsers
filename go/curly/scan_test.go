package curly

import (
	"runtime"
	"strings"
	"testing"
)

// linear holds a count to exact linearity over three sizes: what each
// further n adds is the same, so no part of the work grows with n squared,
// and the count is deterministic where a time would not be.
func linear(t *testing.T, what string, count func(n int) int) {
	t.Helper()

	const n = 2000

	one, two, three := count(n), count(2*n), count(3*n)

	if two-one != three-two || two-one <= 0 {
		t.Errorf("%s: %d, %d, %d at n, 2n, 3n", what, one, two, three)
	}
}

func TestScanStepsAreLinear(t *testing.T) {
	for _, c := range []struct {
		name    string
		message func(n int) string
	}{
		{"nested", func(n int) string { return strings.Repeat("{{a;x:", n) + "v" + strings.Repeat("}}", n) }},
		{"nested, never closed", func(n int) string { return strings.Repeat("{{a;x:", n) + "v" }},
		{"nested, then a line terminator", func(n int) string { return strings.Repeat("{{a;x:", n) + "\n" + strings.Repeat("}}", n) }},
		{"nested, one pair short", func(n int) string { return strings.Repeat("{{a;x:", n) + "v" + strings.Repeat("}}", n-1) }},
		{"opening braces", func(n int) string { return strings.Repeat("{", n) }},
		{"opening pairs in values", func(n int) string { return strings.Repeat("{{a;x:{{", n) }},
		{"siblings", func(n int) string { return strings.Repeat("{{a}}", n) }},
		{"siblings in one value", func(n int) string { return "{{a;x:" + strings.Repeat("{{b}}", n) + "}}" }},
		{"siblings in one unclosed value", func(n int) string { return "{{a;x:" + strings.Repeat("{{b}}", n) }},
		{"escapes", func(n int) string { return "{{" + strings.Repeat("\\}", n) + "}}" }},
		{"segments", func(n int) string { return "{{a" + strings.Repeat(";x:{{b}}", n) + "}}" }},
	} {
		linear(t, c.name, func(n int) int { return build(c.message(n)).sc.steps })
	}
}

func TestScanSettlesEachOpeningOnce(t *testing.T) {
	// Each opening pair of a message nested n deep and never closed is asked
	// about once from the outside and once from inside; the memo answers
	// every later question.
	message := strings.Repeat("{{a;x:", 100)
	sc := newScanner(message)

	if _, _, ok := sc.next(0, len(message)); ok {
		t.Fatalf("a placeholder derived")
	}

	for i := 0; i < len(message); i += 6 {
		if v, ok := sc.known(i); !ok || v != -1 {
			t.Errorf("opening at %d: %d, %v", i, v, ok)
		}
	}
}

func TestAppendJSONStepsAreLinear(t *testing.T) {
	linear(t, "appendJSON", func(n int) int {
		tree := CST(strings.Repeat("{{a;x:", n) + "v" + strings.Repeat("}}", n))
		steps := 0
		tree.appendJSON(nil, &steps)

		return steps
	})
}

func TestTrimmedKeepsAnEscapedBlank(t *testing.T) {
	for _, c := range []struct {
		s          string
		start, end int
	}{
		{"  v  ", 2, 3},
		{"v\\ ", 0, 3},
		{"v\\\\ ", 0, 3},
		{"v\\\\\\ ", 0, 5},
		{" \\  ", 1, 3},
		{"   ", 0, 0},
		{"", 0, 0},
		{"\u3000v\u3000", 3, 4},
		{"v\\\u3000", 0, 5},
		{"\xffv\xff", 0, 3},
	} {
		if start, end := trimmed(c.s, 0, len(c.s)); start != c.start || end != c.end {
			t.Errorf("trimmed(%q) = %d, %d, want %d, %d", c.s, start, end, c.start, c.end)
		}
	}
}

func TestUnesc(t *testing.T) {
	for _, c := range []struct{ in, out string }{
		{"plain", "plain"},
		{"a\\:b", "a:b"},
		{"a\\nb", "a\\nb"},
		{"a\\", "a\\"},
		{"\\\\", "\\"},
		{"\\\u3000", "\u3000"},
		{"\\\u00e9", "\\\u00e9"},
		{"\\\xff", "\\\xff"},
	} {
		if got := unesc(c.in); got != c.out {
			t.Errorf("unesc(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

// bytesAllocated is what f allocates on average over runs.
func bytesAllocated(runs int, f func()) uint64 {
	var before, after runtime.MemStats

	f()
	runtime.ReadMemStats(&before)

	for range runs {
		f()
	}

	runtime.ReadMemStats(&after)

	return (after.TotalAlloc - before.TotalAlloc) / uint64(runs)
}

// The scan records a verdict for each opening pair it settles, so what it
// allocates grows with the pairs a message opens, not with the text around
// them: ten times the text costs the record almost nothing more.
func TestScanRecordGrowsWithItsPairs(t *testing.T) {
	for _, tail := range []string{"{{a}}", "{{a"} {
		text := strings.Repeat("Lorem ipsum dolor sit amet. ", 700)
		short, long := text+tail, strings.Repeat(text, 10)+tail

		a := bytesAllocated(20, func() { CST(short) })
		b := bytesAllocated(20, func() { CST(long) })

		if b > a+uint64(len(long))/16 {
			t.Errorf("%q: %d bytes for %d bytes of message, %d bytes for %d", tail, a, len(short), b, len(long))
		}
	}
}
