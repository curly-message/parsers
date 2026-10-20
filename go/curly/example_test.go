package curly_test

import (
	"fmt"

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
