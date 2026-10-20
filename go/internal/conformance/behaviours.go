package conformance

import (
	"errors"
	"maps"
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

// keepers names the behaviours of README.md that work over one list of what
// was kept, which a case registers under keeping.
var keepers = []string{"keep", "keep-default", "take", "first", "taking", "nest", "keep-nest", "keep-nest-ne"}

// shownRead is a read that answered the empty string as a modifier that goes
// on with it answers: never empty, so a modifier that did not go on shows.
func shownRead(text string) string {
	if text == "" {
		return "empty"
	}

	return text
}

// taking is what the taking behaviour answers: a host value whose ordinary
// string conversion (section 4) runs code, as fmt converts an fmt.Stringer,
// so the read it makes is the conversion's and not the call's. A plain map
// would be serialized instead, and text computed in the call would move the
// read out of the conversion.
type taking struct {
	read func() string
}

func (t taking) String() string {
	return t.read()
}

// The message nest asks a resolution of, the one keep-nest does, and the
// one keep-nest-ne does, whose ne is the format's own: x holds the empty
// string, which differs from b, so it selects that option and produces it
// during its call.
const (
	nested        = "{{x:take; default:F}}"
	nestedTaking  = "{{x:taking; default:F}}"
	nestedDefined = "{{x:ne; b:{{x:taking; default:F}}}}"
)

// descended is the message descend asks a resolution of while it has
// further to go.
const descended = "{{x:descend; default:F}}"

// renested is the message renest asks a resolution of: the one it is
// registered in.
const renested = "[{{x:renest; default:F}}]"

// asking is how a behaviour asks for a resolution while its call is in
// progress: through the means the implementation handed it, where the
// adapter passes one on, and of adapter otherwise.
func asking(adapter Adapter, in ModifierInput) func(Resolution) *Resolved {
	if in.Resolve != nil {
		return in.Resolve
	}

	return adapter.Resolve
}

// renesting is the renest of README.md, which asks adapter for a resolution
// of the message it is registered in, with itself registered, and answers
// its output. It raises where that resolution answered none, which is how an
// adapter shows one its implementation refused. One level past what bound
// lets stand it asks for nothing and answers unrefused, so an
// implementation that refuses nothing fails the case in place of nesting
// until its host ends it.
func renesting(adapter Adapter, bound int) Behaviour {
	standing := 0

	var renest Behaviour

	renest = func(in ModifierInput) (any, error) {
		if standing > bound {
			return "unrefused", nil
		}

		standing++
		defer func() { standing-- }()

		resolved := asking(adapter, in)(Resolution{Message: renested, Payload: map[string]any{"x": ""}, ID: adapter.Undefined, Modifiers: map[string]Behaviour{"renest": renest}})

		if resolved == nil {
			return nil, errors.New("The resolution renest asked for answered no output.")
		}

		return resolved.Output, nil
	}

	return renest
}

// keeping is the behaviours of README.md over one list of what was kept.
// Each call builds them afresh, so what one resolution kept is read by no
// other but the one a nest, a keep-nest or a keep-nest-ne among them asks
// adapter for while its call is in progress, handing it these same
// behaviours: a runner calls it once per resolution it asks for, with the
// adapter it asks.
func keeping(adapter Adapter) map[string]Behaviour {
	return over(adapter, nil)
}

// descending is those behaviours with the descend of README.md beside them,
// which asks adapter for hops resolutions one inside another, each handed
// these same behaviours and descend, the innermost of them of inner.
func descending(adapter Adapter, hops int, inner string) map[string]Behaviour {
	return over(adapter, &descent{hops, inner})
}

// descent is where a descend among the behaviours over one list leads:
// through hops resolutions one inside another, the innermost of them of
// inner.
type descent struct {
	hops  int
	inner string
}

func over(adapter Adapter, d *descent) map[string]Behaviour {
	var kept []func() string

	claimed := map[int]bool{}

	// claim is the most recently kept input no take has claimed, claimed
	// before it is read, so a read that reaches take again finds the one
	// kept before it.
	claim := func() func() string {
		for i := len(kept) - 1; i >= 0; i-- {
			if !claimed[i] {
				claimed[i] = true

				return kept[i]
			}
		}

		return nil
	}

	take := func() string {
		if read := claim(); read != nil {
			return shownRead(read())
		}

		return "none"
	}

	record := map[string]Behaviour{}

	// registered is what a resolution asked for from here is handed: the
	// behaviours under their own names, and descend beside them where a
	// descent is under way.
	registered := record

	record["keep"] = func(in ModifierInput) (any, error) {
		kept = append(kept, in.Default)

		return "", nil
	}
	record["keep-default"] = func(in ModifierInput) (any, error) {
		kept = append(kept, in.Default)

		return in.Default(), nil
	}
	record["take"] = func(ModifierInput) (any, error) {
		return take(), nil
	}
	record["first"] = func(ModifierInput) (any, error) {
		if len(kept) == 0 {
			return "none", nil
		}

		return shownRead(kept[0]()), nil
	}
	record["taking"] = func(ModifierInput) (any, error) {
		return taking{take}, nil
	}
	// nest is the output of a resolution of message asked of the adapter,
	// over these same behaviours.
	nest := func(in ModifierInput, message string) (any, error) {
		resolved := asking(adapter, in)(Resolution{Message: message, Payload: map[string]any{"x": ""}, ID: adapter.Undefined, Modifiers: registered})

		if resolved == nil {
			return nil, errors.New("The resolution nest asked for answered no output.")
		}

		return resolved.Output, nil
	}

	record["nest"] = func(in ModifierInput) (any, error) {
		return nest(in, nested)
	}
	record["keep-nest"] = func(in ModifierInput) (any, error) {
		kept = append(kept, in.Default)

		return nest(in, nestedTaking)
	}
	record["keep-nest-ne"] = func(in ModifierInput) (any, error) {
		kept = append(kept, in.Default)

		return nest(in, nestedDefined)
	}

	if d != nil {
		registered = maps.Clone(record)
		descending := 0

		// With fewer calls in progress than the descent has hops, the next
		// hop; with as many, the message the descent leads to.
		registered["descend"] = func(in ModifierInput) (any, error) {
			descending++
			defer func() { descending-- }()

			if descending < d.hops {
				return nest(in, descended)
			}

			return nest(in, d.inner)
		}
	}

	return registered
}

// register is the modifiers a case registers: the catalogue's behaviours
// under modifiers, and those over one list of what was kept under keeping,
// built afresh for the resolution, with nil where the case registers none.
func register(modifiers, keepingNames map[string]string, adapter Adapter) map[string]Behaviour {
	if modifiers == nil && keepingNames == nil {
		return nil
	}

	registered := make(map[string]Behaviour, len(modifiers)+len(keepingNames))

	for name, behaviour := range modifiers {
		registered[name] = behaviours[behaviour]
	}

	if keepingNames != nil {
		kept := keeping(adapter)

		for name, behaviour := range keepingNames {
			registered[name] = kept[behaviour]
		}
	}

	return registered
}
