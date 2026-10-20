package curly

import (
	"errors"
	"reflect"
)

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

// MaxReentry is how many resolutions one lineage holds in progress at once,
// counted through each means to ask the parser hands out (see Modifier), and
// through each read of an option or default that host code makes other than
// as the modifier of that placeholder while it is current (see
// Option.Value). A resolution Parser.Resolve starts counts as the first of
// its lineage, and one asked for past MaxReentry is refused. It is the bound
// section 13 has an implementation keep in place of the host's limit on how
// deep a resolution that reaches itself goes, which in Go is a goroutine's
// stack outgrowing its maximum, a fatal error. A lineage of MaxReentry
// resolutions, each asked for from a placeholder at
// MaxNesting, takes about 9.9 MB of the goroutine's stack on a 64-bit
// platform and 6.3 MB on a 32-bit one, and at most about 48 MB and 27 MB
// with Go 1.26, 77 MB and 47 MB with Go 1.27, where each is asked for from
// inside encoding/json, which is handed no value nested deeper than 64
// levels, of the 512 MiB and 128 MiB a goroutine's stack reaches at most by
// default: it grows by doubling, up to Go's limits of 1 GB and 250 MB.
const MaxReentry = 1000

// ErrReentryLimit is the error Resolver.Resolve returns for a resolution it
// refuses, one that would count past MaxReentry.
var ErrReentryLimit = errors.New("curly: more than MaxReentry resolutions in progress")

// errZeroInput is the error the zero Resolver's Resolve returns, the zero
// Input's among them: no resolution handed it out, so it has no lineage to
// count with.
var errZeroInput = errors.New("curly: Resolve of an Input or Resolver the parser did not hand out")

// hostDepth is how many levels a value the parser hands to encoding/json
// may nest, as prewalk counts them over every field encoding/json may write.
// encoding/json recurses on the goroutine's stack, so this bounds what one
// conversion adds to it. A resolution a method asks for through an Input, as
// encoding/json writes a value the asking resolution converts, sits below at
// most hostDepth levels of encoding/json, and this bound is what keeps what
// it adds to a lineage's stack within what MaxReentry allows for: the levels
// take about 560 bytes each on a 64-bit platform and 300 on a 32-bit one
// with Go 1.26, and 1 020 and 620 with Go 1.27, where encoding/json writes a
// typed map of typed maps, the costliest shape measured.
const hostDepth = 64

// Undefined is the host's undefined where a value has to be written: as a
// message, a payload entry, a wrapper member or a layer property it reads
// as a missing key does, but that a map whose value member holds it is still
// a wrapper, one with no value. A plain object's member holding it is left
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
	// from goroutines of its own, calls of one resolution can overlap. A
	// handler that resolves a message asks through the Resolver it is
	// handed.
	OnReport func(Report, Resolver)
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
	onReport       func(Report, Resolver)
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
// own, and after it has returned. A read never waits: one of an option or
// default being rendered, on whatever goroutine, one whose rendering needs
// an option or a fallback chain another goroutine's read is rendering, and
// one that meets a value being converted, are refused (see Option.Value).
//
// The parser makes Resolve wait for a modifier's goroutine in two ways. The
// resolution holds one lock while parser code runs, on any goroutine, and
// releases it around every call into host code, so each time the goroutine
// that called Resolve needs the lock, it also waits for the parser code a
// read on a modifier's goroutine is running under it, each read up to its
// next call into host code, a whole conversion included: work the limits of
// section 13 bound, never host code. And where the walk, outside any read,
// needs what a read is producing, the fallback chain a placeholder takes
// after its modifier was asked or a value it converts, it waits for that
// read. Host code that waits for Resolve to return while it produces what the
// walk needs waits forever, and so does Resolve. A read that finished during
// the call answers its text again after it. One still producing an option or
// default when Resolve returns, in host code or waiting to take the
// resolution's lock back, goes on only to finish: it calls no more host code,
// reads no more of the payload, reports nothing and spends no budget, and it
// answers the empty string. Any other read once Resolve has returned, one
// still waiting for the lock among them, answers what a read finished during
// the call, or the empty string where none did. Host code still running when
// Resolve returns, a method of a value or a modifier's goroutine, can read
// the payload until it returns, so the caller changes the payload only once
// that code has returned.
//
// Go names no goroutine, so the parser counts along what it hands out, not by
// where code runs. A lineage is the resolution Parser.Resolve started and
// every resolution asked for through a means handed out in a resolution of
// the lineage: a modifier's Input, or the Resolver a Texter and an OnReport
// handler are handed. MaxReentry bounds the resolutions and counted reads of
// one lineage in progress at once, on whatever goroutines, so host code that
// resolves asks through its means. Parser.Resolve starts a lineage of its own
// wherever it is called: host code that calls it beneath a resolution, when
// it was handed a means, is counted nowhere, and bounds its own recursion,
// which otherwise ends at the goroutine's stack, and with it the process.
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
	// caller's, then the wrapper's. It is nil where no layer holds anything
	// under the name. The defaults and the caller's are composed once in a
	// resolution, and a wrapper's layer with them once for each run of the
	// modifier's placeholders whose wrappers hold that layer; the map is
	// shared by every placeholder its composition serves, so a modifier
	// never writes to it, and the layers do not change while the resolution
	// reads them.
	Props map[string]any
	// Locale is the caller's locale, as the caller supplied it.
	Locale *string

	s *slot
}

