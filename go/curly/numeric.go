package curly

import (
	"math"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// appendFloat writes x as ECMAScript's Number::toString writes a number,
// over the shortest digits that read back as x at its bit size: plain
// decimal from 1e-6 up to 1e21 and exponent form outside it, 0 for both
// zeros, and NaN, Infinity and -Infinity as JavaScript spells them. Go has
// no single ordinary conversion of a float and fmt's would write the
// 1000000 json.Unmarshal hands every Go host as 1e+06, so the format's
// reference spelling is the one chosen.
func appendFloat(dst []byte, x float64, bits int) []byte {
	switch {
	case math.IsNaN(x):
		return append(dst, "NaN"...)
	case math.IsInf(x, 1):
		return append(dst, "Infinity"...)
	case math.IsInf(x, -1):
		return append(dst, "-Infinity"...)
	case x == 0:
		return append(dst, '0')
	}

	if x < 0 {
		dst = append(dst, '-')
		x = -x
	}

	var scratch [32]byte

	e := strconv.AppendFloat(scratch[:0], x, 'e', -1, bits)
	mark := 0

	for e[mark] != 'e' {
		mark++
	}

	exponent, _ := strconv.Atoi(string(e[mark+1:]))
	digits := make([]byte, 0, mark)

	for _, c := range e[:mark] {
		if c != '.' {
			digits = append(digits, c)
		}
	}

	// The value is 0.digits times ten to the n, with k digits.
	k, n := len(digits), exponent+1

	switch {
	case k <= n && n <= 21:
		dst = append(dst, digits...)

		for range n - k {
			dst = append(dst, '0')
		}
	case 0 < n && n <= 21:
		dst = append(dst, digits[:n]...)
		dst = append(dst, '.')
		dst = append(dst, digits[n:]...)
	case -6 < n && n <= 0:
		dst = append(dst, '0', '.')

		for range -n {
			dst = append(dst, '0')
		}

		dst = append(dst, digits...)
	default:
		dst = append(dst, digits[0])

		if k > 1 {
			dst = append(dst, '.')
			dst = append(dst, digits[1:]...)
		}

		dst = append(dst, 'e')

		if n-1 >= 0 {
			dst = append(dst, '+')
		}

		dst = strconv.AppendInt(dst, int64(n-1), 10)
	}

	return dst
}

// trimBlank drops the format's whitespace class from both ends of s, the
// class of section 6 rather than Go's.
func trimBlank(s string) string {
	start := 0

	for start < len(s) {
		blank, width := isBlankAt(s, start)

		if !blank {
			break
		}

		start += width
	}

	end := len(s)

	for end > start {
		r, width := lastRune(s[start:end])

		if isInvalid(r, width) || !isBlankRune(r) {
			break
		}

		end -= width
	}

	return s[start:end]
}

// toNumber is the ordinary numeric conversion of text (sections 4 and 11):
// the whitespace class trimmed, then Go's own reading of a float. A result
// out of range keeps the infinity or zero it rounds to. A conversion yields
// a number only where it succeeds and its result is not NaN, since NaN is no
// number to compare: strconv reads "NaN" as one.
func toNumber(s string) (float64, bool) {
	s = trimBlank(s)

	if s == "" {
		return 0, false
	}

	x, err := strconv.ParseFloat(s, 64)

	if err != nil {
		if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
			return 0, false
		}
	}

	return x, !math.IsNaN(x)
}

// lower is Unicode's full, locale-independent lower-case mapping, the one
// String.prototype.toLowerCase applies (section 11.1): unicode.ToLower's
// simple mapping but for the two characters Unicode's SpecialCasing maps
// otherwise in every language, U+0130 to i and U+0307, and a capital sigma
// ending a word to the final sigma. A byte that starts no valid sequence is
// kept as itself: strings.ToLower would make every such byte the same
// replacement character, and two different bytes would then compare equal.
func lower(s string) string {
	i := 0

	for i < len(s) {
		c := s[i]

		if c >= utf8.RuneSelf || ('A' <= c && c <= 'Z') {
			break
		}

		i++
	}

	if i == len(s) {
		return s
	}

	out := make([]byte, i, len(s))
	copy(out, s[:i])

	for i < len(s) {
		r, width := runeAt(s, i)

		if r < utf8.RuneSelf {
			if 'A' <= r && r <= 'Z' {
				r += 'a' - 'A'
			}

			out = append(out, byte(r))
		} else if isInvalid(r, width) {
			out = append(out, s[i])
		} else if r == '\u0130' {
			out = append(out, "i\u0307"...)
		} else if r == '\u03a3' && finalSigma(s, i, i+width) {
			out = utf8.AppendRune(out, '\u03c2')
		} else {
			out = utf8.AppendRune(out, unicode.ToLower(r))
		}

		i += width
	}

	return string(out)
}

// finalSigma is Unicode's Final_Sigma condition on the character from start
// to end: a cased letter before it and none after it, each side read past
// the case-ignorable characters next to it. A character both cased and
// case-ignorable is passed over, as ICU and so String.prototype.toLowerCase
// read it. Each side reads only the run of case-ignorable characters next
// to the sigma, so a text costs what its length does, however many sigmas
// it holds.
func finalSigma(s string, start, end int) bool {
	before := start

	for before > 0 {
		r, width := utf8.DecodeLastRuneInString(s[:before])

		if !caseIgnorable(r, width) {
			if !cased(r, width) {
				return false
			}

			break
		}

		before -= width
	}

	if before == 0 {
		return false
	}

	for after := end; after < len(s); {
		r, width := utf8.DecodeRuneInString(s[after:])

		if !caseIgnorable(r, width) {
			return !cased(r, width)
		}

		after += width
	}

	return true
}

// wordBreakMid is Word_Break MidLetter, MidNumLet and Single_Quote, the
// part of Case_Ignorable the unicode package has no table for.
var wordBreakMid = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x0027, Hi: 0x0027, Stride: 1},
		{Lo: 0x002e, Hi: 0x002e, Stride: 1},
		{Lo: 0x003a, Hi: 0x003a, Stride: 1},
		{Lo: 0x00b7, Hi: 0x00b7, Stride: 1},
		{Lo: 0x0387, Hi: 0x0387, Stride: 1},
		{Lo: 0x055f, Hi: 0x055f, Stride: 1},
		{Lo: 0x05f4, Hi: 0x05f4, Stride: 1},
		{Lo: 0x2018, Hi: 0x2019, Stride: 1},
		{Lo: 0x2024, Hi: 0x2024, Stride: 1},
		{Lo: 0x2027, Hi: 0x2027, Stride: 1},
		{Lo: 0xfe13, Hi: 0xfe13, Stride: 1},
		{Lo: 0xfe52, Hi: 0xfe52, Stride: 1},
		{Lo: 0xfe55, Hi: 0xfe55, Stride: 1},
		{Lo: 0xff07, Hi: 0xff07, Stride: 1},
		{Lo: 0xff0e, Hi: 0xff0e, Stride: 1},
		{Lo: 0xff1a, Hi: 0xff1a, Stride: 1},
	},
	LatinOffset: 4,
}

// caseIgnorable is Unicode's Case_Ignorable property; a byte that starts no
// valid sequence has none.
func caseIgnorable(r rune, width int) bool {
	return !isInvalid(r, width) && unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk, wordBreakMid)
}

// cased is Unicode's Cased property: Lowercase, Uppercase or a titlecase
// letter.
func cased(r rune, width int) bool {
	return !isInvalid(r, width) && unicode.In(r, unicode.Lower, unicode.Upper, unicode.Title, unicode.Other_Lowercase, unicode.Other_Uppercase)
}
