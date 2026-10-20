package curly_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"text/template"

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
	p := curly.New(curly.Options{CustomModifiers: modifiers, ModifierDefaults: defaults, OnReport: func(curly.Report, curly.Resolver) {}})

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

	p := curly.New(curly.Options{CustomModifiers: m, OnReport: func(curly.Report, curly.Resolver) { reports++ }})
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
	synctest.Test(t, func(t *testing.T) {
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

		p := curly.New(curly.Options{CustomModifiers: m, OnReport: func(curly.Report, curly.Resolver) { reports.Add(1) }})
		done := make(chan string, 1)

		go func() {
			done <- p.Resolve("{{v:m; a:<{{s}}{{t}}{{v:nope}}>}}", curly.Context{Payload: map[string]any{"v": "x", "s": g, "t": after}})
		}()

		// The read is held in String until Resolve has returned, so a Resolve
		// that waited for it would leave every goroutine of the bubble
		// blocked, which synctest fails at once.
		got := <-done

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

		p = curly.New(curly.Options{CustomModifiers: m, OnReport: func(curly.Report, curly.Resolver) { reports.Add(1) }})

		if got := p.Resolve("{{v:m; a:<{{s}}{{v:nope}}>}}", curly.Context{Payload: map[string]any{"v": "x", "s": s}}); got != "<S>" || kept.Options[0].Value() != "<S>" || s.calls.Load() != 1 || reports.Load() != 1 {
			t.Errorf("got %q, after %q, String called %d times, %d reports", got, kept.Options[0].Value(), s.calls.Load(), reports.Load())
		}
	})
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

