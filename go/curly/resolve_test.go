package curly_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/curly-message/parsers/go/curly"
)

// resolved is one resolution's output and the codes it reported, in order.
type resolved struct {
	out   string
	codes []curly.Code
}

func resolveWith(options curly.Options, message any, payload map[string]any) (resolved, []curly.Report) {
	var reports []curly.Report

	options.OnReport = func(r curly.Report) { reports = append(reports, r) }
	out := curly.New(options).Resolve(message, curly.Context{Payload: payload, ID: "case"})
	r := resolved{out: out}

	for _, report := range reports {
		r.codes = append(r.codes, report.Code)
	}

	return r, reports
}

func resolve(message any, payload map[string]any) resolved {
	r, _ := resolveWith(curly.Options{}, message, payload)

	return r
}

type resolveCase struct {
	name    string
	message any
	payload map[string]any
	out     string
	codes   []curly.Code
}

func check(t *testing.T, cases []resolveCase) {
	t.Helper()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolve(c.message, c.payload)

			if got.out != c.out || fmt.Sprint(got.codes) != fmt.Sprint(c.codes) {
				t.Errorf("got %q %v, want %q %v", got.out, got.codes, c.out, c.codes)
			}
		})
	}
}

func TestResolveSubstitutes(t *testing.T) {
	check(t, []resolveCase{
		{"text alone", "Hello!", nil, "Hello!", nil},
		{"value", "Hello, {{name}}!", map[string]any{"name": "Alice"}, "Hello, Alice!", nil},
		{"padding", "{{ name }}", map[string]any{"name": "Alice"}, "Alice", nil},
		{"absent", "[{{name}}]", nil, "[]", nil},
		{"absent from a payload", "[{{name}}]", map[string]any{"other": "x"}, "[]", nil},
		{"the empty string is a value", "[{{name; default:D}}]", map[string]any{"name": ""}, "[]", nil},
		{"a value is data", "{{v}}", map[string]any{"v": "{{w}} \\{"}, "{{w}} \\{", nil},
		{"escapes in the message", "\\{{v}} \\\\ \\;", map[string]any{"v": "x"}, "{{v}} \\ ;", nil},
		{"one brace", "{ {{v}} }", map[string]any{"v": "x"}, "{ x }", nil},
		{"a refused pair is text", "{{a\nb}} {{v}}", map[string]any{"v": "x"}, "{{a\nb}} x", nil},
		{"a key holding a placeholder", "{{a{{v}}}}", map[string]any{"v": "x"}, "{{ax}}", nil},
		{"keys are case-sensitive", "{{Name}}", map[string]any{"name": "x"}, "", nil},
		{"a name unescaped", "{{a\\;b}}", map[string]any{"a;b": "x"}, "x", nil},
		{"a message that is a number", 42.0, nil, "42", nil},
		{"a message that is an object", map[string]any{"a": 1.0}, nil, `{"a":1}`, nil},
		{"a message that is undefined", curly.Undefined{}, nil, "", nil},
		{"a message nothing describes", cyclic(), nil, "", nil},
		{"a message that is nil", nil, nil, "<nil>", nil},
		{"a message converted is still scanned", []any{"{{v}}"}, map[string]any{"v": "x"}, `["x"]`, nil},
		{"a payload entry holding Undefined", "[{{v; default:D}}]", map[string]any{"v": curly.Undefined{}}, "[D]", nil},
		{"a payload entry holding nil", "[{{v; default:D}}]", map[string]any{"v": nil}, "[<nil>]", nil},
	})
}

func cyclic() map[string]any {
	m := map[string]any{}
	m["self"] = m

	return m
}

func TestResolveKeylessPlaceholderLooksNothingUp(t *testing.T) {
	check(t, []resolveCase{
		{"bare", "[{{}}]", map[string]any{"": "x"}, "[]", nil},
		{"modifier only", "[{{:eq; x:X}}]", map[string]any{"": "x"}, "[]", nil},
		{"payload default", "[{{:eq; x:X}}]", map[string]any{"default": "D"}, "[D]", nil},
		{"near miss: an escaped blank is a key", "[{{\\ }}]", map[string]any{" ": "x"}, "[x]", nil},
	})
}

