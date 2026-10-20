package conformance

import (
	"maps"
	"reflect"
	"slices"
	"testing"
)

// The audit is held here to a set and an adapter made for each other: an
// oracle that answers every case of the set as it expects, built so that
// each defect of the catalogue has something to alter. "{a|b}" and the
// astral character after it are a tree no grammar need agree with; the
// runner holds a tree to the properties of CST.md and to the case alone.
const auditMessage = "{a|b}\U0001F600"

func auditTree() *Node {
	yes := true

	return message(0, 9,
		&Node{Type: "placeholder", Start: 0, End: 5, Nodes: []*Node{
			leaf("open", 0, 1),
			{Type: "key", Start: 1, End: 2, Name: ptr("x")},
			leaf("separator", 2, 3),
			{Type: "escape", Start: 3, End: 4, Cancels: &yes},
			leaf("close", 4, 5),
		}},
		leaf("text", 5, 9),
	)
}

func auditSet(t testing.TB, tree bool) *Set {
	t.Helper()

	vendored, err := Vendored()

	if err != nil {
		t.Fatal(err)
	}

	core := fileOf(LevelCore, "9",
		obj(t, `{"id": "audit/whitespace", "description": "Pins an output with whitespace at both ends.", "message": " {{v}} ", "payload": {"v": "x"}, "expected": {"output": " x ", "reports": []}}`),
		obj(t, `{"id": "audit/reports", "description": "Pins two reports.", "message": "{{a:m}}{{b:n}}", "expected": {"output": "", "reports": [{"code": "unknown-modifier", "origin": "message", "id": "a"}, {"code": "missing-options", "origin": "payload", "id": "b"}]}}`),
		obj(t, `{"id": "audit/read-over-limit", "description": "Pins a limit.", "generate": "read-over-limit"}`),
	)
	intl := fileOf(LevelIntl, "11.2",
		obj(t, `{"id": "audit/format", "description": "Pins a request.", "message": "{{v:number; maximumFractionDigits:1}}", "payload": {"v": 1.5}, "expected": {"format": {"api": "NumberFormat", "options": {"maximumFractionDigits": 1}, "input": 1.5}}}`),
	)
	set := &Set{Version: "audit", Fixtures: []Fixture{core, intl}, Defects: vendored.Defects}

	if tree {
		set.Fixtures = append(set.Fixtures, Fixture{Name: "tree.json", File: obj(t, `{"format": "curly-message-3", "kind": "tree", "section": "6", "cases": [
			{"id": "audit/tree", "description": "Pins a tree.", "message": "{a|b}\ud83d\ude00", "expected": [
				{"type": "placeholder", "text": "{a|b}", "nodes": [
					{"type": "open", "text": "{"}, {"type": "key", "text": "a", "name": "x"}, {"type": "separator", "text": "|"},
					{"type": "escape", "text": "b", "cancels": true}, {"type": "close", "text": "}"}
				]},
				{"type": "text", "text": "\ud83d\ude00"}
			]}
		]}`)})
	}

	return set
}

func auditAdapter(levels []Level, tree bool) Adapter {
	limit := limitsID
	a, b := "a", "b"
	answers := map[string]Resolved{
		" {{v}} ":                               {Output: " x ", Reports: reportsOf()},
		"{{a:m}}{{b:n}}":                        {Output: "", Reports: reportsOf(Report{Code: "unknown-modifier", Origin: "message", ID: &a}, Report{Code: "missing-options", Origin: "payload", ID: &b})},
		reads + "{{w}}":                         {Output: "ok", Reports: reportsOf(Report{Code: "read-limit", Origin: "limit", ID: &limit, Limit: testLimits.Read})},
		auditMessage:                            {Output: auditMessage, Reports: reportsOf()},
		"{{v:number; maximumFractionDigits:1}}": {Output: "1.5", Reports: reportsOf(), Formats: &[]Request{{API: NumberFormat, Options: map[string]any{"maximumFractionDigits": 1.0}, Input: 1.5}}},
	}
	adapter := adapterOf(func(in Resolution) *Resolved {
		text, _ := in.Message.(string)
		answer, ok := answers[text]

		if !ok {
			return &Resolved{Output: "an unknown message", Reports: reportsOf()}
		}

		return &answer
	}, levels...)

	if tree {
		adapter.CST = &CST{Unit: UTF8, Parse: func(string) *Node { return auditTree() }}
	}

	return adapter
}

func outcomes(audited []Audited) map[string]string {
	got := map[string]string{}

	for _, a := range audited {
		got[a.ID] = a.Outcome + " (" + string(a.Observed) + ")"
	}

	return got
}

