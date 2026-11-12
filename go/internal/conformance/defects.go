package conformance

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// answering is a defect that changes what the adapter answers. change
// answers with the altered resolution, or false where this resolution held
// nothing to alter, which is what Reached is read from.
func answering(adapter Adapter, change func(Resolved) (Resolved, bool)) Mutation {
	reached := false
	resolve := adapter.Resolve
	adapter.Resolve = func(input Resolution) *Resolved {
		resolved := resolve(input)

		if resolved == nil {
			return nil
		}

		changed, ok := change(*resolved)

		if !ok {
			return resolved
		}

		reached = true

		return &changed
	}

	return Mutation{adapter, func() bool { return reached }}
}

// reporting is a defect of the reports, which an implementation need not
// observe at all.
func reporting(adapter Adapter, change func([]Report) ([]Report, bool)) Mutation {
	return answering(adapter, func(resolved Resolved) (Resolved, bool) {
		if resolved.Reports == nil {
			return resolved, false
		}

		changed, ok := change(slices.Clone(*resolved.Reports))

		if !ok {
			return resolved, false
		}

		resolved.Reports = &changed

		return resolved, true
	})
}

// instead is a defect that answers nothing of what the implementation
// produced.
func instead(resolve func(Resolution) *Resolved) func(Adapter) Mutation {
	return func(adapter Adapter) Mutation {
		adapter.Resolve = resolve

		return Mutation{adapter, func() bool { return true }}
	}
}

// claiming is a defect of what the adapter claims about itself. Those
// claims are read before anything runs, so such a defect reaches the
// runner wherever the claim it alters was one the adapter made.
func claiming(adapter Adapter, claim func(*Adapter), reached bool) Mutation {
	claim(&adapter)

	return Mutation{adapter, func() bool { return reached }}
}

// copyTree is a tree copied node by node on a stack of its own, a node the
// tree holds twice copied once, so a defect alters the copy and never the
// tree the implementation answered with.
func copyTree(root *Node) *Node {
	copies := map[*Node]*Node{}
	clone := func(node *Node) (*Node, bool) {
		if node == nil {
			return nil, false
		}

		if copied, ok := copies[node]; ok {
			return copied, false
		}

		copied := *node
		copied.Nodes = slices.Clone(node.Nodes)
		copies[node] = &copied

		return &copied, true
	}

	top, _ := clone(root)
	stack := []*Node{top}

	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if node == nil {
			continue
		}

		for i, child := range node.Nodes {
			copied, fresh := clone(child)
			node.Nodes[i] = copied

			if fresh {
				stack = append(stack, copied)
			}
		}
	}

	return top
}

// parsing is a defect of the tree, which an implementation need not offer
// at all. An adapter that offers none carries nothing to alter, so the
// defect is unreachable rather than missed.
func parsing(adapter Adapter, change func(tree *Node, message string) bool) Mutation {
	cst := adapter.CST

	if cst == nil {
		return Mutation{adapter, func() bool { return false }}
	}

	reached := false
	wrapped := *cst
	wrapped.Parse = func(message string) *Node {
		tree := cst.Parse(message)
		copied := copyTree(tree)

		if copied == nil || !change(copied, message) {
			return tree
		}

		reached = true

		return copied
	}
	adapter.CST = &wrapped

	return Mutation{adapter, func() bool { return reached }}
}

// everywhere is a defect applied to every node it recognizes, which
// reaches the runner where it recognized at least one. change alters the
// node in place, the node being the defect's own copy.
func everywhere(adapter Adapter, change func(node *Node, message string) bool) Mutation {
	return parsing(adapter, func(tree *Node, message string) bool {
		altered := false
		seen := map[*Node]bool{}
		stack := []*Node{tree}

		for len(stack) > 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			if node == nil || seen[node] {
				continue
			}

			seen[node] = true

			if change(node, message) {
				altered = true
			}

			stack = append(stack, node.Nodes...)
		}

		return altered
	})
}

// spelling is the text a span covers, in the unit the adapter declared:
// what a name reads as before it is unescaped.
func spelling(message string, unit SpanUnit, node *Node) (string, bool) {
	offsets := boundaries(message, unit)

	if node.Start < 0 || node.End >= len(offsets) || node.Start > node.End {
		return "", false
	}

	from, to := offsets[node.Start], offsets[node.End]

	if from < 0 || to < 0 {
		return "", false
	}

	return message[from:to], true
}

func otherCode(code string) string {
	if code == "unknown-modifier" {
		return "failed-modifier"
	}

	return "unknown-modifier"
}

func otherOrigin(origin string) string {
	if origin == "message" {
		return "payload"
	}

	return "message"
}

// otherID is an id no case names.
const otherID = "an id no case names"

// otherRequest is a request the set states nowhere, so a case compared on
// one fails on it.
func otherRequest() []Request {
	return []Request{{API: NumberFormat, Options: map[string]any{"style": "percent"}, Input: 0.0}}
}

