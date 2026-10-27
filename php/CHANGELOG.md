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
* **`Parser::MAX_REENTRY` and `Parser::MAX_IN_PROGRESS` count resolutions
  alone** (SPEC.md section 13). An option or a fallback chain host code asks
  for is part of the walk of its resolution and takes no place, each
  rendering at most once, so a resolution's own walk never meets either
  bound; asking for a text while it is rendering still throws `ReentryLimit`.