// Goroutines of one modifier that read its options at once: each option is
// produced once, and a read arriving while another goroutine's read is
// inside host code producing the same text is refused, since the parser
// cannot tell it from a read that production itself makes. So each read is
// answered the text or the empty string, a read after them all the text,
// and text no host code runs for, which is produced under the lock, always.
func TestExtensionsConcurrentReads(t *testing.T) {
	calls := 0
	want := []string{"X", "countedcounted", "N", "I"}
	read := func(in curly.Input, i int) string {
		if i == 3 {
			return in.Default()
		}

		return in.Options[i].Value()
	}

	var out [8]string

	m := map[string]curly.Modifier{
		"fan": func(in curly.Input) (any, error) {
			var wg sync.WaitGroup

			for i := range out {
				wg.Go(func() { out[i] = read(in, i%4) })
			}

			wg.Wait()

			after := make([]string, 4)

			for i := range after {
				after[i] = read(in, i)
			}

			return strings.Join(after, ","), nil
		},
		"upper": upper,
	}

	var mu sync.Mutex

	codes := map[curly.Code]int{}

	p := curly.New(curly.Options{CustomModifiers: m, OnReport: func(r curly.Report, _ curly.Resolver) {
		mu.Lock()
		codes[r.Code]++
		mu.Unlock()
	}})

	got := p.Resolve("{{v:fan; a:{{v:upper}}; b:{{c}}{{c}}; c:{{v:nope; default:N}}; default:{{w; default:I}}}}", curly.Context{Payload: map[string]any{"v": "x", "c": counted{calls: &calls}}})

	if got != strings.Join(want, ",") || !maps.Equal(codes, map[curly.Code]int{curly.UnknownModifier: 1}) || calls != 1 {
		t.Errorf("got %q, %v, String called %d times", got, codes, calls)
	}

	for i, text := range out {
		if text != want[i%4] && (text != "" || i%4 == 3) {
			t.Errorf("read %d answered %q", i, text)
		}
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

// A value converts once, and a read host code makes never waits for one: the
// parser cannot tell a conversion beside the read from one the read is
// inside. The second of two goroutines whose options name one value,
// arriving while the first is inside the value's String method, is refused:
// the empty string, not the placeholder's fallback chain. Near miss: read
// once the first has returned, the value is its text.
func TestExtensionsConcurrentConversion(t *testing.T) {
	g := gate{calls: new(atomic.Int32), entered: make(chan struct{}), release: make(chan struct{})}

	m := map[string]curly.Modifier{"both": func(in curly.Input) (any, error) {
		first := make(chan string)

		go func() { first <- in.Options[0].Value() }()

		<-g.entered

		second := in.Options[1].Value()

		close(g.release)

		return <-first + " " + second + " " + in.Options[2].Value(), nil
	}}

	got := curly.New(curly.Options{CustomModifiers: m}).Resolve("{{v:both; a:{{s}}; b:{{s; default:D}}; c:{{s}}}}", curly.Context{Payload: map[string]any{"v": "x", "s": g}})

	if got != "S  S" || g.calls.Load() != 1 {
		t.Errorf("got %q, String called %d times", got, g.calls.Load())
	}
}

func TestExtensionsProps(t *testing.T) {
	type layer map[string]any

	var props map[string]any

	written := true

	m := map[string]curly.Modifier{"m": func(in curly.Input) (any, error) {
		props = in.Props

		// A write, which no modifier should make, reaches no layer.
		if written {
			in.Props["written"] = true
		}

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

			// No map is made where nothing is held under the name.
			if (want == nil) != (props == nil) {
				t.Errorf("%s: handed a nil map: %v", what, props == nil)
			}
		}
	}

	value := map[string]any{"v": "x"}
	defaults := map[string]any{"m": map[string]any{"a": "defaults", "b": "defaults", "c": "defaults"}, "other": map[string]any{"z": 1}}
	call := map[string]any{"m": map[string]any{"b": "call", "c": "call"}}
	wrapped := map[string]any{"v": map[string]any{"value": "x", "props": map[string]any{"m": map[string]any{"c": "wrapper"}}}}

	run("none", nil, nil, value, nil)
	run("each layer per property", defaults, call, wrapped, map[string]any{"a": "defaults", "b": "call", "c": "wrapper"})
	run("own name only", map[string]any{"other": map[string]any{"z": 1}}, map[string]any{"other": map[string]any{"z": 1}}, value, nil)
	run("Undefined names nothing", defaults, map[string]any{"m": map[string]any{"b": curly.Undefined{}}}, value, map[string]any{"a": "defaults", "b": "defaults", "c": "defaults"})
	run("nil names null", defaults, map[string]any{"m": map[string]any{"b": nil}}, value, map[string]any{"a": "defaults", "b": nil, "c": "defaults"})
	run("a named map is a layer", map[string]any{"m": layer{"a": 1}}, map[string]any{"m": map[string]string{"b": "2"}}, map[string]any{"v": map[string]any{"value": "x", "props": layer{"m": layer{"c": 3}}}}, map[string]any{"a": 1, "b": "2", "c": 3})
	// Near miss: something held under the name that is no layer composes to
	// an empty map.
	run("near miss: a map keyed otherwise is none", map[string]any{"m": map[int]any{1: 1}}, map[string]any{"m": "b"}, map[string]any{"v": map[string]any{"value": "x", "props": map[string]any{"m": []any{"c"}}}}, map[string]any{})
	run("a wrapper's props holding Undefined", defaults, nil, map[string]any{"v": map[string]any{"value": "x", "props": curly.Undefined{}}}, map[string]any{"a": "defaults", "b": "defaults", "c": "defaults"})
	run("near miss: no wrapper, no wrapper props", nil, nil, map[string]any{"v": map[string]any{"value": "x", "props": map[string]any{"m": map[string]any{"c": 3}}, "other": 1}}, nil)

	// With nothing to hold there is no map to write to, so the write is a
	// panic, which fails the modifier as any does.
	expect(t, "a write where nothing is composed", host(m, "{{v:m; default:D}}", value), "D", curly.FailedModifier)

	written = false

	expect(t, "IgnoreWrappers", host(m, "{{v:m}}", wrapped), "")

	p := curly.New(curly.Options{CustomModifiers: m, IgnoreWrappers: true, ModifierDefaults: defaults})
	p.Resolve("{{v:m}}", curly.Context{Payload: wrapped})

	if fmt.Sprint(props) != "map[a:defaults b:defaults c:defaults]" {
		t.Errorf("IgnoreWrappers: got %v", props)
	}
}

// Props are composed once per resolution of the layers every placeholder
// shares, and once more for each run of a modifier's placeholders whose
// wrappers hold one layer under its name. Every placeholder a composition
// serves is handed the same map, so a layer is walked once however many
// placeholders read it: what a resolution allocates for one placeholder
// more is the same whether the layer holds one property or thousands.
func TestExtensionsPropsAreComposedOnce(t *testing.T) {
	var handed []uintptr

	var seen []string

	printed := true
	record := func(props map[string]any) {
		handed = append(handed, reflect.ValueOf(props).Pointer())

		if printed {
			seen = append(seen, fmt.Sprint(props))
		}
	}

	p := curly.New(curly.Options{
		ModifierDefaults: map[string]any{"m": map[string]any{"a": "defaults"}, "o": map[string]any{"a": "other"}},
		CustomModifiers: map[string]curly.Modifier{
			"m": func(in curly.Input) (any, error) {
				record(in.Props)

				return "", nil
			},
			"n": func(in curly.Input) (any, error) {
				record(in.Props)

				return "", nil
			},
			"o": func(in curly.Input) (any, error) {
				record(in.Props)

				return "", nil
			},
		},
	})

	layer := map[string]any{"b": "wrapper"}
	wrapper := func(layer any) map[string]any {
		return map[string]any{"value": 1, "props": map[string]any{"m": layer, "o": layer}}
	}
	payload := map[string]any{"a": wrapper(layer), "b": wrapper(layer), "c": wrapper(map[string]any{"b": "wrapper"}), "d": 1, "e": wrapper("no layer")}

	p.Resolve("{{a:m}}{{a:m}}{{a:o}}{{b:m}}{{c:m}}{{d:m}}{{e:m}}{{d:o}}{{d:m}}{{a:n}}", curly.Context{Payload: payload})

	want := []string{
		"map[a:defaults b:wrapper]", "map[a:defaults b:wrapper]", "map[a:other b:wrapper]", "map[a:defaults b:wrapper]", "map[a:defaults b:wrapper]",
		"map[a:defaults]", "map[a:defaults]", "map[a:other]", "map[a:defaults]", "map[]",
	}

	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", seen, want)
	}

	// The same layer, through one wrapper or two, is one composition, with
	// another modifier asked in between or not; near misses: an equal layer
	// that is another map, no layer, and another modifier's name are each
	// their own, and what is no layer is none.
	same := [][2]int{{0, 1}, {1, 3}, {5, 6}, {6, 8}}
	other := [][2]int{{0, 2}, {3, 4}, {4, 5}, {5, 7}, {8, 9}}

	for _, pair := range same {
		if handed[pair[0]] != handed[pair[1]] {
			t.Errorf("placeholders %v were handed two maps: %v", pair, handed)
		}
	}

	for _, pair := range other {
		if handed[pair[0]] == handed[pair[1]] {
			t.Errorf("placeholders %v were handed one map: %v", pair, handed)
		}
	}

	if handed[9] != 0 {
		t.Errorf("a map was made of no layer: %v", handed)
	}

	// The next resolution composes its own.
	first := handed[0]
	handed = nil
	p.Resolve("{{a:m}}", curly.Context{Payload: payload})

	if handed[0] == first {
		t.Error("a composition outlived its resolution")
	}

	// Each of many layers in one resolution is its own composition, shared
	// by the run of placeholders over it.
	handed, seen = nil, nil
	many, message := map[string]any{}, ""

	for i := range 20 {
		many[fmt.Sprint("v", i)] = wrapper(map[string]any{"b": i})
		message += fmt.Sprintf("{{v%d:m}}{{v%d:m}}", i, i)
	}

	p.Resolve(message+"{{v0:m}}", curly.Context{Payload: many})

	for i := range 20 {
		if seen[2*i] != fmt.Sprintf("map[a:defaults b:%d]", i) || handed[2*i+1] != handed[2*i] || (i > 0 && handed[2*i] == handed[2*i-1]) {
			t.Fatalf("layer %d of 20: %v, the maps %v", i, seen[2*i], handed)
		}
	}

	if seen[40] != seen[0] {
		t.Errorf("a layer met again: got %v, want %v", seen[40], seen[0])
	}

	if raceEnabled {
		return
	}

	printed = false

	perPlaceholder := func(width int) float64 {
		layer := make(map[string]any, width)

		for i := range width {
			layer[fmt.Sprint("k", i)] = i
		}

		context := curly.Context{Payload: map[string]any{"a": wrapper(layer)}}
		allocs := func(placeholders int) float64 {
			message := strings.Repeat("{{a:m}}", placeholders)

			return testing.AllocsPerRun(5, func() {
				handed = handed[:0]
				p.Resolve(message, context)
			})
		}

		return (allocs(300) - allocs(100)) / 200
	}

	if narrow, wide := perPlaceholder(1), perPlaceholder(5000); wide > narrow+1 {
		t.Errorf("a placeholder more allocates %v times over a layer of one property and %v over one of 5000", narrow, wide)
	}
}

// What a wrapper's layer is composed into is a copy of the layers beneath
// it, and a resolution keeps the one it composed last: over a wide layer,
// what it holds once it has composed for 200 wrappers is a few copies of
// the layer, where keeping each held 200. A copy is weighed by what making
// one allocates, and the heap is read after a collection, so what the rest
// of the process frees meanwhile only lowers what is read.
func TestExtensionsPropsOfManyWrappersAreNotKept(t *testing.T) {
	const width, wrappers = 5000, 200

	wide := make(map[string]any, width)

	for i := range width {
		wide[fmt.Sprint("k", i)] = i
	}

	heap := func() int64 {
		var stats runtime.MemStats

		runtime.GC()
		runtime.ReadMemStats(&stats)

		return int64(stats.HeapAlloc)
	}

	var live int64

	p := curly.New(curly.Options{
		ModifierDefaults: map[string]any{"m": wide},
		CustomModifiers: map[string]curly.Modifier{
			"m": func(curly.Input) (any, error) { return "", nil },
			"measure": func(curly.Input) (any, error) {
				live = heap()

				return "", nil
			},
		},
	})

	held := func(wrappers int) int64 {
		payload := map[string]any{}

		var message strings.Builder

		for i := range wrappers {
			payload[fmt.Sprint("v", i)] = map[string]any{"value": 1, "props": map[string]any{"m": map[string]any{"b": i}}}
			fmt.Fprintf(&message, "{{v%d:m}}", i)
		}

		message.WriteString("{{v0:measure}}")

		source := message.String()
		before := heap()
		p.Resolve(source, curly.Context{Payload: payload})

		return live - before
	}

	var before, after runtime.MemStats

	runtime.ReadMemStats(&before)
	clone := maps.Clone(wide)
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(clone)

	copied := int64(after.TotalAlloc - before.TotalAlloc)

	if many := held(wrappers); copied < width || many > 20*copied {
		t.Errorf("the resolution held %d bytes after %d wrappers, where a copy of the layer is %d", many, wrappers, copied)
	}
}

// What a resolution that composes nothing and calls no host code allocates
// in bytes, on a 64-bit platform: what it keeps for the host code a
// modifier may run, and for the properties it may compose, is allocated
// where a resolution first needs it, so a message that asks neither pays for
// neither. Each ceiling is what Go 1.26 and Go 1.27 allocate on amd64 and
// 48 bytes, less than anything a walk could come to hold again. Near miss: a parser
// with a host modifier and defaults for it registered allocates the same
// for a message that does not ask it.
func TestExtensionsCostAPlainMessageNothing(t *testing.T) {
	if raceEnabled || strconv.IntSize != 64 {
		t.Skip("the ceilings are those of a 64-bit build without the race detector")
	}

	bytes := func(p *curly.Parser, message string, context curly.Context) uint64 {
		const runs = 200

		var before, after runtime.MemStats

		p.Resolve(message, context)

		// The count is the process's, so the least of a few rounds is the
		// one nothing else allocated in.
		least := uint64(math.MaxUint64)

		for range 5 {
			runtime.ReadMemStats(&before)

			for range runs {
				p.Resolve(message, context)
			}

			runtime.ReadMemStats(&after)

			least = min(least, (after.TotalAlloc-before.TotalAlloc)/runs)
		}

		return least
	}

	plain := curly.New(curly.Options{})
	hosted := curly.New(curly.Options{
		ModifierDefaults: map[string]any{"m": map[string]any{"a": 1.0}},
		CustomModifiers:  map[string]curly.Modifier{"m": func(in curly.Input) (any, error) { return in.Value, nil }},
	})

	for _, c := range []struct {
		message string
		payload map[string]any
		ceiling uint64
	}{
		{"Hello!", nil, 272 + 48},
		{"Hello, {{name}}!", map[string]any{"name": "Alice"}, 672 + 48},
		{"{{n}} {{n; 1:file; default:files;}}", map[string]any{"n": 5.0}, 1256 + 48},
	} {
		context := curly.Context{Payload: c.payload}

		for _, p := range []*curly.Parser{plain, hosted} {
			if n := bytes(p, c.message, context); n > c.ceiling {
				t.Errorf("%q: %v bytes, ceiling %v", c.message, n, c.ceiling)
			}
		}
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

// A resolution asked for through Parser.Resolve starts a chain of its own,
// counted with no other, whatever host code asks for it: a modifier that
// resolves that way stands more than MaxReentry resolutions deep and is
// refused none. Such a modifier bounds its own recursion, as this one does;
// one that asks through its Input is counted (TestExtensionsReentryLimit).
func TestExtensionsParserResolveStartsAChain(t *testing.T) {
	const depth = curly.MaxReentry + 5

	var p *curly.Parser

	calls := 0
	context := curly.Context{Payload: map[string]any{"x": 1}}
	p = curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{"t": func(curly.Input) (any, error) {
		if calls++; calls == depth {
			return "x", nil
		}

		return p.Resolve("[{{x:t}}]", context), nil
	}}})

	if got := p.Resolve("[{{x:t}}]", context); got != strings.Repeat("[", depth)+"x"+strings.Repeat("]", depth) {
		t.Errorf("got %d bytes after %d calls", len(got), calls)
	}
}

// self is a parser whose modifier self resolves message through its Input
// against payload, until ask says to stop, with what it is handed and what
// it answers left to answer, and its reports collected.
func self(message string, payload map[string]any, ask func() bool, answer func(string, error) (any, error)) (*curly.Parser, *[]curly.Code) {
	var p *curly.Parser

	var codes []curly.Code

	p = curly.New(curly.Options{
		OnReport: func(r curly.Report, _ curly.Resolver) { codes = append(codes, r.Code) },
		CustomModifiers: map[string]curly.Modifier{"self": func(in curly.Input) (any, error) {
			if !ask() {
				return "x", nil
			}

			return answer(in.Resolver().Resolve(message, curly.Context{Payload: payload}))
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
	q := curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{"self": func(in curly.Input) (any, error) {
		var got string

		done := make(chan error)

		go func() {
			var err error
			got, err = in.Resolver().Resolve(message, curly.Context{Payload: payload})
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
						if _, err := in.Resolver().Resolve("{{v:keep}}", curly.Context{Payload: payload}); err != nil {
							return nil, err
						}
					}
				}

				got, err := kept.Resolver().Resolve(message, curly.Context{Payload: payload})

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
			if _, err := in.Resolver().Resolve("{{v}}", curly.Context{Payload: payload}); err != nil {
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
// a message whose modifier asks for another from an option of its own at
// MaxNesting counts as one more, however deep in its message it asks.
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
		OnReport: func(r curly.Report, _ curly.Resolver) { codes = append(codes, r.Code) },
		CustomModifiers: map[string]curly.Modifier{
			"option": func(in curly.Input) (any, error) { return in.Options[0].Value(), nil },
			"self": func(in curly.Input) (any, error) {
				asked++
				got, err := in.Resolver().Resolve(message, curly.Context{Payload: payload})

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

	p := curly.New(curly.Options{
		OnReport: func(r curly.Report, _ curly.Resolver) { codes = append(codes, r.Code) },
		CustomModifiers: map[string]curly.Modifier{"self": func(in curly.Input) (any, error) {
			asked++

			return map[string]any{"a": value(&reentrant{func() {
				called++

				if _, err := in.Resolver().Resolve(message, curly.Context{Payload: payload}); err != nil {
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

// A resolution counts the resolutions in progress before it and never the
// levels a read of theirs stands at on another goroutine: a modifier whose
// goroutines are parked inside encoding/json, past a value nested a level
// short of the 64 the parser hands it at most, resolves through its Input,
// each read counting as one resolution at most.
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

			got, err = in.Resolver().Resolve("{{v}}", curly.Context{Payload: payload})

			close(release)
			wg.Wait()

			return got, err
		}}})

		if got := p.Resolve(message, curly.Context{Payload: payload}); got != "v" || err != nil {
			t.Errorf("%d parked: got %q %v", readers, got, err)
		}
	}
}

// keeping is the modifiers of a chain of kept reads: keep keeps the first
// option it is handed and answers k, and read takes the option kept last and
// answers its value, or e where none is kept. again reads once more the
// option read took last.
func keeping() (map[string]curly.Modifier, *[]curly.Option) {
	var kept []curly.Option

	var last curly.Option

	return map[string]curly.Modifier{
		"keep": func(in curly.Input) (any, error) {
			kept = append(kept, in.Options[0])

			return "k", nil
		},
		"read": func(curly.Input) (any, error) {
			if len(kept) == 0 {
				return "e", nil
			}

			last = kept[len(kept)-1]
			kept = kept[:len(kept)-1]

			return last.Value(), nil
		},
		"again": func(curly.Input) (any, error) { return last.Value(), nil },
		"own":   func(in curly.Input) (any, error) { return in.Options[0].Value() + in.Default(), nil },
	}, &kept
}

// An option a modifier kept and another placeholder's modifier reads renders
// at the depth of the placeholder that wrote it, whatever read it, so a
// message two levels deep stands as many such reads one inside another as it
// has placeholders. Each counts as a resolution in progress while it
// renders: with the one Parser.Resolve started, MaxReentry-1 of them stand
// inside one another, and the read past them is refused, which answers the
// empty string and fails nothing: the modifier that made it goes on with
// it, and so does each read around it. A message of 20 000 placeholders
// ends there as well, where uncounted it took a level of the goroutine's
// stack for each.
func TestExtensionsKeptReads(t *testing.T) {
	const flat = "{{x:read; default:D}}"

	// The same read made from a placeholder at MaxNesting, below modifiers
	// that each read their own option and default.
	deep := flat

	for range curly.MaxNesting - 2 {
		deep = "{{x:own; a:" + deep + "; default:}}"
	}

	payload := map[string]any{"x": 1}

	for _, c := range []struct {
		what, body, tail string
		n                int
		out              string
		codes            []curly.Code
	}{
		{"none kept", flat, "{{x:read}}", 0, "e", nil},
		{"a few", flat, "{{x:read}}", 5, "e", nil},
		{"at the bound", flat, "{{x:read}}", curly.MaxReentry - 1, "e", nil},
		{"one past the bound", flat, "{{x:read}}", curly.MaxReentry, "", nil},
		{"20 000 placeholders", flat, "{{x:read}}", 20000, "", nil},
		{"near miss: own reads at MaxNesting count nothing", deep, "{{x:read}}", curly.MaxReentry - 1, "e", nil},
		{"one past the bound from MaxNesting", deep, "{{x:read}}", curly.MaxReentry, "", nil},
		// The refused read gave its count back and recorded nothing: the
		// option it was refused is rendered by the next read of it, whose
		// own read finds the five still kept.
		{"a refused read leaves nothing", flat, "{{x:read}}{{x:again}}", curly.MaxReentry + 5, "e", nil},
	} {
		m, _ := keeping()
		got := host(m, strings.Repeat("{{x:keep; o:"+c.body+"}}", c.n)+c.tail, payload)

		expect(t, c.what, got, strings.Repeat("k", c.n)+c.out, c.codes...)
	}

	// Near miss: reads that each return before the next are never in
	// progress at once, however many they are.
	m, kept := keeping()
	m["all"] = func(curly.Input) (any, error) {
		var out strings.Builder

		for _, o := range *kept {
			out.WriteString(o.Value())
		}

		return out.String(), nil
	}

	const n = 2 * curly.MaxReentry

	expect(t, "one after another", host(m, strings.Repeat("{{x:keep; o:{{x}}}}", n)+"{{x:all}}", payload), strings.Repeat("k", n)+strings.Repeat("1", n))
}

// A modifier reading its own options is the walk only as the last host code
// called: read from inside one of them by another placeholder's modifier,
// its other options are kept reads, and count. The first is its own, so with
// the resolution itself MaxReentry options stand inside one another.
func TestExtensionsKeptReadsOfOnePlaceholder(t *testing.T) {
	for _, c := range []struct {
		n     int
		out   string
		codes []curly.Code
	}{
		{curly.MaxReentry, "e", nil},
		{curly.MaxReentry + 1, "", nil},
	} {
		var kept []curly.Option

		m := map[string]curly.Modifier{
			"all": func(in curly.Input) (any, error) {
				kept = in.Options[:len(in.Options)-1]

				return in.Options[len(in.Options)-1].Value(), nil
			},
			"read": func(curly.Input) (any, error) {
				if len(kept) == 0 {
					return "e", nil
				}

				o := kept[len(kept)-1]
				kept = kept[:len(kept)-1]

				return o.Value(), nil
			},
		}

		expect(t, fmt.Sprint(c.n, " options"), host(m, "{{x:all"+strings.Repeat("; o:{{x:read; default:D}}", c.n)+"}}", map[string]any{"x": 1}), c.out, c.codes...)
	}
}

// taking is a value whose String method reads the option kept last.
type taking struct {
	kept *[]curly.Option
	id   int
}

func (v taking) String() string {
	if len(*v.kept) == 0 {
		return "e"
	}

	o := (*v.kept)[len(*v.kept)-1]
	*v.kept = (*v.kept)[:len(*v.kept)-1]

	return o.Value()
}

// A kept read counts whatever host code makes it, a method of a value
// among them, and a refused one fails nothing: the method goes on with the
// empty string, and the value converts to what it answers.
func TestExtensionsKeptReadsFromAValue(t *testing.T) {
	for _, c := range []struct {
		n     int
		out   string
		codes []curly.Code
	}{
		{curly.MaxReentry - 1, "e", nil},
		{curly.MaxReentry, "", nil},
	} {
		m, kept := keeping()
		payload := map[string]any{"x": 1}

		var message strings.Builder

		for i := range c.n + 1 {
			payload[fmt.Sprint("s", i)] = taking{kept, i}
		}

		for i := range c.n {
			fmt.Fprintf(&message, "{{x:keep; o:{{s%d; default:D}}}}", i)
		}

		fmt.Fprintf(&message, "{{s%d}}", c.n)
		expect(t, fmt.Sprint(c.n, " kept"), host(m, message.String(), payload), strings.Repeat("k", c.n)+c.out, c.codes...)
	}
}

// A modifier stays current through the host code that runs beneath its
// call, a value's conversion method among them, so that method reading the
// modifier's own option reads it for nothing, with every resolution of the
// chain in progress. Near miss: the same method reading the option of a
// modifier that has returned counts, and is refused there.
func TestExtensionsOwnReadsFromAConversion(t *testing.T) {
	for _, c := range []struct {
		what, out string
		own       bool
	}{
		{"own", "a[O]", true},
		{"returned", "a[]", false},
	} {
		m, kept := keeping()

		var h0, h curly.Input

		m["h0"] = func(in curly.Input) (any, error) {
			h0 = in

			return "a", nil
		}
		m["h"] = func(in curly.Input) (any, error) {
			h = in
			s, err := in.Resolver().Resolve("{{g}}", curly.Context{Payload: map[string]any{"g": renders{func() string {
				if c.own {
					return h.Options[0].Value()
				}

				return h0.Options[0].Value()
			}}}})

			return "[" + s + "]", err
		}

		// With the one Parser.Resolve started, n+1 resolutions are in
		// progress as h is called, and the one h asks for is the last
		// MaxReentry admits.
		n := curly.MaxReentry - 2
		message, payload := readsInside(n, "{{x:h0; o:P}}{{x:h; o:O}}", kept)
		expect(t, c.what, host(m, message, payload), strings.Repeat("k", n)+c.out)
	}
}

// readsInside is a message of n kept reads, each of which counts, standing
// one inside another, the innermost rendering innermost, and its payload.
func readsInside(n int, innermost string, kept *[]curly.Option) (string, map[string]any) {
	payload := map[string]any{"x": 1}

	var message strings.Builder

	fmt.Fprintf(&message, "{{x:keep; o:%s}}", innermost)

	for i := 1; i < n; i++ {
		fmt.Fprintf(&message, "{{x:keep; o:{{s%d}}}}", i)
	}

	for i := 1; i <= n; i++ {
		payload[fmt.Sprint("s", i)] = taking{kept, i}
	}

	fmt.Fprintf(&message, "{{s%d}}", n)

	return message.String(), payload
}

// The modifier current in a chain is the one called last that has not
// returned, on whatever goroutine: one called after another on a second
// goroutine takes the current from it, so that one's read of its own option
// counts as a kept read does, and is refused past the bound. A modifier
// called beneath its call, here in a resolution it asks for, takes it the
// same way, and that resolution counts as well.
func TestExtensionsOwnReadsBesideAnotherGoroutine(t *testing.T) {
	for _, c := range []struct {
		what, out string
		n         int
		beneath   bool
	}{
		{"beside", "A", curly.MaxReentry - 3, false},
		{"beside, past the bound", "", curly.MaxReentry - 2, false},
		{"beneath", "A", curly.MaxReentry - 4, true},
		{"beneath, past the bound", "", curly.MaxReentry - 3, true},
	} {
		m, kept := keeping()
		aCalled, bCalled, aRead := make(chan struct{}), make(chan struct{}), make(chan struct{})

		var a curly.Input

		m["pair"] = func(in curly.Input) (any, error) {
			out := make([]string, 2)

			var reads sync.WaitGroup

			reads.Go(func() { out[0] = in.Options[0].Value() })
			<-aCalled
			reads.Go(func() { out[1] = in.Options[1].Value() })
			reads.Wait()

			return out[0], nil
		}
		m["a"] = func(in curly.Input) (any, error) {
			a = in
			close(aCalled)
			<-bCalled

			defer close(aRead)

			if c.beneath {
				return in.Resolver().Resolve("{{x:c}}", curly.Context{Payload: map[string]any{"x": 1}})
			}

			return in.Options[0].Value(), nil
		}
		m["b"] = func(curly.Input) (any, error) {
			close(bCalled)
			<-aRead

			return "", nil
		}
		m["c"] = func(curly.Input) (any, error) { return a.Options[0].Value(), nil }

		message, payload := readsInside(c.n, "{{x:pair; o:{{x:a; o:A}}; p:{{x:b}}}}", kept)
		expect(t, c.what, host(m, message, payload), strings.Repeat("k", c.n)+c.out)
	}
}

// handingOver resolves n placeholders whose modifier hands its Input to a
// goroutine of the test's, which reads the option of each in turn from
// inside the one before: how deep the reads stood, what the innermost read
// answered, and the reports.
func handingOver(n int) (got string, deepest int32, innermost any, reports []curly.Report) {
	handed, entered, done := make(chan curly.Input), make(chan struct{}), make(chan struct{})

	var depth, deepestRead atomic.Int32

	var innermostRead atomic.Value

	var mu sync.Mutex

	m := map[string]curly.Modifier{
		"hand": func(in curly.Input) (any, error) {
			select {
			case handed <- in:
			case <-done:
				return "k", nil
			}

			select {
			case <-entered:
			case <-done:
			}

			return "k", nil
		},
		"inside": func(curly.Input) (any, error) {
			deepestRead.Store(max(deepestRead.Load(), depth.Add(1)))
			defer depth.Add(-1)

			entered <- struct{}{}

			in, more := <-handed

			if !more {
				innermostRead.Store("e")

				return "e", nil
			}

			text := in.Options[0].Value()

			if depth.Load() == deepestRead.Load() {
				innermostRead.Store(text)
			}

			return text, nil
		},
	}

	go func() {
		defer close(done)

		(<-handed).Options[0].Value()
	}()

	p := curly.New(curly.Options{CustomModifiers: m, OnReport: func(r curly.Report, _ curly.Resolver) {
		mu.Lock()
		defer mu.Unlock()

		reports = append(reports, r)
	}})
	got = p.Resolve(strings.Repeat("{{x:hand; o:{{x:inside}}}}", n), curly.Context{Payload: map[string]any{"x": 1}})
	close(handed)
	<-done

	return got, deepestRead.Load(), innermostRead.Load(), reports
}

// A read counts on whatever goroutine host code makes it. A modifier that
// hands its Input to a goroutine of its own has that goroutine read the
// option of each placeholder in turn from inside the one before: the first
// is read beneath no host code and counts nothing, and each after it is made
// by the modifier inside, while the modifier that handed it over is the
// current one where it was called. Such a read is that modifier's own, and counts
// nothing, for MaxNesting of them beneath each resolution or counted read of
// the chain, and counts past that, so with the resolution itself fewer than
// (MaxNesting+1)*MaxReentry of them stand inside one another, however many
// placeholders hand one over. The read past them answers the empty string,
// is not recorded and fails nothing. Near miss: three times MaxReentry of
// them are all answered.
func TestExtensionsKeptReadsAcrossGoroutines(t *testing.T) {
	for _, c := range []struct {
		what      string
		n         int
		deepest   [2]int32
		innermost string
	}{
		{"past the bound", 10 * curly.MaxReentry, [2]int32{curly.MaxNesting * curly.MaxReentry, (curly.MaxNesting + 1) * curly.MaxReentry}, ""},
		{"near miss: within it", 3 * curly.MaxReentry, [2]int32{3 * curly.MaxReentry, 3*curly.MaxReentry + 1}, "e"},
	} {
		got, deepest, innermost, reports := handingOver(c.n)

		if got != strings.Repeat("k", c.n) || deepest < c.deepest[0] || deepest >= c.deepest[1] || innermost != c.innermost {
			t.Errorf("%s: got %d bytes, %d reads deep, the innermost answering %q", c.what, len(got), deepest, innermost)
		}

		if len(reports) != 0 {
			t.Errorf("%s: got %v, want the modifier whose read was refused to go on", c.what, reports)
		}
	}
}

// A modifier that reads its options at once from goroutines of its own
// stays current only until one of those options calls a modifier, so each
// read after that counts, as does the read each modifier nested in those
// options makes of its own option beneath it, once another of its level was
// called after it: with k such reads standing in each, (MaxReentry-1)/k
// options read at once are answered every one, 499 with one level of them
// and 249 with three. Past that the bound refuses reads, how many is the
// schedule's to say, each answers the empty string, and nothing fails.
func TestExtensionsOwnReadsFromGoroutines(t *testing.T) {
	for _, levels := range []int{1, 3} {
		for _, n := range []int{(curly.MaxReentry - 1) / (levels + 1), curly.MaxReentry + 500} {
			ownReadsFromGoroutines(t, n, levels)
		}
	}
}

func ownReadsFromGoroutines(t *testing.T, n, levels int) {
	var mu sync.Mutex

	cond := sync.NewCond(&mu)
	arrived, gone := make([]int, levels+1), 0

	// Each modifier of a level waits until every read has called one of
	// that level or been refused, so none of them is current as it reads.
	barrier := func(level int) {
		mu.Lock()
		defer mu.Unlock()

		arrived[level]++
		cond.Broadcast()

		for arrived[level]+gone < n {
			cond.Wait()
		}
	}

	m := map[string]curly.Modifier{
		"all": func(in curly.Input) (any, error) {
			out := make([]string, len(in.Options))

			var reads sync.WaitGroup

			for i, o := range in.Options {
				reads.Go(func() {
					out[i] = o.Value()

					// A read answered nothing is a read refused, which never
					// reached the levels below the one it was refused at.
					if out[i] == "" {
						mu.Lock()
						gone++
						cond.Broadcast()
						mu.Unlock()
					}
				})
			}

			reads.Wait()

			return strings.Join(out, ""), nil
		},
		"deep": func(in curly.Input) (any, error) {
			barrier(levels)

			return in.Value, nil
		},
	}

	option := "{{x:deep}}"

	for level := levels - 1; level >= 0; level-- {
		name := "hold" + strconv.Itoa(level)
		option = "{{x:" + name + "; a:" + option + "}}"
		m[name] = func(in curly.Input) (any, error) {
			barrier(level)

			return in.Options[0].Value(), nil
		}
	}

	var codes []curly.Code

	p := curly.New(curly.Options{CustomModifiers: m, OnReport: func(r curly.Report, _ curly.Resolver) {
		mu.Lock()
		defer mu.Unlock()

		codes = append(codes, r.Code)
	}})

	got := p.Resolve("{{x:all"+strings.Repeat("; o:"+option, n)+"}}", curly.Context{Payload: map[string]any{"x": 1}})

	if got != strings.Repeat("1", len(got)) || len(got) > n || (levels+1)*n < curly.MaxReentry && len(got) != n || len(codes) != 0 {
		t.Errorf("%d levels: got %d bytes of %d, reports %v", levels, len(got), n, codes)
	}
}

// A read refused during a resolution a modifier asked for answers the empty
// string, and fails nothing in the message that asks it or in the one that
// asked for the resolution. Near miss: one read fewer is answered.
func TestExtensionsRefusedReadInANestedResolution(t *testing.T) {
	for _, c := range []struct {
		n       int
		tail    string
		reports string
	}{
		{curly.MaxReentry - 2, "q[e]", "[]"},
		{curly.MaxReentry - 1, "q[]", "[]"},
	} {
		var kept []curly.Option

		var reports []string

		var p *curly.Parser

		take := func() curly.Option {
			o := kept[len(kept)-1]
			kept = kept[:len(kept)-1]

			return o
		}

		p = curly.New(curly.Options{
			OnReport: func(r curly.Report, _ curly.Resolver) {
				reports = append(reports, fmt.Sprint(r.Code, " in ", r.ID, ": ", r.Text))
			},
			CustomModifiers: map[string]curly.Modifier{
				"keep": func(in curly.Input) (any, error) {
					kept = append(kept, in.Options[0])

					return "k", nil
				},
				"read": func(in curly.Input) (any, error) {
					switch len(kept) {
					case 0:
						return "e", nil
					case 1:
						return in.Resolver().Resolve("{{x:q}}", curly.Context{Payload: map[string]any{"x": 1}, ID: "inner"})
					}

					return take().Value(), nil
				},
				"q": func(curly.Input) (any, error) { return "q[" + take().Value() + "]", nil },
			},
		})

		got := p.Resolve(strings.Repeat("{{x:keep; o:{{x:read}}}}", c.n)+"{{x:read}}", curly.Context{Payload: map[string]any{"x": 1}, ID: "outer"})

		if got != strings.Repeat("k", c.n)+c.tail || fmt.Sprint(reports) != c.reports {
			t.Errorf("%d kept: got %q after the kept, reports %v, want %q and %v", c.n, got[min(len(got), c.n):], reports, c.tail, c.reports)
		}
	}
}

// renders is a value whose String method reads a default.
type renders struct{ read func() string }

func (v renders) String() string { return v.read() }

// What is being produced can be produced no second time, and a read host
// code makes cannot wait for it, since the parser cannot tell a read beside
// the production from one the production itself makes. So a read of it is
// refused at once on whatever goroutine: it answers the empty string and
// fails nothing, and the host code that made it, a modifier or a method of a
// value, goes on with it. Near miss: the same read once the text is
// produced is answered it.
func TestExtensionsReadOfWhatIsBeingProduced(t *testing.T) {
	var kept curly.Input

	var inner, later, beside string

	var done chan struct{}

	own := func(read func(curly.Input) string) curly.Modifier {
		return func(in curly.Input) (any, error) {
			kept = in
			text := read(in)
			later = read(in)

			return text, nil
		}
	}

	option := func(in curly.Input) string { return in.Options[0].Value() }
	chain := func(in curly.Input) string { return in.Default() }

	reading := func(read func(curly.Input) string, apart bool) map[string]curly.Modifier {
		return map[string]curly.Modifier{
			"own": own(read),
			// The modifier reads from a goroutine of its own, on whose stack
			// the rendering then is.
			"apart": own(func(in curly.Input) string {
				var text string

				var reads sync.WaitGroup

				reads.Go(func() { text = read(in) })
				reads.Wait()

				return text
			}),
			"again": func(curly.Input) (any, error) {
				if !apart {
					inner = read(kept)

					return "a", nil
				}

				go func() {
					defer close(done)

					beside = read(kept)
				}()

				<-done

				return "a", nil
			},
		}
	}

	for _, c := range []struct {
		what, message string
		read          func(curly.Input) string
		apart         bool
		out           string
		codes         []curly.Code
	}{
		{"an option", "{{x:own; o:<{{x:again; default:D}}>}}", option, false, "<a>", nil},
		{"an inline default", "{{x:own; default:<{{x:again; default:D}}>}}", chain, false, "<a>", nil},
		{"an option read on a goroutine", "{{x:apart; o:<{{x:again; default:D}}>}}", option, false, "<a>", nil},
		{"from another goroutine", "{{x:own; o:<{{x:again; default:D}}>}}", option, true, "<a>", nil},
		{"a default from another goroutine", "{{x:own; default:<{{x:again; default:D}}>}}", chain, true, "<a>", nil},
	} {
		inner, later, beside, done = "", "", "", make(chan struct{})

		if c.apart {
			inner = c.out
		} else {
			beside = c.out
			close(done)
		}

		expect(t, c.what, host(reading(c.read, c.apart), c.message, map[string]any{"x": 1}), c.out, c.codes...)
		<-done

		if c.apart != (inner == c.out) || later != c.out || c.apart != (beside == "") {
			t.Errorf("%s: the read inside answered %q, the read after %q, the read beside %q", c.what, inner, later, beside)
		}
	}

	// The payload's default, read as a link of the chain, by a method that
	// reads the chain it is a link of: the method goes on with the empty
	// string, so the value converts to what it answers.
	m := map[string]curly.Modifier{"own": own(chain)}
	payload := map[string]any{"x": 1, "default": renders{func() string {
		inner = kept.Default()

		return "linked"
	}}}

	inner = "unread"

	expect(t, "a link of the chain", host(m, "{{x:own; default:I}}", payload), "linked")

	if inner != "" || later != "linked" {
		t.Errorf("a link of the chain: the read inside answered %q, the read after %q", inner, later)
	}

	// A goroutine a modifier inside the text being produced started is not
	// the goroutine producing that text, and its read of it is refused at
	// once all the same, beneath a modifier or not. It records nothing, so
	// the production goes on and the read after it is answered the text.
	for _, c := range []struct {
		what, inner, beside string
		modifier            bool
	}{
		{"inside it on another goroutine, in a modifier", "", "r", true},
		{"inside it on another goroutine", "unread", "", false},
	} {
		inner, later, beside, done = "unread", "", "", make(chan struct{})
		m = map[string]curly.Modifier{
			"own": own(option),
			"hand": func(in curly.Input) (any, error) {
				go func() {
					defer close(done)

					if c.modifier {
						beside = in.Options[0].Value()
					} else {
						beside = option(kept)
					}
				}()

				<-done

				return "n", nil
			},
			"read": func(curly.Input) (any, error) {
				inner = option(kept)

				return "r", nil
			},
		}

		expect(t, c.what, host(m, "{{x:own; o:<{{x:hand; b:{{x:read}}}}>}}", map[string]any{"x": 1}), "<n>")
		<-done

		if inner != c.inner || beside != c.beside || later != "<n>" {
			t.Errorf("%s: the read inside answered %q, the read beside %q, the read after %q", c.what, inner, beside, later)
		}
	}

	// The chain the walk itself takes for a modifier that failed, read from
	// inside it through the Input the modifier kept.
	m = map[string]curly.Modifier{
		"fail": func(in curly.Input) (any, error) {
			kept = in

			return nil, errors.New("failed")
		},
		"again": func(curly.Input) (any, error) {
			inner = kept.Default()

			return "a", nil
		},
	}
	inner = "unread"

	expect(t, "the walk's own chain", host(m, "{{x:fail; default:<{{x:again; default:D}}>}}", map[string]any{"x": 1}), "<a>", curly.FailedModifier)

	if inner != "" {
		t.Errorf("the walk's own chain: the read inside answered %q", inner)
	}
}

// A read host code makes waits for no rendering another goroutine is
// making: one whose option reaches a fallback chain a modifier's goroutine
// is rendering is refused there (section 13), which hands that placeholder
// the empty string and is not recorded, so the goroutine's read renders the
// chain, and a later read answers it. Here the value the goroutine renders
// waits for the first read to return, so a read that waited would leave
// every goroutine of the bubble blocked, which synctest fails at once.
func TestExtensionsReadBesideARenderingDoesNotWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := gate{calls: new(atomic.Int32), entered: make(chan struct{}), release: make(chan struct{})}
		fell := make(chan string, 1)

		var kept curly.Input

		m := map[string]curly.Modifier{
			"outer": func(in curly.Input) (any, error) {
				read := in.Options[0].Value()
				close(g.release)

				return read + "|" + <-fell + "|" + kept.Default(), nil
			},
			"fails": func(in curly.Input) (any, error) {
				kept = in

				go func() { fell <- in.Default() }()

				<-g.entered

				return nil, errors.New("failed")
			},
		}

		got := host(m, "{{x:outer; a:<{{x:fails; default:D{{g}}}}>}}", map[string]any{"x": 1, "g": g})

		expect(t, "a read beside a rendering", got, "<>|DS|DS", curly.FailedModifier)

		if g.calls.Load() != 1 {
			t.Errorf("String called %d times", g.calls.Load())
		}
	})
}

// A walk that waited for what a modifier's goroutine was producing goes on
// as it was: what it renders next is still told to be on its own stack, so
// a read of that from inside it is refused rather than waited for, and
// answers the empty string. A read that waited would leave every goroutine
// of the bubble blocked, which synctest fails at once.
func TestExtensionsReadAfterTheWalkWaited(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var kept curly.Input

		entered := make(chan struct{})

		m := map[string]curly.Modifier{
			// The modifier fails while its goroutine is inside its default,
			// so the walk waits for that default.
			"spawn": func(in curly.Input) (any, error) {
				go in.Default()
				<-entered

				return nil, errors.New("failed")
			},
			// Every other goroutine of the bubble is blocked once the walk
			// waits.
			"hold": func(curly.Input) (any, error) {
				close(entered)
				synctest.Wait()

				return "h", nil
			},
			"fail": func(in curly.Input) (any, error) {
				kept = in

				return nil, errors.New("failed")
			},
			"again": func(curly.Input) (any, error) { return kept.Default(), nil },
		}

		got := host(m, "{{x:spawn; default:<{{x:hold}}>}}{{x:fail; default:<{{x:again; default:D}}>}}", map[string]any{"x": 1})

		expect(t, "a read after the walk waited", got, "<h><>", curly.FailedModifier, curly.FailedModifier)
	})
}

// A resolution asked for where none of its chain is in progress counts as
// the first and is never refused: one Parser.Resolve starts, which counts
// apart from the chain, even from a modifier at MaxReentry, and one through
// an Input whose resolution has returned. The zero Input belongs to no
// resolution and resolves nothing.
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
			return in.Resolver().Resolve(message, curly.Context{Payload: payload})
		}

		kept = in
		_, deepest = in.Resolver().Resolve("{{v}}", curly.Context{Payload: payload})
		fresh = p.Resolve("{{v}}", curly.Context{Payload: payload})

		return "x", nil
	}}})

	if got := p.Resolve(message, curly.Context{Payload: payload}); len(got) != 2*curly.MaxReentry+1 || !errors.Is(deepest, curly.ErrReentryLimit) || fresh != "v" {
		t.Errorf("at the bound: got %.40q (%d bytes) %v %q", got, len(got), deepest, fresh)
	}

	if got, err := kept.Resolver().Resolve("{{v}}", curly.Context{Payload: payload}); got != "v" || err != nil {
		t.Errorf("kept: got %q %v", got, err)
	}

	if got, err := (curly.Input{}).Resolver().Resolve("{{v}}", curly.Context{Payload: payload}); got != "" || err == nil {
		t.Errorf("zero: got %q %v", got, err)
	}
}

type gatedText struct {
	text    string
	entered chan struct{}
	release chan struct{}
}

func (g gatedText) String() string {
	close(g.entered)
	<-g.release

	return g.text
}

// The walk waits for each conversion a read is making, the second as the
// first: what a read marks on the resolution is the read's, and the walk
// waking from a wait is the walk still.
func TestExtensionsWalkWaitsForEachConversion(t *testing.T) {
	v := gatedText{"V", make(chan struct{}), make(chan struct{})}
	u := gatedText{"U", make(chan struct{}), make(chan struct{})}

	var (
		reports int
		wg      sync.WaitGroup
	)

	p := curly.New(curly.Options{
		OnReport: func(curly.Report, curly.Resolver) { reports++ },
		CustomModifiers: map[string]curly.Modifier{"fan": func(in curly.Input) (any, error) {
			wg.Add(2)

			go func() { defer wg.Done(); in.Options[0].Value() }()
			<-v.entered

			go func() { defer wg.Done(); in.Options[1].Value() }()
			<-u.entered

			// The walk reaches {{v}} and waits once this returns; the
			// yields let it get there before v is released.
			go func() {
				for range 1000 {
					runtime.Gosched()
				}

				close(v.release)

				for range 1000 {
					runtime.Gosched()
				}

				close(u.release)
			}()

			return "ok", nil
		}},
	})

	got := p.Resolve("{{x:fan; a:{{v}}; b:{{u}}}}{{v}}{{u; default:MISSING}}",
		curly.Context{Payload: map[string]any{"x": 1, "v": v, "u": u}})

	wg.Wait()

	if got != "okVU" || reports != 0 {
		t.Errorf("got %q with %d reports, want okVU with none", got, reports)
	}
}

// A read that meets a value another read is converting is refused where it
// meets it: the empty string, no report for that value, and the empty string
// is the option's value for the rest of the resolution.
func TestExtensionsReadMeetingAConversionInProgress(t *testing.T) {
	c := gatedText{"S", make(chan struct{}), make(chan struct{})}

	var (
		reports       []curly.Code
		during, after [2]string
	)

	p := curly.New(curly.Options{
		OnReport: func(r curly.Report, _ curly.Resolver) { reports = append(reports, r.Code) },
		CustomModifiers: map[string]curly.Modifier{"fan": func(in curly.Input) (any, error) {
			done := make(chan struct{})

			go func() { during[0] = in.Options[0].Value(); close(done) }()
			<-c.entered

			during[1] = in.Options[1].Value()

			close(c.release)
			<-done

			after[0], after[1] = in.Options[0].Value(), in.Options[1].Value()

			return after[0] + "|" + after[1], nil
		}},
	})

	got := p.Resolve("{{x:fan; a:<{{c}}>; b:[{{c; default:GONE}}]}} then {{c}}",
		curly.Context{Payload: map[string]any{"x": 1, "c": c}})

	if got != "<S>| then S" || during != [2]string{"<S>", ""} || after != [2]string{"<S>", ""} || len(reports) != 0 {
		t.Errorf("got %q, during %q, after %q, reports %v", got, during, after, reports)
	}
}

// rereading reads its option a while a goroutine converts the value it
// names, rereads its option b, which meets that value, as often as a caller
// handed the empty string would, and answers the last read of b.
func rereading(c gatedText, times int) curly.Modifier {
	return func(in curly.Input) (any, error) {
		done := make(chan struct{})

		go func() { in.Options[0].Value(); close(done) }()
		<-c.entered

		for range times {
			in.Options[1].Value()
		}

		close(c.release)
		<-done

		return "[" + in.Options[1].Value() + "]", nil
	}
}

// What a refused read reached before it met the conversion is reached once
// (section 14.3), however often host code reads the option again.
func TestExtensionsRefusedReadIsProducedOnce(t *testing.T) {
	for _, times := range []int{1, 10, 1000} {
		c := gatedText{"S", make(chan struct{}), make(chan struct{})}
		reports := map[string]int{}
		counted := 0

		p := curly.New(curly.Options{
			OnReport: func(r curly.Report, _ curly.Resolver) { reports[string(r.Code)+" "+r.Text]++ },
			CustomModifiers: map[string]curly.Modifier{
				"count": func(curly.Input) (any, error) { counted++; return "n", nil },
				"fan":   rereading(c, times),
			},
		})

		got := p.Resolve("{{x:fan; a:{{c}}; b:{{x:nope}}{{x:count}}{{gone}}[{{c}}]}}",
			curly.Context{Payload: map[string]any{"x": 1, "c": c}})

		if got != "[]" || counted != 1 || len(reports) != 1 || reports["unknown-modifier {{x:nope}}"] != 1 {
			t.Errorf("%d reads: got %q, count called %d times, reports %v", times, got, counted, reports)
		}
	}
}

// What a refused read produced is charged to the read budget once, so
// reading the option again takes nothing from the rest of the message.
func TestExtensionsRefusedReadIsChargedOnce(t *testing.T) {
	for _, times := range []int{1, 50, 200} {
		c := gatedText{"S", make(chan struct{}), make(chan struct{})}

		var reports []curly.Code

		p := curly.New(curly.Options{
			OnReport:        func(r curly.Report, _ curly.Resolver) { reports = append(reports, r.Code) },
			CustomModifiers: map[string]curly.Modifier{"fan": rereading(c, times)},
		})

		got := p.Resolve("{{x:fan; a:{{c}}; b:{{big}}[{{c}}]}} tail {{name}}",
			curly.Context{Payload: map[string]any{"x": 1, "c": c, "name": "Ann", "big": strings.Repeat("b", 1000)}})

		if got != "[] tail Ann" || len(reports) != 0 {
			t.Errorf("%d reads: got %q, reports %v", times, got, reports)
		}
	}
}

type countedTexter struct{ calls *int }

func (c countedTexter) CurlyText(curly.Resolver) string { *c.calls++; return "T" }

func (c countedTexter) String() string { *c.calls += 100; return "S" }

type holder struct{ V any }

// CurlyText stands where String does: for the value, behind a pointer and
// for an operand of a struct. A slice or a map is JSON, which calls neither.
func TestExtensionsTexterWhereStringIs(t *testing.T) {
	p := curly.New(curly.Options{})

	for _, tc := range []struct {
		name  string
		value func(countedTexter) any
		want  string
		calls int
	}{
		{"value", func(v countedTexter) any { return v }, "T", 1},
		{"pointer", func(v countedTexter) any { return &v }, "T", 1},
		{"struct", func(v countedTexter) any { return holder{v} }, "{T}", 1},
		{"slice", func(v countedTexter) any { return []any{v} }, "[{}]", 0},
		{"map", func(v countedTexter) any { return map[string]any{"k": v} }, `{"k":{}}`, 0},
	} {
		calls := 0
		got := p.Resolve("{{m}}", curly.Context{Payload: map[string]any{"m": tc.value(countedTexter{&calls})}})

		if got != tc.want || calls != tc.calls {
			t.Errorf("%s: got %q after %d calls, want %q after %d", tc.name, got, calls, tc.want, tc.calls)
		}
	}
}

// lazy is a lazily translated value: it becomes text by resolving the
// message msg points to, which names it again, through the Resolver it is
// handed, and counts how often it is asked.
type lazy struct {
	msg   *string
	asked *int
}

func (l lazy) CurlyText(r curly.Resolver) string {
	*l.asked++

	text, err := r.Resolve(*l.msg, curly.Context{Payload: map[string]any{"m": l}})
	if err != nil {
		return "[stopped]"
	}

	return text
}

// A value that resolves the message naming it through the Resolver it is
// handed is counted on the lineage of the resolution converting it: the
// MaxReentry-th resolution is refused, with no modifier in the message, at
// the top or seven inline defaults deep.
func TestExtensionsTexterIsCounted(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	for _, msg := range []string{"Hello {{m}}!", "<{{m}}>", strings.Repeat("{{zz; default:", 7) + "{{m}}" + strings.Repeat("}}", 7)} {
		asked := 0
		got := curly.New(curly.Options{}).Resolve(msg, curly.Context{Payload: map[string]any{"m": lazy{&msg, &asked}}})

		if asked != curly.MaxReentry || !strings.Contains(got, "[stopped]") {
			t.Errorf("%.20q: %d bytes, asked %d times", msg, len(got), asked)
		}
	}
}

// translating is a value that renders a text/template whose function
// translates with the Resolver the value was handed: the means reaches code
// the value calls in turn.
type translating struct {
	t     **template.Template
	asked *int
}

func (v translating) CurlyText(r curly.Resolver) string {
	var b strings.Builder

	_ = (*v.t).Execute(&b, r)

	return b.String()
}

func TestExtensionsTexterHandsItsResolverOn(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	var tm *template.Template

	asked := 0
	tm = template.Must(template.New("t").Funcs(template.FuncMap{"t": func(r curly.Resolver) string {
		asked++

		text, err := r.Resolve("Hello {{m}}!", curly.Context{Payload: map[string]any{"m": translating{&tm, &asked}}})
		if err != nil {
			return "[stopped]"
		}

		return text
	}}).Parse(`{{with .}}{{t .}}{{end}}`))

	got := curly.New(curly.Options{}).Resolve("Hello {{m}}!", curly.Context{Payload: map[string]any{"m": translating{&tm, &asked}}})

	if asked != curly.MaxReentry || !strings.Contains(got, "[stopped]") {
		t.Errorf("%d bytes, asked %d times", len(got), asked)
	}
}

// A resolution asked for through a Resolver counts with the counted reads
// of its lineage: a value converted inside MaxReentry-1 counted reads asks
// for the one resolution past the bound, and is refused.
func TestExtensionsTexterCountsWithCountedReads(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	var kept []curly.Input

	p := curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{
		"keep": func(in curly.Input) (any, error) { kept = append(kept, in); return "", nil },
		"take": func(in curly.Input) (any, error) {
			if len(kept) == 0 {
				return "none", nil
			}

			k := kept[len(kept)-1]
			kept = kept[:len(kept)-1]

			return k.Default(), nil
		},
	}})

	asked := 0
	resolving := texterFunc(func(r curly.Resolver) string {
		asked++

		if _, err := r.Resolve("x", curly.Context{}); err != nil {
			return "[stopped]"
		}

		return "[resolved]"
	})

	const reads = curly.MaxReentry - 1

	msg := "{{x:keep; default:{{m}}}}" + strings.Repeat("{{x:keep; default:{{x:take; default:F}}}}", reads-1) + "{{x:take; default:F}}"

	for _, c := range []struct {
		what string
		msg  string
		want string
	}{
		{"past the bound", msg, "[stopped]"},
		{"near miss: one read fewer", "{{x:keep; default:{{m}}}}" + strings.Repeat("{{x:keep; default:{{x:take; default:F}}}}", reads-2) + "{{x:take; default:F}}", "[resolved]"},
	} {
		kept, asked = nil, 0

		if got := p.Resolve(c.msg, curly.Context{Payload: map[string]any{"x": "", "m": resolving}}); got != c.want || asked != 1 {
			t.Errorf("%s: got %q, value asked %d times", c.what, got, asked)
		}
	}
}

