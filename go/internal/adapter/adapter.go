// Package adapter is the adapter of section 14.3 for package curly: what
// the conformance runner drives the parser through. It translates the
// runner's calling convention into the parser's and nothing else.
//
// A message id that is not text is passed to the parser as "", which names
// none: Context.ID is a string. The parser's reports carry their origin and
// their limit, and the id where the resolution named one.
package adapter

import (
	"maps"
	"sync"

	"github.com/curly-message/parsers/go/curly"
	"github.com/curly-message/parsers/go/internal/conformance"
)

// New is the adapter of a parser configured per case. With intl nil it
// claims Core and Extensions and exposes no formatting requests; with a
// supplier it claims Intl as well and exposes every request the parser
// makes of it, so a case is measured on the request rather than on the
// supplier's locale data (section 11.2). unexpressible names what the
// supplier cannot express, as the supplier documents it.
func New(intl curly.Intl, unexpressible map[conformance.API][]string) conformance.Adapter {
	levels := []conformance.Level{conformance.LevelCore, conformance.LevelExtensions}

	if intl != nil {
		levels = []conformance.Level{conformance.LevelCore, conformance.LevelIntl, conformance.LevelExtensions}
	}

	return conformance.Adapter{
		Levels: levels,
		Limits: conformance.Limits{
			Output:     curly.MaxOutputLength,
			Read:       curly.MaxReadLength,
			Conversion: curly.MaxConversionNodes,
			Nesting:    curly.MaxNesting,
		},
		Unexpressible: unexpressible,
		Undefined:     curly.Undefined{},
		Resolve: func(in conformance.Resolution) *conformance.Resolved {
			return resolve(intl, in)
		},
		CST: &conformance.CST{Unit: conformance.UTF8, Parse: parse},
	}
}

func resolve(intl curly.Intl, in conformance.Resolution) *conformance.Resolved {
	var mu sync.Mutex
	reports := []conformance.Report{}
	options := curly.Options{
		ModifierDefaults: in.Defaults,
		OnReport: func(r curly.Report) {
			mu.Lock()
			defer mu.Unlock()

			reports = append(reports, report(r))
		},
	}

	if len(in.Modifiers) > 0 {
		options.CustomModifiers = make(map[string]curly.Modifier, len(in.Modifiers))

		for name, behaviour := range in.Modifiers {
			options.CustomModifiers[name] = modifier(behaviour)
		}
	}

	var recorded *recorder

	if intl != nil {
		recorded = &recorder{intl: intl, requests: []conformance.Request{}}
		options.Intl = recorded
	}

	id, _ := in.ID.(string)
	output := curly.New(options).Resolve(in.Message, curly.Context{
		Payload: in.Payload,
		Props:   in.Props,
		Locale:  in.Locale,
		ID:      id,
	})

	mu.Lock()
	defer mu.Unlock()

	resolved := &conformance.Resolved{Output: output, Reports: &reports}

	if recorded != nil {
		resolved.Formats = &recorded.requests
	}

	return resolved
}

func report(r curly.Report) conformance.Report {
	observed := conformance.Report{Code: string(r.Code), Origin: string(r.Origin), Limit: r.Limit}

	if r.ID != "" {
		id := r.ID
		observed.ID = &id
	}

	return observed
}

func modifier(behaviour conformance.Behaviour) curly.Modifier {
	return func(in curly.Input) (any, error) {
		options := make([]conformance.OptionInput, len(in.Options))

		for i, option := range in.Options {
			options[i] = conformance.OptionInput{Key: option.Key, Value: option.Value}
		}

		return behaviour(conformance.ModifierInput{
			Value:   in.Value,
			Options: options,
			Props:   in.Props,
			Locale:  in.Locale,
			Default: in.Default,
		})
	}
}

// recorder passes each request on to the supplier and keeps it, with the
// category a selection request answered.
type recorder struct {
	intl     curly.Intl
	mu       sync.Mutex
	requests []conformance.Request
}

func (r *recorder) record(request conformance.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, request)
}

func (r *recorder) NumberFormat(locale string, options map[string]any, input float64) (string, error) {
	r.record(conformance.Request{API: conformance.NumberFormat, Options: maps.Clone(options), Input: input})

	return r.intl.NumberFormat(locale, options, input)
}

func (r *recorder) DateTimeFormat(locale string, options map[string]any, input float64) (string, error) {
	r.record(conformance.Request{API: conformance.DateTimeFormat, Options: maps.Clone(options), Input: input})

	return r.intl.DateTimeFormat(locale, options, input)
}

func (r *recorder) RelativeTimeFormat(locale string, options map[string]any, value float64, unit string) (string, error) {
	r.record(conformance.Request{API: conformance.RelativeTimeFormat, Options: maps.Clone(options), Input: []any{value, unit}})

	return r.intl.RelativeTimeFormat(locale, options, value, unit)
}

func (r *recorder) PluralRules(locale string, options map[string]any, input float64) (curly.Category, error) {
	category, err := r.intl.PluralRules(locale, options, input)
	r.record(conformance.Request{API: conformance.PluralRules, Options: maps.Clone(options), Input: input, Category: string(category)})

	return category, err
}

// parse is the parser's tree in the runner's terms, converted on a stack of
// its own: a message of nested placeholders is as deep as it is long.
func parse(message string) *conformance.Node {
	type job struct {
		from *curly.Node
		to   *conformance.Node
	}

	tree := curly.CST(message)
	root := &conformance.Node{}
	jobs := []job{{&tree, root}}

	for len(jobs) > 0 {
		next := jobs[len(jobs)-1]
		jobs = jobs[:len(jobs)-1]
		from, to := next.from, next.to
		to.Type, to.Start, to.End = string(from.Kind), from.Start, from.End

		switch from.Kind {
		case curly.KindKey, curly.KindModifier, curly.KindOptionKey:
			name := from.Name
			to.Name = &name
		case curly.KindEscape:
			cancels := from.Cancels
			to.Cancels = &cancels
		}

		if len(from.Nodes) == 0 {
			continue
		}

		to.Nodes = make([]*conformance.Node, len(from.Nodes))

		for i := range from.Nodes {
			to.Nodes[i] = &conformance.Node{}
			jobs = append(jobs, job{&from.Nodes[i], to.Nodes[i]})
		}
	}

	return root
}
