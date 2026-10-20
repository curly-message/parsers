package curly

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// walk is the state of one resolution: the message, its scanner, what the
// payload is read through, and the budgets the resolution spends (section
// 13). Nothing in it outlives the call, so a nested resolution started from
// inside one owns budgets, a nesting count and a record of its own.
//
// mu is held while parser code runs and released around every call into
// host code, so a modifier that reads its options from goroutines of its
// own serializes them on the parser's state and never deadlocks on it.
// closed is set as the call returns, which waits for none of them: parser
// code that runs after that, on a modifier's goroutine, starts no host code,
// reads nothing more of the payload and spends nothing, and what it produces
// is never answered. chain is the count of its chain: the one Parser.Resolve
// started holds it in its walk, and every resolution asked for through an
// Input of one of them, or a text one renders for a read off its walk,
// counts on, closed or not.
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
	closed     bool
	held       chain
	chain      *chain
}

// chain counts what one chain holds in progress, the resolutions and the
// texts rendered for a read off a walk, and books each read it refuses to
// the host code of the chain that returns first among the code running when
// it was refused. Go names no goroutine, so the code that made the read
// cannot be told apart from other code running then. Each call into host
// code begins an epoch, counted from 1 by began, and a refusal is pending at
// the latest epoch begun, so the code running then is the code that began at
// it or before and has not returned. Pending refusals, one per epoch, form a
// stack with the latest on top, so the code that returns takes off the top
// every refusal made since it began, and fails for them. The stack holds at
// most one entry per call into host code the chain made, and is allocated
// at the first refusal, which no resolution short of the bound makes.
type chain struct {
	count    atomic.Int32
	began    atomic.Int64
	refusals atomic.Pointer[refusals]
}

// refusals is the stack of a chain's pending refusals, and top its latest
// entry, 0 where it is empty.
type refusals struct {
	top     atomic.Int64
	mu      sync.Mutex
	pending []int64
}

