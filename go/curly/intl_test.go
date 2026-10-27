package curly_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/curly-message/parsers/go/curly"
)

// supplier is an Intl that records each request as one line and answers it
// with a text of its own, or with its category.
type supplier struct {
	requests []string
	category curly.Category
	err      error
	panics   bool
}

func (s *supplier) answer(api, locale string, options map[string]any, input ...any) error {
	line := []string{api, locale, fmt.Sprint(options)}

	for _, in := range input {
		line = append(line, fmt.Sprint(in))
	}

	s.requests = append(s.requests, strings.Join(line, " "))

	if s.panics {
		panic("password=hunter2")
	}

	return s.err
}

func (s *supplier) NumberFormat(locale string, options map[string]any, input float64) (string, error) {
	return "<number>", s.answer("NumberFormat", locale, options, input)
}

func (s *supplier) DateTimeFormat(locale string, options map[string]any, input float64) (string, error) {
	return "<date>", s.answer("DateTimeFormat", locale, options, input)
}

func (s *supplier) RelativeTimeFormat(locale string, options map[string]any, value float64, unit string) (string, error) {
	return "<ago>", s.answer("RelativeTimeFormat", locale, options, value, unit)
}

func (s *supplier) PluralRules(locale string, options map[string]any, input float64) (curly.Category, error) {
	return s.category, s.answer("PluralRules", locale, options, input)
}

type intlCase struct {
	name     string
	message  string
	payload  map[string]any
	props    map[string]any
	out      string
	requests []string
	codes    []curly.Code
}

func runIntl(t *testing.T, defaults map[string]any, s *supplier, locale *string, cases []intlCase) {
	t.Helper()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s.requests = nil

			var codes []curly.Code

			p := curly.New(curly.Options{Intl: s, ModifierDefaults: defaults, OnReport: func(r curly.Report) { codes = append(codes, r.Code) }})
			out := p.Resolve(c.message, curly.Context{Payload: c.payload, Props: c.props, Locale: locale})

			if out != c.out || fmt.Sprint(codes) != fmt.Sprint(c.codes) || strings.Join(s.requests, "\n") != strings.Join(c.requests, "\n") {
				t.Errorf("got %q %v\n%s\nwant %q %v\n%s", out, codes, strings.Join(s.requests, "\n"), c.out, c.codes, strings.Join(c.requests, "\n"))
			}
		})
	}
}

func TestIntlIsTheSuppliers(t *testing.T) {
	payload := map[string]any{"n": 1.5}

	for _, name := range []string{"number", "currency", "date", "ago", "plural", "ordinal"} {
		got, _ := resolveWith(curly.Options{}, "{{n:"+name+"; one:ONE; default:D}}", payload)
		expect(t, name+" without a supplier", got, "D", curly.UnknownModifier)
	}

	got := host(map[string]curly.Modifier{"number": upper}, "{{n:number}}", map[string]any{"n": "x"})
	expect(t, "a host's own number", got, "X")

	got, _ = resolveWith(curly.Options{Intl: &supplier{}, CustomModifiers: map[string]curly.Modifier{"plural": upper}}, "{{n:plural}}", map[string]any{"n": "x"})
	expect(t, "a host's own plural over a supplier's", got, "X")
}

func TestIntlLocale(t *testing.T) {
	cases := []intlCase{
		{"number", "[{{n:number; default:D}}]", map[string]any{"n": 10.0}, nil, "[]", nil, []curly.Code{curly.MissingLocale}},
		{"no link stands in", "[{{n:number}}]", map[string]any{"n": map[string]any{"value": 10.0, "default": "W"}, "default": "P"}, nil, "[]", nil, []curly.Code{curly.MissingLocale}},
		{"before the value", "[{{n:date; default:D}}]", map[string]any{"n": " "}, nil, "[]", nil, []curly.Code{curly.MissingLocale}},
		{"plural", "[{{n:plural; 1:ONE; default:D}}]", map[string]any{"n": 1.0}, nil, "[]", nil, []curly.Code{curly.MissingLocale}},
		{"near miss: an absent value takes the chain", "[{{n:number; default:D}}]", nil, nil, "[D]", nil, nil},
		{"near miss: a selection with no options", "[{{n:ordinal; default:D}}]", map[string]any{"n": 1.0}, nil, "[D]", nil, []curly.Code{curly.MissingOptions}},
	}

	runIntl(t, nil, &supplier{}, nil, cases)
	runIntl(t, nil, &supplier{}, ptr(""), cases)
	runIntl(t, nil, &supplier{}, ptr(" "), []intlCase{
		{"near miss: blank is a locale", "{{n:number}}", map[string]any{"n": 1.0}, nil, "<number>", []string{"NumberFormat   map[maximumFractionDigits:2] 1"}, nil},
	})
}

