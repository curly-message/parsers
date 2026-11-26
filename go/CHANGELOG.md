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
  `props`, and `Context.Locale`. The properties are one read-only map: the
  layers every placeholder shares are composed once in a resolution, a
  wrapper's layer once for each run of placeholders whose wrappers hold it,
  and the map is shared by the placeholders its composition serves.
  Resolutions are counted along a lineage: `Parser.Resolve` begins one and
  is never refused, and the parser hands each piece of host code it calls a
  `Resolver` that continues it, a modifier through `Input.Resolver`, a
  value implementing `Texter` through `CurlyText`, and the
  `Options.OnReport` handler, a `func(Report, Resolver)`;
  `Resolver.With` resolves with another parser on the same lineage. A
  resolution one past `MaxReentry` is refused with `ErrReentryLimit`. A
  read of an option or default kept from another call counts as a
  resolution in progress while it renders, and one past `MaxReentry` is
  refused: it renders nothing, answers the empty string and fails nothing.
  A read never waits: one of an option or default being rendered is refused
  at once in the same way, and one that meets a value being converted is
  refused where it meets it, the empty string being that option's or
  default's one production.
- The Intl level, delegated to the host's facility through `Options.Intl`:
  `number`, `currency`, `date`, `ago`, `plural` and `ordinal`, with every
  rule of the format decided in the parser and only the text and the plural
  category asked of the supplier. A supplier implementing `IntlResolving` is
  handed the `Resolver` of the resolution asking it.
- A report's origin is read off the report: `Code.Origin` is no longer
  exported, as it was in `v1.0.0-next.0`, and a host reads `Report.Origin`.
