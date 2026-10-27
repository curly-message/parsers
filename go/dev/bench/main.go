// Command bench measures the rows of the benchmark that ../../bench/rows.mjs
// lists, so that the family's harness, which runs on Node, measures Go code:
//
//	bench list          one JSON object per row: {"name": ..., "kind": ...}
//	bench count <row>   the row's count
//	bench serve <row>   sets the row up and writes a byte; then, for each
//	                    byte read, runs one batch of the row and writes a
//	                    byte, until its input closes
//
// A time row runs a batch of a fixed number of iterations, the same on both
// sides of a comparison and named in the row, so the round trip over the
// pipe is a constant both sides pay.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/curly-message/parsers/go/curly"
)

type row struct {
	name  string
	count func() float64
	// setup returns one iteration of a time row.
	setup func() func()
	batch int
}

func (r row) kind() string {
	if r.count != nil {
		return "count"
	}

	return "time"
}

func (r row) title() string {
	if r.count != nil {
		return r.name
	}

	return r.name + ", " + times(r.batch)
}

func times(n int) string {
	if n == 1 {
		return "once"
	}

	digits := strconv.Itoa(n)

	var b strings.Builder

	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(' ')
		}

		b.WriteRune(d)
	}

	return b.String() + " times"
}

// Messages as a catalogue holds them: interpolation, a selection, a plural,
// a placeholder an option value writes, and a formatted number and date.
var messages = []string{
	"Hello, {{name}}!",
	"{{name}} has {{count:number}} new {{count:plural; one:message; other:messages}}.",
	"{{gender; male:He; female:She; default:They}} replied to {{thread}}.",
	"{{count:plural; one:{{count}} file; few:{{count}} files; other:{{count}} files}} in {{folder; :the root; default:{{folder}}}}.",
	"Total: {{amount:number}}, due {{due:date}}.",
}

// The payload as json.Unmarshal hands it to a host, numbers as float64, but
// for the date, which a host holds as a time.Time.
var payload = map[string]any{
	"name":   "Ada",
	"count":  3.0,
	"gender": "female",
	"thread": "Plans",
	"folder": "docs",
	"amount": 1234.5,
	"due":    time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
}

// standIn answers every request at once, so that a row measures what the
// parser does around a request rather than a host's locale data.
type standIn struct{}

func (standIn) NumberFormat(_ string, _ map[string]any, input float64) (string, error) {
	return strconv.FormatFloat(input, 'f', -1, 64), nil
}

func (standIn) DateTimeFormat(_ string, _ map[string]any, input float64) (string, error) {
	return time.UnixMilli(int64(input)).UTC().Format(time.DateOnly), nil
}

func (standIn) RelativeTimeFormat(_ string, _ map[string]any, value float64, unit string) (string, error) {
	return strconv.FormatFloat(value, 'f', -1, 64) + " " + unit, nil
}

func (standIn) PluralRules(_ string, _ map[string]any, input float64) (curly.Category, error) {
	if input == 1 {
		return curly.CategoryOne, nil
	}

	return curly.CategoryOther, nil
}

var english = "en"

func catalogue(p *curly.Parser) {
	for _, m := range messages {
		p.Resolve(m, curly.Context{Payload: payload, Locale: &english})
	}
}

func nested(depth int) string {
	return strings.Repeat("{{v; a:", depth) + "x" + strings.Repeat("}}", depth)
}

func flat(count int) string {
	var b strings.Builder

	for i := range count {
		if i > 0 {
			b.WriteByte(' ')
		}

		fmt.Fprintf(&b, "{{p%d}}", i)
	}

	return b.String()
}

// shared is a slice fmt prints, since it is not plain.
type shared []any

// zeroWide holds zero-size elements before the field that tells two of it
// apart, so that comparing two reads every element, and its String method
// prints it in no bytes.
type zeroWide struct {
	A [400000]struct{}
	I int
}

func (zeroWide) String() string { return "" }

// record is a struct of the shape a host's own data takes, which
// encoding/json writes inside a plain array.
type record struct {
	Name  string
	Count int
	Tags  []string
	Inner struct{ A, B float64 }
}

// counted is a value whose String method counts its calls.
type counted struct{ calls *int }

func (c counted) String() string {
	*c.calls++

	return "v"
}

func resolving(message string, options curly.Options, context curly.Context) func() func() {
	return func() func() {
		p := curly.New(options)

		return func() { p.Resolve(message, context) }
	}
}

func valued(v any) curly.Context {
	return curly.Context{Payload: map[string]any{"v": v}}
}

