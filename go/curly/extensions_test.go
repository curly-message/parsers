package curly_test

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/curly-message/parsers/go/curly"
)

// host resolves with the modifiers given, reporting into the result.
func host(modifiers map[string]curly.Modifier, message string, payload map[string]any) resolved {
	got, _ := resolveWith(curly.Options{CustomModifiers: modifiers}, message, payload)

	return got
}

func answering(answer any, err error) map[string]curly.Modifier {
	return map[string]curly.Modifier{"m": func(curly.Input) (any, error) { return answer, err }}
}

func upper(in curly.Input) (any, error) { return strings.ToUpper(in.Value), nil }

func TestExtensionsRegistration(t *testing.T) {
	payload := map[string]any{"v": "abc"}

	expect(t, "by name", host(map[string]curly.Modifier{"upper": upper}, "{{v:upper}}", payload), "ABC")
	expect(t, "near miss: another name is unknown", host(map[string]curly.Modifier{"upper": upper}, "{{v:lower; default:D}}", payload), "D", curly.UnknownModifier)
	expect(t, "a nil entry registers nothing", host(map[string]curly.Modifier{"upper": nil}, "{{v:upper; default:D}}", payload), "D", curly.UnknownModifier)

	// A host modifier in a selection's place is asked like any, options or
	// none, and so is one answering where a selection is implied.
	expect(t, "over a selection", host(map[string]curly.Modifier{"eq": upper}, "{{v:eq}}", payload), "ABC")
	expect(t, "near miss: the format's own reports", host(nil, "{{v:eq; default:D}}", payload), "D", curly.MissingOptions)
	expect(t, "an implied eq", host(map[string]curly.Modifier{"eq": upper}, "{{v; abc:X}}", payload), "ABC")
	expect(t, "near miss: no options imply nothing", host(map[string]curly.Modifier{"eq": upper}, "{{v}}", payload), "abc")
}

func TestExtensionsRegistrationIsCopied(t *testing.T) {
	modifiers := map[string]curly.Modifier{"m": upper}
	defaults := map[string]any{"m": map[string]any{"a": "1"}}
	p := curly.New(curly.Options{CustomModifiers: modifiers, ModifierDefaults: defaults, OnReport: func(curly.Report) {}})

	props := func(in curly.Input) (any, error) { return in.Props, nil }
	modifiers["m"] = props
	modifiers["n"] = props
	defaults["m"].(map[string]any)["a"] = "2"
	defaults["n"] = map[string]any{"b": "3"}

	if got := p.Resolve("{{v:m}}{{v:n; default:-}}", curly.Context{Payload: map[string]any{"v": "x"}}); got != "X-" {
		t.Errorf("got %q", got)
	}
}

func TestExtensionsAnswers(t *testing.T) {
	const message = "[{{v:m; default:D}}]"

	payload := map[string]any{"v": "x"}

	expect(t, "text", host(answering("A", nil), message, payload), "[A]")
	expect(t, "empty text is an answer", host(answering("", nil), message, payload), "[]")
	expect(t, "nil is none", host(answering(nil, nil), message, payload), "[D]")
	expect(t, "Undefined is none", host(answering(curly.Undefined{}, nil), message, payload), "[D]")
	expect(t, "an error", host(answering("A", errors.New("no")), message, payload), "[D]", curly.FailedModifier)
	expect(t, "an undescribable answer", host(answering(cyclic(), nil), message, payload), "[D]", curly.UnserializableValue)
	expect(t, "an answer whose String panics", host(answering(secret{}, nil), message, payload), "[D]", curly.UnserializableValue)

	// An answer becomes text as a payload value does; it is no payload
	// entry, so a wrapper's shape is a value.
	var typedNil *nilSafe

	for _, answer := range []any{1.5, math.Copysign(0, -1), true, 42, []any{1.0, "a", nil}, map[string]any{"b": 1.0, "a": curly.Undefined{}}, typedNil, (*int)(nil), celsius(21), []byte("hi"), map[string]any{"value": "V"}} {
		want, _ := resolveWith(curly.Options{IgnoreWrappers: true}, "{{v}}", map[string]any{"v": answer})

		expect(t, fmt.Sprintf("%T %v", answer, answer), host(answering(answer, nil), "{{v:m}}", payload), want.out)
	}
}

