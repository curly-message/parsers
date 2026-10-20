package conformance

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// The release of the set this runner vendors: the archive attached to the
// conformance-v4.1.0 release of curly-message/spec, and the sha256 its
// release notes state.
const (
	archivePath   = "set/conformance-4.1.0.zip"
	archiveRoot   = "conformance-4.1.0/"
	archiveDigest = "9011c603dd2a87f563557c60f32a8216f99cb1935dfcf54199f309e31496fbfd"
)

// Environment names the variable that points a run at a set under
// development instead of the vendored release: a directory laid out as the
// conformance package is, with the fixtures under fixtures/ and the defect
// catalogue beside them.
const Environment = "CURLY_CONFORMANCE_FIXTURES"

// format is the versioned identifier of the format this set reads.
const format = "curly-message-3"

//go:embed set/conformance-4.1.0.zip
var vendored embed.FS

// Fixture is one fixture file of the set, as JSON decodes it, under the name
// it is shipped as.
type Fixture struct {
	Name string
	File map[string]any
}

// Set is a conformance set: its version, its fixture files sorted by name,
// and its defect catalogue.
type Set struct {
	Version  string
	Fixtures []Fixture
	Defects  []Defect
}

// Vendored is the release this runner vendors, its archive checked against
// the digest the release states before anything in it is read.
var Vendored = sync.OnceValues(func() (*Set, error) {
	data, err := vendored.ReadFile(archivePath)

	if err != nil {
		return nil, err
	}

	return readArchive(data, archiveDigest)
})

// FromEnvironment is the set a run reads: the directory Environment names
// where it is set, and the vendored release otherwise.
func FromEnvironment() (*Set, error) {
	if dir := os.Getenv(Environment); dir != "" {
		return Load(dir)
	}

	return Vendored()
}

func readArchive(data []byte, digest string) (*Set, error) {
	sum := sha256.Sum256(data)

	if got := hex.EncodeToString(sum[:]); got != digest {
		return nil, fmt.Errorf("the archive's sha256 is %s; the release states %s", got, digest)
	}

	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))

	if err != nil {
		return nil, err
	}

	files := map[string][]byte{}

	for _, entry := range archive.File {
		name, ok := strings.CutPrefix(entry.Name, archiveRoot)

		if !ok || strings.HasSuffix(name, "/") {
			continue
		}

		r, err := entry.Open()

		if err != nil {
			return nil, err
		}

		contents, err := io.ReadAll(r)
		r.Close()

		if err != nil {
			return nil, err
		}

		files[name] = contents
	}

	return assemble(files)
}

// Load reads a set from a directory laid out as the conformance package is:
// fixtures/*.json, defects.json and index.json.
func Load(dir string) (*Set, error) {
	files := map[string][]byte{}
	names, err := filepath.Glob(filepath.Join(dir, "fixtures", "*.json"))

	if err != nil {
		return nil, err
	}

	for _, path := range append(names, filepath.Join(dir, "defects.json"), filepath.Join(dir, "index.json")) {
		contents, err := os.ReadFile(path)

		if err != nil {
			return nil, err
		}

		rel, err := filepath.Rel(dir, path)

		if err != nil {
			return nil, err
		}

		files[filepath.ToSlash(rel)] = contents
	}

	return assemble(files)
}

// assemble reads the set out of its files, each checked against its schema
// before it is read.
func assemble(files map[string][]byte) (*Set, error) {
	set := &Set{}
	manifest, err := checked("index.json", files["index.json"], checkManifest)

	if err != nil {
		return nil, err
	}

	set.Version, _ = manifest["version"].(string)

	var names []string

	for name := range files {
		if strings.HasPrefix(name, "fixtures/") && strings.HasSuffix(name, ".json") {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	for _, name := range names {
		file, err := checked(name, files[name], checkFixtureFile)

		if err != nil {
			return nil, err
		}

		set.Fixtures = append(set.Fixtures, Fixture{Name: strings.TrimPrefix(name, "fixtures/"), File: file})
	}

	if err := listed(manifest, set.Fixtures); err != nil {
		return nil, err
	}

	catalogue, err := checked("defects.json", files["defects.json"], checkCatalogue)

	if err != nil {
		return nil, err
	}

	for _, entry := range catalogue["defects"].([]any) {
		d := entry.(map[string]any)
		document, _ := d["document"].(string)
		section, _ := d["section"].(string)

		set.Defects = append(set.Defects, Defect{
			ID:          d["id"].(string),
			Description: d["description"].(string),
			Document:    document,
			Section:     section,
			Expects:     Verdict(d["expects"].(string)),
		})
	}

	return set, nil
}

// listed holds the manifest to the files: each file listed once, with the
// level or kind, the section and the number of cases it holds, and each
// case's id unique across the set.
func listed(manifest map[string]any, fixtures []Fixture) error {
	entries := map[string]map[string]any{}

	for _, entry := range manifest["files"].([]any) {
		members := entry.(map[string]any)
		path := members["path"].(string)

		if _, twice := entries[path]; twice {
			return fmt.Errorf("index.json lists %s twice", path)
		}

		entries[path] = members
	}

	if len(entries) != len(fixtures) {
		return fmt.Errorf("index.json lists %d files; the set holds %d", len(entries), len(fixtures))
	}

	ids := map[string]string{}

	for _, fixture := range fixtures {
		entry, ok := entries["fixtures/"+fixture.Name]

		if !ok {
			return fmt.Errorf("index.json does not list fixtures/%s", fixture.Name)
		}

		cases := fixture.File["cases"].([]any)

		for _, key := range []string{"level", "kind", "section"} {
			if entry[key] != fixture.File[key] {
				return fmt.Errorf("index.json gives fixtures/%s the %s %s; the file states %s", fixture.Name, key, shown(entry[key]), shown(fixture.File[key]))
			}
		}

		if entry["cases"] != float64(len(cases)) {
			return fmt.Errorf("index.json gives fixtures/%s %s cases; the file holds %d", fixture.Name, shown(entry["cases"]), len(cases))
		}

		for _, c := range cases {
			id := c.(map[string]any)["id"].(string)

			if file, twice := ids[id]; twice {
				return fmt.Errorf("the case %s is in fixtures/%s and in fixtures/%s", id, file, fixture.Name)
			}

			ids[id] = fixture.Name
		}
	}

	return nil
}

// checked decodes one file of the set and holds it to its schema.
func checked(name string, data []byte, check func(*checker, any)) (map[string]any, error) {
	if data == nil {
		return nil, fmt.Errorf("the set holds no %s", name)
	}

	if err := duplicateKey(data); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	var value any

	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	c := &checker{}

	check(c, value)

	if c.err != nil {
		return nil, fmt.Errorf("%s: %w", name, c.err)
	}

	return value.(map[string]any), nil
}

// Without is the set less the cases the ids name, its version and catalogue
// kept.
func (s *Set) Without(ids ...string) *Set {
	drop := map[string]bool{}

	for _, id := range ids {
		drop[id] = true
	}

	out := &Set{Version: s.Version, Defects: s.Defects}

	for _, f := range s.Fixtures {
		file := map[string]any{}

		for k, v := range f.File {
			file[k] = v
		}

		var kept []any

		for _, c := range f.File["cases"].([]any) {
			if id, _ := c.(map[string]any)["id"].(string); !drop[id] {
				kept = append(kept, c)
			}
		}

		file["cases"] = kept
		out.Fixtures = append(out.Fixtures, Fixture{Name: f.Name, File: file})
	}

	return out
}