func TestIntlNumber(t *testing.T) {
	failed := []curly.Code{curly.FailedModifier}

	runIntl(t, map[string]any{"number": map[string]any{"useGrouping": false, "maximumFractionDigits": 4.0}}, &supplier{}, ptr("cs"), []intlCase{
		{"layers per property", "{{n:number}}", map[string]any{"n": map[string]any{"value": 1.5, "props": map[string]any{"number": map[string]any{"maximumFractionDigits": 1.0}}}}, map[string]any{"number": map[string]any{"useGrouping": true}}, "<number>",
			[]string{"NumberFormat cs map[maximumFractionDigits:1 useGrouping:true] 1.5"}, nil},
		{"a default maximum", "{{n:number}}", map[string]any{"n": 1.5}, nil, "<number>", []string{"NumberFormat cs map[maximumFractionDigits:4 useGrouping:false] 1.5"}, nil},
	})

	runIntl(t, nil, &supplier{}, ptr("en"), []intlCase{
		{"two digits at most", "{{n:number}}", map[string]any{"n": 1234.5678}, nil, "<number>", []string{"NumberFormat en map[maximumFractionDigits:2] 1234.5678"}, nil},
		{"text", "{{n:number}}", map[string]any{"n": " +1e3 "}, nil, "<number>", []string{"NumberFormat en map[maximumFractionDigits:2] 1000"}, nil},
		{"widened to a minimum", "{{n:number}}", map[string]any{"n": 1.0}, map[string]any{"number": map[string]any{"minimumFractionDigits": "4"}}, "<number>", []string{"NumberFormat en map[maximumFractionDigits:4 minimumFractionDigits:4] 1"}, nil},
		{"near miss: a minimum under two", "{{n:number}}", map[string]any{"n": 1.0}, map[string]any{"number": map[string]any{"minimumFractionDigits": true}}, "<number>", []string{"NumberFormat en map[maximumFractionDigits:2 minimumFractionDigits:true] 1"}, nil},
		{"near miss: no number", "{{n:number}}", map[string]any{"n": 1.0}, map[string]any{"number": map[string]any{"minimumFractionDigits": []any{5.0}}}, "<number>", []string{"NumberFormat en map[maximumFractionDigits:2 minimumFractionDigits:[5]] 1"}, nil},
		{"a named maximum decides", "{{n:number}}", map[string]any{"n": 1.0}, map[string]any{"number": map[string]any{"minimumFractionDigits": 4, "maximumFractionDigits": nil}}, "<number>", []string{"NumberFormat en map[maximumFractionDigits:<nil> minimumFractionDigits:4] 1"}, nil},
		{"Undefined names nothing", "{{n:number}}", map[string]any{"n": 1.0}, map[string]any{"number": map[string]any{"maximumFractionDigits": curly.Undefined{}}}, "<number>", []string{"NumberFormat en map[maximumFractionDigits:2] 1"}, nil},
		{"a segment is no property", "{{n:number; maximumFractionDigits:5}}", map[string]any{"n": 1.0}, nil, "<number>", []string{"NumberFormat en map[maximumFractionDigits:2] 1"}, nil},
		{"the answer is data", "{{n:number}}{{n:number}}", map[string]any{"n": 1.0}, nil, "<number><number>", []string{"NumberFormat en map[maximumFractionDigits:2] 1", "NumberFormat en map[maximumFractionDigits:2] 1"}, nil},
		{"blank", "{{n:number; default:D}}", map[string]any{"n": " \t"}, nil, "D", nil, failed},
		{"a leading part", "{{n:number; default:D}}", map[string]any{"n": "12px"}, nil, "D", nil, failed},
		{"an infinity", "{{n:number; default:D}}", map[string]any{"n": math.Inf(1)}, nil, "D", nil, failed},
		{"NaN", "{{n:number; default:D}}", map[string]any{"n": "NaN"}, nil, "D", nil, failed},
	})

	for what, s := range map[string]*supplier{"an error": {err: errors.New("no")}, "a panic": {panics: true}} {
		var reports []curly.Report

		p := curly.New(curly.Options{Intl: s, OnReport: func(r curly.Report) { reports = append(reports, r) }})
		out := p.Resolve("[{{n:number; default:D}}]", curly.Context{Payload: map[string]any{"n": 1.0}, Locale: ptr("en")})

		if out != "[D]" || len(reports) != 1 || reports[0].Code != curly.FailedModifier || strings.Contains(fmt.Sprintf("%+v", reports), "hunter2") {
			t.Errorf("%s: got %q %+v", what, out, reports)
		}
	}
}

