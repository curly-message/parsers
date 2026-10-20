package curly

import (
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Intl is the host's internationalization facility, which the modifiers of
// the Intl level delegate to (sections 11.2 and 11.5). The parser decides
// everything the format decides: the locale test, what a number and a date
// are, how the layers compose, the defaults and pins, the unit and count of
// a relative time, and which option a category selects. What it asks is a
// request in the vocabulary of ECMAScript's Intl, the options a fresh map
// the supplier may keep; the supplier performs it in its locale data.
//
// A method that returns an error or panics is a request the host cannot
// make, and the placeholder takes its fallback chain, reporting
// FailedModifier; so does a PluralRules answer that is none of the six
// categories.
type Intl interface {
	// NumberFormat formats input as Intl.NumberFormat(locale,
	// options).format(input) does; number and currency ask it.
	NumberFormat(locale string, options map[string]any, input float64) (string, error)
	// DateTimeFormat formats input, milliseconds since the Unix epoch, as
	// Intl.DateTimeFormat(locale, options).format(input) does.
	DateTimeFormat(locale string, options map[string]any, input float64) (string, error)
	// RelativeTimeFormat formats a count of unit, one of second, minute,
	// hour, day, week, month and year, as Intl.RelativeTimeFormat(locale,
	// options).format(value, unit) does.
	RelativeTimeFormat(locale string, options map[string]any, value float64, unit string) (string, error)
	// PluralRules is the category Intl.PluralRules(locale,
	// options).select(input) puts input in.
	PluralRules(locale string, options map[string]any, input float64) (Category, error)
}

// Category is a plural category of the Unicode CLDR plural rules.
type Category string

// The six categories a PluralRules request answers with.
const (
	CategoryZero  Category = "zero"
	CategoryOne   Category = "one"
	CategoryTwo   Category = "two"
	CategoryFew   Category = "few"
	CategoryMany  Category = "many"
	CategoryOther Category = "other"
)

// intlBuiltin is the modifier of the Intl level under name, which a parser
// knows only where the host supplied the facility it delegates to.
func intlBuiltin(name string) (run func(*slot) (string, Code), selection, known bool) {
	switch name {
	case "number":
		return number, false, true
	case "currency":
		return currency, false, true
	case "date":
		return date, false, true
	case "ago":
		return ago, false, true
	case "plural":
		return plural, true, true
	case "ordinal":
		return ordinal, true, true
	}

	return nil, false, false
}

// locale is the caller's locale where one is available: a locale supplied
// empty is none (section 11.2).
func (s *slot) locale() (string, bool) {
	if l := s.w.locale; l != nil && *l != "" {
		return *l, true
	}

	return "", false
}

// formattable is the number text is, where a formatting modifier can format
// it: the numeric conversion, which reads blank text as none, yielding
// something finite.
func formattable(text string) (float64, bool) {
	x, ok := toNumber(text)

	return x, ok && !math.IsInf(x, 0)
}

// numberOf is a property's value as ECMAScript's ToNumber reads it, for the
// properties the parser itself computes with: a number as itself, a boolean
// as 1 or 0, nil as 0, text by the numeric conversion with blank text 0, and
// anything else NaN.
func numberOf(v any) float64 {
	if v == nil {
		return 0
	}

	rv := reflect.ValueOf(v)

	switch rv.Kind() {
	case reflect.Bool:
		if rv.Bool() {
			return 1
		}

		return 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	case reflect.String:
		if trimBlank(rv.String()) == "" {
			return 0
		}

		if x, ok := toNumber(rv.String()); ok {
			return x
		}
	}

	return math.NaN()
}

// widened applies number's default maximum of two fraction digits where no
// layer named one, widened to reach a larger minimum a layer named: a
// default, not a cap (section 11.2).
func widened(props map[string]any) map[string]any {
	if _, named := props["maximumFractionDigits"]; !named {
		maximum := 2.0

		if minimum, named := props["minimumFractionDigits"]; named {
			maximum = max(maximum, numberOf(minimum))

			if math.IsNaN(maximum) {
				maximum = 2
			}
		}

		props["maximumFractionDigits"] = maximum
	}

	return props
}

// ask makes a formatting request of the host's facility, contained.
func (s *slot) ask(request func(Intl) (string, error)) (string, Code) {
	var text string

	var err error

	if !s.w.host(func() { text, err = request(s.w.p.intl) }) || err != nil {
		return "", FailedModifier
	}

	return text, ""
}

func number(s *slot) (string, Code) {
	locale, ok := s.locale()

	if !ok {
		return "", MissingLocale
	}

	n, ok := formattable(s.value)

	if !ok {
		return "", FailedModifier
	}

	props := widened(s.w.composed("number", s.wrapper))

	return s.ask(func(intl Intl) (string, error) { return intl.NumberFormat(locale, props, n) })
}

// currency formats the value times its ratio in the currency style, which
// is what the modifier is and so is pinned over every layer. The ratio is
// the format's own property and the product already reflects it, so the
// request carries none.
func currency(s *slot) (string, Code) {
	locale, ok := s.locale()

	if !ok {
		return "", MissingLocale
	}

	n, ok := formattable(s.value)

	if !ok {
		return "", FailedModifier
	}

	props := s.w.composed("currency", s.wrapper)

	if ratio, named := props["ratio"]; named {
		n *= numberOf(ratio)
		delete(props, "ratio")
	}

	if math.IsNaN(n) || math.IsInf(n, 0) {
		return "", FailedModifier
	}

	props["style"] = "currency"

	return s.ask(func(intl Intl) (string, error) { return intl.NumberFormat(locale, props, n) })
}

// date reads its value as a timestamp first, and only text that is no
// number as a date (section 11.2).
func date(s *slot) (string, Code) {
	locale, ok := s.locale()

	if !ok {
		return "", MissingLocale
	}

	ms, ok := formattable(s.value)

	if !ok {
		if ms, ok = parseDate(s.value); !ok {
			return "", FailedModifier
		}
	}

	props := s.w.composed("date", s.wrapper)

	return s.ask(func(intl Intl) (string, error) { return intl.DateTimeFormat(locale, props, ms) })
}

// The layouts a date's text is read in, Go having no layout-free reading of
// a date, after the form time.Time's String method writes, so a time.Time
// in the payload stays a date: RFC 3339, whose reading takes a fraction of a
// second written or not, then a date and time and a date alone, in UTC.
var dateLayouts = [...]string{
	time.RFC3339,
	time.DateTime,
	time.DateOnly,
}

func parseDate(text string) (float64, bool) {
	// time.Time's String method writes a monotonic clock reading after the
	// date, which is no part of it.
	if i := strings.LastIndex(text, " m="); i >= 0 && i+3 < len(text) && (text[i+3] == '+' || text[i+3] == '-') {
		text = text[:i]
	}

	if ms, ok := parseTimeString(text); ok {
		return ms, true
	}

	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, text); err == nil {
			return float64(t.UnixMilli()), true
		}
	}

	return 0, false
}

