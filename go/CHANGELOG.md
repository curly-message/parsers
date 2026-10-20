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
  and the map is shared by the placeholders its composition serves. A read
  of an option or default kept from another call counts as a resolution in
  progress while it renders, and one past `MaxReentry` renders nothing and
  fails the host code called last on the goroutine that made it; a read from
  inside the option's or default's own rendering, on the goroutine rendering
  it, is refused at once in the same way.
  `Input.Resolve` resolves a message the modifier asks for, counting the
  resolutions of its chain in progress, whichever `Input` asks, and
  refusing one past `MaxReentry` with `ErrReentryLimit`.