func TestIntlCurrency(t *testing.T) {
	failed := []curly.Code{curly.FailedModifier}

	runIntl(t, map[string]any{"currency": map[string]any{"currency": "EUR", "style": "percent"}}, &supplier{}, ptr("de"), []intlCase{
		{"the style is pinned", "{{n:currency}}", map[string]any{"n": map[string]any{"value": 2.0, "props": map[string]any{"currency": map[string]any{"style": "decimal"}}}}, nil, "<number>",
			[]string{"NumberFormat de map[currency:EUR style:currency] 2"}, nil},
		{"times the ratio, which the request leaves out", "{{n:currency}}", map[string]any{"n": "2"}, map[string]any{"currency": map[string]any{"ratio": 100}}, "<number>", []string{"NumberFormat de map[currency:EUR style:currency] 200"}, nil},
		{"a ratio as text", "{{n:currency}}", map[string]any{"n": 2.0}, map[string]any{"currency": map[string]any{"ratio": " 0.5 "}}, "<number>", []string{"NumberFormat de map[currency:EUR style:currency] 1"}, nil},
		{"a null ratio", "{{n:currency}}", map[string]any{"n": 2.0}, map[string]any{"currency": map[string]any{"ratio": nil}}, "<number>", []string{"NumberFormat de map[currency:EUR style:currency] 0"}, nil},
		{"no default maximum", "{{n:currency}}", map[string]any{"n": 2.0}, map[string]any{"currency": map[string]any{"currency": curly.Undefined{}}}, "<number>", []string{"NumberFormat de map[currency:EUR style:currency] 2"}, nil},
		{"a ratio no number", "{{n:currency; default:D}}", map[string]any{"n": 2.0}, map[string]any{"currency": map[string]any{"ratio": "x"}}, "D", nil, failed},
		{"a product overflowing", "{{n:currency; default:D}}", map[string]any{"n": 1e308}, map[string]any{"currency": map[string]any{"ratio": 10}}, "D", nil, failed},
		{"near miss: a product at the edge", "{{n:currency}}", map[string]any{"n": 1e307}, map[string]any{"currency": map[string]any{"ratio": 10}}, "<number>", []string{"NumberFormat de map[currency:EUR style:currency] 1e+308"}, nil},
		{"a value no number", "{{n:currency; default:D}}", map[string]any{"n": "two"}, nil, "D", nil, failed},
	})
}

type stamp struct{ t time.Time }

func (s stamp) String() string { return s.t.Format(time.RFC3339Nano) }

func decodedTime(t *testing.T, text string) time.Time {
	t.Helper()

	var at time.Time

	if err := json.Unmarshal([]byte(text), &at); err != nil {
		t.Fatal(err)
	}

	return at
}

