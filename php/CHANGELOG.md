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
  and the texts host code asks for from outside the call to their own
  modifier** (SPEC.md section 13). An option or a fallback chain the call to
  its own modifier asks for, the conversion of what that call answered
  included, renders where the message nests it and takes no place, so a
  resolution's own walk never meets either bound. One asked for anywhere
  else, by a modifier it was kept for, a handler or a payload value's
  conversion, takes a place as it renders, so a message whose placeholders
  each read what the one before kept stops at the bound of its thread. A read
  refused there, or one of a text while it is rendering, answers the empty
  string and fails the host code the parser called that asked, throwing
  nothing; made where no such code runs on its thread, as in a fiber a
  modifier started, it fails the host code the parser called that waits on
  that fiber, having started or resumed it, so that modifier reports
  `failed-modifier` whatever the fiber's code answers.
