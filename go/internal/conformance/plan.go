package conformance

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
)

var levels = []Level{LevelCore, LevelIntl, LevelExtensions}

var apis = []API{NumberFormat, DateTimeFormat, RelativeTimeFormat, PluralRules}

func joined[T ~string](values []T, separator string) string {
	written := make([]string, len(values))

	for i, value := range values {
		written[i] = string(value)
	}

	return strings.Join(written, separator)
}

// claims holds the adapter's statements about itself, and the levels
// option, to the vocabulary of sections 2, 11.2, 11.5 and 13 and of CST.md
// section 4 before anything runs: a level that is not one of the three, or
// a limit that is not a count, refuses the run rather than skipping or
// failing a case. Go has no undefined of its own, so an adapter states the
// value its implementation reads as nothing passed, and that statement is
// held too.
func claims(adapter Adapter, options Options) error {
	if adapter.Resolve == nil {
		return errors.New("The adapter must supply the call that resolves a message as Resolve.")
	}

	unknown := func(claimed []Level, where string) error {
		for _, level := range claimed {
			if !slices.Contains(levels, level) {
				return fmt.Errorf("%s names the level %s; the levels are core, intl and extensions.", where, shown(string(level)))
			}
		}

		return nil
	}

	if err := unknown(adapter.Levels, "The adapter"); err != nil {
		return err
	}

	if !slices.Contains(adapter.Levels, LevelCore) {
		return errors.New("The adapter must claim the core level.")
	}

	if options.Levels != nil {
		if err := unknown(options.Levels, "The levels option"); err != nil {
			return err
		}

		for _, level := range options.Levels {
			if !slices.Contains(adapter.Levels, level) {
				return fmt.Errorf("The adapter does not claim the %s level.", level)
			}
		}
	}

	for _, limit := range []struct {
		name  string
		value int
	}{
		{"output", adapter.Limits.Output},
		{"read", adapter.Limits.Read},
		{"conversion", adapter.Limits.Conversion},
		{"nesting", adapter.Limits.Nesting},
	} {
		if limit.value < 1 {
			return fmt.Errorf("The adapter must declare its %s limit as a positive integer.", limit.name)
		}
	}

	if adapter.Reentry != nil && *adapter.Reentry < 0 {
		return errors.New("The adapter must state its bound on counted reads, reentry, as a non-negative integer.")
	}

	for _, api := range sortedKeys(apiKeys(adapter.Unexpressible)) {
		if !slices.Contains(apis, API(api)) {
			return fmt.Errorf("The adapter names the facility %s; the facilities are %s.", shown(api), joined(apis, ", "))
		}

		for _, entry := range adapter.Unexpressible[API(api)] {
			if entry.Property == "" {
				return fmt.Errorf("The adapter must name what it cannot express of a %s request as entries each naming a property, with the values or the request it cannot be expressed in.", api)
			}
		}
	}

	if cst := adapter.CST; cst != nil {
		if cst.Parse == nil {
			return errors.New("An adapter that offers a tree must supply the call that produces one as CST.Parse.")
		}

		if !slices.Contains(units, cst.Unit) {
			return fmt.Errorf("The adapter counts its spans in %s; the units are %s.", shown(string(cst.Unit)), joined(units, ", "))
		}
	}

	if adapter.Undefined == nil {
		return errors.New("The adapter must state the value its implementation reads as nothing passed, as Undefined.")
	}

	return nil
}

func apiKeys(declared map[API][]Unexpressible) map[string]bool {
	keys := make(map[string]bool, len(declared))

	for api := range declared {
		keys[string(api)] = true
	}

	return keys
}

// holds says whether a request's properties hold value under name, compared
// as a request is compared.
func holds(options map[string]any, name string, value any) bool {
	held, ok := options[name]

	return ok && same(held, value)
}

// covers is the reason an entry gives a request it covers, or nothing where
// it covers none: the property must be one the request reads, among the
// values the entry lists, in a request whose properties hold every value its
// condition gives.
func covers(entry Unexpressible, api string, options map[string]any) string {
	if _, ok := options[entry.Property]; !ok {
		return ""
	}

	if len(entry.Values) > 0 && !slices.ContainsFunc(entry.Values, func(value any) bool { return holds(options, entry.Property, value) }) {
		return ""
	}

	conditions := sortedKeys(entry.When)

	for _, name := range conditions {
		if !holds(options, name, entry.When[name]) {
			return ""
		}
	}

	var value, condition string

	if len(entry.Values) > 0 {
		written, _ := canonical(options[entry.Property])
		value = "the value " + written + " of "
	}

	if len(conditions) > 0 {
		held := make([]string, len(conditions))

		for i, name := range conditions {
			written, _ := canonical(entry.When[name])
			held[i] = name + " is " + written
		}

		condition = " whose " + strings.Join(held, " and ")
	}

	return fmt.Sprintf("The adapter cannot express %sthe %s property of a %s request%s.", value, entry.Property, api, condition)
}