func TestExtensionsPanicsAreContained(t *testing.T) {
	for what, m := range map[string]curly.Modifier{
		"a panic":                    func(curly.Input) (any, error) { panic("password=hunter2") },
		"a panic of an error":        func(curly.Input) (any, error) { panic(failing{}) },
		"an error whose text panics": func(curly.Input) (any, error) { return nil, failing{} },
		"a runtime error":            func(in curly.Input) (any, error) { return in.Options[len(in.Options)], nil },
	} {
		got, reports := resolveWith(curly.Options{CustomModifiers: map[string]curly.Modifier{"m": m}}, "a{{v:m; default:D}}b{{v}}", map[string]any{"v": "x"})
		expect(t, what, got, "aDbx", curly.FailedModifier)

		if text := fmt.Sprintf("%+v", reports); strings.Contains(text, "hunter2") || reports[0].Text != "{{v:m; default:D}}" {
			t.Errorf("%s: reported %s", what, text)
		}
	}
}

// A modifier is asked about a value; where there is none, it is not asked.
func TestExtensionsAreAskedAboutAValue(t *testing.T) {
	calls := 0
	m := map[string]curly.Modifier{"m": func(curly.Input) (any, error) { calls++; return "A", nil }}

	expect(t, "keyless", host(m, "[{{:m; default:D}}]", nil), "[D]")
	expect(t, "absent", host(m, "[{{v:m; default:D}}]", nil), "[D]")
	expect(t, "Undefined", host(m, "[{{v:m; default:D}}]", map[string]any{"v": curly.Undefined{}}), "[D]")
	expect(t, "undescribable", host(m, "[{{v:m; default:D}}]", map[string]any{"v": cyclic()}), "[D]", curly.UnserializableValue)

	if calls != 0 {
		t.Errorf("asked %d times", calls)
	}

	var value string

	m["m"] = func(in curly.Input) (any, error) { value = in.Value; return "A", nil }
	expect(t, "near miss: an empty value is one", host(m, "[{{v:m}}]", map[string]any{"v": ""}), "[A]")
	expect(t, "a wrapper's value", host(m, "[{{v:m}}]", map[string]any{"v": map[string]any{"value": 2.5}}), "[A]")

	if value != "2.5" {
		t.Errorf("got value %q", value)
	}
}

func TestExtensionsOptions(t *testing.T) {
	var keys []string

	m := map[string]curly.Modifier{"m": func(in curly.Input) (any, error) {
		keys = keys[:0]

		for _, o := range in.Options {
			keys = append(keys, o.Key)
		}

		return in.Options[2].Value() + in.Options[2].Value() + in.Options[0].Value(), nil
	}}

	// The unread option names a modifier nobody registered, which it would
	// report were it rendered; the read one reports once however often read.
	expect(t, "", host(m, "{{v:m; b:{{v:nope; default:1}}; a\\;:{{w:nope}}; b:{{v}}{{v:upper; default:2}}; default:D}}", map[string]any{"v": "x"}), "x2x21", curly.UnknownModifier, curly.UnknownModifier)

	if want := []string{"b", "a;", "b"}; !slices.Equal(keys, want) {
		t.Errorf("keys %q, want %q", keys, want)
	}
}

func TestExtensionsDefault(t *testing.T) {
	reading := func(times int) map[string]curly.Modifier {
		return map[string]curly.Modifier{"m": func(in curly.Input) (any, error) {
			var out string

			for range times {
				out += in.Default()
			}

			return "<" + out + ">", nil
		}}
	}

	const message = "{{v:m; default:{{w:nope; default:I}}}}"

	expect(t, "unread", host(reading(0), message, map[string]any{"v": "x"}), "<>")
	expect(t, "read twice, walked once", host(reading(2), message, map[string]any{"v": "x"}), "<II>", curly.UnknownModifier)
	expect(t, "the whole chain", host(reading(1), message, map[string]any{"v": map[string]any{"value": "x", "default": "W"}}), "<W>")
	expect(t, "the payload's link", host(reading(1), message, map[string]any{"v": "x", "default": "P"}), "<P>")

	// A modifier's own fallback is its answer of none.
	expect(t, "no answer", host(answering(nil, nil), "{{v:m; default:{{w:nope; default:I}}}}", map[string]any{"v": "x"}), "I", curly.UnknownModifier)
}

// The input's options and default are the placeholder's for as long as the
// modifier holds them: a modifier that keeps them reads them after it
// returned.
func TestExtensionsInputOutlivesTheCall(t *testing.T) {
	var kept curly.Input

	m := map[string]curly.Modifier{"m": func(in curly.Input) (any, error) {
		kept = in
		return in.Options[0].Value(), nil
	}}

	got, reports := resolveWith(curly.Options{CustomModifiers: m}, "{{v:m; a:{{v}}; b:{{v:upper; default:B}}; default:{{w; default:I}}}}", map[string]any{"v": "x"})
	expect(t, "during", got, "x")

	if len(reports) != 0 || kept.Options[0].Value() != "x" || kept.Options[1].Value() != "B" || kept.Default() != "I" {
		t.Errorf("after: %q %q %q", kept.Options[0].Value(), kept.Options[1].Value(), kept.Default())
	}

	var zero curly.Input

	if zero.Default() != "" || (curly.Option{}).Value() != "" {
		t.Error("the zero input answers")
	}
}