// The table of section 10.
func TestResolveFallbackChain(t *testing.T) {
	const message = "Hello, {{name; default:Guest;}}!"

	check(t, []resolveCase{
		{"value", message, map[string]any{"name": "Alice", "default": "Friend"}, "Hello, Alice!", nil},
		{"empty value", message, map[string]any{"name": "", "default": "Friend"}, "Hello, !", nil},
		{"wrapper default", message, map[string]any{"name": map[string]any{"default": "You"}, "default": "Friend"}, "Hello, You!", nil},
		{"payload default", message, map[string]any{"default": "Friend"}, "Hello, Friend!", nil},
		{"inline default", message, map[string]any{}, "Hello, Guest!", nil},
		{"empty string", "[{{name}}]", map[string]any{}, "[]", nil},
		{"an empty inline default stops the chain", "[{{name; default:}}]", nil, "[]", nil},
		{"an empty wrapper default stops the chain", "[{{name; default:D}}]", map[string]any{"name": map[string]any{"default": ""}}, "[]", nil},
		{"the first inline default", "{{name; default:A; default:B}}", nil, "A", nil},
		{"an inline default is message text", "{{name; default:\\{{{v}}}}", map[string]any{"v": "x"}, "{x", nil},
		{"a payload default is data", "{{name}}", map[string]any{"default": "\\{{v}}"}, "\\{{v}}", nil},
		{"an undescribable link is skipped", "{{name; default:D}}", map[string]any{"name": map[string]any{"default": cyclic()}, "default": cyclic()}, "D", []curly.Code{curly.UnserializableValue, curly.UnserializableValue}},
		{"a wrapper default holding Undefined is no link", "{{name; default:D}}", map[string]any{"name": map[string]any{"default": curly.Undefined{}}}, "D", nil},
		{"a payload default holding Undefined is no link", "{{name; default:D}}", map[string]any{"default": curly.Undefined{}}, "D", nil},
	})
}

func TestResolveDefaultPlaceholder(t *testing.T) {
	check(t, []resolveCase{
		{"reads the payload default", "{{default}}", map[string]any{"default": "D"}, "D", nil},
		{"is never a wrapper", "{{default}}", map[string]any{"default": map[string]any{"value": "V"}}, `{"value":"V"}`, nil},
		{"reports an undescribable default once", "{{default; default:I}}", map[string]any{"default": cyclic()}, "I", []curly.Code{curly.UnserializableValue}},
		{"near miss: another key reports it as its link", "{{v; default:I}}", map[string]any{"default": cyclic()}, "I", []curly.Code{curly.UnserializableValue}},
		{"a selection falls back to itself", "{{default:eq; x:X}}", map[string]any{"default": "y"}, "y", nil},
	})
}

func TestResolveWrappers(t *testing.T) {
	type attrs map[string]any

	check(t, []resolveCase{
		{"value", "{{n}}", map[string]any{"n": map[string]any{"value": "V", "default": "D"}}, "V", nil},
		{"near miss: another member makes it a value", "{{n}}", map[string]any{"n": map[string]any{"value": 1.0, "unit": "kg"}}, `{"unit":"kg","value":1}`, nil},
		{"props alone", "[{{n; default:I}}]", map[string]any{"n": map[string]any{"props": map[string]any{}}}, "[I]", nil},
		{"near miss: an empty object is a value", "{{n; default:I}}", map[string]any{"n": map[string]any{}}, "{}", nil},
		{"value holding Undefined", "{{n; default:I}}", map[string]any{"n": map[string]any{"value": curly.Undefined{}, "default": "D"}}, "D", nil},
		{"any plain object", "{{n}}", map[string]any{"n": map[string]string{"value": "V"}}, "V", nil},
		{"near miss: a named map is no plain object", "{{n}}", map[string]any{"n": attrs{"value": "V"}}, "map[value:V]", nil},
		{"value converted", "{{n}}", map[string]any{"n": map[string]any{"value": []any{1.0}}}, "[1]", nil},
	})

	got, _ := resolveWith(curly.Options{IgnoreWrappers: true}, "{{n}}", map[string]any{"n": map[string]any{"value": "V"}})

	if got.out != `{"value":"V"}` {
		t.Errorf("IgnoreWrappers: got %q", got.out)
	}
}

