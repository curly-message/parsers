package curly

import "unicode/utf8"

const hexDigits = "0123456789abcdef"

// appendJSONString writes s as encoding/json writes a string with HTML
// escaping off, byte for byte: the quote, the backslash and control
// characters escaped, a byte that starts no valid sequence as U+FFFD, and
// U+2028 and U+2029, which encoding/json escapes unconditionally for JSONP.
func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0

	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' {
				i++

				continue
			}

			dst = append(dst, s[start:i]...)

			switch b {
			case '"', '\\':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xf])
			}

			i++
			start = i

			continue
		}

		r, width := utf8.DecodeRuneInString(s[i:])

		if isInvalid(r, width) {
			dst = append(dst, s[start:i]...)
			dst = append(dst, `\ufffd`...)
			i++
			start = i

			continue
		}

		if r == '\u2028' || r == '\u2029' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hexDigits[r&0xf])
			i += width
			start = i

			continue
		}

		i += width
	}

	dst = append(dst, s[start:]...)

	return append(dst, '"')
}
