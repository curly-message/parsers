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
// Parser code runs under a lock, released around every call into host code,
// so a modifier that reads its options from goroutines of its own serializes
// them on the parser's state and never deadlocks on it. The lock is mu until
// the walk has a lineage, and the lineage's after. closed is set as the call
// returns, which waits for none of them: parser code that runs after that,
// on a modifier's goroutine, starts no host code, reads nothing more of the
// payload and spends nothing, and what it produces is never answered.
//
// lineage is nil until the resolution hands an Input to a modifier, and in a
// resolution asked for through one it is there from the start: a resolution
// that asks no host-defined modifier keeps no account of what host code
// holds of it. composing is nil until a modifier's properties are composed.
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
	lineage    *lineage
	composing  *composing
	closed     bool
	reading    bool
	refused    bool
}

// lineage is what the resolutions of one lineage share: the one
// Parser.Resolve started, which sets it up as it first hands an Input to a
// modifier, and every one asked for through an Input of one of them. mu is
// the lock of the walk that set it up, which every walk of the lineage runs
// its parser code under, and settled is signalled on it as something a read
// produces is settled. held is how many resolutions of the lineage are in
// progress, closed or not, and how many reads that count as one are
// producing their text (see slot.enter). cur is the placeholder whose
// modifier the lineage called last of those that have not returned, and
// owns how many reads are producing text of the placeholder current as they
// began.
type lineage struct {
	mu      *sync.Mutex
	settled *sync.Cond
	held    int
	owns    int
	cur     *slot
}

// lock is the lock the walk's parser code runs under.
func (w *walk) lock() *sync.Mutex {
	if w.lineage != nil {
		return w.lineage.mu
	}

	return &w.mu
}

// withLineage is the walk's lineage, set up where it has none: for the walk
// of the resolution Parser.Resolve started, which it counts as the first in
// progress, and whose lock it keeps.
func (w *walk) withLineage() *lineage {
	if w.lineage == nil {
		w.lineage = &lineage{mu: &w.mu, held: 1}
	}

	return w.lineage
}

// ended gives back the place the resolution held among those of its lineage
// in progress, where it has a lineage.
func (w *walk) ended() {
	if w.lineage != nil {
		w.lineage.held--
	}
}

// host runs code the parser does not own, without the walk's lock, and
// reports whether it returned while the call had not: a panic in it is
// contained here, so it resolves the placeholder that ran it rather than
// escaping Resolve. Once the walk is closed it runs nothing.
func (w *walk) host(f func()) (completed bool) {
	if w.stopped() {
		return false
	}

	mu, reading := w.lock(), w.reading

	mu.Unlock()

	defer func() {
		if !completed {
			_ = recover()
		}

		mu.Lock()

		w.reading = reading

		completed = completed && !w.closed
	}()

	f()

	return true
}

// asking makes s the placeholder whose modifier the lineage called last.
func (w *walk) asking(s *slot) {
	c := w.lineage
	s.outer, s.called = c.cur, calling
	c.cur = s
}

// answered marks the call of the modifier of s returned, and leaves current
// the last called before it that has not.
func (w *walk) answered(s *slot) {
	c := w.lineage
	s.called = returned

	for c.cur != nil && c.cur.called == returned {
		c.cur = c.cur.outer
	}
}