func TestResolveComparisons(t *testing.T) {
	check(t, []resolveCase{
		{"eq", "{{v; a:A; b:B; default:D}}", map[string]any{"v": "b"}, "B", nil},
		{"eq named", "{{v:eq; a:A; b:B; default:D}}", map[string]any{"v": "c"}, "D", nil},
		{"eq ignores case", "{{v; ABC:A}}", map[string]any{"v": "aBc"}, "A", nil},
		{"eq selects an empty option", "[{{v; a:; default:D}}]", map[string]any{"v": "a"}, "[]", nil},
		{"eq compares a number as text", "{{v; 1:one; default:D}}", map[string]any{"v": 1.0}, "one", nil},
		{"near miss: 1.0 is not the text 1", "{{v; 1.0:one; default:D}}", map[string]any{"v": 1.0}, "D", nil},
		{"eq on a million from JSON", "{{v; 1000000:M; default:D}}", map[string]any{"v": 1e6}, "M", nil},
		{"KELVIN SIGN lowers to k", "{{v; \u212a:K; default:D}}", map[string]any{"v": "k"}, "K", nil},
		{"SHARP S lowers to its small letter", "{{v; \u1e9e:S; default:D}}", map[string]any{"v": "\u00df"}, "S", nil},
		{"near miss: STRASSE is no fold of the sharp s", "{{v; STRASSE:S; default:D}}", map[string]any{"v": "stra\u00dfe"}, "D", nil},
		{"the full mapping lowers U+0130 to i and a dot above", "{{v; \u0130:X; default:D}}", map[string]any{"v": "i\u0307"}, "X", nil},
		{"near miss: U+0130 does not lower to i alone", "{{v; \u0130:X; default:D}}", map[string]any{"v": "i"}, "D", nil},
		{"a sigma ending a word lowers to final sigma", "{{v; \u039f\u0394\u039f\u03a3:X; default:D}}", map[string]any{"v": "\u03bf\u03b4\u03bf\u03c2"}, "X", nil},
		{"near miss: a sigma ending a word is no medial sigma", "{{v; \u039f\u0394\u039f\u03a3:X; default:D}}", map[string]any{"v": "\u03bf\u03b4\u03bf\u03c3"}, "D", nil},
		{"a sigma inside a word lowers to medial sigma", "{{v; \u03a3\u0391:X; default:D}}", map[string]any{"v": "\u03c3\u03b1"}, "X", nil},
		{"a sigma alone is no final sigma", "{{v; \u03a3:X; default:D}}", map[string]any{"v": "\u03c3"}, "X", nil},
		{"case-ignorable characters are passed over around a sigma", "{{v; A.\u03a3'.:X; default:D}}", map[string]any{"v": "a.\u03c2'."}, "X", nil},
		{"near miss: a cased letter past them makes the sigma medial", "{{v; A.\u03a3'B:X; default:D}}", map[string]any{"v": "a.\u03c2'b"}, "D", nil},
		{"a sigma after a digit is no final sigma", "{{v; 1\u03a3:X; default:D}}", map[string]any{"v": "1\u03c3"}, "X", nil},
		{"invalid bytes compare as themselves", "{{v; \xff:A; default:D}}", map[string]any{"v": "\xff"}, "A", nil},
		{"near miss: two invalid bytes differ", "{{v; \xfe:A; default:D}}", map[string]any{"v": "\xff"}, "D", nil},
		{"ne", "{{v:ne; a:A; b:B}}", map[string]any{"v": "A"}, "B", nil},
		{"ne absent takes the chain", "{{v:ne; a:A; default:D}}", nil, "D", nil},
		{"lt ascending", "{{v:lt; 10:A; 5:B; default:D}}", map[string]any{"v": 3.0}, "B", nil},
		{"lt none", "{{v:lt; 10:A; 5:B; default:D}}", map[string]any{"v": 30.0}, "D", nil},
		{"gt descending", "{{v:gt; 1:A; 5:B; default:D}}", map[string]any{"v": 7.0}, "B", nil},
		{"lt passes a NaN key over", "{{v:lt; 10:A; NaN:N; 5:B; default:D}}", map[string]any{"v": 3.0}, "B", nil},
		{"lt passes nan over", "{{v:lt; 10:A; nan:N; 5:B; default:D}}", map[string]any{"v": 3.0}, "B", nil},
		{"Inf is a number", "{{v:lt; Inf:I; 5:B; default:D}}", map[string]any{"v": 7.0}, "I", nil},
		{"-Inf is a number", "{{v:gt; -Inf:I; 5:B; default:D}}", map[string]any{"v": 3.0}, "I", nil},
		{"1_000 is a number", "{{v:gt; 1_000:K; default:D}}", map[string]any{"v": 1001.0}, "K", nil},
		{"0x10 is no number", "{{v:gt; 0x10:H; default:D}}", map[string]any{"v": 17.0}, "D", nil},
		{"a value that is no number", "{{v:lt; 10:A; default:D}}", map[string]any{"v": "ten"}, "D", nil},
		{"a value read with padding", "{{v:lt; 10:A; default:D}}", map[string]any{"v": " 3\t"}, "A", nil},
		{"a tie keeps source order", "{{v:lt; 2.0:A; 2:B; default:D}}", map[string]any{"v": 1.0}, "A", nil},
		{"a tie keeps source order descending", "{{v:gt; 2:A; 2.0:B; default:D}}", map[string]any{"v": 3.0}, "A", nil},
		{"lte equality leg", "{{v:lte; abc:X}}", map[string]any{"v": "abc"}, "X", nil},
		{"lte equality first", "{{v:lte; 10:A; 5:B}}", map[string]any{"v": 5.0}, "B", nil},
		{"lte strict leg", "{{v:lte; 10:A; 5:B}}", map[string]any{"v": 4.0}, "B", nil},
		{"gte equality first", "{{v:gte; 1:A; 5:B}}", map[string]any{"v": 5.0}, "B", nil},
		{"gte strict leg", "{{v:gte; 1:A; 5:B; default:D}}", map[string]any{"v": 3.0}, "A", nil},
		{"gte none", "{{v:gte; 1:A; 5:B; default:D}}", map[string]any{"v": 0.0}, "D", nil},
		{"absent", "{{v:lt; 10:A; default:D}}", nil, "D", nil},
		{"no options", "{{v:eq; default:D}}", map[string]any{"v": "a"}, "D", []curly.Code{curly.MissingOptions}},
		{"no options and no key", "{{:lt; default:D}}", map[string]any{"v": "a"}, "D", nil},
		{"no options over an absent value", "{{v:gte; default:D}}", nil, "D", []curly.Code{curly.MissingOptions}},
		{"names are case-sensitive", "{{v:EQ; a:A; default:D}}", map[string]any{"v": "a"}, "D", []curly.Code{curly.UnknownModifier}},
		{"an unknown modifier over an absent value", "{{v:nope; default:D}}", nil, "D", []curly.Code{curly.UnknownModifier}},
		{"a selected option renders", "{{v; a:<{{w}}>; default:D}}", map[string]any{"v": "a", "w": "W"}, "<W>", nil},
	})
}

