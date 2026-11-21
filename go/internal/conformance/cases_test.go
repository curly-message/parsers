package conformance

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func answers(t testing.TB, resolved *Resolved) Adapter {
	t.Helper()

	return adapterOf(func(Resolution) *Resolved { return resolved }, LevelCore, LevelIntl)
}

func TestExecute(t *testing.T) {
	plain := concreteCase(t, "a/plain", "")
	reported := concreteCase(t, "a/reported", `{"expected": {"output": "x", "reports": [
		{"code": "unknown-modifier", "origin": "message", "id": "a.id"},
		{"code": "output-limit", "origin": "limit"}
	]}}`)
	run := func(c map[string]any, resolved *Resolved) Outcome {
		return outcomeOf(t, answers(t, resolved), c, LevelCore, Options{})
	}

	t.Run("passes where the output and the reports match", func(t *testing.T) {
		if got := run(plain, &Resolved{Output: "x", Reports: reportsOf()}); got != (Outcome{OK: true}) {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("compares the output exactly", func(t *testing.T) {
		for _, output := range []string{" x", "x ", "X", "", "x\n"} {
			got := run(plain, &Resolved{Output: output, Reports: reportsOf()})
			expectReason(t, got, "The output differs.")

			if got.Expected != "x" || got.Actual != output {
				t.Errorf("got %+v", got)
			}
		}
	})

	t.Run("fails where the adapter raised, and the run goes on", func(t *testing.T) {
		raising := func(value any) Adapter {
			return adapterOf(func(Resolution) *Resolved { panic(value) })
		}

		for value, actual := range map[any]string{"plain": "plain", errors.New("an error"): "an error", 42: "42"} {
			got := outcomeOf(t, raising(value), plain, LevelCore, Options{})
			expectReason(t, got, "The adapter raised.")

			if got.Actual != actual {
				t.Errorf("got %+v, want %s", got, actual)
			}
		}

		cyclic := map[string]any{}
		cyclic["self"] = cyclic
		got := outcomeOf(t, raising(cyclic), plain, LevelCore, Options{})

		if got.Actual != "a map[string]interface {} JSON cannot describe" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("fails where the adapter answered with no resolution", func(t *testing.T) {
		got := run(plain, nil)
		expectReason(t, got, "The adapter answered with no resolution.")
	})

	t.Run("compares reports by count, then code, in order", func(t *testing.T) {
		expectReason(t, run(reported, &Resolved{Output: "x", Reports: reportsOf()}), "Expected 2 reports, got 0 reports.")
		expectReason(t, run(plain, &Resolved{Output: "x", Reports: reportsOf(Report{Code: "read-limit"})}), "Expected 0 reports, got 1 report.")
		expectReason(t, run(reported, &Resolved{Output: "x", Reports: reportsOf(Report{Code: "output-limit"}, Report{Code: "unknown-modifier"})}), "The code of report 1 differs.")
		expectReason(t, run(reported, &Resolved{Output: "x", Reports: reportsOf(Report{Code: "unknown-modifier"}, Report{Code: "read-limit"})}), "The code of report 2 differs.")
	})

	t.Run("compares origin, id and limit only where both sides carry them", func(t *testing.T) {
		pass := []Report{{Code: "unknown-modifier"}, {Code: "output-limit"}}

		if got := run(reported, &Resolved{Output: "x", Reports: &pass}); !got.OK {
			t.Errorf("unobserved fields: got %+v", got)
		}

		full := []Report{{Code: "unknown-modifier", Origin: "message", ID: ptr("a.id"), Limit: 9}, {Code: "output-limit", Origin: "limit", ID: ptr("other"), Limit: 5}}

		if got := run(reported, &Resolved{Output: "x", Reports: &full}); !got.OK {
			t.Errorf("fields the case expects nothing of: got %+v", got)
		}

		expectReason(t, run(reported, &Resolved{Output: "x", Reports: reportsOf(Report{Code: "unknown-modifier", Origin: "payload"}, pass[1])}), "The origin of report 1 differs.")
		expectReason(t, run(reported, &Resolved{Output: "x", Reports: reportsOf(Report{Code: "unknown-modifier", ID: ptr("b.id")}, pass[1])}), "The id of report 1 differs.")

		limited := generatedCase("limits/over", "nesting-over-limit")
		got := outcomeOf(t, answers(t, &Resolved{Output: "fallback", Reports: reportsOf(Report{Code: "nesting-limit", Limit: 4})}), limited, LevelCore, Options{})
		expectReason(t, got, "The limit of report 1 differs.")
	})

	t.Run("passes without the reports where the adapter observes none, and says so", func(t *testing.T) {
		if got := run(reported, &Resolved{Output: "x"}); got != (Outcome{OK: true, Unobserved: true}) {
			t.Errorf("got %+v", got)
		}

		expectReason(t, run(reported, &Resolved{Output: "y"}), "The output differs.")
	})

	t.Run("refuses a case expecting neither an output nor a format", func(t *testing.T) {
		plan, _ := NewPlan(adapterOf(echo), Options{Set: setOf(fileOf(LevelCore, "9", concreteCase(t, "a/none", `{"expected": {}}`)))})
		_, err := plan.Cases[0].Execute()
		expectError(t, err, "The case a/none expects neither an output nor a format.")
	})
}

// performer is a host that answers every request with its own text, and
// every selection with a category it was given.
type performer struct {
	category string
	err      error
	got      []Request
	locales  []*string
}

func (p *performer) Perform(locale *string, request Request) (string, error) {
	p.got = append(p.got, request)
	p.locales = append(p.locales, locale)

	if p.err != nil {
		return "", p.err
	}

	if request.API == PluralRules {
		return p.category, nil
	}

	return "formatted " + string(request.API), nil
}

const selection = `{"message": "{{n:plural}}", "payload": {"n": 1}, "locale": "en", "expected": {
	"format": {"api": "PluralRules", "options": {"type": "cardinal"}, "input": 1},
	"categories": {"zero": "Z", "one": "O", "two": "T", "few": "F", "many": "M", "other": "R"}
}}`

func TestFormats(t *testing.T) {
	formatting := concreteCase(t, "b/format", `{"message": "{{n:number}}", "locale": "de", "expected": {"format": {"api": "NumberFormat", "options": {"maximumFractionDigits": 2, "useGrouping": false}, "input": 1234.5}}}`)
	relative := concreteCase(t, "b/ago", `{"message": "{{n:ago}}", "expected": {"format": {"api": "RelativeTimeFormat", "options": {"numeric": "auto"}, "input": [-1, "day"]}}}`)
	selecting := concreteCase(t, "b/plural", selection)
	exposing := func(c map[string]any, output string, requests ...Request) Outcome {
		return outcomeOf(t, answers(t, &Resolved{Output: output, Reports: reportsOf(), Formats: &requests}), c, LevelIntl, Options{})
	}
	request := Request{API: NumberFormat, Options: map[string]any{"useGrouping": false, "maximumFractionDigits": 2.0}, Input: 1234.5}

	t.Run("measures an exposed request on the request, not the text", func(t *testing.T) {
		if got := exposing(formatting, "whatever the data made", request); !got.OK {
			t.Errorf("got %+v", got)
		}

		if got := exposing(relative, "", Request{API: RelativeTimeFormat, Options: map[string]any{"numeric": "auto"}, Input: []any{-1.0, "day"}}); !got.OK {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("expects exactly one request, and compares its facility, its properties and its input", func(t *testing.T) {
		expectReason(t, exposing(formatting, ""), "Expected one formatting request, got 0.")
		expectReason(t, exposing(formatting, "", request, request), "Expected one formatting request, got 2.")

		other := request
		other.API = DateTimeFormat
		expectReason(t, exposing(formatting, "", other), "The facility of the request differs.")

		other = request
		other.Options = map[string]any{"maximumFractionDigits": 2.0}
		expectReason(t, exposing(formatting, "", other), "The properties of the request differ.")

		other = request
		other.Options = map[string]any{"maximumFractionDigits": 2.0, "useGrouping": "false"}
		expectReason(t, exposing(formatting, "", other), "The properties of the request differ.")

		other = request
		other.Input = 1234.0
		expectReason(t, exposing(formatting, "", other), "The input of the request differs.")
		expectReason(t, exposing(relative, "", Request{API: RelativeTimeFormat, Options: map[string]any{"numeric": "auto"}, Input: []any{-1.0, "days"}}), "The input of the request differs.")
	})

	t.Run("reads no properties as the empty set, and tells the zeros apart", func(t *testing.T) {
		bare := concreteCase(t, "b/bare", `{"expected": {"format": {"api": "NumberFormat", "input": 0}}}`)

		if got := exposing(bare, "", Request{API: NumberFormat, Options: map[string]any{}, Input: 0.0}); !got.OK {
			t.Errorf("got %+v", got)
		}

		expectReason(t, exposing(bare, "", Request{API: NumberFormat, Input: negativeZero()}), "The input of the request differs.")
		expectReason(t, exposing(bare, "", Request{API: NumberFormat, Options: map[string]any{"minimumFractionDigits": negativeZero()}, Input: 0.0}), "The properties of the request differ.")
	})

	t.Run("holds an exposed selection to the output its category selects", func(t *testing.T) {
		plural := Request{API: PluralRules, Options: map[string]any{"type": "cardinal"}, Input: 1.0, Category: "one"}

		if got := exposing(selecting, "O", plural); !got.OK {
			t.Errorf("got %+v", got)
		}

		plural.Category = "other"

		if got := exposing(selecting, "R", plural); !got.OK {
			t.Errorf("got %+v", got)
		}

		got := exposing(selecting, "O", plural)
		expectReason(t, got, "The output differs from the one the category other selects.")

		for _, category := range []string{"", "One", "single"} {
			plural.Category = category
			expectReason(t, exposing(selecting, "O", plural), "The selection request carries no category the plural rules name.")
		}
	})

	t.Run("performs the request on the runner's host where the adapter exposes none", func(t *testing.T) {
		host := &performer{}
		adapter := answers(t, &Resolved{Output: "formatted NumberFormat", Reports: reportsOf()})

		if got := outcomeOf(t, adapter, formatting, LevelIntl, Options{Host: host}); !got.OK {
			t.Errorf("got %+v", got)
		}

		if len(host.got) != 1 || !reflect.DeepEqual(host.got[0], request) || *host.locales[0] != "de" {
			t.Errorf("performed %+v in %v", host.got, host.locales)
		}

		expectReason(t, outcomeOf(t, answers(t, &Resolved{Output: "other text", Reports: reportsOf()}), formatting, LevelIntl, Options{Host: host}), "The output differs.")

		host.category = "few"

		if got := outcomeOf(t, answers(t, &Resolved{Output: "F", Reports: reportsOf()}), selecting, LevelIntl, Options{Host: host}); !got.OK {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("fails a case it has no host to perform on", func(t *testing.T) {
		got := outcomeOf(t, answers(t, &Resolved{Output: "x", Reports: reportsOf()}), formatting, LevelIntl, Options{})
		expectReason(t, got, "The runner has no facility to perform the request on.")
	})

	t.Run("ends the run where the host cannot perform a request, or answers a category the case names nothing for", func(t *testing.T) {
		adapter := answers(t, &Resolved{Output: "x", Reports: reportsOf()})
		_, err := Run(adapter, Options{Set: setOf(fileOf(LevelIntl, "11.2", formatting)), Host: &performer{err: errors.New("no data")}})
		expectError(t, err, "The host could not perform the request of the case b/format: no data")

		_, err = Run(adapter, Options{Set: setOf(fileOf(LevelIntl, "11.2", selecting)), Host: &performer{category: "several"}})
		expectError(t, err, `The case b/plural names no output for the category "several".`)
	})
}

func negativeZero() float64 {
	zero := 0.0

	return -zero
}

func generatedCase(id, generate string) map[string]any {
	return map[string]any{"id": id, "description": "Pins " + generate + ".", "generate": generate}
}

func TestGenerated(t *testing.T) {
	build := func(generate string, answer *Resolved) (Resolution, Outcome) {
		var got Resolution
		adapter := adapterOf(func(in Resolution) *Resolved {
			got = in

			return answer
		})
		bound := 2
		adapter.Reentry = &bound

		return got, outcomeOf(t, adapter, generatedCase("limits/"+generate, generate), LevelCore, Options{})
	}
	limit := func(code, origin string, n int) Report {
		return Report{Code: code, Origin: origin, ID: ptr("limits"), Limit: n}
	}

	for _, c := range []struct {
		generate string
		message  string
		payload  map[string]any
		answer   *Resolved
	}{
		{"output-at-limit", "{{v; a:xxxxx;}}", map[string]any{"v": "a"}, &Resolved{Output: "xxxxx", Reports: reportsOf()}},
		{"output-over-limit", "A{{v; a:xxxxxx;}}B", map[string]any{"v": "a"}, &Resolved{Output: "AB", Reports: reportsOf(limit("output-limit", "limit", 5))}},
		{"output-over-limit-continues", "{{v; a:xxxxxx;}}{{w}}", map[string]any{"v": "a", "w": "tail"}, &Resolved{Output: "tail", Reports: reportsOf(limit("output-limit", "limit", 5))}},
		{"read-at-limit", "{{v:eq; nomatch:X; default:ok;}}", map[string]any{"v": "xxxxxxx"}, &Resolved{Output: "ok", Reports: reportsOf()}},
		{"read-over-limit", "{{v:eq; nomatch:X; default:ok;}}{{w}}", map[string]any{"v": "xxxxxxxx", "w": "tail"}, &Resolved{Output: "ok", Reports: reportsOf(limit("read-limit", "limit", 7))}},
		{"nesting-at-limit", "{{v; a:{{v; a:{{v; a:settled; default:fallback;}};}};}}", map[string]any{"v": "a"}, &Resolved{Output: "settled", Reports: reportsOf()}},
		{"nesting-over-limit", "{{v; a:{{v; a:{{v; a:{{v; a:settled; default:fallback;}};}};}};}}", map[string]any{"v": "a"}, &Resolved{Output: "fallback", Reports: reportsOf(limit("nesting-limit", "message", 3))}},
		{"conversion-over-limit-continues", "{{v; default:D}}{{w}}", map[string]any{"v": []any{"xx", "xx", "xx", "xx", "xx", "xx", "xx"}, "w": "tail"}, &Resolved{Output: "Dtail", Reports: reportsOf(unserializableReport)}},
		{"nesting-over-limit-converts", "{{v; a:{{v; a:{{v; a:{{w; default:fallback;}};}};}};}}", map[string]any{"v": "a", "w": unserializable()}, &Resolved{Output: "fallback", Reports: reportsOf(unserializableReport, limit("nesting-limit", "message", 3))}},
		{"nesting-over-limit-reads", "{{v; a:{{v; a:{{v; a:{{w; default:fallback;}};}};}};}}{{v}}", map[string]any{"v": "a", "w": "xxxxxxxx"}, &Resolved{Output: "fallback", Reports: reportsOf(limit("nesting-limit", "message", 3), limit("read-limit", "limit", 7))}},
		{"nesting-over-limit-read-spent", "{{v; a:{{v; a:{{v; a:{{w:eq; nomatch:X;}}{{v; default:fallback;}};}};}};}}", map[string]any{"v": "a", "w": "xxxxxxxx"}, &Resolved{Output: "", Reports: reportsOf(limit("nesting-limit", "message", 3), limit("read-limit", "limit", 7))}},
		{"nesting-over-limit-unknown-modifier", "{{v; a:{{v; a:{{v; a:{{w:zz; default:fallback;}};}};}};}}", map[string]any{"v": "a", "w": "x"}, &Resolved{Output: "fallback", Reports: reportsOf(limit("nesting-limit", "message", 3))}},
		{"nesting-over-limit-missing-options", "{{v; a:{{v; a:{{v; a:{{w:eq; default:fallback;}};}};}};}}", map[string]any{"v": "a", "w": "x"}, &Resolved{Output: "fallback", Reports: reportsOf(limit("nesting-limit", "message", 3))}},
		{"reentry-at-limit", keep + keep + take, keptPayload, &Resolved{Output: "none", Reports: reportsOf()}},
		{"reentry-over-limit", keep + "{{x:keep; default:[" + take + "]}}" + keep + take, keptPayload, &Resolved{Output: "[empty]", Reports: reportsOf()}},
		{"reentry-refused-not-recorded", keep + "{{x:keep; default:[" + take + "]}}" + keep + take + "{{x:first; default:F}}", keptPayload, &Resolved{Output: "[empty]none", Reports: reportsOf()}},
		{"reentry-own-reads-free", "{{x:keep; default:{{x:read; default:{{x:read; default:z}}}}}}" + keep + take, keptPayload, &Resolved{Output: "z", Reports: reportsOf()}},
		{"reentry-over-limit-by-a-conversion", keep + "{{x:keep; default:[{{x:taking; default:F}}]}}" + keep + take, keptPayload, &Resolved{Output: "[empty]", Reports: reportsOf()}},
		{"reentry-own-read-in-a-nested-resolution", "{{x:keep; default:{{x:keep-nest; default:z}}}}" + take, keptPayload, &Resolved{Output: "z", Reports: reportsOf()}},
		{"reentry-over-limit-beneath-a-defined-modifier", "{{x:keep; default:" + keepNestNe + "}}" + take, keptPayload, &Resolved{Output: "<empty>", Reports: reportsOf()}},
	} {
		t.Run(c.generate, func(t *testing.T) {
			got, outcome := build(c.generate, c.answer)

			if got.Message != c.message || !reflect.DeepEqual(got.Payload, c.payload) || got.ID != "limits" {
				t.Errorf("built %+v", got)
			}

			if !outcome.OK {
				t.Errorf("got %+v", outcome)
			}

			wrong := *c.answer
			wrong.Output += "!"
			_, outcome = build(c.generate, &wrong)
			expectReason(t, outcome, "The output differs.")
		})
	}

	t.Run("reentry-resolutions-over-limit", func(t *testing.T) {
		resolve := func(answer *Resolved) (Resolution, Outcome) {
			var got Resolution
			adapter := adapterOf(func(in Resolution) *Resolved {
				got = in

				return answer
			})
			bound := 2
			adapter.Reentry = &bound
			adapter.Resolutions = true
			adapter.Limits.Output = 7

			return got, outcomeOf(t, adapter, generatedCase("limits/resolutions", "reentry-resolutions-over-limit"), LevelCore, Options{})
		}
		got, outcome := resolve(&Resolved{Output: "[[[F]]]", Reports: reportsOf()})

		if got.Message != "[{{x:renest; default:F}}]" || !reflect.DeepEqual(got.Payload, keptPayload) || got.ID != "limits" || len(got.Modifiers) != 1 || got.Modifiers["renest"] == nil || !outcome.OK {
			t.Errorf("built %+v, got %+v", got, outcome)
		}

		_, outcome = resolve(&Resolved{Output: "[[[[F]]]]", Reports: reportsOf()})
		expectReason(t, outcome, "The output differs.")
	})

	t.Run("conversion-at-limit", func(t *testing.T) {
		got, outcome := build("conversion-at-limit", &Resolved{Output: "ok", Reports: reportsOf()})

		if got.Message != "{{v:ne; nomatch:ok; default:refused;}}" || visits(got.Payload["v"]) != 7 || !outcome.OK {
			t.Errorf("built %+v, got %+v", got, outcome)
		}
	})

	t.Run("conversion-over-limit", func(t *testing.T) {
		got, outcome := build("conversion-over-limit", &Resolved{Output: "D", Reports: reportsOf(unserializableReport)})

		if got.Message != "{{v; default:D}}" || visits(got.Payload["v"]) != 8 || !outcome.OK {
			t.Errorf("built %+v, got %+v", got, outcome)
		}

		_, outcome = build("conversion-over-limit", &Resolved{Output: "D", Reports: reportsOf()})
		expectReason(t, outcome, "Expected 1 report, got 0 reports.")
	})

	t.Run("visits as many nodes as it states, sharing one map, its members holding undefined", func(t *testing.T) {
		for _, n := range []int{2, 5, 10, 17, 1000, 100001} {
			value := visiting(n, absent{})

			if got := visits(value); got != n {
				t.Errorf("visiting(%d) visits %d", n, got)
			}

			if text, _ := stringify(value); len(text) > 16*n {
				t.Errorf("visiting(%d) writes %d bytes", n, len(text))
			}
		}
	})

	t.Run("nesting-chain-over-limit", func(t *testing.T) {
		reports := slices.Repeat([]Report{limit("nesting-limit", "message", 3)}, 20000)
		got, outcome := build("nesting-chain-over-limit", &Resolved{Output: "x", Reports: &reports})

		if got.Message != strings.Repeat("{{v; default:", 20003)+"x"+strings.Repeat("}}", 20003) || len(got.Payload) != 0 || !outcome.OK {
			t.Errorf("built a message of %d bytes, got %+v", len(got.Message.(string)), outcome)
		}

		short := reports[1:]
		_, outcome = build("nesting-chain-over-limit", &Resolved{Output: "x", Reports: &short})
		expectReason(t, outcome, "Expected 20000 reports, got 19999 reports.")
	})

	t.Run("registers the behaviours over one list of what was kept", func(t *testing.T) {
		for generate, names := range map[string][]string{
			"reentry-at-limit":                        {"first", "keep", "read", "take"},
			"reentry-over-limit-by-a-conversion":      {"first", "keep", "read", "take", "taking"},
			"reentry-own-read-in-a-nested-resolution": {"first", "keep", "keep-nest", "read", "take", "taking"},
			// Nothing is registered under the name of the format's own ne.
			"reentry-over-limit-beneath-a-defined-modifier": {"first", "keep", "keep-nest-ne", "read", "take", "taking"},
		} {
			got, _ := build(generate, &Resolved{})

			if registered := slices.Sorted(maps.Keys(got.Modifiers)); !slices.Equal(registered, names) {
				t.Errorf("%s registers %v", generate, registered)
			}
		}
	})

	t.Run("builds the reads past a bound of 0 outside every other", func(t *testing.T) {
		for generate, want := range map[string]struct{ message, output string }{
			"reentry-at-limit":                              {take, "none"},
			"reentry-over-limit":                            {keep + take, "empty"},
			"reentry-refused-not-recorded":                  {keep + take + "{{x:first; default:F}}", "emptyempty"},
			"reentry-over-limit-by-a-conversion":            {keep + "{{x:taking; default:F}}", "empty"},
			"reentry-own-read-in-a-nested-resolution":       {"{{x:keep-nest; default:z}}", "z"},
			"reentry-over-limit-beneath-a-defined-modifier": {keepNestNe, "<empty>"},
			"reentry-resolutions-over-limit":                {"[{{x:renest; default:F}}]", "[F]"},
		} {
			zero := 0
			adapter := adapterOf(echo)
			adapter.Reentry = &zero
			p, _ := generated(generate, adapter)

			if p.input.Message != want.message || p.expected.output != want.output {
				t.Errorf("%s: built %q expecting %q", generate, p.input.Message, p.expected.output)
			}
		}
	})

	t.Run("stands at the bound by resolutions one inside another where the adapter states its bound counts them", func(t *testing.T) {
		for _, c := range []struct{ generate, inner, output string }{
			{"reentry-own-read-in-a-nested-resolution", keepNest, "z"},
			{defined, keepNestNe, "<empty>"},
		} {
			for bound, message := range map[int]string{1: c.inner, 3: descended, 40000: descended} {
				var got Resolution
				adapter := adapterOf(func(in Resolution) *Resolved {
					got = in

					return &Resolved{Output: c.output, Reports: reportsOf()}
				})
				adapter.Reentry = &bound
				adapter.Resolutions = true
				adapter.KeepsNothing = true
				outcome := outcomeOf(t, adapter, generatedCase("limits/"+c.generate, c.generate), LevelCore, Options{})
				registered := slices.Sorted(maps.Keys(got.Modifiers))

				if got.Message != message || !reflect.DeepEqual(got.Payload, keptPayload) || got.ID != "limits" || !outcome.OK {
					t.Errorf("%s at %d: built %+v, got %+v", c.generate, bound, got, outcome)
				}

				if !slices.Equal(registered, []string{"descend", "first", "keep", "keep-default", "keep-nest", "keep-nest-ne", "nest", "take", "taking"}) {
					t.Errorf("%s at %d registers %v", c.generate, bound, registered)
				}
			}
		}
	})

	t.Run("refuses to build from a bound the adapter does not state", func(t *testing.T) {
		for _, generate := range []string{"reentry-at-limit", "reentry-resolutions-over-limit"} {
			_, err := executeGenerated(adapterOf(echo), generatedCase("limits/reentry", generate))
			expectError(t, err, "The case limits/reentry is built from a bound on counted reads the adapter does not state.")
		}
	})

	t.Run("expects the refused resolution reported where the bound is 0, and nowhere else", func(t *testing.T) {
		for n, want := range map[int]int{0: 1, 1: 0} {
			adapter := adapterOf(echo)
			adapter.Reentry = &n
			p, _ := generated("reentry-resolutions-over-limit", adapter)

			if len(p.expected.reports) != want || want == 1 && (p.expected.reports[0].Code != "failed-modifier" || p.expected.reports[0].Origin != "payload") {
				t.Errorf("at %d: expects %+v", n, p.expected.reports)
			}
		}
	})

	t.Run("a report carrying no limit or no id is held to nothing", func(t *testing.T) {
		_, outcome := build("output-over-limit", &Resolved{Output: "AB", Reports: reportsOf(Report{Code: "output-limit"})})

		if !outcome.OK {
			t.Errorf("got %+v", outcome)
		}
	})

	t.Run("builds from the declared limits", func(t *testing.T) {
		var got Resolution
		adapter := adapterOf(func(in Resolution) *Resolved {
			got = in

			return &Resolved{}
		})
		adapter.Limits.Output = 100000
		outcomeOf(t, adapter, generatedCase("limits/at", "output-at-limit"), LevelCore, Options{})

		if got.Message != "{{v; a:"+strings.Repeat("x", 100000)+";}}" {
			t.Errorf("built a message of %d bytes", len(got.Message.(string)))
		}
	})

	t.Run("refuses a construction it does not carry", func(t *testing.T) {
		plan, _ := NewPlan(adapterOf(echo), Options{Set: setOf(fileOf(LevelCore, "13", generatedCase("limits/bogus", "constructor")))})
		_, err := plan.Cases[0].Execute()
		expectError(t, err, `The case limits/bogus names no construction: "constructor".`)
	})
}

// visits is how many values a serialization of value visits, following a
// shared slice or map each time it meets it.
func visits(value any) int {
	count := 0
	stack := []any{value}

	for len(stack) > 0 {
		next := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		count++

		switch v := next.(type) {
		case []any:
			stack = append(stack, v...)
		case map[string]any:
			for _, member := range v {
				stack = append(stack, member)
			}
		}
	}

	return count
}

// keep is the part the constructions of kept reads repeat, and keptPayload
// the payload each reads.
const keep = "{{x:keep; default:" + take + "}}"

var (
	keptPayload          = map[string]any{"x": ""}
	unserializableReport = Report{Code: "unserializable-value", Origin: "payload", ID: ptr("limits")}
)
