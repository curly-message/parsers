package curly

import (
	"errors"
	"reflect"
	"sync/atomic"
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

// MaxReentry is how many resolutions one chain holds in progress at once,
// counted through Input.Resolve. A chain is a resolution Parser.Resolve
// starts and every one asked for through an Input of one of them, whichever
// of their Inputs it is: a resolution asked for there counts as one more
// than those of its chain in progress, and one that would count past
// MaxReentry is refused. It is the bound section 13 lets an implementation
// set in place of the host's limit on how deep a resolution that reaches
// itself goes, which in Go is a goroutine's stack outgrowing its maximum, a
// fatal error. A resolution Parser.Resolve starts counts as the first. A
// chain of MaxReentry resolutions, each asked for from a placeholder at
// MaxNesting, takes about 9.4 MB of the goroutine's stack on a 64-bit
// platform and 5.9 MB on a 32-bit one, and at most about 47 MB and 26 MB
// with Go 1.26, 77 MB and 47 MB with Go 1.27, where each is asked for from
// inside encoding/json, which is handed no value nested deeper than 64
// levels, of the 512 MiB and 128 MiB a goroutine's stack reaches at most by
// default: it grows by doubling, up to Go's limits of 1 GB and 250 MB.
const MaxReentry = 1000

// ErrReentryLimit is the error Input.Resolve returns for a resolution it
// refuses, one that would count past MaxReentry.
var ErrReentryLimit = errors.New("curly: more than MaxReentry resolutions in progress")

// hostDepth is how many levels a value the parser hands to encoding/json
// may nest, as prewalk counts them over every field encoding/json may write.
// encoding/json recurses on the goroutine's stack, so this bounds what one
// conversion adds to it. A resolution a method asks for through an Input, as
// encoding/json writes a value the asking resolution converts, sits below at
// most hostDepth levels of encoding/json, and this bound is what keeps what
// it adds to a chain's stack within what MaxReentry allows for: the levels
// take about 560 bytes each on a 64-bit platform and 300 on a 32-bit one
// with Go 1.26, and 1 020 and 620 with Go 1.27, where encoding/json writes a
// typed map of typed maps, the costliest shape measured.
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
// waits for itself: until Resolve returns, on a goroutine of its own, and
// forever on the goroutine that called Resolve.
//
// The parser makes Resolve wait for a modifier's goroutine in two ways. The
// resolution holds one lock while parser code runs, on any goroutine, and
// releases it around every call into host code, so each time the goroutine
// that called Resolve needs the lock, it also waits for the parser code a
// read on a modifier's goroutine is running under it, each read up to its
// next call into host code, a whole conversion included: work the limits of
// section 13 bound, never host code. And where code on the goroutine that
// called Resolve reads what a read is producing, it waits for that read by
// the rule above: the walk, which reads the fallback chain a placeholder
// takes after its modifier was asked and the values it converts, or any
// host code the walk calls there. Host code that waits for Resolve to
// return while it produces what that code reads waits forever, and so does
// Resolve. A read that finished during the call answers its text again
// after it. One still producing an option or default when Resolve returns,
// in host code or waiting to take the resolution's lock back, goes on only
// to finish: it calls no more host code, reads no more of the payload,
// reports nothing and spends no budget, and it answers the empty string. Any
// other read once Resolve has returned, one still waiting for the lock or
// for that producer among them, answers what a read finished during the
// call, or the empty string where none did. Host code still running when
// Resolve returns, a method of a value or a modifier's goroutine, can read
// the payload until it returns, so the caller changes the payload only once
// that code has returned.
//
// A modifier that resolves a message through Input.Resolve passes on the
// count MaxReentry bounds, through any Input of the chain, on whatever
// goroutine it calls it, so resolutions of one chain in progress at once on
// several goroutines count together. One that resolves through
// Parser.Resolve starts a chain of its own, counted from the first: Go names
// no goroutine a count could be kept for, so where resolutions started that
// way reach themselves without end, the goroutine's stack outgrows its
// maximum and the process ends.
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
// first call. Once Resolve has returned, it is the chain a call finished
// walking during the resolution, or the empty string where none did.
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
// modifier never asks for is never resolved. Once Resolve has returned, it
// is the value a call finished rendering during the resolution, or the empty
// string where none did.
func (o Option) Value() string {
	if o.s == nil {
		return ""
	}

	w := o.s.w
	w.mu.Lock()
	defer w.mu.Unlock()

	return o.s.option(o.i)
}

// Resolve resolves a message against a context as p.Resolve does, as a
// resolution the modifier asks for: it counts as one more than the
// resolutions in progress of the chain in's resolution belongs to, whether
// that one is in progress or has returned, and so as the first where none
// of them is in progress; the zero Input counts from none. One that would
// count past MaxReentry resolves nothing and returns ErrReentryLimit, which
// the modifier handles as any error, and which, returned as the modifier's
// own, fails the modifier. Only the resolutions are counted, never the walk
// of one: an option or default it renders counts nothing.
func (in Input) Resolve(p *Parser, message any, context Context) (string, error) {
	if in.s == nil {
		return p.Resolve(message, context), nil
	}

	chain := in.s.w.chain

	if chain.Add(1) > MaxReentry {
		chain.Add(-1)

		return "", ErrReentryLimit
	}

	return p.resolve(message, context, chain), nil
}

// Resolve resolves a message against a context in one walk (section 5). A
// message that is not a string becomes text first, as a payload value does,
// and one nothing describes resolves to the empty string, as does Undefined.
// It returns once its own walk is done, and waits for a modifier's
// goroutine only as Modifier says: for the parser code a read there runs
// under the resolution's lock, up to its next call into host code, and for
// what a read there is producing where code on its own goroutine reads it.
// Resolve never panics on a message or a payload: a defect resolves the
// placeholder it is in and is told to OnReport.
func (p *Parser) Resolve(message any, context Context) string {
	return p.resolve(message, context, nil)
}

// resolve is Resolve for a resolution of the chain whose count of
// resolutions in progress chain is, already counting this one; a nil chain
// starts one of its own.
func (p *Parser) resolve(message any, context Context, chain *atomic.Int32) string {
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

	if chain == nil {
		w.held.Store(1)
		chain = &w.held
	}

	w.chain = chain
	defer chain.Add(-1)

	w.mu.Lock()
	defer w.mu.Unlock()
	defer w.close()

	source, ok := w.convert(message, noSite)

	if !ok {
		return ""
	}

	w.src = source
	w.sc = newScanner(source)

	return w.render(0, len(source), 1)
}