// An option and a default that nobody asks for are never read.
func TestResolveReadsWhatIsAsked(t *testing.T) {
	check(t, []resolveCase{
		{"an option passed over", "{{v; a:A; b:{{x}}}}", map[string]any{"v": "a", "x": cyclic()}, "A", nil},
		{"a default passed over", "{{v; a:A; default:{{x}}}}", map[string]any{"v": "a", "x": cyclic()}, "A", nil},
		{"near miss: the option asked for", "{{v; a:A; b:{{x}}}}", map[string]any{"v": "b", "x": cyclic()}, "", []curly.Code{curly.UnserializableValue}},
		{"a payload default passed over", "{{v}}", map[string]any{"v": "x", "default": cyclic()}, "x", nil},
	})
}

func TestResolveReports(t *testing.T) {
	_, reports := resolveWith(curly.Options{}, "{{v:nope}} {{w:eq}}", map[string]any{"v": 1.0, "w": 1.0})

	want := []curly.Report{
		{Code: curly.UnknownModifier, Origin: curly.OriginMessage, Message: "A placeholder named a modifier this parser does not know.", ID: "case", Text: "{{v:nope}}"},
		{Code: curly.MissingOptions, Origin: curly.OriginMessage, Message: "A selection was given no options to select from, so the placeholder took its fallback chain.", ID: "case", Text: "{{w:eq}}"},
	}

	if fmt.Sprint(reports) != fmt.Sprint(want) {
		t.Errorf("got %+v\nwant %+v", reports, want)
	}

	origins := map[curly.Code]curly.Origin{
		curly.UnknownModifier:     curly.OriginMessage,
		curly.FailedModifier:      curly.OriginPayload,
		curly.MissingOptions:      curly.OriginMessage,
		curly.UnserializableValue: curly.OriginPayload,
		curly.MissingLocale:       curly.OriginPayload,
		curly.NestingLimit:        curly.OriginMessage,
		curly.OutputLimit:         curly.OriginLimit,
		curly.ReadLimit:           curly.OriginLimit,
		curly.Code("other"):       "",
	}

	for code, origin := range origins {
		if code.Origin() != origin {
			t.Errorf("%s: origin %q, want %q", code, code.Origin(), origin)
		}
	}
}

