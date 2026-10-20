package curly

import "reflect"

// The limits of section 13. Each is a number the format documents rather
// than a setting: what one resolution costs is bounded by the message
// because of them. Text is measured in bytes of UTF-8.
const (
	// MaxOutputLength is how many bytes of placeholder results the output
	// of one resolution carries; a result past it resolves to the empty
	// string and reports OutputLimit. The message's own text is the
	// caller's and counts against nothing.
	MaxOutputLength = 100000
	// MaxReadLength is how many bytes of value text one resolution reads
	// from the payload before a placeholder that would read more resolves
	// to the empty string and reports ReadLimit.
	MaxReadLength = 100000
	// MaxConversionNodes is how many values one conversion visits before it
	// gives the value up as undescribable.
	MaxConversionNodes = 100000
	// MaxNesting is how many levels of placeholder resolve, the outermost
	// first; a placeholder deeper than that takes its fallback chain and
	// reports NestingLimit.
	MaxNesting = 8
	// MaxReportedLength is how many bytes of a placeholder Report.Text
	// carries before the cut.
	MaxReportedLength = 120
)

// hostDepth is how deep a value the parser hands to code it does not own,
// encoding/json among it, which recurses on the goroutine's stack. A value
// nested deeper is undescribable rather than handed over, which bounds what
// one conversion adds to the stack.
const hostDepth = 10000

// Undefined is the host's undefined where a value has to be written: as a
// message, a payload entry, a wrapper member or a layer property it reads
// exactly as a missing key does. A plain object's member holding it is left
// out of the object's JSON, and an element holding it is written null, as
// ECMAScript writes the two.
type Undefined struct{}

// MarshalJSON writes null, for where encoding/json meets Undefined inside a
// value the parser hands it.
func (Undefined) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

// String writes undefined, for where a value printed as fmt prints it holds
// one.
func (Undefined) String() string {
	return "undefined"
}

// Options configure a Parser. New reads them once: a map changed after New
// returns changes nothing the parser does.
type Options struct {
	// CustomModifiers registers the host's own modifiers by name (section
	// 11.3). A name the format defines is replaced by the host's; a nil entry
	// registers nothing.
	CustomModifiers map[string]Modifier
	// ModifierDefaults is the bottom layer of a modifier's properties
	// (section 11.2), keyed by modifier name; an entry is read where it is a
	// map keyed by a string type.
	ModifierDefaults map[string]any
	// OnReport is told about each defect a resolution meets, in the order
	// the resolution meets them. Nil reports nowhere. A handler that panics
	// loses that report, and the resolution carries on. It is called on the
	// goroutine that met the defect, so where a modifier reads its options
	// from goroutines of its own, calls of one resolution can overlap.
	OnReport func(Report)
	// IgnoreWrappers turns off the recognition of value wrappers (section
	// 4.1): every payload entry is a value, whatever its shape.
	IgnoreWrappers bool
}

// Context is what one resolution reads besides the message.
type Context struct {
	// Payload holds the values the message's placeholders name.
	Payload map[string]any
	// Props is the caller's layer of modifier properties, keyed by modifier
	// name, between the parser's ModifierDefaults and a wrapper's props.
	Props map[string]any
	// Locale is the locale a modifier is handed: nil where the caller
	// supplied none, which a host modifier can tell apart from an empty one.
	Locale *string
	// ID names the message in the reports of its resolution; "" names none.
	ID string
}

// Parser resolves messages. It is immutable once New returns it, and safe
// for concurrent use.
type Parser struct {
	custom         map[string]Modifier
	defaults       map[string]map[string]any
	onReport       func(Report)
	ignoreWrappers bool
}

// New returns a Parser configured by options.
func New(options Options) *Parser {
	p := &Parser{
		onReport:       options.OnReport,
		ignoreWrappers: options.IgnoreWrappers,
	}

	for name, m := range options.CustomModifiers {
		if m == nil {
			continue
		}

		if p.custom == nil {
			p.custom = map[string]Modifier{}
		}

		p.custom[name] = m
	}

	for name, layer := range options.ModifierDefaults {
		props := map[string]any{}

		if mergeLayer(props, layer) {
			if p.defaults == nil {
				p.defaults = map[string]map[string]any{}
			}

			p.defaults[name] = props
		}
	}

	return p
}

// Modifier is a modifier a host registers (section 11.3). Its answer
// becomes text as a payload value does. An answer of nil or Undefined is no
// answer, and the placeholder takes its fallback chain; so does a modifier
// that returns an error or panics, which reports FailedModifier.
//
// A modifier may read its options and its default from goroutines of its
// own, and after it has returned. A read arriving while another renders the
// same option or default, or converts the same value, waits for that one, so
// a modifier that reaches, through state it keeps, an option or default from
// inside its own rendering, or a value from inside that value's own method,
// waits forever.
type Modifier func(Input) (any, error)

// Input is what a modifier receives (section 11).
type Input struct {
	// Value is the text the placeholder's value converted to.
	Value string
	// Options are the placeholder's options in the order it wrote them,
	// duplicates kept.
	Options []Option
	// Props are the properties the layers hold under the modifier's own
	// name, composed property by property: the parser's defaults, then the
	// caller's, then the wrapper's. The map is the modifier's own.
	Props map[string]any
	// Locale is the caller's locale, as the caller supplied it.
	Locale *string

	s *slot
}

// Default is the placeholder's fallback chain (section 10), walked on the
// first call.
func (in Input) Default() string {
	if in.s == nil {
		return ""
	}

	w := in.s.w
	w.mu.Lock()
	defer w.mu.Unlock()

	return in.s.chain()
}

// Option is one option of a placeholder.
type Option struct {
	// Key is the option's key, unescaped.
	Key string

	s *slot
	i int
}

// Value is the option's value, rendered on the first call: an option a
// modifier never asks for is never resolved.
func (o Option) Value() string {
	if o.s == nil {
		return ""
	}

	w := o.s.w
	w.mu.Lock()
	defer w.mu.Unlock()

	return o.s.option(o.i)
}

// Resolve resolves a message against a context in one walk (section 5). A
// message that is not a string becomes text first, as a payload value does,
// and one nothing describes resolves to the empty string, as does Undefined.
// Resolve never panics on a message or a payload: a defect resolves the
// placeholder it is in and is told to OnReport.
func (p *Parser) Resolve(message any, context Context) string {
	w := &walk{
		p:          p,
		payload:    context.Payload,
		payloadPtr: reflect.ValueOf(context.Payload).Pointer(),
		props:      context.Props,
		locale:     context.Locale,
		id:         context.ID,
		output:     MaxOutputLength,
		read:       MaxReadLength,
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	source, ok := w.convert(message, noSite)

	if !ok {
		return ""
	}

	w.src = source
	w.sc = newScanner(source)

	return w.render(0, len(source), 1)
}
