package conformance

import (
	"encoding/json"
	"fmt"
	"maps"
	"testing"
)

var testLimits = Limits{Output: 5, Read: 7, Conversion: 7, Nesting: 3}

// absent is the test adapters' value for nothing passed.
type absent struct{}

// parsed is JSON as a fixture file holds it.
func parsed(t testing.TB, text string) any {
	t.Helper()

	var value any

	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatalf("%s: %v", text, err)
	}

	return value
}

func obj(t testing.TB, text string) map[string]any {
	t.Helper()

	return parsed(t, text).(map[string]any)
}

func adapterOf(resolve func(Resolution) *Resolved, levels ...Level) Adapter {
	if len(levels) == 0 {
		levels = []Level{LevelCore}
	}

	return Adapter{Levels: levels, Limits: testLimits, Undefined: absent{}, Resolve: resolve}
}

// echo is an implementation of nothing but {{v}}, reporting nothing.
func echo(in Resolution) *Resolved {
	return &Resolved{Output: fmt.Sprint(in.Payload["v"]), Reports: &[]Report{}}
}

func fileOf(level Level, section string, cases ...map[string]any) Fixture {
	written := make([]any, len(cases))

	for i, c := range cases {
		written[i] = c
	}

	return Fixture{Name: string(level) + ".json", File: map[string]any{"format": format, "level": string(level), "section": section, "cases": written}}
}

func setOf(fixtures ...Fixture) *Set {
	return &Set{Version: "test", Fixtures: fixtures}
}

// concreteCase is a case resolving {{v}} over {v: x} to x, with the
// members over replaced.
func concreteCase(t testing.TB, id string, over string) map[string]any {
	t.Helper()

	c := obj(t, fmt.Sprintf(`{"id": %q, "description": "Pins %s.", "message": "{{v}}", "payload": {"v": "x"}, "expected": {"output": "x"}}`, id, id))

	if over != "" {
		maps.Copy(c, obj(t, over))
	}

	return c
}

// outcomeOf runs one case at a level through an adapter.
func outcomeOf(t testing.TB, adapter Adapter, c map[string]any, level Level, options Options) Outcome {
	t.Helper()

	options.Set = setOf(fileOf(level, "9", c))
	plan, err := NewPlan(adapter, options)

	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Cases) != 1 {
		t.Fatalf("planned %d cases, skipped %v", len(plan.Cases), plan.Skipped)
	}

	outcome, err := plan.Cases[0].Execute()

	if err != nil {
		t.Fatal(err)
	}

	return outcome
}

func reportsOf(reports ...Report) *[]Report {
	if reports == nil {
		reports = []Report{}
	}

	return &reports
}

func ptr(s string) *string {
	return &s
}

func expectReason(t testing.TB, outcome Outcome, reason string) {
	t.Helper()

	if outcome.OK || outcome.Reason != reason {
		t.Errorf("got %+v, want a failure: %s", outcome, reason)
	}
}

func expectError(t testing.TB, err error, message string) {
	t.Helper()

	if err == nil || err.Error() != message {
		t.Errorf("got the error %v, want: %s", err, message)
	}
}