// parseTimeString reads the form time.Time's String method writes,
// 2006-01-02 15:04:05.999999999 -0700 MST, as that method writes it: a year
// of four digits or more, signed where it is negative, and the zone's name
// as it stands, whatever it is, since the offset before it decides the
// instant. It reads every field itself and counts the instant from the
// date in UTC: time.Parse reads a year of four digits only, a name only
// where it looks like an abbreviation, and an offset the host's own zone
// has at that instant as that zone, whose rules then move the instant along
// with any arithmetic on it.
func parseTimeString(text string) (float64, bool) {
	start := 0

	if strings.HasPrefix(text, "-") {
		start = 1
	}

	end := start

	for end < len(text) && isDigit(text[end]) {
		end++
	}

	// Nine digits at most keep the year in an int on every platform; a year
	// past them is past what a date can format anyway.
	if digits := end - start; digits < 4 || digits > 9 {
		return 0, false
	}

	year, _ := strconv.Atoi(text[:end])
	rest := text[end:]

	if len(rest) < 15 || rest[0] != '-' || rest[3] != '-' || rest[6] != ' ' || rest[9] != ':' || rest[12] != ':' {
		return 0, false
	}

	month, okMonth := twoDigits(rest[1:])
	day, okDay := twoDigits(rest[4:])
	hour, okHour := twoDigits(rest[7:])
	minute, okMinute := twoDigits(rest[10:])
	second, okSecond := twoDigits(rest[13:])

	if !okMonth || !okDay || !okHour || !okMinute || !okSecond || month < 1 || month > 12 || day < 1 || hour > 23 || minute > 59 || second > 59 {
		return 0, false
	}

	rest = rest[15:]
	nanosecond := 0

	if strings.HasPrefix(rest, ".") {
		n := 1

		for n < len(rest) && n <= 9 && isDigit(rest[n]) {
			nanosecond = nanosecond*10 + int(rest[n]-'0')
			n++
		}

		if n == 1 {
			return 0, false
		}

		for range 10 - n {
			nanosecond *= 10
		}

		rest = rest[n:]
	}

	// The offset, then the zone's name, which is never empty.
	if len(rest) < 8 || rest[0] != ' ' || (rest[1] != '+' && rest[1] != '-') || rest[6] != ' ' {
		return 0, false
	}

	hours, okHours := twoDigits(rest[2:])
	minutes, okMinutes := twoDigits(rest[4:])

	if !okHours || !okMinutes || minutes > 59 {
		return 0, false
	}

	offset := time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute

	if rest[1] == '-' {
		offset = -offset
	}

	t := time.Date(year, time.Month(month), day, hour, minute, second, nanosecond, time.UTC)

	// time.Date moves a day the month lacks into the next month.
	if t.Day() != day {
		return 0, false
	}

	t = t.Add(-offset)

	return float64(t.Unix())*1000 + float64(t.Nanosecond()/1e6), true
}

