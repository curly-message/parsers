package curly

import (
	"bytes"
	"math"
	"reflect"
	"runtime"
	"slices"
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
// is never answered. held is how many resolutions of its chain are in
// progress, kept in the walk of the one Parser.Resolve started: that one,
// every one asked for through an Input of one of them, which Input.Resolve
// counts on, closed or not, and every read of an option or default that
// counts as one while it produces its text (see slot.enter).
//
// side is nil until the resolution hands an Input to a modifier, and in a
// resolution asked for through one it is there from the start: a resolution
// that asks no host-defined modifier keeps no account of its calls into
// host code, since nothing it handed out can be read from them. composing
// is nil until a modifier's properties are composed.
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
	side       *side
	composing  *composing
	closed     bool
	held       atomic.Int32
}

// side is what a resolution keeps once host code holds an Input of its
// chain. on is the thread the strand running the walk's code is on, a
// strand being that code from where it was entered, by Resolve or by a read
// of an option or default, to where that returns; it is nil until the walk
// has told, which it does at the strand's first call into host code, and
// fresh is set where it has told only that the strand began on a goroutine
// no call into host code is in progress on. made is the thread the strand
// set up for such a goroutine, base the span of the message the strand
// produces, pos where the placeholder it reached last opens, and linking is
// set while it reads the payload's links of that placeholder's fallback
// chain: what a call into host code is made from within is told by the
// three. asking is the placeholder whose modifier the next call into host
// code asks. A call into host code keeps what its strand runs with and gives
// it back as it returns, and so does a wait, since other strands run
// meanwhile.
//
// root is the walk of the resolution Parser.Resolve started, nil in that
// walk itself, whose side holds what the chain shares: main, the thread of
// the goroutine the chain runs on unless told otherwise, and others, the
// threads of the goroutines host code started, by the number the runtime
// gives each.
type side struct {
	settled *sync.Cond
	root    *walk
	on      *thread
	made    *thread
	asking  *slot
	base    [2]int
	pos     int
	fresh   bool
	linking bool

	main   thread
	mu     sync.Mutex
	others map[uint64]*thread
	apart  atomic.Int32
}

// thread is the calls into host code in progress on one goroutine, the
// innermost last: Go names no goroutine, so the walk tells one from another
// by whether a call into host code is on its stack, and, once host code has
// started goroutines that are in host code themselves, by the number the
// runtime writes at the head of a stack trace.
type thread struct {
	mu    sync.Mutex
	id    uint64
	calls []hostCall
	first [1]hostCall
}

// hostCall is one call into host code: the walk that made it, the
// placeholder whose modifier it asks, nil where it is any other host code,
// what the strand it was made from gets back as it returns, whether it has
// returned, and whether a read made during it was refused, which fails it.
type hostCall struct {
	w        *walk
	asked    *slot
	made     *thread
	base     [2]int
	pos      int
	linking  bool
	returned bool
	refused  bool
}

// whole is the span of a strand that produces the message itself.
var whole = [2]int{0, math.MaxInt}

// sided is the walk's side, set up where the walk has none: for the walk of
// the resolution Parser.Resolve started, whose strand is on the chain's main
// thread.
func (w *walk) sided() *side {
	if w.side == nil {
		x := w.aside(nil)
		x.main.calls = x.main.first[:0]
		x.on = &x.main
	}

	return w.side
}

// aside gives the walk a side, for a strand that produces the message and
// has not told its thread: root is the walk that began the chain, nil where
// w is that walk.
func (w *walk) aside(root *walk) *side {
	x := &side{root: root, base: whole}
	w.side = x

	return x
}

// first is the walk of the resolution that began the chain w belongs to,
// which holds the chain's count and its threads.
func (w *walk) first() *walk {
	if w.side != nil && w.side.root != nil {
		return w.side.root
	}

	return w
}