// A modifier may read its options from goroutines of its own, which
// serialize on the resolution's state; under -race this is the test that
// they do, through option values that call host code of their own: a nested
// modifier and a String method.
func TestExtensionsConcurrentReads(t *testing.T) {
	calls := 0

	m := map[string]curly.Modifier{
		"fan": func(in curly.Input) (any, error) {
			var wg sync.WaitGroup

			out := make([]string, 8)

			for i := range out {
				wg.Go(func() {
					if i%4 == 3 {
						out[i] = in.Default()
					} else {
						out[i] = in.Options[i%4].Value()
					}
				})
			}

			wg.Wait()

			return strings.Join(out, ","), nil
		},
		"upper": upper,
	}

	var mu sync.Mutex

	codes := map[curly.Code]int{}

	p := curly.New(curly.Options{CustomModifiers: m, OnReport: func(r curly.Report) {
		mu.Lock()
		codes[r.Code]++
		mu.Unlock()
	}})

	got := p.Resolve("{{v:fan; a:{{v:upper}}; b:{{c}}{{c}}; c:{{v:nope; default:N}}; default:{{w; default:I}}}}", curly.Context{Payload: map[string]any{"v": "x", "c": counted{calls: &calls}}})

	if got != "X,countedcounted,N,I,X,countedcounted,N,I" || !maps.Equal(codes, map[curly.Code]int{curly.UnknownModifier: 1}) || calls != 1 {
		t.Errorf("got %q, %v, String called %d times", got, codes, calls)
	}
}

