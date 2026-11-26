# Changelog

### 1.0.0 (Unreleased)

* **The first release of the PHP implementation.** `CurlyMessage\Parser`
  resolves messages of `curly-message-3` at the Core and Extensions levels on
  PHP 8.2 and newer with no extension beyond those PHP always carries, and at
  the Intl level where ext-intl is loaded. `Cst::parse` describes a message as
  CST.md specifies, in bytes of the PHP string. The package passes the
  specification's conformance set 4.2.0.
* **`date` reads text by PHP's own date parser alone** since 1.0.0-RC.0,
  which read ECMAScript's date time string format first. A date with no
  time, `2024-03-05` or `2024-03`, is midnight in PHP's default time zone
  rather than at UTC, and `2024-02-30` and `2024-03-05T24:00`, which PHP
  warns of, are no date: the placeholder takes its fallback chain and
  reports `failed-modifier`.
* **Nothing runs on a resolution's account once it returned** (SPEC.md
  section 11.3). An `Option::value()` or a `ModifierInput::default()` host code
  kept and calls later answers the text a call during the resolution
  rendered, and the empty string where none finished one, rendering,
  reporting and converting nothing; a rendering a fiber left suspended in the
  resolution and resumes then answers the empty string.
* **`Parser::MAX_REENTRY` counts resolutions, and the texts host code reads
  outside the call they were handed to** (SPEC.md section 13). An option or
  a fallback chain read by the call of the modifier it was handed to is part
  of the walk of its resolution and takes no place, so a resolution's own
  walk never meets the bound. One read
  from anywhere else, as by a modifier that kept it from another placeholder,
  renders at a level the nesting limit does not count, so it takes a place
  while it renders. Where there is none the read is refused: it answers the
  empty string and fails nothing, so what the modifier then answers is taken
  as any answer is. A text
  asked for while it is rendering is refused the same way, at once, and never
  rendered a second time. A refused read throws nothing: `ReentryLimit` is
  thrown by `resolve` alone.
* **`Parser::MAX_IN_PROGRESS` is gone** since 1.0.0-RC.0, which refused a
  resolution begun beneath another on its thread of execution while 512 so
  begun were in progress in the process. Fibers left suspended in such
  resolutions held those places, so 512 of them refused every nested
  `resolve` in the process, however shallow each fiber was. `MAX_REENTRY`,
  per thread of execution, a fiber being one, is the only re-entry bound, as
  section 13 asks. A recursion that resolves its levels by turns in a fiber
  of its own and beneath the one before on its stack, so that no fiber is
  deep, now reaches no bound the parser keeps, and PHP's memory limit ends
  it with a fatal error rather than `ReentryLimit`, as it does one that
  begins every level in a fiber of its own: the host bounds both.
* **A modifier's props are a read-only `CurlyMessage\Props`**, read as an
  array is and by `has`, `get` and `toArray`. A resolution composes the layers
  every placeholder shares once for each modifier name and hands every
  placeholder naming it the same properties, a wrapper's own laid over them
  without either being copied, so a wide layer is neither walked nor kept
  again for each placeholder or each wrapped key. A modifier that writes into
  its props fails as one that throws does. A shared layer host code changes
  while a resolution is in progress is read as it was when first composed.
* **`Parser`'s `onRequest` option, `FormatRequest` and `Facility` are no
  longer public** since 1.0.0-RC.0. Observing the requests the Intl
  modifiers make is a seam of the conformance adapter, not a promise to a
  host.
* **`Props::with()` is gone** since 1.0.0-RC.0, **and `new Props($layers)` is
  public**: it composes `$layers`, the uppermost first, as the parser
  composes a modifier's props. `Props::only()` stays, internal as it was,
  and reads the layers alone.
* **`ReportCode::origin()`, `ReportCode::limit()` and `ReportCode::message()`
  are no longer public** since 1.0.0-RC.0: a `Report` carries its `origin`,
  its `limit` and its `message`. `ReportCode::OutputLimit->limit()` answered
  `Parser::MAX_OUTPUT_LENGTH` where the report it named could carry
  `Parser::MAX_TEXT_LENGTH`.
