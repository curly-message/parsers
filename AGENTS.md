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

- **One directory per language** — `js/` and `python/` today. Each is a fully
  standalone package with its own build, tests, configs, README, CHANGELOG
  and version line. There is no root package manifest and no workspace: run
  every command from inside the implementation directory, for every
  directory a change touches.
- **Release tags are namespaced by directory** — `js-v1.0.0`, `py-v0.9.0` — so
  moving an implementation to its own repository later is not a breaking event.
  The publish workflows cut them (`.github/workflows/publish-js.yml`,
  `publish-python.yml`; the README's Releasing section says what they do).
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
  its script, `.githooks/js.sh` for `js/` and `.githooks/python.sh` for
  `python/`.
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

## python/

Tech stack of `python/` — **ground truth, do not assume otherwise**:

| | |
|---|---|
| Package | `curly-message` on PyPI, imported as `curly_message`, unreleased; the first version, `1.0.0rc0`, is published by the workflow through a pending trusted publisher |
| Language | Python, typed (`py.typed`) |
| Package manager | uv, one lockfile (`uv.lock`); the commands are `tasks.py`'s |
| Build | hatchling, pinned by hash in `build-constraints.txt`: an sdist and a pure wheel |
| Tests | pytest; the conformance set vendored as `tests/conformance/conformance-4.1.0.zip`, pinned by its digest |
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
- **Only `dict` storage holds entries.** Reads by a key a message supplies go
  through `own_value`, through `dict`'s own lookup: a subclass's
  `__getitem__`, `get` or `__missing__` never answers, and a `ChainMap` or a
  `MappingProxyType` holds nothing. This is the prototype-key rule in Python.
- **The resolution limits** are `MAX_OUTPUT_LENGTH`, `MAX_READ_LENGTH`,
  `MAX_CONVERSION_NODES` and `MAX_NESTING` in `_types.py`, held to the same
  values and the same rules as `js/`'s.
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
