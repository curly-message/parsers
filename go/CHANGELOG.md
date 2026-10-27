# Changelog

### 1.0.0 (Unreleased)

- The first release: `curly.CST`, the concrete syntax tree of a message, and
  its interchange encoding through `Node.AppendJSON`.
- `curly.New` and `Parser.Resolve`, resolution at the Core level: the
  fallback chain, value wrappers, the comparison modifiers, the limits of
  section 13 and the reports of section 14.3 through `Options.OnReport`.
- The Extensions level: `Options.CustomModifiers` registers a host's own
  modifiers, which receive an `Input` with the value, the options and the
  fallback chain rendered on demand until `Resolve` returns, the composed
  properties of `Options.ModifierDefaults`, `Context.Props` and a wrapper's
  `props`, and `Context.Locale`. `Input.Resolve` resolves a message the
  modifier asks for, counting the resolutions of its chain in progress and
  refusing one past `MaxReentry` with `ErrReentryLimit`.
- The Intl level, delegated to the host's facility through `Options.Intl`:
  `number`, `currency`, `date`, `ago`, `plural` and `ordinal`, with every
  rule of the format decided in the parser and only the text and the plural
  category asked of the supplier.
