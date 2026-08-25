# AGENTS.md

Behavioral guidelines for LLM coding assistants working on the
**curly-message parsers** monorepo. Applies to anything that drives commits,
PRs, or file edits on this repo.

**Precedence:** These repo rules override individual LLM memory or personal
preference. If your own memory conflicts with this file, follow this file.

Everything this repository expects is in this file; it defers to nothing
outside it. The sections before the numbered ones are what is particular to
this repository; sections 1-14 are the working rules.

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
- **No host-framework coupling in a published surface.** A parser here
  implements the format, not any host library's calling convention; an adapter
  that presents it to one belongs in that host's own repository. A dependency
  on a host library here is a blocking change — stop and ask.
- **Behavior changes are format changes.** Anything that alters what a message
  resolves to needs a matching change in the spec repository, in the same
  round of work.

## Current state

`js/` resolves over the format's own named inputs and depends on no host
library. Nothing here is published yet, so there is no migration cost to
getting the shape right.

---

## 1. Think before coding

**Don't guess. Don't hide confusion. Surface tradeoffs.**

- State assumptions explicitly; if unsure, ask. Clarifying questions belong in
  chat **before** mistakes show up in the diff.
- If multiple interpretations exist, present them — don't pick silently.
- For non-trivial changes, propose the plan in chat **before** touching files.
- **Refetch before reasoning, don't recall.** In long sessions, fetch current
  state (PR/issue meta, branch state, file content, CI status) instead of
  trusting memory. The user may have merged a PR, a hook may have touched
  files. In-session recall is a cache; the system is the source of truth.

## 2. Simplicity first

Minimum code that solves the problem. No speculative features or abstractions.
Validate only at boundaries — internal contracts are contracts. That said, a
parser deliberately **fails soft** at its public edges (see §11): an unknown
modifier, a missing payload key, or a message that references its own
placeholder must degrade to something sensible, not throw.

## 3. Surgical changes

**Touch only what you must. Match existing style even if you'd write it
differently.**

- Don't "improve" adjacent code/formatting unrelated to the task.
- Don't refactor what isn't broken.
- Remove only the imports/vars/types **your** change orphaned.
- Notice unrelated dead code or a bug? **Mention it in chat** — don't fix
  silently in the same PR.

## 4. Verify before committing

**Every commit's tip is green.**

- There is no root manifest: run the checks from inside the implementation
  directory (`cd js`), and run them for every directory your change touches.
- Before each code commit: `npm test` and `npm run lint` pass. `pretest` builds
  and typechecks first, so a green `npm test` covers all three. Report real
  output — never claim done without running them.
- Type/lint/build errors never reach a commit, not even WIP.
- Doc-only changes skip the build but still verify links resolve and markdown
  renders.

## 5. Commit on approval

**Local changes are the default. Committing is the user's call.**

- Respond to requests by editing **locally**; show the diff; ask "ok?".
- Commit only after explicit approval ("ok", "commit it", "create the commit",
  or a fixup request).
- A modified working tree between turns is the **expected state**, not mess to
  clean up unprompted.
- **Approval is scoped to the named changes.** Approving X doesn't authorize
  bundling unrelated untracked files into the commit.

## 6. Incremental commits & fixup hygiene

- **One concern per commit.** Each commit is self-contained and lands code in
  its **final form**.
- **Never** add code in one commit and refactor it away in a later commit on
  the same branch. Refinement of something this branch already introduced →
  `git commit --fixup=<sha>` + `git rebase -i --autosquash`, not an "address
  review" commit. Standalone commits are reserved for genuinely new concerns.
- "Fix this"/"amend"/refinement language within an active branch means the
  **fixup workflow**, not a new commit.
- After an approved fixup, the local autosquash rebase **and** the
  `git push --force-with-lease` are part of the same approved step — no
  separate approval, but only for the presented changes.
- Commit messages: imperative mood, `type(scope): summary`. Scope the
  implementation directory when the change is inside one (`feat(js): …`,
  `fix(js): …`); leave it off for root-level changes (`docs: …`, `chore: …`).

## 7. Branch & push discipline

- **Default branch is `main`** (verify via `git remote show origin` if unsure).
  Never commit straight to it; never force-push a shared branch.
- **`main` carries only finished product.** Nothing merges that is not complete
  and shippable as-is — no temporary bootstraps or workarounds for unpublished
  dependencies, no half-built surfaces. Branch-only scaffolding (e.g. a `file:`
  dev dependency standing in for an unpublished package) is removed before
  merge; until its precondition is met, the PR waits as a draft.
- **Branch from `main`, not from whatever HEAD happens to be:**
  `git fetch origin && git switch main && git pull --ff-only`, then create the
  branch. Use a descriptive prefix: `fix/<slug>`, `feat/<slug>`,
  `chore/<slug>`, `docs/<slug>`.
- Rebase on `main` before pushing a feature branch; on conflicts, **stop and
  ask** — resolution is judgment, not automation. Never merge `main` into a
  feature branch.

## 8. PRs

- **Open a PR only when explicitly asked.** Keep it narrowly scoped; list
  out-of-scope follow-ups under `## Notes` rather than expanding silently.
- Title ≤ 70 chars, describes the overarching scope. Body: a short summary +
  what was tested (real results: build/test/audit), and the linked issue via
  closing keywords (`Closes #N` / `Fixes #N`) when one exists.
- **Keep PR meta in lockstep with the branch.** After every push, re-check that
  the title, summary, test results, and "in/out of scope" still match the diff.
  Drift is a defect, not a follow-up.

## 9. Docs track code

