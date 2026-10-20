package conformance

import (
	"errors"
	"fmt"
)

// tag is the key that marks an object of a case's inputs as standing for a
// value JSON cannot write.
const tag = "$curly"

// Nodes is a value whose serialization visits at least count nodes.
// Serialization follows a shared slice again every time it meets one, so
// each level of []any{below, below} spends one node on itself and twice what
// below spends, and the tree doubles its visits per level without ever
// holding a cycle.
func Nodes(count float64) any {
	tree := []any{}

	for visits := 1.0; visits < count; visits = visits*2 + 1 {
		tree = []any{tree, tree}
	}

	return tree
}

func unserializable() any {
	value := map[string]any{}
	value["self"] = value

	return value
}

func tagged(members map[string]any, undefined any) (any, error) {
	switch members[tag] {
	case "undefined":
		return undefined, nil
	case "unserializable":
		return unserializable(), nil
	case "nodes":
		count, ok := members["count"].(float64)

		if !ok {
			return nil, errors.New("A nodes tag needs a numeric count.")
		}

		return Nodes(count), nil
	}

	return nil, fmt.Errorf("Unknown tag %s.", shown(members[tag]))
}

// decode is the host value a case's input stands for: each tagged object
// replaced, at every depth, and everything else as encoding/json decodes it,
// every number a float64. The undefined tag decodes to the adapter's own
// absence. Each call builds the value afresh, so no case shares one with
// another.
func decode(value any, undefined any) (any, error) {
	type job struct {
		value any
		set   func(any)
	}

	var root any
	jobs := []job{{value, func(v any) { root = v }}}

	for len(jobs) > 0 {
		next := jobs[len(jobs)-1]
		jobs = jobs[:len(jobs)-1]

		switch v := next.value.(type) {
		case []any:
			built := make([]any, len(v))
			next.set(built)

			for i := len(v) - 1; i >= 0; i-- {
				jobs = append(jobs, job{v[i], func(entry any) { built[i] = entry }})
			}
		case map[string]any:
			if _, ok := v[tag]; ok {
				decoded, err := tagged(v, undefined)

				if err != nil {
					return nil, err
				}

				next.set(decoded)

				continue
			}

			built := make(map[string]any, len(v))
			next.set(built)
			keys := sortedKeys(v)

			for i := len(keys) - 1; i >= 0; i-- {
				jobs = append(jobs, job{v[keys[i]], func(entry any) { built[keys[i]] = entry }})
			}
		default:
			next.set(v)
		}
	}

	return root, nil
}

// isUndefinedTag says whether a case's input is written as the undefined
// tag itself.
func isUndefinedTag(value any) bool {
	members, ok := value.(map[string]any)

	return ok && members[tag] == "undefined"
}
