package conformance

import (
	"strings"
	"testing"
)

const resolutionFile = `{"format": "curly-message-3", "level": "core", "section": "3", "cases": [
	{"id": "a/concrete", "description": "Pins a case.", "section": "3.1", "message": "{{v}}", "payload": {"v": 1}, "props": {}, "locale": "en", "messageId": "m", "modifiers": {"m": "upper"}, "defaults": {}, "expected": {"output": "1", "reports": [{"code": "unknown-modifier", "origin": "message", "id": "m"}]}},
	{"id": "a/format", "description": "Pins a request.", "message": "{{v:number}}", "expected": {"format": {"api": "NumberFormat", "options": {}, "input": 1}}},
	{"id": "a/plural", "description": "Pins a selection.", "message": "{{v:plural}}", "expected": {"format": {"api": "PluralRules", "input": 1}, "categories": {"zero": "", "one": "", "two": "", "few": "", "many": "", "other": ""}}},
	{"id": "a/generated", "description": "Pins a construction.", "generate": "nesting-at-limit"}
]}`

const treeFile = `{"format": "curly-message-3", "kind": "tree", "section": "6", "cases": [
	{"id": "t/tree", "description": "Pins a tree.", "message": "a", "resolves": "a", "expected": [{"type": "placeholder", "text": "a", "nodes": [{"type": "key", "text": "a", "name": "a"}, {"type": "escape", "text": "", "cancels": true}]}]}
]}`

const catalogueFile = `{"format": "curly-message-3", "defects": [{"id": "a-defect", "description": "Breaks.", "document": "CST.md", "section": "4", "expects": "fail"}]}`

const manifestFile = `{"format": "curly-message-3", "version": "4.1.0", "files": [{"path": "fixtures/a.json", "level": "core", "section": "3", "cases": 4}, {"path": "fixtures/t.json", "kind": "tree", "section": "6", "cases": 1}]}`

func fixtureError(text string) error {
	_, err := checked("f.json", []byte(text), checkFixtureFile)

	return err
}

// edited is the document with one span of it replaced, which must occur.
func edited(t testing.TB, document, old, replacement string) string {
	t.Helper()

	if !strings.Contains(document, old) {
		t.Fatalf("the document holds no %s", old)
	}

	return strings.Replace(document, old, replacement, 1)
}