type texterFunc func(curly.Resolver) string

func (f texterFunc) CurlyText(r curly.Resolver) string { return f(r) }

// A reporting handler that resolves a message which reports, through the
// Resolver it is handed, is counted on the lineage of the resolution that
// reported: the MaxReentry-th is refused.
func TestExtensionsOnReportIsCounted(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	calls, refused := 0, 0
	p := curly.New(curly.Options{OnReport: func(_ curly.Report, r curly.Resolver) {
		calls++

		if _, err := r.Resolve("log {{v:nope}}", curly.Context{}); err != nil {
			refused++
		}
	}})

	if got := p.Resolve("a {{v:nope; default:D}}", curly.Context{}); got != "a D" || calls != curly.MaxReentry || refused != 1 {
		t.Errorf("got %q, handler called %d times, refused %d", got, calls, refused)
	}
}

// A modifier that asks, through its Input, for the resolution of the message
// it is in stands MaxReentry resolutions deep, and the one it asks for past
// them is refused, so the innermost takes its fallback chain.
func TestExtensionsResolutionsOfALineage(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	const msg = "[{{x:renest; default:F}}]"

	p := curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{
		"renest": func(in curly.Input) (any, error) {
			return in.Resolver().Resolve(msg, curly.Context{Payload: map[string]any{"x": ""}})
		},
	}})

	want := strings.Repeat("[", curly.MaxReentry) + "F" + strings.Repeat("]", curly.MaxReentry)

	if got := p.Resolve(msg, curly.Context{Payload: map[string]any{"x": ""}}); got != want {
		t.Errorf("got %d bytes, want %d", len(got), len(want))
	}
}

