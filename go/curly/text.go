package curly

import "unicode/utf8"

// lineTerm is the format's line-term production (section 6, note 1): the
// four code points a placeholder holds in no position. The scanner, the
// whitespace class and the escaping of a report's text all read the set from
// here, so an amendment moves one list rather than three.
var lineTerm = [...]rune{'\n', '\r', '\u2028', '\u2029'}

// whitespace is the rest of the class section 6 enumerates; the class is
// these and lineTerm.
var whitespace = [...]rune{
	'\t', '\v', '\f', ' ', '\u00a0', '\u1680',
	'\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006',
	'\u2007', '\u2008', '\u2009', '\u200a',
	'\u202f', '\u205f', '\u3000', '\ufeff',
}

// The ASCII members of both sets, so that the scan reads a byte without
// decoding it; filled from the lists above rather than spelled again.
var asciiTerm, asciiBlank [utf8.RuneSelf]bool

func init() {
	for _, r := range lineTerm {
		if r < utf8.RuneSelf {
			asciiTerm[r] = true
			asciiBlank[r] = true
		}
	}

	for _, r := range whitespace {
		if r < utf8.RuneSelf {
			asciiBlank[r] = true
		}
	}
}

func isLineTerm(r rune) bool {
	for _, t := range lineTerm {
		if r == t {
			return true
		}
	}

	return false
}

// isBlankRune reports whether r is in the format's whitespace class, which is
// neither Go's nor a Unicode property (section 6, note 8).
func isBlankRune(r rune) bool {
	if r < utf8.RuneSelf {
		return r >= 0 && asciiBlank[r]
	}

	if isLineTerm(r) {
		return true
	}

	for _, w := range whitespace {
		if r == w {
			return true
		}
	}

	return false
}

// runeAt reads the character at byte offset i the way range reads a string:
// a byte that starts no valid sequence is one character standing for itself,
// answered as utf8.RuneError of width 1.
func runeAt(s string, i int) (rune, int) {
	if b := s[i]; b < utf8.RuneSelf {
		return rune(b), 1
	}

	return utf8.DecodeRuneInString(s[i:])
}

// lastRune is runeAt read backwards from the end of s.
func lastRune(s string) (rune, int) {
	if b := s[len(s)-1]; b < utf8.RuneSelf {
		return rune(b), 1
	}

	return utf8.DecodeLastRuneInString(s)
}

// isInvalid tells a byte that starts no valid sequence from a written U+FFFD.
func isInvalid(r rune, width int) bool {
	return r == utf8.RuneError && width == 1
}

func isLineTermAt(s string, i int) bool {
	if b := s[i]; b < utf8.RuneSelf {
		return asciiTerm[b]
	}

	r, width := utf8.DecodeRuneInString(s[i:])

	return !isInvalid(r, width) && isLineTerm(r)
}

func isBlankAt(s string, i int) (bool, int) {
	if b := s[i]; b < utf8.RuneSelf {
		return asciiBlank[b], 1
	}

	r, width := utf8.DecodeRuneInString(s[i:])

	return !isInvalid(r, width) && isBlankRune(r), width
}

// cancels reports whether a backslash in front of the character cancels a
// structural meaning rather than denoting itself (section 7).
func cancels(r rune, width int) bool {
	switch r {
	case ':', ';', '{', '}', '\\':
		return true
	}

	return !isInvalid(r, width) && isBlankRune(r)
}

// escapeEnd is where the escape sequence opening at i ends: a backslash
// consumes the character after it, a whole code point, and a backslash with
// nothing left before to to consume ends one past itself.
func escapeEnd(s string, i, to int) int {
	if i+1 >= to {
		return i + 2
	}

	_, width := runeAt(s[:to], i+1)

	return i + 1 + width
}

// unesc removes the escape sequences of section 7 from message text: a
// sequence that cancels a structural meaning leaves the character alone, any
// other leaves both characters. Text holding no backslash is returned as it is.
func unesc(s string) string {
	first := -1

	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			first = i

			break
		}
	}

	if first < 0 {
		return s
	}

	out := make([]byte, 0, len(s))
	out = append(out, s[:first]...)

	for i := first; i < len(s); {
		if s[i] != '\\' || i+1 >= len(s) {
			out = append(out, s[i])
			i++

			continue
		}

		r, width := runeAt(s, i+1)

		if !cancels(r, width) {
			out = append(out, '\\')
		}

		out = append(out, s[i+1:i+1+width]...)
		i += 1 + width
	}

	return string(out)
}
