package curly

import (
	"reflect"
	"sync"
)

// walk is the state of one resolution: the message, its scanner, what the
// payload is read through, and the budgets the resolution spends (section
// 13). Nothing in it outlives the call, so a nested resolution started from
// inside one owns budgets, a nesting count and a record of its own.
//
// mu is held while parser code runs and released around every call into
// host code, so a modifier that reads its options from goroutines of its
// own serializes them on the parser's state and never deadlocks on it.
type walk struct {
	mu         sync.Mutex
	p          *Parser
	src        string
	sc         *scanner
	payload    map[string]any
	payloadPtr uintptr
	props      map[string]any
	locale     *string
	id         string
	output     int
	read       int
	record     map[recordKey]recorded
	settled    *sync.Cond
}

// host runs code the parser does not own, without the walk's lock, and
// reports whether it returned: a panic in it is contained here, so it
// resolves the placeholder that ran it rather than escaping Resolve.
func (w *walk) host(f func()) (completed bool) {
	w.mu.Unlock()

	defer func() {
		if !completed {
			_ = recover()
		}

		w.mu.Lock()
	}()

	f()

	return true
}

// render is a span of the message: the text it writes itself with its
// escape sequences removed, and the placeholders it writes resolved at
// depth. The whole message is one such span at depth 1, and so is each
// option value and inline default one level below the placeholder holding
// it. Past MaxNesting every placeholder is refused, and the walk over what
// they fall back to goes on a stack of its own rather than the goroutine's.
func (w *walk) render(from, to, depth int) string {
	if depth > MaxNesting {
		return w.renderRefused(from, to)
	}

	var out []byte

	at := from

	for {
		open, close, ok := w.sc.next(at, to)

		if !ok {
			break
		}

		out = append(out, unesc(w.src[at:open])...)
		resolved := w.placeholder(open, close, depth)

		// Only what the output carries is counted, and a result nested in
		// another reaches it through the one around it, so the charge is
		// made where the walk meets the message itself.
		switch {
		case depth > 1:
			out = append(out, resolved...)
		case len(resolved) <= w.output:
			w.output -= len(resolved)
			out = append(out, resolved...)
		default:
			w.report(OutputLimit, w.src[open:close])
		}

		at = close
	}

	if out == nil {
		return unesc(w.src[at:to])
	}

	return string(append(out, unesc(w.src[at:to])...))
}

// slot is one placeholder being resolved: what it declares, what it read,
// and what it has rendered so far, so an option value or the fallback chain
// is produced where it is first asked for and never again.
type slot struct {
	w        *walk
	spelling string
	depth    int
	d        declaration
	wrapper  any
	value    string
	has      bool
	options  []string
	rendered []settling
	fallback string
	fell     settling
}

// settling is how far an option value or a fallback chain is produced.
type settling uint8

const (
	unsettled settling = iota
	producing
	produced
)

// claim reports whether the caller is the one to produce what state
// guards. A placeholder is reached at most once (section 5), so a reader
// arriving from another goroutine while one produces it, which happens while
// the producer has the lock released around host code, waits for that one
// rather than producing it, and its placeholders' reports, a second time.
func (w *walk) claim(state *settling) bool {
	for *state == producing {
		w.wait()
	}

	if *state == produced {
		return false
	}

	*state = producing

	return true
}

// settle marks what state guards produced, and wakes whoever waits on it.
func (w *walk) settle(state *settling) {
	*state = produced
	w.wake()
}

// wait holds the caller until something another goroutine produces is
// settled.
func (w *walk) wait() {
	if w.settled == nil {
		w.settled = sync.NewCond(&w.mu)
	}

	w.settled.Wait()
}

func (w *walk) wake() {
	if w.settled != nil {
		w.settled.Broadcast()
	}
}