func TestIntlDate(t *testing.T) {
	now := time.Now()
	at := time.Date(2024, 3, 5, 10, 0, 0, 0, time.FixedZone("CET", 3600))
	want := func(t time.Time) []string {
		return []string{fmt.Sprint("DateTimeFormat en map[timeZone:UTC] ", float64(t.UnixMilli()))}
	}

	props := map[string]any{"date": map[string]any{"timeZone": "UTC"}}

	runIntl(t, nil, &supplier{}, ptr("en"), []intlCase{
		{"a timestamp", "{{d:date}}", map[string]any{"d": 1709632800000.0}, props, "<date>", []string{"DateTimeFormat en map[timeZone:UTC] 1.7096328e+12"}, nil},
		{"digits are a timestamp before a year", "{{d:date}}", map[string]any{"d": "2024"}, props, "<date>", []string{"DateTimeFormat en map[timeZone:UTC] 2024"}, nil},
		{"and so is a signed number", "{{d:date}}", map[string]any{"d": " +1000 "}, props, "<date>", []string{"DateTimeFormat en map[timeZone:UTC] 1000"}, nil},
		{"a time.Time", "{{d:date}}", map[string]any{"d": at}, props, "<date>", want(at), nil},
		{"one read off the clock", "{{d:date}}", map[string]any{"d": now}, props, "<date>", want(now), nil},
		{"one in a zone with no name", "{{d:date}}", map[string]any{"d": at.In(time.FixedZone("", 3600))}, props, "<date>", want(at), nil},
		{"one in a zone named by its offset", "{{d:date}}", map[string]any{"d": at.In(time.FixedZone("+0545", 20700))}, props, "<date>", want(at), nil},
		{"one west of Greenwich", "{{d:date}}", map[string]any{"d": at.In(time.FixedZone("", -34200))}, props, "<date>", want(at), nil},
		{"one decoded from JSON", "{{d:date}}", map[string]any{"d": decodedTime(t, `"2024-03-05T10:00:00+01:00"`)}, props, "<date>", want(at), nil},
		{"one in a year past 9999", "{{d:date}}", map[string]any{"d": time.Date(12000, 2, 29, 10, 0, 0, 5e6, time.UTC)}, props, "<date>", want(time.Date(12000, 2, 29, 10, 0, 0, 5e6, time.UTC)), nil},
		{"one before the year 1", "{{d:date}}", map[string]any{"d": time.Date(-1, 3, 1, 10, 0, 0, 5e6, time.UTC)}, props, "<date>", want(time.Date(-1, 3, 1, 10, 0, 0, 5e6, time.UTC)), nil},
		{"near miss: no zone after the offset", "{{d:date; default:D}}", map[string]any{"d": "2024-03-05 10:00:00 +0100"}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"near miss: a blank zone after the offset", "{{d:date; default:D}}", map[string]any{"d": "2024-03-05 10:00:00 +0100 "}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"near miss: a year of three digits", "{{d:date; default:D}}", map[string]any{"d": "999-03-05 10:00:00 +0100 CET"}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"near miss: a day a year past 9999 lacks", "{{d:date; default:D}}", map[string]any{"d": "12001-02-29 10:00:00 +0000 UTC"}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"near miss: a day the month lacks", "{{d:date; default:D}}", map[string]any{"d": "2024-04-31 10:00:00 +0100 CET"}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"near miss: an hour past the day", "{{d:date; default:D}}", map[string]any{"d": "2024-03-05 24:00:00 +0100 CET"}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"near miss: a point with no fraction", "{{d:date; default:D}}", map[string]any{"d": "2024-03-05 10:00:00. +0100 CET"}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"near miss: a fraction past the nanosecond", "{{d:date; default:D}}", map[string]any{"d": "2024-03-05 10:00:00.0000000001 +0100 CET"}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"near miss: an offset of sixty minutes", "{{d:date; default:D}}", map[string]any{"d": "2024-03-05 10:00:00 +0060 CET"}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"an offset to the minute and a fraction to the nanosecond", "{{d:date}}", map[string]any{"d": "2024-03-05 10:00:00.123456789 +0545 NPT"}, props, "<date>", want(time.Date(2024, 3, 5, 4, 15, 0, 123456789, time.UTC)), nil},
		{"RFC 3339", "{{d:date}}", map[string]any{"d": stamp{now}}, props, "<date>", want(now), nil},
		{"RFC 3339 to the second", "{{d:date}}", map[string]any{"d": "2024-03-05T10:00:00+01:00"}, props, "<date>", want(at), nil},
		{"a date and time in UTC", "{{d:date}}", map[string]any{"d": "2024-03-05 09:00:00"}, props, "<date>", want(at), nil},
		{"a date in UTC", "{{d:date}}", map[string]any{"d": "2024-03-05"}, props, "<date>", want(time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC)), nil},
		{"near miss: a layout nobody documented", "{{d:date; default:D}}", map[string]any{"d": "March 5, 2024"}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"near miss: a clock reading alone", "{{d:date; default:D}}", map[string]any{"d": " m=+0.1"}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"blank", "{{d:date; default:D}}", map[string]any{"d": ""}, props, "D", nil, []curly.Code{curly.FailedModifier}},
		{"an infinity", "{{d:date; default:D}}", map[string]any{"d": "-Infinity"}, props, "D", nil, []curly.Code{curly.FailedModifier}},
	})

	// time.Time's String method writes a zone's name as it stands, after the
	// offset, which decides the instant whatever the name.
	var named []intlCase

	for _, name := range []string{"UTC+1", "Z", "utc", "Europe/Prague", "+01:00", "UTC+01:00", "MESZ", "+0200", "a zone"} {
		named = append(named, intlCase{"one in a zone named " + name, "{{d:date}}", map[string]any{"d": at.In(time.FixedZone(name, 3600))}, props, "<date>", want(at), nil})
	}

	runIntl(t, nil, &supplier{}, ptr("en"), named)
}

