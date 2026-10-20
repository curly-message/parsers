package conformance

import (
	"errors"
	"strings"
)

// behaviours is the catalogue of README.md, each behaviour a function of
// section 11's inputs.
var behaviours = map[string]Behaviour{
	"upper": func(in ModifierInput) (any, error) {
		return strings.Map(func(r rune) rune {
			if 'a' <= r && r <= 'z' {
				return r - 'a' + 'A'
			}

			return r
		}, in.Value), nil
	},
	"echo": func(in ModifierInput) (any, error) {
		return in.Value, nil
	},
	"empty": func(ModifierInput) (any, error) {
		return "", nil
	},
	"nothing": func(ModifierInput) (any, error) {
		return nil, nil
	},
	"raise": func(ModifierInput) (any, error) {
		return nil, errors.New("The raise behaviour raised.")
	},
	"default": func(in ModifierInput) (any, error) {
		return in.Default(), nil
	},
	"options": func(in ModifierInput) (any, error) {
		written := make([]string, len(in.Options))

		for i, option := range in.Options {
			written[i] = option.Key + "=" + option.Value()
		}

		return strings.Join(written, ","), nil
	},
	"props": func(in ModifierInput) (any, error) {
		names := sortedKeys(in.Props)
		written := make([]string, len(names))

		for i, name := range names {
			value, ok := stringify(in.Props[name])

			if !ok {
				return nil, errors.New("The props behaviour met a value JSON cannot describe.")
			}

			written[i] = name + "=" + value
		}

		return strings.Join(written, ","), nil
	},
	"locale": func(in ModifierInput) (any, error) {
		if in.Locale == nil {
			return "none", nil
		}

		return *in.Locale, nil
	},
	"object": func(in ModifierInput) (any, error) {
		return map[string]any{"answer": in.Value}, nil
	},
}