// Two parsers whose modifiers include each other's entries count together
// through Resolver.With.
func TestExtensionsResolverWithAnotherParser(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	var a, b *curly.Parser

	calls := 0
	include := func(other **curly.Parser) curly.Modifier {
		return func(in curly.Input) (any, error) {
			calls++

			return in.Resolver().With(*other).Resolve("<{{x:inc}}>", curly.Context{Payload: map[string]any{"x": 1}})
		}
	}
	a = curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{"inc": include(&b)}})
	b = curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{"inc": include(&a)}})

	if got := a.Resolve("<{{x:inc}}>", curly.Context{Payload: map[string]any{"x": 1}}); calls != curly.MaxReentry || len(got) != 2*curly.MaxReentry {
		t.Errorf("%d bytes, %d calls", len(got), calls)
	}

	if got, err := (curly.Resolver{}).Resolve("x", curly.Context{}); got != "" || err == nil {
		t.Errorf("zero: got %q %v", got, err)
	}
}

type selfReading struct{ kept *curly.Input }

func (s selfReading) String() string { return "<" + s.kept.Options[0].Value() + ">" }

// A value whose String reads a kept option naming that same value meets its
// own conversion in progress: the read is refused there, nothing waits, and
// the empty string is the option's value from then on.
func TestExtensionsConversionReadingItself(t *testing.T) {
	var kept curly.Input

	p := curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{
		"keep": func(in curly.Input) (any, error) { kept = in; return "", nil },
	}})

	if got := p.Resolve("{{x:keep; o:[{{c}}]}}{{c}}", curly.Context{Payload: map[string]any{"x": 1, "c": selfReading{&kept}}}); got != "<>" || kept.Options[0].Value() != "" {
		t.Errorf("got %q, then %q", got, kept.Options[0].Value())
	}
}

