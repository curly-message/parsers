package conformance

import (
	"math"
	"reflect"
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
