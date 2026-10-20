package conformance

import (
	"reflect"
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
	refused("an unknown facility", with(func(a *Adapter) { a.Unexpressible = map[API][]string{"Bogus": {"x"}} }), empty,
		`The adapter names the facility "Bogus"; the facilities are NumberFormat, DateTimeFormat, RelativeTimeFormat, PluralRules.`)
	refused("a tree without a parse", with(func(a *Adapter) { a.CST = &CST{Unit: UTF8} }), empty, "An adapter that offers a tree must supply the call that produces one as CST.Parse.")
	refused("an unknown unit", with(func(a *Adapter) { a.CST = &CST{Unit: "utf-32", Parse: func(string) *Node { return nil }} }), empty,
		`The adapter counts its spans in "utf-32"; the units are utf-8, utf-16, code-point.`)
	refused("no undefined", with(func(a *Adapter) { a.Undefined = nil }), empty, "The adapter must state the value its implementation reads as nothing passed, as Undefined.")
	refused("no resolve", with(func(a *Adapter) { a.Resolve = nil }), empty, "The adapter must supply the call that resolves a message as Resolve.")

	t.Run("claims within the vocabulary", func(t *testing.T) {
		adapter := with(func(a *Adapter) {
			a.Levels = []Level{LevelCore, LevelIntl, LevelExtensions}
			a.Unexpressible = map[API][]string{NumberFormat: {"useGrouping"}}
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
		cannot := func(unexpressible map[API][]string) Plan {
			adapter := adapterOf(echo, LevelCore, LevelIntl)
			adapter.Unexpressible = unexpressible
			plan, err := NewPlan(adapter, Options{Set: setOf(fileOf(LevelIntl, "11.2", c))})

			if err != nil {
				t.Fatal(err)
			}

			return plan
		}

		if got := cannot(map[API][]string{NumberFormat: {"useGrouping"}}).Skipped; len(got) != 1 || got[0].Reason != "The adapter cannot express the useGrouping property of a NumberFormat request." {
			t.Errorf("got %+v", got)
		}

		for _, other := range []map[API][]string{{NumberFormat: {"style"}}, {DateTimeFormat: {"useGrouping"}}, nil} {
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
