package conformance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
)

// The set's files are held to schema/fixture.schema.json and
// schema/defect.schema.json by hand, since the standard library has no JSON
// Schema engine. A file is decoded into maps and slices and never into
// structs, because encoding/json matches a struct's fields without regard to
// case: "Expected" would read as "expected", and a case the schema refuses
// would run. Each object is held to the exact keys its site names.

// The patterns of the schema. Go's regexp runs in time linear in its input,
// so neither can backtrack over a file.
var (
	sectionPattern    = regexp.MustCompile(`^([0-9]+(\.[0-9]+)?|A\.[0-9]+)$`)
	identifierPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*/[a-z0-9]+(-[a-z0-9]+)*$`)
	defectPattern     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

// checker walks a decoded file against its schema on a stack of its own,
// keeping the first departure it meets.
type checker struct {
	err  error
	work []item
}

type item struct {
	path  string
	value any
	rule  rule
}

// A rule checks one value, pushing what it holds onto the checker.
type rule func(c *checker, path string, value any)

func (c *checker) fail(path, format string, args ...any) {
	if c.err == nil {
		c.err = fmt.Errorf("%s: %s", path, fmt.Sprintf(format, args...))
	}
}

func (c *checker) push(path string, value any, r rule) {
	c.work = append(c.work, item{path, value, r})
}

func (c *checker) run(value any, r rule) {
	c.push("the file", value, r)

	for len(c.work) > 0 && c.err == nil {
		next := c.work[len(c.work)-1]
		c.work = c.work[:len(c.work)-1]
		next.rule(c, next.path, next.value)
	}
}

func anything(*checker, string, any) {}

func text(c *checker, path string, value any) {
	if _, ok := value.(string); !ok {
		c.fail(path, "%s is not a string", shown(value))
	}
}

func nonEmpty(c *checker, path string, value any) {
	if s, ok := value.(string); !ok || s == "" {
		c.fail(path, "%s is not a non-empty string", shown(value))
	}
}

func boolean(c *checker, path string, value any) {
	if _, ok := value.(bool); !ok {
		c.fail(path, "%s is not a boolean", shown(value))
	}
}

func object(c *checker, path string, value any) {
	if _, ok := value.(map[string]any); !ok {
		c.fail(path, "%s is not an object", shown(value))
	}
}

func count(c *checker, path string, value any) {
	if n, ok := value.(float64); !ok || n < 0 || n != float64(int(n)) {
		c.fail(path, "%s is not a count", shown(value))
	}
}

func matching(pattern *regexp.Regexp) rule {
	return func(c *checker, path string, value any) {
		if s, ok := value.(string); !ok || !pattern.MatchString(s) {
			c.fail(path, "%s does not match %s", shown(value), pattern)
		}
	}
}

func oneOf(values ...string) rule {
	return func(c *checker, path string, value any) {
		if s, ok := value.(string); !ok || !slices.Contains(values, s) {
			c.fail(path, "%s is none of %s", shown(value), shown(values))
		}
	}
}

// formatted is the rule of every file's format, which names the file's
// format in the words the plan refuses one with.
func formatted(c *checker, path string, value any) {
	if value != format {
		c.fail(path, "the file targets the format %s; this set reads %s", shown(value), format)
	}
}

func list(least int, each rule) rule {
	return func(c *checker, path string, value any) {
		items, ok := value.([]any)

		if !ok {
			c.fail(path, "%s is not an array", shown(value))

			return
		}

		if len(items) < least {
			c.fail(path, "holds %d items, fewer than %d", len(items), least)

			return
		}

		for i := len(items) - 1; i >= 0; i-- {
			c.push(path+"["+strconv.Itoa(i)+"]", items[i], each)
		}
	}
}

// property is one property a site names.
type property struct {
	name string
	rule rule
}

// site is an object of the schema whose additionalProperties is false: the
// keys it names, those it requires, and what else it states of itself.
type site struct {
	what       string
	required   []string
	properties []property
	also       func(c *checker, path string, value map[string]any)
}

func (s *site) check(c *checker, path string, value any) {
	members, ok := value.(map[string]any)

	if !ok {
		c.fail(path, "the %s %s is not an object", s.what, shown(value))

		return
	}

	for _, key := range sortedKeys(members) {
		if !slices.ContainsFunc(s.properties, func(p property) bool { return p.name == key }) {
			c.fail(path, "the %s holds the key %s, which its schema does not name", s.what, shown(key))

			return
		}
	}

	for _, key := range s.required {
		if _, ok := members[key]; !ok {
			c.fail(path, "the %s lacks the key %s", s.what, shown(key))

			return
		}
	}

	if s.also != nil {
		s.also(c, path, members)
	}

	for i := len(s.properties) - 1; i >= 0; i-- {
		if member, ok := members[s.properties[i].name]; ok {
			c.push(path+"."+s.properties[i].name, member, s.properties[i].rule)
		}
	}
}

var (
	section    = matching(sectionPattern)
	identifier = matching(identifierPattern)
	level      = oneOf(string(LevelCore), string(LevelIntl), string(LevelExtensions))
	nodeType   = oneOf("message", "placeholder", "open", "close", "separator", "space", "key", "modifier", "option-key", "option-value", "text", "escape")
	behaviour  = oneOf("upper", "echo", "empty", "nothing", "raise", "default", "options", "props", "locale", "object")
)

// generators names the constructions a generated case may name, in the
// order README.md states them.
var generators = []string{
	"output-at-limit",
	"output-over-limit",
	"output-over-limit-continues",
	"read-at-limit",
	"read-over-limit",
	"conversion-over-limit",
	"nesting-at-limit",
	"nesting-over-limit",
}

func modifiers(c *checker, path string, value any) {
	members, ok := value.(map[string]any)

	if !ok {
		c.fail(path, "%s is not an object", shown(value))

		return
	}

	for _, name := range sortedKeys(members) {
		if name == "" {
			c.fail(path, "registers a modifier under the empty name")

			return
		}

		c.push(path+"."+name, members[name], behaviour)
	}
}

var categoriesSite = &site{
	what:     "categories",
	required: []string{"zero", "one", "two", "few", "many", "other"},
	properties: []property{
		{"zero", text}, {"one", text}, {"two", text}, {"few", text}, {"many", text}, {"other", text},
	},
}

var formatSite = &site{
	what:     "format",
	required: []string{"api", "input"},
	properties: []property{
		{"api", oneOf(string(NumberFormat), string(DateTimeFormat), string(RelativeTimeFormat), string(PluralRules))},
		{"options", object},
		{"input", anything},
	},
}

var reportSite = &site{
	what:     "report",
	required: []string{"code", "origin"},
	properties: []property{
		{"code", oneOf("unknown-modifier", "failed-modifier", "missing-options", "unserializable-value", "missing-locale", "nesting-limit", "output-limit", "read-limit")},
		{"origin", oneOf("message", "payload", "limit")},
		{"id", text},
	},
}

var expectedSite = &site{
	what: "expectation",
	properties: []property{
		{"output", text},
		{"format", formatSite.check},
		{"categories", categoriesSite.check},
		{"reports", list(0, reportSite.check)},
	},
	also: func(c *checker, path string, members map[string]any) {
		_, output := members["output"]
		request, format := members["format"]

		if output == format {
			c.fail(path, "the expectation states an output or a format, and not both")

			return
		}

		// The schema's if: a format that is an object whose api, where it
		// has one, is PluralRules.
		plural := false

		if fields, ok := request.(map[string]any); ok {
			api, named := fields["api"]
			plural = !named || api == string(PluralRules)
		}

		if _, categories := members["categories"]; categories != plural {
			c.fail(path, "the expectation names categories where, and only where, it states a PluralRules request")
		}
	},
}

var concreteSite = &site{
	what:     "case",
	required: []string{"id", "description", "message", "expected"},
	properties: []property{
		{"id", identifier},
		{"description", nonEmpty},
		{"section", section},
		{"message", anything},
		{"payload", object},
		{"props", object},
		{"locale", text},
		{"messageId", anything},
		{"modifiers", modifiers},
		{"defaults", object},
		{"expected", expectedSite.check},
	},
}

var generatedSite = &site{
	what:     "generated case",
	required: []string{"id", "description", "generate"},
	properties: []property{
		{"id", identifier},
		{"description", nonEmpty},
		{"section", section},
		{"generate", oneOf(generators...)},
	},
}

// A case is one of the two: a generated case names its construction, and
// the concrete site names no generate, so the key tells them apart.
func resolutionCase(c *checker, path string, value any) {
	if members, ok := value.(map[string]any); ok {
		if _, generated := members["generate"]; generated {
			generatedSite.check(c, path, value)

			return
		}
	}

	concreteSite.check(c, path, value)
}

var expectedNodeSite = &site{
	what:     "expected node",
	required: []string{"type", "text"},
}

func init() {
	expectedNodeSite.properties = []property{
		{"type", nodeType},
		{"text", text},
		{"name", text},
		{"cancels", boolean},
		{"nodes", list(0, expectedNodeSite.check)},
	}
}

var treeCaseSite = &site{
	what:     "tree case",
	required: []string{"id", "description", "message", "expected"},
	properties: []property{
		{"id", identifier},
		{"description", nonEmpty},
		{"section", section},
		{"message", text},
		{"expected", list(0, expectedNodeSite.check)},
		{"resolves", text},
	},
}

var resolutionFileSite = &site{
	what:     "fixture file",
	required: []string{"format", "level", "section", "cases"},
	properties: []property{
		{"format", formatted},
		{"kind", oneOf("resolution")},
		{"level", level},
		{"section", section},
		{"cases", list(1, resolutionCase)},
	},
}

var treeFileSite = &site{
	what:     "tree fixture file",
	required: []string{"format", "kind", "section", "cases"},
	properties: []property{
		{"format", formatted},
		{"kind", oneOf("tree")},
		{"section", section},
		{"cases", list(1, treeCaseSite.check)},
	},
}

// A file is one of the two: a tree file says so, and a resolution file
// says nothing else.
func checkFixtureFile(c *checker, value any) {
	c.run(value, func(c *checker, path string, value any) {
		if members, ok := value.(map[string]any); ok && members["kind"] == "tree" {
			treeFileSite.check(c, path, value)

			return
		}

		resolutionFileSite.check(c, path, value)
	})
}

var defectSite = &site{
	what:     "defect",
	required: []string{"id", "description", "expects"},
	properties: []property{
		{"id", matching(defectPattern)},
		{"description", nonEmpty},
		{"document", oneOf("SPEC.md", "CST.md")},
		{"section", section},
		{"expects", oneOf(string(VerdictError), string(VerdictFail), string(VerdictSkip), string(VerdictUnobserved))},
	},
}

var catalogueSite = &site{
	what:     "catalogue",
	required: []string{"format", "defects"},
	properties: []property{
		{"format", formatted},
		{"defects", list(1, defectSite.check)},
	},
}

func checkCatalogue(c *checker, value any) {
	c.run(value, catalogueSite.check)
}

// The manifest has no schema of its own; this is the shape README.md gives
// it, held as strictly as the files it lists.
var manifestEntrySite = &site{
	what:     "manifest entry",
	required: []string{"path", "level", "section", "cases"},
	properties: []property{
		{"path", nonEmpty},
		{"level", level},
		{"section", section},
		{"cases", count},
	},
}

var manifestTreeEntrySite = &site{
	what:     "manifest entry",
	required: []string{"path", "kind", "section", "cases"},
	properties: []property{
		{"path", nonEmpty},
		{"kind", oneOf("tree")},
		{"section", section},
		{"cases", count},
	},
}

var manifestSite = &site{
	what:     "manifest",
	required: []string{"format", "version", "files"},
	properties: []property{
		{"format", formatted},
		{"version", nonEmpty},
		{"files", list(1, func(c *checker, path string, value any) {
			if members, ok := value.(map[string]any); ok && members["kind"] == "tree" {
				manifestTreeEntrySite.check(c, path, value)

				return
			}

			manifestEntrySite.check(c, path, value)
		})},
	},
}

func checkManifest(c *checker, value any) {
	c.run(value, manifestSite.check)
}

// duplicateKey refuses an object that holds one key twice, which decoding
// into a map would settle silently by keeping the last.
func duplicateKey(data []byte) error {
	type frame struct {
		object bool
		key    bool
		keys   map[string]bool
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	var stack []frame
	valued := func() {
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].key = true
		}
	}

	for {
		token, err := decoder.Token()

		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err
		}

		if delim, ok := token.(json.Delim); ok {
			switch delim {
			case '{':
				stack = append(stack, frame{object: true, key: true, keys: map[string]bool{}})
			case '[':
				stack = append(stack, frame{})
			default:
				stack = stack[:len(stack)-1]
				valued()
			}

			continue
		}

		if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].key {
			key := token.(string)

			if stack[n-1].keys[key] {
				return fmt.Errorf("an object holds the key %s twice", shown(key))
			}

			stack[n-1].keys[key] = true
			stack[n-1].key = false

			continue
		}

		valued()
	}
}
