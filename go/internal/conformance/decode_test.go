package conformance

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestDecode(t *testing.T) {
	t.Run("replaces each tag at every depth, and leaves the rest as JSON decodes it", func(t *testing.T) {
		got, err := decode(parsed(t, `{"a": [1, {"$curly": "undefined"}, {"b": {"$curly": "undefined"}}], "c": "text", "d": null, "e": true}`), absent{})

		if err != nil {
			t.Fatal(err)
		}

		want := map[string]any{"a": []any{1.0, absent{}, map[string]any{"b": absent{}}}, "c": "text", "d": nil, "e": true}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v", got)
		}
	})

	t.Run("decodes unserializable to an object holding itself", func(t *testing.T) {
		got, _ := decode(parsed(t, `{"$curly": "unserializable"}`), absent{})
		m := got.(map[string]any)

		if reflect.ValueOf(m["self"]).Pointer() != reflect.ValueOf(m).Pointer() || len(m) != 1 {
			t.Errorf("got %v", m)
		}

		if _, ok := stringify(m); ok {
			t.Error("JSON described a value holding itself")
		}
	})

	t.Run("decodes nodes to a tree visiting at least the count, holding no cycle", func(t *testing.T) {
		for _, count := range []float64{0, 1, 2, 3, 8, 100001} {
			got, _ := decode(map[string]any{tag: "nodes", "count": count}, absent{})

			if n := visits(got); float64(n) < count || (count > 1 && float64(n) >= 2*count+1) {
				t.Errorf("count %v: %d visits", count, n)
			}

			if _, ok := stringify(got); !ok {
				t.Errorf("count %v: JSON could not describe the tree", count)
			}
		}
	})

	t.Run("refuses an unknown tag and a nodes tag with no numeric count", func(t *testing.T) {
		_, err := decode(parsed(t, `[{"$curly": "bogus"}]`), absent{})
		expectError(t, err, `Unknown tag "bogus".`)

		_, err = decode(parsed(t, `{"v": {"$curly": "nodes", "count": "8"}}`), absent{})
		expectError(t, err, "A nodes tag needs a numeric count.")

		_, err = decode(parsed(t, `{"$curly": "nodes"}`), absent{})
		expectError(t, err, "A nodes tag needs a numeric count.")
	})

	t.Run("decodes no refusing tag, which planning leaves out", func(t *testing.T) {
		_, err := decode(parsed(t, `{"v": {"$curly": "refusing", "members": ["a"]}}`), absent{})
		expectError(t, err, "This host has no value whose entries raise where they are read, so a case holding a refusing tag is left out rather than decoded.")
	})

	t.Run("builds the value afresh on each call", func(t *testing.T) {
		written := parsed(t, `{"v": {"w": [1]}}`)
		a, _ := decode(written, absent{})
		b, _ := decode(written, absent{})
		a.(map[string]any)["v"].(map[string]any)["w"].([]any)[0] = 2.0

		if !reflect.DeepEqual(b, parsed(t, `{"v": {"w": [1]}}`)) || !reflect.DeepEqual(written, b) {
			t.Error("two decodes shared a value")
		}
	})

	t.Run("walks a deep input on a stack of its own", func(t *testing.T) {
		var deep any = "x"

		for range 100000 {
			deep = []any{deep}
		}

		got, err := decode(deep, absent{})

		for range 100000 {
			got = got.([]any)[0]
		}

		if err != nil || got != "x" {
			t.Errorf("got %v, %v", got, err)
		}
	})
}