// Default is the placeholder's fallback chain (section 10), walked on the
// first call. Once Resolve has returned, it is the chain a call finished
// walking during the resolution, or the empty string where none did. It
// counts as Option.Value does where host code other than the placeholder's
// modifier reads it.
func (in Input) Default() string {
	if in.s == nil {
		return ""
	}

	mu := in.s.w.lock()
	mu.Lock()
	defer mu.Unlock()

	defer in.s.w.hostRead()()

	return in.s.chainRead(true)
}

// Option is one option of a placeholder.
type Option struct {
	// Key is the option's key, unescaped.
	Key string

	s *slot
	i int
}

// Value is the option's value, rendered on the first call: an option a
// modifier never asks for is never resolved. Once Resolve has returned, it
// is the value a call finished rendering during the resolution, or the empty
// string where none did.
//
// A read while the modifier the option was handed to is current, the one its
// lineage called last of those that have not returned, renders for nothing
// where nothing of that placeholder is being rendered: what it renders is
// nested one level further in, which MaxNesting bounds, up to MaxNesting such
// reads in progress for each resolution or counted read of the lineage. Any
// other read renders a body from outside the one being rendered, and counts
// as one more resolution of the lineage in progress until it has rendered.
// One that would count past MaxReentry is refused (section 13): it renders
// nothing, answers the empty string, leaves the option for a later read and
// fails nothing. A read of an option being rendered is refused the same way,
// on whatever goroutine: the parser cannot tell a read beside the rendering
// from one the rendering itself makes, so it never waits for one, and a read
// whose rendering needs an option or a fallback chain another goroutine's
// read is rendering is refused there: the placeholder that needs it takes the
// empty string, and it is left to that read. A read that meets a value being
// converted is refused where it meets it, for the same reason: nothing is
// reported for that value, and the empty string is the option's value for the
// rest of the resolution, so what the read had reached is not reached again.
// A modifier that reads one option, or options naming one value, from several
// goroutines therefore reads them once before it starts them.
func (o Option) Value() string {
	if o.s == nil {
		return ""
	}

	mu := o.s.w.lock()
	mu.Lock()
	defer mu.Unlock()

	defer o.s.w.hostRead()()

	return o.s.optionRead(o.i, true)
}

// Resolver is the means the parser hands host code to ask for a resolution
// counted with the lineage of the resolution that called it: a modifier has
// its Input's, and a Texter and an OnReport handler are each handed one.
// The zero Resolver resolves nothing and returns an error.
type Resolver struct {
	w *walk
	p *Parser
}

// Resolver is the means to ask that the modifier handed in was given.
func (in Input) Resolver() Resolver {
	if in.s == nil {
		return Resolver{}
	}

	return Resolver{in.s.w, in.s.w.p}
}

// With is r asking p: a resolution by another parser that counts with the
// same lineage.
func (r Resolver) With(p *Parser) Resolver {
	r.p = p

	return r
}

// Resolve resolves a message against a context, with the parser that handed r
// out or the one With named, as one more resolution of the lineage r was
// handed out in, whether that resolution is in progress or has returned. One
// that would count past MaxReentry resolves nothing and returns
// ErrReentryLimit, which a modifier handles as any error, and which, returned
// as the modifier's own, fails the modifier. A Resolver kept keeps the
// resolution it was handed out in from being collected. The zero Resolver
// resolves nothing and returns an error.
func (r Resolver) Resolve(message any, context Context) (string, error) {
	if r.w == nil || r.p == nil {
		return "", errZeroInput
	}

	c := r.w.lineage

	c.mu.Lock()

	if c.held >= MaxReentry {
		c.mu.Unlock()

		return "", ErrReentryLimit
	}

	c.held++
	c.mu.Unlock()

	return r.p.resolve(message, context, c), nil
}

// Texter is a payload value that becomes text by resolving a message, a
// lazily translated one say: the parser converts it by CurlyText before any
// other method, and hands it the means to ask. CurlyText stands where
// String does, so a value in a slice or a map, which is written as JSON, is
// asked for neither.
type Texter interface {
	CurlyText(Resolver) string
}

// resolver is the means to ask handed to host code the walk is about to
// call.
func (w *walk) resolver() Resolver {
	w.withLineage()

	return Resolver{w, w.p}
}

// Resolve resolves a message against a context in one walk (section 5). A
// message that is not a string becomes text first, as a payload value does,
// and one nothing describes resolves to the empty string, as does Undefined.
// It returns once its own walk is done, and waits for a modifier's
// goroutine only for the parser code a read there runs under the
// resolution's lock, up to its next call into host code, and for what such
// a read is producing where its own walk needs it. Resolve never panics on
// a message or a payload: a defect resolves the placeholder it is in and is
// told to OnReport. It starts a lineage of its own and is never refused (see
// Modifier).
func (p *Parser) Resolve(message any, context Context) string {
	return p.resolve(message, context, nil)
}

// resolve is Resolve for a resolution of lineage c, whose count of
// resolutions in progress already counts this one; a nil c starts a lineage
// of its own.
func (p *Parser) resolve(message any, context Context, c *lineage) string {
	w := &walk{
		p:          p,
		payload:    context.Payload,
		payloadPtr: reflect.ValueOf(context.Payload).Pointer(),
		props:      context.Props,
		locale:     context.Locale,
		id:         context.ID,
		output:     MaxOutputLength,
		read:       MaxReadLength,
		lineage:    c,
	}

	mu := w.lock()

	mu.Lock()
	defer mu.Unlock()

	defer w.ended()
	defer w.close()

	source, ok := w.convert(message, noSite)

	if !ok {
		return ""
	}

	w.src = source
	w.sc = newScanner(source)

	return w.render(0, len(source), 1)
}