func isDigit(b byte) bool {
	return '0' <= b && b <= '9'
}

func twoDigits(s string) (int, bool) {
	if !isDigit(s[0]) || !isDigit(s[1]) {
		return 0, false
	}

	return int(s[0]-'0')*10 + int(s[1]-'0'), true
}

// The units ago counts in, each a fixed multiple of the one below it.
var ladder = [...]struct {
	unit     string
	multiple float64
}{
	{"second", 1000},
	{"minute", 60},
	{"hour", 60},
	{"day", 24},
	{"week", 7},
	{"month", 13.0 / 3},
	{"year", 12},
}

// agoUnit is the unit a format property names, singular or plural, or auto.
// The names are compared exactly, and a format naming none is a property
// the modifier cannot process.
func agoUnit(format any) (string, bool) {
	rv := reflect.ValueOf(format)

	if !rv.IsValid() || rv.Kind() != reflect.String {
		return "", false
	}

	f := rv.String()

	if f == "auto" {
		return f, true
	}

	for _, rung := range ladder {
		if f == rung.unit || f == rung.unit+"s" {
			return rung.unit, true
		}
	}

	return "", false
}

// climb is the count and unit of a delta of ms milliseconds: seconds first,
// then each unit above taken while the count rounds to one or more, or up
// to the unit named, rounding at every step (section 11.2). math.Round takes
// a half away from zero and keeps the sign of a zero, so a delta and its
// negation count alike.
func climb(ms float64, named string) (float64, string) {
	count, unit := math.Round(ms/ladder[0].multiple), ladder[0].unit

	for _, rung := range ladder[1:] {
		if unit == named {
			break
		}

		next := math.Round(count / rung.multiple)

		if named == "auto" && math.Abs(next) < 1 {
			break
		}

		count, unit = next, rung.unit
	}

	return count, unit
}

// ago formats a signed delta from now, the unit chosen on its magnitude.
// The format property is the format's own, and the count and unit already
// reflect it, so the request carries none; its numeric phrasing is auto
// where no layer names one.
func ago(s *slot) (string, Code) {
	locale, ok := s.locale()

	if !ok {
		return "", MissingLocale
	}

	ms, ok := formattable(s.value)

	if !ok {
		return "", FailedModifier
	}

	props := s.w.composed("ago", s.wrapper)

	if _, named := props["numeric"]; !named {
		props["numeric"] = "auto"
	}

	named := "auto"

	if format, ok := props["format"]; ok {
		if named, ok = agoUnit(format); !ok {
			return "", FailedModifier
		}

		delete(props, "format")
	}

	count, unit := climb(ms, named)

	return s.ask(func(intl Intl) (string, error) { return intl.RelativeTimeFormat(locale, props, count, unit) })
}

func plural(s *slot) (string, Code) {
	return byCategory(s, "cardinal")
}

func ordinal(s *slot) (string, Code) {
	return byCategory(s, "ordinal")
}

// byCategory selects the first option whose key is a number equal to the
// value, and where none is, the first whose key is the category the host's
// rules put the value in (section 11.5). plural takes the category of the
// number as number would show it, so it reads number's digit properties
// beneath its own; the rule type is what each modifier is, pinned over every
// layer.
func byCategory(s *slot, rules string) (string, Code) {
	locale, ok := s.locale()

	if !ok {
		return "", MissingLocale
	}

	n, ok := formattable(s.value)

	// An ordinal of a fraction names no position.
	if !ok || rules == "ordinal" && n != math.Trunc(n) {
		return "", FailedModifier
	}

	for i, o := range s.d.options {
		if key, ok := formattable(o.key); ok && key == n {
			return s.option(i), ""
		}
	}

	var props map[string]any

	if rules == "cardinal" {
		props = s.w.shownDigits(s.wrapper)

		for k, v := range s.w.composed("plural", s.wrapper) {
			props[k] = v
		}

		widened(props)
	} else {
		props = s.w.composed("ordinal", s.wrapper)
	}

	props["type"] = rules

	var category Category

	var err error

	if !s.w.host(func() { category, err = s.w.p.intl.PluralRules(locale, props, n) }) || err != nil {
		return "", FailedModifier
	}

	switch category {
	case CategoryZero, CategoryOne, CategoryTwo, CategoryFew, CategoryMany, CategoryOther:
	default:
		return "", FailedModifier
	}

	for i, o := range s.d.options {
		if o.key == string(category) {
			return s.option(i), ""
		}
	}

	return s.chain(), ""
}
