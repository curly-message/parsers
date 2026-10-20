package curly_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"runtime"
	"runtime/debug"
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

// Once Resolve has returned, an input a modifier kept answers what a read
// produced during the call, and the empty string for what none did: nothing
// resolves, reports or runs host code after the call.
func TestExtensionsInputAfterTheCall(t *testing.T) {
	var kept, readAll curly.Input

	m := map[string]curly.Modifier{
		"m": func(in curly.Input) (any, error) {
			kept = in

			return in.Options[0].Value(), nil
		},
		"all": func(in curly.Input) (any, error) {
			readAll = in

			return in.Options[0].Value() + in.Options[1].Value() + in.Default(), nil
		},
	}

	calls := 0
	reports := 0

	p := curly.New(curly.Options{CustomModifiers: m, OnReport: func(curly.Report) { reports++ }})
	payload := map[string]any{"v": "x", "s": counted{calls: &calls}}

	got := p.Resolve("{{v:m; a:{{v}}; b:{{s}}{{v:nope; default:B}}; default:{{s}}{{w:nope; default:I}}}}", curly.Context{Payload: payload})

	if got != "x" || reports != 0 || calls != 0 {
		t.Fatalf("during: got %q, %d reports, String called %d times", got, reports, calls)
	}

	if a, b, d := kept.Options[0].Value(), kept.Options[1].Value(), kept.Default(); a != "x" || b != "" || d != "" || reports != 0 || calls != 0 {
		t.Errorf("after: %q %q %q, %d reports, String called %d times", a, b, d, reports, calls)
	}

	// The near miss: what the call read, it answers again after.
	got = p.Resolve("{{v:all; a:{{v}}; b:{{s}}{{v:nope; default:B}}; default:{{s}}{{w:nope; default:I}}}}", curly.Context{Payload: payload})

	if got != "xcountedBcountedI" || reports != 2 || calls != 1 {
		t.Fatalf("read during: got %q, %d reports, String called %d times", got, reports, calls)
	}

	if a, b, d := readAll.Options[0].Value(), readAll.Options[1].Value(), readAll.Default(); a != "x" || b != "countedB" || d != "countedI" || reports != 2 || calls != 1 {
		t.Errorf("read during, after: %q %q %q, %d reports, String called %d times", a, b, d, reports, calls)
	}

	var zero curly.Input

	if zero.Default() != "" || (curly.Option{}).Value() != "" {
		t.Error("the zero input answers")
	}
}

// waitingInResolve reports whether a goroutine waits inside Resolve for a
// read another one is making.
func waitingInResolve() bool {
	buf := make([]byte, 1<<20)
	stacks := string(buf[:runtime.Stack(buf, true)])

	for _, g := range strings.Split(stacks, "\n\n") {
		if strings.Contains(g, "sync.(*Cond).Wait") && strings.Contains(g, "curly.(*Parser).Resolve") && !strings.Contains(g, "curly.(*walk).host") {
			return true
		}
	}

	return false
}

// released is a gate whose String method returns at once.
func released() gate {
	open := make(chan struct{})
	close(open)

	return gate{calls: new(atomic.Int32), entered: make(chan struct{}), release: open}
}

// Resolve never waits for a read a modifier's goroutine is making when the
// call ends. Once the host code that read is in returns, the read finishes
// without starting host code, reporting or answering text, as a read after
// the call does.
func TestExtensionsResolveDoesNotWaitForReadsInFlight(t *testing.T) {
	g := gate{calls: new(atomic.Int32), entered: make(chan struct{}), release: make(chan struct{})}
	after := released()

	var reports atomic.Int32

	var kept curly.Input

	read := make(chan string, 1)

	m := map[string]curly.Modifier{"m": func(in curly.Input) (any, error) {
		kept = in

		go func() { read <- in.Options[0].Value() }()

		<-g.entered

		return "r", nil
	}}

	p := curly.New(curly.Options{CustomModifiers: m, OnReport: func(curly.Report) { reports.Add(1) }})
	done := make(chan string, 1)

	go func() {
		done <- p.Resolve("{{v:m; a:<{{s}}{{t}}{{v:nope}}>}}", curly.Context{Payload: map[string]any{"v": "x", "s": g, "t": after}})
	}()

	<-g.entered

	var got string

	for returned := false; !returned; {
		select {
		case got = <-done:
			returned = true
		default:
			if waitingInResolve() {
				close(g.release)
				t.Fatal("Resolve waits for a read a modifier's goroutine is making")
			}

			runtime.Gosched()
		}
	}

	if got != "r" || reports.Load() != 0 {
		t.Fatalf("got %q, %d reports", got, reports.Load())
	}

	close(g.release)

	if during := <-read; during != "" || kept.Options[0].Value() != "" || g.calls.Load() != 1 || after.calls.Load() != 0 || reports.Load() != 0 {
		t.Errorf("read %q, after %q, String called %d and %d times, %d reports", during, kept.Options[0].Value(), g.calls.Load(), after.calls.Load(), reports.Load())
	}

	// The near miss: a read the modifier waits for is part of the call, and
	// resolves, reports and answers its text again after.
	s := released()

	m["m"] = func(in curly.Input) (any, error) {
		kept = in

		go func() { read <- in.Options[0].Value() }()

		return <-read, nil
	}

	p = curly.New(curly.Options{CustomModifiers: m, OnReport: func(curly.Report) { reports.Add(1) }})

	if got := p.Resolve("{{v:m; a:<{{s}}{{v:nope}}>}}", curly.Context{Payload: map[string]any{"v": "x", "s": s}}); got != "<S>" || kept.Options[0].Value() != "<S>" || s.calls.Load() != 1 || reports.Load() != 1 {
		t.Errorf("got %q, after %q, String called %d times, %d reports", got, kept.Options[0].Value(), s.calls.Load(), reports.Load())
	}
}

