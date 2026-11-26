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