func TestResolveReportTextIsEscaped(t *testing.T) {
	for _, c := range []struct{ message, text string }{
		{"{{v:nope; a:\"q\" \\\\ \t}}", `{{v:nope; a:\"q\" \\\\ \t}}`},
		{"{{v:nope; a:\x01\x7f}}", "{{v:nope; a:\\u0001\x7f}}"},
		{"{{v:nope; a:\xff}}", `{{v:nope; a:\ufffd}}`},
	} {
		_, reports := resolveWith(curly.Options{}, c.message, nil)

		if len(reports) != 1 || reports[0].Text != c.text {
			t.Errorf("%q: got %+v, want text %q", c.message, reports, c.text)
		}
	}
}

func TestResolveReportTextIsCut(t *testing.T) {
	long := "{{v:nope; a:" + strings.Repeat("x", 200) + "}}"
	_, reports := resolveWith(curly.Options{}, long, nil)

	if want := long[:curly.MaxReportedLength] + "..."; reports[0].Text != want {
		t.Errorf("got %q, want %q", reports[0].Text, want)
	}

	// A three-byte character straddling the cut is left out whole.
	straddling := "{{v:nope; a:" + strings.Repeat("x", curly.MaxReportedLength-13) + "\u20ac}}"
	_, reports = resolveWith(curly.Options{}, straddling, nil)

	if want := straddling[:curly.MaxReportedLength-1] + "..."; reports[0].Text != want {
		t.Errorf("got %q, want %q", reports[0].Text, want)
	}

	exact := "{{v:nope; a:" + strings.Repeat("x", curly.MaxReportedLength-14) + "}}"
	_, reports = resolveWith(curly.Options{}, exact, nil)

	if len(exact) != curly.MaxReportedLength || reports[0].Text != exact {
		t.Errorf("got %q, want %q uncut", reports[0].Text, exact)
	}
}

func TestResolveSurvivesAPanickingOnReport(t *testing.T) {
	p := curly.New(curly.Options{OnReport: func(curly.Report) { panic("logger down") }})

	if got := p.Resolve("{{v:nope; default:D}} {{w}}", curly.Context{Payload: map[string]any{"w": "W"}}); got != "D W" {
		t.Errorf("got %q", got)
	}
}

func TestResolveFloats(t *testing.T) {
	var payload map[string]any

	if err := json.Unmarshal([]byte(`{"a":1000000,"b":2500000,"c":0.00001,"d":1e21,"e":-0,"f":0.1,"g":123456789012}`), &payload); err != nil {
		t.Fatal(err)
	}

	check(t, []resolveCase{
		{"from JSON", "{{a}} {{b}} {{c}} {{d}} {{e}} {{f}} {{g}}", payload, "1000000 2500000 0.00001 1e+21 0 0.1 123456789012", nil},
		{"NaN and the infinities", "{{a}} {{b}} {{c}}", map[string]any{"a": math.NaN(), "b": math.Inf(1), "c": math.Inf(-1)}, "NaN Infinity -Infinity", nil},
		{"float32", "{{a}}", map[string]any{"a": float32(0.1)}, "0.1", nil},
		{"integers", "{{a}} {{b}} {{c}}", map[string]any{"a": int8(-8), "b": uint64(math.MaxUint64), "c": 7}, "-8 18446744073709551615 7", nil},
		{"NaN in JSON is refused", "{{a; default:D}}", map[string]any{"a": []any{math.NaN()}}, "D", []curly.Code{curly.UnserializableValue}},
		{"-0 in JSON", "{{a}}", map[string]any{"a": []any{math.Copysign(0, -1)}}, "[0]", nil},
		{"floats nested in a struct keep fmt's form", "{{a}}", map[string]any{"a": struct{ F float64 }{1e6}}, "{1e+06}", nil},
	})
}

