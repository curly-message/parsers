# Parsers

Official implementations of the [Curly Message Format](https://github.com/curly-message/spec)
— a small message syntax for software translations, in which values are
interpolated through double-curly placeholders that may carry a modifier, a set
of options and a fallback.

```json
{
  "greeting": "Hello, {{name; default:Guest;}}!",
  "inbox": "You have {{count:number;}} {{count; 1:message; default:messages;}}."
}
```

## Status

**Stable.** The JavaScript implementation is on npm as
`@curly-message/parser`, and implements `curly-message-3` — version 3 of the
format, which the specification states is stable.

It resolves over the format's own inputs and depends on no host library.

The Go implementation, `github.com/curly-message/parsers/go`, implements the
same version and is released as a prerelease, `v1.0.0-next.0`.

The Rust implementation, `curly-message-parser`, implements the same version
and is on crates.io as a prerelease, `1.0.0-next.0`.

The Python implementation, `curly-message` on PyPI, implements the same
version and is released as a release candidate, `1.0.0rc0`.

The PHP implementation, `curly-message/parser` for Composer, implements the
same version and is released as a release candidate, `1.0.0-RC.0`. It is
developed in `php/` and served
to Composer from
[`curly-message/parser-php`](https://github.com/curly-message/parser-php),
which only its publish workflow writes to (see *Releasing*).

## Implementations

| Path | Package | State |
| --- | --- | --- |
| [`js/`](./js) | `@curly-message/parser` | Stable, `latest` |
| [`go/`](./go) | `github.com/curly-message/parsers/go` | Prerelease, `v1.0.0-next.0` |
| [`rust/`](./rust) | `curly-message-parser` | Prerelease, `1.0.0-next.0` |
| [`python/`](./python) | `curly-message` (PyPI) | Release candidate, `1.0.0rc0` |
| [`php/`](./php) | `curly-message/parser` (Packagist) | Release candidate, `1.0.0-RC.0` |

One directory per language, each a standalone package with its own build,
tests and version line. Release tags are namespaced by directory: `js-v1.0.0`,
`rust-v1.0.0`, `python-v1.0.0` or `php-v1.0.0` for a package a registry addresses by name,
so it can leave for its own repository without that being a breaking change
for the others; and `go/v1.0.0` for a Go module, whose version the go command
reads only from a tag named after the module's directory. A Go module's path
is where it lives, so moving one changes what every importer writes.

## Pre-commit hook

`.githooks/pre-commit` runs the check of each implementation directory a
commit touches, and skips a directory whose tools are missing, saying which
and how to install them, rather than failing the commit. `npm ci` in `js/`
installs it; without Node, `git config core.hooksPath .githooks` does. Git
then runs every hook from `.githooks/`, and none from `.git/hooks/`.

The check of a directory is `.githooks/<dir>.sh`: `npm run lint` for `js/`,
for `go/` the formatting, vet and ASCII checks of the Lint job of
`tests-go.yml`, for `rust/` the format, clippy and harness checks of the Lint
job of `tests-cargo.yml`, for `python/` the ruff and mypy checks of the Lint
job of `tests-python.yml`, and for `php/` `composer lint`, the formatting and
source checks of the Lint job of `tests-php.yml`.

## Benchmarks

Each implementation directory has a benchmark, run by the command its README
names (`npm run bench` in `js/`, `sh bench/prepare.sh && node
bench/harness.mjs` in `go/` and in `rust/`, `uv run --no-project tasks.py
bench-venv && node bench/harness.mjs` in `python/`, `composer bench` in
`php/`), that measures what its
build costs: counts, such as what checking a call costs a type checker, sizes,
and times. A PR is benchmarked against its base by `bench.yml`, for each
implementation it
touches, which posts a table per implementation on the PR (a PR from a fork
finds them in the job's summary). An implementation is benchmarked where the
branch or the base has its benchmark, so a PR that drops the benchmark of an
implementation it changes fails. A count that grew, a row gone missing or
changing kind, or a row that fails on the base fails the check unless the PR
carries the `bench-accepted` label; adding or removing the label runs it
again. The base is compared only where its commit has the implementation's
benchmark; where it has none, as on the PR that adds the implementation or its
benchmark, or one that renames its directory, nothing is compared, and only the
branch's own benchmark can fail the check.

## Releasing

Each implementation is released from `main` by a workflow of its own, with a
tag namespaced by its directory.

A release of the JavaScript, Go, Rust or Python parser that stops after its
tag exists is finished by **Re-run failed jobs** on its run, before any
further release of that implementation and within 30 days of the run or the
build artifact's retention, whichever ends first: the re-run reuses the run's
version and build, makes the same commit, finds it tagged, pushes nothing, and
publishes to the registry and to GitHub what is not there yet. Re-running all
jobs does not finish it: the Go workflow refuses a version already tagged
before it builds, and in the others a new build writes another benchmark than
the tag holds. A release that stops before its tag exists is dispatched again,
and cuts the same version. A publish that fails saying the version is
published already, right after the registry took it, is re-run once more. A
release that cannot be finished this way — a defect in the workflow's own
steps after the push, or a run past that window — stays as far as it got, its
tag and whatever the registry took, and the next version is released. The PHP
workflow finishes a stopped release through its `mirror` input instead
(below).

### JavaScript

A release is cut from `main` by the **JavaScript parser publish** workflow
(`.github/workflows/publish-js.yml`, started by hand). The changelog's pending
section names the version it will be released as — `### 1.1.0 (Unreleased)` —
so a version is settled in the commit that writes the section rather than in
the dispatch. `patch`, `minor` and `major` cut that section under the `latest`
dist-tag, and the run is refused unless the bump arrives at the version the
section names — over an open prerelease line that is the bump which drops the
prerelease rather than the one that opened it, so `1.1.0-next.3` reaches
`1.1.0` under `patch`. `next` publishes a prerelease of that same version —
`1.1.0-next.0`, then `.1` — under the `next` dist-tag and leaves the section
open, because a prerelease has not released what the section names. The
workflow runs the test matrix, bumps the version, cuts the section where the
release closes it, writes the benchmark of the build into `BENCH.md`, commits,
tags (`js-v1.0.0`), pushes, publishes to npm, and publishes a GitHub release
carrying that changelog section.

After the tests, the release runs in three jobs, so that no code a dependency
ships runs where the release's credentials are: one settles the version, one
installs, builds and benchmarks with no credential that can push or publish,
and the last takes that build as files. Of the three, only the second runs a
dependency's code, and only the last holds the credentials, in the `release`
environment.

The commit, the tag and the release are made as a GitHub App, whose client
ID and private key the repository holds as the `APP_CLIENT_ID` variable and
the `APP_PRIVATE_KEY` secret; kept in the `release` environment alone, which
only `main` can deploy to, the key is out of every other job's reach. npm
holds no token: the workflow is the package's
[trusted publisher](https://docs.npmjs.com/trusted-publishers), registered,
with `release` as its environment, in the package's settings on npmjs.com or
with
`npm trust github --file publish-js.yml --repository curly-message/parsers --environment release --allow-publish`
— the calling workflow's filename, which is the one the registry checks — and
the registry attaches provenance itself. A trusted publisher can be
registered only for a package that exists, so the first version,
`1.0.0-next.0`, was published by hand from `main` by a maintainer of the scope
(`cd js && npm ci && npm publish --access public --tag next`); the workflow
refuses to run for a package the registry does not know.

### Go

A Go module has no registry: its version is the tag `go/vX.Y.Z`, which the
module proxy and the checksum database record, with the hash of what it
holds, the first time anyone asks for it. A version cannot be withdrawn
once recorded; a later version retracts it. The **Go parser release**
workflow (`.github/workflows/publish-go.yml`, started by hand with the
module directory and a release type) reads the changelog's pending section
and the release types as the JavaScript workflow does. After the tests, a
job with no credential benchmarks the module into `BENCH.md`, makes the
release commit locally and computes, from that commit, the `h1:` hash the
proxy will record. The release job, in the `release` environment, makes the
same commit as the GitHub App and pushes it with its tag only when its tree
is the one the build hashed. It then holds the proxy's hash for the version
to the build's and publishes a GitHub release carrying the changelog
section, never marked as the repository's latest, which stays the
JavaScript parser's. There is no registry token to hold and no first
version to publish by hand: the workflow cuts the first prerelease as it
cuts every later version.

### Rust

The Rust parser is released by the **Rust parser publish** workflow
(`.github/workflows/publish-rust.yml`, started by hand, calling
`publish-cargo.yml`), reading the changelog's pending section and the release
types as the JavaScript workflow does. crates.io has no dist-tags: `next`
publishes a prerelease version (`1.1.0-next.0`, then `.1`), which Cargo
resolves only for a requirement that names a prerelease, as npm installs a
`next` version only when asked for it. The workflow runs the test matrix,
bumps `version` in `rust/Cargo.toml` and the crate's entry in
`rust/Cargo.lock`, writes the benchmark into `BENCH.md`, verifies the package
(`cargo package` builds the crate from the files it ships), commits, tags
(`rust-v1.0.0`), pushes, publishes to crates.io and publishes a GitHub
release carrying the changelog section, never marked as the repository's
latest, which stays the JavaScript parser's. It runs in the same three jobs, and
the release job publishes only a crate whose files hash as the verified one's
did.

crates.io holds no token: the workflow is the crate's
[trusted publisher](https://rust-lang.github.io/rfcs/3691-trusted-publishing-cratesio.html),
registered in the crate's settings on crates.io (repository
`curly-message/parsers`, workflow `publish-rust.yml`, environment
`release`), and `rust-lang/crates-io-auth-action` exchanges the job's OIDC
token for a token it revokes when the job ends. A trusted publisher can be registered only for
a crate that exists, so the first version, `1.0.0-next.0`, was published by
hand from `main` by an owner with an API token
(`cd rust && cargo publish --locked -p curly-message-parser`); the workflow
refuses to run for a crate crates.io does not know.

### Python

The Python parser is released by the **Python parser publish** workflow
(`.github/workflows/publish-python.yml`, started by hand), reading the
changelog's pending section and the release types as the JavaScript workflow
does: `patch`, `minor` and `major` cut `### 1.1.0 (Unreleased)` and must
arrive at the version it names, and `next` publishes a release candidate of
that version — `1.1.0rc0`, then `rc1`, PEP 440's spelling — and leaves the
section open. PyPI has no dist-tags: a candidate is a prerelease, which `pip`
installs only when asked (`pip install --pre curly-message`, or the exact
version). The workflow runs the test matrix (`tests-python.yml`), writes the
version into `python/pyproject.toml`, `python/uv.lock` and the benchmark's
`python/package.json`, builds the sdist and the wheel with the hash-pinned
build backend, writes the benchmark of that wheel into `BENCH.md`, commits,
tags (`python-v1.0.0`), pushes, publishes to PyPI with an attestation of each
file, and publishes a GitHub release carrying the changelog section, never
marked as the repository's latest, which stays the JavaScript parser's. It runs
in the same three jobs, with the same App, written out in the workflow rather
than called from `publish.yml`, because PyPI does not accept a reusable
workflow as a trusted publisher.

PyPI holds no token: the workflow is the project's
[trusted publisher](https://docs.pypi.org/trusted-publishers/), with
`release` as its environment. A trusted publisher can be registered before
the project exists, as a *pending publisher* in the PyPI account of a
maintainer, naming the project `curly-message`, the repository
`curly-message/parsers`, the workflow `publish-python.yml` and the
environment `release`; the first release, `1.0.0rc0` under `next`, then
created the project, so no version was published by hand. Before anything is
pushed, the release job checks that PyPI does not hold the version, unless
origin holds its tag for a re-run to finish, and makes the OIDC exchange the
publish step will make, so a publisher that is missing or names something
else fails the run before a tag exists.

### PHP

The PHP parser is released by the **PHP parser publish** workflow
(`.github/workflows/publish-php.yml`, started by hand). It reads the
changelog's pending section and the release types as the JavaScript workflow
does: `patch`, `minor` and `major` cut `### 1.1.0 (Unreleased)` and must
arrive at the version it names, and `next` cuts a release candidate of that
version — `1.1.0-RC.0`, then `.1`, Composer's spelling — and leaves the
section open. It runs the test matrix (`tests-php.yml`), writes the version
into the benchmark's `php/package.json`, writes the benchmark into
`BENCH.md`, commits and tags `php-v1.0.0` here, in the same three jobs as the
JavaScript workflow, as the same App, from the same `release` environment.

Packagist reads a package from the root of a repository, and its versions
from tags its version parser accepts, which neither `php/` nor `php-v1.0.0`
is. So the package is developed here and served from
[`curly-message/parser-php`](https://github.com/curly-message/parser-php),
which nothing but this workflow writes to. The workflow splits the history of
`php/` off at the release's tag with `git subtree split`, over the whole
history, so the same commit always splits to the same commit and each
release's split descends from the one before it; it refuses a split whose
tree is not what `php/` holds at the tag, and pushes it to that repository's
`main`, tagged `v1.0.0`, where Packagist's GitHub hook reads it. Both
repositories get a GitHub release of their tag, carrying the changelog
section. The release here is never marked latest, which stays with the
JavaScript parser's releases. In `curly-message/parser-php`, a release is
marked latest, a backport after a later major included, and a release
candidate is not. The run ends once Packagist lists the version. No registry
credential exists: what Packagist serves is what the App pushed. The
archive Composer installs is GitHub's of that tag, so `php/.gitattributes`
decides what it holds, as it does for the archive the suite tests under
`composer test:dist`.

Nothing is pushed until both repositories have been checked and both pushes
tried dry, and this repository is pushed first; neither push is forced. A run
that stops after its tag landed here is finished by running the workflow with
`mirror` set to that tag (`php-v1.0.0`): it pushes to
`curly-message/parser-php` only what that repository lacks of the release,
refuses where it holds something else under the release's tag or a `main`
that neither holds the split nor is an ancestor of it, publishes only the
GitHub releases that are missing, and waits for Packagist again.

Packagist is told of `curly-message/parser-php` by hand, once, with its
GitHub hook, and a submission reads `composer.json` off the repository's
`main`, which the first release is the one to push. So the first run goes
ahead while that repository is empty, and the repository is submitted while
the run waits for Packagist, or before a `mirror` run that finishes it; once
the repository holds a release, a run is refused while Packagist does not know
the package.

What the workflow needs outside this repository is set by hand: the App
installed on `curly-message/parser-php` with read and write access to its
contents, and allowed past any rule that guards that repository's `main` or
its tags.

## Specification

The format is defined in [`curly-message/spec`](https://github.com/curly-message/spec).
Its machine-readable identifier is `curly-message`; versioned references use
`curly-message-3`, and so on.

Issues for this repository are filed in the family's shared tracker,
[`curly-message/spec`](https://github.com/curly-message/spec/issues).

## License

[MIT](./LICENSE)
