package curly_test

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/curly-message/parsers/go/curly"
)

func Example() {
	p := curly.New(curly.Options{})

	fmt.Println(p.Resolve("Hello, {{name; default:Guest;}}!", curly.Context{
		Payload: map[string]any{"name": "Alice"},
	}))
	fmt.Println(p.Resolve("Hello, {{name; default:Guest;}}!", curly.Context{}))
	// Output:
	// Hello, Alice!
	// Hello, Guest!
}

func ExampleParser_Resolve_selection() {
	p := curly.New(curly.Options{})
	message := "{{n}} {{n; 1:file; default:files;}}, {{size:gt; 1000:large; 10:medium; default:small}}"

	fmt.Println(p.Resolve(message, curly.Context{Payload: map[string]any{"n": 1, "size": 2048}}))
	fmt.Println(p.Resolve(message, curly.Context{Payload: map[string]any{"n": 5, "size": 3}}))
	// Output:
	// 1 file, large
	// 5 files, small
}

func ExampleParser_Resolve_wrapper() {
	p := curly.New(curly.Options{})
	payload := map[string]any{
		"user":    map[string]any{"default": "a guest"},
		"default": "someone",
	}

	fmt.Println(p.Resolve("Signed in as {{user}}.", curly.Context{Payload: payload}))
	// Output:
	// Signed in as a guest.
}

func ExampleOptions_onReport() {
	p := curly.New(curly.Options{
		OnReport: func(r curly.Report) {
			fmt.Printf("%s (%s) in %s: %s\n", r.Code, r.Origin, r.ID, r.Text)
		},
	})

	fmt.Println(p.Resolve("{{n:EQ; 1:one; default:other}}", curly.Context{
		Payload: map[string]any{"n": 1},
		ID:      "greeting",
	}))
	// Output:
	// unknown-modifier (message) in greeting: {{n:EQ; 1:one; default:other}}
	// other
}

func ExampleUndefined() {
	p := curly.New(curly.Options{})

	fmt.Println(p.Resolve("{{a}}, {{b; default:none}}, {{c}}", curly.Context{
		Payload: map[string]any{
			"a": map[string]any{"x": curly.Undefined{}, "y": 1},
			"b": curly.Undefined{},
			"c": []any{curly.Undefined{}},
		},
	}))
	// Output:
	// {"y":1}, none, [null]
}

func ExampleOptions_customModifiers() {
	p := curly.New(curly.Options{
		CustomModifiers: map[string]curly.Modifier{
			// shout writes the value upper-cased, followed by the value of
			// the option keyed by its mark property.
			"shout": func(in curly.Input) (any, error) {
				for _, o := range in.Options {
					if o.Key == in.Props["mark"] {
						return strings.ToUpper(in.Value) + o.Value(), nil
					}
				}

				return in.Default(), nil
			},
		},
		ModifierDefaults: map[string]any{"shout": map[string]any{"mark": "bang"}},
	})

	const message = "{{name:shout; bang:!; ask:?; default:...}}"

	payload := map[string]any{"name": "Alice"}

	fmt.Println(p.Resolve(message, curly.Context{Payload: payload}))
	fmt.Println(p.Resolve(message, curly.Context{
		Payload: payload,
		Props:   map[string]any{"shout": map[string]any{"mark": "ask"}},
	}))
	fmt.Println(p.Resolve(message, curly.Context{
		Payload: payload,
		Props:   map[string]any{"shout": map[string]any{"mark": "none"}},
	}))
	// Output:
	// ALICE!
	// ALICE?
	// ...
}

// english stands in for a host's locale data: it knows one locale, and
// numbers and plural categories in it.
type english struct{}

func (english) NumberFormat(locale string, options map[string]any, input float64) (string, error) {
	if locale != "en" {
		return "", errors.New("no data for " + locale)
	}

	scale := 1.0

	if digits, ok := options["maximumFractionDigits"].(float64); ok {
		scale = math.Pow(10, digits)
	}

	return strconv.FormatFloat(math.Round(input*scale)/scale, 'f', -1, 64), nil
}

func (english) DateTimeFormat(string, map[string]any, float64) (string, error) {
	return "", errors.New("no dates")
}

func (english) RelativeTimeFormat(string, map[string]any, float64, string) (string, error) {
	return "", errors.New("no relative times")
}

func (english) PluralRules(locale string, options map[string]any, input float64) (curly.Category, error) {
	if locale != "en" {
		return "", errors.New("no data for " + locale)
	}

	if options["type"] == "cardinal" && input == 1 {
		return curly.CategoryOne, nil
	}

	return curly.CategoryOther, nil
}

func ExampleOptions_intl() {
	p := curly.New(curly.Options{
		Intl: english{},
		OnReport: func(r curly.Report) {
			fmt.Println(r.Code)
		},
	})

	const message = "{{n:plural; one:one file; other:{{n:number}} files}}"

	en := "en"

	fmt.Println(p.Resolve(message, curly.Context{Payload: map[string]any{"n": 1}, Locale: &en}))
	fmt.Println(p.Resolve(message, curly.Context{Payload: map[string]any{"n": 1234.5678}, Locale: &en}))
	fmt.Printf("%q\n", p.Resolve(message, curly.Context{Payload: map[string]any{"n": 1}}))
	// Output:
	// one file
	// 1234.57 files
	// missing-locale
	// ""
}
