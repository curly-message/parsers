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
	// MaxConversionNodes is how many values one conversion to JSON visits
	// before it gives the value up as undescribable. A value fmt prints is
	// not a serialization and is not held to it: it is bounded by its text,
	// and, apart, by the values and string bytes inside the keys of the maps
	// it sorts, all of them together and counted again at each reference a
	// map is reached through, at most twice MaxSerializedLength plus two of
	// them.
	MaxConversionNodes = 100000
	// MaxSerializedLength is how many bytes one conversion writes, as JSON
	// or as fmt prints a value, before it gives the value up as
	// undescribable. A value fmt prints is not a serialization, and for it
	// this bound stands in for the host's cap on a string, which Go does
	// not have.
	MaxSerializedLength = 10000000
	// MaxNesting is how many levels of placeholder resolve, the outermost
	// first; a placeholder deeper than that takes its fallback chain and
	// reports NestingLimit.
	MaxNesting = 8
	// MaxReportedLength is how many bytes of a placeholder Report.Text
	// carries before the cut.
	MaxReportedLength = 120
)

// hostDepth is how many levels a value the parser hands to encoding/json
// may nest, as prewalk counts them over every field encoding/json may write.
// encoding/json recurses on the goroutine's stack, so this bounds what one
// conversion adds to it.
const hostDepth = 64

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

// Options configure a Parser. New reads them once.
type Options struct {
	// OnReport is told about each defect a resolution meets, in the order
	// the resolution meets them. Nil reports nowhere. A handler that panics
	// loses that report, and the resolution carries on.
	OnReport func(Report)
	// IgnoreWrappers turns off the recognition of value wrappers (section
	// 4.1): every payload entry is a value, whatever its shape.
	IgnoreWrappers bool
}

// Context is what one resolution reads besides the message.
type Context struct {
	// Payload holds the values the message's placeholders name.
	Payload map[string]any
	// ID names the message in the reports of its resolution; "" names none.
	ID string
}

// Parser resolves messages. It is immutable once New returns it, and safe
// for concurrent use.
type Parser struct {
	onReport       func(Report)
	ignoreWrappers bool
}

// New returns a Parser configured by options.
func New(options Options) *Parser {
	return &Parser{
		onReport:       options.OnReport,
		ignoreWrappers: options.IgnoreWrappers,
	}
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
		id:         context.ID,
		output:     MaxOutputLength,
		read:       MaxReadLength,
	}

	source, ok := w.convert(message, noSite)

	if !ok {
		return ""
	}

	w.src = source
	w.sc = newScanner(source)

	return w.render(0, len(source), 1)
}
