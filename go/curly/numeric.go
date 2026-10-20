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

// lower is the host's ordinary, locale-independent lower-case conversion,
// rune by rune, with a byte that starts no valid sequence kept as itself:
// strings.ToLower would make every such byte the same replacement
// character, and two different bytes would then compare equal.
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
		} else {
			out = utf8.AppendRune(out, unicode.ToLower(r))
		}

		i += width
	}

	return string(out)
}