var rows = []row{
	{name: "allocations: resolving five catalogue messages with a stand-in Intl", count: func() float64 {
		p := curly.New(curly.Options{Intl: standIn{}})

		return testing.AllocsPerRun(100, func() { catalogue(p) })
	}},
	{name: "String calls: one value read by 100 placeholders", count: func() float64 {
		calls := 0

		curly.New(curly.Options{}).Resolve(strings.Repeat("{{v}}", 100), valued(counted{&calls}))

		return float64(calls)
	}},
	{name: "resolve: five catalogue messages with a stand-in Intl", batch: 1000, setup: func() func() {
		p := curly.New(curly.Options{Intl: standIn{}})

		return func() { catalogue(p) }
	}},
	{name: "resolve: 5 000 placeholders", batch: 10, setup: resolving(flat(5000), curly.Options{}, curly.Context{Payload: map[string]any{}})},
	{name: "resolve: nested 10 000 levels", batch: 10, setup: resolving(nested(10000), curly.Options{}, valued("a"))},
	{name: "resolve: a placeholder of 10 000 blanks", batch: 100, setup: resolving("{{"+strings.Repeat(" ", 10000)+"}}", curly.Options{}, curly.Context{})},
	{name: "resolve: a key with 10 000 blanks inside", batch: 100, setup: resolving("{{a"+strings.Repeat(" ", 10000)+"b}}", curly.Options{}, curly.Context{})},
	{name: "resolve: 1 000 placeholders that never close", batch: 10, setup: resolving(strings.Repeat("{{a; c:", 1000), curly.Options{}, curly.Context{})},
	{name: "resolve: 5 000 options", batch: 10, setup: resolving("{{a"+strings.Repeat("; a", 5000)+"}}", curly.Options{}, curly.Context{Payload: map[string]any{"a": "a"}})},
	{name: "resolve: an option key of 50 000 characters", batch: 100, setup: resolving("{{a; "+strings.Repeat("x", 50000)+":v}}", curly.Options{}, curly.Context{Payload: map[string]any{"a": "a"}})},
	{name: "resolve: wrapper props of 5 000 entries", batch: 10, setup: func() func() {
		props := map[string]any{}

		for i := range 5000 {
			props["p"+strconv.Itoa(i)] = float64(i)
		}

		return resolving("{{v:test}}", curly.Options{
			CustomModifiers: map[string]curly.Modifier{"test": func(in curly.Input) (any, error) { return in.Value, nil }},
		}, curly.Context{
			Payload: map[string]any{"v": map[string]any{"value": "a", "props": map[string]any{"test": props}}},
			Props:   map[string]any{"test": map[string]any{"q": 1.0}},
		})()
	}},
	{name: "resolve: 100 000 nested inline defaults past the nesting limit", batch: 1, setup: resolving(strings.Repeat("{{a; default:", 100000)+"x"+strings.Repeat("}}", 100000), curly.Options{}, curly.Context{})},
	{name: "resolve: a plain object of 5 000 members", batch: 10, setup: func() func() {
		members := map[string]any{}

		for i := range 5000 {
			members["m"+strconv.Itoa(i)] = float64(i)
		}

		return resolving("{{v}}", curly.Options{}, valued(members))()
	}},
	{name: "resolve: a plain array of 5 000 structs, each written by encoding/json", batch: 10, setup: func() func() {
		records := make([]record, 5000)

		for i := range records {
			records[i] = record{Name: "r" + strconv.Itoa(i), Count: i, Tags: []string{"a", "b"}}
		}

		return resolving("{{v:eq; a:x; default:D}}", curly.Options{}, valued(records))()
	}},
	{name: "resolve: an array nested 99 999 levels", batch: 1, setup: func() func() {
		var v any = []any{}

		for range 99998 {
			v = []any{v}
		}

		return resolving("{{v}}", curly.Options{}, valued(v))()
	}},
	{name: "resolve: a struct of 1 000 fields, as fmt prints it", batch: 10, setup: func() func() {
		fields := make([]reflect.StructField, 1000)

		for i := range fields {
			fields[i] = reflect.StructField{Name: "F" + strconv.Itoa(i), Type: reflect.TypeFor[float64]()}
		}

		v := reflect.New(reflect.StructOf(fields)).Elem()

		for i := range fields {
			v.Field(i).SetFloat(float64(i))
		}

		return resolving("{{v}}", curly.Options{}, valued(v.Interface()))()
	}},
	{name: "resolve: a DAG of 25 levels, given up at the node limit", batch: 10, setup: func() func() {
		var v any = "leaf"

		for range 25 {
			v = map[string]any{"a": v, "b": v}
		}

		return resolving("{{v:eq; a:A; default:D}}", curly.Options{}, valued(v))()
	}},
	{name: "resolve: a map of 5 000 array keys, as fmt prints it", batch: 10, setup: func() func() {
		m := make(map[[2]int]int, 5000)

		for i := range 5000 {
			m[[2]int{i % 7, -i}] = i
		}

		return resolving("{{v}}", curly.Options{}, valued(m))()
	}},
	{name: "resolve: a map of 10 000 keys of 256 numbers, as fmt prints it", batch: 10, setup: func() func() {
		m := make(map[[256]int]int, 10000)

		for i := range 10000 {
			m[[256]int{i}] = i
		}

		return resolving("{{v; default:D}}", curly.Options{}, valued(m))()
	}},
	{name: "resolve: a map of 70 000 keys of 256 numbers, sorted, then refused past the text bound", batch: 1, setup: func() func() {
		m := make(map[[256]int8]int, 70000)

		for i := range 70000 {
			m[[256]int8{int8(i), int8(i >> 8), int8(i >> 16)}] = i
		}

		return resolving("{{v; default:D}}", curly.Options{}, valued(m))()
	}},
	{name: "resolve: a map of 49 keys of 400 000 zero-size elements a method prints, sorted", batch: 1, setup: func() func() {
		m := make(map[zeroWide]int, 49)

		for i := range 49 {
			m[zeroWide{I: i}] = i
		}

		return resolving("{{v; default:D}}", curly.Options{}, valued(m))()
	}},
	{name: "resolve: a map of 50 keys of 400 000 zero-size elements a method prints, refused at the count", batch: 1, setup: func() func() {
		m := make(map[zeroWide]int, 50)

		for i := range 50 {
			m[zeroWide{I: i}] = i
		}

		return resolving("{{v; default:D}}", curly.Options{}, valued(m))()
	}},
	{name: "resolve: a named slice shared over 21 levels, as fmt prints it", batch: 1, setup: func() func() {
		var v any = "x"

		for range 21 {
			v = shared{v, v}
		}

		return resolving("{{v; default:D}}", curly.Options{}, valued(v))()
	}},
	{name: "resolve: 20 000 bytes of text around one placeholder", batch: 100, setup: resolving(strings.Repeat("Lorem ipsum dolor sit amet. ", 714)+"{{v}}", curly.Options{}, valued("a"))},
	{name: "cst: five catalogue messages", batch: 1000, setup: func() func() {
		return func() {
			for _, m := range messages {
				curly.CST(m)
			}
		}
	}},
	{name: "cst: 5 000 placeholders", batch: 10, setup: func() func() {
		message := flat(5000)

		return func() { curly.CST(message) }
	}},
	{name: "cst: nested 250 levels", batch: 100, setup: func() func() {
		message := nested(250)

		return func() { curly.CST(message) }
	}},
	{name: "cst: the encoding of a tree nested 100 000 levels", batch: 1, setup: func() func() {
		tree := curly.CST(nested(100000))

		var buf []byte

		return func() { buf = tree.AppendJSON(buf[:0]) }
	}},
}