// host runs code the parser does not own, without the walk's lock, and
// reports whether it returned while the call had not, having made no read
// the walk refused: a panic in it is contained here, so it resolves the
// placeholder that ran it rather than escaping Resolve. Once the walk is
// closed it runs nothing.
//
//go:noinline
func (w *walk) host(f func()) (completed bool) {
	if w.closed {
		return false
	}

	t, at := w.calling()

	w.mu.Unlock()

	defer func() {
		if !completed {
			_ = recover()
		}

		w.mu.Lock()
		completed = !w.returned(t, at) && completed && !w.closed
	}()

	f()

	return true
}

// calling puts a call into host code among those in progress on the thread
// the running strand is on, telling that thread first where the strand has
// not, and answers the thread and the call's index there. A walk without a
// side keeps none. It is a call of its own so that it costs host, which
// stays on the goroutine's stack for as long as the host code runs, no
// frame.
//
//go:noinline
func (w *walk) calling() (*thread, int) {
	x := w.side

	if x == nil {
		return nil, 0
	}

	asked := x.asking
	x.asking = nil

	if x.on == nil {
		shared := w.first().side
		fresh := x.fresh

		if !fresh {
			x.on, fresh = shared.whose()
		}

		if fresh {
			x.on = shared.apartThread()
			x.made = x.on
		}
	}

	t := x.on

	t.mu.Lock()
	t.calls = append(t.calls, hostCall{w: w, asked: asked, made: x.made, base: x.base, pos: x.pos, linking: x.linking})
	at := len(t.calls) - 1
	t.mu.Unlock()

	return t, at
}

// returned takes the call at index at off the calls in progress on t, gives
// the strand that made it back what it runs with, and reports whether a
// read made during the call was refused. An index stands for one call for
// as long as that call is in progress.
func (w *walk) returned(t *thread, at int) (refused bool) {
	if t == nil {
		return false
	}

	t.mu.Lock()

	call := t.calls[at]
	t.calls[at] = hostCall{returned: true}

	n := len(t.calls)

	for n > 0 && t.calls[n-1].returned {
		n--
	}

	t.calls = t.calls[:n]

	t.mu.Unlock()

	x := w.side
	x.on, x.made, x.fresh, x.base = t, call.made, false, call.base
	x.pos, x.linking = call.pos, call.linking

	return call.refused
}

// asking reports whether the call in progress made last on t asks the
// modifier of s.
func (t *thread) asking(s *slot) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return len(t.calls) > 0 && t.calls[len(t.calls)-1].asked == s
}

// refuse fails the call in progress made last on t, if any.
func (t *thread) refuse() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.calls) > 0 {
		t.calls[len(t.calls)-1].refused = true
	}
}

// producing reports whether a call in progress on t was made from within
// what state guards for s: the option value or inline default at span, or,
// where links is set, the payload's links of the fallback chain. A strand
// renders only what it claimed itself, so a call made at a placeholder
// inside the span, by a strand whose own span holds it, is inside its
// rendering.
func (t *thread) producing(s *slot, span [2]int, links bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, call := range t.calls {
		if call.w != s.w || call.returned {
			continue
		}

		if links && call.linking && call.pos == s.open {
			return true
		}

		if call.base[0] <= span[0] && span[1] <= call.base[1] && span[0] <= call.pos && call.pos < span[1] {
			return true
		}
	}

	return false
}

// whose tells the thread the calling goroutine is on, from parser code that
// host code called or that a strand runs: fresh where no call into host
// code is in progress on it, the thread set up for it where host code
// started it, and the chain's main thread otherwise.
func (x *side) whose() (t *thread, fresh bool) {
	if !hosted() {
		return nil, true
	}

	if x.apart.Load() == 0 {
		return &x.main, false
	}

	id := goroutine()

	x.mu.Lock()
	defer x.mu.Unlock()

	if t := x.others[id]; t != nil {
		return t, false
	}

	return &x.main, false
}

