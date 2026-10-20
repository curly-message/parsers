// Command size resolves one message and prints it: the program whose
// stripped binary the benchmark's size rows weigh, so that they follow what
// the parser adds to a host's binary.
package main

import (
	"os"

	"github.com/curly-message/parsers/go/curly"
)

func main() {
	p := curly.New(curly.Options{})

	os.Stdout.WriteString(p.Resolve("Hello, {{name; default:Guest;}}!\n", curly.Context{
		Payload: map[string]any{"name": "Ada"},
	}))
}