var nilGateEntered, nilGateRelease chan struct{}

// nilGate is a gate on a nil pointer receiver, whose String method returns
// normally, so fmt would go on to print what follows it.
type nilGate struct{}

func (*nilGate) String() string {
	close(nilGateEntered)
	<-nilGateRelease

	return "N"
}

// Once Resolve has returned, a read whose host code returns after it reads
// nothing more of the payload, which the caller may then change; under -race
// this is the test that it does not read the map below.
func TestExtensionsReadInFlightLeavesThePayloadAfterTheCall(t *testing.T) {
	nilGateEntered, nilGateRelease = make(chan struct{}), make(chan struct{})
	written := map[int]int{1: 1}
	read := make(chan string, 1)

	m := map[string]curly.Modifier{"m": func(in curly.Input) (any, error) {
		go func() { read <- in.Options[0].Value() }()

		<-nilGateEntered

		return "r", nil
	}}

	p := curly.New(curly.Options{CustomModifiers: m})
	got := p.Resolve("{{v:m; a:{{w}}}}", curly.Context{Payload: map[string]any{"v": "x", "w": named{(*nilGate)(nil), written}}})

	close(nilGateRelease)

	written[2] = 2

	if during := <-read; got != "r" || during != "" {
		t.Errorf("got %q, read %q", got, during)
	}
}

// A modifier may read its options from goroutines of its own, which
// serialize on the resolution's state; under -race this is the test that
// they do, through option values that call host code of their own: a nested
// modifier and a String method. A text read once is answered again.
func TestExtensionsConcurrentReads(t *testing.T) {
	calls := 0

	m := map[string]curly.Modifier{
		"fan": func(in curly.Input) (any, error) {
			var wg sync.WaitGroup

			out := make([]string, 8)
			read := func(i int) string {
				if i%4 == 3 {
					return in.Default()
				}

				return in.Options[i%4].Value()
			}

			for i := range 4 {
				wg.Go(func() { out[i] = read(i) })
			}

			wg.Wait()

			for i := 4; i < 8; i++ {
				out[i] = read(i)
			}

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

// self is a parser whose modifier self resolves message through its Input
// against payload, until ask says to stop, with what it is handed and what
// it answers left to answer, and its reports collected.
func self(message string, payload map[string]any, ask func() bool, answer func(string, error) (any, error)) (*curly.Parser, *[]curly.Code) {
	var p *curly.Parser

	var codes []curly.Code

	p = curly.New(curly.Options{
		OnReport: func(r curly.Report) { codes = append(codes, r.Code) },
		CustomModifiers: map[string]curly.Modifier{"self": func(in curly.Input) (any, error) {
			if !ask() {
				return "x", nil
			}

			return answer(in.Resolve(p, message, curly.Context{Payload: payload}))
		}},
	})

	return p, &codes
}

func always() bool { return true }

// A modifier that resolves its own message through its Input holds
// MaxReentry resolutions in progress, and the one it asks for past them is
// refused into the modifier that asked: an error it lets pass fails it, so
// its placeholder takes the fallback chain and every resolution around it
// answers. Without the bound the chain ends the process at the stack set
// here, a fatal error rather than a failing test.
func TestExtensionsReentryLimit(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	const message = "<{{v:self; default:D}}>"

	payload := map[string]any{"v": "v"}
	nested := func(innermost string) string {
		return strings.Repeat("<", curly.MaxReentry) + innermost + strings.Repeat(">", curly.MaxReentry)
	}

	var refused []error

	p, codes := self(message, payload, always, func(got string, err error) (any, error) {
		if err != nil {
			refused = append(refused, err)
		}

		return got, err
	})

	if got := p.Resolve(message, curly.Context{Payload: payload}); got != nested("D") || len(refused) != 1 || !errors.Is(refused[0], curly.ErrReentryLimit) ||
		fmt.Sprint(*codes) != fmt.Sprint([]curly.Code{curly.FailedModifier}) {
		t.Errorf("refused: got %.40q (%d bytes) %v %v", got, len(got), refused, *codes)
	}

	// The modifier may handle the refusal as any error.
	p, codes = self(message, payload, always, func(got string, err error) (any, error) {
		if err != nil {
			return "H", nil
		}

		return got, nil
	})

	if got := p.Resolve(message, curly.Context{Payload: payload}); got != nested("H") || len(*codes) != 0 {
		t.Errorf("handled: got %.40q (%d bytes) %v", got, len(got), *codes)
	}

	// The count goes on through an Input on a goroutine of the modifier's
	// own all the same.
	var q *curly.Parser

	q = curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{"self": func(in curly.Input) (any, error) {
		var got string

		done := make(chan error)

		go func() {
			var err error
			got, err = in.Resolve(q, message, curly.Context{Payload: payload})
			done <- err
		}()

		if err := <-done; err != nil {
			return "G", nil
		}

		return got, nil
	}}})

	if got := q.Resolve(message, curly.Context{Payload: payload}); got != nested("G") {
		t.Errorf("on a goroutine: got %.40q (%d bytes)", got, len(got))
	}

	// Near miss: the MaxReentry-th resolution asks for none, so none is
	// refused.
	asked := 0
	p, codes = self(message, payload, func() bool { asked++; return asked < curly.MaxReentry }, func(got string, err error) (any, error) { return got, err })

	if got := p.Resolve(message, curly.Context{Payload: payload}); got != nested("x") || asked != curly.MaxReentry || len(*codes) != 0 {
		t.Errorf("near miss: got %.40q (%d bytes) after %d %v", got, len(got), asked, *codes)
	}
}

// The count is of the chain, not of the Input a resolution is asked for
// through: a modifier that resolves its message through an Input kept from
// further out, the outermost resolution's or one whose own resolution has
// returned, holds MaxReentry resolutions in progress all the same, and the
// one it asks for past them is refused.
func TestExtensionsReentryThroughAnOuterInput(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	const message = "<{{v:self; default:D}}>"

	payload := map[string]any{"v": "v"}
	want := strings.Repeat("<", curly.MaxReentry) + "D" + strings.Repeat(">", curly.MaxReentry)

	for _, keep := range []string{"the outermost", "a returned"} {
		var p *curly.Parser

		var kept *curly.Input

		var refused []error

		asked := 0
		p = curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{
			"keep": func(in curly.Input) (any, error) { kept = &in; return "", nil },
			"self": func(in curly.Input) (any, error) {
				asked++

				if kept == nil {
					kept = &in

					if keep == "a returned" {
						if _, err := in.Resolve(p, "{{v:keep}}", curly.Context{Payload: payload}); err != nil {
							return nil, err
						}
					}
				}

				got, err := kept.Resolve(p, message, curly.Context{Payload: payload})

				if err != nil {
					refused = append(refused, err)
				}

				return got, err
			},
		}})

		if got := p.Resolve(message, curly.Context{Payload: payload}); got != want || asked != curly.MaxReentry || len(refused) != 1 || !errors.Is(refused[0], curly.ErrReentryLimit) {
			t.Errorf("through %s Input: got %.40q (%d bytes) after %d, refused %v", keep, got, len(got), asked, refused)
		}
	}

	// Near miss: resolutions asked for one after another through the
	// outermost Input are never in progress at once, so none is refused.
	var p *curly.Parser

	var refused []error

	p = curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{"many": func(in curly.Input) (any, error) {
		for range 2 * curly.MaxReentry {
			if _, err := in.Resolve(p, "{{v}}", curly.Context{Payload: payload}); err != nil {
				refused = append(refused, err)
			}
		}

		return "x", nil
	}}})

	if got := p.Resolve("{{v:many}}", curly.Context{Payload: payload}); got != "x" || len(refused) != 0 {
		t.Errorf("one after another: got %q, refused %v", got, refused)
	}
}

