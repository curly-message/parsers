package conformance

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
)

// reading is a resolver of the messages the constructions of kept reads
// write, as section 13 reads them: what a counted read is, the bound it
// meets, the refusal of a read whose own production is in progress, and
// which modifier is current where a modifier, the conversion of its answer
// or a resolution asked for inside another makes the read. It reads
// placeholders of a key, a host-defined modifier or the format's own ne,
// and options, and nothing else the format holds; a modifier registered
// under ne is called in place of the format's, as a host-defined one.
//
// It counts an own read as the walk's wherever the modifier is current and
// the read's own production is not in progress, so it never takes the
// permission section 13 gives to count one while an option value or default
// of another placeholder, begun beneath the modifier's call, is in
// production. With definedNotCurrent, ne is not current while it produces
// the option it selected: the one way to read section 13 wrongly that
// reentry-over-limit-beneath-a-defined-modifier is there to tell apart, and
// that no defect of the catalogue can make of an implementation.
type reading struct {
	// bound is the bound on counted reads the implementation keeps.
	bound int
	// resolutions is whether the bound counts a resolution asked for
	// inside another as well, as section 13 lets it.
	resolutions       bool
	definedNotCurrent bool
}

// part is text, or a placeholder where p is set.
type part struct {
	text string
	p    *written
}

type written struct {
	key, modifier string
	options       []writtenOption
	fallback      []part
}

type writtenOption struct {
	key   string
	value []part
}

// closing is the end of the placeholder opening at from, past its closing
// braces.
func closing(text string, from int) int {
	depth := 0

	for at := from; at < len(text); {
		switch {
		case strings.HasPrefix(text[at:], "{{"):
			depth++
			at += 2
		case strings.HasPrefix(text[at:], "}}"):
			depth--
			at += 2

			if depth == 0 {
				return at
			}
		default:
			at++
		}
	}

	panic("an unclosed placeholder in " + text)
}

// segments is the text split at each ; no nested placeholder holds.
func segments(text string) []string {
	var found []string
	start := 0

	for at := 0; at < len(text); {
		switch {
		case strings.HasPrefix(text[at:], "{{"):
			at = closing(text, at)
		case text[at] == ';':
			found = append(found, text[start:at])
			at++
			start = at
		default:
			at++
		}
	}

	return append(found, text[start:])
}

func parseWritten(text string) []part {
	var parts []part
	start := 0

	for {
		at := strings.Index(text[start:], "{{")

		if at < 0 {
			return append(parts, part{text: text[start:]})
		}

		at += start
		end := closing(text, at)
		all := segments(text[at+2 : end-2])
		key, modifier, _ := strings.Cut(strings.TrimSpace(all[0]), ":")
		p := &written{key: key, modifier: modifier}

		for _, segment := range all[1:] {
			if strings.TrimSpace(segment) == "" {
				continue
			}

			name, value, _ := strings.Cut(segment, ":")

			if name = strings.TrimSpace(name); name == "default" {
				p.fallback = parseWritten(value)
			} else {
				p.options = append(p.options, writtenOption{name, parseWritten(value)})
			}
		}

		parts = append(parts, part{text: text[start:at]}, part{p: p})
		start = end
	}
}

// resolver answers each resolution asked of it, all on one thread of
// execution, which a resolution asked for inside another shares: the counted
// reads producing their text, the resolutions in progress, and the
// placeholders whose modifiers are current, the innermost last. A resolution
// it refuses answers nil.
func (r reading) resolver() func(Resolution) *Resolved {
	counted, active := 0, 0
	var current []*written

	return func(in Resolution) *Resolved {
		inside := r.resolutions && active > 0

		if inside && counted+1 > r.bound {
			return nil
		}

		active++

		if inside {
			counted++
		}

		defer func() {
			active--

			if inside {
				counted--
			}
		}()

		reports := []Report{}
		report := func(code, origin string) {
			id, _ := in.ID.(string)
			reports = append(reports, Report{Code: code, Origin: origin, ID: &id})
		}

		var resolve func(*written) string

		render := func(parts []part) string {
			var b strings.Builder

			for _, each := range parts {
				if each.p == nil {
					b.WriteString(each.text)
				} else {
					b.WriteString(resolve(each.p))
				}
			}

			return b.String()
		}

		// reader reads the default of p: the walk's where the modifier of p
		// is current and the default is not being produced, and counted
		// otherwise.
		reader := func(p *written) func() string {
			var text string
			produced, producing := false, false

			return func() string {
				switch {
				case produced:
					return text
				case producing:
					return ""
				}

				counts := len(current) == 0 || current[len(current)-1] != p

				if counts && counted+1 > r.bound {
					return ""
				}

				if counts {
					counted++
				}

				producing = true
				text, produced = render(p.fallback), true
				producing = false

				if counts {
					counted--
				}

				return text
			}
		}

		resolve = func(p *written) string {
			value, present := in.Payload[p.key]
			behaviour, registered := in.Modifiers[p.modifier]

			if p.modifier == "ne" && !registered {
				for _, option := range p.options {
					if present && option.key != fmt.Sprint(value) {
						if !r.definedNotCurrent {
							current = append(current, p)
							defer func() { current = current[:len(current)-1] }()
						}

						return render(option.value)
					}
				}

				return render(p.fallback)
			}

			switch {
			case p.modifier != "" && !registered:
				report("unknown-modifier", "message")

				return render(p.fallback)
			case !present:
				return render(p.fallback)
			case p.modifier == "":
				return fmt.Sprint(value)
			}

			options := make([]OptionInput, len(p.options))

			for i, option := range p.options {
				options[i] = OptionInput{option.key, func() string { return render(option.value) }}
			}

			current = append(current, p)
			answer, err := behaviour(ModifierInput{Value: fmt.Sprint(value), Options: options, Props: map[string]any{}, Default: reader(p)})
			current = current[:len(current)-1]

			if err != nil {
				report("failed-modifier", "payload")

				return render(p.fallback)
			}

			return fmt.Sprint(answer)
		}

		message, _ := in.Message.(string)
		output := render(parseWritten(message))

		return &Resolved{Output: output, Reports: &reports}
	}
}