// begin is what every placeholder does before it can be refused: parse,
// look its value up, and read it, which spends the read budget and reports
// a value no conversion describes.
func (w *walk) begin(open, close, depth int) slot {
	s := slot{w: w, spelling: w.src[open:close], depth: depth}
	s.d = parsePlaceholder(w.src, open, close, w.sc)

	// A placeholder naming no key looks nothing up (section 9.2).
	if !s.d.hasKey {
		return s
	}

	entry, present := w.payload[s.d.key]

	if present && isUndefinedValue(entry) {
		present = false
	}

	at := site{w.payloadPtr, s.d.key}

	// The payload's root default is the fallback itself, never
	// configuration (section 4.1).
	if present && !w.p.ignoreWrappers && s.d.key != "default" && isWrapper(entry) {
		s.wrapper = entry
		entry, at, present = member(entry, "value")
	}

	s.value, s.has = w.payloadText(entry, present, at, s.spelling)

	return s
}

// payloadText is the text a value read from the payload converts to,
// charged to the read budget whether or not any of it reaches the output.
func (w *walk) payloadText(v any, present bool, at site, spelling string) (string, bool) {
	if !present {
		return "", false
	}

	text, ok := w.convert(v, at)

	if !ok {
		w.report(UnserializableValue, spelling)

		return "", false
	}

	w.read -= len(text)

	return text, true
}

// links is the payload's part of the fallback chain (section 10): the
// wrapper's default, then the payload's. A placeholder keyed default has
// already read the payload's as its value, and reads it again neither for
// the report nor for the budget.
func (s *slot) links() (string, bool) {
	w := s.w

	if s.wrapper != nil {
		v, at, present := member(s.wrapper, "default")

		if text, ok := w.payloadText(v, present, at, s.spelling); ok {
			return text, true
		}
	}

	if s.d.hasKey && s.d.key == "default" {
		return s.value, s.has
	}

	v, present := w.payload["default"]

	return w.payloadText(v, present && !isUndefinedValue(v), site{w.payloadPtr, "default"}, s.spelling)
}

// chain is the placeholder's fallback chain, walked on the first reader's
// call.
func (s *slot) chain() string {
	if s.w.claim(&s.fell) {
		text, ok := s.links()

		if !ok && s.d.hasDefault {
			text = s.w.render(s.d.inline[0], s.d.inline[1], s.depth+1)
		}

		s.fallback = text
		s.w.settle(&s.fell)
	}

	return s.fallback
}

// option is the rendered value of the option at i.
func (s *slot) option(i int) string {
	if s.rendered == nil {
		s.options = make([]string, len(s.d.options))
		s.rendered = make([]settling, len(s.d.options))
	}

	if s.w.claim(&s.rendered[i]) {
		span := s.d.options[i].value
		s.options[i] = s.w.render(span[0], span[1], s.depth+1)
		s.w.settle(&s.rendered[i])
	}

	return s.options[i]
}

// placeholder resolves one placeholder at a depth MaxNesting allows, in the
// order section 9 reads it.
func (w *walk) placeholder(open, close, depth int) string {
	// The budget is tested before this placeholder reads, so the one that
	// spent it past the limit resolved all the same and the next one pays.
	if w.read < 0 {
		w.report(ReadLimit, w.src[open:close])

		return ""
	}

	s := w.begin(open, close, depth)
	name := s.d.modifier
	custom, run, selection, known := w.p.lookup(name)

	if name != "" && !known {
		w.report(UnknownModifier, s.spelling)

		return s.chain()
	}

	// A selection asked to select from no options is a defect of how the
	// placeholder is written, so it reports before the value is asked
	// anything; one naming no key compares nothing and is no selection.
	if s.d.hasKey && len(s.d.options) == 0 && selection {
		w.report(MissingOptions, s.spelling)

		return s.chain()
	}

	if !s.has {
		return s.chain()
	}

	if name == "" {
		if len(s.d.options) == 0 {
			return s.value
		}

		name = "eq"
		custom, run, _, _ = w.p.lookup(name)
	}

	if custom != nil {
		return w.call(&s, name, custom)
	}

	text, code := run(&s)

	if code != "" {
		w.report(code, s.spelling)

		// A locale nobody supplied is the one failure the chain does not
		// answer: a default stands in for a value, never for a locale.
		if code == MissingLocale {
			return ""
		}

		return s.chain()
	}

	return text
}

// lookup is the modifier answering to name: the host's where it registered
// one, the format's otherwise. Only the format's own selections are
// selections; a host modifier in a selection's place is asked like any.
func (p *Parser) lookup(name string) (custom Modifier, run func(*slot) (string, Code), selection, known bool) {
	if m, ok := p.custom[name]; ok {
		return m, nil, false, true
	}

	run, selection, known = builtin(name)

	return nil, run, selection, known
}