// A time.Time's text names its offset, and the instant it names is the same
// whatever zone the host runs in, one with offsets of its own included.
func TestIntlDateInTheHostsZone(t *testing.T) {
	zone := func(name string) *time.Location {
		loc, err := time.LoadLocation(name)

		if err != nil {
			t.Fatal(err)
		}

		return loc
	}

	props := map[string]any{"date": map[string]any{"timeZone": "UTC"}}
	want := func(t time.Time) []string {
		return []string{fmt.Sprint("DateTimeFormat en map[timeZone:UTC] ", float64(t.UnixMilli()))}
	}

	hosts := []struct {
		zone  string
		times []time.Time
	}{
		{"America/New_York", []time.Time{
			time.Date(2024, 3, 20, 10, 0, 0, 0, zone("America/Chicago")),
			time.Date(2024, 10, 30, 10, 0, 0, 0, zone("America/Bogota")),
			time.Date(2024, 11, 3, 6, 30, 0, 0, time.UTC).In(zone("America/New_York")),
		}},
		{"Europe/London", []time.Time{
			time.Unix(0, 0).UTC(),
			{},
			time.Date(2024, 3, 28, 10, 0, 0, 0, zone("Europe/Paris")),
		}},
		{"Europe/Prague", []time.Time{
			time.Date(1975, 7, 1, 12, 0, 0, 0, time.FixedZone("", 7200)),
		}},
	}

	local := time.Local

	defer func() { time.Local = local }()

	for _, host := range hosts {
		time.Local = zone(host.zone)

		var cases []intlCase

		for _, at := range host.times {
			cases = append(cases, intlCase{host.zone + " reads " + at.String(), "{{d:date}}", map[string]any{"d": at}, props, "<date>", want(at), nil})
		}

		runIntl(t, nil, &supplier{}, ptr("en"), cases)
	}
}

