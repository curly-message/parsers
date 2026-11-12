// Package conformance runs the conformance set of the Curly Message Format
// against an implementation, as the set's RUNNER.md states a runner in
// another language does. It imports nothing of the implementation it
// measures: an implementation meets it through an Adapter (section 14.3),
// which is where its own calling convention is translated into this one.
package conformance

// Level is a conformance level of section 2.
type Level string

// The three levels of section 2.
const (
	LevelCore       Level = "core"
	LevelIntl       Level = "intl"
	LevelExtensions Level = "extensions"
)

// Limits are the limits of section 13 an implementation permits, as it
// documents them, in the host's own string unit where they measure text.
// The generated cases are built from them.
type Limits struct {
	Output     int
	Read       int
	Conversion int
	Nesting    int
}

// API is a facility a locale-dependent modifier delegates to (sections 11.2
// and 11.5).
type API string

// The four facilities of section 11.2.
const (
	NumberFormat       API = "NumberFormat"
	DateTimeFormat     API = "DateTimeFormat"
	RelativeTimeFormat API = "RelativeTimeFormat"
	PluralRules        API = "PluralRules"
)

// SpanUnit is a unit CST.md section 4 lets an implementation count its spans
// in.
type SpanUnit string

// The three units of CST.md section 4.
const (
	UTF8      SpanUnit = "utf-8"
	UTF16     SpanUnit = "utf-16"
	CodePoint SpanUnit = "code-point"
)

// OptionInput is one option as a host-defined modifier receives it: the key
// as the placeholder wrote it, and the value behind a call, because an
// option a modifier never reads is never resolved (section 9.4).
type OptionInput struct {
	Key   string
	Value func() string
}

// ModifierInput is what a host-defined modifier receives (sections 11 and
// 11.3): the value and the default as text, the options in the order the
// placeholder wrote them, the props composed under the modifier's own name,
// and the locale as the caller supplied it, nil where it supplied none. The
// default is read through a call, because section 10 walks the chain only
// where a modifier asks for it.
type ModifierInput struct {
	Value   string
	Options []OptionInput
	Props   map[string]any
	Locale  *string
	Default func() string
}

// Behaviour is a behaviour of the set's catalogue, as a function the adapter
// wraps into its implementation's own modifier signature. A returned error
// is the behaviour raising; a nil answer is the host's nothing.
type Behaviour func(ModifierInput) (any, error)

// Resolution is one resolution's inputs, decoded into host values: the four
// inputs of section 4 and the message's id, plus the host-defined modifiers
// a case registers (section 11.3) and the implementation-configured defaults
// of section 11.2. A field a case does not write is nil.
type Resolution struct {
	Message   any
	Payload   map[string]any
	Props     map[string]any
	Locale    *string
	ID        any
	Modifiers map[string]Behaviour
	Defaults  map[string]any
}

// Report is a report as the adapter observed it. Only the code is required;
// a field the adapter carries is held to what the case expects of it, and
// one it leaves out is unobserved: Origin "", ID nil and Limit 0 carry
// nothing, a limit being a positive count.
type Report struct {
	Code   string  `json:"code"`
	Origin string  `json:"origin,omitempty"`
	ID     *string `json:"id,omitempty"`
	Limit  int     `json:"limit,omitempty"`
}

// Request is a formatting request in the vocabulary of section 11.2: the
// facility, the properties it is constructed with, and its input: a number
// for NumberFormat and PluralRules, milliseconds since the epoch for
// DateTimeFormat, and []any{value, unit} for RelativeTimeFormat. An exposed
// selection request (section 11.5) carries the category the host answered.
type Request struct {
	API      API            `json:"api"`
	Options  map[string]any `json:"options,omitempty"`
	Input    any            `json:"input"`
	Category string         `json:"category,omitempty"`
}

// Resolved is what a resolution produced. Reports nil says the
// implementation does not report, and every expectation about reports goes
// unobserved; Formats nil says it does not expose the requests a resolution
// made (section 11.2), and a formatting case is then measured on its text.
type Resolved struct {
	Output  string
	Reports *[]Report
	Formats *[]Request
}