// The count is of resolutions, never of the walk of one: each resolution of
// a message whose modifier asks for another from an option at MaxNesting
// counts as one more, however deep in its message it asks.
func TestExtensionsReentryThroughOptions(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	message := "{{v:self; default:D}}"

	for range curly.MaxNesting - 1 {
		message = "{{v:option; a:" + message + "}}"
	}

	payload := map[string]any{"v": "v"}

	var p *curly.Parser

	var codes []curly.Code

	asked := 0
	p = curly.New(curly.Options{
		OnReport: func(r curly.Report) { codes = append(codes, r.Code) },
		CustomModifiers: map[string]curly.Modifier{
			"option": func(in curly.Input) (any, error) { return in.Options[0].Value(), nil },
			"self": func(in curly.Input) (any, error) {
				asked++
				got, err := in.Resolve(p, message, curly.Context{Payload: payload})

				return "(" + got + ")", err
			},
		},
	})

	got := p.Resolve(message, curly.Context{Payload: payload})
	want := strings.Repeat("(", curly.MaxReentry-1) + "D" + strings.Repeat(")", curly.MaxReentry-1)

	if got != want || asked != curly.MaxReentry || fmt.Sprint(codes) != fmt.Sprint([]curly.Code{curly.FailedModifier}) {
		t.Errorf("got %.40q (%d bytes) after %d %v", got, len(got), asked, codes)
	}
}

// reentrant runs resolve when encoding/json writes it.
type reentrant struct{ resolve func() }

func (r *reentrant) MarshalJSON() ([]byte, error) {
	r.resolve()

	return []byte(`"x"`), nil
}

// reentrantText runs resolve when encoding/json writes it by AppendText,
// which encoding/json built on encoding/json/v2 calls.
type reentrantText struct{ resolve func() }

func (r *reentrantText) AppendText(b []byte) ([]byte, error) {
	r.resolve()

	return append(b, 'x'), nil
}

type chainNode struct {
	N *chainNode
	L any
}

// chainOf nests leaf in nodes chainNodes, so encoding/json meets it at depth
// 2*nodes+2 of the value the parser hands it.
func chainOf(nodes int, leaf any) *chainNode {
	root := &chainNode{L: leaf}

	for range nodes - 1 {
		root = &chainNode{N: root}
	}

	return root
}

// reentryThrough resolves a message whose modifier answers value, which
// holds a method encoding/json calls that resolves the message again
// through the modifier's Input: how many times the modifier was asked,
// how many times the method resolved, the resolutions it was refused and
// the reports.
func reentryThrough(value func(*reentrant) any) (got string, asked, called int, refused []error, codes []curly.Code) {
	const message = "{{v:self; default:D}}"

	payload := map[string]any{"v": "v"}

	var p *curly.Parser

	p = curly.New(curly.Options{
		OnReport: func(r curly.Report) { codes = append(codes, r.Code) },
		CustomModifiers: map[string]curly.Modifier{"self": func(in curly.Input) (any, error) {
			asked++

			return map[string]any{"a": value(&reentrant{func() {
				called++

				if _, err := in.Resolve(p, message, curly.Context{Payload: payload}); err != nil {
					refused = append(refused, err)
				}
			}})}, nil
		}},
	})

	return p.Resolve(message, curly.Context{Payload: payload}), asked, called, refused, codes
}

