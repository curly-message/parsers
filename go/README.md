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
reads it, and must not resolve without end where nothing counts it: neither
can be contained. Go ends the process when a goroutine's stack outgrows its
maximum, with no error a program can recover from, so the parser bounds the
resolutions in progress itself where it can count them: those a modifier
asks for through an `Input` (see Host modifiers). Go names no goroutine a
count could be kept for, so a resolution `Parser.Resolve` starts counts as
the first of a chain of its own wherever it is called from. A method of a
value, a reporting handler, the `Intl` supplier, or a modifier that resolves
through `Parser.Resolve` rather than an `Input` of its chain, leaves the
count, as section 13 says a chain leaving a thread of execution leaves its
bound; yet that chain stays on the goroutine's stack, so one that reaches
itself without end ends the process rather than the placeholder taking its
fallback chain. That is the host's to avoid: there this port does not meet
section 13's requirement to contain the failure.

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
that met the defect, so calls of one resolution can overlap.

A modifier may resolve another message, which spends budgets of its own.
`Input.Resolve(p, message, context)` resolves it as `p.Resolve` does, with
any parser, as one more resolution in progress than those of its chain: the
resolution `Parser.Resolve` started and every one asked for through an
`Input` of one of them, whichever of their `Input`s it is asked through. One
that would count past `MaxReentry`, 1000, resolves nothing and returns
`curly.ErrReentryLimit`. One asked for from a method `encoding/json`
calls, as it writes a value the asking resolution converts, sits below every
level above that method on the goroutine's stack, and the parser hands
`encoding/json` no value nested more than 64 levels deep, so
it counts as one all the same. The modifier handles the refusal as any
error; returned as its own, it fails the modifier, whose placeholder reports
`failed-modifier` and takes its fallback chain, and every resolution around
it answers. So a message whose modifiers resolve it again through their
`Input` ends in a fallback chain by the 1000th resolution rather than ending
the process. Only resolutions count, never the walk of one: the options and
the default a modifier reads count nothing, so each resolution of the chain
renders to `MaxNesting`. An `Input` kept from further out counts on all the
same, the outermost resolution's or one whose own resolution has returned,
so a chain that resolves through one rather than its own deepens the count
as it deepens the stack. And an `Input` counts on wherever it is called, on
a goroutine of the modifier's own as well: there the chain continues on a
stack of its own, which section 13, counting each thread of execution
apart, counts from none, and resolutions of one chain in progress at once
on several goroutines count together, so this port can refuse a resolution
the section never refuses. An `Input` whose chain has no resolution left in
progress, and the zero `Input`, count from none, as `Parser.Resolve` does,
so a chain that leaves them is one the bound does not reach (see Values).

The parser makes `Resolve` wait for a modifier's goroutine in two ways. Each
time the goroutine that called `Resolve` needs the resolution's lock, it also
waits for the parser code a read on a modifier's goroutine is running under
it, each read up to its next call into host code, a whole conversion
included: work the limits of section 13 bound, never host code. So a
modifier's goroutine converting a large value holds `Resolve` until that
conversion ends, even where nothing on the goroutine that called `Resolve`
reads it. And where code on the goroutine that called `Resolve` reads what a
read is producing, it waits for that read by the rule above: the walk, which
reads the fallback chain a placeholder takes after its modifier was asked and
the values it converts, or any host code the walk calls there. A read that
finished during the call, and an `Input` kept past `Resolve`, answers the
text that read produced. A read still producing an option or default when
`Resolve` returns, in host code or waiting to take the resolution's lock
back, goes on only to finish: it calls no more host code, reads no more of
the payload, reports nothing and spends no budget, and it answers the empty
string. Any other read once `Resolve` has returned, one still waiting for the
lock or for that producer among them, answers what a read finished during the
call, or the empty string where none did. Host code still running when
`Resolve` returns, a method of a value or a modifier's goroutine, can read
the payload until it returns, so the caller changes the payload only once
that code has returned.

None of these can be contained, and each is the host's to avoid: a modifier
that calls `runtime.Goexit` or panics on a goroutine of its own; one that,
through state it keeps, reads an option or default of a placeholder while
that option or default is being rendered, or a value from inside a method of
that value the parser called, which waits for itself; and host code that
waits for `Resolve` to return while it produces what code on the goroutine
that called `Resolve` reads. A read waiting for itself on a goroutine of
the modifier's waits until `Resolve` returns; on the goroutine that called
`Resolve` it waits forever, and so does `Resolve`.

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
ends the process: a chain of 1000 resolutions, each asked for by a modifier
that does nothing else from a placeholder at `MaxNesting`, takes about
9.4 MB of the goroutine's stack on a 64-bit platform, 5.9 MB on a 32-bit
one and 15.5 MB under the race detector, and where each is asked for from a
method `encoding/json` calls 64 levels into a value, about 47 MB, 26 MB and
64 MB with Go 1.26 and 77 MB, 47 MB and 96 MB with Go 1.27, where those
levels are typed maps, the costliest shape measured. A goroutine's stack
grows by doubling, up to Go's default limits of 1 GB and 250 MB, so it
reaches 512 MiB and 128 MiB at most; that leaves each resolution about
490 KB, or 108 KB, with Go 1.26 and 460 KB, or 87 KB, with Go 1.27, for the
host's own frames. A host that
lowers the maximum
with `debug.SetMaxStack` below what its chain takes ends the process before
the bound refuses. A
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
request the parser makes rather than a host's locale data.
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

## License

[MIT](./LICENSE)
