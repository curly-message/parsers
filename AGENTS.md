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

- **One directory per language** — `js/` and `rust/` today. Each is a fully
  standalone package with its own build, tests, configs, README, CHANGELOG
  and version line. There is no root package manifest and no workspace: run
  every command from inside the implementation directory, for every
  directory a change touches.
- **Release tags are namespaced by directory** — `js-v1.0.0`, `py-v0.9.0` — so
  moving an implementation to its own repository later is not a breaking event.
  The publish workflows cut them (`.github/workflows/publish-js.yml`,
  `publish-rust.yml`; the README's Releasing section says what they do).
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
  its script, `.githooks/js.sh` for `js/` and `.githooks/rust.sh` for
  `rust/`.
- **`bench.yml` reads its packages from the tree**: each directory that holds
  a `bench/rows.mjs` outside a `bench/check/`, with its own copy of
  `bench/harness.mjs` and `bench/check/`. On a PR it benchmarks those with a
  changed file that is neither a `*.md` nor under the package's `tests/`, or
  all of them where `bench.yml` itself changed. It sets up Node, which the
  harness runs on, and a toolchain per manifest the package holds: uv for a
  `pyproject.toml`, Go from a `go.mod`, PHP from the `.php-version` beside a
  `composer.json` (with `intl` and `opcache`), and the runner's rustup, which
  reads `rust-toolchain.toml`, for a `Cargo.toml`. Each side is prepared from
  the package's directory, by `sh bench/prepare.sh` where the package has
  one, and by `npm ci --ignore-scripts && npm run build` otherwise. A base
  that does not hold the package is not prepared, and the harness compares
  nothing against it.
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
`MAX_READ_LENGTH`, `MAX_CONVERSION_NODES` and `MAX_NESTING` are what keep the
cost of a resolution bounded by the message rather than by the payload: a
value no conversion terminates on, a message nested past what a host will
walk, and a payload handing back more text than a caller can hold each stop at
one of them. Don't remove them, don't raise them to buy behavior, and treat a
change to any of them as a format change.

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

## rust/

Tech stack of `rust/` — **ground truth, do not assume otherwise**:

| | |
|---|---|
| Crate | `curly-message-parser`, library `curly_message_parser`, unreleased; the first version, `1.0.0-next.0`, is published by hand |
| Language | Rust, edition 2024, MSRV 1.88 |
| Package manager | cargo, one workspace rooted at `rust/` (the parser, `conformance/`, `bench/`), `Cargo.lock` committed and every command `--locked` |
| Build | `cargo build`; `cargo package` builds the `.crate` from the files it ships |
| Tests | `cargo test`, with and without the `intl` feature; the conformance set vendored and checked by its sha256 (`conformance/set/`) |
| Lint | `rustfmt` (`rustfmt.toml`) and clippy at `-D warnings` under the `[workspace.lints]` table of `rust/Cargo.toml`, both run by `tests-cargo.yml`'s Lint job and by `.githooks/rust.sh` |
| Runtime dependencies | none; the `intl` feature brings ICU4X 2.3, `icu_experimental` 0.6 and `jiff` 0.2 |
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
| `src/depth.rs` | containment of host code and the per-thread count of resolutions in progress |
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
  `MAX_SERIALIZED_LENGTH`, the most one serialization writes, in code units
  of UTF-16 as V8 counts a string, and `MAX_REENTRY`. Each is
  load-bearing.
- **The walk keeps its own stack.** A message, a value and a tree of any
  depth are walked, converted and dropped without a call per level. The
  host's stack is reached only through host code (a modifier, an `Intl`
  facility, a `Display`, a report handler), every call of which goes through
  `depth::host_call`, contained, with no borrow of the walker's state held
  across it.
- **`MAX_REENTRY` counts resolutions, and its cost is measured.** It
  bounds the resolutions in progress on one thread, across parsers, and
  nothing of one resolution's walk: the nesting limit bounds what a walk
  puts on the thread's stack, the options and defaults its modifiers read,
  so no resolution is cut short by the ones around it. A refusal unwinds
  with `ReentryLimit` from `Parser::resolve`, outside its backstop, so it
  reaches the host code that asked, and `std::panic::resume_unwind` keeps
  the panic hook out of it; one refused while the thread already unwinds
  answers the empty string instead, since an unwind leaving a `Drop` would
  abort the process, and `depth::contained` fails the host call that asked
  for it all the same. Its worst cases, `MAX_REENTRY` resolutions each as
  deep into its options as a placeholder resolves, one of them for each
  request of the Intl facility, run on a 512 KiB thread in a debug build on
  every OS leg; the one whose innermost formats a date in a zone name the
  process has not used yet runs on a 1 MiB thread, in a binary of its own,
  since jiff reads a zone's rules the first time the process formats in each
  of its names, an alias being a name of its own, and each read costs a
  debug build about 230 KiB. A change that grows what
  a resolution costs the stack, or raises the bound, is checked there.
  Stack exhaustion aborts the process rather than panicking, so this bound
  is the only containment there is. The count, and the flag
  `depth::contained` reads for a refusal made while the thread unwinds, are
  `thread_local!`s, and whether the thread unwinds is
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
  the golden files of Node's answers are written by their `golden.mjs`, never
  by hand.
- **Docs** are `rust/README.md` (the crates.io page, its examples run as
  doc tests), `rust/CHANGELOG.md` and the doc comments of the public items.

### Tests

- `rust/tests/`: `resolve.rs` for resolution, `modifiers.rs` for the
  Extensions level, `limits.rs` for the limits and the stack-independence
  tests, `panics.rs` (its own binary, with a silent panic hook) for
  containment, the refusal past `MAX_REENTRY` and its worst case, `cost.rs`
  and `allocations.rs` for cost by count, `cst.rs` for the tree, `value.rs`
  for the value model, `fuzz.rs` for messages written at random, `intl.rs`
  (with the facility's worst cases of `MAX_REENTRY`), `intl_supplier.rs`
  and `intl_parity.rs` for the Intl level, `intl_stack.rs` (its own binary,
  so no zone is used before it) for its worst case in a zone name, and
  `conformance.rs` for the set and the audit.
- With `intl`, the set and the audit run twice: on the requests the parser
  exposes, and on the text, against Node's answers recorded in
  `conformance/set/golden-4.1.0.json`. `CURLY_EXPECT_INTL` holds a leg to the
  level it means to test.
- The Packaged job of `tests-cargo.yml` tests the `.crate` a release ships:
  its own tests from the unpacked archive, and `tests/` in a project that
  depends on it. This stands in for `--mode dist`.
- Drive behavior through the public API; a table of pure helpers (the number
  conversions) is a unit test inside the crate. Cost is asserted by count,
  with a counting allocator and call counters, never by time, and no test
  assumes more stack than the thread it spawns.