func TestStringify(t *testing.T) {
	for value, want := range map[float64]string{
		0: "0", 1: "1", -1.5: "-1.5", 1234.5678: "1234.5678", 1e21: "1e+21", 1e20: "100000000000000000000",
		123456789012345680000: "123456789012345680000", 0.000001: "0.000001", 0.0000001: "1e-7", -2.5e-7: "-2.5e-7",
		5e-324: "5e-324", math.MaxFloat64: "1.7976931348623157e+308", 0.30000000000000004: "0.30000000000000004", 1e300: "1e+300",
	} {
		if got := esNumber(value); got != want {
			t.Errorf("esNumber(%v) = %s, want %s", value, got, want)
		}
	}

	for _, c := range []struct {
		value any
		want  string
	}{
		{negativeZero(), "0"},
		{math.NaN(), "null"},
		{math.Inf(-1), "null"},
		{"a\"b\\c\n\t\u0001</>\u00e9\U0001F600", `"a\"b\\c\n\t\u0001</>` + "\u00e9\U0001F600" + `"`},
		{map[string]any{"b": 1.0, "a": []any{true, nil, "x"}, "c": map[string]any{}}, `{"a":[true,null,"x"],"b":1,"c":{}}`},
		{[]any{}, "[]"},
		{[]string{"x<y"}, `["x<y"]`},
		{Report{Code: "read-limit", Limit: 5}, `{"code":"read-limit","limit":5}`},
	} {
		if got, ok := stringify(c.value); !ok || got != c.want {
			t.Errorf("stringify(%#v) = %s, %v, want %s", c.value, got, ok, c.want)
		}
	}

	shared := []any{"x"}

	if got, ok := stringify([]any{shared, shared}); !ok || got != `[["x"],["x"]]` {
		t.Errorf("a shared value: %s, %v", got, ok)
	}

	held := []any{nil, nil}
	held[0] = held

	if _, ok := stringify(map[string]any{"v": held}); ok {
		t.Error("described a slice holding itself")
	}

	if _, ok := stringify(Nodes(stringifyBudget * 4)); ok {
		t.Error("wrote past the budget")
	}

	if _, ok := stringify(func() {}); ok {
		t.Error("described a function")
	}
}

// call is what b answers for in, failing the test where it raises.
func call(t *testing.T, b Behaviour, in ModifierInput) any {
	t.Helper()

	got, err := b(in)

	if err != nil {
		t.Fatal(err)
	}

	return got
}