// apartThread sets up the thread of the calling goroutine, one that host
// code started.
func (x *side) apartThread() *thread {
	t := &thread{id: goroutine()}
	t.calls = t.first[:0]

	x.mu.Lock()
	defer x.mu.Unlock()

	if x.others == nil {
		x.others = map[uint64]*thread{}
	}

	x.others[t.id] = t
	x.apart.Add(1)

	return t
}

// forget drops the thread a strand set up, as the strand ends.
func (x *side) forget(t *thread) {
	x.mu.Lock()
	defer x.mu.Unlock()

	if x.others[t.id] == t {
		delete(x.others, t.id)
		x.apart.Add(-1)
	}
}

// hostReturn is where host returns to from the host code it called, which
// every goroutine in host code has on its stack.
var (
	hostReturn uintptr
	hostOnce   sync.Once
)

// hosted reports whether a call into host code is in progress on the
// calling goroutine, by looking for host among its callers. What it costs
// grows with the frames between the caller and the nearest such call.
func hosted() bool {
	hostOnce.Do(func() {
		w := &walk{}
		w.mu.Lock()
		defer w.mu.Unlock()

		w.host(func() {
			var pcs [2]uintptr

			if runtime.Callers(1, pcs[:]) == 2 {
				hostReturn = pcs[1]
			}
		})
	})

	var near [16]uintptr

	pcs := near[:]

	for {
		n := runtime.Callers(2, pcs)

		if slices.Contains(pcs[:n], hostReturn) {
			return true
		}

		if n < len(pcs) {
			return false
		}

		pcs = make([]uintptr, 16*len(pcs))
	}
}

// goroutine is the number the runtime gives the calling goroutine, read off
// the head of its stack trace, "goroutine 7 [running]:", the one place Go
// writes it; 0 where the trace begins otherwise.
func goroutine() uint64 {
	var buf [32]byte

	head := buf[:runtime.Stack(buf[:], false)]

	const prefix = "goroutine "

	if !bytes.HasPrefix(head, []byte(prefix)) {
		return 0
	}

	var id uint64

	for _, c := range head[len(prefix):] {
		if c < '0' || c > '9' {
			break
		}

		id = id*10 + uint64(c-'0')
	}

	return id
}

// enter is what a read of an option or default of s does where host code
// made it: it reports whether the read is the one to produce what state
// guards, which is the option value or inline default at span and, where
// links is set, the payload's links of the fallback chain before it.
//
// What is being produced on another goroutine is waited for, as claim
// waits. What is being produced on the reading goroutine itself, further up
// its stack, can neither be waited for nor produced a second time, so the
// read is refused at once (section 13): it answers the empty string and
// fails the host code called last there.
//
// A read that produces is the walk's, and counts nothing, where the modifier
// asked about s makes it as the host code called last on its goroutine and
// not yet returned: what it renders is nested one level below s, so
// MaxNesting bounds how many such reads stand one inside another. So does a
// read on a goroutine no call into host code is in progress on, whose stack
// holds nothing to stand inside. Any other read renders at the depth of s
// whatever host code made it, so nothing but this count bounds how many
// stand one inside another: it counts as one more resolution of the chain
// in progress until it settles, and one that would count past MaxReentry is
// refused as above.
//
// Which goroutine a read is made on is told only where the answer depends
// on it: not where the chain's host code runs on one goroutine and the
// modifier asked about s was called last, which is every read a modifier
// makes of its own options while nothing it started reads another.
func (s *slot) enter(state *settling, span [2]int, links bool) bool {
	w := s.w
	shared := w.first().side

	var t *thread

	fresh, told := false, false

	for (*state == producing || *state == counted) && !w.closed {
		if !told {
			t, fresh = shared.whose()
			told = true
		}

		if !fresh && t.producing(s, span, links) {
			t.refuse()

			return false
		}

		w.wait()
	}

	if *state != unsettled || w.closed {
		return false
	}

	*state = producing

	if !told && (shared.apart.Load() != 0 || !shared.main.asking(s)) {
		t, fresh = shared.whose()
		told = true
	}

	if told && !fresh && !t.asking(s) {
		if w.first().held.Add(1) > MaxReentry {
			w.first().held.Add(-1)
			t.refuse()

			*state = unsettled
			w.wake()

			return false
		}

		*state = counted
	}

	x := w.side
	x.on, x.made, x.fresh, x.base = t, nil, fresh, span
	x.pos, x.linking = s.open, false

	return true
}

