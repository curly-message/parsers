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
or array that is neither plain nor a scalar, or whose pointer has a method
`encoding/json` describes it by, is written by `encoding/json`, as it
writes that value inside the map or slice: an element of a slice through its
pointer's method, a map's value without it. The parser writes a byte that
starts no valid UTF-8 sequence as `\ufffd`, the form `encoding/json` writes
up to Go 1.26; from Go 1.27 `encoding/json` writes the character itself, so
in a value it writes that form follows the Go the host builds with. So do
the methods it describes a value by: `MarshalJSON` and `MarshalText`, and
where it is built on `encoding/json/v2`, from Go 1.27 or with
`GOEXPERIMENT=jsonv2`, `AppendText` and `MarshalJSONTo` as well. There it
names a map's key by `AppendText` too, writes a slice of a byte type that
has one of them as an array rather than in base64, and writes a map keyed by
a float, a pointer or an interface where it can write each key as a name,
and an empty map whatever its key: the parser hands those over rather than
refusing them.

The parser walks a value on a stack of its own, at any depth. A value it
cannot describe is read as missing and reported as `unserializable-value`: a
cycle, one whose text, as JSON or as fmt prints it, would be longer than
`MaxSerializedLength` bytes, a plain object or array that visits more than
`MaxConversionNodes` values, NaN or an infinity inside JSON, a value
`encoding/json` refuses or one it would reach nested deeper than 64 levels,
counted over every field it may write, and a `String`, `Error`, `Format` or
`MarshalJSON` method that panics. A value fmt prints is the host's own
conversion rather than a
serialization, so a large one that ends prints however many values it
visits, a shared one printed again at each reference; it is refused only
where its text would be too long, or where the keys of the maps it sorts
would hold too much. Before a map of two keys or more is sorted as fmt
orders them, its keys are counted, each value inside a key and each byte of
a string in one, apart from the values the conversion visits. The count is
one for the whole value, which adds each map it sorts at each reference the
map is reached through: a count past twice `MaxSerializedLength` plus two,
20 000 002, refuses the value. Keys
that print as fmt prints them reach that count only once the text is too
long; keys a `String` method prints in fewer bytes than they hold can reach
it first. The sort reads no pair of keys past what either holds, so it reads
at most the count in each of its passes, log2 of the number of keys rounded
up. Too long is the host's cap on a string, which Go does not have, so
`MaxSerializedLength` stands in for it, where JavaScript's is the longest
string V8 makes, 536 870 888 UTF-16 units; the key count bounds work fmt
does that JavaScript has no counterpart of. Both are known departures from
the format, which bounds only a serialization. A value fmt prints in more
than 10 000 000 bytes, and in fewer than JavaScript's 536 870 888 units, is
refused here and described there: with `type named []any` and `v` the string
`x` wrapped as `named{v, v}` 22 times, 16 777 213 bytes as fmt prints it,
`{{v; default:D}}` resolves to `D` and reports `unserializable-value`, where
JavaScript, given the same shape as an `Array` subclass, resolves it to the
empty string and reports `output-limit`. And a map with short text is
refused for its keys: with `type K struct{ A [1000000]struct{}; I int }`,
whose `String` method writes nothing, each key counts 1 000 003, and with
`v` a `map[K]int` of 20 keys, 74 bytes as fmt prints it, `{{v; default:D}}`
resolves to `D` and reports `unserializable-value`, where with 19 keys it
resolves to the 70 bytes fmt prints. With 12 keys, a map counts 12 000 036
and resolves to the 42 bytes fmt prints, but two of them, `named{m1, m2}`,
or one reached twice, `named{m, m}`, 87 bytes as fmt prints either, count
24 000 072, and `{{v; default:D}}` resolves to `D` and reports
`unserializable-value`. A method that
panics on a nil pointer receiver writes `<nil>`, as fmt does. A pointer that
fmt writes as an address is written as an address. Each value converts once per resolution however
many placeholders read it.

Host code the parser calls, a method of a value or the reporting handler,
must not change the payload while the resolution reads it, and must not
resolve without end: neither can be contained. Go ends the process when a
goroutine's stack outgrows its maximum, with no error a program can recover
from, and the parser sets no bound on the resolutions in progress, on a
goroutine or in the process. So a method of a value or a reporting handler
that resolves a message reaching itself again ends the process rather than
the placeholder taking its fallback chain: this port does not meet section
13's requirement to contain that failure.

### Comparisons

`eq` and `ne` compare as text, each side lowered by Unicode's full,
locale-independent case mapping, the one JavaScript's `toLowerCase` applies:
`unicode.ToLower`'s simple mapping, but for `U+0130`, which lowers to `i`
followed by `U+0307`, and a capital sigma that ends a word, which lowers to
the final sigma `U+03C2`. So `KELVIN SIGN` selects `k`, `STRASSE` does not
select the word spelled with `U+00DF`, and `U+0130` does not select `i`. The
mapping follows the Unicode version of the Go the host builds with,
`unicode.Version`: 15.0.0 on Go 1.26. A byte that starts no valid UTF-8
sequence compares as itself, and is neither cased nor case-ignorable where
it stands next to a sigma. `lt`, `gt`, `lte` and `gte` read
numbers with `strconv.ParseFloat` after trimming the format's whitespace:
`Inf`, `1e3` and `1_000` are numbers, and `0x10`, blank text and `NaN` are
not.

### Limits and reports

The limits of section 13 are `MaxOutputLength`, `MaxReadLength`,
`MaxConversionNodes`, `MaxSerializedLength` and `MaxNesting`, the text
limits counted in bytes. `MaxSerializedLength`, 10 000 000 bytes, is the
text one conversion writes, a value's own methods' text included; a value
it refuses spends none of the read budget. A
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
