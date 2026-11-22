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
