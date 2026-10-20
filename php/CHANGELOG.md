# Changelog

### 1.0.0 (Unreleased)

* **The first release of the PHP implementation.** `CurlyMessage\Parser`
  resolves messages of `curly-message-3` at the Core and Extensions levels on
  PHP 8.2 and newer with no extension beyond those PHP always carries, and at
  the Intl level where ext-intl is loaded. `Cst::parse` describes a message as
  CST.md specifies, in bytes of the PHP string. The package passes the
  specification's conformance set 4.1.0 but for
  `comparisons/infinity-compares`, which reads the text `Infinity` as
  ECMAScript's numeric conversion does.
* **Nothing runs on a resolution's account once it returned** (SPEC.md
  section 11.3). An `Option::value()` or a `ModifierInput::default()` host code
  kept and calls later answers the text a call during the resolution
  rendered, and the empty string where none finished one, rendering,
  reporting and converting nothing; a rendering a fiber left suspended in the
  resolution and resumes then answers the empty string.
* **`Parser::MAX_REENTRY` and `Parser::MAX_IN_PROGRESS` count resolutions,
  and the texts host code reads outside the call they were handed to**
  (SPEC.md section 13). An option or a fallback chain read by the call of the
  modifier it was handed to is part of the walk of its resolution and takes
  no place, so a resolution's own walk never meets either bound. One read
  from anywhere else, as by a modifier that kept it from another placeholder,
  renders at a level the nesting limit does not count, so it takes a place
  while it renders. Where there is none the read is refused: it answers the
  empty string, and the modifier being asked on that thread fails whatever it
  answers, taking its fallback chain and reporting `failed-modifier`. A text
  asked for while it is rendering is refused the same way, at once, and never
  rendered a second time. A refused read throws nothing: `ReentryLimit` is
  thrown by `resolve` alone.
* **A modifier's props are a read-only `CurlyMessage\Props`**, read as an
  array is and by `has`, `get` and `toArray`, and so are a `FormatRequest`'s
  `options`. A resolution composes the layers every placeholder shares once
  for each modifier name and hands every placeholder naming it the same
  properties, a wrapper's own laid over them without either being copied, so
  a wide layer is neither walked nor kept again for each placeholder or each
  wrapped key. A modifier that writes into its props fails as one that throws
  does. A shared layer host code changes while a resolution is in progress is
  read as it was when first composed.