// A resolution asked for from a method encoding/json calls, as it writes a
// value the asking resolution converts, sits below every level above that
// method on the goroutine's stack. The parser hands encoding/json no value
// nested deeper than 64 levels, so each resolution of the chain counts as
// one: a chain through a method at that depth holds MaxReentry resolutions
// within the stack set here, and a value holding one a level deeper is not
// handed over.
func TestExtensionsReentryThroughEncodingJSON(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	if _, asked, _, refused, codes := reentryThrough(func(r *reentrant) any { return chainOf(31, r) }); asked != curly.MaxReentry || len(refused) != 1 || !errors.Is(refused[0], curly.ErrReentryLimit) || len(codes) != 0 {
		t.Errorf("at 64 levels: %d resolutions, refused %v %v", asked, refused, codes)
	}

	// Near miss: a level deeper, the value is not handed over, and the
	// method that would resolve is never called.
	if got, asked, called, refused, codes := reentryThrough(func(r *reentrant) any { return chainOf(31, []*reentrant{r}) }); got != "D" || asked != 1 || called != 0 || len(refused) != 0 ||
		fmt.Sprint(codes) != fmt.Sprint([]curly.Code{curly.UnserializableValue}) {
		t.Errorf("at 65 levels: got %q after %d resolutions, %d calls, refused %v %v", got, asked, called, refused, codes)
	}

	// And so with AppendText, where encoding/json calls it, from Go 1.27.
	if written, _ := json.Marshal([]any{&reentrantText{func() {}}}); string(written) == `["x"]` {
		if _, asked, _, refused, codes := reentryThrough(func(r *reentrant) any { return chainOf(31, &reentrantText{r.resolve}) }); asked != curly.MaxReentry || len(refused) != 1 || !errors.Is(refused[0], curly.ErrReentryLimit) || len(codes) != 0 {
			t.Errorf("AppendText at 64 levels: %d resolutions, refused %v %v", asked, refused, codes)
		}
	}

	if got, asked, called, refused, codes := reentryThrough(func(r *reentrant) any { return chainOf(31, []*reentrantText{{r.resolve}}) }); got != "D" || asked != 1 || called != 0 || len(refused) != 0 ||
		fmt.Sprint(codes) != fmt.Sprint([]curly.Code{curly.UnserializableValue}) {
		t.Errorf("AppendText at 65 levels: got %q after %d resolutions, %d calls, refused %v %v", got, asked, called, refused, codes)
	}
}

type (
	hiddenInner struct {
		X any `json:"b"`
	}
	hiddenOuter struct {
		X int `json:"a"`
		hiddenInner
	}
)

// A method in a field a field of the same Go name further out hides from
// reflect.VisibleFields is one encoding/json calls all the same, and is
// measured as deep as it sits.
func TestExtensionsReentryThroughHiddenField(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	if got, asked, called, refused, codes := reentryThrough(func(r *reentrant) any { return hiddenOuter{1, hiddenInner{chainOf(30, []*reentrant{r})}} }); got != "D" || asked != 1 || called != 0 || len(refused) != 0 ||
		fmt.Sprint(codes) != fmt.Sprint([]curly.Code{curly.UnserializableValue}) {
		t.Errorf("at 65 levels: got %q after %d resolutions, %d calls, refused %v %v", got, asked, called, refused, codes)
	}

	// Near miss: at 64 levels it is handed over, and the chain through it
	// holds MaxReentry resolutions.
	if _, asked, _, refused, codes := reentryThrough(func(r *reentrant) any { return hiddenOuter{1, hiddenInner{chainOf(30, r)}} }); asked != curly.MaxReentry || len(refused) != 1 || !errors.Is(refused[0], curly.ErrReentryLimit) || len(codes) != 0 {
		t.Errorf("at 64 levels: %d resolutions, refused %v %v", asked, refused, codes)
	}
}

// parked is a value encoding/json writes past levels of plain structure
// before it calls the method that blocks.
type parked struct {
	Deep *chainNode
	Leaf *blocking
}

type blocking struct{ parked, release chan struct{} }

func (b *blocking) MarshalJSON() ([]byte, error) {
	b.parked <- struct{}{}
	<-b.release

	return []byte(`"x"`), nil
}

// A resolution counts what its chain holds in progress before it and
// nothing of how deep a read of theirs is on another goroutine: a modifier
// whose goroutines are parked inside encoding/json, past a value nested a
// level short of the 64 the parser hands it at most, resolves through its
// Input short of the bound.
func TestExtensionsReentryBesideEncodingJSON(t *testing.T) {
	for _, readers := range []int{1, 2} {
		parkedAt, release := make(chan struct{}), make(chan struct{})
		payload := map[string]any{"v": "v"}
		message := "{{v:m"

		for i := range readers {
			name := fmt.Sprint("d", i)
			payload[name] = map[string]any{"a": &parked{chainOf(30, nil), &blocking{parkedAt, release}}}
			message += fmt.Sprintf("; %d:{{%s}}", i, name)
		}

		message += "}}"

		var p *curly.Parser

		var err error

		p = curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{"m": func(in curly.Input) (any, error) {
			var wg sync.WaitGroup

			for _, o := range in.Options {
				wg.Go(func() { o.Value() })
			}

			for range in.Options {
				<-parkedAt
			}

			var got string

			got, err = in.Resolve(p, "{{v}}", curly.Context{Payload: payload})

			close(release)
			wg.Wait()

			return got, err
		}}})

		if got := p.Resolve(message, curly.Context{Payload: payload}); got != "v" || err != nil {
			t.Errorf("%d parked: got %q %v", readers, got, err)
		}
	}
}