type celsius float64

func (c celsius) String() string { return fmt.Sprintf("%.1f C", float64(c)) }

type nilSafe struct{}

func (n *nilSafe) String() string {
	if n == nil {
		return "nobody"
	}

	return "somebody"
}

type deref struct{ name string }

func (d *deref) String() string { return d.name }

type secret struct{}

func (secret) String() string { panic("password=hunter2") }

type failing struct{}

func (failing) Error() string { panic("password=hunter2") }

type formatting struct{}

func (formatting) Format(s fmt.State, verb rune) { panic("password=hunter2") }

type marshalling struct{}

func (marshalling) MarshalJSON() ([]byte, error) { panic("password=hunter2") }

type refusing struct{}

func (refusing) MarshalJSON() ([]byte, error) { return nil, errors.New("password=hunter2") }

type formatted struct{}

func (formatted) Format(s fmt.State, verb rune) { fmt.Fprintf(s, "F(%c)", verb) }

type node struct {
	Name string
	Next *node
}

type selfish []any

func TestResolveConverts(t *testing.T) {
	type attrs map[string]any

	type key string

	when := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	loop := selfish{1}
	loop[0] = loop
	n := &node{Name: "a"}
	n.Next = n

	check(t, []resolveCase{
		{"Undefined members are left out", "{{a}}", map[string]any{"a": map[string]any{"x": curly.Undefined{}, "y": 1.0}}, `{"y":1}`, nil},
		{"Undefined elements are null", "{{a}}", map[string]any{"a": []any{curly.Undefined{}, nil}}, "[null,null]", nil},
		{"keys sorted, HTML left alone", "{{a}}", map[string]any{"a": map[string]any{"b": "<&>", "a": "\u2028"}}, `{"a":"\u2028","b":"<&>"}`, nil},
		{"a byte that is not UTF-8 escaped", "{{a}}", map[string]any{"a": map[string]any{"\xff": "\xfe a"}}, `{"\ufffd":"\ufffd a"}`, nil},
		{"a plain map of any element", "{{a}}", map[string]any{"a": map[string]int{"x": 1}}, `{"x":1}`, nil},
		{"near miss: a named map prints", "{{a}}", map[string]any{"a": attrs{"x": 1}}, "map[x:1]", nil},
		{"near miss: a map keyed by a named string prints", "{{a}}", map[string]any{"a": map[key]any{"x": 1}}, "map[x:1]", nil},
		{"a plain slice", "{{a}}", map[string]any{"a": []int{1, 2}}, "[1,2]", nil},
		{"near miss: bytes print", "{{a}}", map[string]any{"a": []byte("hi")}, "[104 105]", nil},
		{"near miss: an array prints", "{{a}}", map[string]any{"a": [2]int{1, 2}}, "[1 2]", nil},
		{"a pointer to a struct", "{{a}}", map[string]any{"a": &struct{ A, B int }{1, 2}}, "&{1 2}", nil},
		{"a time", "{{a}}", map[string]any{"a": when}, when.String(), nil},
		{"a time in JSON", "{{a}}", map[string]any{"a": []any{when}}, `["2024-05-06T07:08:09Z"]`, nil},
		{"a Stringer", "{{a}}", map[string]any{"a": celsius(21.5)}, "21.5 C", nil},
		{"a Stringer nested", "{{a}}", map[string]any{"a": struct{ C celsius }{21.5}}, "{21.5 C}", nil},
		{"a Formatter", "{{a}}", map[string]any{"a": formatted{}}, "F(v)", nil},
		{"an error", "{{a}}", map[string]any{"a": errors.New("boom")}, "boom", nil},
		{"a typed nil with a nil-safe String", "{{a}}", map[string]any{"a": (*nilSafe)(nil)}, "nobody", nil},
		{"a typed nil whose String dereferences", "{{a}}", map[string]any{"a": (*deref)(nil)}, "<nil>", nil},
		{"a typed nil nested", "{{a}}", map[string]any{"a": struct{ D *deref }{}}, "{<nil>}", nil},
		{"a duration", "{{a}}", map[string]any{"a": time.Hour}, "1h0m0s", nil},
		{"a complex number", "{{a}}", map[string]any{"a": complex(1, -2)}, "(1-2i)", nil},
		{"a nil map", "{{a}}", map[string]any{"a": map[string]any(nil)}, "null", nil},
		{"a nil error", "{{a}}", map[string]any{"a": error(nil)}, "<nil>", nil},
	})

	panics := []resolveCase{
		{"String", "{{a; default:D}}", map[string]any{"a": secret{}}, "D", []curly.Code{curly.UnserializableValue}},
		{"Error", "{{a; default:D}}", map[string]any{"a": failing{}}, "D", []curly.Code{curly.UnserializableValue}},
		{"Format", "{{a; default:D}}", map[string]any{"a": formatting{}}, "D", []curly.Code{curly.UnserializableValue}},
		{"String in a struct", "{{a; default:D}}", map[string]any{"a": struct{ S secret }{}}, "D", []curly.Code{curly.UnserializableValue}},
		{"String in a named map", "{{a; default:D}}", map[string]any{"a": attrs{"s": secret{}}}, "D", []curly.Code{curly.UnserializableValue}},
		{"String in a pointer", "{{a; default:D}}", map[string]any{"a": &struct{ S secret }{}}, "D", []curly.Code{curly.UnserializableValue}},
		{"MarshalJSON in a plain object", "{{a; default:D}}", map[string]any{"a": map[string]any{"m": marshalling{}}}, "D", []curly.Code{curly.UnserializableValue}},
		{"MarshalJSON failing in a plain array", "{{a; default:D}}", map[string]any{"a": []any{refusing{}}}, "D", []curly.Code{curly.UnserializableValue}},
		{"String in a plain array", "{{a; default:D}}", map[string]any{"a": []any{struct{ S secret }{}}}, `[{"S":{}}]`, nil},
		{"a slice holding itself", "{{a; default:D}}", map[string]any{"a": loop}, "D", []curly.Code{curly.UnserializableValue}},
		{"a pointer cycle in JSON", "{{a; default:D}}", map[string]any{"a": []any{n}}, "D", []curly.Code{curly.UnserializableValue}},
		{"a channel in JSON", "{{a; default:D}}", map[string]any{"a": []any{make(chan int)}}, "D", []curly.Code{curly.UnserializableValue}},
		{"a plain cycle", "{{a; default:D}}", map[string]any{"a": cyclic()}, "D", []curly.Code{curly.UnserializableValue}},
	}

	check(t, panics)

	// fmt writes a pointer below the top level as its address, so a cycle
	// through one is no cycle to the conversion.
	if got := resolve("{{a}}", map[string]any{"a": n}).out; !strings.HasPrefix(got, "&{a 0x") {
		t.Errorf("a pointer cycle: got %q", got)
	}

	for _, c := range panics {
		if out := resolve(c.message, c.payload).out; strings.Contains(out, "hunter2") {
			t.Errorf("%s: a panic's text reached the output: %q", c.name, out)
		}
	}
}