Update docs in the same PR that invalidates them. Scope: this file, the root
`README.md`, and — per implementation directory — its `README.md`, its
`CHANGELOG.md`, and the JSDoc in `src/types.ts`. A code change that contradicts
a doc updates the doc (ideally the same commit/fixup). Remove a feature →
remove its docs. Discover stale docs unrelated to your task → flag in chat
(§3). A change to what a message resolves to also updates the specification
repository, not just these docs.

## 10. Coding conventions

- Formatting contract (the `@stylistic` block in each implementation's
  `eslint.config.js` — for `js/`: 2-space indent, single quotes, semicolons,
  trailing commas on multiline, spaced object braces, no trailing whitespace,
  at most one consecutive blank line, newline at EOF). Let `npm run lint`
  handle it.
- Prefer the **functional, immutable** style for shared state (computed-key
  spread `{ ...acc, [k]: v }`, `reduce`). That spread form has `DefineProperty`
  semantics — it can't pollute `Object.prototype`. On a **measured hot path** a
  function-local accumulator may be built by mutation instead, but only into a
  null-prototype object (`Object.create(null)`), and it must be finished with a
  single spread before it escapes to consumers.
- Keep `index.ts` for the resolution pipeline; put pure, reusable helpers in
  `utils.ts` and modifier implementations in `modifiers.ts`.
- Diagnostics go through `console.warn` at the interpolation guards, and
  nowhere else. A parser has no logger of its own; adding one is a change to
  the public surface, so propose it rather than introducing it in passing.
- **Reuse before reimplementing** — `ownValue`, `unesc`, `getModifierDefaults`
  already exist. Grep before adding a helper; bend an existing one rather than
  forking.
- **Abstraction beats duplication.** When code repeats the same (or near-same)
  logical structure, that repetition is a candidate for abstraction — factor
  the shared shape into one named unit (helper, type, constant). This is
  distinct from §2's ban on *speculative* abstraction: §2 forbids inventing
  indirection for a hypothetical future; this rule consolidates duplication
  that **already exists**. When a fix would add a second copy of an existing
  structure, prefer extracting the shared core over pasting the copy. If the
  consolidation is large or reshapes call sites, surface the trade-off (§1) and
  recommend rather than refactor silently (§3).

## 11. Security & robustness posture

A parser has no eval, DOM, filesystem or network of its own; it turns
consumer-supplied text and consumer-supplied data into a string. The realistic
risks are **DoS / robustness / prototype-chain**, not RCE/XSS.

- **Prototype keys are missing payload entries.** Reads of the payload by a
  key taken from the message must use an own-property check — that is what
  `ownValue` is for — so `toString`/`__proto__`/`constructor` resolve as
  missing rather than as inherited members.
- **Never bracket-assign a message-supplied key onto a plain object.**
  `target[key] = value` routes `'__proto__'` through the prototype setter, so
  no own key is recorded and the object's prototype is replaced. Write with a
  computed-key spread (`{ ...target, [key]: value }`, which is
  `DefineProperty`) or target an `Object.create(null)` object.
- **The interpolation guards are load-bearing.** `MAX_INTERPOLATION_PASSES`
  and `MAX_INTERPOLATION_LENGTH` are what make a payload value that references
  or multiplies its own placeholder terminate instead of hanging the caller.
  Don't remove them, don't raise them to buy behavior, and treat a change to
  either as a format change (§ Implementation independence).
- **Placeholder matching is regex over consumer-controlled text**, which makes
  it the ReDoS surface here. A new or widened pattern needs an explicit check
  for catastrophic backtracking before it lands; if you touch one and can't
  establish that, flag it.
- **Fail soft at the edges.** An unrecognized modifier falls back to `eq`, a
  missing payload key yields the declared default. Resolving a message must not
  throw.

## 12. Comments & language

- Default to **no** comment; code says *what*, comments say *why* (a hidden
  constraint, an invariant, a non-obvious workaround). Never reference the
  current task, a fixed bug, or a PR number — that rots.
- A name that needs a comment to explain it is the wrong name — fix the name.
- If you need a paragraph-long comment to justify why the workaround is OK,
  the code is wrong — fix the code.
- **All committed/published artifacts in English** — code comments, commit
  messages, PR titles/bodies, doc files. Chat may stay in the user's preferred
  language; the moment it crosses into something on GitHub, switch to English.

## 13. Tests

- For `js/`, tests live in `tests/specs/` — `index.spec.ts` for resolution,
  `types.spec.ts` for the type surface — with fixtures in `tests/data/`.
- Drive behavior through the **public API** (`createParser(options).resolve`).
  Pure helpers may be imported directly from `src/` when that yields a more
  deterministic test.
- A type test must exercise the value it types. A closure that is declared and
  never invoked asserts nothing at runtime, and passes whatever it contains.
- Wait on awaitable promises or observable state, never on wall-clock sleeps —
  the CI matrix has six legs and timing-based tests flake on the slow ones.
- **Bug fixes are test-driven (red → green).** Write a test that reproduces the
  bug, confirm it **fails** against the unfixed code, then fix, then watch it
  pass. State that you verified both directions. Commit the regression test
  **alongside** the fix — never a fix without it.
- An assertion that contradicts the specification is a bug in one of them.
  Decide which, and say so (§ Implementation independence) — don't quietly
  reshape the test to match the code.

## 14. Output style

- Terse. Lead with results. No "I'll do X" preamble, no trailing recap of the
  diff.
- **One file = one visible operation** — create/modify files as discrete edits,
  not a shell loop that emits many at once, so each appears as its own diff.
- **No emojis** in code, commit messages, or PR descriptions unless requested.

---

**These guidelines are working if:** PRs review easily, commits read as a
single coherent story, an implementation never drifts from the specification by
accident, and clarifying questions show up in chat before mistakes show up in
the diff.
