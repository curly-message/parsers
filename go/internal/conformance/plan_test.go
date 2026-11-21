package conformance

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestClaims(t *testing.T) {
	empty := Options{Set: setOf()}
	refused := func(name string, adapter Adapter, options Options, message string) {
		t.Run(name, func(t *testing.T) {
			_, err := NewPlan(adapter, options)
			expectError(t, err, message)
		})
	}
	with := func(change func(*Adapter)) Adapter {
		adapter := adapterOf(echo)
		change(&adapter)

		return adapter
	}

	refused("no core", adapterOf(echo, LevelIntl), empty, "The adapter must claim the core level.")
	refused("no levels", with(func(a *Adapter) { a.Levels = nil }), empty, "The adapter must claim the core level.")
	refused("an unknown level", adapterOf(echo, LevelCore, "foo"), empty, `The adapter names the level "foo"; the levels are core, intl and extensions.`)
	refused("a case-variant level", adapterOf(echo, "Core"), empty, `The adapter names the level "Core"; the levels are core, intl and extensions.`)
	refused("an unknown level option", adapterOf(echo), Options{Set: setOf(), Levels: []Level{LevelCore, ""}}, `The levels option names the level ""; the levels are core, intl and extensions.`)
	refused("an unclaimed level option", adapterOf(echo), Options{Set: setOf(), Levels: []Level{LevelCore, LevelIntl}}, "The adapter does not claim the intl level.")

	for _, limit := range []struct {
		name  string
		limit func(*Limits)
	}{
		{"output", func(l *Limits) { l.Output = 0 }},
		{"read", func(l *Limits) { l.Read = -1 }},
		{"conversion", func(l *Limits) { l.Conversion = 0 }},
		{"nesting", func(l *Limits) { l.Nesting = 0 }},
	} {
		refused("no "+limit.name+" limit", with(func(a *Adapter) { limit.limit(&a.Limits) }), empty, "The adapter must declare its "+limit.name+" limit as a positive integer.")
	}

	refused("limits checked in order", with(func(a *Adapter) { a.Limits = Limits{} }), empty, "The adapter must declare its output limit as a positive integer.")
	refused("an unknown facility", with(func(a *Adapter) { a.Unexpressible = map[API][]Unexpressible{"Bogus": {{Property: "x"}}} }), empty,
		`The adapter names the facility "Bogus"; the facilities are NumberFormat, DateTimeFormat, RelativeTimeFormat, PluralRules.`)
	refused("an entry naming no property", with(func(a *Adapter) { a.Unexpressible = map[API][]Unexpressible{NumberFormat: {{Values: []any{2.0}}}} }), empty,
		"The adapter must name what it cannot express of a NumberFormat request as entries each naming a property, with the values or the request it cannot be expressed in.")
	refused("a negative reentry", with(func(a *Adapter) { negative := -1; a.Reentry = &negative }), empty, "The adapter must state its bound on counted reads, reentry, as a non-negative integer.")
	refused("a tree without a parse", with(func(a *Adapter) { a.CST = &CST{Unit: UTF8} }), empty, "An adapter that offers a tree must supply the call that produces one as CST.Parse.")
	refused("an unknown unit", with(func(a *Adapter) { a.CST = &CST{Unit: "utf-32", Parse: func(string) *Node { return nil }} }), empty,
		`The adapter counts its spans in "utf-32"; the units are utf-8, utf-16, code-point.`)
	refused("no undefined", with(func(a *Adapter) { a.Undefined = nil }), empty, "The adapter must state the value its implementation reads as nothing passed, as Undefined.")
	refused("no resolve", with(func(a *Adapter) { a.Resolve = nil }), empty, "The adapter must supply the call that resolves a message as Resolve.")

	t.Run("claims within the vocabulary", func(t *testing.T) {
		adapter := with(func(a *Adapter) {
			a.Levels = []Level{LevelCore, LevelIntl, LevelExtensions}
			a.Unexpressible = map[API][]Unexpressible{NumberFormat: {{Property: "useGrouping"}}}
			zero := 0
			a.Reentry = &zero
			a.KeepsNothing = true
			a.CST = &CST{Unit: CodePoint, Parse: func(string) *Node { return nil }}
		})

		if _, err := NewPlan(adapter, Options{Set: setOf(), Levels: []Level{LevelIntl}}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPlan(t *testing.T) {
	t.Run("refuses a file targeting another format", func(t *testing.T) {
		file := fileOf(LevelCore, "9", concreteCase(t, "a/one", ""))
		file.File["format"] = "curly-message-2"
		_, err := NewPlan(adapterOf(echo), Options{Set: setOf(file)})
		expectError(t, err, `The fixture file core.json targets the format "curly-message-2"; this set reads curly-message-3.`)
	})

	t.Run("identifies each case, the case's section over the file's", func(t *testing.T) {
		plan, err := NewPlan(adapterOf(echo), Options{Set: setOf(fileOf(LevelCore, "9", concreteCase(t, "a/one", ""), concreteCase(t, "a/two", `{"section": "9.2"}`)))})

		if err != nil {
			t.Fatal(err)
		}

		want := []Identity{
			{ID: "a/one", File: "core.json", Level: LevelCore, Section: "9", Description: "Pins a/one."},
			{ID: "a/two", File: "core.json", Level: LevelCore, Section: "9.2", Description: "Pins a/two."},
		}

		for i, planned := range plan.Cases {
			if planned.Identity != want[i] {
				t.Errorf("case %d: got %+v, want %+v", i, planned.Identity, want[i])
			}
		}
	})

	t.Run("leaves out what the adapter does not claim, what the option excludes, and a tree it does not offer", func(t *testing.T) {
		tree := Fixture{Name: "cst.json", File: obj(t, `{"format": "curly-message-3", "kind": "tree", "section": "6", "cases": [{"id": "cst/empty", "description": "Pins the empty message.", "message": "", "expected": []}]}`)}
		set := setOf(fileOf(LevelCore, "9", concreteCase(t, "a/one", "")), fileOf(LevelIntl, "11.2", concreteCase(t, "b/one", "")), fileOf(LevelExtensions, "11.3", concreteCase(t, "c/one", "")), tree)
		plan, err := NewPlan(adapterOf(echo, LevelCore, LevelExtensions), Options{Set: set, Levels: []Level{LevelCore}})

		if err != nil {
			t.Fatal(err)
		}

		var lines []string

		for _, s := range plan.Skipped {
			lines = append(lines, s.Line())
		}

		want := []string{
			"SKIP b/one (section 11.2): The adapter does not claim the intl level.",
			"SKIP c/one (section 11.3): The extensions level is not among the levels being run.",
			"SKIP cst/empty (CST.md section 6): The adapter offers no concrete syntax tree.",
		}

		if !reflect.DeepEqual(lines, want) || len(plan.Cases) != 1 {
			t.Errorf("got %q and %d cases, want %q and 1", lines, len(plan.Cases), want)
		}
	})

	t.Run("leaves out a request reading a property the adapter cannot express", func(t *testing.T) {
		c := concreteCase(t, "b/grouping", `{"expected": {"format": {"api": "NumberFormat", "options": {"useGrouping": false, "maximumFractionDigits": 2}, "input": 1}}}`)
		cannot := func(unexpressible map[API][]Unexpressible) Plan {
			adapter := adapterOf(echo, LevelCore, LevelIntl)
			adapter.Unexpressible = unexpressible
			plan, err := NewPlan(adapter, Options{Set: setOf(fileOf(LevelIntl, "11.2", c))})

			if err != nil {
				t.Fatal(err)
			}

			return plan
		}

		for entry, reason := range map[*Unexpressible]string{
			{Property: "useGrouping"}:                                                       "The adapter cannot express the useGrouping property of a NumberFormat request.",
			{Property: "maximumFractionDigits", Values: []any{1.0, 2.0}}:                    "The adapter cannot express the value 2 of the maximumFractionDigits property of a NumberFormat request.",
			{Property: "maximumFractionDigits", When: map[string]any{"useGrouping": false}}: "The adapter cannot express the maximumFractionDigits property of a NumberFormat request whose useGrouping is false.",
		} {
			if got := cannot(map[API][]Unexpressible{NumberFormat: {{Property: "style"}, *entry}}).Skipped; len(got) != 1 || got[0].Reason != reason {
				t.Errorf("%+v: got %+v", *entry, got)
			}
		}

		for _, other := range []map[API][]Unexpressible{
			{NumberFormat: {{Property: "style"}}},
			{DateTimeFormat: {{Property: "useGrouping"}}},
			{NumberFormat: {{Property: "maximumFractionDigits", Values: []any{-2.0, "2"}}}},
			{NumberFormat: {{Property: "maximumFractionDigits", When: map[string]any{"useGrouping": true}}}},
			{NumberFormat: {{Property: "maximumFractionDigits", When: map[string]any{"style": "percent"}}}},
			nil,
		} {
			if got := cannot(other); len(got.Cases) != 1 {
				t.Errorf("%v: got %+v", other, got.Skipped)
			}
		}
	})

	t.Run("hands the adapter the decoded inputs and the behaviours the case names", func(t *testing.T) {
		var got Resolution
		adapter := adapterOf(func(in Resolution) *Resolved {
			got = in

			return &Resolved{Output: "x", Reports: reportsOf()}
		}, LevelCore, LevelExtensions)
		c := concreteCase(t, "c/inputs", `{
			"message": {"$curly": "undefined"},
			"payload": {"v": [1, {"$curly": "undefined"}], "w": {"$curly": "unserializable"}},
			"props": {"number": {"useGrouping": false}},
			"locale": "de",
			"messageId": 7,
			"modifiers": {"shout": "upper"},
			"defaults": {"number": {"maximumFractionDigits": 1}}
		}`)

		if outcome := outcomeOf(t, adapter, c, LevelExtensions, Options{}); !outcome.OK {
			t.Fatal(outcome)
		}

		if got.Message != (absent{}) || got.ID != 7.0 || *got.Locale != "de" {
			t.Errorf("message %v, id %v, locale %v", got.Message, got.ID, got.Locale)
		}

		if v := got.Payload["v"].([]any); v[0] != 1.0 || v[1] != (absent{}) {
			t.Errorf("payload v %v", v)
		}

		if w := got.Payload["w"].(map[string]any); reflect.ValueOf(w["self"]).Pointer() != reflect.ValueOf(w).Pointer() {
			t.Errorf("payload w does not hold itself")
		}

		if !reflect.DeepEqual(got.Props, map[string]any{"number": map[string]any{"useGrouping": false}}) || !reflect.DeepEqual(got.Defaults, map[string]any{"number": map[string]any{"maximumFractionDigits": 1.0}}) {
			t.Errorf("props %v, defaults %v", got.Props, got.Defaults)
		}

		if answer, err := got.Modifiers["shout"](ModifierInput{Value: "ab"}); answer != "AB" || err != nil || len(got.Modifiers) != 1 {
			t.Errorf("modifiers %v", got.Modifiers)
		}
	})

	t.Run("leaves what a case does not write unset, its id the adapter's absence", func(t *testing.T) {
		var got Resolution
		adapter := adapterOf(func(in Resolution) *Resolved {
			got = in

			return &Resolved{Output: "x", Reports: reportsOf()}
		})
		outcomeOf(t, adapter, obj(t, `{"id": "a/bare", "description": "Pins a bare case.", "message": "x", "expected": {"output": "x"}}`), LevelCore, Options{})

		if got.Message != "x" || got.Payload != nil || got.Props != nil || got.Defaults != nil || got.Locale != nil || got.Modifiers != nil || got.ID != (absent{}) {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("decodes the inputs afresh for each execution", func(t *testing.T) {
		var seen []map[string]any
		adapter := adapterOf(func(in Resolution) *Resolved {
			seen = append(seen, in.Payload)

			return &Resolved{Output: "x", Reports: reportsOf()}
		})
		plan, err := NewPlan(adapter, Options{Set: setOf(fileOf(LevelCore, "9", concreteCase(t, "a/one", "")))})

		if err != nil {
			t.Fatal(err)
		}

		plan.Cases[0].Execute()
		plan.Cases[0].Execute()

		if reflect.ValueOf(seen[0]).Pointer() == reflect.ValueOf(seen[1]).Pointer() {
			t.Error("two executions shared one payload")
		}
	})

	t.Run("refuses a case whose input decodes to no object", func(t *testing.T) {
		plan, _ := NewPlan(adapterOf(echo), Options{Set: setOf(fileOf(LevelCore, "9", concreteCase(t, "a/nodes", `{"payload": {"$curly": "nodes", "count": 3}}`)))})
		_, err := plan.Cases[0].Execute()
		expectError(t, err, "The case a/nodes writes its payload as something that decodes to no object.")
	})

	t.Run("reads an input written as the undefined tag as unset", func(t *testing.T) {
		var got Resolution
		adapter := adapterOf(func(in Resolution) *Resolved {
			got = in

			return &Resolved{Output: "x", Reports: reportsOf()}
		})
		outcomeOf(t, adapter, concreteCase(t, "a/undefined", `{"payload": {"$curly": "undefined"}}`), LevelCore, Options{})

		if got.Payload != nil {
			t.Errorf("got %v", got.Payload)
		}
	})
}

func TestRun(t *testing.T) {
	adapter := adapterOf(func(in Resolution) *Resolved {
		switch in.Payload["v"] {
		case "fail":
			return &Resolved{Output: "other", Reports: reportsOf()}
		case "unobserved":
			return &Resolved{Output: "unobserved"}
		}

		return echo(in)
	}, LevelCore)
	set := setOf(
		fileOf(LevelCore, "9", concreteCase(t, "a/pass", ""), concreteCase(t, "a/fail", `{"payload": {"v": "fail"}, "expected": {"output": "fail"}}`), concreteCase(t, "a/unobserved", `{"payload": {"v": "unobserved"}, "expected": {"output": "unobserved"}}`)),
		fileOf(LevelIntl, "11.2", concreteCase(t, "b/skip", "")),
	)
	result, err := Run(adapter, Options{Set: set})

	if err != nil {
		t.Fatal(err)
	}

	ids := func(planned []Planned) (out []string) {
		for _, p := range planned {
			out = append(out, p.ID)
		}

		return out
	}

	if got := ids(result.Passed); !reflect.DeepEqual(got, []string{"a/pass", "a/unobserved"}) {
		t.Errorf("passed %v", got)
	}

	if got := ids(result.Unobserved); !reflect.DeepEqual(got, []string{"a/unobserved"}) {
		t.Errorf("unobserved %v", got)
	}

	if len(result.Failed) != 1 || result.Failed[0].ID != "a/fail" || len(result.Skipped) != 1 || result.Skipped[0].ID != "b/skip" {
		t.Errorf("failed %v, skipped %v", result.Failed, result.Skipped)
	}

	want := strings.Join([]string{
		`FAIL a/fail (section 9): The output differs. Expected "fail", actual "other". Pins a/fail.`,
		"SKIP b/skip (section 11.2): The adapter does not claim the intl level.",
		"2 passed, 1 failed, 1 skipped, 1 passed with reports unobserved",
	}, "\n")

	if got := Summarize(result); got != want {
		t.Errorf("summary\n%s\nwant\n%s", got, want)
	}

	expectError(t, Check(adapter, Options{Set: set}), "1 of 3 conformance cases failed:\n"+`FAIL a/fail (section 9): The output differs. Expected "fail", actual "other". Pins a/fail.`)

	if err := Check(adapterOf(echo), Options{Set: setOf(fileOf(LevelCore, "9", concreteCase(t, "a/pass", "")))}); err != nil {
		t.Error(err)
	}

	if got := Summarize(Result{}); got != "0 passed, 0 failed, 0 skipped" {
		t.Errorf("empty summary %q", got)
	}
}

func TestFailureLineBound(t *testing.T) {
	long := strings.Repeat("y", 1000)
	line := Failed{Planned{Identity: Identity{ID: "a/long", Section: "13", Description: "Pins a long output."}}, failure("The output differs.", long, "x")}.Line()
	want := `FAIL a/long (section 13): The output differs. Expected "` + strings.Repeat("y", 199) + `..., actual "x". Pins a long output.`

	if line != want {
		t.Errorf("got %s", line)
	}

	cyclic := map[string]any{}
	cyclic["self"] = cyclic

	if got := shown(cyclic); got != "a map[string]interface {} JSON cannot describe" {
		t.Errorf("got %s", got)
	}
}

func TestLeftOut(t *testing.T) {
	reasons := func(adapter Adapter, cases ...map[string]any) []string {
		t.Helper()

		plan, err := NewPlan(adapter, Options{Set: setOf(fileOf(adapter.Levels[len(adapter.Levels)-1], "13", cases...))})

		if err != nil {
			t.Fatal(err)
		}

		got := make([]string, len(plan.Skipped))

		for i, s := range plan.Skipped {
			got[i] = s.ID + ": " + s.Reason
		}

		return got
	}
	bound := func(n int, keepsNothing bool) Adapter {
		adapter := adapterOf(echo, LevelCore, LevelExtensions)
		adapter.Reentry = &n
		adapter.KeepsNothing = keepsNothing

		return adapter
	}
	generatedCase := func(construction string) map[string]any {
		return obj(t, `{"id": "g/`+construction+`", "description": "Pins it.", "generate": "`+construction+`"}`)
	}
	keeping := func(id, behaviour string) map[string]any {
		return concreteCase(t, id, `{"keeping": {"k": "`+behaviour+`"}}`)
	}

	t.Run("compares a value of an entry with the sign of zero", func(t *testing.T) {
		adapter := adapterOf(echo, LevelCore, LevelIntl)
		adapter.Unexpressible = map[API][]Unexpressible{RelativeTimeFormat: {{Property: "numeric", When: map[string]any{"style": "long"}}, {Property: "input", Values: []any{math.Copysign(0, -1)}}}}
		c := concreteCase(t, "b/zero", `{"expected": {"format": {"api": "RelativeTimeFormat", "options": {"input": 0}, "input": [0, "day"]}}}`)

		if got := reasons(adapter, c); len(got) != 0 {
			t.Errorf("0 left out as -0: %q", got)
		}

		c["expected"].(map[string]any)["format"].(map[string]any)["options"].(map[string]any)["input"] = math.Copysign(0, -1)

		if got := reasons(adapter, c); !reflect.DeepEqual(got, []string{"b/zero: The adapter cannot express the value -0 of the input property of a RelativeTimeFormat request."}) {
			t.Errorf("got %q", got)
		}
	})

	t.Run("leaves out a case whose inputs hold a refusing tag, at any depth", func(t *testing.T) {
		got := reasons(adapterOf(echo, LevelCore, LevelExtensions),
			concreteCase(t, "c/refusing", `{"props": {"number": {"$curly": "refusing", "members": ["style"]}}}`),
			concreteCase(t, "c/deep", `{"payload": {"v": [{"w": {"$curly": "refusing", "members": []}}]}}`),
			concreteCase(t, "c/written", `{"payload": {"v": {"members": ["refusing"]}}}`))
		want := []string{
			"c/refusing: The host has no value the implementation reads whose entries raise where they are read.",
			"c/deep: The host has no value the implementation reads whose entries raise where they are read.",
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("leaves out the constructions of kept reads where the adapter states no bound", func(t *testing.T) {
		adapter := adapterOf(echo, LevelCore, LevelExtensions)
		got := reasons(adapter, generatedCase("reentry-at-limit"), generatedCase("output-at-limit"), keeping("c/nest", "nest"), keeping("c/first", "first"))

		if !reflect.DeepEqual(got, []string{"g/reentry-at-limit: The adapter states no bound on counted reads."}) {
			t.Errorf("got %q", got)
		}
	})

	t.Run("leaves out own reads and a nest at a bound of 0", func(t *testing.T) {
		got := reasons(bound(0, false), generatedCase("reentry-at-limit"), generatedCase("reentry-own-reads-free"), keeping("c/nest", "nest"), keeping("c/keep", "keep"))
		want := []string{
			"g/reentry-own-reads-free: The adapter states a bound of 0 on counted reads, so no counted read stands for own reads to be made inside.",
			"c/nest: The adapter states a bound of 0 on counted reads, so the resolution a modifier asks for inside another may be refused.",
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}

		if got := reasons(bound(1, false), generatedCase("reentry-own-reads-free"), keeping("c/nest", "nest")); len(got) != 0 {
			t.Errorf("at a bound of 1: %q", got)
		}
	})

	t.Run("leaves out a keep-nest, a keep-nest-ne and the two constructions around them at a bound of 0", func(t *testing.T) {
		nested := "The adapter states a bound of 0 on counted reads, so the resolution a modifier asks for inside another may be refused."
		cases := []map[string]any{generatedCase("reentry-own-read-in-a-nested-resolution"), generatedCase("reentry-over-limit-beneath-a-defined-modifier"), keeping("c/keep-nest", "keep-nest"), keeping("c/keep-nest-ne", "keep-nest-ne")}
		want := []string{"g/reentry-own-read-in-a-nested-resolution: " + nested, "g/reentry-over-limit-beneath-a-defined-modifier: " + nested, "c/keep-nest: " + nested, "c/keep-nest-ne: " + nested}

		for _, resolutions := range []bool{false, true} {
			adapter := bound(0, false)
			adapter.Resolutions = resolutions

			if got := reasons(adapter, cases...); !reflect.DeepEqual(got, want) {
				t.Errorf("got %q, want %q", got, want)
			}
		}

		if got := reasons(bound(1, false), cases...); len(got) != 0 {
			t.Errorf("at a bound of 1: %q", got)
		}
	})

	t.Run("runs the two constructions around a resolution a modifier asks for where no modifier keeps an input, only where they are built from resolutions", func(t *testing.T) {
		keepsNothing := "The adapter states that a modifier cannot keep an input past its call."
		cases := []map[string]any{generatedCase("reentry-own-read-in-a-nested-resolution"), generatedCase("reentry-over-limit-beneath-a-defined-modifier")}

		for _, c := range []struct {
			keepsNothing, resolutions bool
			want                      []string
		}{
			{false, false, nil},
			{true, true, nil},
			{true, false, []string{"g/reentry-own-read-in-a-nested-resolution: " + keepsNothing, "g/reentry-over-limit-beneath-a-defined-modifier: " + keepsNothing}},
		} {
			adapter := bound(2, c.keepsNothing)
			adapter.Resolutions = c.resolutions

			if got := reasons(adapter, cases...); !slices.Equal(got, c.want) {
				t.Errorf("keeping nothing %v, counting resolutions %v: got %q", c.keepsNothing, c.resolutions, got)
			}
		}

		unstated := adapterOf(echo, LevelCore, LevelExtensions)
		unstated.Resolutions = true

		if got := reasons(unstated, cases...); len(got) != 2 || got[1] != "g/reentry-over-limit-beneath-a-defined-modifier: The adapter states no bound on counted reads." {
			t.Errorf("with no bound: %q", got)
		}
	})

	t.Run("runs the construction of resolutions only where the adapter states that its bound counts them", func(t *testing.T) {
		counting := func(n int, resolutions bool, output int) Adapter {
			adapter := bound(n, true)
			adapter.Resolutions = resolutions
			adapter.Limits.Output = output

			return adapter
		}
		unstated := adapterOf(echo, LevelCore, LevelExtensions)
		unstated.Resolutions = true

		for name, c := range map[string]struct {
			adapter Adapter
			want    string
		}{
			"no bound":             {unstated, "The adapter states no bound on counted reads."},
			"resolutions unstated": {counting(2, false, 7), "The adapter does not state that its bound counts a resolution asked for inside another."},
			"an output too short":  {counting(2, true, 6), "The output limit the adapter states does not hold the brackets of as many resolutions as its bound lets stand."},
			"near miss: it runs, though nothing is kept": {counting(2, true, 7), ""},
		} {
			got := reasons(c.adapter, generatedCase("reentry-resolutions-over-limit"))

			if c.want == "" && len(got) != 0 || c.want != "" && !reflect.DeepEqual(got, []string{"g/reentry-resolutions-over-limit: " + c.want}) {
				t.Errorf("%s: got %q", name, got)
			}
		}
	})

	t.Run("leaves out what reads a kept input after its modifier returned where the adapter keeps nothing", func(t *testing.T) {
		got := reasons(bound(3, true), generatedCase("reentry-over-limit"), keeping("c/keep", "keep"), keeping("c/first", "first"), keeping("c/take", "take"), keeping("c/taking", "taking"), keeping("c/nest", "nest"), keeping("c/keep-default", "keep-default"))
		keepsNothing := "The adapter states that a modifier cannot keep an input past its call."
		want := []string{"g/reentry-over-limit: " + keepsNothing, "c/keep: " + keepsNothing, "c/first: " + keepsNothing}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
