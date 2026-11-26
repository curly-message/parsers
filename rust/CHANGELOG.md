# Changelog

### 1.0.0 (Unreleased)

* **First release.** Implements `curly-message-3`: Core and Extensions, and
  Intl with the `intl` feature, through ICU4X.
* **Locale data is ICU4X's.** `IcuIntl` canonicalizes a locale by the alias
  data ICU4X carries before it reads any data, so an alias reads as the tag
  it names, and one that names a locale without data reads the data that
  locale falls back to. The locale data, aliases, rules and fallback of the
  Intl level are ICU4X's: where they differ from another host's, `IcuIntl`
  answers as ICU4X does.
* **ICU4X 2.4.** With `intl`, the crate builds on ICU4X 2.4 and
  `icu_experimental` 0.7, and so on CLDR 49's locale data, where
  `1.0.0-next.0` built on ICU4X 2.3: an application's graph holds ICU4X
  2.4.x beside it.
* **The number conversions are public.** `string_conversion` (section 4),
  `numeric_conversion` (sections 4 and 11.1) and `modifier_input`
  (sections 11.2 and 11.5) read and write a number as the parser does.
* **`Value::Date` is gone.** An instant is `Value::Number` of its
  milliseconds since the epoch, which `date` formats, or `Value::host` of a
  date crate's type, whose `Display` text `date` reads.
* **The enums a later revision can extend are `non_exhaustive`:** `Code`,
  `Api`, `Unit`, `Input`, `Level` and `Value`, so a match on one outside the
  crate takes a wildcard arm, and a new variant is not a breaking change.
* **`MAX_SERIALIZED_LENGTH` counts bytes, 10 000 000 of them.** One
  serialization of a value, and a value's `Display`, writes at most
  10 000 000 bytes of UTF-8, the unit every other text limit of the crate
  counts in, where `1.0.0-next.0` let it write 536 870 888 code units of
  UTF-16, the most a string holds in V8. A value whose text is longer now
  describes nothing: its placeholder takes the fallback chain and reports
  `unserializable-value`.
* **A report's origin is read off the report.** `Code::origin` is no longer
  public: a host reads `Report::origin`, as the JavaScript and Python parsers
  expose it.
