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

- **One directory per language** — `js/` today. Each is a fully standalone
  package with its own build, tests, configs, README, CHANGELOG and version
  line. There is no root package manifest and no workspace: run every command
  from inside the implementation directory, for every directory a change
  touches.
- **Release tags are namespaced by directory** — `js-v1.0.0`, `py-v0.9.0` — so
  moving an implementation to its own repository later is not a breaking event.
  The publish workflow cuts them (`.github/workflows/publish-js.yml`; the
  README's Releasing section says what it does).
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
  its script, `.githooks/js.sh` for `js/`.
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