// call asks a host modifier, contained, and converts its answer as a
// payload value converts, charged to the output and not to the read budget:
// it is the placeholder's result rather than text it read.
func (w *walk) call(s *slot, name string, m Modifier) string {
	in := Input{Value: s.value, Props: w.composed(name, s.wrapper), s: s}

	if w.locale != nil {
		locale := *w.locale
		in.Locale = &locale
	}

	if len(s.d.options) > 0 {
		in.Options = make([]Option, len(s.d.options))

		for i, o := range s.d.options {
			in.Options[i] = Option{Key: o.key, s: s, i: i}
		}
	}

	var answer any

	var err error

	if !w.host(func() { answer, err = m(in) }) || err != nil {
		w.report(FailedModifier, s.spelling)

		return s.chain()
	}

	if answer == nil || isUndefinedValue(answer) {
		return s.chain()
	}

	text, ok := w.convert(answer, noSite)

	if !ok {
		w.report(UnserializableValue, s.spelling)

		return s.chain()
	}

	return text
}

// renderRefused is render past MaxNesting, where every placeholder is
// refused and resolves to its fallback chain. The chain's inline default is
// message text holding placeholders refused in turn, so each one is a frame
// on this stack rather than a call: a message nested to any depth costs the
// goroutine's stack nothing, and every refused placeholder reports.
func (w *walk) renderRefused(from, to int) string {
	type frame struct {
		at, to int
		out    []byte
	}

	stack := []frame{{at: from, to: to}}

	for {
		f := &stack[len(stack)-1]
		open, close, ok := w.sc.next(f.at, f.to)

		if !ok {
			f.out = append(f.out, unesc(w.src[f.at:f.to])...)

			if len(stack) == 1 {
				return string(f.out)
			}

			done := f.out
			stack = stack[:len(stack)-1]
			stack[len(stack)-1].out = append(stack[len(stack)-1].out, done...)

			continue
		}

		f.out = append(f.out, unesc(w.src[f.at:open])...)
		f.at = close

		if w.read < 0 {
			w.report(ReadLimit, w.src[open:close])

			continue
		}

		s := w.begin(open, close, 0)
		w.report(NestingLimit, s.spelling)

		if text, ok := s.links(); ok {
			f.out = append(f.out, text...)
		} else if s.d.hasDefault {
			stack = append(stack, frame{at: s.d.inline[0], to: s.d.inline[1]})
		}
	}
}

func isUndefinedValue(v any) bool {
	_, ok := v.(Undefined)

	return ok
}

// isWrapper reports whether a payload entry is a value wrapper (section
// 4.1): a plain object of one to three members, each named value, default
// or props. Recognition reads at most three keys, so it costs the same
// whatever the entry holds.
func isWrapper(v any) bool {
	switch m := v.(type) {
	case map[string]any:
		if len(m) == 0 || len(m) > 3 {
			return false
		}

		for key := range m {
			if !wrapped(key) {
				return false
			}
		}

		return true
	case nil, string, float64, int, bool:
		return false
	}

	rv := reflect.ValueOf(v)

	if !plainObject(rv.Type()) || rv.Len() == 0 || rv.Len() > 3 {
		return false
	}

	for it := rv.MapRange(); it.Next(); {
		if !wrapped(it.Key().String()) {
			return false
		}
	}

	return true
}

func wrapped(key string) bool {
	return key == "value" || key == "default" || key == "props"
}

// member is a wrapper's member, where it is read, and whether it is there;
// one holding Undefined is not.
func member(wrapper any, name string) (any, site, bool) {
	if m, ok := wrapper.(map[string]any); ok {
		v, present := m[name]

		return v, site{reflect.ValueOf(m).Pointer(), name}, present && !isUndefinedValue(v)
	}

	rv := reflect.ValueOf(wrapper)
	at := siteOf(rv, name)
	v := rv.MapIndex(reflect.ValueOf(name))

	if !v.IsValid() {
		return nil, at, false
	}

	value := v.Interface()

	return value, at, !isUndefinedValue(value)
}
