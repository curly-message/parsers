# Changelog

## 1.0.0

* **The package is `curly-message-parser`, imported as
  `curly_message_parser`.** `1.0.0rc0` and `1.0.0` were first published as
  `curly-message`, imported as `curly_message`; that PyPI project is
  archived and takes no more releases. To move, install
  `curly-message-parser` in its place (`pip install curly-message-parser`,
  with `[intl]` where the extra was installed) and import
  `curly_message_parser`. Nothing else changes from `1.0.0` as published
  under the former name.
* **The first implementation of `curly-message-3` in Python.** `create_parser`
  resolves a message at the Core and Extensions levels with no dependencies,
  and at the Intl level where Babel 2.14 or later is installed, as the
  `intl` extra installs it; `levels()` answers which. `cst` describes a
  message as the tree of CST.md, its spans in code points. Every case of
  the conformance set passes, at every level and over the tree, but the five
  that hand over a modifier's layer of props refusing to be read: a `dict`
  entry raises only where it is looked up by name, and a layer is enumerated,
  but for the digit entries `plural` looks up by name in the `number` layer.
* **`Level` is exported**: the type `levels()` answers members of.
* **A host builds a modifier's input from strings.** `Option` and
  `ModifierInput`, built to test a modifier directly, take an option's value
  and a default as `str` and raise `TypeError` on anything else, where
  `1.0.0rc0` took a callable.
* **A date without a zone is local time as Python's `datetime` reads it.**
  Outside Windows, local time is the C library's, from `TZ` or
  `/etc/localtime` and the system's zone database alone (a `TZ` naming a
  zone the system lacks is UTC, whatever `tzdata` holds), and follows a
  later `time.tzset()`. A time shown in it without a `timeZone` names the
  local zone by its offset (`GMT+1`, `GMT` at offset zero), not by a zone
  name. A local time a transition skips is read as the running Python reads
  it: at the offset after the transition on 3.11, before it from 3.12. A
  time at a local offset of a day or more cannot be formatted, where it was
  read at offset zero.
* **A unit is read at the next length where one holds nothing for it.** A
  length of a unit's data that holds neither the plural category nor
  `other` gives the pattern to the next one, and the `long` length falls
  back to the `short`, where `1.0.0rc0` wrote the number without its unit.
