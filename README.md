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
same version and is not yet released.

## Implementations

| Path | Package | State |
| --- | --- | --- |
| [`js/`](./js) | `@curly-message/parser` | Stable, `latest` |
| [`go/`](./go) | `github.com/curly-message/parsers/go` | Unreleased |

One directory per language, each a standalone package with its own build,
tests and version line. Release tags are namespaced by directory: `js-v1.0.0`
for a package a registry addresses by name, so it can leave for its own
repository without that being a breaking change for the others; and
`go/v1.0.0` for a Go module, whose version the go command reads only from a
tag named after the module's directory. A Go module's path is where it
lives, so moving one changes what every importer writes.

## Pre-commit hook

`.githooks/pre-commit` runs the check of each implementation directory a
commit touches, and skips a directory whose tools are missing, saying which
and how to install them, rather than failing the commit. `npm ci` in `js/`
installs it; without Node, `git config core.hooksPath .githooks` does. Git
then runs every hook from `.githooks/`, and none from `.git/hooks/`.

The check of a directory is `.githooks/<dir>.sh`: `npm run lint` for `js/`,
and for `go/` the formatting, vet and ASCII checks of the Lint job of
`tests-go.yml`.

## Benchmarks

Each implementation directory has a benchmark, run by the command its README
names (`npm run bench` in `js/`, `sh bench/prepare.sh && node
bench/harness.mjs` in `go/`), that measures what its build costs: counts, such
as what checking a call costs a type checker, sizes, and times. A PR is
benchmarked against its base by `bench.yml`, for each implementation it
touches, which posts a table per implementation on the PR (a PR from a fork
finds them in the job's summary). A count that grew, a row gone missing or
changing kind, or a row that fails on the base fails the check unless the PR
carries the `bench-accepted` label; adding or removing the label runs it
again. A base that does not hold the implementation, as on the PR that adds
it, compares nothing and fails nothing.

## Releasing

Each implementation is released from `main` by a workflow of its own, with a
tag namespaced by its directory.

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
to the build's and publishes a GitHub release. There is no registry token to
hold and no first version to publish by hand: the workflow cuts the first
prerelease as it cuts every later version.

## Specification

The format is defined in [`curly-message/spec`](https://github.com/curly-message/spec).
Its machine-readable identifier is `curly-message`; versioned references use
`curly-message-3`, and so on.

Issues for this repository are filed in the family's shared tracker,
[`curly-message/spec`](https://github.com/curly-message/spec/issues).

## License

[MIT](./LICENSE)