type pointerText uint8

func (*pointerText) MarshalText() ([]byte, error) { return []byte("t"), nil }

type pointerJSON struct{ A int }

func (*pointerJSON) MarshalJSON() ([]byte, error) { return []byte(`"s"`), nil }

type octetKind uint8

// A member of a plain value is written as encoding/json writes it in that
// value. An element of a slice is addressable, so a method of its pointer
// writes it, as one does inside a struct; a map's value is not, so the
// method is not called. A slice of a byte kind with no such method is
// base64.
func TestResolveWritesMembersAsEncodingJSON(t *testing.T) {
	type items struct{ Items []pointerText }

	type octets struct{ Octets []octetKind }

	for _, v := range []any{
		[]pointerText{1, 2},
		[]pointerJSON{{1}},
		map[string]any{"x": []pointerJSON{{1}}},
		[]any{[]pointerText{1}, items{[]pointerText{1}}},
		[]any{[]octetKind{1, 2}, octets{[]octetKind{1, 2}}},
		[][]pointerJSON{{{1}}},
		[]struct{ P pointerJSON }{{}},
		map[string]pointerJSON{"x": {1}},
		[]any{pointerJSON{1}, pointerText(1)},
		[]*pointerJSON{{1}, nil},
	} {
		want, err := json.Marshal(v)

		if err != nil {
			t.Fatal(err)
		}

		if got := resolve("{{v}}", map[string]any{"v": v}); got.out != string(want) || got.codes != nil {
			t.Errorf("%T: got %q %v, want %q", v, got.out, got.codes, want)
		}
	}

	expect(t, "near miss: bytes of a kind of their own print", resolve("{{v}}", map[string]any{"v": []octetKind{1, 2}}), "[1 2]")
}

