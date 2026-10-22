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