// enter is what a read of an option or default of s does where host code
// made it: it reports whether the read is the one to produce what state
// guards. A read of what is being produced is refused (section 13): it
// answers the empty string, is not recorded, and fails nothing.
//
// A read that produces is the walk's, and counts nothing, where the modifier
// asked about s is current in the lineage and nothing of s is being produced:
// what it renders is nested one level below s, so MaxNesting bounds how many
// such reads stand one inside another beneath each resolution or counted
// read of the lineage, and one past that many, which only a read outliving
// its modifier's call makes, is not the walk's. Any other renders at the
// depth of s whatever host code made it, so it counts as one more resolution
// of the lineage in progress until it settles, and one that would count past
// MaxReentry is refused as above.
func (s *slot) enter(state *settling) bool {
	w := s.w
	c := w.lineage

	if *state != unsettled || w.stopped() {
		return false
	}

	if c.cur == s && s.busy == 0 && c.owns < MaxNesting*c.held {
		c.owns++
		*state = owned

		return true
	}

	if c.held >= MaxReentry {
		return false
	}

	c.held++
	*state = counted

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

	for !w.stopped() {
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
// is produced where it is first asked for and never again. Where it has a
// lineage, called is how far the call of its modifier is, outer the
// placeholder current as it was called, and busy how many of its option
// values and its fallback chain are being produced.
type slot struct {
	w        *walk
	outer    *slot
	open     int
	spelling string
	depth    int
	d        declaration
	wrapper  any
	value    string
	options  []string
	rendered []settling
	fallback string
	has      bool
	fell     settling
	called   call
	busy     int32
}

// call is how far the call of a placeholder's modifier is.
type call uint8

const (
	uncalled call = iota
	calling
	returned
)

// settling is how far an option value or a fallback chain is produced.
type settling uint8

const (
	unsettled settling = iota
	producing
	// counted is producing by a read that counts as a resolution in
	// progress until it settles.
	counted
	// owned is producing by a read of the placeholder of the modifier
	// current as it began, which counts on owns until it settles.
	owned
	produced
)

// claim reports whether the caller is the one to produce what state
// guards. A placeholder is reached at most once (section 5), so the walk,
// arriving from another goroutine while one produces it, which happens while
// the producer has the lock released around host code, waits for that one
// rather than producing it, and its placeholders' reports, a second time. A
// read host code made waits for none (section 13): it is refused, is not
// recorded, and the placeholder that asked takes the empty string. Once the
// walk is closed nobody produces it, and nobody waits.
func (w *walk) claim(state *settling) bool {
	for (*state == producing || *state == counted || *state == owned) && !w.stopped() {
		if w.reading {
			return false
		}

		w.wait()
	}

	if *state != unsettled || w.stopped() {
		return false
	}

	*state = producing

	return true
}

// settle marks what state guards produced, and wakes whoever waits on it.
// What settles once the walk is closed was not produced during the call, so
// it is left unproduced, and answered by no read. What settles under a read
// the walk refused is produced, as the empty string (see walk.answer): its
// placeholders are reached once, so no later read produces it.
func (w *walk) settle(state *settling) {
	switch *state {
	case counted:
		w.lineage.held--
	case owned:
		w.lineage.owns--
	}

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
// settled. Only what a read by host code produces is ever waited for, so the
// walk has a lineage.
func (w *walk) wait() {
	c := w.lineage

	if c.settled == nil {
		c.settled = sync.NewCond(c.mu)
	}

	reading := w.reading

	c.settled.Wait()

	w.reading = reading
}

// stopped reports whether the walk produces nothing more: for good once the
// resolution has returned, and until the read returns where a read host code
// made met a value being converted.
func (w *walk) stopped() bool {
	return w.closed || w.refused
}

// hostRead marks the lock's holder a read host code made, and returns what
// ends the read: it clears a refusal the read met and gives the mark back.
func (w *walk) hostRead() (done func()) {
	reading := w.reading
	w.reading = true

	return func() {
		w.reading, w.refused = reading, false
	}
}

// answer is what a read that produced text records for it: the text, or the
// empty string where the read was refused as it produced it.
func (w *walk) answer(text string) string {
	if w.refused {
		return ""
	}

	return text
}

func (w *walk) wake() {
	if c := w.lineage; c != nil && c.settled != nil {
		c.settled.Broadcast()
	}
}

// begin is what every placeholder does before it can be refused: parse,
// look its value up, and read it, which spends the read budget and reports
// a value no conversion describes.
func (w *walk) begin(open, close, depth int) slot {
	s := slot{w: w, open: open, spelling: w.src[open:close], depth: depth}
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
	if !present || w.stopped() {
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

// chain is the placeholder's fallback chain as the walk takes it, walked on
// the first reader's call.
func (s *slot) chain() string {
	return s.chainRead(false)
}

// chainRead is the placeholder's fallback chain, walked on the first
// reader's call, which is host code's where read is set. Once the walk is
// closed it is what a read produced during the call, or the empty string,
// and so is a read the walk refused where it met a value being converted,
// for the rest of the resolution.
func (s *slot) chainRead(read bool) string {
	w := s.w

	var inline [2]int

	if s.d.hasDefault {
		inline = s.d.inline
	}

	if read && s.enter(&s.fell) || !read && w.claim(&s.fell) {
		s.busy++
		text, ok := s.links()

		if !ok && s.d.hasDefault {
			text = w.render(inline[0], inline[1], s.depth+1)
		}

		s.fallback = w.answer(text)
		s.busy--
		w.settle(&s.fell)
	}

	if s.fell != produced {
		return ""
	}

	return s.fallback
}

// option is the rendered value of the option at i as the walk reads it.
func (s *slot) option(i int) string {
	return s.optionRead(i, false)
}

// optionRead is the rendered value of the option at i, which host code
// reads where read is set, and once the walk is closed what a read produced
// during the call, or the empty string, as is one the walk refused where it
// met a value being converted, for the rest of the resolution.
func (s *slot) optionRead(i int, read bool) string {
	if s.rendered == nil {
		s.options = make([]string, len(s.d.options))
		s.rendered = make([]settling, len(s.d.options))
	}

	span := s.d.options[i].value

	if read && s.enter(&s.rendered[i]) || !read && s.w.claim(&s.rendered[i]) {
		s.busy++
		s.options[i] = s.w.answer(s.w.render(span[0], span[1], s.depth+1))
		s.busy--
		s.w.settle(&s.rendered[i])
	}

	if s.rendered[i] != produced {
		return ""
	}

	return s.options[i]
}

// placeholder resolves one placeholder at a depth MaxNesting allows, in the
// order section 9 reads it.
func (w *walk) placeholder(open, close, depth int) string {
	if w.stopped() {
		return ""
	}

	// The budget is tested before this placeholder reads, so the one that
	// spent it past the limit resolved all the same and the next one pays.
	if w.read < 0 {
		w.report(ReadLimit, w.src[open:close])

		return ""
	}

	s := w.begin(open, close, depth)

	if w.stopped() {
		return ""
	}

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

	// Where host code holds an Input of the lineage, a modifier the format
	// defines is current while it runs, as one a host defines is.
	var text string

	var code Code

	if w.lineage != nil {
		w.asking(&s)
		text, code = run(&s)
		w.answered(&s)
	} else {
		text, code = run(&s)
	}

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

	if !known && p.intl != nil {
		run, selection, known = intlBuiltin(name)
	}

	return nil, run, selection, known
}

// call asks a host modifier, contained, and converts its answer as a
// payload value converts, charged to the output and not to the read budget:
// it is the placeholder's result rather than text it read.
func (w *walk) call(s *slot, name string, m Modifier) string {
	in := Input{Value: s.value, Props: w.composition(name, s.wrapper, false, nil).props, s: s}

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

	w.withLineage()
	w.asking(s)
	completed := w.host(func() { answer, err = m(in) })
	w.answered(s)

	if !completed || err != nil {
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

	for !w.stopped() {
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
