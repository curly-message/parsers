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

The PHP implementation, `curly-message/parser` for Composer, implements the
same version and is not yet released: how it reaches Packagist is open in
[curly-message/spec#109](https://github.com/curly-message/spec/issues/109),
and nothing is published until that is decided.

## Implementations

| Path | Package | State |
| --- | --- | --- |
| [`js/`](./js) | `@curly-message/parser` | Stable, `latest` |
| [`php/`](./php) | `curly-message/parser` (Packagist) | Unreleased |

One directory per language, each a standalone package with its own build,
tests and version line. Release tags are namespaced by directory
(`js-v1.0.0`, `php-v1.0.0`), so an implementation can leave for its own
repository without that being a breaking change for the others.

## Pre-commit hook

`.githooks/pre-commit` runs the check of each implementation directory a
commit touches, and skips a directory whose tools are missing, saying which
and how to install them, rather than failing the commit. `npm ci` in `js/`
installs it; without Node, `git config core.hooksPath .githooks` does. Git
then runs every hook from `.githooks/`, and none from `.git/hooks/`.

The check of a directory is `.githooks/<dir>.sh`: `npm run lint` for `js/`,
and for `php/` `composer lint`, the formatting and source checks of the Lint
job of `tests-php.yml`.

## Benchmarks

Each implementation directory has a benchmark, run by the command its README
names (`npm run bench` in `js/`, `composer bench` in `php/`), that measures
what its build costs: counts, such as what checking a call costs a type
checker, sizes, and times. A PR is benchmarked against its base by
`bench.yml`, for each implementation it touches, which posts a table per implementation on the PR (a PR from a fork
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

### PHP

The PHP parser is released by the **PHP parser publish** workflow
(`.github/workflows/publish-php.yml`, started by hand), which waits on
[curly-message/spec#109](https://github.com/curly-message/spec/issues/109)
and is not run until that is decided. It reads the changelog's pending
section and the release types as the JavaScript workflow does: `patch`,
`minor` and `major` cut `### 1.1.0 (Unreleased)` and must arrive at the
version it names, and `next` cuts a release candidate of that version —
`1.1.0-RC.0`, then `.1`, Composer's spelling — and leaves the section open. It
runs the test matrix (`tests-php.yml`), writes the version into the
benchmark's `php/package.json`, writes the benchmark into `BENCH.md`, commits,
tags `php-v1.0.0` here, and publishes a GitHub release carrying the changelog
section, in the same three jobs as the JavaScript workflow.

Packagist reads a package from the root of a repository, and its versions
from tags its version parser accepts, which neither `php/` nor `php-v1.0.0`
is. So the workflow also splits the history of `php/` off with
`git subtree split` and pushes it to the read-only repository
`curly-message/parser-php`, tagged `v1.0.0`, where Packagist's GitHub hook
reads it, and the run ends once Packagist lists the version. No registry credential exists: what Packagist serves is what the
App pushed. That repository is submitted to Packagist by hand, once, with its
GitHub hook, and the workflow refuses to run while Packagist does not know the
package. Nothing is pushed until both pushes have been tried dry, and this
repository is pushed first; where the second push fails after the first
landed, running the workflow with `mirror` set to the tag that landed
(`php-v1.0.0`) pushes it to `curly-message/parser-php` alone.

## Specification

The format is defined in [`curly-message/spec`](https://github.com/curly-message/spec).
Its machine-readable identifier is `curly-message`; versioned references use
`curly-message-3`, and so on.

Issues for this repository are filed in the family's shared tracker,
[`curly-message/spec`](https://github.com/curly-message/spec/issues).

## License

[MIT](./LICENSE)
