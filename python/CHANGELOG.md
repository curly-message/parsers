# Changelog

### 1.0.0 (Unreleased)

* **The first implementation of `curly-message-3` in Python.** `create_parser`
  resolves a message at the Core and Extensions levels with no dependencies,
  and at the Intl level where Babel 2.14 or later is installed, as the
  `intl` extra installs it; `levels()` answers which. `cst` describes a
  message as the tree of CST.md, its spans in code points. Every case of
  the conformance set 4.1.0 passes, at every level and over the tree.
