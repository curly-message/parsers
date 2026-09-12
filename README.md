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
`@curly-message/parser`, and implements `curly-message-1` — version 1 of the
format, which the specification states is stable.

It resolves over the format's own inputs and depends on no host library.

## Implementations

| Path | Package | State |
| --- | --- | --- |
| [`js/`](./js) | `@curly-message/parser` | Stable, `latest` |

One directory per language, each a standalone package with its own build,
tests and version line. Release tags are namespaced by directory
(`js-v1.0.0`), so an implementation can leave for its own repository without
that being a breaking change for the others.

## Releasing

A release is cut from `main` by the **JavaScript parser publish** workflow
(`.github/workflows/publish-js.yml`, started by hand). `next` bumps the
prerelease counter and publishes under the `next` dist-tag; `patch`, `minor`
and `major` cut a release under `latest`, closing any prerelease line. The
workflow runs the test matrix, bumps the version, turns the changelog's
`## Unreleased` section into the version's, commits, tags (`js-v1.0.0`),
pushes, publishes to npm, and publishes a GitHub release carrying that
changelog section. A release whose changelog has no `## Unreleased` section
is refused.

The commit, the tag and the release are made as a GitHub App, whose id and
private key the repository holds as the `APP_ID` variable and the
`APP_PRIVATE_KEY` secret. npm holds no token: the workflow is the package's
[trusted publisher](https://docs.npmjs.com/trusted-publishers), registered
in the package's settings on npmjs.com or with
`npm trust github --file publish-js.yml --repository curly-message/parsers --allow-publish`
— the calling workflow's filename, which is the one the registry checks — and
the registry attaches provenance itself. A trusted publisher can be
registered only for a package that exists, so the first version,
`1.0.0-next.0`, was published by hand from `main` by a maintainer of the scope
(`cd js && npm ci && npm publish --access public --tag next`); the workflow
refuses to run for a package the registry does not know.

## Specification

The format is defined in [`curly-message/spec`](https://github.com/curly-message/spec).
Its machine-readable identifier is `curly-message`; versioned references use
`curly-message-1`, and so on.

## License

[MIT](./LICENSE)