func TestAudit(t *testing.T) {
	t.Run("catches every defect of the catalogue where each reaches the runner", func(t *testing.T) {
		set := auditSet(t, true)
		audited, err := Audit(auditAdapter([]Level{LevelCore, LevelIntl}, true), Options{Set: set})

		if err != nil {
			t.Fatal(err)
		}

		if len(audited) != 26 {
			t.Fatalf("audited %d defects", len(audited))
		}

		for _, a := range audited {
			if a.Outcome != Caught {
				t.Errorf("%s: %s, observed %s, expected %s", a.ID, a.Outcome, a.Observed, a.Expects)
			}
		}
	})

	t.Run("answers unreachable where the adapter carries nothing the defect alters", func(t *testing.T) {
		set := auditSet(t, false)
		set.Fixtures = set.Fixtures[:1]
		audited, err := Audit(auditAdapter([]Level{LevelCore}, false), Options{Set: set})

		if err != nil {
			t.Fatal(err)
		}

		var unreachable []string

		for _, a := range audited {
			if a.Outcome == Unreachable {
				unreachable = append(unreachable, a.ID)
			} else if a.Outcome != Caught {
				t.Errorf("%s: %s, observed %s", a.ID, a.Outcome, a.Observed)
			}
		}

		want := []string{"formats-wrong", "claims-core-only", "unexpressible-declared", "tree-unoffered", "tree-node-dropped", "tree-node-retyped", "tree-span-shifted", "tree-name-raw", "tree-cancels-inverted", "tree-unit-changed", "tree-unit-unknown"}

		if slices.Sort(unreachable); !slices.Equal(unreachable, slices.Sorted(slices.Values(want))) {
			t.Errorf("unreachable: %v", unreachable)
		}
	})

	t.Run("answers missed where the runner answers other than the defect expects", func(t *testing.T) {
		set := auditSet(t, false)
		set.Fixtures = set.Fixtures[:1]
		set.Defects = slices.Clone(set.Defects)

		for i := range set.Defects {
			if set.Defects[i].ID == "output-truncated" {
				set.Defects[i].Expects = VerdictSkip
			}
		}

		audited, _ := Audit(auditAdapter([]Level{LevelCore}, false), Options{Set: set})

		if got := outcomes(audited)["output-truncated"]; got != "missed (fail)" {
			t.Errorf("got %s", got)
		}
	})

	t.Run("refuses an adapter that fails the set", func(t *testing.T) {
		set := auditSet(t, false)
		adapter := auditAdapter([]Level{LevelCore, LevelIntl}, false)
		resolve := adapter.Resolve
		adapter.Resolve = func(in Resolution) *Resolved {
			if in.Message == " {{v}} " {
				return &Resolved{Output: "x", Reports: reportsOf()}
			}

			return resolve(in)
		}

		_, err := Audit(adapter, Options{Set: set})
		expectError(t, err, "The audit needs an adapter that passes the set; this one fails 1 of 4 cases.")
	})

	t.Run("refuses a defect it does not carry", func(t *testing.T) {
		set := auditSet(t, false)
		set.Defects = append(slices.Clone(set.Defects), Defect{ID: "invented", Description: "Breaks.", Expects: VerdictFail})

		_, err := Audit(auditAdapter([]Level{LevelCore, LevelIntl}, false), Options{Set: set})
		expectError(t, err, `The catalogue names a defect this runner does not carry: "invented".`)
	})

	t.Run("carries exactly the catalogue", func(t *testing.T) {
		vendored, _ := Vendored()
		var ids []string

		for _, d := range vendored.Defects {
			ids = append(ids, d.ID)
		}

		if slices.Sort(ids); !slices.Equal(ids, slices.Sorted(maps.Keys(Mutations))) {
			t.Errorf("the catalogue names %v", ids)
		}
	})
}

func TestCopyTree(t *testing.T) {
	shared := leaf("text", 0, 1)
	root := message(0, 2, shared, shared)
	root.Nodes = append(root.Nodes, root)
	copied := copyTree(root)

	if copied == root || copied.Nodes[0] == shared || copied.Nodes[0] != copied.Nodes[1] || copied.Nodes[2] != copied {
		t.Fatal("the copy shares a node with the tree, or splits one the tree holds twice")
	}

	copied.Nodes[0].Type = "space"
	copied.Nodes = copied.Nodes[:1]

	if shared.Type != "text" || len(root.Nodes) != 3 || !reflect.DeepEqual(copyTree(nil), (*Node)(nil)) {
		t.Error("altering the copy altered the tree")
	}
}