func alike(a, b Report) bool {
	return a.Code == b.Code && a.Origin == b.Origin && a.Limit == b.Limit && (a.ID == nil) == (b.ID == nil) && (a.ID == nil || *a.ID == *b.ID)
}

// whitespace is ECMAScript's WhiteSpace and LineTerminator, what
// String.prototype.trim takes off: the trim a runner that tidies an output
// would reach for.
func whitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', '\u2028', '\u2029', '\ufeff':
		return true
	}

	return unicode.Is(unicode.Zs, r)
}

// Mutations are the catalogue's defects as this runner carries them: each
// wraps a conforming adapter into one that is wrong in exactly one way.
var Mutations = map[string]func(Adapter) Mutation{
	"output-truncated": func(adapter Adapter) Mutation {
		return answering(adapter, func(resolved Resolved) (Resolved, bool) {
			if resolved.Output == "" {
				return resolved, false
			}

			_, size := utf8.DecodeLastRuneInString(resolved.Output)
			resolved.Output = resolved.Output[:len(resolved.Output)-size]

			return resolved, true
		})
	},

	"output-trimmed": func(adapter Adapter) Mutation {
		return answering(adapter, func(resolved Resolved) (Resolved, bool) {
			trimmed := strings.TrimFunc(resolved.Output, whitespace)

			if trimmed == resolved.Output {
				return resolved, false
			}

			resolved.Output = trimmed

			return resolved, true
		})
	},

	"answers-nothing": instead(func(Resolution) *Resolved { return nil }),

	"raises": instead(func(Resolution) *Resolved { panic(errors.New("The raises defect raised.")) }),

	"reports-dropped": func(adapter Adapter) Mutation {
		return reporting(adapter, func(reports []Report) ([]Report, bool) {
			return []Report{}, len(reports) > 0
		})
	},

	"reports-extra": func(adapter Adapter) Mutation {
		return reporting(adapter, func(reports []Report) ([]Report, bool) {
			return append(reports, Report{Code: "unknown-modifier"}), true
		})
	},

	"reports-reversed": func(adapter Adapter) Mutation {
		return reporting(adapter, func(reports []Report) ([]Report, bool) {
			reversed := slices.Clone(reports)
			slices.Reverse(reversed)

			return reversed, !slices.EqualFunc(reports, reversed, alike)
		})
	},

	"report-code-changed": func(adapter Adapter) Mutation {
		return reporting(adapter, func(reports []Report) ([]Report, bool) {
			for i := range reports {
				reports[i].Code = otherCode(reports[i].Code)
			}

			return reports, len(reports) > 0
		})
	},

	"report-origin-changed": func(adapter Adapter) Mutation {
		return reporting(adapter, func(reports []Report) ([]Report, bool) {
			carried := false

			for i := range reports {
				if reports[i].Origin != "" {
					reports[i].Origin = otherOrigin(reports[i].Origin)
					carried = true
				}
			}

			return reports, carried
		})
	},

	"report-id-changed": func(adapter Adapter) Mutation {
		return reporting(adapter, func(reports []Report) ([]Report, bool) {
			carried := false

			for i := range reports {
				if reports[i].ID != nil {
					id := otherID
					reports[i].ID = &id
					carried = true
				}
			}

			return reports, carried
		})
	},

	"report-limit-changed": func(adapter Adapter) Mutation {
		return reporting(adapter, func(reports []Report) ([]Report, bool) {
			carried := false

			for i := range reports {
				if reports[i].Limit != 0 {
					reports[i].Limit++
					carried = true
				}
			}

			return reports, carried
		})
	},

	// The one defect that answers what the implementation produced and
	// states something else about it: the output stands, and only the
	// request is wrong, so a runner that ignores the request answers nothing
	// at all.
	"formats-wrong": func(adapter Adapter) Mutation {
		resolve := adapter.Resolve
		adapter.Resolve = func(input Resolution) *Resolved {
			resolved := resolve(input)

			if resolved == nil {
				return nil
			}

			changed := *resolved
			requests := otherRequest()
			changed.Formats = &requests

			return &changed
		}

		return Mutation{adapter, func() bool { return slices.Contains(adapter.Levels, LevelIntl) }}
	},

	"reports-unobserved": func(adapter Adapter) Mutation {
		return answering(adapter, func(resolved Resolved) (Resolved, bool) {
			if resolved.Reports == nil {
				return resolved, false
			}

			resolved.Reports = nil

			return resolved, true
		})
	},

	"claims-core-only": func(adapter Adapter) Mutation {
		return claiming(adapter, func(a *Adapter) { a.Levels = []Level{LevelCore} }, len(adapter.Levels) > 1)
	},

	"unexpressible-declared": func(adapter Adapter) Mutation {
		return claiming(adapter, func(a *Adapter) {
			a.Unexpressible = map[API][]string{NumberFormat: {"maximumFractionDigits"}}
		}, slices.Contains(adapter.Levels, LevelIntl))
	},

	"claims-no-core": func(adapter Adapter) Mutation {
		return claiming(adapter, func(a *Adapter) {
			a.Levels = slices.DeleteFunc(slices.Clone(a.Levels), func(level Level) bool { return level == LevelCore })
		}, true)
	},

	"claims-unknown-level": func(adapter Adapter) Mutation {
		return claiming(adapter, func(a *Adapter) { a.Levels = append(slices.Clone(a.Levels), "ecmascript") }, true)
	},

	"claims-no-limits": func(adapter Adapter) Mutation {
		return claiming(adapter, func(a *Adapter) { a.Limits.Nesting = 0 }, true)
	},

	"tree-unoffered": func(adapter Adapter) Mutation {
		return claiming(adapter, func(a *Adapter) { a.CST = nil }, adapter.CST != nil)
	},

	"tree-node-dropped": func(adapter Adapter) Mutation {
		return parsing(adapter, func(tree *Node, _ string) bool {
			if len(tree.Nodes) == 0 {
				return false
			}

			tree.Nodes = tree.Nodes[:len(tree.Nodes)-1]

			return true
		})
	},

	"tree-node-retyped": func(adapter Adapter) Mutation {
		return everywhere(adapter, func(node *Node, _ string) bool {
			if node.Type != "separator" {
				return false
			}

			node.Type = "text"

			return true
		})
	},

	"tree-span-shifted": func(adapter Adapter) Mutation {
		return everywhere(adapter, func(node *Node, _ string) bool {
			if node.Type != "placeholder" {
				return false
			}

			node.Start++

			return true
		})
	},

	"tree-name-raw": func(adapter Adapter) Mutation {
		return everywhere(adapter, func(node *Node, message string) bool {
			if !slices.Contains(named, node.Type) || adapter.CST == nil {
				return false
			}

			spelled, ok := spelling(message, adapter.CST.Unit, node)

			if !ok || (node.Name != nil && *node.Name == spelled) {
				return false
			}

			node.Name = &spelled

			return true
		})
	},

	"tree-cancels-inverted": func(adapter Adapter) Mutation {
		return everywhere(adapter, func(node *Node, _ string) bool {
			if node.Type != "escape" {
				return false
			}

			inverted := node.Cancels == nil || !*node.Cancels
			node.Cancels = &inverted

			return true
		})
	},

	"tree-unit-changed": func(adapter Adapter) Mutation {
		reached := adapter.CST != nil && adapter.CST.Unit != CodePoint

		return claiming(adapter, func(a *Adapter) {
			if a.CST != nil {
				changed := *a.CST
				changed.Unit = CodePoint
				a.CST = &changed
			}
		}, reached)
	},

	"tree-unit-unknown": func(adapter Adapter) Mutation {
		return claiming(adapter, func(a *Adapter) {
			if a.CST != nil {
				changed := *a.CST
				changed.Unit = "utf-32"
				a.CST = &changed
			}
		}, adapter.CST != nil)
	},
}