func TestSchema(t *testing.T) {
	t.Run("passes each kind of document the schema describes", func(t *testing.T) {
		for name, err := range map[string]error{
			"resolution": fixtureError(resolutionFile),
			"tree":       fixtureError(treeFile),
			"catalogue":  func() error { _, err := checked("d.json", []byte(catalogueFile), checkCatalogue); return err }(),
			"manifest":   func() error { _, err := checked("i.json", []byte(manifestFile), checkManifest); return err }(),
		} {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	})

	for _, c := range []struct {
		name, document, old, replacement, err string
	}{
		{"a key the file does not name", resolutionFile, `"level"`, `"extra": 1, "level"`, `the fixture file holds the key "extra", which its schema does not name`},
		{"a key that differs by case", resolutionFile, `"expected": {"output": "1"`, `"Expected": {"output": "1"`, `the file.cases[0]: the case holds the key "Expected", which its schema does not name`},
		{"a missing key", resolutionFile, `"description": "Pins a case.", `, ``, `the file.cases[0]: the case lacks the key "description"`},
		{"a key an expectation does not name", resolutionFile, `"output": "1", `, `"output": "1", "extra": 1, `, `the expectation holds the key "extra"`},
		{"a key a report does not name", resolutionFile, `"id": "m"}`, `"id": "m", "text": ""}`, `the report holds the key "text"`},
		{"a report with no origin", resolutionFile, `, "origin": "message"`, ``, `the report lacks the key "origin"`},
		{"a code no report carries", resolutionFile, `"unknown-modifier"`, `"bogus"`, `"bogus" is none of`},
		{"a key a format does not name", resolutionFile, `"input": 1}}}`, `"input": 1, "locale": "en"}}}`, `the format holds the key "locale"`},
		{"categories short of one", resolutionFile, `"many": "", `, ``, `the categories lacks the key "many"`},
		{"an output and a format", resolutionFile, `{"format": {"api": "NumberFormat"`, `{"output": "", "format": {"api": "NumberFormat"`, `the file.cases[1].expected: the expectation states an output or a format, and not both`},
		{"neither an output nor a format", resolutionFile, `"expected": {"output": "1", `, `"expected": {`, `the file.cases[0].expected: the expectation states an output or a format, and not both`},
		{"categories beside a formatting request", resolutionFile, `"options": {}, "input": 1}}`, `"options": {}, "input": 1}, "categories": {}}`, `the file.cases[1].expected: the expectation names categories where, and only where, it states a PluralRules request`},
		{"a selection request with no categories", resolutionFile, `, "categories": {"zero": "", "one": "", "two": "", "few": "", "many": "", "other": ""}`, ``, `the file.cases[2].expected: the expectation names categories where, and only where`},
		{"an unknown construction", resolutionFile, `"nesting-at-limit"`, `"nesting-past-limit"`, `the file.cases[3].generate: "nesting-past-limit" is none of`},
		{"a generated case with a message", resolutionFile, `"generate": "nesting-at-limit"`, `"generate": "nesting-at-limit", "message": ""`, `the generated case holds the key "message"`},
		{"a behaviour no modifier has", resolutionFile, `{"m": "upper"}`, `{"m": "lower"}`, `the file.cases[0].modifiers.m: "lower" is none of`},
		{"a modifier with no name", resolutionFile, `{"m": "upper"}`, `{"": "upper"}`, `registers a modifier under the empty name`},
		{"an id outside the pattern", resolutionFile, `"a/concrete"`, `"A/concrete"`, `"A/concrete" does not match`},
		{"another format", resolutionFile, `"curly-message-3"`, `"curly-message-2"`, `the file.format: the file targets the format "curly-message-2"; this set reads curly-message-3`},
		{"no cases", treeFile, `"cases": [`, `"cases": [], "x": [`, `the tree fixture file holds the key "x"`},
		{"a node with no text", treeFile, `{"type": "key", "text": "a", "name": "a"}`, `{"type": "key", "name": "a"}`, `the expected node lacks the key "text"`},
		{"a key a node does not name", treeFile, `"cancels": true}`, `"cancels": true, "start": 0}`, `the expected node holds the key "start"`},
		{"a kind no node has", treeFile, `"type": "escape"`, `"type": "word"`, `"word" is none of`},
		{"a key a defect does not name", catalogueFile, `"expects"`, `"level": "core", "expects"`, `the defect holds the key "level"`},
		{"a verdict no defect expects", catalogueFile, `"expects": "fail"`, `"expects": "crash"`, `"crash" is none of`},
		{"a key a manifest entry does not name", manifestFile, `"cases": 4}`, `"cases": 4, "size": 1}`, `the manifest entry holds the key "size"`},
		{"a count that is not one", manifestFile, `"cases": 4}`, `"cases": 4.5}`, `4.5 is not a count`},
	} {
		t.Run("refuses "+c.name, func(t *testing.T) {
			document := edited(t, c.document, c.old, c.replacement)
			var err error

			switch c.document {
			case catalogueFile:
				_, err = checked("d.json", []byte(document), checkCatalogue)
			case manifestFile:
				_, err = checked("i.json", []byte(document), checkManifest)
			default:
				err = fixtureError(document)
			}

			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("got %v, want an error holding: %s", err, c.err)
			}
		})
	}

	t.Run("refuses a key held twice at any depth, where decoding would keep the last", func(t *testing.T) {
		for _, document := range []string{
			edited(t, resolutionFile, `"level": "core"`, `"level": "core", "level": "intl"`),
			edited(t, resolutionFile, `"payload": {"v": 1}`, `"payload": {"v": 1, "w": [{"x": 1, "x": 2}]}`),
		} {
			if err := fixtureError(document); err == nil || !strings.Contains(err.Error(), "twice") {
				t.Errorf("got %v", err)
			}
		}

		if err := duplicateKey([]byte(`{"a": {"b": 1}, "b": [{"a": 1}, {"a": 1}], "c": {"a": {}}}`)); err != nil {
			t.Errorf("one key in two objects: %v", err)
		}
	})

	t.Run("walks deep nodes on a stack of its own", func(t *testing.T) {
		depth := 3000
		nodes := strings.Repeat(`{"type": "placeholder", "text": "", "nodes": [`, depth) + strings.Repeat(`]}`, depth)
		document := `{"format": "curly-message-3", "kind": "tree", "section": "6", "cases": [{"id": "t/deep", "description": "Pins depth.", "message": "", "expected": [` + nodes + `]}]}`

		if err := fixtureError(document); err != nil {
			t.Error(err)
		}

		if err := fixtureError(strings.Replace(document, `"text": ""`, `"text": 1`, 1)); err == nil {
			t.Error("passed a node with no text at the top of a deep tree")
		}
	})
}
