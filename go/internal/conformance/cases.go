package conformance

import (
	"fmt"
	"slices"
	"strings"
)

// categories are the plural categories of section 11.5.
var categories = []string{"zero", "one", "two", "few", "many", "other"}

// expectedReport is a report a case expects: a written case's, or a
// generated case's, which also carries the limit the adapter declared.
type expectedReport struct {
	Code   string  `json:"code"`
	Origin string  `json:"origin"`
	ID     *string `json:"id,omitempty"`
	Limit  int     `json:"limit,omitempty"`
}

// expectation is what a case expects: an output, or the request whose
// result on the runner's host decides it.
type expectation struct {
	id         string
	output     string
	format     map[string]any
	categories map[string]any
	locale     *string
	reports    []expectedReport
}

// prepared is a case with its inputs built and its expectation stated.
type prepared struct {
	input    Resolution
	expected expectation
}

func failure(reason string, expected, actual any) Outcome {
	return Outcome{Reason: reason, Expected: expected, Actual: actual}
}

func (e expectation) api() API {
	api, _ := e.format["api"].(string)

	return API(api)
}

// request is the case's request in the runner's own terms.
func (e expectation) request() Request {
	options, _ := e.format["options"].(map[string]any)

	return Request{API: e.api(), Options: options, Input: e.format["input"]}
}

// shown is what a failure names as expected before the adapter answered:
// the output, or the request where the case states one.
func (e expectation) shown() any {
	if e.format != nil {
		return e.format
	}

	return e.output
}

// text is the output a case expects of an adapter that exposes no
// requests: the output it states, or the text the runner's host makes of
// its request, or the output the category the host answers selects. A
// case without a host to perform its request on fails; a host that cannot
// perform it, or a category the case names no output for, ends the run.
func (e expectation) text(host Host) (string, *Outcome, error) {
	if e.format == nil {
		return e.output, nil, nil
	}

	if host == nil {
		outcome := failure("The runner has no facility to perform the request on.", e.format, nil)

		return "", &outcome, nil
	}

	answer, err := host.Perform(e.locale, e.request())

	if err != nil {
		return "", nil, fmt.Errorf("The host could not perform the request of the case %s: %w", e.id, err)
	}

	if e.api() != PluralRules {
		return answer, nil, nil
	}

	chosen, ok := e.categories[answer].(string)

	if !ok || !slices.Contains(categories, answer) {
		return "", nil, fmt.Errorf("The case %s names no output for the category %s.", e.id, shown(answer))
	}

	return chosen, nil, nil
}

// object reads an input a case writes as an object: absent, or written as
// the undefined tag, it is nil.
func objectInput(c map[string]any, name string, undefined any) (map[string]any, error) {
	raw, ok := c[name]

	if !ok || isUndefinedTag(raw) {
		return nil, nil
	}

	decoded, err := decode(raw, undefined)

	if err != nil {
		return nil, err
	}

	members, ok := decoded.(map[string]any)

	if !ok {
		return nil, fmt.Errorf("The case %s writes its %s as something that decodes to no object.", c["id"], name)
	}

	return members, nil
}

func expectedReports(written any) []expectedReport {
	items, _ := written.([]any)
	reports := make([]expectedReport, len(items))

	for i, item := range items {
		members := item.(map[string]any)
		reports[i].Code, _ = members["code"].(string)
		reports[i].Origin, _ = members["origin"].(string)

		if id, ok := members["id"].(string); ok {
			reports[i].ID = &id
		}
	}

	return reports
}

func concrete(adapter Adapter, c map[string]any) (prepared, error) {
	var p prepared
	var err error
	id, _ := c["id"].(string)

	if p.input.Message, err = decode(c["message"], adapter.Undefined); err != nil {
		return p, err
	}

	p.input.ID = adapter.Undefined

	if written, ok := c["messageId"]; ok {
		if p.input.ID, err = decode(written, adapter.Undefined); err != nil {
			return p, err
		}
	}

	if p.input.Payload, err = objectInput(c, "payload", adapter.Undefined); err != nil {
		return p, err
	}

	if p.input.Props, err = objectInput(c, "props", adapter.Undefined); err != nil {
		return p, err
	}

	if p.input.Defaults, err = objectInput(c, "defaults", adapter.Undefined); err != nil {
		return p, err
	}

	if locale, ok := c["locale"].(string); ok {
		p.input.Locale = &locale
	}

	if written, ok := c["modifiers"].(map[string]any); ok {
		p.input.Modifiers = map[string]Behaviour{}

		for name, entry := range written {
			named, _ := entry.(string)
			b, ok := behaviours[named]

			if !ok {
				return p, fmt.Errorf("The case %s registers the behaviour %s, which the catalogue does not name.", id, shown(entry))
			}

			p.input.Modifiers[name] = b
		}
	}

	expected, _ := c["expected"].(map[string]any)
	p.expected.id = id
	p.expected.output, _ = expected["output"].(string)
	p.expected.format, _ = expected["format"].(map[string]any)
	p.expected.categories, _ = expected["categories"].(map[string]any)
	p.expected.locale = p.input.Locale
	p.expected.reports = expectedReports(expected["reports"])

	if _, ok := expected["output"].(string); !ok && p.expected.format == nil {
		return p, fmt.Errorf("The case %s expects neither an output nor a format.", id)
	}

	return p, nil
}