func TestKeeping(t *testing.T) {
	// read is an input kept by one modifier, read by another, that counts
	// its reads and answers text.
	read := func(text string, count *int) ModifierInput {
		return ModifierInput{Default: func() string { *count++; return text }}
	}

	t.Run("keeps an input unread, and takes the last one no take has claimed", func(t *testing.T) {
		k := keeping(adapterOf(echo))
		var a, b int

		if call(t, k["take"], ModifierInput{}) != "none" || call(t, k["first"], ModifierInput{}) != "none" {
			t.Error("found an input where none was kept")
		}

		if call(t, k["keep"], read("A", &a)) != "" || call(t, k["keep-default"], read("", &b)) != "" || a != 0 || b != 1 {
			t.Errorf("keep read %d times, keep-default %d", a, b)
		}

		if got := call(t, k["take"], ModifierInput{}); got != "empty" {
			t.Errorf("the first take answered %v", got)
		}

		if got := call(t, k["take"], ModifierInput{}); got != "A" || a != 1 {
			t.Errorf("the second take answered %v", got)
		}

		if call(t, k["take"], ModifierInput{}) != "none" || call(t, k["first"], ModifierInput{}) != "A" || a != 2 {
			t.Error("a take found an input every take had claimed, or first read another")
		}
	})

	t.Run("claims an input before reading it, so a take inside the read finds the one before", func(t *testing.T) {
		k := keeping(adapterOf(echo))
		var inner string
		call(t, k["keep"], ModifierInput{Default: func() string { return "outer" }})
		call(t, k["keep"], ModifierInput{Default: func() string { taken, _ := k["take"](ModifierInput{}); inner, _ = taken.(string); return "" }})

		if got := call(t, k["take"], ModifierInput{}); got != "empty" || inner != "outer" {
			t.Errorf("answered %v, the take inside %q", got, inner)
		}
	})

	t.Run("answers a value whose conversion takes", func(t *testing.T) {
		k := keeping(adapterOf(echo))
		var count int
		call(t, k["keep"], read("A", &count))
		answer := call(t, k["taking"], ModifierInput{})

		if count != 0 {
			t.Fatal("taking read in its call")
		}

		if got := fmt.Sprint(answer); got != "A" || count != 1 {
			t.Errorf("converted to %q", got)
		}
	})

	t.Run("asks the adapter for a resolution over the same list", func(t *testing.T) {
		var asked Resolution
		var k map[string]Behaviour
		adapter := adapterOf(func(in Resolution) *Resolved {
			asked = in

			taken, _ := in.Modifiers["take"](ModifierInput{})

			return &Resolved{Output: fmt.Sprint(taken)}
		})
		k = keeping(adapter)
		call(t, k["keep"], ModifierInput{Default: func() string { return "A" }})

		if got := call(t, k["nest"], ModifierInput{}); got != "A" {
			t.Errorf("nest answered %v", got)
		}

		if asked.Message != "{{x:take; default:F}}" || !reflect.DeepEqual(asked.Payload, map[string]any{"x": ""}) || asked.ID != (absent{}) || len(asked.Modifiers) != 8 || asked.Modifiers["ne"] != nil {
			t.Errorf("asked for %+v", asked)
		}

		nothing := keeping(adapterOf(func(Resolution) *Resolved { return nil }))

		if _, err := nothing["nest"](ModifierInput{}); err == nil || err.Error() != "The resolution nest asked for answered no output." {
			t.Errorf("got %v", err)
		}
	})

	t.Run("keeps a default and asks for a resolution that reads it as it converts", func(t *testing.T) {
		var asked Resolution
		k := keeping(adapterOf(func(in Resolution) *Resolved {
			asked = in

			taken, _ := in.Modifiers["taking"](ModifierInput{})

			return &Resolved{Output: fmt.Sprint(taken)}
		}))

		if got := call(t, k["keep-nest"], ModifierInput{Default: func() string { return "A" }}); got != "A" {
			t.Errorf("keep-nest answered %v", got)
		}

		if asked.Message != "{{x:taking; default:F}}" || !reflect.DeepEqual(asked.Payload, map[string]any{"x": ""}) || call(t, k["first"], ModifierInput{}) != "A" {
			t.Errorf("asked for %+v", asked)
		}
	})

	t.Run("keeps a default and asks for a comparison of the format, registering nothing under its name", func(t *testing.T) {
		var asked Resolution
		k := keeping(adapterOf(func(in Resolution) *Resolved {
			asked = in

			taken, _ := in.Modifiers["taking"](ModifierInput{})

			return &Resolved{Output: fmt.Sprint(taken)}
		}))

		if got := call(t, k["keep-nest-ne"], ModifierInput{Default: func() string { return "A" }}); got != "A" {
			t.Errorf("keep-nest-ne answered %v", got)
		}

		if _, registered := asked.Modifiers["ne"]; asked.Message != "{{x:ne; b:{{x:taking; default:F}}}}" || !reflect.DeepEqual(asked.Payload, map[string]any{"x": ""}) || registered || call(t, k["first"], ModifierInput{}) != "A" {
			t.Errorf("asked for %+v", asked)
		}
	})

	t.Run("descends through resolutions one inside another, the innermost of the message it leads to", func(t *testing.T) {
		for hops, want := range map[int][]string{1: {"I"}, 3: {descended, descended, "I"}} {
			var asked []string
			adapter := adapterOf(func(in Resolution) *Resolved {
				message, _ := in.Message.(string)
				asked = append(asked, message)

				if _, registered := in.Modifiers["descend"]; len(in.Modifiers) != 9 || !registered {
					t.Errorf("asked over %d modifiers", len(in.Modifiers))
				}

				if message != descended {
					return &Resolved{Output: message}
				}

				answer, err := in.Modifiers["descend"](ModifierInput{})

				if err != nil {
					return nil
				}

				return &Resolved{Output: "(" + fmt.Sprint(answer) + ")"}
			})
			d := descending(adapter, hops, "I")

			if got := call(t, d["descend"], ModifierInput{}); got != strings.Repeat("(", hops-1)+"I"+strings.Repeat(")", hops-1) || !slices.Equal(asked, want) {
				t.Errorf("at %d hops answered %v after asking for %q", hops, got, asked)
			}
		}

		if _, ok := keeping(adapterOf(echo))["descend"]; ok {
			t.Error("registered descend with no descent under way")
		}

		if _, err := descending(adapterOf(func(Resolution) *Resolved { return nil }), 2, "I")["descend"](ModifierInput{}); err == nil {
			t.Error("a refused resolution raised nothing")
		}
	})

	t.Run("builds the list afresh for each resolution", func(t *testing.T) {
		names := map[string]string{"k": "keep", "t": "take"}
		one := register(nil, names, adapterOf(echo))
		other := register(map[string]string{"u": "upper"}, names, adapterOf(echo))
		call(t, one["k"], ModifierInput{Default: func() string { return "A" }})

		if call(t, other["t"], ModifierInput{}) != "none" || call(t, one["t"], ModifierInput{}) != "A" || len(other) != 3 {
			t.Error("two resolutions share what was kept")
		}

		if register(nil, nil, adapterOf(echo)) != nil {
			t.Error("registered modifiers where the case registers none")
		}
	})
}