// framedOutcomes is what one of the two constructions around a resolution a
// modifier asks for answers over r at the bounds 1 to 3, r keeping the
// stated bound plus by and stated is what its adapter states of counting
// resolutions: "ok" where it passes, and the output otherwise.
func framedOutcomes(t *testing.T, generate string, r reading, by int, stated bool) []string {
	t.Helper()

	got := make([]string, 0, 3)

	for bound := 1; bound <= 3; bound++ {
		kept := r
		kept.bound = bound + by
		adapter := adapterOf(kept.resolver(), LevelCore, LevelExtensions)
		adapter.Limits.Output = 100
		adapter.Limits.Nesting = 4
		adapter.Reentry = &bound
		adapter.Resolutions = stated
		outcome := outcomeOf(t, adapter, generatedCase("limits-extensions/"+generate, generate), LevelExtensions, Options{})

		if outcome.OK {
			got = append(got, "ok")
		} else {
			got = append(got, fmt.Sprint(outcome.Actual))
		}
	}

	return got
}

func TestFramed(t *testing.T) {
	own := "reentry-own-read-in-a-nested-resolution"
	each := func(output string) string {
		return fmt.Sprint([]string{output, output, output})
	}
	expect := func(t *testing.T, name string, got []string, want string) {
		t.Helper()

		if fmt.Sprint(got) != want {
			t.Errorf("%s: got %v, want %s", name, got, want)
		}
	}

	// A modifier the format defines is current while it runs, so the read
	// made beneath it is counted, and refused at the bound: the first where
	// the bound counts the resolution, the second where it counts reads
	// alone. Over an implementation in which ne is not current the read is
	// left to the modifier that kept the default, and the case fails with
	// <z>, which a runner that registered a modifier of its own under ne,
	// or built the option's text before the comparison ran, would pass.
	t.Run("fails reentry-over-limit-beneath-a-defined-modifier, answering <z>, over an implementation in which a modifier the format defines is not current", func(t *testing.T) {
		for _, c := range []struct{ resolutions, stated bool }{{false, false}, {true, true}, {true, false}} {
			name := fmt.Sprintf("counting resolutions %v, stated %v", c.resolutions, c.stated)
			expect(t, name, framedOutcomes(t, defined, reading{resolutions: c.resolutions}, 0, c.stated), each("ok"))
			expect(t, name+", ne not current", framedOutcomes(t, defined, reading{resolutions: c.resolutions, definedNotCurrent: true}, 0, c.stated), each("<z>"))
			// A bound kept one higher than stated lets the read through.
			expect(t, name+", a bound one higher", framedOutcomes(t, defined, reading{resolutions: c.resolutions}, 1, c.stated), each("<z>"))
		}
	})

	// The modifier that asks for a resolution stays current in it, so its
	// read there is free at the bound, whatever modifier of the format is
	// current beneath it elsewhere.
	t.Run("passes reentry-own-read-in-a-nested-resolution however it is built, whether or not ne is current", func(t *testing.T) {
		for _, c := range []struct{ resolutions, stated bool }{{false, false}, {true, true}, {true, false}} {
			for _, notCurrent := range []bool{false, true} {
				expect(t, fmt.Sprintf("counting resolutions %v, stated %v, ne not current %v", c.resolutions, c.stated, notCurrent), framedOutcomes(t, own, reading{resolutions: c.resolutions, definedNotCurrent: notCurrent}, 0, c.stated), each("ok"))
			}
		}
	})

	// Built by resolutions, the second shows a bound kept one lower at every
	// bound by its output, where the first shows it only by a report.
	t.Run("fails a bound kept one lower than stated where it counts resolutions", func(t *testing.T) {
		expect(t, "counting resolutions", framedOutcomes(t, defined, reading{resolutions: true}, -1, true), each("<z>"))
		expect(t, "counting reads alone", framedOutcomes(t, defined, reading{}, -1, false), each("ok"))
	})
}