// ended drops the thread the running strand set up, if it set one up, as
// the strand ends.
func (w *walk) ended() {
	if x := w.side; x != nil && x.made != nil {
		w.first().side.forget(x.made)
		x.on, x.made = nil, nil
	}
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
// is produced where it is first asked for and never again.
type slot struct {
	w        *walk
	open     int
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
	// counted is producing by a read that counts as a resolution in
	// progress until it settles.
	counted
	produced
)

// claim reports whether the caller is the one to produce what state
// guards. A placeholder is reached at most once (section 5), so a reader
// arriving from another goroutine while one produces it, which happens while
// the producer has the lock released around host code, waits for that one
// rather than producing it, and its placeholders' reports, a second time.
// Once the walk is closed nobody produces it, and nobody waits.
func (w *walk) claim(state *settling) bool {
	for (*state == producing || *state == counted) && !w.closed {
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
	if *state == counted {
		w.first().held.Add(-1)
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
// settled, and gives its strand back what it runs with, which the strands
// running meanwhile have replaced. Only what a read by host code produces is
// ever waited for, so the walk has a side.
func (w *walk) wait() {
	x := w.side
	on, made, fresh, base, pos, linking := x.on, x.made, x.fresh, x.base, x.pos, x.linking

	if x.settled == nil {
		x.settled = sync.NewCond(&w.mu)
	}

	x.settled.Wait()

	x.on, x.made, x.fresh, x.base, x.pos, x.linking = on, made, fresh, base, pos, linking
}

func (w *walk) wake() {
	if x := w.side; x != nil && x.settled != nil {
		x.settled.Broadcast()
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

// chain is the placeholder's fallback chain as the walk takes it, walked on
// the first reader's call.
func (s *slot) chain() string {
	return s.chainRead(false)
}

// chainRead is the placeholder's fallback chain, walked on the first
// reader's call, which is host code's where read is set. Once the walk is
// closed it is what a read produced during the call, or the empty string,
// and so is a read the walk refused, which leaves nothing for a later one
// to answer.
func (s *slot) chainRead(read bool) string {
	w := s.w

	var inline [2]int

	if s.d.hasDefault {
		inline = s.d.inline
	}

	if read && s.enter(&s.fell, inline, true) || !read && w.claim(&s.fell) {
		// What the links convert may call into host code before any modifier
		// was handed an Input, so the walk may have no side yet.
		if x := w.side; x != nil {
			x.pos, x.linking = s.open, true
		}

		text, ok := s.links()

		if x := w.side; x != nil {
			x.linking = false
		}

		if !ok && s.d.hasDefault {
			text = w.render(inline[0], inline[1], s.depth+1)
		}

		s.fallback = text
		w.settle(&s.fell)

		if read {
			w.ended()
		}
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
// during the call, or the empty string, as is a read the walk refused.
func (s *slot) optionRead(i int, read bool) string {
	if s.rendered == nil {
		s.options = make([]string, len(s.d.options))
		s.rendered = make([]settling, len(s.d.options))
	}

	span := s.d.options[i].value

	if read && s.enter(&s.rendered[i], span, false) || !read && s.w.claim(&s.rendered[i]) {
		s.options[i] = s.w.render(span[0], span[1], s.depth+1)
		s.w.settle(&s.rendered[i])

		if read {
			s.w.ended()
		}
	}

	if s.rendered[i] != produced {
		return ""
	}

	return s.options[i]
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

	if x := w.side; x != nil {
		x.pos = open
	}

	s := w.begin(open, close, depth)

	if w.closed {
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

	w.sided().asking = s

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

		if x := w.side; x != nil {
			x.pos = open
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