func find(name string) (row, bool) {
	for _, r := range rows {
		if r.title() == name {
			return r, true
		}
	}

	return row{}, false
}

func serve(r row) error {
	op := r.setup()
	in := bufio.NewReader(os.Stdin)
	reply := []byte{'\n'}

	if _, err := os.Stdout.Write(reply); err != nil {
		return err
	}

	for {
		if _, err := in.ReadByte(); err != nil {
			return nil
		}

		for range r.batch {
			op()
		}

		if _, err := os.Stdout.Write(reply); err != nil {
			return err
		}
	}
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "list" {
		out := json.NewEncoder(os.Stdout)

		for _, r := range rows {
			if err := out.Encode(map[string]string{"name": r.title(), "kind": r.kind()}); err != nil {
				return err
			}
		}

		return nil
	}

	if len(args) != 2 || (args[0] != "count" && args[0] != "serve") {
		return fmt.Errorf("usage: bench list | bench count <row> | bench serve <row>")
	}

	kind := "count"

	if args[0] == "serve" {
		kind = "time"
	}

	r, ok := find(args[1])

	if !ok || r.kind() != kind {
		return fmt.Errorf("no %s row is named %q", kind, args[1])
	}

	if args[0] == "count" {
		_, err := fmt.Println(r.count())

		return err
	}

	return serve(r)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
}