func TestIntlAgo(t *testing.T) {
	const (
		second = 1000.0
		minute = 60 * second
		hour   = 60 * minute
		day    = 24 * hour
	)

	climbs := []struct {
		delta  float64
		count  string
		unit   string
		format any
	}{
		{0, "0", "second", nil},
		{499, "0", "second", nil},
		{500, "1", "second", nil},
		{-100, "-0", "second", nil},
		{29 * second, "29", "second", nil},
		{30 * second, "1", "minute", nil},
		{-30 * second, "-1", "minute", nil},
		{30 * minute, "1", "hour", nil},
		{-30 * minute, "-1", "hour", nil},
		{90 * minute, "2", "hour", nil},
		{-90 * minute, "-2", "hour", nil},
		{12 * hour, "1", "day", nil},
		{3.5 * day, "1", "week", nil},
		{26 * day, "1", "month", nil},
		{6 * 30 * day, "1", "year", nil},
		{1e15, "31797", "year", nil},
		{3 * hour, "0", "day", "day"},
		{800 * day, "6.912e+07", "second", "second"},
		{800 * day, "2", "year", "years"},
		{2 * hour, "2", "hour", "auto"},
	}

	for _, c := range climbs {
		props := map[string]any{"ago": map[string]any{}}

		if c.format != nil {
			props["ago"] = map[string]any{"format": c.format}
		}

		count := c.count

		if count == "-0" {
			count = fmt.Sprint(math.Copysign(0, -1))
		}

		runIntl(t, nil, &supplier{}, ptr("en"), []intlCase{
			{fmt.Sprint(c.delta, " ", c.format), "{{d:ago}}", map[string]any{"d": c.delta}, props, "<ago>", []string{"RelativeTimeFormat en map[numeric:auto] " + count + " " + c.unit}, nil},
		})
	}

	failed := []curly.Code{curly.FailedModifier}

	runIntl(t, map[string]any{"ago": map[string]any{"numeric": "always", "style": "short"}}, &supplier{}, ptr("en"), []intlCase{
		{"a layer names numeric", "{{d:ago}}", map[string]any{"d": -day}, nil, "<ago>", []string{"RelativeTimeFormat en map[numeric:always style:short] -1 day"}, nil},
		{"a null numeric is named", "{{d:ago}}", map[string]any{"d": -day}, map[string]any{"ago": map[string]any{"numeric": nil}}, "<ago>", []string{"RelativeTimeFormat en map[numeric:<nil> style:short] -1 day"}, nil},
		{"a unit compared exactly", "{{d:ago; default:D}}", map[string]any{"d": day}, map[string]any{"ago": map[string]any{"format": "YEAR"}}, "D", nil, failed},
		{"a unit off the ladder", "{{d:ago; default:D}}", map[string]any{"d": day}, map[string]any{"ago": map[string]any{"format": "quarter"}}, "D", nil, failed},
		{"a null format", "{{d:ago; default:D}}", map[string]any{"d": day}, map[string]any{"ago": map[string]any{"format": nil}}, "D", nil, failed},
		{"near miss: an Undefined format is auto", "{{d:ago}}", map[string]any{"d": day}, map[string]any{"ago": map[string]any{"format": curly.Undefined{}}}, "<ago>", []string{"RelativeTimeFormat en map[numeric:always style:short] 1 day"}, nil},
		{"a value no number", "{{d:ago; default:D}}", map[string]any{"d": "soon"}, nil, "D", nil, failed},
	})
}