// limitsID is the id every generated case reports under.
const limitsID = "limits"

// writes is an output of the stated length written entirely by the
// message: the placeholder selects an option, and an option value is the
// message's own text, so what reaches the output is the placeholder's
// result while the payload was read for one character.
func writes(length int) string {
	return "{{v; a:" + strings.Repeat("x", length) + ";}}"
}

// reads selects nothing from a value of any length: the read budget is
// spent whole and two characters reach the output.
const reads = "{{v:eq; nomatch:X; default:ok;}}"

// nest is a message nesting the stated number of levels, each selecting the
// option that holds the next. The innermost declares a fallback, so a level
// the nesting limit refuses is visible as the chain it takes.
func nest(levels int) string {
	around := max(levels-1, 0)

	return strings.Repeat("{{v; a:", around) + "{{v; a:settled; default:fallback;}}" + strings.Repeat(";}}", around)
}

func limitReport(code, origin string, declared int) expectedReport {
	id := limitsID

	return expectedReport{Code: code, Origin: origin, ID: &id, Limit: declared}
}

func generated(name string, limits Limits) (prepared, bool) {
	id := limitsID
	in := func(message string, payload map[string]any) Resolution {
		return Resolution{Message: message, Payload: payload, ID: id}
	}
	out := func(output string, reports ...expectedReport) expectation {
		return expectation{id: name, output: output, reports: reports}
	}

	switch name {
	case "output-at-limit":
		return prepared{in(writes(limits.Output), map[string]any{"v": "a"}), out(strings.Repeat("x", limits.Output))}, true
	case "output-over-limit":
		// The placeholder resolves to the empty string and the message's own
		// text still renders, because that text is the caller's and is
		// counted against no limit.
		return prepared{in("A"+writes(limits.Output+1)+"B", map[string]any{"v": "a"}), out("AB", limitReport("output-limit", "limit", limits.Output))}, true
	case "output-over-limit-continues":
		// A result the output has no room for spends nothing, so the
		// placeholder after it is resolved and carried.
		return prepared{in(writes(limits.Output+1)+"{{w}}", map[string]any{"v": "a", "w": "tail"}), out("tail", limitReport("output-limit", "limit", limits.Output))}, true
	case "read-at-limit":
		return prepared{in(reads, map[string]any{"v": strings.Repeat("x", limits.Read)}), out("ok")}, true
	case "read-over-limit":
		// The budget is tested before a placeholder reads, so the one that
		// spent it past the limit still resolves and the one after it pays.
		return prepared{in(reads+"{{w}}", map[string]any{"v": strings.Repeat("x", limits.Read+1), "w": "tail"}), out("ok", limitReport("read-limit", "limit", limits.Read))}, true
	case "conversion-over-limit":
		return prepared{in("{{v; default:D}}", map[string]any{"v": Nodes(float64(limits.Conversion) + 1)}), out("D", expectedReport{Code: "unserializable-value", Origin: "payload", ID: &id})}, true
	case "nesting-at-limit":
		return prepared{in(nest(limits.Nesting), map[string]any{"v": "a"}), out("settled")}, true
	case "nesting-over-limit":
		// The innermost placeholder is the one refused, and a nesting limit
		// is a defect of the message: it takes its fallback chain, and every
		// level around it selects the option that holds it.
		return prepared{in(nest(limits.Nesting+1), map[string]any{"v": "a"}), out("fallback", limitReport("nesting-limit", "message", limits.Nesting))}, true
	}

	return prepared{}, false
}

// canonical is a value as a request is compared by: its entries, never the
// order a host happened to hold them in.
func canonical(value any) (string, bool) {
	return stringify(value)
}

func same(a, b any) bool {
	x, ok := canonical(a)
	y, fine := canonical(b)

	return ok && fine && x == y
}

// compareFormats holds the requests a resolution exposed to the one the
// case states (section 11.2). A case that states a request writes the
// placeholder alone, so the resolution it describes makes exactly one.
func compareFormats(e expectation, actual []Request) *Outcome {
	if len(actual) != 1 {
		outcome := failure(fmt.Sprintf("Expected one formatting request, got %d.", len(actual)), []any{e.format}, actual)

		return &outcome
	}

	request := actual[0]
	wanted := e.request()

	var reason string

	switch {
	case request.API != wanted.API:
		reason = "The facility of the request differs."
	case !same(orEmpty(request.Options), orEmpty(wanted.Options)):
		reason = "The properties of the request differ."
	case !same(request.Input, wanted.Input):
		reason = "The input of the request differs."
	default:
		return nil
	}

	outcome := failure(reason, e.format, request)

	return &outcome
}

