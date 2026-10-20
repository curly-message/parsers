# curly-message for Go

The Go implementation of the
[Curly Message Format](https://github.com/curly-message/spec),
`curly-message-3`.

```sh
go get github.com/curly-message/parsers/go
```

```go
import "github.com/curly-message/parsers/go/curly"
```

## Resolving a message

```go
p := curly.New(curly.Options{})

p.Resolve("Hello, {{name; default:Guest;}}!", curly.Context{
	Payload: map[string]any{"name": "Alice"},
}) // "Hello, Alice!"
```

A `Parser` is immutable once `New` returns it and safe for concurrent use.
`Resolve` never panics on a message or a payload: a defect resolves the
placeholder it is in, and `Options.OnReport` is told about it.

The parser implements the Core level of the format: substitution, the
fallback chain, value wrappers (`Options.IgnoreWrappers` turns their
recognition off) and the comparisons `eq`, `ne`, `lt`, `gt`, `lte` and `gte`.
Every other modifier name is unknown.

### Values

Everything a message carries is text, and a payload value becomes text by its
own type:

| Value | Text |
|-------|------|
| `string` | itself |
| a `float64` or `float32` | as ECMAScript writes a number: `1000000`, `0.00001`, `1e+21`, `0` for `-0`, `NaN`, `Infinity` |
| an integer, a `bool` | in decimal, `true`, `false` |
| a plain object: an unnamed map keyed by `string` | JSON, keys sorted, HTML left unescaped |
| a plain array: an unnamed slice of anything but bytes, which `encoding/json` writes in base64 | JSON |
| `curly.Undefined{}` | nothing: it reads as a missing key, is left out of an object's JSON and is `null` in an array's |
| anything else | what `fmt.Sprint` writes: `nil` is `<nil>`, a struct `{a b}`, a `[]byte` `[104 105]` |

So a value `json.Unmarshal` decodes resolves to the text JavaScript gives the
same JSON, but for the order of an object's keys, which a Go map does not
keep. A named map or slice is not plain and prints as fmt prints it; give a
type that should read otherwise a `String` method. A value inside a plain object
or array that is neither plain nor a scalar, or whose pointer has a
`MarshalJSON` or `MarshalText` method, is written by `encoding/json`, as it
writes that value inside the map or slice: an element of a slice through its
pointer's method, a map's value without it. The parser writes a byte that
starts no valid UTF-8 sequence as `\ufffd`, the form `encoding/json` writes
up to Go 1.26; from Go 1.27 `encoding/json` writes the character itself, so
in a value it writes that form follows the Go the host builds with.

The parser walks a value on a stack of its own, at any depth. A value it
cannot describe is read as missing and reported as `unserializable-value`: a
cycle, one that visits more than `MaxConversionNodes` values, every value
inside a map's keys counted once at each reference to the map, since sorting
the keys reads each one even where a `String` method prints a key, NaN or an
infinity inside JSON, a value `encoding/json` refuses or one it would reach
nested deeper than 10 000 levels, and a `String`, `Error`, `Format` or
`MarshalJSON` method that panics. A method that panics on a nil pointer
receiver writes `<nil>`, as fmt does. A pointer that fmt writes as an address
is written as an address. Each value converts once per resolution however
many placeholders read it.

A method the parser calls must not change the payload while the resolution
reads it, and must not resolve without end: neither can be contained.

### Comparisons

`eq` and `ne` compare as text, each side lowered by Unicode's simple case
mapping, the one `unicode.ToLower` applies: `KELVIN SIGN` selects `k`, and
`STRASSE` does not select the word spelled with `U+00DF`. A byte that starts
no valid UTF-8 sequence compares as itself. `lt`, `gt`, `lte` and `gte` read
numbers with `strconv.ParseFloat` after trimming the format's whitespace:
`Inf`, `1e3` and `1_000` are numbers, and `0x10`, blank text and `NaN` are
not.

### Limits and reports

The limits of section 13 are `MaxOutputLength`, `MaxReadLength`,
`MaxConversionNodes` and `MaxNesting`, the text limits counted in bytes. A
`Report` carries the code and origin of section 14.2, an English message,
the `Context.ID`, the limit it reached and the placeholder as the message
spells it: cut to `MaxReportedLength` bytes and escaped as JSON escapes a
string, every line terminator of the format included, so it never forges a
line in a log. A panicking `OnReport` loses that report and the resolution
carries on.

## Describing a message

`curly.CST(message)` returns the concrete syntax tree of
[CST.md](https://github.com/curly-message/spec/blob/main/CST.md): the
message, its placeholders and their parts, every escape sequence. Spans are
byte offsets into the message's UTF-8 (unit `utf-8`); a byte that starts no
valid UTF-8 sequence is one character, as `range` reads it.

`Node.AppendJSON` writes the interchange encoding of CST.md section 8 at any
depth of nesting. `json.Marshal` of a `Node` writes the same JSON, but with
`<`, `>` and `&` escaped as `\u003c`, `\u003e` and `\u0026`, as encoding/json
escapes them in what any `MarshalJSON` writes; a `json.Encoder` with
`SetEscapeHTML(false)` writes `AppendJSON`'s bytes. encoding/json refuses JSON
nested past 10 000 levels, so either fails on a tree nested past about 5 000
placeholders; `AppendJSON` does not.

## Development

Run from `go/`:

| Command | What it does |
|---------|--------------|
| `go build ./...` | build |
| `go vet ./...` | vet |
| `go test ./...` | the suite |
| `gofmt -l .` | prints every file the formatting contract rejects; `gofmt -w .` fixes them |
| `go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...` | lint |
| `node dev/vectors.mjs` | rewrite `curly/testdata/floats.json`, Node's text for each float the suite checks |
| `go mod tidy -diff` | module hygiene |

Every `.go` file is ASCII: a character outside ASCII is written as an escape.

## License

[MIT](./LICENSE)