// unexpressible is why a case whose request reads what the adapter
// documented it cannot express (section 11.2) is left out: it measures the
// host's facility rather than the implementation.
func unexpressible(c map[string]any, adapter Adapter) string {
	if _, generated := c["generate"]; generated {
		return ""
	}

	expected, _ := c["expected"].(map[string]any)
	request, ok := expected["format"].(map[string]any)

	if !ok {
		return ""
	}

	api, _ := request["api"].(string)
	options, _ := request["options"].(map[string]any)

	for _, entry := range adapter.Unexpressible[API(api)] {
		if reason := covers(entry, api, orEmpty(options)); reason != "" {
			return reason
		}
	}

	return ""
}

// refused is why a case whose inputs hold a refusing tag is left out: the
// adapter reads its payload and props as plain maps and slices (section 4),
// so this host has no value whose entries raise where they are read, and
// leaving the tag out would hand over props that refuse nothing.
func refused(c map[string]any) string {
	if _, generated := c["generate"]; generated || !refusingIn(c) {
		return ""
	}

	return "The host has no value the implementation reads whose entries raise where they are read."
}

// later are the behaviours that read a kept input only after the modifier
// that kept it returned: keep reads nothing it keeps, so only a later
// modifier does, and first reads only what an earlier one kept.
var later = []string{"keep", "first"}

const keepsNothing = "The adapter states that a modifier cannot keep an input past its call."

// nests are the behaviours that ask for a resolution inside another.
var nests = []string{"nest", "keep-nest", "keep-nest-ne"}

const nestedAtZero = "The adapter states a bound of 0 on counted reads, so the resolution a modifier asks for inside another may be refused."

// counting is why the construction of resolutions one inside another is
// left out. It keeps no input, so what the adapter states of keeping does
// not reach it; it measures a bound that counts resolutions, and writes two
// brackets for each that stands, which the output limit has to hold.
func counting(adapter Adapter) string {
	switch {
	case adapter.Reentry == nil:
		return "The adapter states no bound on counted reads."
	case !adapter.Resolutions:
		return "The adapter does not state that its bound counts a resolution asked for inside another."
	case len(renestedOutput(*adapter.Reentry)) > adapter.Limits.Output:
		return "The output limit the adapter states does not hold the brackets of as many resolutions as its bound lets stand."
	}

	return ""
}

// kept is why a case built from the bound on counted reads, or one that
// reads a kept input after the modifier that kept it returned, is left out:
// it measures what the adapter stated it does not do. So is a resolution
// asked for inside another at a bound of 0, which an implementation whose
// bound counts resolutions refuses. The two constructions built around a
// resolution a modifier asks for are built from resolutions where the
// adapter states that its bound counts them, and then keep no input past
// its call, so they run whatever it states of keeping.
func kept(c map[string]any, adapter Adapter) string {
	construction, generated := c["generate"].(string)

	if !generated {
		written, _ := c["keeping"].(map[string]any)
		registered := make([]string, 0, len(written))

		for _, behaviour := range written {
			named, _ := behaviour.(string)
			registered = append(registered, named)
		}

		if adapter.KeepsNothing && slices.ContainsFunc(registered, func(b string) bool { return slices.Contains(later, b) }) {
			return keepsNothing
		}

		if adapter.Reentry != nil && *adapter.Reentry == 0 && slices.ContainsFunc(registered, func(b string) bool { return slices.Contains(nests, b) }) {
			return nestedAtZero
		}

		return ""
	}

	if construction == resolutions {
		return counting(adapter)
	}

	if !slices.Contains(counted, construction) {
		return ""
	}

	frame := slices.Contains(framed, construction)

	if adapter.KeepsNothing && !(frame && adapter.Resolutions) {
		return keepsNothing
	}

	if adapter.Reentry == nil {
		return "The adapter states no bound on counted reads."
	}

	if construction == "reentry-own-reads-free" && *adapter.Reentry == 0 {
		return "The adapter states a bound of 0 on counted reads, so no counted read stands for own reads to be made inside."
	}

	if frame && *adapter.Reentry == 0 {
		return nestedAtZero
	}

	return ""
}

// exclusion is why a level's cases are left out, or nothing where they
// run.
func exclusion(level Level, adapter Adapter, options Options) string {
	if !slices.Contains(adapter.Levels, level) {
		return fmt.Sprintf("The adapter does not claim the %s level.", level)
	}

	if options.Levels != nil && !slices.Contains(options.Levels, level) {
		return fmt.Sprintf("The %s level is not among the levels being run.", level)
	}

	return ""
}

// readable refuses a file whose format this set does not read rather than
// skipping it: what a runner could not read is not what an implementation
// passed.
func readable(set *Set) error {
	for _, fixture := range set.Fixtures {
		if written := fixture.File["format"]; written != format {
			return fmt.Errorf("The fixture file %s targets the format %s; this set reads %s.", fixture.Name, shown(written), format)
		}
	}

	return nil
}