func orEmpty(options map[string]any) map[string]any {
	if options == nil {
		return map[string]any{}
	}

	return options
}

// compareSelection reads the category an exposed selection request carries
// and holds the output to the one the case names for it: the category is
// the host's, and the choice is the format's (section 11.5).
func compareSelection(e expectation, request Request, output string) *Outcome {
	if !slices.Contains(categories, request.Category) {
		outcome := failure("The selection request carries no category the plural rules name.", categories, request.Category)

		return &outcome
	}

	chosen, _ := e.categories[request.Category].(string)

	if output == chosen {
		return nil
	}

	outcome := failure(fmt.Sprintf("The output differs from the one the category %s selects.", request.Category), chosen, output)

	return &outcome
}

func reportCount(n int) string {
	if n == 1 {
		return "1 report"
	}

	return fmt.Sprintf("%d reports", n)
}

// compareReports holds the reports to the case's, in order. A field is
// compared where the adapter's report carries it and the case expects
// something of it: the specification prescribes a report no shape.
func compareReports(expected []expectedReport, actual []Report) *Outcome {
	if len(expected) != len(actual) {
		outcome := failure(fmt.Sprintf("Expected %s, got %s.", reportCount(len(expected)), reportCount(len(actual))), expected, actual)

		return &outcome
	}

	for i, report := range actual {
		wanted := expected[i]
		var field string

		switch {
		case report.Code != wanted.Code:
			field = "code"
		case report.Origin != "" && wanted.Origin != "" && report.Origin != wanted.Origin:
			field = "origin"
		case report.ID != nil && wanted.ID != nil && *report.ID != *wanted.ID:
			field = "id"
		case report.Limit != 0 && wanted.Limit != 0 && report.Limit != wanted.Limit:
			field = "limit"
		default:
			continue
		}

		outcome := failure(fmt.Sprintf("The %s of report %d differs.", field, i+1), wanted, report)

		return &outcome
	}

	return nil
}

// raised is what an adapter panicked with, as text.
func raised(value any) string {
	switch v := value.(type) {
	case error:
		return v.Error()
	case string:
		return v
	}

	return shown(value)
}

// answer puts the inputs to the adapter. A panic is the adapter raising,
// which fails the case and only the case.
func answer(adapter Adapter, input Resolution, expected any) (resolved *Resolved, outcome *Outcome) {
	defer func() {
		if value := recover(); value != nil {
			failed := failure("The adapter raised.", expected, raised(value))
			resolved, outcome = nil, &failed
		}
	}()

	resolved = adapter.Resolve(input)

	if resolved == nil {
		failed := failure("The adapter answered with no resolution.", expected, nil)

		return nil, &failed
	}

	return resolved, nil
}

func execute(adapter Adapter, p prepared, host Host) (Outcome, error) {
	resolved, failed := answer(adapter, p.input, p.expected.shown())

	if failed != nil {
		return *failed, nil
	}

	if p.expected.format != nil && resolved.Formats != nil {
		if failed := compareFormats(p.expected, *resolved.Formats); failed != nil {
			return *failed, nil
		}

		if p.expected.api() == PluralRules {
			if failed := compareSelection(p.expected, (*resolved.Formats)[0], resolved.Output); failed != nil {
				return *failed, nil
			}
		}
	} else {
		output, failed, err := p.expected.text(host)

		if err != nil {
			return Outcome{}, err
		}

		if failed != nil {
			return *failed, nil
		}

		if resolved.Output != output {
			return failure("The output differs.", output, resolved.Output), nil
		}
	}

	if resolved.Reports == nil {
		return Outcome{OK: true, Unobserved: true}, nil
	}

	if failed := compareReports(p.expected.reports, *resolved.Reports); failed != nil {
		return *failed, nil
	}

	return Outcome{OK: true}, nil
}

func executeConcrete(adapter Adapter, c map[string]any, host Host) (Outcome, error) {
	p, err := concrete(adapter, c)

	if err != nil {
		return Outcome{}, err
	}

	return execute(adapter, p, host)
}

func executeGenerated(adapter Adapter, c map[string]any) (Outcome, error) {
	p, ok := generated(c["generate"].(string), adapter.Limits)

	if !ok {
		return Outcome{}, fmt.Errorf("The case %s names no construction: %s.", c["id"], shown(c["generate"]))
	}

	return execute(adapter, p, nil)
}