// A resolution asked for where none of its chain is in progress counts as
// the first and is never refused: one Parser.Resolve starts, even from a
// modifier at MaxReentry, one through the zero Input, and one through an
// Input whose resolution has returned.
func TestExtensionsReentryFromNone(t *testing.T) {
	const message = "<{{v:self; default:D}}>"

	payload := map[string]any{"v": "v"}

	var p *curly.Parser

	var kept curly.Input

	var fresh string

	var deepest error

	asked := 0
	p = curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{"self": func(in curly.Input) (any, error) {
		asked++

		if asked < curly.MaxReentry {
			return in.Resolve(p, message, curly.Context{Payload: payload})
		}

		kept = in
		_, deepest = in.Resolve(p, "{{v}}", curly.Context{Payload: payload})
		fresh = p.Resolve("{{v}}", curly.Context{Payload: payload})

		return "x", nil
	}}})

	if got := p.Resolve(message, curly.Context{Payload: payload}); len(got) != 2*curly.MaxReentry+1 || !errors.Is(deepest, curly.ErrReentryLimit) || fresh != "v" {
		t.Errorf("at the bound: got %.40q (%d bytes) %v %q", got, len(got), deepest, fresh)
	}

	if got, err := kept.Resolve(p, "{{v}}", curly.Context{Payload: payload}); got != "v" || err != nil {
		t.Errorf("kept: got %q %v", got, err)
	}

	if got, err := (curly.Input{}).Resolve(p, "{{v}}", curly.Context{Payload: payload}); got != "v" || err != nil {
		t.Errorf("zero: got %q %v", got, err)
	}
}

// keeping is a parser whose modifier k keeps the input it is handed and
// answers k, and whose modifier m reads the first option of the input kept
// last, which it forgets, or answers e where none is kept; its reports are
// collected.
func keeping() (*curly.Parser, *[]curly.Code) {
	var kept []curly.Input

	var codes []curly.Code

	return curly.New(curly.Options{
		OnReport: func(r curly.Report) { codes = append(codes, r.Code) },
		CustomModifiers: map[string]curly.Modifier{
			"k": func(in curly.Input) (any, error) {
				kept = append(kept, in)

				return "k", nil
			},
			"m": func(curly.Input) (any, error) {
				if len(kept) == 0 {
					return "e", nil
				}

				last := kept[len(kept)-1]
				kept = kept[:len(kept)-1]

				return last.Options[0].Value(), nil
			},
		},
	}), &codes
}

// A read of an option a placeholder kept from elsewhere renders it as one
// resolution of the chain in progress more: the resolution holds the first,
// and each kept option the chain reads one more, so MaxReentry-1 of them
// render, and the read past them is refused into the modifier that made it.
// Without the bound the chain ends the process at the stack set here, a
// fatal error rather than a failing test.
func TestExtensionsKeptReads(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(8 << 20))

	payload := map[string]any{"x": "x"}

	for _, c := range []struct {
		kept  int
		want  string
		codes []curly.Code
	}{
		{curly.MaxReentry - 1, strings.Repeat("k", curly.MaxReentry-1) + "e", nil},
		{curly.MaxReentry, strings.Repeat("k", curly.MaxReentry), []curly.Code{curly.FailedModifier}},
		{20000, strings.Repeat("k", 20000), []curly.Code{curly.FailedModifier}},
	} {
		p, codes := keeping()

		if got := p.Resolve(strings.Repeat("{{x:k; o:{{x:m}}}}", c.kept)+"{{x:m}}", curly.Context{Payload: payload}); got != c.want || fmt.Sprint(*codes) != fmt.Sprint(c.codes) {
			t.Errorf("%d kept: got %.40q (%d bytes) %v", c.kept, got, len(got), *codes)
		}
	}
}

// A placeholder's options read from inside the render of another of them
// count as the kept options of several placeholders do: the modifier's own
// read of its first option renders on the walk, and each further one counts
// as one resolution more.
func TestExtensionsKeptReadsThroughOptions(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	resolveWithOptions := func(n int) (string, []curly.Code) {
		var kept curly.Input

		var codes []curly.Code

		next := 0
		p := curly.New(curly.Options{
			OnReport: func(r curly.Report) { codes = append(codes, r.Code) },
			CustomModifiers: map[string]curly.Modifier{
				"k": func(in curly.Input) (any, error) {
					kept = in

					return in.Options[0].Value(), nil
				},
				"m": func(curly.Input) (any, error) {
					if next++; next == len(kept.Options) {
						return "e", nil
					}

					return kept.Options[next].Value(), nil
				},
			},
		})

		var message strings.Builder

		message.WriteString("{{x:k")

		for i := range n {
			fmt.Fprintf(&message, "; o%d:{{x:m}}", i)
		}

		message.WriteString("}}")

		return p.Resolve(message.String(), curly.Context{Payload: map[string]any{"x": "x"}}), codes
	}

	if got, codes := resolveWithOptions(curly.MaxReentry); got != "e" || len(codes) != 0 {
		t.Errorf("%d options: got %q %v", curly.MaxReentry, got, codes)
	}

	if got, codes := resolveWithOptions(curly.MaxReentry + 1); got != "" || fmt.Sprint(codes) != fmt.Sprint([]curly.Code{curly.FailedModifier}) {
		t.Errorf("%d options: got %q %v", curly.MaxReentry+1, got, codes)
	}
}