func TestResolveEveryReadReportsAnUndescribableValue(t *testing.T) {
	got := resolve("{{a}}{{a}}{{a; default:D}}", map[string]any{"a": secret{}})

	if got.out != "D" || len(got.codes) != 3 {
		t.Errorf("got %q %v", got.out, got.codes)
	}
}

type counted struct {
	calls *int
	F     float64
}

func (c counted) String() string {
	*c.calls++

	return "counted"
}

type closure func() string

func (f closure) String() string { return f() }

// A value converts once however many placeholders read it, and a record
// keyed by where it was read tells two values apart that equality cannot.
func TestResolveConvertsOnce(t *testing.T) {
	calls := 0
	shared := &counted{calls: &calls}
	message := strings.Repeat("{{a}}", 100)

	if resolve(message, map[string]any{"a": shared}).out != strings.Repeat("counted", 100) || calls != 1 {
		t.Errorf("a pointer read 100 times converted %d times", calls)
	}

	calls = 0
	resolve(message, map[string]any{"a": counted{calls: &calls, F: math.NaN()}})

	if calls != 1 {
		t.Errorf("a struct holding NaN read 100 times converted %d times", calls)
	}

	calls = 0
	value := counted{calls: &calls}
	resolve("{{a}}{{b}}", map[string]any{"a": value, "b": value})

	if calls != 2 {
		t.Errorf("two copies under two keys converted %d times, want 2", calls)
	}

	calls = 0
	resolve("{{a}}{{a}}", map[string]any{"a": counted{calls: &calls}})
	resolve("{{a}}", map[string]any{"a": counted{calls: &calls}})

	if calls != 2 {
		t.Errorf("two resolutions converted %d times, want 2", calls)
	}

	first, second := "first", "second"
	got := resolve("{{a}} {{b}}", map[string]any{
		"a": closure(func() string { return first }),
		"b": closure(func() string { return second }),
	})

	if got.out != "first second" {
		t.Errorf("two closures of one type: got %q", got.out)
	}
}

func TestResolveFromInsideAConversion(t *testing.T) {
	p := curly.New(curly.Options{})
	inner := closure(func() string {
		return p.Resolve("<{{b}}>", curly.Context{Payload: map[string]any{"b": "B"}})
	})

	if got := p.Resolve("{{a}}", curly.Context{Payload: map[string]any{"a": inner}}); got != "<B>" {
		t.Errorf("got %q", got)
	}
}

func TestResolveConcurrently(t *testing.T) {
	p := curly.New(curly.Options{OnReport: func(curly.Report) {}})
	payload := map[string]any{"v": "b", "o": map[string]any{"a": []any{1.0}}, "s": celsius(1)}
	done := make(chan string)

	for range 8 {
		go func() {
			out := ""

			for range 100 {
				out = p.Resolve("{{v; a:A; b:B}} {{o}} {{s}} {{x:nope}}", curly.Context{Payload: payload})
			}

			done <- out
		}()
	}

	for range 8 {
		if out := <-done; out != `B {"a":[1]} 1.0 C ` {
			t.Errorf("got %q", out)
		}
	}
}
