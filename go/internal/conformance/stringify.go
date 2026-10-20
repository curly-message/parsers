package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// stringifyBudget bounds what stringify visits: a value that shares what it
// holds is written again at every reference, and can spell more than any
// comparison reads.
const stringifyBudget = 1 << 21

// stringify writes a value as ECMAScript's JSON.stringify writes the value
// JSON decodes to, with an object's keys sorted: a Go map has no order of
// its own, and two values are compared by their entries. A number is
// written as Number::toString writes it, so 1 reads 1 and -0 reads 0, and
// text is escaped as JSON.stringify escapes it, leaving / and everything
// outside ASCII as it is. A value of another type is written as
// encoding/json writes it. ok is false where JSON cannot describe the
// value.
func stringify(value any) (string, bool) {
	type task struct {
		literal string
		value   any
		written bool
		leaves  container
	}

	var out strings.Builder
	stack := []task{{value: value}}
	// open holds the containers being written, so a value that holds itself
	// is one JSON cannot describe, as JSON.stringify refuses it.
	open := map[container]bool{}
	enter := func(value any) (container, bool) {
		v := reflect.ValueOf(value)
		at := container{v.Pointer(), v.Len()}

		if open[at] {
			return at, false
		}

		open[at] = true

		return at, true
	}

	for visited := 0; len(stack) > 0; visited++ {
		if visited > stringifyBudget {
			return "", false
		}

		next := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if next.written {
			out.WriteString(next.literal)
			delete(open, next.leaves)

			continue
		}

		switch v := next.value.(type) {
		case nil:
			out.WriteString("null")
		case bool:
			out.WriteString(strconv.FormatBool(v))
		case string:
			quote(&out, v)
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				out.WriteString("null")
			} else {
				out.WriteString(esNumber(v))
			}
		case int:
			out.WriteString(strconv.Itoa(v))
		case []any:
			if len(v) == 0 {
				out.WriteString("[]")

				continue
			}

			at, ok := enter(v)

			if !ok {
				return "", false
			}

			stack = append(stack, task{literal: "]", written: true, leaves: at})

			for i := len(v) - 1; i >= 0; i-- {
				stack = append(stack, task{value: v[i]})

				if i > 0 {
					stack = append(stack, task{literal: ",", written: true})
				}
			}

			out.WriteByte('[')
		case map[string]any:
			at, ok := enter(v)

			if !ok {
				return "", false
			}

			keys := sortedKeys(v)
			stack = append(stack, task{literal: "}", written: true, leaves: at})

			for i := len(keys) - 1; i >= 0; i-- {
				var key strings.Builder

				if i > 0 {
					key.WriteByte(',')
				}

				quote(&key, keys[i])
				key.WriteByte(':')
				stack = append(stack, task{value: v[keys[i]]}, task{literal: key.String(), written: true})
			}

			out.WriteByte('{')
		default:
			var buf bytes.Buffer
			encoder := json.NewEncoder(&buf)
			encoder.SetEscapeHTML(false)

			if encoder.Encode(v) != nil {
				return "", false
			}

			out.Write(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
		}
	}

	return out.String(), true
}

// container identifies a map or a slice being written: a slice by its
// elements and its length, since two slices of one array are two values.
type container struct {
	at     uintptr
	length int
}

// quote writes text as JSON.stringify quotes it.
func quote(out *strings.Builder, s string) {
	out.WriteByte('"')

	for _, r := range s {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(out, `\u%04x`, r)
			} else {
				out.WriteRune(r)
			}
		}
	}

	out.WriteByte('"')
}

// esNumber writes a finite number as ECMAScript's Number::toString does.
func esNumber(f float64) string {
	if f == 0 {
		return "0"
	}

	sign := ""

	if f < 0 {
		sign, f = "-", -f
	}

	mantissa, exponent, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, 64), "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	e, _ := strconv.Atoi(exponent)
	k, n := len(digits), e+1

	switch {
	case k <= n && n <= 21:
		return sign + digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return sign + digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return sign + "0." + strings.Repeat("0", -n) + digits
	}

	written := digits[:1]

	if k > 1 {
		written += "." + digits[1:]
	}

	if n-1 < 0 {
		return sign + written + "e-" + strconv.Itoa(1-n)
	}

	return sign + written + "e+" + strconv.Itoa(n-1)
}

// shownLength bounds what a failure line quotes of a value: an output can
// be as long as the output limit, and a value can be derived from the
// payload.
const shownLength = 200

// shown is a value as a failure line quotes it.
func shown(value any) string {
	s, ok := stringify(value)

	if !ok {
		s = fmt.Sprintf("a %T JSON cannot describe", value)
	}

	if utf8.RuneCountInString(s) <= shownLength {
		return s
	}

	runes := []rune(s)

	return string(runes[:shownLength]) + "..."
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