// atTheBound resolves message as the MaxReentry-th resolution in progress of
// its chain, asked for through an Input, with modifiers beside hold, which
// asks for them, and probe, which asks for one more and answers ! where it is
// refused; its reports are collected.
func atTheBound(modifiers map[string]curly.Modifier, message string, payload map[string]any) (string, []curly.Code, error) {
	var p *curly.Parser

	var codes []curly.Code

	var err error

	asked := 0
	all := map[string]curly.Modifier{
		"hold": func(in curly.Input) (any, error) {
			if asked++; asked < curly.MaxReentry-1 {
				return in.Resolve(p, "{{v:hold}}", curly.Context{Payload: payload})
			}

			var got string

			got, err = in.Resolve(p, message, curly.Context{Payload: payload})

			return got, err
		},
		"probe": func(in curly.Input) (any, error) {
			if _, err := in.Resolve(p, "", curly.Context{}); errors.Is(err, curly.ErrReentryLimit) {
				return "!", nil
			}

			return "?", nil
		},
	}

	maps.Copy(all, modifiers)

	p = curly.New(curly.Options{OnReport: func(r curly.Report) { codes = append(codes, r.Code) }, CustomModifiers: all})

	return p.Resolve("{{v:hold}}", curly.Context{Payload: payload}), codes, err
}

// The near miss: a modifier's reads of its own options and default render on
// the walk of its resolution and count nothing, one after another or nested
// to MaxNesting, so at MaxReentry resolutions in progress they all render.
func TestExtensionsOwnReadsAtTheBound(t *testing.T) {
	nested := "{{v:probe}}"

	for range curly.MaxNesting - 1 {
		nested = "{{v:own; a:" + nested + "}}"
	}

	got, codes, err := atTheBound(map[string]curly.Modifier{
		"all": func(in curly.Input) (any, error) {
			return in.Options[0].Value() + in.Options[1].Value() + in.Options[2].Value() + in.Default(), nil
		},
		"own": func(in curly.Input) (any, error) { return "(" + in.Options[0].Value() + ")", nil },
	}, "{{v:all; a:{{v:probe}}; b:B; c:{{v}}; default:D}}"+nested, map[string]any{"v": "v"})

	if want := "!BvD" + strings.Repeat("(", curly.MaxNesting-1) + "!" + strings.Repeat(")", curly.MaxNesting-1); got != want || len(codes) != 0 || err != nil {
		t.Errorf("got %q %v %v, want %q", got, codes, err, want)
	}
}

// A modifier's own read made while another of its texts renders counts, even
// on the goroutine the modifier was asked on: the parser cannot tell it from
// a read on a goroutine of the modifier's own. At the bound it is refused, and
// the refusal fails the value whose method is running.
func TestExtensionsOwnReadWhileATextRenders(t *testing.T) {
	two := func(g gate) map[string]curly.Modifier {
		return map[string]curly.Modifier{"two": func(in curly.Input) (any, error) {
			read := make(chan string, 1)

			go func() { read <- in.Options[0].Value() }()

			<-g.entered

			mine := in.Options[1].Value()

			close(g.release)

			return <-read + "|" + mine, nil
		}}
	}
	message := "{{v:two; a:{{s}}; b:B; default:F}}"
	fresh := func() gate {
		return gate{calls: new(atomic.Int32), entered: make(chan struct{}), release: make(chan struct{})}
	}

	g := fresh()

	if got, codes, err := atTheBound(two(g), message, map[string]any{"v": "v", "s": g}); got != "|" || fmt.Sprint(codes) != fmt.Sprint([]curly.Code{curly.UnserializableValue}) || err != nil {
		t.Errorf("at the bound: got %q %v %v", got, codes, err)
	}

	// Near miss: short of the bound, both reads render.
	g = fresh()

	if got := host(two(g), message, map[string]any{"v": "v", "s": g}); got.out != "S|B" || len(got.codes) != 0 {
		t.Errorf("short of it: got %+v", got)
	}
}

// A read refused at the bound leaves what it read unproduced, so the option
// renders where its own modifier reads it on the walk.
func TestExtensionsRefusedReadRecordsNothing(t *testing.T) {
	var kept curly.Input

	modifiers := map[string]curly.Modifier{
		"k": func(in curly.Input) (any, error) {
			kept = in

			return in.Options[0].Value() + "|" + in.Options[1].Value(), nil
		},
		"m": func(curly.Input) (any, error) { return kept.Options[1].Value(), nil },
	}
	payload := map[string]any{"v": "v"}

	if got, codes, err := atTheBound(modifiers, "{{v:k; a:{{v:m}}; b:B}}", payload); got != "|B" || fmt.Sprint(codes) != fmt.Sprint([]curly.Code{curly.FailedModifier}) || err != nil {
		t.Errorf("at the bound: got %q %v %v", got, codes, err)
	}

	// Near miss: short of the bound, the read renders the option, and the
	// modifier's own read answers it again.
	p := curly.New(curly.Options{CustomModifiers: modifiers})

	if got := p.Resolve("{{v:k; a:{{v:m}}; b:B}}", curly.Context{Payload: payload}); got != "B|B" {
		t.Errorf("short of it: got %q", got)
	}
}

