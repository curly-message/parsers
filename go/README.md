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
recognition off) and the comparisons `eq`, `ne`, `lt`, `gt`, `lte` and `gte`;
the Extensions level: the modifiers a host registers; and, where
`Options.Intl` supplies the facility it delegates to, the Intl level:
`number`, `currency`, `date`, `ago`, `plural` and `ordinal`. Every other
modifier name is unknown.

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

### Host modifiers

```go
p := curly.New(curly.Options{
	CustomModifiers: map[string]curly.Modifier{
		"upper": func(in curly.Input) (any, error) {
			return strings.ToUpper(in.Value), nil
		},
	},
})

p.Resolve("{{name:upper}}", curly.Context{
	Payload: map[string]any{"name": "Alice"},
}) // "ALICE"
```

`Options.CustomModifiers` registers a modifier by name; one named as a
modifier of the format replaces it, so a host `eq` also answers a placeholder
that selects without naming a modifier, and over a selection nothing reports
`missing-options`. A nil entry registers nothing. A modifier is asked only
about a value: a placeholder naming no key, or a key the payload lacks, takes
its fallback chain without asking.

A modifier receives an `Input`:

- `Value`, the text the value became.
- `Options`, in the order the placeholder writes them, duplicates kept.
  `Option.Value()` renders an option on its first call; one never asked for
  is never resolved and reports nothing. `Input.Default()` is the fallback
  chain, walked on its first call.
- `Props`, the properties written under the modifier's own name, composed
  property by property from `Options.ModifierDefaults`, then
  `Context.Props`, then the `props` member of the wrapper the value came in.
  A layer's entry is read where it is a map keyed by a string type; a
  property holding `curly.Undefined{}` names nothing, and one holding `nil`
  names null. The map is the modifier's own, empty where no layer wrote.
- `Locale`, `Context.Locale` as the caller supplied it: nil where it supplied
  none, and a copy of its own otherwise.

The answer becomes text as a payload value does, and spends the output but
none of the read budget. An answer of nil or `curly.Undefined{}` is none, and
the placeholder takes its fallback chain; so it does, reporting
`failed-modifier`, where the modifier returns an error or panics, whose value
is never printed, and reporting `unserializable-value` where nothing
describes the answer.

A modifier may read its options and its default from goroutines of its own,
and after it returned. Each resolution holds a lock while parser code runs
and releases it around every call into host code, so those reads serialize
on the resolution and never deadlock on it, and a reader arriving while
another renders the same option, or converts the same value, waits for that
one: an option is rendered, and its placeholders report, once, and a value
converts once. `OnReport` is called with the lock released, on the goroutine
that met the defect, so calls of one resolution can overlap. A modifier may
resolve another message, which spends budgets of its own.

Neither can be contained, and both are the host's to avoid: a modifier that
calls `runtime.Goexit` or panics on a goroutine of its own, and one that,
through state it keeps, reads an option or default of a placeholder while
that option or default is being rendered, or a value from inside a method of
that value the parser called, which waits for itself forever.

### The Intl level

The formatting modifiers and the plural selections depend on locale data,
which this package does not carry. `Options.Intl` takes the host's facility
as an `Intl`, four requests in the vocabulary of ECMAScript's `Intl`:
`NumberFormat`, `DateTimeFormat`, `RelativeTimeFormat` and `PluralRules`.
Without one, those six names are unknown.

```go
p := curly.New(curly.Options{Intl: supplier}) // the host's curly.Intl

en := "en"

p.Resolve("{{n:plural; one:one file; other:{{n:number}} files}}", curly.Context{
	Payload: map[string]any{"n": 1234.5678},
	Locale:  &en,
}) // "1,234.57 files", where the supplier formats as Intl does in "en"
```

The parser decides everything the format decides and asks the supplier only
for the text or the category. A locale is tested first: where
`Context.Locale` is nil or empty, the placeholder resolves to the empty
string and reports `missing-locale`. The value must then be a finite number
by the conversion the comparisons use, so blank text, `12px` and `Infinity`
take the fallback chain and report `failed-modifier`. The properties are
composed as a host modifier's are, and each request carries a map of its
own:

| Modifier | Request |
|----------|---------|
| `number` | `NumberFormat` of the value, with `maximumFractionDigits` 2 where no layer names it, widened to a larger `minimumFractionDigits` a layer names |
| `currency` | `NumberFormat` of the value times the `ratio` property, which the request leaves out, with `style` `currency` over every layer |
| `date` | `DateTimeFormat` of the value read as milliseconds since the epoch, and failing that as text in one of the layouts below |
| `ago` | `RelativeTimeFormat` of the count and unit the value, a signed delta in milliseconds, climbs to, with `numeric` `auto` where no layer names it; the `format` property names the unit, and the request leaves it out |
| `plural` | an option whose key is a number equal to the value; failing that, `PluralRules` with the digit properties composed under `number`, its own over them, `number`'s default maximum and `type` `cardinal` |
| `ordinal` | the same for an integer, with its own properties alone and `type` `ordinal` |

A property is read with ECMAScript's `ToNumber` where the parser computes
with it, `minimumFractionDigits` and `ratio`: a Go number as itself, a
`bool` as 1 or 0, `nil` as 0, blank text as 0 and other text by the numeric
conversion, and anything else as NaN.

`date` reads text, Go having no layout-free reading of a date, in these
layouts: the one `time.Time`'s `String` method writes, read as it writes
it, so a `time.Time` in the payload stays a date in any zone, and the same
instant whatever zone the host runs in: its monotonic clock reading dropped,
a year of four to nine digits, signed where it is negative, and whatever
name the zone has after the offset, which decides the instant to the minute
`String` writes it in; RFC 3339, with a fraction of a second or without; and
`time.DateTime` and `time.DateOnly`, in UTC. A `time.Duration` prints as
`1h0m0s` and is no number.

A request that returns an error or panics, and a category that is none of
the six, take the fallback chain and report `failed-modifier`. The supplier's
text is the placeholder's result, emitted as it stands.

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

The suite runs the specification's conformance set and audits its runner
against the set's defect catalogue. The set is the release vendored at
`internal/conformance/set/`, checked against the sha256 its release states;
`CURLY_CONFORMANCE_FIXTURES=<dir>` runs a set under development instead, a
directory laid out as the conformance package is. The set runs at Core and
Extensions, the Intl level left out, and again at every level against a
stand-in supplier with each formatting request exposed, which measures the
request the parser makes rather than a host's locale data.
`internal/adapter/testdata/failing.txt` lists the cases of the vendored set
the parser is known to fail, and why.

## License

[MIT](./LICENSE)
