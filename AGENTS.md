# AGENTS.md

Behavioral guidelines for LLM coding assistants working on the
**curly-message parsers** repository.

**Precedence:** These repo rules override individual LLM memory or personal
preference. If your own memory conflicts with this file, follow this file.

This repository follows the rules of the Curly Message Format family in
[`spec`'s AGENTS.md](https://github.com/curly-message/spec/blob/main/AGENTS.md)
(sections 1-14). What follows is only what differs here.

Those rules are not in this file, and nothing loads them for you: before any
change, read `spec`'s AGENTS.md in full — `../spec/AGENTS.md` when it is
checked out beside this repository on an up-to-date `main`, otherwise
[the raw file](https://raw.githubusercontent.com/curly-message/spec/main/AGENTS.md)
— and follow it as fully as the rules below.

---

## The repository

Official implementations of the
[Curly Message Format](https://github.com/curly-message/spec).

- **One directory per language** — `js/`, `go/`, `rust/`, `python/` and `php/`
  today. Each is a fully standalone package with its own build, tests,
  configs, README, CHANGELOG and version line. There is no root package
  manifest and no workspace: run every command from inside the implementation
  directory, for every directory a change touches.
- **Release tags are namespaced by directory** — `js-v1.0.0`,
  `python-v1.0.0`, `php-v1.0.0` — so moving an implementation to its own repository later
  is not a breaking event.
  A Go module's tag is `go/v1.0.0`, the only form the go command reads a
  version from for a module in a subdirectory; a Go module's path is where it
  lives, so moving one is a breaking event whatever its tags. The publish
  workflows cut them (`.github/workflows/publish-js.yml`, `publish-go.yml`,
  `publish-rust.yml`, `publish-python.yml`, `publish-php.yml`; the README's
  Releasing section says what they do). A package whose registry reads only a
  repository's root and its own tag spelling is served from a repository of
  its own that only its publish workflow writes to: `php/` from
  `curly-message/parser-php`, tagged `v1.0.0` there for `php-v1.0.0` here.
- The root holds only `README.md`, this file, `CLAUDE.md`, `LICENSE`,
  `.gitignore`, `.githooks/` and `.github/workflows/`. An implementation
  directory may hold a `.gitignore` of its own, for what only its toolchain
  writes.
- **One pre-commit hook for every directory.** `.githooks/pre-commit`, POSIX
  `sh`, runs once `core.hooksPath` is `.githooks` (`npm ci` in `js/` sets it,
  `git config core.hooksPath .githooks` sets it without Node). For each
  top-level directory a commit touches, it runs `.githooks/<dir>.sh` from
  inside that directory, where there is one. A script exits 0 when its check
  passes; 75 when a tool it needs is missing, having printed which and how to
  install it, which skips the directory (`<dir>: skipped`) without failing the
  commit; and anything else to fail the commit. An implementation's check is
  its script: `.githooks/js.sh` for `js/`, `.githooks/go.sh` for `go/`,
  `.githooks/rust.sh` for `rust/`, `.githooks/python.sh` for `python/` and
  `.githooks/php.sh` for `php/`.
- **`bench.yml` reads its packages from the branch and the base**: each
  directory that holds a `bench/rows.mjs` outside a `bench/check/`, with its
  own copy of `bench/harness.mjs` and `bench/check/`, on the branch, or on the
  base where the branch still holds a file of it that is neither a `*.md` nor
  under its `tests/`. On a PR it benchmarks those with a changed file that is
  neither a `*.md` nor under the package's `tests/`, or all of them where
  `bench.yml` itself changed; a package whose branch lacks its
  `bench/rows.mjs`, `bench/harness.mjs` or `bench/check/check.mjs` fails,
  naming what is missing. It sets up Node, which the harness runs on, and a
  toolchain per manifest the package holds: uv for a `pyproject.toml`, Go
  from a `go.mod`, PHP from the `.php-version` beside a `composer.json` (with
  `intl` and `opcache`), and the runner's rustup, which reads
  `rust-toolchain.toml`, for a `Cargo.toml`. Each side is prepared from the
  package's directory, by `sh bench/prepare.sh` where the package has one,
  and by `npm ci --ignore-scripts && npm run build` otherwise. The base is
  prepared and compared only where its commit has the package's
  `bench/rows.mjs`; where it has none, as on the PR that adds the package or
  its benchmark, or one that renames its directory, nothing is compared.
- **macOS runs the ends of each test matrix.** GitHub places few macOS
  jobs at once, so on macOS each test workflow runs only the oldest and the
  newest version of its runtime or toolchain, and one variant of an axis
  Linux runs both ways (Python and PHP with Intl, Rust with `intl`); Linux
  and Windows run the whole matrix. A job GitHub never places ends cancelled
  with no runner and no step: `rerun.yml` re-runs the failed jobs of a test
  workflow's first attempt once, where every job that neither succeeded nor
  was skipped is such a job, and leaves alone a run with one that ran.
- Issues live in the family's
  [shared tracker](https://github.com/curly-message/spec/issues).

## js/

Tech stack of `js/` — **ground truth, do not assume otherwise**:

| | |
|---|---|
| Package | `@curly-message/parser`, released, `latest` |
| Language | TypeScript, ESM only |
| Package manager | npm |
| Build | tsup, one minified entry point |
| Tests | vitest; the conformance set as a `devDependency` |
| Lint | ESLint flat config with `@stylistic`, run by a pre-commit hook |
| Runtime dependencies | none |
| Supported runtimes | Node 22+, Bun, Deno 2 |
| CI | `tests-js.yml` (calls `tests.yml`), `bench.yml` and `bench-label.yml`, `publish-js.yml` (calls `publish.yml`) |

Commands, run from `js/`:

| Command | What it does |
|---------|--------------|
| `npm ci` | install from the lockfile |
| `npm test` | build, typecheck the source and the shipped declarations, lint, then the suite against the source and against the build — what CI runs |
| `npm run test:bun`, `npm run test:deno` | build, then the suite on Bun or Deno — what the runtime legs of CI run |
| `npm run bench` | build, then the benchmark (`bench/rows.mjs`) |
| `npm run bench -- --compare <dir>` | the same, beside the package checked out and built at `<dir>` — what `bench.yml` runs on a PR |
| `npm run lint:fix` | fix what the formatting contract reports |
| `npm run dev` | rebuild on change |

Map of `js/`:

| Path | Role |
|------|------|
| `src/index.ts` | `createParser` and the resolution pipeline |
| `src/extract.ts` | `createExtractor`: the parameters a message names |
| `src/cst.ts` | `cst`: the tree a message is described by |
| `src/modifiers.ts` | the modifiers the format defines |
| `src/utils.ts` | pure helpers, the placeholder scanner among them, which all three read |
| `src/types.ts` | the public types and their doc comments |
| `tests/specs/` | the suite (see *Tests*) |
| `tests/conformance/adapter.ts` | the adapter of the specification's section 14.3 |
| `bench/rows.mjs` | the rows of the benchmark, each read off the build; `bench/harness.mjs` measures them, `bench/check/` holds the rows it is checked on, and both are the same in every repository of the family |

### Architecture you must respect

**The public surface is released.** One entry point carries `createParser`
for resolution, `createExtractor` for the build-time scanner and `cst` for
describing a message as it is written, each re-exported from `index.ts` rather
than from a subpath of its own. A change to it is a breaking change and costs
every caller a migration: propose one rather than making it.

**Diagnostics leave through the `onReport` option and nowhere else** — from a
placeholder the parser could not resolve, from the chain a message resolves
through, and from the limits a resolution stops at. The format specifies no
channel to report through, so a parser writes to none. Reporting somewhere
directly, adding a `code`, or widening what a `Report` carries, is a change to
the public surface.

**Reuse the helpers that guard the payload** — `ownValue`, `unesc`,
`mergeLayer`. Grep before adding a helper.

**Prototype keys are missing payload entries.** Reads of the payload by a key
taken from the message go through `ownValue`, so
`toString`/`__proto__`/`constructor` resolve as missing rather than as
inherited members.

**The resolution limits are load-bearing.** `MAX_OUTPUT_LENGTH`,
`MAX_READ_LENGTH`, `MAX_CONVERSION_NODES`, `MAX_SERIALIZED_LENGTH`,
`MAX_NESTING` and `MAX_REENTRY` are what keep the cost of a resolution bounded
by the message rather than by the payload: a value no conversion terminates
on, a serialization longer than every supported engine holds, a message nested
past what a host will walk, a payload handing back more text than a caller can
hold, and a flat message whose modifiers each read an option or a default the
one before kept each stop at one of them. Don't remove them, don't raise them
to buy behavior, and treat a change to any of them as a format change.

**A report carries message text, and carries it escaped.** `Report.text` is
the placeholder that reported or the message that held it, never a payload
value. It leaves truncated and with every line terminator escaped — a consumer
that writes a report somewhere must not be able to have a message forge a line
there. `JSON.stringify` is not enough on its own: it leaves raw every
terminator it has no short escape for, U+2028 and U+2029 among them. The
escaping reads its set from `LINE_TERM`, the one list the scanner reads too,
so amending `line-term` reaches both — never spell the terminators out a
second time. Anything new a `Report` carries out takes the same treatment.

**The scan is a hand-written walk over consumer-controlled text**, not a
pattern: it reads each code point once and settles each opening pair into a
memo, which is what keeps a message of nested braces linear instead of
exponential. Reaching for a regular expression here, or dropping the memo,
needs an explicit check that neither reintroduces backtracking or re-reading;
if you touch the scan and can't establish that, flag it.

**Resolving a message must not throw.** A missing payload key yields the
declared default, and so does a modifier the parser does not know — that one
reports as well, because naming a modifier nobody registered is a defect in
the message rather than a comparison to run.

**Docs in each implementation directory** are its `README.md` (the npm page),
its `CHANGELOG.md` and the doc comments in `src/types.ts`.

### Tests

- `js/tests/specs/`: `index.spec.ts` for resolution, `extract.spec.ts` for
  the parameters a message names, `cst.spec.ts` for the tree a message is
  described by, `types.spec.ts` for the type surface, `checker.spec.ts` for
  what the types cost a consumer's checker, and `conformance.spec.ts` for the
  specification's conformance set, driven through
  `tests/conformance/adapter.ts` — with fixtures in `tests/data/`.
- `js/tests/types/`: `types.spec.ts` compiled a second time, as a consumer
  compiles it — against the shipped declarations, at `skipLibCheck: false` —
  with `surface.ts` asserting those declarations are the source's.
  `npm run typecheck:dist` runs it; `npm test` runs it before the suite.
- Drive behavior through `createParser(options).resolve`, `createExtractor`
  and `cst`; pure helpers may be imported from `src/` when that yields a more
  deterministic test.
- The suite imports the package by its name, `@curly-message/parser`, and
  runs twice: against the source, and under `--mode dist` against the build a
  release ships. An import of the package by a path into `src/` tests the
  source twice.
- What a message resolves to is the conformance set's to pin: a case the
  suite adds beyond it is either an implementation detail or a gap in the set
  (§ Architecture of `spec`'s AGENTS.md).

## go/

Tech stack of `go/` — **ground truth, do not assume otherwise**:

| | |
|---|---|
| Module | `github.com/curly-message/parsers/go`, package `curly`, released as a prerelease, `v1.0.0-next.0`; tags `go/vX.Y.Z` |
| Language | Go 1.26+ |
| Build | the go command; no build step ships |
| Tests | `go test`, examples included; the conformance set vendored and checked by its sha256 (`internal/conformance/set/`) |
| Lint | `gofmt`, `go vet`, staticcheck v0.8.2, `go mod tidy -diff`, ASCII-only `.go`, all run by `tests-go.yml`'s Lint job; `.githooks/go.sh` runs `gofmt`, `go vet` and the ASCII check |
| Runtime dependencies | none: `go list -m all` prints the module alone |
| Supported toolchains | the Go `go.mod` names, the latest patch of that release, and the latest stable, on Linux, macOS and Windows, and Go 1.26 with `GOEXPERIMENT=jsonv2`, on Linux |
| CI | `tests-go.yml`, `bench.yml` and `bench-label.yml`, `publish-go.yml` |

Commands, run from `go/`:

| Command | What it does |
|---------|--------------|
| `go test ./...` | the suite, the conformance set and the defect audit included — what CI runs on each leg |
| `go test -race ./...` | the same under the race detector |
| `gofmt -l .`, `go vet ./...`, `go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...`, `go mod tidy -diff` | the formatting and lint contract, also run with `go -C dev` |
| `sh bench/prepare.sh && node bench/harness.mjs` | build, then the benchmark (`bench/rows.mjs`) |
| `node bench/harness.mjs --compare <dir>` | the same, beside the module checked out at `<dir>` and built by its `bench/prepare.sh` — what `bench.yml` runs on a PR |
| `go -C dev run ./pack -module go -version <v> -proxy <dir>` | the module zip as the proxy builds it, from a commit; needs a plain clone, not a linked worktree |

Map of `go/`:

| Path | Role |
|------|------|
| `curly/curly.go` | `New`, `Options`, `Context`, `Parser.Resolve`, the limits |
| `curly/scan.go`, `curly/text.go` | the placeholder scanner and the format's text classes, `lineTerm` among them |
| `curly/walk.go`, `curly/layers.go`, `curly/modifiers.go` | the resolution walk, the property layers, the modifiers the format defines |
| `curly/value.go`, `curly/jsonw.go`, `curly/fmtv.go`, `curly/numeric.go` | the value model: the JSON and `%v` writers, number text |
| `curly/intl.go` | the `Intl` interface the Intl modifiers delegate to |
| `curly/report.go` | `Report` and its escaping |
| `curly/cst.go` | `CST` and `Node.AppendJSON` |
| `internal/conformance/` | the runner of the set, and the vendored release |
| `internal/adapter/` | the adapter of section 14.3, the set run and the audit |
| `dev/` | a never-tagged module: `bench`, `size`, `pack` |
| `bench/rows.mjs`, `bench/prepare.sh` | the rows, read off the programs `prepare.sh` builds into `dist/`; `bench/harness.mjs` and `bench/check/` are the family's copies |

### Architecture you must respect

The rules of `js/` above hold here, in their Go form:

- **The public surface** is package `curly`'s exported identifiers. Until
  the first release it is a prerelease's and promises nothing; from then
  on, the API job of `tests-go.yml` fails on a change `gorelease` calls
  incompatible.
- **Diagnostics leave through `Options.OnReport`** and nowhere else.
- **A Go value has no prototype.** An own-entry read is a map lookup, and a
  value is plain by its `reflect` type: an unnamed map keyed by `string`, or
  an unnamed slice of anything but bytes.
- **The limits** are `MaxOutputLength`, `MaxReadLength`, `MaxConversionNodes`,
  `MaxSerializedLength` and `MaxNesting`, plus `hostDepth`, how deep a value
  the parser hands to code it does not own, and `MaxReentry`, how many
  resolutions a lineage holds in progress, through a `Resolver` and through
  reads of options and defaults kept from another call. Each is
  load-bearing, as in `js/`.
- **A report carries message text, escaped.** `lineTerm`
  (`curly/text.go`) plays the role of `LINE_TERM`: the scanner and the
  report escaping both read it.
- **The scan is a hand-written walk with a memo**, as in `js/`.
- **Recursion is bounded by the parser, not the payload.** The walk's
  recursion is bounded by `MaxNesting`; the parser's own JSON and `%v`
  writers walk on a stack of their own, and hand code they do not own only a
  value at most `hostDepth` deep. `Parser.Resolve` begins a lineage and is
  never refused, and each piece of host code the parser calls is handed a
  `Resolver` that continues it: a modifier through its `Input`, a `Texter`,
  the `OnReport` handler and an `IntlResolving` supplier. A resolution asked
  for through one counts as one more than those of its lineage in progress,
  and is refused past `MaxReentry`, an error the caller handles rather than
  a panic; so does a read of an option or default by any host code but that
  placeholder's modifier while it is current, which past the bound answers
  the empty string and fails nothing. A read never waits: one of what is
  being rendered is refused at once, and one that meets a value being
  converted is refused there and recorded as that option's one production.
  Host code that calls `Parser.Resolve` where it was handed a `Resolver`
  has left its lineage, and bounds that recursion itself. Nothing reads the
  goroutine's stack.
- **Properties are composed once per resolution** for the layers every
  placeholder shares, and once for each run of a modifier's placeholders
  whose wrappers hold one layer; the walk keeps the shared composition and,
  for each modifier, the one composed last, never one per wrapper and
  nothing past the walk. A resolution that composes nothing and calls no
  host-defined modifier allocates nothing for either.
- **Each resolution holds a walk lock** and releases it around every call
  into host code.
- **Resolving a message must not panic**: a panicking method, modifier,
  supplier or `OnReport` is contained.
- **Docs** are `go/README.md`, `go/CHANGELOG.md` and the doc comments of the
  exported identifiers.

### Tests

- The suite runs the conformance set at Core and Extensions. It runs it
  again at every level against a stand-in supplier that exposes each
  request.
- The Pack job tests the module zip as a consumer meets it: fetched from a
  file proxy and run in the read-only module cache. This stands in for
  `--mode dist`.
- Cost is asserted by count, with `testing.AllocsPerRun` and call
  counters, never by time.

## rust/

Tech stack of `rust/` — **ground truth, do not assume otherwise**:

| | |
|---|---|
| Crate | `curly-message-parser`, library `curly_message_parser`, released as a prerelease; the first version, `1.0.0-next.0`, was published by hand |
| Language | Rust, edition 2024, MSRV 1.88 |
| Package manager | cargo, one workspace rooted at `rust/` (the parser, `conformance/`, `bench/`), `Cargo.lock` committed and every command `--locked` |
| Build | `cargo build`; `cargo package` builds the `.crate` from the files it ships |
| Tests | `cargo test`, with and without the `intl` feature; the conformance set vendored and checked by its sha256 (`conformance/set/`) |
| Lint | `rustfmt` (`rustfmt.toml`) and clippy at `-D warnings` under the `[workspace.lints]` table of `rust/Cargo.toml`, both run by `tests-cargo.yml`'s Lint job and by `.githooks/rust.sh` |
| Runtime dependencies | none; the `intl` feature brings ICU4X 2.4, `icu_experimental` 0.7 and `jiff` 0.2 |
| Supported toolchains | Rust 1.88 and the latest stable, on Linux, macOS and Windows |
| CI | `tests-rust.yml` (calls `tests-cargo.yml`; weekly, resolves the newest ICU4X beside the crate), `bench.yml` and `bench-label.yml`, `publish-rust.yml` (calls `publish-cargo.yml`) |

Commands, run from `rust/`:

| Command | What it does |
|---------|--------------|
| `cargo test -p curly-message-parser --locked` | the suite, the conformance set and the audit at Core and Extensions, the README's examples and the doc comments' |
| `CURLY_EXPECT_INTL=1 cargo test -p curly-message-parser --locked --features intl` | the same with the Intl level, on both readings of the set |
| `cargo test -p curly-message-conformance --locked` | the runner's own tests |
| `cargo fmt --all --check` | the formatting contract (`cargo fmt --all` fixes it) |
| `cargo clippy --workspace --all-targets --locked -- -D warnings`, and again with `--all-features` | the lint contract, tests included |
| `RUSTDOCFLAGS=-Dwarnings cargo doc -p curly-message-parser --no-deps --all-features --locked` | the docs, every public item documented |
| `sh bench/prepare.sh && node bench/harness.mjs` | build the release build into `bench/dist/`, then the benchmark (`bench/rows.mjs`) |
| `node bench/harness.mjs --compare <dir>` | the same, beside the crate checked out at `<dir>` and prepared by its `bench/prepare.sh` — what `bench.yml` runs on a PR |

Map of `rust/`:

| Path | Role |
|------|------|
| `src/lib.rs` | the public surface and the limits |
| `src/parser.rs` | `Parser`, its builder, `Context` and `Message`; the backstop of `resolve` |
| `src/walk.rs` | one resolution: the walk on an explicit stack, the fallback chain, the modifier call |
| `src/depth.rs` | containment of host code, the per-thread count of resolutions in progress and the current modifier |
| `src/scan.rs`, `src/syntax.rs` | the placeholder scanner, `LINE_TERM` among its classes, which the walk and the tree both read; what a placeholder declares, and unescaping |
| `src/cst.rs` | `cst`: the tree, an arena with byte spans |
| `src/value.rs`, `src/convert.rs`, `src/number.rs` | the value model of section 4, its conversion to text, and ECMAScript's numbers and dates |
| `src/modifier.rs`, `src/props.rs`, `src/builtins.rs` | `Modifier` and its input, the property layers as a borrowed view, the comparisons of section 11.1 |
| `src/intl.rs`, `src/formatting.rs` | the `Intl` interface the Intl modifiers delegate to, and the modifiers' requests |
| `src/ecma.rs`, `src/icu.rs` | with `intl`: ECMA-402's reading of a request's options, and `IcuIntl`, with `UNEXPRESSIBLE` |
| `src/report.rs` | `Report`, `Code`, `Origin` and the escaped excerpt |
| `tests/` | the suite, linked against the crate as a consumer links it; `tests/conformance/adapter.rs` is the adapter of section 14.3 |
| `conformance/` | `curly-message-conformance`, `publish = false`: the runner of the set, the vendored release and Node's answers to its requests |
| `bench/` | `curly-message-bench`, `publish = false`: the programs the rows run; `bench/rows.mjs`, `bench/prepare.sh`; `bench/harness.mjs` and `bench/check/` are the family's copies |

### Architecture you must respect

The rules of `js/` above hold here, in their Rust form:

- **The public surface** is the crate's public items. Until the first
  release it is a prerelease's and promises nothing; from then on, the Lint
  job of `tests-cargo.yml` holds it to the newest version on crates.io with
  `cargo semver-checks`.
- **Diagnostics leave through `ParserBuilder::on_report`** and nowhere
  else; the lints deny `print_stdout` and `print_stderr`, and ICU4X's own
  messages go to the `log` facade.
- **The value model is the crate's.** Rust has no dynamic value, so `Value`
  is the host type section 4 reads: an `Object` holds its own entries in
  insertion order, every key a name (`__proto__` included), and a `Host`
  value is read through its `Display` alone.
- **The limits** are `MAX_OUTPUT_LENGTH`, `MAX_READ_LENGTH`,
  `MAX_CONVERSION_NODES` and `MAX_NESTING`, as in `js/`, the text ones in
  bytes of UTF-8, the unit a `str` is indexed by; plus
  `MAX_SERIALIZED_LENGTH`, the most one serialization writes, in bytes of
  UTF-8 too, and `MAX_REENTRY`. Each is load-bearing.
- **The walk keeps its own stack.** A message, a value and a tree of any
  depth are walked, converted and dropped without a call per level. The
  host's stack is reached only through host code (a modifier, an `Intl`
  facility, a `Display`, a report handler), every call of which goes through
  `depth::host_call`, contained, with no borrow of the walker's state held
  across it.
- **`MAX_REENTRY` counts resolutions and the reads that are not the walk's,
  and its cost is measured.** It bounds the resolutions in progress on one
  thread, across parsers, and with them every read of an option's value or a
  default, through a `ModifierInput` a modifier lent, that is not the walk's
  (one made while that placeholder's modifier is not the current one, the
  innermost modifier call on the thread that has not returned, or while one
  of that placeholder's options or its default is being rendered), each while
  it renders its text; and nothing of one resolution's walk: the nesting
  limit bounds what a walk puts on the thread's stack, the fallback chain and
  the reads the current modifier makes of its own placeholder, so no
  resolution is cut short by the ones around it. A counted read past the
  bound, and a read of an option or a default from inside its own rendering
  whoever makes it, answers the empty string, records nothing, unwinds
  nothing and fails nothing. The refusal of a resolution unwinds
  with `ReentryLimit` from `Parser::resolve`, outside its backstop, so it
  reaches the host code that asked, and `std::panic::resume_unwind` keeps
  the panic hook out of it; one refused while the thread already unwinds
  answers the empty string instead, since an unwind leaving a `Drop` would
  abort the process, and `depth::contained` fails the host call that asked
  for it all the same. Its worst cases, `MAX_REENTRY` resolutions each as
  deep into its options as a placeholder resolves, one of them for each
  request of the Intl facility, and as many counted reads of a lent input,
  the innermost reading its own options as deep as a placeholder resolves,
  run on a 512 KiB thread in a debug build on
  every OS leg; the one whose innermost formats a date in a zone name the
  process has not used yet runs on a 1 MiB thread, in a binary of its own,
  since jiff reads a zone's rules the first time the process formats in each
  of its names, an alias being a name of its own, and each read costs a
  debug build about 230 KiB. A change that grows what
  a resolution costs the stack, or raises the bound, is checked there.
  Stack exhaustion aborts the process rather than panicking, so this bound
  is the only containment there is. The count, the current modifier, and
  the flag `depth::contained` reads for a refusal made while the thread
  unwinds, are `thread_local!`s, and whether the thread unwinds is
  `std::thread::panicking`'s, all the OS thread's: stackful fibers that share
  an OS thread share them, so while one is suspended in a `Drop` mid-unwind a
  refusal on any of them answers the empty string and can fail another
  fiber's host call in place of the one that asked, and the resolution of one
  moved to another OS thread in its middle stays counted on the thread it
  left and lowers the count of the thread it finishes on: the guard of a
  resolution in progress, `depth::InProgress`, is not `Send`, so the fiber is
  moved only under a `Send` claimed for a stack that does not have it, by a
  host's `unsafe impl Send` or a coroutine library's own.
  That is where the port falls short of section 13, which counts per thread
  of execution, and the README's Limits and the docs of `MAX_REENTRY` and
  `ReentryLimit` say so.
- **A report carries message text, escaped.** `LINE_TERM` (`src/scan.rs`)
  plays its role in `js/`: the scanner and the report escaping both read it.
- **The scan is a hand-written walk** that records each opening pair's
  verdict once, as `js/`'s memo does, and `src/` holds no regular
  expression.
- **Numbers are ECMAScript's.** What text a number reads as and how a number
  is written follow ECMAScript's `StringToNumber` and `Number::toString`, not
  Rust's `parse` and `Display`.
- **Resolving a message must not panic**: the lints deny `unwrap`, `expect`,
  `panic`, unchecked indexing and arithmetic in `src/`, a panicking modifier,
  `Intl` facility, `Display` or report handler is contained, and
  `tests/fuzz.rs` fails on any panic raised from `src/`. The one unwind
  `src/` starts is the refusal of a resolution past `MAX_REENTRY`, into host
  code the parser called. `unsafe` is forbidden in `src/`.
- **The `intl` feature configures nothing.** It makes `IcuIntl` available;
  a parser claims the Intl level only when its builder is handed a supplier,
  so cargo's feature unification never changes what another crate's parser
  resolves.
- **Generated files**: `target/` and `bench/dist/` are never committed, and
  the golden files of Node's answers are written by `conformance/golden.mjs`,
  never by hand.
- **Docs** are `rust/README.md` (the crates.io page, its examples run as
  doc tests), `rust/CHANGELOG.md` and the doc comments of the public items.

### Tests

- `rust/tests/`: `resolve.rs` for resolution, `modifiers.rs` for the
  Extensions level, `limits.rs` for the limits and the stack-independence
  tests, `panics.rs` (its own binary, with a silent panic hook) for
  containment, the refusal past `MAX_REENTRY` and its worst case, `lent.rs`
  for the reads of an input a modifier lends, through a scoped thread-local,
  counted and refused, `cost.rs`
  and `allocations.rs` for cost by count, `cst.rs` for the tree, `value.rs`
  for the value model, `fuzz.rs` for messages written at random, `intl.rs`
  (with the facility's worst cases of `MAX_REENTRY`), `intl_supplier.rs`
  and `intl_parity.rs` for the Intl level, `intl_stack.rs` (its own binary,
  so no zone is used before it) for its worst case in a zone name, and
  `conformance.rs` for the set and the audit.
- With `intl`, the set and the audit run twice: on the requests the parser
  exposes, and on the text, against Node's answers recorded in
  `conformance/set/golden-4.2.0.json`. `CURLY_EXPECT_INTL` holds a leg to the
  level it means to test.
- The Packaged job of `tests-cargo.yml` tests the `.crate` a release ships:
  its own tests from the unpacked archive, and `tests/` in a project that
  depends on it. This stands in for `--mode dist`.
- Drive behavior through the public API; a table of pure helpers (the number
  conversions) is a unit test inside the crate. Cost is asserted by count,
  with a counting allocator and call counters, never by time, and no test
  assumes more stack than the thread it spawns.

## python/

Tech stack of `python/` — **ground truth, do not assume otherwise**:

| | |
|---|---|
| Package | `curly-message` on PyPI, imported as `curly_message`, released as a release candidate; the first version, `1.0.0rc0`, was published by the workflow through a pending trusted publisher |
| Language | Python, typed (`py.typed`) |
| Package manager | uv, one lockfile (`uv.lock`); the commands are `tasks.py`'s |
| Build | hatchling, pinned by hash in `build-constraints.txt`: an sdist and a pure wheel |
| Tests | pytest; the conformance set vendored as its release archive in `tests/conformance/`, pinned by its digest (`PIN` in `tests/conformance/runner/archive.py`) |
| Lint | ruff (lint and format) and mypy strict, run by the Lint job of `tests-python.yml` and by `.githooks/python.sh` |
| Runtime dependencies | none; the `intl` extra adds Babel (2.14 or later) and tzdata |
| Supported runtimes | CPython 3.11 to 3.15 on Linux, macOS and Windows; PyPy and free-threaded builds are not claimed |
| CI | `tests-python.yml`, `bench.yml` and `bench-label.yml`, `publish-python.yml` (self-contained: PyPI takes no reusable workflow as a trusted publisher) |

Commands, run from `python/`:

| Command | What it does |
|---------|--------------|
| `uv sync --locked --extra intl` | install from the lockfile |
| `uv run --no-project tasks.py test` | build, lint, typecheck the source and the built wheel, then the suite against the source and against the wheel — what CI's legs with Intl run |
| `uv run --no-project tasks.py test-core`, `... test-lowest` | build, then the same suite without the `intl` extra, or over Babel 2.14.0 — the other legs of CI |
| `uv run --no-project tasks.py test-sdist` | build, then the sdist's own `test` in the tree the sdist unpacks to, as a downstream build runs it — what CI's Pack job runs |
| `uv run --no-project tasks.py lint`, `... typecheck` | ruff, then mypy — what CI's Lint job and the pre-commit hook run |
| `uv run --no-project tasks.py lint-fix` | fix what the formatting contract reports |
| `uv run --no-project tasks.py bench-venv`, then `node bench/harness.mjs` | build, then the benchmark (`bench/rows.mjs`) over the wheel in `.bench-venv`; `sh bench/prepare.sh` is the first half |
| `node bench/harness.mjs --compare <dir>` | the same, beside the package checked out and prepared at `<dir>` — what `bench.yml` runs on a PR |

Map of `python/`:

| Path | Role |
|------|------|
| `src/curly_message/__init__.py` | the public surface: `create_parser`, `cst`, `levels`, `UNDEFINED`, the limits and the types |
| `src/curly_message/_resolve.py` | `create_parser` and the resolution walk, on an explicit stack, with its limits and reports |
| `src/curly_message/_cst.py` | `cst`: the tree a message is described by, its spans in code points |
| `src/curly_message/_modifiers.py` | the modifiers the format defines, and the detection of the Intl supplier |
| `src/curly_message/_intl/` | the Intl modifiers over Babel: validation, digits, numbers, plurals, dates, relative time, locales |
| `src/curly_message/_convert.py` | the value model of section 4: a value as text and as a number, and the wrappers of section 4.1 |
| `src/curly_message/_utils.py` | pure helpers, the placeholder scanner, `LINE_TERM` and `own_value` among them |
| `src/curly_message/_types.py` | the public types, the limits and their doc comments |
| `src/curly_message/_reads.py` | how a read of an option's value or of a default renders and what it counts, `MAX_REENTRY` and the state a thread has in progress |
| `src/curly_message/_requests.py` | the private seam that records the formatting requests a resolution makes, for the conformance adapter |
| `tests/specs/` | the suite (see *Tests*) |
| `tests/conformance/` | the runner (`runner/`), the adapter of section 14.3 (`adapter.py`), the vendored set and `fetch.py` |
| `tests/types/surface.py` | the types the wheel ships, checked as a consumer's checker reads them |
| `tasks.py` | every command above, in the standard library alone |
| `bench/rows.mjs` | the rows of the benchmark, each read off the wheel through `bench/serve.py`; `bench/prepare.sh` builds what they read, `package.json` names the package and its version to the harness, and `bench/harness.mjs` and `bench/check/` are the family's copies |

### Architecture you must respect

The rules of `js/` hold here, with these readings:

- **The public surface** is what `curly_message/__init__.py` exports; the
  modules named with a leading underscore are private.
- **Diagnostics leave through `on_report` and nowhere else**, and resolving
  never raises: a handler that raises loses that report and nothing else.
  The one exception is a resolution host code asks for while another is in
  progress on the same call stack, which raises `ReentryLimitError` where
  `MAX_REENTRY` are in progress on its thread, and passes on the
  `RecursionError` or `MemoryError` its walk ran out in.
- **Only `dict` storage holds entries.** Reads by a key a message supplies go
  through `own_value`, through `dict`'s own lookup: a subclass's
  `__getitem__`, `get` or `__missing__` never answers, and a `ChainMap` or a
  `MappingProxyType` holds nothing. This is the prototype-key rule in Python.
- **The resolution limits** are `MAX_OUTPUT_LENGTH`, `MAX_READ_LENGTH`,
  `MAX_CONVERSION_NODES` and `MAX_NESTING` in `_types.py`, held to the same
  values and the same rules as `js/`'s.
- **`MAX_REENTRY` bounds a resolution that reaches itself**, in place of the
  interpreter's recursion limit (section 13): the resolutions in progress at
  once on one thread, counted in a `threading.local`. A resolution host code
  asks for takes a place, and so does a read of an option's value or of a
  default by any code but the modifier it was handed to while the walk is
  asking that modifier: such a read renders at the depth of the placeholder
  that wrote it, so the nesting limit never meets a chain of them. A modifier
  reading what it was handed is the walk and takes none. Hold a new way for
  host code to make the walk render to the same count. A read of what its
  own thread is already rendering is refused at once, and one of what another
  thread is rendering waits for it.
- **`MAX_REENTRY` is sized to a frame budget**, which `test_cost.py` holds:
  a read in progress stands three frames on the interpreter's stack (the
  modifier, `_reads.read` with the loop of `drive` written into it, and the
  walk's `_tail`), so the deepest chain the two bounds allow fits the 600
  frames a default interpreter has left under a caller 400 deep. A frame
  added to a read, or a raise of `MAX_REENTRY` or `MAX_NESTING`, breaks that:
  a modifier the format defines reads its options in its own frame, never
  through a helper. A modifier the format defines touches no per-thread
  state, and a message with no placeholder takes no place.
- **A props layer is enumerated once per resolution** for each modifier name
  and wrapper, and the composition answers every later placeholder: what
  composing costs grows with the layers, never with the placeholders times
  their width. Every modifier, the format's or the host's, is handed that
  one composition read-only (a `MappingProxyType`), with no copy per
  placeholder; a wrapper's own props are laid over the layers beneath it as a
  `ChainMap`, never copied with them, and the record ends with the
  resolution. The request an Intl modifier reads off that composition, its
  options converted or refused, is read once and kept in the same record,
  so a long property text costs its length once, not once per placeholder.
- **A report escapes every line terminator of `LINE_TERM`**, and also every
  code point `str.splitlines` breaks on (read off the running CPython, never
  spelled out) and every lone surrogate, since a consumer may split a report
  the host's way or encode it to UTF-8.
- **Intl is detected without importing Babel** (`importlib.util.find_spec`),
  so importing the package costs nothing it does not use, and every Babel call
  runs in the high-precision decimal context of `_intl/digits.py`, handed
  exact digits.
- **The walk is an explicit stack**, never Python recursion: a message nested
  past the interpreter's recursion limit resolves the same as any other.
- **Source is ASCII**; a test holds every source, test and bench file to it.

### Tests

- `python/tests/specs/`: `test_resolve.py` for resolution, `test_cst.py` for
  the tree, `test_intl.py` for the Intl modifiers, `test_cost.py` for what a
  resolution costs by count, `test_package.py` for the package's metadata and
  build, `test_ascii.py` for the source's characters.
- `python/tests/conformance/`: `test_conformance.py` runs every case of the
  set through `adapter.py`, at every level and over the tree, with the cases
  left out where the leg claims no Intl and the defect audit;
  `test_runner.py` tests the runner on toy adapters.
- `python/tests/types/surface.py`: the types the wheel ships, checked by mypy
  against the installed wheel before the suite runs on it.
- Every suite runs twice, against the source and against the built wheel
  (`CURLY_MESSAGE_DIST=1`); `CURLY_MESSAGE_EXPECT_INTL` makes a leg fail where
  Intl is not what it expects.
- What a message resolves to is the conformance set's to pin, as for `js/`.

## php/

Tech stack of `php/` — **ground truth, do not assume otherwise**:

| | |
|---|---|
| Package | `curly-message/parser` for Composer, released as a release candidate, `1.0.0-RC.0`; served from `curly-message/parser-php`, which only `publish-php.yml` writes to |
| Language | PHP with `declare(strict_types=1)`, namespace `CurlyMessage\` (PSR-4 over `src/`) |
| Package manager | Composer: `composer.lock` for the package and its suite, and `tools/qa/composer.lock` for php-cs-fixer and PHPStan |
| Build | none: what a release ships is `git archive` of `php/` under its `.gitattributes`, which `composer test:dist` installs by copy and tests |
| Tests | PHPUnit 11; the conformance set fetched by `composer fetch-set` at the version and SHA-256 `conformance/set.json` pins, run through the runner in `conformance/` |
| Lint | php-cs-fixer and `tools/check-source.php` (`composer lint`), run by `.githooks/php.sh` and the Lint job of `tests-php.yml`; PHPStan at level max (`composer analyse`) |
| Runtime dependencies | none; ext-intl is suggested, and satisfies the Intl level |
| Supported runtimes | PHP 8.2 to 8.5 on Linux, macOS and Windows, with ext-intl and without; thread-safe (ZTS) builds are not tested |
| CI | `tests-php.yml`, `bench.yml` and `bench-label.yml` (PHP from `.php-version`), `publish-php.yml` (tags `php-vX.Y.Z` here and pushes the split of `php/` to `curly-message/parser-php`, tagged `vX.Y.Z`) |

Commands, run from `php/`:

| Command | What it does |
|---------|--------------|
| `composer install` | install from the lockfiles, `tools/qa/`'s included |
| `composer test` | fetch the set, lint, analyse, run the suite, then `composer test:dist` — what CI's Tests, Lint and Artifact jobs run between them, but for the Lint job's check of the case table (`composer unicode -- --check`) |
| `composer test:dist` | build the archive a release ships, install it into a scratch project by copy, and run the unit and conformance suites, the surface probe and a `php -n` smoke run against it — what CI's Artifact job runs |
| `composer lint`, `composer analyse` | the formatting contract and the source checks, then PHPStan — what CI's Lint job runs, beside the check of the case table; the pre-commit hook runs the first |
| `composer lint:fix` | fix what the formatting contract reports |
| `composer unicode` | regenerate the case table from the pinned Unicode data; `composer unicode -- --check` fetches that data and fails where the table differs from what it generates, as CI's Lint job runs it |
| `composer bench` | the benchmark (`bench/rows.mjs`), on Node 22 or later; `sh bench/prepare.sh` prepares a side |
| `composer bench -- --compare <dir>` | the same, beside the package checked out and prepared at `<dir>` — what `bench.yml` runs on a PR |
| `php tools/memory.php` | what ends a process at 128M in each shape of the README's Memory table; `php tools/memory.php <limit>` at another |

Map of `php/`:

| Path | Role |
|------|------|
| `src/Parser.php` | `Parser`: its options, `resolve`, `levels`, `unexpressible`, and the limits |
| `src/Cst.php`, `src/Cst/` | `Cst::parse`: the tree a message is described by, its spans in bytes |
| `src/ModifierInput.php`, `src/Option.php`, `src/Props.php` | what a host modifier is handed, its props read-only |
| `src/Report.php`, `src/ReportCode.php`, `src/Origin.php` | what `onReport` is handed |
| `src/ReentryLimit.php`, `src/Undefined.php` | the one throw, and the host's undefined |
| `src/Internal/Resolution.php` | one resolution: the walk, its budgets and the texts host code asks for |
| `src/Internal/Conversion.php` | a value as text (section 4), the JSON written and bounded here |
| `src/Internal/Layers.php` | reads of the payload, wrappers and props layers, by own entries only |
| `src/Internal/Scanner.php`, `src/Internal/Text.php` | the placeholder scanner, `LINE_TERM` and the pure text helpers |
| `src/Internal/Guard.php`, `src/Internal/Contained.php` | the re-entry bound, and the containment of host code |
| `src/Internal/Comparisons.php`, `src/Internal/Intl/` | the format's comparisons, and its Intl modifiers over ext-intl |
| `src/Internal/Excerpt.php` | the escaped text a report carries |
| `conformance/` | the runner of the set and the audit of its defects, and the set's pin |
| `tests/Unit/`, `tests/Conformance/`, `tests/Runner/`, `tests/Types/surface.php`, `tests/Scripts/` | the suite (see *Tests*) |
| `tools/` | the archive test, the `php -n` smoke run, the source checks, the case table's generator, the memory probe, and `qa/`'s tools |
| `bench/rows.mjs` | the rows of the benchmark, each run by `bench/worker.php` on the package's own autoloader; `bench/harness.mjs` and `bench/check/` are the family's copies |

### Architecture you must respect

The rules of `js/` hold here, with these readings:

- **The public surface** is every class under `CurlyMessage\` outside
  `CurlyMessage\Internal\`, whose classes are `@internal`.
- **Diagnostics leave through `onReport` and nowhere else.**
- **Reads go through `Layers::own`**: a container is a PHP array or a
  `stdClass`, read by its own entries alone, so an `ArrayAccess` or a
  `__get` never answers. This is the prototype-key rule in PHP.
- **Props are composed once and shared.** `Layers::props` composes the
  layers every placeholder shares once for each modifier name and hands them
  out as a read-only `Props`, a wrapper's own laid over them uncopied.
  Nothing a resolution keeps for a placeholder or a key may grow with what a
  shared layer holds, and a modifier of the format reads its props by name,
  never by walking them.
- **The resolution limits** are `Parser::MAX_OUTPUT_LENGTH`,
  `MAX_READ_LENGTH`, `MAX_CONVERSION_NODES` and `MAX_NESTING`, held to the
  same values and the same rules as `js/`'s, counted in bytes. Two bounds of
  PHP's own stand beside them, as load-bearing: `MAX_TEXT_LENGTH`, the
  longest text the parser writes; and `MAX_REENTRY`, the resolutions in
  progress on one thread of execution, a fiber being one, where an option or
  a fallback chain host code reads outside the call it was handed to takes a
  place while it renders. PHP ends a recursion at its memory limit with a
  fatal error no `catch` sees, so `MAX_REENTRY` stands in for the host's
  limit of section 13, and, as it asks, never refuses a resolution begun
  where none is in progress on its thread. No bound counts across threads of
  execution: a recursion that spreads across fibers is the host's to bound,
  and one it does not bound ends at the memory limit.
- **`ReentryLimit` is all `resolve` throws**, past the re-entry bound:
  everything host code runs where the parser calls it — a modifier, a
  handler, `__toString`, `jsonSerialize`, a `__destruct` as the parser lets
  go of what it holds — runs inside `Contained`. A refused read of an option
  or a fallback chain throws nothing: it answers the empty string and fails
  nothing, as section 13 asks (`Option::value`, `ModifierInput::default`).
  What a `__destruct` PHP's cycle collector runs throws is the one exception:
  it leaves wherever the collector ran.
- **A report escapes every line terminator of `Text::LINE_TERM`**, the list
  the scanner reads.
- **Source is ASCII**, and the library calls nothing an optional extension or
  a dev dependency defines outside the Intl level: `tools/check-source.php`
  holds the source to both, and the `php -n` smoke run of `composer test:dist`
  checks the second on the archive.

### Tests

- `php/tests/Unit/`: resolution (`ParserTest.php`), host modifiers
  (`ModifiersTest.php`), re-entry (`ReentryTest.php`), the limits, the
  conversion of values, the tree (`CstTest.php`), the Intl modifiers with
  ext-intl and without, and `Internal/` for the pure helpers. What has to run
  in a process of its own, at a memory limit of its own, is a script in
  `tests/Scripts/`.
- `php/tests/Conformance/`: every case of the set through `adapter.php`,
  and the audit of the runner against the set's defects; `tests/Runner/`
  tests the runner itself.
- `php/tests/Types/surface.php`: the public surface as a consumer's analysis
  reads it, analysed by PHPStan against the source and against the archive's
  copy.
- The unit and conformance suites run twice: against the source, and through
  `composer test:dist` against the archive a release ships.
- What a message resolves to is the conformance set's to pin, as for `js/`.