// A refusal fails the modifier whose read was refused and none around it: the
// modifier whose option holds that placeholder answers with its fallback.
func TestExtensionsRefusalFailsTheReader(t *testing.T) {
	var kept curly.Input

	got, codes, err := atTheBound(map[string]curly.Modifier{
		"keep": func(in curly.Input) (any, error) { kept = in; return "", nil },
		"anc":  func(in curly.Input) (any, error) { return "A" + in.Options[0].Value(), nil },
		"desc": func(curly.Input) (any, error) { return kept.Options[0].Value(), nil },
	}, "{{v:keep; o:O}}{{v:anc; a:[{{v:desc; default:F}}]}}", map[string]any{"v": "v"})

	if got != "A[F]" || fmt.Sprint(codes) != fmt.Sprint([]curly.Code{curly.FailedModifier}) || err != nil {
		t.Errorf("got %q %v %v", got, codes, err)
	}
}

// Refusals pending together are each taken off by the host code that returns
// first after them, and only by it. Two refusals of one modifier, with host
// code begun and returned between them, fail that modifier once and leave
// nothing to fail the modifier around it; a refusal of a modifier, then one
// of a modifier its own option runs, fail each of them.
func TestExtensionsRefusalsPendingTogether(t *testing.T) {
	var kept curly.Input

	modifiers := map[string]curly.Modifier{
		"keep": func(in curly.Input) (any, error) { kept = in; return "", nil },
		"up":   upper,
		"twice": func(in curly.Input) (any, error) {
			return kept.Options[0].Value() + in.Options[0].Value() + kept.Options[0].Value(), nil
		},
		"anc":  func(in curly.Input) (any, error) { return kept.Options[0].Value() + "K" + in.Options[0].Value(), nil },
		"desc": func(curly.Input) (any, error) { return kept.Options[0].Value(), nil },
	}
	payload := map[string]any{"v": "v"}

	for _, c := range []struct {
		message string
		want    string
		codes   []curly.Code
	}{
		{"{{v:keep; o:O}}{{v:twice; a:{{v:up}}; default:F}}", "F", []curly.Code{curly.FailedModifier}},
		{"{{v:keep; o:O}}{{v:anc; a:[{{v:desc; default:G}}]; default:F}}", "F", []curly.Code{curly.FailedModifier, curly.FailedModifier}},
	} {
		if got, codes, err := atTheBound(modifiers, c.message, payload); got != c.want || fmt.Sprint(codes) != fmt.Sprint(c.codes) || err != nil {
			t.Errorf("%s: got %q %v %v, want %q %v", c.message, got, codes, err, c.want, c.codes)
		}
	}
}

// A modifier that reaches, through state it keeps, an option from inside
// that option's own render is answered the empty string at once, and its
// placeholder fails: the option renders once, with that placeholder's
// fallback, and a read after it answers that.
func TestExtensionsSelfReachingRead(t *testing.T) {
	var kept curly.Input

	var codes []curly.Code

	p := curly.New(curly.Options{
		OnReport: func(r curly.Report) { codes = append(codes, r.Code) },
		CustomModifiers: map[string]curly.Modifier{
			"k": func(in curly.Input) (any, error) {
				kept = in
				in.Options[0].Value()

				return "k", nil
			},
			"r": func(curly.Input) (any, error) { return kept.Options[0].Value(), nil },
		},
	})

	if got := p.Resolve("{{x:k; o:{{x:r; default:D}}}}{{x:r}}", curly.Context{Payload: map[string]any{"x": "x"}}); got != "kD" || fmt.Sprint(codes) != fmt.Sprint([]curly.Code{curly.FailedModifier}) {
		t.Errorf("got %q %v", got, codes)
	}
}

// A read, on the goroutine that called Resolve, of an option a goroutine of
// the modifier's own is rendering is answered the empty string at once
// rather than waiting, and fails the modifier. The goroutine's own read
// finishes once Resolve has returned, and answers the empty string too.
func TestExtensionsReadOfATextInFlight(t *testing.T) {
	g := gate{calls: new(atomic.Int32), entered: make(chan struct{}), release: make(chan struct{})}
	read := make(chan string, 1)

	var second string

	m := map[string]curly.Modifier{"m": func(in curly.Input) (any, error) {
		go func() { read <- in.Options[0].Value() }()

		<-g.entered
		second = in.Options[0].Value()

		return "r", nil
	}}

	got := host(m, "{{v:m; a:{{s}}; default:D}}", map[string]any{"v": "x", "s": g})

	close(g.release)

	if first := <-read; got.out != "D" || second != "" || first != "" || fmt.Sprint(got.codes) != fmt.Sprint([]curly.Code{curly.FailedModifier}) {
		t.Errorf("got %q %v, read %q and %q", got.out, got.codes, first, second)
	}
}

// held is a set of modifiers that each wait, once asked, until the test
// releases them, then answer their name without its first letter.
type held map[string]struct{ entered, release chan struct{} }

func holding(names ...string) held {
	h := held{}

	for _, name := range names {
		h[name] = struct{ entered, release chan struct{} }{make(chan struct{}), make(chan struct{})}
	}

	return h
}

func (h held) modifiers(k curly.Modifier) map[string]curly.Modifier {
	modifiers := map[string]curly.Modifier{"k": k}

	for name, m := range h {
		modifiers[name] = func(curly.Input) (any, error) {
			close(m.entered)
			<-m.release

			return name[1:], nil
		}
	}

	return modifiers
}

// reading reads an option on a goroutine of its own, once the modifier its
// render asks has entered.
func (h held) reading(in curly.Input, i int, name string) chan string {
	read := make(chan string, 1)

	go func() { read <- in.Options[i].Value() }()

	<-h[name].entered

	return read
}