// NewPlan is one entry per case the adapter's levels require, each ready to
// run, beside the cases left out and why. A tree file declares no level,
// because CST.md is not one of them: its cases run where the adapter offers
// a tree, and are left out where it does not.
func NewPlan(adapter Adapter, options Options) (Plan, error) {
	var plan Plan

	if err := claims(adapter, options); err != nil {
		return plan, err
	}

	set := options.Set

	if set == nil {
		vendored, err := Vendored()

		if err != nil {
			return plan, err
		}

		set = vendored
	}

	if err := readable(set); err != nil {
		return plan, err
	}

	for _, fixture := range set.Fixtures {
		file := fixture.File
		tree := file["kind"] == "tree"
		fileSection, _ := file["section"].(string)
		fileLevel, _ := file["level"].(string)
		cases, _ := file["cases"].([]any)

		for _, entry := range cases {
			c := entry.(map[string]any)
			id, _ := c["id"].(string)
			description, _ := c["description"].(string)
			caseSection, ok := c["section"].(string)

			if !ok {
				caseSection = fileSection
			}

			identity := Identity{ID: id, File: fixture.Name, Section: caseSection, Description: description}
			var reason string
			var execute func() (Outcome, error)

			if tree {
				identity.Document = "CST.md"

				if adapter.CST == nil {
					reason = "The adapter offers no concrete syntax tree."
				}

				execute = func() (Outcome, error) { return executeTree(adapter, c) }
			} else {
				identity.Level = Level(fileLevel)

				reason = cmp.Or(exclusion(identity.Level, adapter, options), unexpressible(c, adapter), refused(c), kept(c, adapter))

				if _, generated := c["generate"]; generated {
					execute = func() (Outcome, error) { return executeGenerated(adapter, c) }
				} else {
					host := options.Host
					execute = func() (Outcome, error) { return executeConcrete(adapter, c, host) }
				}
			}

			if reason != "" {
				plan.Skipped = append(plan.Skipped, Skipped{identity, reason})
			} else {
				plan.Cases = append(plan.Cases, Planned{identity, execute})
			}
		}
	}

	return plan, nil
}

// Run executes a plan and sorts the outcomes. A case the runner cannot read
// ends the run with an error.
func Run(adapter Adapter, options Options) (Result, error) {
	var result Result
	plan, err := NewPlan(adapter, options)

	if err != nil {
		return result, err
	}

	result.Skipped = plan.Skipped

	for _, planned := range plan.Cases {
		outcome, err := planned.Execute()

		if err != nil {
			return Result{}, err
		}

		switch {
		case !outcome.OK:
			result.Failed = append(result.Failed, Failed{planned, outcome})
		case outcome.Unobserved:
			result.Passed = append(result.Passed, planned)
			result.Unobserved = append(result.Unobserved, planned)
		default:
			result.Passed = append(result.Passed, planned)
		}
	}

	return result, nil
}

// where is where the sentence a case pins is written. The document is
// named only where it is not the specification.
func where(identity Identity) string {
	if identity.Document != "" {
		return identity.Document + " section " + identity.Section
	}

	return "section " + identity.Section
}

// Line is the line a failure is printed as.
func (f Failed) Line() string {
	return fmt.Sprintf("FAIL %s (%s): %s Expected %s, actual %s. %s", f.ID, where(f.Identity), f.Outcome.Reason, shown(f.Outcome.Expected), shown(f.Outcome.Actual), f.Description)
}

// Line is the line a case left out is printed as.
func (s Skipped) Line() string {
	return fmt.Sprintf("SKIP %s (%s): %s", s.ID, where(s.Identity), s.Reason)
}

// Summarize is the text a run prints: one line per failure and per case
// left out, then the counts.
func Summarize(result Result) string {
	var lines []string

	for _, f := range result.Failed {
		lines = append(lines, f.Line())
	}

	for _, s := range result.Skipped {
		lines = append(lines, s.Line())
	}

	counts := fmt.Sprintf("%d passed, %d failed, %d skipped", len(result.Passed), len(result.Failed), len(result.Skipped))

	if n := len(result.Unobserved); n > 0 {
		counts += fmt.Sprintf(", %d passed with reports unobserved", n)
	}

	return strings.Join(append(lines, counts), "\n")
}

// Check runs the set and answers an error listing every failure, where
// anything failed.
func Check(adapter Adapter, options Options) error {
	result, err := Run(adapter, options)

	if err != nil {
		return err
	}

	if len(result.Failed) == 0 {
		return nil
	}

	lines := make([]string, len(result.Failed))

	for i, f := range result.Failed {
		lines[i] = f.Line()
	}

	return fmt.Errorf("%d of %d conformance cases failed:\n%s", len(result.Failed), len(result.Passed)+len(result.Failed), strings.Join(lines, "\n"))
}
