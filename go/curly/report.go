package curly

import (
	"strconv"
	"unicode/utf8"
)

// Code names a defect a resolution met, as section 14.2 spells it.
type Code string

// The eight codes of section 14.2.
const (
	UnknownModifier     Code = "unknown-modifier"
	FailedModifier      Code = "failed-modifier"
	MissingOptions      Code = "missing-options"
	UnserializableValue Code = "unserializable-value"
	MissingLocale       Code = "missing-locale"
	NestingLimit        Code = "nesting-limit"
	OutputLimit         Code = "output-limit"
	ReadLimit           Code = "read-limit"
)

// Origin is whose defect a report describes: the message's, the payload's,
// or neither, where a resolution reached one of its limits.
type Origin string

// The three origins of section 14.2.
const (
	OriginMessage Origin = "message"
	OriginPayload Origin = "payload"
	OriginLimit   Origin = "limit"
)

// Origin is the origin every report of the code carries. A code that is
// none of the eight has none.
func (c Code) Origin() Origin {
	switch c {
	case UnknownModifier, MissingOptions, NestingLimit:
		return OriginMessage
	case FailedModifier, UnserializableValue, MissingLocale:
		return OriginPayload
	case OutputLimit, ReadLimit:
		return OriginLimit
	}

	return ""
}

// Report describes a defect a resolution met (section 14.3).
type Report struct {
	Code   Code
	Origin Origin
	// Message says in English what happened; it carries nothing from the
	// payload.
	Message string
	// ID is the Context.ID of the resolution, "" where it named none.
	ID string
	// Limit is the limit a NestingLimit, OutputLimit or ReadLimit report
	// reached, and 0 on every other report.
	Limit int
	// Text is the placeholder that reported, as the message spells it: cut
	// to MaxReportedLength bytes at a character boundary, with "..." after a
	// cut, and escaped as JSON escapes a string's contents, every line
	// terminator of the format included, so that a consumer writing it to a
	// log cannot have a message forge a line there. Text is message text and
	// never a payload value.
	Text string
}

var reportMessages = map[Code]string{
	UnknownModifier:     "A placeholder named a modifier this parser does not know.",
	FailedModifier:      "A modifier could not produce a result, so the placeholder took its fallback chain.",
	MissingOptions:      "A selection was given no options to select from, so the placeholder took its fallback chain.",
	UnserializableValue: "A value could not become text, so resolution read it as missing.",
	MissingLocale:       "A modifier that depends on a locale was given none, so the placeholder resolved to the empty string.",
	NestingLimit:        "A placeholder was nested deeper than " + strconv.Itoa(MaxNesting) + " levels, so it took its fallback chain.",
	OutputLimit:         "A placeholder resolved to text this resolution has no room for, and would have carried the output past " + strconv.Itoa(MaxOutputLength) + " bytes, so it resolved to the empty string.",
	ReadLimit:           "Resolution read more than " + strconv.Itoa(MaxReadLength) + " bytes of value text, so this placeholder resolved to the empty string.",
}

func limitOf(c Code) int {
	switch c {
	case NestingLimit:
		return MaxNesting
	case OutputLimit:
		return MaxOutputLength
	case ReadLimit:
		return MaxReadLength
	}

	return 0
}

// excerpt is the text a report carries of a placeholder: cut where range
// would stop at or before MaxReportedLength, so no character is split, and
// escaped.
func excerpt(s string) string {
	if len(s) > MaxReportedLength {
		cut := 0

		for i := range s {
			if i > MaxReportedLength {
				break
			}

			cut = i
		}

		s = s[:cut] + "..."
	}

	return string(appendEscaped(nil, s))
}

// appendEscaped writes s as JSON.stringify writes a string's contents, and
// then every member of lineTerm JSON has no short escape for as \u escapes
// of its own, read off isLineTerm rather than spelled again. A byte that
// starts no valid sequence is written as the replacement character's
// escape.
func appendEscaped(dst []byte, s string) []byte {
	for i := 0; i < len(s); {
		r, width := runeAt(s, i)
		i += width

		switch {
		case r == '"' || r == '\\':
			dst = append(dst, '\\', byte(r))
		case r == '\b':
			dst = append(dst, '\\', 'b')
		case r == '\f':
			dst = append(dst, '\\', 'f')
		case r == '\n':
			dst = append(dst, '\\', 'n')
		case r == '\r':
			dst = append(dst, '\\', 'r')
		case r == '\t':
			dst = append(dst, '\\', 't')
		case r < 0x20 || isLineTerm(r) || isInvalid(r, width):
			if isInvalid(r, width) {
				r = utf8.RuneError
			}

			dst = append(dst, '\\', 'u',
				hexDigits[r>>12&0xf], hexDigits[r>>8&0xf], hexDigits[r>>4&0xf], hexDigits[r&0xf])
		default:
			dst = append(dst, s[i-width:i]...)
		}
	}

	return dst
}

func (w *walk) report(code Code, spelling string) {
	if w.p.onReport == nil {
		return
	}

	r := Report{
		Code:    code,
		Origin:  code.Origin(),
		Message: reportMessages[code],
		ID:      w.id,
		Limit:   limitOf(code),
		Text:    excerpt(spelling),
	}

	w.host(func() { w.p.onReport(r) })
}
