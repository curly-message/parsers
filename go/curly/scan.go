package curly

import "math"

// part is which part of a placeholder the scan is inside. Only a value admits
// a nested placeholder (section 6, note 10), so the scan carries the part to
// know whether a "{{" it meets opens one or refuses the construct.
type part uint8

const (
	partKey part = iota
	partModifier
	partOptionKey
	partOptionValue
)

type frame struct {
	open, index int
	part        part
}

// scanner reads one message for the placeholders it derives. A verdict is
// final (section 6, note 11): whether a placeholder derives at a position is a
// function of the message and that position alone, so an answer is recorded
// and never recomputed. That record is what keeps the scan linear in the
// length of the message, since note 7 has the scan resume one byte past a
// brace that opened nothing and the attempts overlap.
type scanner struct {
	s string
	// verdict holds close+1 for a position whose placeholder derives, -1 for
	// one that derives nothing, 0 for one not asked about, a page of
	// pageSize positions at a time. A page is made on the first position
	// settled in it, so a long text with few placeholders pays for the pages
	// its pairs open in rather than for its length; a message of one page
	// keeps it in one. wide is the same record for a message whose offsets
	// do not fit an int32.
	verdict [][]int32
	one     [1][]int32
	wide    map[int]int
	stack   []frame
	steps   int
}

const (
	pageBits = 10
	pageSize = 1 << pageBits
)

func newScanner(s string) *scanner {
	return &scanner{s: s}
}

func (sc *scanner) known(open int) (int, bool) {
	if sc.wide != nil {
		v, ok := sc.wide[open]

		return v, ok
	}

	if sc.verdict == nil {
		return 0, false
	}

	page := sc.verdict[open>>pageBits]

	if page == nil {
		return 0, false
	}

	switch v := page[open&(pageSize-1)]; {
	case v == 0:
		return 0, false
	case v < 0:
		return -1, true
	default:
		return int(v) - 1, true
	}
}

func (sc *scanner) settle(open, close int) {
	if sc.wide == nil && sc.verdict == nil {
		switch {
		case len(sc.s) >= math.MaxInt32:
			sc.wide = map[int]int{}
		case len(sc.s) <= pageSize:
			sc.one[0] = make([]int32, len(sc.s))
			sc.verdict = sc.one[:]
		default:
			sc.verdict = make([][]int32, (len(sc.s)+pageSize-1)>>pageBits)
		}
	}

	if sc.wide != nil {
		sc.wide[open] = close

		return
	}

	page := sc.verdict[open>>pageBits]

	if page == nil {
		page = make([]int32, pageSize)
		sc.verdict[open>>pageBits] = page
	}

	if close < 0 {
		page[open&(pageSize-1)] = -1
	} else {
		page[open&(pageSize-1)] = int32(close + 1)
	}
}

// end is where a complete placeholder opening at open ends. It walks an
// explicit stack rather than recursing: nesting is not otherwise limited
// (section 6, note 10), and a message nested a hundred thousand levels deep
// must not take the goroutine's stack down with it.
func (sc *scanner) end(open int) (int, bool) {
	if v, ok := sc.known(open); ok {
		return v, v >= 0
	}

	s := sc.s
	stack := append(sc.stack[:0], frame{open: open, index: open + 2, part: partKey})

	// Every construct still open when the scan dies is one whose closing pair
	// never arrived, so each of them derives nothing.
	refuse := func() (int, bool) {
		for _, f := range stack {
			sc.settle(f.open, -1)
		}

		sc.stack = stack

		return 0, false
	}

	for len(stack) > 0 {
		sc.steps++
		f := &stack[len(stack)-1]

		if f.index >= len(s) {
			return refuse()
		}

		c := s[f.index]

		switch {
		case c == '\\':
			if f.index+1 < len(s) && isLineTermAt(s, f.index+1) {
				return refuse()
			}

			f.index = escapeEnd(s, f.index, len(s))
		case isLineTermAt(s, f.index):
			return refuse()
		case c == ';':
			f.part = partOptionKey
			f.index++
		case c == ':':
			// The first colon of a part is its separator and a later one is
			// content (section 6, notes 3 and 4), so a part only moves on.
			switch f.part {
			case partKey:
				f.part = partModifier
			case partOptionKey:
				f.part = partOptionValue
			}

			f.index++
		case c == '{' && f.index+1 < len(s) && s[f.index+1] == '{':
			// A "{{" in a value must open a complete placeholder or the
			// construct around it does not derive at all; anywhere else it
			// opens none, so the construct around it does not derive either.
			if f.part != partOptionValue {
				return refuse()
			}

			if v, ok := sc.known(f.index); ok {
				if v < 0 {
					return refuse()
				}

				f.index = v

				continue
			}

			stack = append(stack, frame{open: f.index, index: f.index + 2, part: partKey})
		case c == '}' && f.index+1 < len(s) && s[f.index+1] == '}':
			close := f.index + 2

			sc.settle(f.open, close)
			stack = stack[:len(stack)-1]

			if len(stack) == 0 {
				sc.stack = stack

				return close, true
			}

			stack[len(stack)-1].index = close
		default:
			f.index++
		}
	}

	sc.stack = stack

	return 0, false
}

// next finds the next placeholder opening at or after from, closing by to.
// The braces a refused construct opened are text, and the search resumes at
// the very next byte rather than past the pair (section 6, note 7).
func (sc *scanner) next(from, to int) (int, int, bool) {
	s := sc.s

	for i := from; i < to; i++ {
		sc.steps++

		switch s[i] {
		case '\\':
			i = escapeEnd(s, i, to) - 1
		case '{':
			if i+1 >= to || s[i+1] != '{' {
				continue
			}

			if close, ok := sc.end(i); ok {
				return i, close, true
			}
		}
	}

	return 0, 0, false
}

// trimmed is the span between from and to with its blank padding dropped,
// empty at from where the span is padding throughout. Whitespace an escape
// sequence claims is text, not padding. Only the padding is read, from either
// end: an option value is a span that holds every placeholder nested in it,
// so reading the whole span at each level would read a deep message as many
// times over as it nests.
func trimmed(s string, from, to int) (int, int) {
	start := from

	for start < to {
		blank, width := isBlankAt(s[:to], start)

		if !blank {
			break
		}

		start += width
	}

	if start == to {
		return from, from
	}

	end := to

	for end > start {
		r, width := lastRune(s[start:end])

		if isInvalid(r, width) || !isBlankRune(r) {
			break
		}

		// A blank is text where a backslash consumes it, which is where an
		// odd run of backslashes inside the span precedes it.
		run := 0

		for end-width-1-run >= from && s[end-width-1-run] == '\\' {
			run++
		}

		if run%2 == 1 {
			break
		}

		end -= width
	}

	return start, end
}

// separated divides the span between from and to at every sep no escape
// sequence claims and, where the scanner is given, no nested placeholder
// writes (section 6, note 5). There is always at least one span.
func separated(s string, from, to int, sep byte, sc *scanner) [][2]int {
	var parts [][2]int

	start := from

	for i := from; i < to; i++ {
		switch c := s[i]; {
		case c == '\\':
			i = escapeEnd(s, i, to) - 1
		case sc != nil && c == '{' && i+1 < to && s[i+1] == '{':
			if close, ok := sc.end(i); ok && close <= to {
				i = close - 1
			}
		case c == sep:
			parts = append(parts, [2]int{start, i})
			start = i + 1
		}
	}

	return append(parts, [2]int{start, to})
}