// The defects of the bound reach the runner through the two constructions
// around a resolution a modifier asks for only as each is built for the
// adapter.
func TestAuditFramed(t *testing.T) {
	own := "reentry-own-read-in-a-nested-resolution"
	vendored, err := Vendored()

	if err != nil {
		t.Fatal(err)
	}

	audited := []string{"reentry-limit-overstated", "reentry-limit-understated", "resolutions-overstated"}
	only := func(constructions ...string) *Set {
		cases := make([]map[string]any, len(constructions))

		for i, construction := range constructions {
			cases[i] = generatedCase("limits-extensions/"+construction, construction)
		}

		set := setOf(fileOf(LevelExtensions, "13", cases...))

		for _, defect := range vendored.Defects {
			if slices.Contains(audited, defect.ID) {
				set.Defects = append(set.Defects, defect)
			}
		}

		return set
	}
	// over is an adapter of r stating a bound of reentry, and resolutions as
	// r reads them unless told otherwise.
	over := func(r reading, reentry int, change ...func(*Adapter)) Adapter {
		adapter := adapterOf(r.resolver(), LevelCore, LevelExtensions)
		adapter.Limits.Output = 100
		adapter.Limits.Nesting = 4
		adapter.Reentry = &reentry
		adapter.Resolutions = r.resolutions

		for _, c := range change {
			c(&adapter)
		}

		return adapter
	}
	keepsNothing := func(a *Adapter) { a.KeepsNothing = true }
	unstated := func(a *Adapter) { a.Resolutions = false }
	unobserved := func(a *Adapter) {
		resolve := a.Resolve
		a.Resolve = func(in Resolution) *Resolved {
			resolved := resolve(in)

			if resolved != nil {
				resolved.Reports = nil
			}

			return resolved
		}
	}

	for name, c := range map[string]struct {
		set     *Set
		adapter Adapter
		want    [3]string
	}{
		// Built from kept reads they stand a place short of a bound that
		// counts reads alone, so an overstated bound shows in neither, and
		// an understated one lets the refused read through.
		"the near miss from kept reads": {only(own), over(reading{bound: 3}, 3), [3]string{Unreachable, Unreachable, Unreachable}},
		"the defined from kept reads":   {only(defined), over(reading{bound: 3}, 3), [3]string{Unreachable, Caught, Caught}},
		// Built from resolutions, the near miss answers the same where the
		// resolution is refused, and neither needs an input kept.
		"the near miss from resolutions":             {only(own), over(reading{bound: 3, resolutions: true}, 3), [3]string{Unreachable, Unreachable, Unreachable}},
		"the defined from resolutions":               {only(defined), over(reading{bound: 3, resolutions: true}, 3), [3]string{Caught, Caught, Unreachable}},
		"the defined from resolutions, nothing kept": {only(defined), over(reading{bound: 3, resolutions: true}, 3, keepsNothing), [3]string{Caught, Caught, Unreachable}},
		"both from kept reads, nothing kept":         {only(own, defined), over(reading{bound: 3}, 3, keepsNothing), [3]string{Unreachable, Unreachable, Caught}},
		// Claimed of an implementation whose bound does count resolutions,
		// the claim holds, so nothing tells it apart.
		"the defined counting resolutions, unstated":   {only(defined), over(reading{bound: 3, resolutions: true}, 3, unstated), [3]string{Unreachable, Caught, Missed}},
		"the defined stated one lower at a bound of 1": {only(defined), over(reading{bound: 1}, 1), [3]string{Unreachable, Unreachable, Unreachable}},
		// Built from kept reads at a stated bound of 1, the read beneath ne
		// is refused over a bound of 0 as well.
		"the defined over a bound of 0":               {only(defined), over(reading{bound: 0}, 0), [3]string{Unreachable, Unreachable, Unreachable}},
		"the defined over no bound":                   {only(defined), over(reading{bound: 3}, 3, func(a *Adapter) { a.Reentry = nil }), [3]string{Unreachable, Unreachable, Unreachable}},
		"the near miss over a bound of 0, observed":   {only(own), over(reading{resolutions: true}, 0), [3]string{Caught, Unreachable, Unreachable}},
		"the near miss over a bound of 0, unobserved": {only(own), over(reading{resolutions: true}, 0, unobserved), [3]string{Unreachable, Unreachable, Unreachable}},
		"the defined over a bound of 0, unobserved":   {only(defined), over(reading{resolutions: true}, 0, unobserved), [3]string{Caught, Unreachable, Unreachable}},
	} {
		got, err := Audit(c.adapter, Options{Set: c.set})

		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		outcomes := map[string]string{}

		for _, a := range got {
			outcomes[a.ID] = a.Outcome
		}

		if want := map[string]string{audited[0]: c.want[0], audited[1]: c.want[1], audited[2]: c.want[2]}; !maps.Equal(outcomes, want) {
			t.Errorf("%s: got %v, want %v", name, outcomes, want)
		}
	}
}
