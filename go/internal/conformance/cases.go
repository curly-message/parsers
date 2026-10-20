package conformance

import (
	"fmt"
	"math"
	"slices"
	"strconv"
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

	named := func(key string, known func(string) bool) (map[string]string, error) {
		written, ok := c[key].(map[string]any)

		if !ok {
			return nil, nil
		}

		names := make(map[string]string, len(written))

		for name, entry := range written {
			behaviour, _ := entry.(string)

			if !known(behaviour) {
				return nil, fmt.Errorf("The case %s registers the behaviour %s, which the catalogue does not name.", id, shown(entry))
			}

			names[name] = behaviour
		}

		return names, nil
	}

	modifiers, err := named("modifiers", func(name string) bool { _, ok := behaviours[name]; return ok })

	if err != nil {
		return p, err
	}

	kept, err := named("keeping", func(name string) bool { return slices.Contains(keepers, name) })

	if err != nil {
		return p, err
	}

	p.input.Modifiers = register(modifiers, kept, adapter)

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

// converts is a value converted whole and compared against a key its text
// cannot equal: the option is chosen only where the value was converted, so
// a conversion abandoned shows in the output as the default, and none of the
// value's text reaches the output either way.
const converts = "{{v:ne; nomatch:ok; default:refused;}}"

// visiting is a value whose serialization visits the stated number of nodes
// and writes little text. Section 13 counts a member the walk visits and
// then omits, and a value reached twice twice: the root holds s references
// to one map of s - 1 members holding the adapter's undefined, each
// reference visiting s nodes, and enough members holding undefined itself to
// make up the rest. So the node count meets the limit long before any bound
// on the text, and an implementation that counts a shared map once, or
// skips what it omits, stops short of it.
func visiting(nodes int, undefined any) map[string]any {
	side := int(math.Sqrt(float64(nodes - 1)))
	shared := make(map[string]any, side-1)

	for i := range side - 1 {
		shared["k"+strconv.Itoa(i)] = undefined
	}

	root := make(map[string]any, nodes-1-side*side+side)

	for i := range side {
		root["s"+strconv.Itoa(i)] = shared
	}

	for i := range nodes - 1 - side*side {
		root["u"+strconv.Itoa(i)] = undefined
	}

	return root
}

// settles is the innermost level of nest unless a construction names
// another: a placeholder that declares a fallback, so a level the nesting
// limit refuses is visible as the chain it takes.
const settles = "{{v; a:settled; default:fallback;}}"

// nest is a message nesting the stated number of levels, each selecting the
// option that holds the next, around innermost.
func nest(levels int, innermost string) string {
	around := max(levels-1, 0)

	return strings.Repeat("{{v; a:", around) + innermost + strings.Repeat(";}}", around)
}

// chainLength is how many levels past the nesting limit the chain
// construction nests: far enough that a walk recursing once per level
// overflows a default call stack on some hosts, though not on every one.
const chainLength = 20000

// chain is a message nesting the stated number of levels, each the inline
// default of the one around it. The payload names none of the keys, so
// every level takes its chain down to the next, and the innermost writes
// the text.
func chain(levels int) string {
	return strings.Repeat("{{v; default:", levels) + "x" + strings.Repeat("}}", levels)
}

func limitReport(code, origin string, declared int) expectedReport {
	id := limitsID

	return expectedReport{Code: code, Origin: origin, ID: &id, Limit: declared}
}

// counted are the constructions built from the bound the adapter states on
// counted reads (section 13) rather than from its limits.
var counted = []string{"reentry-at-limit", "reentry-over-limit", "reentry-refused-not-recorded", "reentry-own-reads-free", "reentry-over-limit-by-a-conversion", "reentry-own-read-in-a-nested-resolution", "reentry-over-limit-beneath-a-defined-modifier"}

// framed are those of them that make their read in a resolution a modifier
// asks for with the bound reached, and are built from resolutions one inside
// another where the adapter states that its bound counts them, which keeps
// no input past the call it was handed to.
var framed = []string{"reentry-own-read-in-a-nested-resolution", defined}

// defined is the construction whose read is refused beneath a modifier of
// the format.
const defined = "reentry-over-limit-beneath-a-defined-modifier"

// resolutions is the construction built from that bound where the adapter
// states that it counts a resolution asked for inside another, which keeps
// no input.
const resolutions = "reentry-resolutions-over-limit"

// renestedOutput is the output of that construction at a bound: one
// resolution more than the bound stands, each writing its brackets.
func renestedOutput(bound int) string {
	return strings.Repeat("[", bound+1) + "F" + strings.Repeat("]", bound+1)
}

// take is a placeholder that reads the input kept last that no take has
// claimed. Its inline default stands for a take that failed, which no case
// expects.
const take = "{{x:take; default:F}}"

// takingRead is a placeholder whose modifier answers a value that reads the
// input kept last that no take has claimed, as it is converted.
const takingRead = "{{x:taking; default:F}}"

// keepNest is a placeholder whose modifier keeps its default unread and asks
// for a resolution in which the conversion of an answer reads it.
const keepNest = "{{x:keep-nest; default:z}}"

// keepNestNe is one whose modifier does the same, of a resolution in which
// that conversion runs while a modifier the format defines produces the
// option it selected, between brackets. Its default holds another: where the
// bound counts the resolution, the read of the first default is one past it;
// where it counts reads alone, that read takes the last place and produces
// the second placeholder, whose default is then read one past the bound.
// Either read is refused, and the text is the same.
const keepNestNe = "<{{x:keep-nest-ne; default:{{x:keep-nest-ne; default:z}}}}>"

// keeps is count placeholders that each keep their default unread, the
// default taking the input kept before it. A take after them claims the
// last one kept and reads it, and each read takes the one kept before, so
// the reads stand one inside another as deep as the placeholders kept,
// while the message nests two levels. What each answers is the text it
// read, or empty, so no result grows with the bound, and x holds the empty
// string, so no placeholder spends the read budget.
func keeps(count int, body string) string {
	return strings.Repeat("{{x:keep; default:"+body+"}}", max(count, 0))
}

// refusing is one input more than the bound lets be read at once. The read
// past it is made by the placeholder in the second input's default, written
// between brackets, so what it answers reaches the output as it answered
// it; where the bound is 0 it is the placeholder after them. It is a take,
// or one whose answer's conversion makes the read.
func refusing(bound int, reader string) string {
	if bound == 0 {
		return keeps(1, take) + reader
	}

	return keeps(1, take) + keeps(1, "["+reader+"]") + keeps(bound-1, take) + take
}

// own is the stated number of levels of own reads: each placeholder's
// modifier reads its own default, which holds the next, and the innermost
// answers z.
func own(levels int) string {
	return strings.Repeat("{{x:read; default:", levels) + "z" + strings.Repeat("}}", levels)
}

func generated(name string, adapter Adapter) (prepared, bool) {
	limits := adapter.Limits
	bound := 0

	if adapter.Reentry != nil {
		bound = *adapter.Reentry
	}

	id := limitsID
	in := func(message string, payload map[string]any) Resolution {
		return Resolution{Message: message, Payload: payload, ID: id}
	}
	out := func(output string, reports ...expectedReport) expectation {
		return expectation{id: name, output: output, reports: reports}
	}
	unserializableReport := expectedReport{Code: "unserializable-value", Origin: "payload", ID: &id}
	nestingReport := limitReport("nesting-limit", "message", limits.Nesting)
	readReport := limitReport("read-limit", "limit", limits.Read)
	reentering := func(message, output string, keepers ...string) (prepared, bool) {
		kept := map[string]string{"keep": "keep", "take": "take", "first": "first"}

		for _, keeper := range keepers {
			kept[keeper] = keeper
		}

		input := in(message, map[string]any{"x": ""})
		input.Modifiers = register(map[string]string{"read": "default"}, kept, adapter)

		return prepared{input, out(output)}, true
	}
	// standing is inner resolved with one place fewer taken than the bound
	// lets stand, so that the resolution its modifier asks for takes the
	// last where the bound counts resolutions. Where the adapter states that
	// it does, the places are resolutions one inside another, the innermost
	// of inner. Otherwise they are counted reads, the innermost producing
	// inner, and the bound may count the resolution or not: the adapter need
	// not say.
	standing := func(inner, keeper, output string) (prepared, bool) {
		if !adapter.Resolutions {
			if bound > 1 {
				return reentering(keeps(1, inner)+keeps(bound-2, take)+take, output, "taking", keeper)
			}

			return reentering(inner, output, "taking", keeper)
		}

		message := inner

		if bound > 1 {
			message = descended
		}

		input := in(message, map[string]any{"x": ""})
		input.Modifiers = descending(adapter, bound-1, inner)

		return prepared{input, out(output)}, true
	}
	past := func(bounded, unbounded string) string {
		if bound > 0 {
			return bounded
		}

		return unbounded
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
		return prepared{in(reads+"{{w}}", map[string]any{"v": strings.Repeat("x", limits.Read+1), "w": "tail"}), out("ok", readReport)}, true
	case "conversion-at-limit":
		return prepared{in(converts, map[string]any{"v": visiting(limits.Conversion, adapter.Undefined)}), out("ok")}, true
	case "conversion-over-limit":
		return prepared{in("{{v; default:D}}", map[string]any{"v": visiting(limits.Conversion+1, adapter.Undefined)}), out("D", unserializableReport)}, true
	case "conversion-over-limit-continues":
		// One node past the limit, each element long enough that the text a
		// finished serialization would write runs past the read budget: a
		// refused serialization produces no text, so it spends none, and the
		// placeholder after it is resolved and carried.
		element := strings.Repeat("x", (limits.Read+limits.Conversion-1)/limits.Conversion+1)
		elements := make([]any, limits.Conversion)

		for i := range elements {
			elements[i] = element
		}

		return prepared{in("{{v; default:D}}{{w}}", map[string]any{"v": elements, "w": "tail"}), out("Dtail", unserializableReport)}, true
	case "nesting-at-limit":
		return prepared{in(nest(limits.Nesting, settles), map[string]any{"v": "a"}), out("settled")}, true
	case "nesting-over-limit":
		// The innermost placeholder is the one refused, and a nesting limit
		// is a defect of the message: it takes its fallback chain, and every
		// level around it selects the option that holds it.
		return prepared{in(nest(limits.Nesting+1, settles), map[string]any{"v": "a"}), out("fallback", nestingReport)}, true
	// Section 9 decides the nesting limit after the value is looked up and
	// converted, so a placeholder nested too deep still converts its value,
	// reports what the conversion reports and spends the read budget, and
	// one reached with the budget spent resolves to the empty string however
	// deeply it nests.
	case "nesting-over-limit-converts":
		return prepared{in(nest(limits.Nesting+1, "{{w; default:fallback;}}"), map[string]any{"v": "a", "w": unserializable()}), out("fallback", unserializableReport, nestingReport)}, true
	case "nesting-over-limit-reads":
		return prepared{in(nest(limits.Nesting+1, "{{w; default:fallback;}}")+"{{v}}", map[string]any{"v": "a", "w": strings.Repeat("x", limits.Read+1)}), out("fallback", nestingReport, readReport)}, true
	case "nesting-over-limit-read-spent":
		return prepared{in(nest(limits.Nesting+1, "{{w:eq; nomatch:X;}}{{v; default:fallback;}}"), map[string]any{"v": "a", "w": strings.Repeat("x", limits.Read+1)}), out("", nestingReport, readReport)}, true
	// The nesting limit is the first of the message errors, and a
	// placeholder that meets one meets none after it.
	case "nesting-over-limit-unknown-modifier":
		return prepared{in(nest(limits.Nesting+1, "{{w:zz; default:fallback;}}"), map[string]any{"v": "a", "w": "x"}), out("fallback", nestingReport)}, true
	case "nesting-over-limit-missing-options":
		return prepared{in(nest(limits.Nesting+1, "{{w:eq; default:fallback;}}"), map[string]any{"v": "a", "w": "x"}), out("fallback", nestingReport)}, true
	case "nesting-chain-over-limit":
		// A refused placeholder takes its chain, whose last link is its
		// inline default, so each level past the limit is refused and
		// reported once and the walk carries on to the innermost text.
		return prepared{in(chain(limits.Nesting+chainLength), map[string]any{}), out("x", slices.Repeat([]expectedReport{nestingReport}, chainLength)...)}, true
	// As many reads as the bound lets stand at once: the innermost take
	// finds nothing left to claim.
	case "reentry-at-limit":
		return reentering(keeps(bound, take)+take, "none")
	// One read more: it is refused, produces nothing and answers the empty
	// string, and the take that made it goes on with it.
	case "reentry-over-limit":
		return reentering(refusing(bound, take), past("[empty]", "empty"))
	// The refused read was not recorded, so a read of the same input
	// afterwards, with no other read in progress, produces it: its take
	// finds every input claimed. Where the bound is 0, that read is refused
	// as well.
	case "reentry-refused-not-recorded":
		return reentering(refusing(bound, take)+"{{x:first; default:F}}", past("[empty]none", "emptyempty"))
	// The innermost of as many counted reads as the bound lets stand
	// produces a stack of own reads as deep as the nesting limit leaves room
	// for: they are the walk's and count toward no bound, so every one of
	// them resolves.
	case "reentry-own-reads-free":
		return reentering(keeps(1, own(max(limits.Nesting-1, 1)))+keeps(bound-1, take)+take, "z")
	// One read more, made by the conversion of an answer rather than by a
	// modifier: it is refused, and the conversion goes on with the empty
	// string.
	case "reentry-over-limit-by-a-conversion":
		return reentering(refusing(bound, takingRead), past("[empty]", "empty"), "taking")
	// With the bound reached, a modifier keeps its default and asks for a
	// resolution, where the conversion of an answer reads that default. The
	// modifier is current there, so the read is its own: a counted read
	// there would be one past the bound.
	case "reentry-own-read-in-a-nested-resolution":
		return standing(keepNest, "keep-nest", "z")
	// The same, with that conversion run while a modifier the format defines
	// produces the option it selected: that modifier is the current one, so
	// the read is no modifier's own, is counted one past the bound and is
	// refused, and the conversion goes on with the empty string.
	case defined:
		return standing(keepNestNe, "keep-nest-ne", "<empty>")
	// One resolution more than the bound lets stand inside the one asked
	// for: it is refused, the modifier that asked fails in the innermost
	// resolution standing, and reports there. Only where the bound is 0 is
	// that the resolution the case asks for.
	case resolutions:
		input := in(renested, map[string]any{"x": ""})
		input.Modifiers = map[string]Behaviour{"renest": renesting(adapter, bound)}

		if bound > 0 {
			return prepared{input, out(renestedOutput(bound))}, true
		}

		return prepared{input, out(renestedOutput(bound), expectedReport{Code: "failed-modifier", Origin: "payload", ID: &id})}, true
	}

	return prepared{}, false
}

// canonical is a value as a request is compared by: its entries, never the
// order a host happened to hold them in. The sign of zero is the request's,
// though JSON writes both zeros alike: a relative time reads -0 as the past
// and 0 as the future.
func canonical(value any) (string, bool) {
	return serialize(value, true)
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
	construction, _ := c["generate"].(string)

	if (slices.Contains(counted, construction) || construction == resolutions) && adapter.Reentry == nil {
		return Outcome{}, fmt.Errorf("The case %s is built from a bound on counted reads the adapter does not state.", c["id"])
	}

	p, ok := generated(construction, adapter)

	if !ok {
		return Outcome{}, fmt.Errorf("The case %s names no construction: %s.", c["id"], shown(c["generate"]))
	}

	return execute(adapter, p, nil)
}