// Node is a node of the concrete syntax tree (CST.md section 8), its span in
// the unit the adapter declared. Name and Cancels are nil where the node
// carries none.
type Node struct {
	Type    string  `json:"type"`
	Start   int     `json:"start"`
	End     int     `json:"end"`
	Nodes   []*Node `json:"nodes,omitempty"`
	Name    *string `json:"name,omitempty"`
	Cancels *bool   `json:"cancels,omitempty"`
}

// CST is what an implementation that offers a tree supplies: the unit its
// spans are counted in, and the call that produces one.
type CST struct {
	Unit  SpanUnit
	Parse func(message string) *Node
}

// Adapter is the adapter of section 14.3. Levels selects the fixtures the
// set runs and Limits builds the generated cases; Unexpressible, where set,
// names the properties of each facility's request the host cannot express
// (section 11.2), and CST, where set, offers the tree. Undefined is the
// value the implementation reads as the caller having passed nothing, which
// a case's undefined tag decodes to: Go has no undefined of its own, so an
// adapter states one. Resolve answers one resolution; a panic is the adapter
// raising, and nil is no resolution at all.
type Adapter struct {
	Levels        []Level
	Limits        Limits
	Unexpressible map[API][]string
	Undefined     any
	Resolve       func(Resolution) *Resolved
	CST           *CST
}

// Host performs a formatting request on the runner's own host, for an
// adapter that exposes no requests: the text a formatting request makes, or
// the category a PluralRules request answers. The locale is the case's,
// nil where it names none.
type Host interface {
	Perform(locale *string, request Request) (string, error)
}

// Options select what a run reads.
type Options struct {
	// Set is the set to run; the vendored release when nil.
	Set *Set
	// Levels runs only these levels, among those the adapter claims.
	Levels []Level
	// Host performs the requests of the formatting cases an adapter that
	// exposes no requests is measured on. Without one, such a case fails.
	Host Host
}

// Identity identifies a case wherever it came from. Level is empty on a
// tree case, which has none, and Document names CST.md there, because
// Section reads against that document rather than the specification.
type Identity struct {
	ID          string
	File        string
	Level       Level
	Document    string
	Section     string
	Description string
}

// Outcome is what running a case answered: passed, passed with its reports
// unobserved, or failed with the first mismatch beside what was expected and
// what came back.
type Outcome struct {
	OK         bool
	Unobserved bool
	Reason     string
	Expected   any
	Actual     any
}

// Planned is a case ready to run. Execute answers its outcome, or an error
// where the case is one the runner cannot read, which ends the run.
type Planned struct {
	Identity
	Execute func() (Outcome, error)
}

// Skipped is a case the plan left out, and why.
type Skipped struct {
	Identity
	Reason string
}

// Plan is the cases an adapter's levels require, each ready to run, beside
// the cases left out.
type Plan struct {
	Cases   []Planned
	Skipped []Skipped
}

// Failed is a case that failed, with its outcome.
type Failed struct {
	Planned
	Outcome Outcome
}

// Result is a plan executed and its outcomes sorted. Unobserved lists the
// cases among Passed whose report expectation went unchecked.
type Result struct {
	Passed     []Planned
	Failed     []Failed
	Skipped    []Skipped
	Unobserved []Planned
}

// Verdict is what a correct runner answers where a defect is present.
type Verdict string

// The four verdicts of RUNNER.md, and the answer of a runner that answered
// nothing.
const (
	VerdictError      Verdict = "error"
	VerdictFail       Verdict = "fail"
	VerdictSkip       Verdict = "skip"
	VerdictUnobserved Verdict = "unobserved"
	VerdictNone       Verdict = "none"
)

// Defect is one defect of the catalogue shipped as defects.json.
type Defect struct {
	ID          string
	Description string
	Document    string
	Section     string
	Expects     Verdict
}

// Audited is what the audit made of one defect: what the runner observed,
// and whether that is the defect caught, missed or unreachable.
type Audited struct {
	Defect
	Observed Verdict
	Outcome  string
}

// The outcomes of an audited defect.
const (
	Caught      = "caught"
	Missed      = "missed"
	Unreachable = "unreachable"
)

// Mutation is a defect carried by an adapter: the adapter carrying it, and
// whether the defect reached the runner at all.
type Mutation struct {
	Adapter Adapter
	Reached func() bool
}