// refuse books a refusal at the current epoch. One made before any host
// code of the chain began, or at an epoch already pending, adds nothing.
func (c *chain) refuse() {
	r := c.refusals.Load()

	if r == nil {
		c.refusals.CompareAndSwap(nil, &refusals{})
		r = c.refusals.Load()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if epoch := c.began.Load(); epoch > r.top.Load() {
		r.pending = append(r.pending, epoch)
		r.top.Store(epoch)
	}
}

// failed takes off the pending refusals made since the host code whose
// epoch is epoch began, and reports whether there were any.
func (c *chain) failed(epoch int64) bool {
	r := c.refusals.Load()

	if r == nil || r.top.Load() < epoch {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	n := len(r.pending)

	for n > 0 && r.pending[n-1] >= epoch {
		n--
	}

	if n == len(r.pending) {
		return false
	}

	r.pending = r.pending[:n]
	top := int64(0)

	if n > 0 {
		top = r.pending[n-1]
	}

	r.top.Store(top)

	return true
}

// host runs code the parser does not own, without the walk's lock, and
// reports whether it returned while the call had not: a panic in it is
// contained here, so it resolves the placeholder that ran it rather than
// escaping Resolve. A refusal the chain books to it, as chain says, fails it
// as a panic does, and fails nothing else. Once the walk is closed it runs
// nothing.
func (w *walk) host(f func()) (completed bool) {
	if w.closed {
		return false
	}

	epoch := w.chain.began.Add(1)

	w.mu.Unlock()

	defer func() {
		if !completed {
			_ = recover()
		}

		w.mu.Lock()
		completed = completed && !w.closed

		if w.chain.failed(epoch) {
			completed = false
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

	for !w.closed {
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
// is produced where it is first asked for and never again. calling is
// whether its modifier is being asked, and rendering how many of its texts
// are being produced.
type slot struct {
	w         *walk
	spelling  string
	depth     int
	d         declaration
	wrapper   any
	value     string
	has       bool
	options   []string
	rendered  []settling
	fallback  string
	fell      settling
	calling   bool
	rendering int
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
// rather than producing it, and its placeholders' reports, a second time,
// where take lets a read made by host code get this far. Once the walk is
// closed nobody produces it, and nobody waits.
func (w *walk) claim(state *settling) bool {
	for *state == producing && !w.closed {
		w.wait()
	}

	if *state != unsettled || w.closed {
		return false
	}

	*state = producing

	return true
}

// settle marks what state guards produced, and wakes whoever waits on it.
// What settles once the walk is closed was not produced during the call, so
// it is left unproduced, and answered by no read.
func (w *walk) settle(state *settling) {
	*state = produced

	if w.closed {
		*state = unsettled
	}

	w.wake()
}

// close ends the resolution as the call returns, without waiting for what a
// modifier's goroutine is producing, and wakes whoever waits on that.
func (w *walk) close() {
	w.closed = true
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
	if !present || w.closed {
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
// call, for host code where hosted. Once the walk is closed it is what a
// read produced during the call, or the empty string.
func (s *slot) chain(hosted bool) string {
	if claimed, counted := s.take(&s.fell, hosted); claimed {
		s.rendering++
		text, ok := s.links()

		if !ok && s.d.hasDefault {
			text = s.w.render(s.d.inline[0], s.d.inline[1], s.depth+1)
		}

		s.fallback = text
		s.rendering--
		s.w.settle(&s.fell)
		s.release(counted)
	}

	if s.fell != produced {
		return ""
	}

	return s.fallback
}

// option is the rendered value of the option at i, for host code where
// hosted, and once the walk is closed what a read produced during the call,
// or the empty string.
func (s *slot) option(i int, hosted bool) string {
	if s.rendered == nil {
		s.options = make([]string, len(s.d.options))
		s.rendered = make([]settling, len(s.d.options))
	}

	if claimed, counted := s.take(&s.rendered[i], hosted); claimed {
		span := s.d.options[i].value
		s.rendering++
		s.options[i] = s.w.render(span[0], span[1], s.depth+1)
		s.rendering--
		s.w.settle(&s.rendered[i])
		s.release(counted)
	}

	if s.rendered[i] != produced {
		return ""
	}

	return s.options[i]
}

// take is claim for a read of one of the slot's texts, and reports as well
// whether producing it counts as one resolution of the chain in progress,
// until release. The walk's own reader waits for one producing the text on
// another goroutine, as claim says. A read host code makes is refused at
// once instead, where the text is being produced, as one the read is itself
// inside the render of is, and where it would count past MaxReentry: it
// counts unless the slot's modifier is being asked and none of its texts is
// being produced, where the read is taken for the modifier's own and renders
// on the walk. A refused read produces nothing, and is booked to the slot's
// chain, which fails host code of its own for it as chain says.
func (s *slot) take(state *settling, hosted bool) (claimed, counted bool) {
	w := s.w

	if hosted && !w.closed && *state != produced {
		if *state == producing {
			w.chain.refuse()

			return false, false
		}

		if !s.calling || s.rendering > 0 {
			if w.chain.count.Add(1) > MaxReentry {
				w.chain.count.Add(-1)
				w.chain.refuse()

				return false, false
			}

			counted = true
		}
	}

	return w.claim(state), counted
}

// release ends the count take began for a read.
func (s *slot) release(counted bool) {
	if counted {
		s.w.chain.count.Add(-1)
	}
}

// placeholder resolves one placeholder at a depth MaxNesting allows, in the
// order section 9 reads it.
func (w *walk) placeholder(open, close, depth int) string {
	if w.closed {
		return ""
	}

	// The budget is tested before this placeholder reads, so the one that
	// spent it past the limit resolved all the same and the next one pays.
	if w.read < 0 {
		w.report(ReadLimit, w.src[open:close])

		return ""
	}

	s := w.begin(open, close, depth)

	if w.closed {
		return ""
	}

	name := s.d.modifier
	custom, run, selection, known := w.p.lookup(name)

	if name != "" && !known {
		w.report(UnknownModifier, s.spelling)

		return s.chain(false)
	}

	// A selection asked to select from no options is a defect of how the
	// placeholder is written, so it reports before the value is asked
	// anything; one naming no key compares nothing and is no selection.
	if s.d.hasKey && len(s.d.options) == 0 && selection {
		w.report(MissingOptions, s.spelling)

		return s.chain(false)
	}

	if !s.has {
		return s.chain(false)
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

		return s.chain(false)
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

	s.calling = true
	completed := w.host(func() { answer, err = m(in) })
	s.calling = false

	if !completed || err != nil {
		w.report(FailedModifier, s.spelling)

		return s.chain(false)
	}

	if answer == nil || isUndefinedValue(answer) {
		return s.chain(false)
	}

	text, ok := w.convert(answer, noSite)

	if !ok {
		w.report(UnserializableValue, s.spelling)

		return s.chain(false)
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

	for !w.closed {
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

	return ""
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
