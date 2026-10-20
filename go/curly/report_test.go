package curly

import "testing"

// No placeholder holds a line terminator, so none reaches Report.Text
// through Resolve; the escaping is held to every member of lineTerm here,
// for the day a report carries text that can hold one.
func TestExcerptEscapesEveryLineTerminator(t *testing.T) {
	want := map[rune]string{'\n': `\n`, '\r': `\r`, '\u2028': `\u2028`, '\u2029': `\u2029`}

	for _, term := range lineTerm {
		if got := excerpt("a" + string(term) + "b"); got != "a"+want[term]+"b" {
			t.Errorf("%U: got %q, want %q", term, got, "a"+want[term]+"b")
		}
	}

	if len(want) != len(lineTerm) {
		t.Errorf("lineTerm has %d members, the test knows %d", len(lineTerm), len(want))
	}

	if got := excerpt("\u0085\u00a0\x1f\xc3"); got != "\u0085\u00a0\\u001f\\ufffd" {
		t.Errorf("got %q", got)
	}
}