// gate is a Stringer whose first call holds until the test lets it go.
type gate struct {
	calls   *atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (g gate) String() string {
	if g.calls.Add(1) == 1 {
		close(g.entered)
		<-g.release
	}

	return "S"
}

// waitingOnConversion reports whether a goroutine waits for a conversion
// another one is making.
func waitingOnConversion() bool {
	buf := make([]byte, 1<<20)
	stacks := string(buf[:runtime.Stack(buf, true)])

	for _, g := range strings.Split(stacks, "\n\n") {
		if strings.Contains(g, "sync.(*Cond).Wait") && strings.Contains(g, "curly.(*walk).convert") {
			return true
		}
	}

	return false
}

// A value read by two options on two goroutines converts once: the second
// reader, arriving while the first is inside the value's String method,
// waits for its text.
func TestExtensionsConcurrentConversion(t *testing.T) {
	g := gate{calls: new(atomic.Int32), entered: make(chan struct{}), release: make(chan struct{})}

	m := map[string]curly.Modifier{"both": func(in curly.Input) (any, error) {
		var wg sync.WaitGroup

		out := make([]string, 2)

		wg.Go(func() { out[0] = in.Options[0].Value() })
		<-g.entered
		wg.Go(func() { out[1] = in.Options[1].Value() })

		for g.calls.Load() == 1 && !waitingOnConversion() {
			runtime.Gosched()
		}

		close(g.release)
		wg.Wait()

		return strings.Join(out, " "), nil
	}}

	got := curly.New(curly.Options{CustomModifiers: m}).Resolve("{{v:both; a:{{s}}; b:{{s}}}}", curly.Context{Payload: map[string]any{"v": "x", "s": g}})

	if got != "S S" || g.calls.Load() != 1 {
		t.Errorf("got %q, String called %d times", got, g.calls.Load())
	}
}

func TestExtensionsProps(t *testing.T) {
	type layer map[string]any

	var props map[string]any

	m := map[string]curly.Modifier{"m": func(in curly.Input) (any, error) {
		props = in.Props
		in.Props["written"] = true

		return "", nil
	}}

	run := func(what string, defaults, call map[string]any, payload map[string]any, want map[string]any) {
		t.Helper()

		p := curly.New(curly.Options{CustomModifiers: m, ModifierDefaults: defaults})

		for range 2 {
			p.Resolve("{{v:m}}", curly.Context{Payload: payload, Props: call})
			delete(props, "written")

			if fmt.Sprint(props) != fmt.Sprint(want) {
				t.Errorf("%s: got %v, want %v", what, props, want)
			}
		}
	}

	value := map[string]any{"v": "x"}
	defaults := map[string]any{"m": map[string]any{"a": "defaults", "b": "defaults", "c": "defaults"}, "other": map[string]any{"z": 1}}
	call := map[string]any{"m": map[string]any{"b": "call", "c": "call"}}
	wrapped := map[string]any{"v": map[string]any{"value": "x", "props": map[string]any{"m": map[string]any{"c": "wrapper"}}}}

	run("none", nil, nil, value, map[string]any{})
	run("each layer per property", defaults, call, wrapped, map[string]any{"a": "defaults", "b": "call", "c": "wrapper"})
	run("own name only", map[string]any{"other": map[string]any{"z": 1}}, map[string]any{"other": map[string]any{"z": 1}}, value, map[string]any{})
	run("Undefined names nothing", defaults, map[string]any{"m": map[string]any{"b": curly.Undefined{}}}, value, map[string]any{"a": "defaults", "b": "defaults", "c": "defaults"})
	run("nil names null", defaults, map[string]any{"m": map[string]any{"b": nil}}, value, map[string]any{"a": "defaults", "b": nil, "c": "defaults"})
	run("a named map is a layer", map[string]any{"m": layer{"a": 1}}, map[string]any{"m": map[string]string{"b": "2"}}, map[string]any{"v": map[string]any{"value": "x", "props": layer{"m": layer{"c": 3}}}}, map[string]any{"a": 1, "b": "2", "c": 3})
	run("near miss: a map keyed otherwise is none", map[string]any{"m": map[int]any{1: 1}}, map[string]any{"m": "b"}, map[string]any{"v": map[string]any{"value": "x", "props": map[string]any{"m": []any{"c"}}}}, map[string]any{})
	run("a wrapper's props holding Undefined", defaults, nil, map[string]any{"v": map[string]any{"value": "x", "props": curly.Undefined{}}}, map[string]any{"a": "defaults", "b": "defaults", "c": "defaults"})
	run("near miss: no wrapper, no wrapper props", nil, nil, map[string]any{"v": map[string]any{"value": "x", "props": map[string]any{"m": map[string]any{"c": 3}}, "other": 1}}, map[string]any{})

	got, _ := resolveWith(curly.Options{CustomModifiers: m, IgnoreWrappers: true}, "{{v:m}}", wrapped)
	expect(t, "IgnoreWrappers", got, "")

	if len(props) != 1 {
		t.Errorf("IgnoreWrappers: got %v", props)
	}
}

func TestExtensionsLocale(t *testing.T) {
	var got *string

	m := map[string]curly.Modifier{"m": func(in curly.Input) (any, error) {
		got = in.Locale

		if in.Locale != nil {
			*in.Locale = "written"
		}

		return "", nil
	}}

	p := curly.New(curly.Options{CustomModifiers: m})

	for _, locale := range []*string{nil, ptr(""), ptr("cs"), ptr(" EN_us ")} {
		var was string

		if locale != nil {
			was = *locale
		}

		p.Resolve("{{v:m}}", curly.Context{Payload: map[string]any{"v": "x"}, Locale: locale})

		switch {
		case locale == nil && got != nil:
			t.Errorf("none: got %q", *got)
		case locale != nil && (got == nil || got == locale || *locale != was):
			t.Errorf("%q: handed %v, left %q", was, got, *locale)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// An answer is the placeholder's result, so it spends the output and none of
// the read budget.
func TestExtensionsAnswerBudgets(t *testing.T) {
	half := strings.Repeat("x", curly.MaxReadLength/2+1)
	payload := map[string]any{"v": "x", "a": half, "w": "tail"}

	expect(t, "read", host(answering(half, nil), "{{v:m}}{{a:eq; y:Y; default:}}{{w}}", payload), half+"tail")
	expect(t, "near miss: a value read", host(nil, "{{a}}{{a:eq; y:Y; default:}}{{w}}", payload), half, curly.ReadLimit)
	expect(t, "output", host(answering(half+half, nil), "a{{v:m}}b", payload), "ab", curly.OutputLimit)
}

// A modifier may resolve another message, which spends budgets of its own.
func TestExtensionsReentrantResolve(t *testing.T) {
	var p *curly.Parser

	at := strings.Repeat("x", curly.MaxReadLength)

	// The outer resolution reads all its budget but the two bytes its two
	// values take, and each inner one all of its own.
	p = curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{"inner": func(in curly.Input) (any, error) {
		return p.Resolve("{{a:eq; y:Y; default:}}{{b}}", curly.Context{Payload: map[string]any{"a": at, "b": in.Value}}), nil
	}}})

	if got := p.Resolve("{{a:eq; y:Y; default:}}{{v:inner}}{{v:inner}}", curly.Context{Payload: map[string]any{"a": at[2:], "v": "v"}}); got != "vv" {
		t.Errorf("got %q", got)
	}
}
