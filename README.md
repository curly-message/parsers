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
same version and is not yet released. It is developed in `php/` and served
to Composer from
[`curly-message/parser-php`](https://github.com/curly-message/parser-php),
which only its publish workflow writes to (see *Releasing*).

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
checker, sizes, and times. A PR is benchmarked against its base by `bench.yml`,
for each implementation it touches, which posts a table per implementation on
the PR (a PR from a fork finds them in the job's summary). An implementation is
benchmarked where the branch or the base has its benchmark, so a PR that drops
the benchmark of an implementation it changes fails. A count that grew, a row
gone missing or changing kind, or a row that fails on the base fails the check
unless the PR carries the `bench-accepted` label; adding or removing the label
runs it again. The base is compared only where its commit has the
implementation's benchmark; where it has none, as on the PR that adds the
implementation or its benchmark, or one that renames its directory, nothing is
compared, and only the branch's own benchmark can fail the check.

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
