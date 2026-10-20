package curly

import "reflect"

// walk is the state of one resolution: the message, its scanner, what the
// payload is read through, and the budgets the resolution spends (section
// 13). Nothing in it outlives the call, so a nested resolution started from
// inside one owns budgets, a nesting count and a record of its own.
type walk struct {
	p          *Parser
	src        string
	sc         *scanner
	payload    map[string]any
	payloadPtr uintptr
	id         string
	output     int
	read       int
	record     map[recordKey]recorded
}

// host runs code the parser does not own and reports whether it returned:
// a panic in it is contained here, so it resolves the placeholder that ran
// it rather than escaping Resolve.
func (w *walk) host(f func()) (completed bool) {
	defer func() {
		if !completed {
			_ = recover()
		}
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
	rendered []bool
	fallback string
	fell     bool
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
	if !s.fell {
		text, ok := s.links()

		if !ok && s.d.hasDefault {
			text = s.w.render(s.d.inline[0], s.d.inline[1], s.depth+1)
		}

		s.fallback, s.fell = text, true
	}

	return s.fallback
}

// option is the rendered value of the option at i.
func (s *slot) option(i int) string {
	if s.rendered == nil {
		s.options = make([]string, len(s.d.options))
		s.rendered = make([]bool, len(s.d.options))
	}

	if !s.rendered[i] {
		span := s.d.options[i].value
		s.options[i], s.rendered[i] = s.w.render(span[0], span[1], s.depth+1), true
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
	run, selection, known := builtin(name)

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

		run, _, _ = builtin("eq")
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