func TestIntlPlural(t *testing.T) {
	const message = "{{n:plural; one:ONE; other:OTHER; 0:ZERO; 2.0:TWO; 2:SECOND; default:D}}"

	s := &supplier{category: curly.CategoryOne}

	runIntl(t, nil, s, ptr("cs"), []intlCase{
		{"a number wins wherever written", message, map[string]any{"n": 0.0}, nil, "ZERO", nil, nil},
		{"compared numerically, the first written", message, map[string]any{"n": "2"}, nil, "TWO", nil, nil},
		{"near miss: not as shown", message, map[string]any{"n": 1.999}, nil, "ONE", []string{"PluralRules cs map[maximumFractionDigits:2 type:cardinal] 1.999"}, nil},
		{"near miss: a key no number never selects", "{{n:plural; Infinity:I; 1,0:C; one:ONE}}", map[string]any{"n": 1.0}, nil, "ONE", []string{"PluralRules cs map[maximumFractionDigits:2 type:cardinal] 1"}, nil},
		{"near miss: a category compared exactly", "{{n:plural; One:ONE; default:D}}", map[string]any{"n": 1.0}, nil, "D", []string{"PluralRules cs map[maximumFractionDigits:2 type:cardinal] 1"}, nil},
		{"a value no number", message, map[string]any{"n": "abc"}, nil, "D", nil, []curly.Code{curly.FailedModifier}},
	})

	defaults := map[string]any{"number": map[string]any{"minimumFractionDigits": 1, "useGrouping": false, "style": "percent"}}

	runIntl(t, defaults, s, ptr("cs"), []intlCase{
		{"number's digits beneath its own", "{{n:plural; one:ONE}}", map[string]any{"n": map[string]any{"value": 1.0, "props": map[string]any{"number": map[string]any{"roundingMode": "floor"}}}},
			map[string]any{"plural": map[string]any{"minimumFractionDigits": 3, "type": "ordinal"}, "number": map[string]any{"trailingZeroDisplay": "auto"}}, "ONE",
			[]string{"PluralRules cs map[maximumFractionDigits:3 minimumFractionDigits:3 roundingMode:floor trailingZeroDisplay:auto type:cardinal] 1"}, nil},
	})

	for _, category := range []curly.Category{curly.CategoryOther, "", "One", "dual"} {
		s := &supplier{category: category}
		want, codes := "D", []curly.Code{curly.FailedModifier}

		if category == curly.CategoryOther {
			want, codes = "OTHER", nil
		}

		runIntl(t, nil, s, ptr("en"), []intlCase{
			{"category " + string(category), message, map[string]any{"n": 5.0}, nil, want, []string{"PluralRules en map[maximumFractionDigits:2 type:cardinal] 5"}, codes},
		})
	}

	runIntl(t, nil, &supplier{category: curly.CategoryFew}, ptr("en"), []intlCase{
		{"a category with no option", message, map[string]any{"n": 5.0}, nil, "D", []string{"PluralRules en map[maximumFractionDigits:2 type:cardinal] 5"}, nil},
	})
	runIntl(t, nil, &supplier{err: errors.New("no")}, ptr("en"), []intlCase{
		{"refused", message, map[string]any{"n": 5.0}, nil, "D", []string{"PluralRules en map[maximumFractionDigits:2 type:cardinal] 5"}, []curly.Code{curly.FailedModifier}},
	})
}

func TestIntlOrdinal(t *testing.T) {
	const message = "{{n:ordinal; one:st; two:nd; few:rd; other:th; 11:th!; default:D}}"

	defaults := map[string]any{"number": map[string]any{"minimumFractionDigits": 1}, "plural": map[string]any{"minimumIntegerDigits": 2}, "ordinal": map[string]any{"type": "cardinal", "x": 1}}

	runIntl(t, defaults, &supplier{category: curly.CategoryTwo}, ptr("en"), []intlCase{
		{"its own layers alone", message, map[string]any{"n": 22.0}, nil, "nd", []string{"PluralRules en map[type:ordinal x:1] 22"}, nil},
		{"a number wins", message, map[string]any{"n": "11"}, nil, "th!", nil, nil},
		{"an integer only", message, map[string]any{"n": 1.5}, nil, "D", nil, []curly.Code{curly.FailedModifier}},
		{"near miss: a large integer", message, map[string]any{"n": 1e300}, nil, "nd", []string{"PluralRules en map[type:ordinal x:1] 1e+300"}, nil},
	})
}

// Each request carries a map of its own, so a supplier that writes to one
// reaches no other.
func TestIntlRequestsAreTheSuppliers(t *testing.T) {
	s := &writing{}
	p := curly.New(curly.Options{Intl: s, ModifierDefaults: map[string]any{"number": map[string]any{"a": 1}}})
	context := curly.Context{Payload: map[string]any{"n": 1.0}, Locale: ptr("en"), Props: map[string]any{"number": map[string]any{"b": 2}}}

	if got := p.Resolve("{{n:number}}{{n:number}}", context); got != "map[a:1 b:2 maximumFractionDigits:2]map[a:1 b:2 maximumFractionDigits:2]" {
		t.Errorf("got %q", got)
	}

	if got := p.Resolve("{{n:number}}", context); got != "map[a:1 b:2 maximumFractionDigits:2]" {
		t.Errorf("again: got %q", got)
	}
}

type writing struct{ supplier }

func (w *writing) NumberFormat(locale string, options map[string]any, input float64) (string, error) {
	text := fmt.Sprint(options)
	options["written"] = true

	return text, nil
}