// verdict is what the runner answered for a defect, read against what it
// answered without one: a case that failed, a case left out, or a case that
// passed with its reports unchecked, where the run without the defect had
// none of those.
func verdict(result, baseline Result) Verdict {
	switch {
	case len(result.Failed) > len(baseline.Failed):
		return VerdictFail
	case len(result.Skipped) > len(baseline.Skipped):
		return VerdictSkip
	case len(result.Unobserved) > len(baseline.Unobserved):
		return VerdictUnobserved
	}

	return VerdictNone
}

// Audit runs the set's defect catalogue against an adapter that passes the
// set, and answers what this runner made of each defect: it caught it, it
// missed it, or the defect never reached it. RUNNER.md states what the
// catalogue is for, and what each defect pins.
func Audit(adapter Adapter, options Options) ([]Audited, error) {
	baseline, err := Run(adapter, options)

	if err != nil {
		return nil, err
	}

	if n := len(baseline.Failed); n > 0 {
		return nil, fmt.Errorf("The audit needs an adapter that passes the set; this one fails %d of %d cases.", n, len(baseline.Passed)+n)
	}

	set := options.Set

	if set == nil {
		if set, err = Vendored(); err != nil {
			return nil, err
		}
	}

	audited := make([]Audited, 0, len(set.Defects))

	for _, defect := range set.Defects {
		mutate, ok := Mutations[defect.ID]

		if !ok {
			return nil, fmt.Errorf("The catalogue names a defect this runner does not carry: %s.", shown(defect.ID))
		}

		mutation := mutate(adapter)
		observed := VerdictError

		if result, err := Run(mutation.Adapter, options); err == nil {
			observed = verdict(result, baseline)
		}

		outcome := Missed

		switch {
		case !mutation.Reached():
			outcome = Unreachable
		case observed == defect.Expects:
			outcome = Caught
		}

		audited = append(audited, Audited{defect, observed, outcome})
	}

	return audited, nil
}