func TestRenesting(t *testing.T) {
	t.Run("asks for the message it is registered in, with itself registered", func(t *testing.T) {
		var asked []Resolution
		refusing := 2
		renest := renesting(adapterOf(func(in Resolution) *Resolved {
			asked = append(asked, in)

			if len(asked) > refusing {
				return nil
			}

			answer, err := in.Modifiers["renest"](ModifierInput{})

			if err != nil {
				return &Resolved{Output: "[F]"}
			}

			return &Resolved{Output: "[" + fmt.Sprint(answer) + "]"}
		}), 5)

		if got := call(t, renest, ModifierInput{}); got != "[[F]]" || len(asked) != 3 {
			t.Errorf("answered %v after %d resolutions", got, len(asked))
		}

		if asked[0].Message != "[{{x:renest; default:F}}]" || !reflect.DeepEqual(asked[0].Payload, map[string]any{"x": ""}) || asked[0].ID != (absent{}) || len(asked[0].Modifiers) != 1 {
			t.Errorf("asked for %+v", asked[0])
		}
	})

	t.Run("asks through the means its input carries in place of the adapter", func(t *testing.T) {
		unasked := adapterOf(func(Resolution) *Resolved { t.Fatal("asked the adapter"); return nil })
		means := func(in Resolution) *Resolved { return &Resolved{Output: fmt.Sprint(in.Message)} }
		k := keeping(unasked)

		for name, b := range map[string]Behaviour{"renest": renesting(unasked, 1), "nest": k["nest"], "keep-nest": k["keep-nest"], "keep-nest-ne": k["keep-nest-ne"], "descend": descending(unasked, 1, "I")["descend"]} {
			if got := call(t, b, ModifierInput{Default: func() string { return "" }, Resolve: means}); got == "" {
				t.Errorf("%s answered %v", name, got)
			}
		}

		if _, err := renesting(unasked, 1)(ModifierInput{Resolve: func(Resolution) *Resolved { return nil }}); err == nil {
			t.Error("a refused resolution raised nothing")
		}
	})

	t.Run("near miss: answers unrefused one level past the bound, asking nothing", func(t *testing.T) {
		asked := 0
		renest := renesting(adapterOf(func(in Resolution) *Resolved {
			asked++

			answer, _ := in.Modifiers["renest"](ModifierInput{})

			return &Resolved{Output: "[" + fmt.Sprint(answer) + "]"}
		}), 1)

		if got := call(t, renest, ModifierInput{}); got != "[[unrefused]]" || asked != 2 {
			t.Errorf("answered %v after %d resolutions", got, asked)
		}
	})

}

func TestBehaviours(t *testing.T) {
	options := []OptionInput{{"a", func() string { return "A" }}, {"b", func() string { return "" }}, {"c", func() string { return "c" }}}
	input := ModifierInput{
		Value:   "abc-\u00e9z",
		Options: options,
		Props:   map[string]any{"useGrouping": true, "maximumFractionDigits": 1.0, "currency": "EUR/\u00e9"},
		Locale:  ptr("de"),
		Default: func() string { return "the default" },
	}

	for name, want := range map[string]any{
		"upper":   "ABC-\u00e9Z",
		"echo":    "abc-\u00e9z",
		"empty":   "",
		"nothing": nil,
		"default": "the default",
		"options": "a=A,b=,c=c",
		"props":   "currency=\"EUR/\u00e9\",maximumFractionDigits=1,useGrouping=true",
		"locale":  "de",
		"object":  map[string]any{"answer": "abc-\u00e9z"},
	} {
		if got, err := behaviours[name](input); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %#v, %v, want %#v", name, got, err, want)
		}
	}

	if got, err := behaviours["raise"](input); got != nil || err == nil || err.Error() != "The raise behaviour raised." {
		t.Errorf("raise: got %v, %v", got, err)
	}

	for locale, want := range map[*string]string{nil: "none", ptr(""): ""} {
		if got, _ := behaviours["locale"](ModifierInput{Locale: locale}); got != want {
			t.Errorf("locale %v: got %v", locale, got)
		}
	}

	read := 0
	lazy := ModifierInput{Options: []OptionInput{{"a", func() string { read++; return "" }}}, Default: func() string { read++; return "" }}

	for _, name := range []string{"upper", "echo", "empty", "nothing", "raise", "props", "locale", "object"} {
		behaviours[name](lazy)
	}

	if read != 0 {
		t.Errorf("behaviours that read neither read %d", read)
	}

	cyclic := map[string]any{}
	cyclic["self"] = cyclic

	if _, err := behaviours["props"](ModifierInput{Props: map[string]any{"v": cyclic}}); err == nil {
		t.Error("props wrote a value holding itself")
	}
}