// resolveHolding resolves the message with k and the held modifiers, and
// lists each report as its code and text: reports of one resolution may
// come from several goroutines.
func resolveHolding(h held, k curly.Modifier, message string) (string, []string) {
	var mu sync.Mutex

	var reports []string

	p := curly.New(curly.Options{CustomModifiers: h.modifiers(k), OnReport: func(r curly.Report) {
		mu.Lock()
		reports = append(reports, string(r.Code)+" "+r.Text)
		mu.Unlock()
	}})

	return p.Resolve(message, curly.Context{Payload: map[string]any{"x": "x"}}), reports
}

// A refused read fails, once, the host code of its chain that returns first
// among the code running when it was refused, on any goroutine: here a
// modifier of an option being rendered, rather than k, which made the read,
// and neither code that started after the refusal nor code further out.
func TestExtensionsRefusalFailsOnce(t *testing.T) {
	h := holding("m1", "m2")

	got, reports := resolveHolding(h, func(in curly.Input) (any, error) {
		a := h.reading(in, 0, "m1")
		refused := in.Options[0].Value()
		b := h.reading(in, 1, "m2")
		close(h["m1"].release)
		first := <-a
		close(h["m2"].release)

		return "k[" + refused + "|" + first + "|" + <-b + "]", nil
	}, "{{x:k; a:{{x:m1}}; b:{{x:m2}}; default:F}}")

	if want := []string{"failed-modifier {{x:m1}}"}; got != "k[||2]" || !slices.Equal(reports, want) {
		t.Errorf("got %q %q, want %q %q", got, reports, "k[||2]", want)
	}

	// The code running when a read is refused is told apart from the code
	// running when an earlier one was: mB, which started between the two
	// refusals, returns first after the second and fails for it, and mC,
	// which started after the first and returns last, fails for neither.
	h = holding("mA", "mB", "mC")

	got, reports = resolveHolding(h, func(in curly.Input) (any, error) {
		a := h.reading(in, 0, "mA")
		first := in.Options[0].Value()
		b := h.reading(in, 1, "mB")
		close(h["mA"].release)
		va := <-a
		c := h.reading(in, 2, "mC")
		second := in.Options[2].Value()
		close(h["mB"].release)
		vb := <-b
		close(h["mC"].release)

		return "k[" + first + "|" + va + "|" + second + "|" + vb + "|" + <-c + "]", nil
	}, "{{x:k; a:{{x:mA}}; b:{{x:mB}}; c:{{x:mC}}; default:F}}")

	if want := []string{"failed-modifier {{x:mA}}", "failed-modifier {{x:mB}}"}; got != "k[||||C]" || !slices.Equal(reports, want) {
		t.Errorf("got %q %q, want %q %q", got, reports, "k[||||C]", want)
	}
}

// renderHook is a Stringer that runs f.
type renderHook struct{ f func() string }

func (r renderHook) String() string { return r.f() }

// A read made on a goroutine of the modifier's own, of an option the walk is
// rendering, is answered the empty string at once: here the render waits for
// that answer, so a read that waited for the render would never return. The
// refusal fails the value's method, the host code running then that returns
// first, so the option renders without it.
func TestExtensionsReadOfATextTheWalkRenders(t *testing.T) {
	inRender := make(chan struct{})
	read := make(chan string, 1)

	first := "unread"

	payload := map[string]any{"v": "x", "s": renderHook{func() string {
		close(inRender)
		first = <-read

		return "S"
	}}}

	got := host(map[string]curly.Modifier{"m": func(in curly.Input) (any, error) {
		go func() {
			<-inRender
			read <- in.Options[0].Value()
		}()

		return "r[" + in.Options[0].Value() + "]", nil
	}}, "{{v:m; a:{{s}}; default:D}}", payload)

	if got.out != "r[]" || first != "" || fmt.Sprint(got.codes) != fmt.Sprint([]curly.Code{curly.UnserializableValue}) {
		t.Errorf("got %q %v, the goroutine read %q", got.out, got.codes, first)
	}
}

// keptByMethods is the Input the methods below read an option of.
var keptByMethods curly.Input

type (
	keptStringer struct{}
	nilStringer  struct{}
	nilError     struct{}
	nilFormatter struct{}
)

func (keptStringer) String() string { return "S" + keptByMethods.Options[0].Value() }

func (*nilStringer) String() string { return "S" + keptByMethods.Options[0].Value() }

func (*nilError) Error() string { return "S" + keptByMethods.Options[0].Value() }

func (*nilFormatter) Format(s fmt.State, _ rune) {
	_, _ = fmt.Fprint(s, "S"+keptByMethods.Options[0].Value())
}

// A refused read fails a value's method that returned as it fails one that
// panicked, and the value describes nothing, on a nil pointer receiver as on
// any other: unlike a method that panics on a nil pointer receiver, which
// prints "<nil>" as fmt does.
func TestExtensionsRefusedReadInAMethodOnANilReceiver(t *testing.T) {
	modifiers := map[string]curly.Modifier{"k": func(in curly.Input) (any, error) {
		keptByMethods = in

		return "k[" + in.Options[0].Value() + "]", nil
	}}

	for _, value := range []any{keptStringer{}, (*nilStringer)(nil), (*nilError)(nil), (*nilFormatter)(nil)} {
		got := host(modifiers, "{{x:k; o:{{s; default:F}}}}", map[string]any{"x": "x", "s": value})

		if got.out != "k[F]" || fmt.Sprint(got.codes) != fmt.Sprint([]curly.Code{curly.UnserializableValue}) {
			t.Errorf("%T: got %q %v", value, got.out, got.codes)
		}
	}
}
