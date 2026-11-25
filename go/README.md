# curly-message for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/curly-message/parsers/go.svg)](https://pkg.go.dev/github.com/curly-message/parsers/go)
[![tests](https://github.com/curly-message/parsers/actions/workflows/tests-go.yml/badge.svg?branch=main)](https://github.com/curly-message/parsers/actions/workflows/tests-go.yml)

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
| a `float64` | as ECMAScript writes a number: `1000000`, `0.00001`, `1e+21`, `0` for `-0`, `NaN`, `Infinity` |
| a `float32` | in the same form, by the fewest digits that read back as that `float32`: `float32(0.1)` is `0.1` |
| an integer, a `bool` | in decimal, `true`, `false` |
| a plain object: an unnamed map keyed by `string` | JSON, keys sorted, HTML left unescaped |
| a plain array: an unnamed slice of anything but bytes, which `encoding/json` writes in base64 | JSON |
| `curly.Undefined{}` | nothing: it reads as a missing key, though a map whose `value` holds it is still a wrapper, one with no value, where `{}` is a plain object; it is left out of an object's JSON and is `null` in an array's |
| anything else | what `fmt.Sprint` writes: `nil` is `<nil>`, a struct `{a b}`, a `[]byte` `[104 105]` |

So a value `json.Unmarshal` decodes resolves to the text JavaScript gives the
same JSON, but for four things. An object's keys are sorted, since a Go map
keeps no order. A null read as the value itself, whether a payload entry or
a wrapper's `value` or `default`, is `nil`, which reads `<nil>` where
JavaScript writes `null`; a null inside an object or array that is written
as JSON is `null` in both. U+2028 and U+2029 inside an object or array are
escaped, as `encoding/json` escapes them, where JavaScript writes them as
they are. And `json.Unmarshal` decodes an escaped lone surrogate, such as `"\ud800"`, to
U+FFFD. A named map or slice is not plain and prints as fmt prints it; give
a type that should read otherwise a `String` method. A value inside a plain object
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

A method the parser calls must not change the payload while the resolution
reads it: that cannot be contained.

A value that becomes text by resolving a message, a lazily translated one
say, implements `curly.Texter`:

```go
type Lazy struct{ Key string }

func (l Lazy) CurlyText(r curly.Resolver) string {
	text, err := r.Resolve(catalogue[l.Key], curly.Context{Payload: payload})
	if err != nil {
		return ""
	}

	return text
}
```

`CurlyText` is tried before `Error` and `String`, wherever those are: for
the value itself, behind a pointer, and for an operand fmt would print
within a struct. Inside a slice or a map a value is serialized as JSON, and
there neither `CurlyText` nor `String` is called: JSON has no text
conversion to hand a `Resolver` to, and a value that must be text in a list
is converted by the host before it enters the payload. The `Resolver` asks
for a resolution counted with the one converting the value (see Host
modifiers), so a value whose message names it again ends in a fallback
chain at `MaxReentry`. `Resolver.With(parser)` resolves with another parser,
counted the same.

Go ends the process when a goroutine's stack outgrows its maximum, with no
error a program can recover from, and names no goroutine a count could be
kept for. So the parser counts along a lineage, as section 13 has it on such
a host: `Parser.Resolve` begins one and is never refused, and every piece of
host code the parser calls is handed a `Resolver` that continues it — a
modifier with its `Input`, `CurlyText`, `OnReport`, and an `Intl` supplier
that implements `curly.IntlResolving`. A method that calls `Parser.Resolve`
where it could ask its `Resolver`, a plain `String` that resolves among
them, is counted by nothing, and where it resolves the message that names
it, it ends the process: section 14.2 leaves that recursion to the host
code that makes it.

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
  names null. The map is nil where no layer holds anything under the name,
  and reads as an empty one. The layers every placeholder shares, the
  defaults and `Context.Props`, are composed once in a resolution. A
  wrapper's layer is composed with them once for each run of the modifier's
  placeholders whose wrappers hold that one layer, and a resolution keeps
  only the one composed last, so the compositions it holds never grow with
  the wrappers of its payload. Every placeholder a composition serves is handed the same
  map: a modifier reads it and never writes to it. Go cannot make a map
  read-only, so a write reaches every placeholder sharing the map, and one
  into the nil map panics, which fails the modifier as any panic does. The
  layers, as the payload, do not change while the resolution reads them.
  Nothing composed outlives its resolution.
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
on the resolution and never deadlock on it. The walk, arriving while a read
renders an option or converts a value, waits for that one: an option is
rendered, and its placeholders report, once, and a value converts once. A
read never waits (below). `OnReport` is called with the lock released, on
the goroutine that met the defect, so calls of one resolution can overlap.

A modifier may resolve another message, which spends budgets of its own.
`Input.Resolver()` is the means to ask: its `Resolve(message, context)`
resolves it with the parser that called the modifier, as one more
resolution of its lineage: the resolution `Parser.Resolve` started and
every one asked for through a `Resolver` handed out in one of them,
whichever goroutine asks and whether or not that one has returned. The
modifier can hand it to code it calls. One that would count past
`MaxReentry`, 1000, resolves
nothing and returns `curly.ErrReentryLimit`. The modifier handles the
refusal as any error; returned as its own, it fails the modifier, whose
placeholder reports `failed-modifier` and takes its fallback chain, and
every resolution around it answers. So a message whose modifiers resolve it
again through their `Input` ends in a fallback chain by the 1000th
resolution rather than ending the process. A `Resolver` kept after
everything it was handed out in has returned still counts on that lineage,
so 1001 goroutines sharing one see a refusal, and it keeps the resolution
it was handed out in, its payload among it, from being collected.

The walk of a resolution counts nothing, and neither does a modifier
reading the options and the default of its own placeholder while it is
current, the one its lineage called last of those that have not returned,
and nothing of that placeholder is being rendered: what such a read renders
is nested one level below the placeholder, so `MaxNesting` such reads stand
one inside another for each resolution or counted read of the lineage. Any
other read, `Option.Value` or `Input.Default` kept from one call and read in
another, in a method of a value, or on a goroutine while another modifier of
the lineage is current, renders a body from outside the one being rendered,
and counts as one more resolution of the lineage in progress until it has
rendered. One that would count past `MaxReentry` is refused: it renders
nothing, answers the empty string, leaves the option or default for a later
read to render and, as section 13 has it, fails nothing. Section 13 lets an
implementation count an own read as well while another placeholder's option
or default, whose rendering began beneath the modifier's call, is being
rendered; this port does not, so a modifier that reads an option it kept
from another placeholder, whose rendering reads the modifier's own option,
counts the first read and not the second.

Goroutines of one modifier are in one lineage, so their reads count
together, with every counted read beneath them. A modifier reading its
options from n goroutines at once, each read standing in k counted reads,
its own and one for each modifier nested in its option that reads its own
option beneath it, is answered for every one where n times k is at most
`MaxReentry` - 1: up to 499 goroutines where each option nests one such
modifier, and 249 where it nests three. Past that, how many are answered
depends on the schedule.

A read never waits, since Go names no goroutine and the parser cannot tell
a read beside a rendering from one that rendering makes. A read of an
option or default while it is being rendered is refused at once, on
whatever goroutine: it answers the empty string, counts nothing and fails
nothing, and leaves the option or default for a later read. So is a read
whose rendering needs an option or a fallback chain another goroutine's
read is rendering, where it needs it: the placeholder that needs it takes
the empty string there, and it is left to the read rendering it. A read
that meets a value whose conversion is in progress, in a method of that
value or beside it on another goroutine, is refused where it meets it, and
that read is the one the option or default gets: nothing is reported for
that value, and the empty string is its value for the rest of the
resolution. What the read reached before that point was reached once, with
its reports and its share of `MaxReadLength`, and no later read reaches it
again. So a modifier
that reads one option, or options naming one value, from several goroutines
reads them once before it starts them.

The parser makes `Resolve` wait for a modifier's goroutine in two ways. Each
time the goroutine that called `Resolve` needs the resolution's lock, it also
waits for the parser code a read on a modifier's goroutine is running under
it, each read up to its next call into host code, a whole conversion
included: work the limits of section 13 bound, never host code. So a
modifier's goroutine converting a large value holds `Resolve` until that
conversion ends, even where nothing on the goroutine that called `Resolve`
reads it. And where the walk, outside any read, needs what a read is
producing, the fallback chain a placeholder takes after its modifier was
asked or a value it converts, it waits for that read. A read that finished
during the call, and an `Input` kept past `Resolve`, answers the
text that read produced. A read still producing an option or default when
`Resolve` returns, in host code or waiting to take the resolution's lock
back, goes on only to finish: it calls no more host code, reads no more of
the payload, reports nothing and spends no budget, and it answers the empty
string. Any other read once `Resolve` has returned, one still waiting for the
lock among them, answers what a read finished during the
call, or the empty string where none did. Host code still running when
`Resolve` returns, a method of a value or a modifier's goroutine, can read
the payload until it returns, so the caller changes the payload only once
that code has returned.

None of these can be contained, and each is the host's to avoid: a modifier
that calls `runtime.Goexit` or panics on a goroutine of its own, and host
code that waits for `Resolve` to return while it produces what the walk
needs, which waits forever, and so does `Resolve`.

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
composed as a host modifier's are, so requests of one resolution can carry
the same map, which the supplier reads and never writes to, and `date`
carries a nil one where no layer holds a property:

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
`String` writes it in; an RFC 3339 date-time as `time.RFC3339` reads it, an
uppercase `T`, seconds, a fraction of a second or none, and `Z` or an offset
`+hh:mm` or `-hh:mm`, so a lowercase `t` or `z` is not read; and
`time.DateTime` and `time.DateOnly`, in UTC. A `time.Duration` prints as
`1h0m0s` and is no number.

A request that returns an error or panics, and a category that is none of
the six, take the fallback chain and report `failed-modifier`. The supplier's
text is the placeholder's result, emitted as it stands.

The parser judges no locale and no property beyond what the format decides:
a locale that is no well-formed language tag, a `currency` request with no
well-formed currency code, and a property outside the range `Intl` accepts
each reach the supplier as written. Where ECMA-402 would throw a
`RangeError` or a `TypeError`, the supplier should return an error, so the
placeholder takes its fallback chain and reports `failed-modifier`; a
well-formed tag it has no data for is the supplier's to match to a locale
it has, as `Intl` does.

### Limits and reports

The limits of section 13 are `MaxOutputLength`, `MaxReadLength`,
`MaxConversionNodes`, `MaxSerializedLength` and `MaxNesting`, the text
limits counted in bytes. `MaxSerializedLength`, 10 000 000 bytes, is the
text one conversion writes, a value's own methods' text included; a value
it refuses spends none of the read budget. `MaxReentry`, 1000, is the bound
section 13 lets the parser set in place of the host's limit, which in Go
ends the process: a lineage of 1000 resolutions, each asked for by a modifier
that does nothing else from a placeholder at `MaxNesting`, takes about
9.9 MB of the goroutine's stack on a 64-bit platform, 6.3 MB on a 32-bit
one and 17.2 MB under the race detector, and where each is asked for from a
method `encoding/json` calls 64 levels into a value, about 48 MB, 27 MB and
66 MB with Go 1.26 and 77 MB, 47 MB and 97 MB with Go 1.27, where those
levels are typed maps, the costliest shape measured. A goroutine's stack
grows by doubling, up to Go's default limits of 1 GB and 250 MB, so it
reaches 512 MiB and 128 MiB at most; that leaves each resolution about
490 KB, or 108 KB, with Go 1.26 and 460 KB, or 87 KB, with Go 1.27, for the
host's own frames. A host that lowers the maximum with `debug.SetMaxStack`
below what its lineage takes ends the process before the bound refuses. A
`Report` carries the code and origin of section 14.2, an English message,
the `Context.ID`, the limit it reached and the placeholder as the message
spells it: cut to `MaxReportedLength` bytes and escaped as JSON escapes a
string, every line terminator of the format included, so it never forges a
line in a log. A panicking `OnReport` loses that report and the resolution
carries on.

No limit counts the work done on what a modifier is handed before it
answers (section 13). Here is what that work grows with. The padding around
an option's value or an inline default is read once in a resolution, from
each end, however deep the option nests, so the work grows with the
padding's length. An option key is converted to a number at each `lt`, `gt`,
`lte`, `gte`, `plural` or `ordinal` placeholder that reads it, which grows
with the key's length.
The props a modifier is handed are composed from the three layers once in a
resolution where no wrapper names properties for that modifier. They are
composed again for each run of placeholders whose wrappers hold one layer
under its name, and each composition grows with the width of the three
layers. A composition for `number`, `plural` or `currency` also reads one
property as a number. For `currency` that property is `ratio`. For `number`
and `plural` it is `minimumFractionDigits`, where no layer names a maximum.
A text there is converted once in a resolution, which grows with its length,
and each later composition that reads the same text finds its number by the
text, which reads the text again but converts nothing.
So props that alternate between two wrappers are composed at each
placeholder, and the work grows with the placeholders times the layers'
width: doubling either doubles the time.

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
| `sh bench/prepare.sh && node bench/harness.mjs` | build, then the benchmark (`bench/rows.mjs`) |
| `node bench/harness.mjs --compare <dir>` | the same, beside the module checked out at `<dir>` and built there by its `bench/prepare.sh` |
| `go -C dev run ./pack -module go -version v0.0.0-pack -proxy <dir>` | zip the module from `HEAD` as the module proxy zips it, into `<dir>` laid out as `GOPROXY=file://<dir>` serves it; `-hash` prints the zip's `h1:` hash |

`dev/` is a module of its own, never tagged, holding the programs the
repository's tooling runs; a command for it names it from `go/`, as
`go -C dev vet ./...` does, since the core module does not contain it.
`pack` reads the commit through `git archive`, so what is not committed
never enters the zip, and needs a clone of its own: `golang.org/x/mod`
recognizes no linked worktree.

CI (`.github/workflows/tests-go.yml`) runs the suite on the oldest and the
newest Go the module supports on Linux, macOS and Windows, again under the
race detector and on 386, a platform whose `int` is 32 bits, the formatting
contract over every module under `go/`, the zip `pack` builds as a consumer
meets it — fetched from a proxy laid out on disk and tested in the read-only
module cache, with nothing else in its build list — and, once a release
exists, `gorelease` against it, failing on a change that breaks its exported
surface.

Every `.go` file is ASCII: a character outside ASCII is written as an escape.

The suite runs the specification's conformance set and audits its runner
against the set's defect catalogue. The set is the release vendored at
`internal/conformance/set/`, checked against the sha256 its release states;
`CURLY_CONFORMANCE_FIXTURES=<dir>` runs a set under development instead, a
directory laid out as the conformance package is. The set runs at Core and
Extensions, the Intl level left out, and again at every level against a
stand-in supplier with each formatting request exposed, which measures the
request the parser makes rather than a host's locale data. Both runs leave
out the cases that hand over a value refusing to be read: a Go host has none,
since payload and props are read as maps, which run no host code.
`internal/adapter/testdata/failing.txt` lists the cases of the vendored set
the parser is known to fail, and why.

## Benchmarks

The benchmark is the family's: `bench/harness.mjs`, on Node, measures the
rows `bench/rows.mjs` lists, and `bench/check/` holds the rows the harness is
checked on; both are copies of the ones in every package of the family.
`bench/prepare.sh` builds the two programs the rows read, into `dist/`:
`dist/size`, a program resolving one message, whose stripped binary the size
rows weigh, and `dist/bench` (from `dev/bench`), which counts the counts and
runs the times. A time row starts `dist/bench` once in each process that
measures it and asks it for one batch of a fixed number of iterations at a
time, the number named in the row, so the round trip over the pipe, tens of
microseconds, is the same on both sides of a comparison and inside the
spread. The rows that ask for a locale's text do so of a stand-in supplier
that answers at once, so they measure what the parser does around a
request. `package.json` is private and carries only the name and version
the harness's table is headed with.

## Releasing

A Go module has no registry: a version is the tag `go/vX.Y.Z` on the
repository, which the module proxy and the checksum database record, with
the hash of what it holds, the first time anyone asks for it, and keep for
good. A version published by mistake cannot be withdrawn; the remedy is a
later version whose `go.mod` retracts it.

A release is cut from `main` by the **Go parser release** workflow
(`.github/workflows/publish-go.yml`, started by hand, with the module
directory to release). The changelog's pending section names the version it
will be released as, `### 1.0.0 (Unreleased)`, and the release types read it
as the JavaScript parser's workflow does: `patch`, `minor` and `major` must
arrive at that version and cut the section, and `next` tags a prerelease of
it, `v1.0.0-next.0` and then `.1`, and leaves the section open. The version
line the bump starts from is `package.json`'s, since the version is not
tagged until the release exists, and the workflow refuses a version already
tagged, and from version 2 on, a module path that does not end in its major
version.

After the test matrix, three jobs run: one settles the version; one, with
no credential, benchmarks the module into `BENCH.md`, makes the release
commit locally and computes, from that commit, the `h1:` hash the proxy
will record (`dev/pack`); and the last, in the `release` environment,
makes the release commit again, refuses to push unless its tree is the one
the build hashed, then tags it, pushes the commit and the tag in one atomic
push, asks the module proxy for the version and fails the run on a hash
other than the build's, and publishes a GitHub release carrying the
changelog section. The commit, the tag and the release are made as the
GitHub App whose client ID and private key the repository holds as the
`APP_CLIENT_ID` variable and the `APP_PRIVATE_KEY` secret of that
environment. No token publishes anything and nothing is published by hand:
the first version is a tag like every other.

A release that stops after its tag exists is finished by **Re-run failed
jobs** on its run, before any further release of the module and within 30
days of the run or the build artifact's retention, whichever ends first: the
re-run reuses the run's version and build, makes the same commit, finds it
tagged, pushes nothing, asks the proxy for the version again, and publishes
the GitHub release unless it is published already. Re-running all jobs does
not finish it, since the job that settles the version refuses one already
tagged, before anything is built. A release that stops before its tag exists
is dispatched again, and cuts the same version. A release that cannot be
finished this way — a defect in the workflow's own steps after the push, or a
run past that window — has no GitHub release, and the next version is
released.

## License

[MIT](./LICENSE)