type readsOwnOption struct{ of **curly.Input }

func (r readsOwnOption) String() string { return "<" + (*r.of).Options[0].Value() + ">" }

// A read of the current modifier's own option is the walk's while another
// placeholder's option it reads is being rendered: of the two readings
// section 13 allows, this port counts an own read only while one of that
// placeholder's own options or its default is. Inside one counted read fewer
// than the bound lets stand, h's read of what k kept is the last counted
// read standing, and the read of h's own option the conversion of g makes
// inside it counts nothing; one counted read more and h's read is refused.
func TestExtensionsOwnReadBeneathACountedRead(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))

	for reads, want := range map[int]string{curly.MaxReentry - 2: "k[<O>]", curly.MaxReentry - 1: "k[]"} {
		var defaults []func() string
		var a curly.Option
		var h *curly.Input
		p := curly.New(curly.Options{CustomModifiers: map[string]curly.Modifier{
			"keep": func(in curly.Input) (any, error) { defaults = append(defaults, in.Default); return "", nil },
			"take": func(curly.Input) (any, error) {
				read := defaults[len(defaults)-1]
				defaults = defaults[:len(defaults)-1]

				return read(), nil
			},
			"k": func(in curly.Input) (any, error) { a = in.Options[0]; return "k", nil },
			"h": func(in curly.Input) (any, error) { h = &in; return "[" + a.Value() + "]", nil },
		}})
		take := "{{x:take; default:F}}"
		msg := "{{x:keep; default:{{x:k; a:{{g}}}}{{x:h; o:O}}}}" + strings.Repeat("{{x:keep; default:"+take+"}}", reads-1) + take

		if got := p.Resolve(msg, curly.Context{Payload: map[string]any{"x": "", "g": readsOwnOption{&h}}}); got != want {
			t.Errorf("inside %d counted reads: got %q, want %q", reads, got, want)
		}
	}
}
