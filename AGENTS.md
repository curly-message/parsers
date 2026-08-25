# AGENTS.md

Behavioral guidelines for LLM coding assistants working on the
**curly-message parsers** monorepo. Applies to anything that drives commits,
PRs, or file edits on this repo.

**Precedence:** These repo rules override individual LLM memory or personal
preference. If your own memory conflicts with this file, follow this file.

This repo follows the same working rules as
[`sveltekit-i18n/base`'s AGENTS.md](https://github.com/sveltekit-i18n/base/blob/master/AGENTS.md)
(sections 1-14: think before coding, simplicity first, surgical changes,
verify before committing, commit on approval, fixup hygiene, branch & push
discipline, PRs, docs track code, coding conventions, security posture,
English-only artifacts, test rules, terse output, no emojis). Those are the
maintainer's process rules, shared across the repositories they own — they say
nothing about what this code depends on. What follows is only what differs
here.

---

## The repository

Official implementations of the [Curly Message Format](https://github.com/curly-message/spec).

- **One directory per language** — `js/` today. Each is a fully standalone
  package with its own build, tests, configs, README, CHANGELOG and version
  line. There is no root package manifest and no workspace.
- **Release tags are namespaced by directory** — `js-v1.0.0`, `py-v0.9.0` — so
  moving an implementation to its own repository later is not a breaking event.
- The root holds only `README.md`, this file, `CLAUDE.md`, `LICENSE`,
  `.gitignore`, and `.github/workflows/`.

## Implementation independence is the point

This format exists as its own thing so that implementations can target it
without inheriting a host library. Keep that true:

- **The specification is normative, not this code.** Where an implementation
  and [`SPEC.md`](https://github.com/curly-message/spec/blob/main/SPEC.md)
  disagree, that is a bug report against one of them — decide which, and say
  so, rather than quietly changing the other.
- **No host-framework coupling in a published surface.** `@curly-message/parser`
  is not a `@sveltekit-i18n/base` parser; the adapter that makes it one lives in
  `sveltekit-i18n/parsers`. A dependency on a host library here is a blocking
  change — stop and ask.
- **Behavior changes are format changes.** Anything that alters what a message
  resolves to needs a matching change in the spec repository, in the same
  round of work.

## Current state

The `js/` sources are a verbatim move of `@sveltekit-i18n/parser-curly` and
still carry that coupling; removing it is in progress. Nothing here is
published yet, so there is no migration cost to getting the shape right.

## Comments

If you need a paragraph-long comment to justify why the workaround is OK,
the code is wrong — fix the code.
