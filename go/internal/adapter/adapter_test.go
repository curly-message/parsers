package adapter_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/curly-message/parsers/go/curly"
	"github.com/curly-message/parsers/go/internal/adapter"
	"github.com/curly-message/parsers/go/internal/conformance"
)

// failing are the cases of the set the parser is known to fail, read from
// testdata/failing.txt: one id per line, # starting a comment. Each must name
// a case of the set, so an entry a later set renamed or dropped is noticed
// rather than left listed as a gap.
func failing(t testing.TB) []string {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join("testdata", "failing.txt"))

	if err != nil {
		t.Fatal(err)
	}

	var ids []string

	for line := range strings.Lines(strings.ReplaceAll(string(contents), "\r\n", "\n")) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			ids = append(ids, line)
		}
	}

	if unknown := unknownCases(set(t), ids); len(unknown) > 0 {
		t.Fatalf("testdata/failing.txt lists %q, which name no case of the set", unknown)
	}

	return ids
}

// unknownCases are the ids that name no case of the set.
func unknownCases(s *conformance.Set, ids []string) []string {
	cases := map[string]bool{}

	for _, f := range s.Fixtures {
		for _, c := range f.File["cases"].([]any) {
			if id, ok := c.(map[string]any)["id"].(string); ok {
				cases[id] = true
			}
		}
	}

	var unknown []string

	for _, id := range ids {
		if !cases[id] {
			unknown = append(unknown, id)
		}
	}

	return unknown
}

func TestUnknownCases(t *testing.T) {
	got := unknownCases(set(t), []string{"resolution/wrapper-near-miss-is-value", "resolution/wrapper-near-miss-is-valu", "resolution/no-such-case"})

	if want := []string{"resolution/wrapper-near-miss-is-valu", "resolution/no-such-case"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func set(t testing.TB) *conformance.Set {
	t.Helper()

	s, err := conformance.FromEnvironment()

	if err != nil {
		t.Fatal(err)
	}

	return s
}

// conform runs every case of the set through the adapter, one subtest
// each: a case left out is skipped with its reason, a known failure is
// skipped with its failure line, and any other failure fails. It answers
// the reasons the cases were left out for, and how many ran.
func conform(t *testing.T, a conformance.Adapter, known []string) (map[string]int, int) {
	plan, err := conformance.NewPlan(a, conformance.Options{Set: set(t)})

	if err != nil {
		t.Fatal(err)
	}

	reasons := map[string]int{}

	for _, s := range plan.Skipped {
		reasons[s.Reason]++
		t.Run(s.ID, func(t *testing.T) { t.Skip(s.Line()) })
	}

	for _, c := range plan.Cases {
		t.Run(c.ID, func(t *testing.T) {
			outcome, err := c.Execute()

			if err != nil {
				t.Fatal(err)
			}

			line := conformance.Failed{Planned: c, Outcome: outcome}.Line()

			switch expected := slices.Contains(known, c.ID); {
			case !outcome.OK && expected:
				t.Skip("a known failure, listed in testdata/failing.txt: " + line)
			case !outcome.OK:
				t.Error(line)
			case expected:
				t.Error("passes, though testdata/failing.txt lists it as failing")
			case outcome.Unobserved:
				t.Error("passes with its reports unobserved, though the parser reports")
			}
		})
	}

	return reasons, len(plan.Cases)
}

// intlCases is how many cases of the set are at the Intl level.
func intlCases(t testing.TB) int {
	n := 0

	for _, f := range set(t).Fixtures {
		if f.File["level"] == string(conformance.LevelIntl) {
			n += len(f.File["cases"].([]any))
		}
	}

	return n
}

func TestConformance(t *testing.T) {
	reasons, ran := conform(t, adapter.New(nil, nil), failing(t))
	intl := intlCases(t)

	if want := map[string]int{"The adapter does not claim the intl level.": intl}; !maps.Equal(reasons, want) {
		t.Errorf("left out %v, want %v", reasons, want)
	}

	t.Logf("%d cases ran, %d left out at the Intl level", ran, intl)
}

// supplier is a stand-in for a host's Intl, for running the Intl level
// with every request exposed: a case is then measured on the request the
// parser makes rather than on the text a supplier's locale data writes
// (section 11.2). It answers each request with the facility's initial, a
// selection with one for 1 and other otherwise, and refuses what
// ECMA-402's SetNumberFormatDigitOptions refuses when both are stated: a
// minimumFractionDigits above maximumFractionDigits.
type supplier struct{}

var errRefused = errors.New("minimumFractionDigits is above maximumFractionDigits")

func refuses(options map[string]any) bool {
	least, stated := options["minimumFractionDigits"].(float64)
	most, bounded := options["maximumFractionDigits"].(float64)

	return stated && bounded && least > most
}

func (supplier) NumberFormat(_ string, options map[string]any, _ float64) (string, error) {
	if refuses(options) {
		return "", errRefused
	}

	return "N", nil
}

func (supplier) DateTimeFormat(string, map[string]any, float64) (string, error) {
	return "D", nil
}

func (supplier) RelativeTimeFormat(string, map[string]any, float64, string) (string, error) {
	return "R", nil
}

func (supplier) PluralRules(_ string, options map[string]any, input float64) (curly.Category, error) {
	switch {
	case refuses(options):
		return "", errRefused
	case input == 1:
		return curly.CategoryOne, nil
	}

	return curly.CategoryOther, nil
}

func TestConformanceRequests(t *testing.T) {
	reasons, ran := conform(t, adapter.New(supplier{}, nil), failing(t))

	if len(reasons) > 0 {
		t.Errorf("left out %v", reasons)
	}

	t.Logf("%d cases ran, the Intl level among them", ran)
}

func TestAudit(t *testing.T) {
	without := set(t).Without(failing(t)...)

	for _, c := range []struct {
		name        string
		adapter     conformance.Adapter
		unreachable []string
	}{
		{"at Core and Extensions", adapter.New(nil, nil), []string{"formats-wrong", "unexpressible-declared"}},
		{"with every request exposed", adapter.New(supplier{}, nil), nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			audited, err := conformance.Audit(c.adapter, conformance.Options{Set: without})

			if err != nil {
				t.Fatal(err)
			}

			var unreachable []string

			for _, a := range audited {
				switch a.Outcome {
				case conformance.Unreachable:
					unreachable = append(unreachable, a.ID)
				case conformance.Missed:
					t.Errorf("%s: missed, the runner answered %s where the catalogue expects %s", a.ID, a.Observed, a.Expects)
				}
			}

			if !slices.Equal(unreachable, c.unreachable) {
				t.Errorf("unreachable: %v, want %v", unreachable, c.unreachable)
			}

			t.Logf("%d of %d defects caught", len(audited)-len(unreachable), len(audited))
		})
	}
}
