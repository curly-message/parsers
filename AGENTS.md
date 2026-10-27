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

- **One directory per language** — `js/` and `php/` today. Each is a fully
  standalone package with its own build, tests, configs, README, CHANGELOG
  and version line. There is no root package manifest and no workspace: run
  every command from inside the implementation directory, for every
  directory a change touches.
- **Release tags are namespaced by directory** — `js-v1.0.0`, `php-v1.0.0` —
  so moving an implementation to its own repository later is not a breaking
  event. The publish workflows cut them (`.github/workflows/publish-js.yml`,
  `publish-php.yml`; the README's Releasing section says what they do).
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
  its script, `.githooks/js.sh` for `js/` and `.githooks/php.sh` for `php/`.
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

## php/

Tech stack of `php/` — **ground truth, do not assume otherwise**:

| | |
|---|---|
| Package | `curly-message/parser` for Composer, unreleased; how it reaches Packagist waits on curly-message/spec#109 |
| Language | PHP with `declare(strict_types=1)`, namespace `CurlyMessage\` (PSR-4 over `src/`) |
| Package manager | Composer: `composer.lock` for the package and its suite, and `tools/qa/composer.lock` for php-cs-fixer and PHPStan |
| Build | none: what a release ships is `git archive` of `php/` under its `.gitattributes`, which `composer test:dist` installs by copy and tests |
| Tests | PHPUnit 11; the conformance set fetched by `composer fetch-set` at the version and SHA-256 `conformance/set.json` pins, run through the runner in `conformance/` |
| Lint | php-cs-fixer and `tools/check-source.php` (`composer lint`), run by `.githooks/php.sh` and the Lint job of `tests-php.yml`; PHPStan at level max (`composer analyse`) |
| Runtime dependencies | none; ext-intl is suggested, and satisfies the Intl level |
| Supported runtimes | PHP 8.2 to 8.5 on Linux, macOS and Windows, with ext-intl and without; thread-safe (ZTS) builds are not tested |
| CI | `tests-php.yml`, `bench.yml` and `bench-label.yml` (PHP from `.php-version`), `publish-php.yml` |

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
| `src/ModifierInput.php`, `src/Option.php` | what a host modifier is handed |
| `src/Report.php`, `src/ReportCode.php`, `src/FormatRequest.php`, `src/Facility.php`, `src/Origin.php` | what `onReport` and `onRequest` are handed |
| `src/ReentryLimit.php`, `src/Undefined.php` | the one throw, and the host's undefined |
| `src/Internal/Resolution.php` | one resolution: the walk, its budgets and the texts host code asks for |
| `src/Internal/Conversion.php` | a value as text (section 4), the JSON written and bounded here |
| `src/Internal/Layers.php` | reads of the payload, wrappers and props layers, by own entries only |
| `src/Internal/Scanner.php`, `src/Internal/Text.php` | the placeholder scanner, `LINE_TERM` and the pure text helpers |
| `src/Internal/Guard.php`, `src/Internal/Contained.php` | the re-entry bounds, and the containment of host code |
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
- **The resolution limits** are `Parser::MAX_OUTPUT_LENGTH`,
  `MAX_READ_LENGTH`, `MAX_CONVERSION_NODES` and `MAX_NESTING`, held to the
  same values and the same rules as `js/`'s, counted in bytes. Three bounds of
  PHP's own stand beside them, as load-bearing: `MAX_TEXT_LENGTH`, the
  longest text the parser writes; `MAX_REENTRY`, the resolutions in progress
  on one thread of execution, a fiber being one, where an option or a
  fallback chain host code asks for takes no place; and `MAX_IN_PROGRESS`, the
  resolutions in progress in the process that were each begun beneath
  another on their thread. PHP ends a recursion at its memory limit with a
  fatal error no `catch` sees, so the last two stand in for the host's limit
  of section 13, and, as it asks, neither refuses a resolution begun where
  none is in progress on its thread.
- **`ReentryLimit` is all `resolve` throws**, past either re-entry bound:
  everything host code runs where the parser calls it — a modifier, a
  handler, `__toString`, `jsonSerialize`, a `__destruct` as the parser lets
  go of what it holds — runs inside `Contained`, and a refused read throws
  into the host code that asked. What a `__destruct` PHP's cycle collector
  runs throws is the one exception: it leaves wherever the collector ran.
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
