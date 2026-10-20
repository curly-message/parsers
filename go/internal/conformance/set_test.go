package conformance

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unpacked writes the vendored release into a directory laid out as the
// conformance package is.
func unpacked(t testing.TB) string {
	t.Helper()

	data, err := vendored.ReadFile(archivePath)

	if err != nil {
		t.Fatal(err)
	}

	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))

	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	for _, entry := range archive.File {
		name, _ := strings.CutPrefix(entry.Name, archiveRoot)

		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}

		r, err := entry.Open()

		if err != nil {
			t.Fatal(err)
		}

		var contents bytes.Buffer
		_, err = contents.ReadFrom(r)
		r.Close()

		if err != nil {
			t.Fatal(err)
		}

		path := filepath.Join(dir, filepath.FromSlash(name))

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, contents.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func caseCount(set *Set) int {
	n := 0

	for _, f := range set.Fixtures {
		n += len(f.File["cases"].([]any))
	}

	return n
}

// rewrite replaces one span of a file of the set, which must occur.
func rewrite(t testing.TB, dir, name, old, replacement string) {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(name))
	contents, err := os.ReadFile(path)

	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(edited(t, string(contents), old, replacement)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSet(t *testing.T) {
	t.Run("reads the vendored release", func(t *testing.T) {
		set, err := Vendored()

		if err != nil {
			t.Fatal(err)
		}

		if set.Version != "4.1.0" || len(set.Fixtures) != 18 || caseCount(set) != 771 || len(set.Defects) != 26 {
			t.Errorf("got %s, %d files, %d cases, %d defects", set.Version, len(set.Fixtures), caseCount(set), len(set.Defects))
		}
	})

	t.Run("refuses an archive its digest does not state", func(t *testing.T) {
		data, _ := vendored.ReadFile(archivePath)
		altered := bytes.Clone(data)
		altered[len(altered)-1] ^= 1

		for _, c := range []struct {
			data   []byte
			digest string
		}{{altered, archiveDigest}, {data, strings.Repeat("0", 64)}} {
			if _, err := readArchive(c.data, c.digest); err == nil || !strings.HasPrefix(err.Error(), "the archive's sha256 is ") {
				t.Errorf("got %v", err)
			}
		}
	})

	t.Run("reads a directory as it reads the release", func(t *testing.T) {
		vendored, _ := Vendored()
		loaded, err := Load(unpacked(t))

		if err != nil {
			t.Fatal(err)
		}

		if loaded.Version != vendored.Version || caseCount(loaded) != caseCount(vendored) || len(loaded.Defects) != len(vendored.Defects) {
			t.Errorf("got %s, %d cases, %d defects", loaded.Version, caseCount(loaded), len(loaded.Defects))
		}

		for i := range loaded.Fixtures {
			if loaded.Fixtures[i].Name != vendored.Fixtures[i].Name {
				t.Errorf("file %d: %s, %s", i, loaded.Fixtures[i].Name, vendored.Fixtures[i].Name)
			}
		}
	})

	t.Run("reads the directory the environment names, and the release without one", func(t *testing.T) {
		dir := unpacked(t)
		rewrite(t, dir, "index.json", `"version": "4.1.0"`, `"version": "4.2.0-dev"`)
		t.Setenv(Environment, dir)

		if set, err := FromEnvironment(); err != nil || set.Version != "4.2.0-dev" {
			t.Errorf("got %v, %v", set, err)
		}

		t.Setenv(Environment, "")

		if set, err := FromEnvironment(); err != nil || set.Version != "4.1.0" {
			t.Errorf("got %v, %v", set, err)
		}

		t.Setenv(Environment, filepath.Join(dir, "absent"))

		if _, err := FromEnvironment(); err == nil {
			t.Error("read a directory that is not there")
		}
	})

	for _, c := range []struct {
		name, file, old, replacement, err string
	}{
		{"a file the manifest does not list", "index.json", `"path": "fixtures/cst.json"`, `"path": "fixtures/other.json"`, "index.json does not list fixtures/cst.json"},
		{"a file listed twice", "index.json", `"path": "fixtures/limits.json"`, `"path": "fixtures/cst.json"`, "index.json lists fixtures/cst.json twice"},
		{"another count of cases", "index.json", `"section": "13",
      "cases": 8`, `"section": "13",
      "cases": 9`, "index.json gives fixtures/limits.json 9 cases; the file holds 8"},
		{"another level", "index.json", `"path": "fixtures/limits.json",
      "level": "core"`, `"path": "fixtures/limits.json",
      "level": "intl"`, `index.json gives fixtures/limits.json the level "intl"; the file states "core"`},
		{"an id two files hold", "fixtures/limits.json", `"id": "limits/output-at-limit"`, `"id": "grammar/literal-text"`, "the case grammar/literal-text is in fixtures/grammar.json and in fixtures/limits.json"},
		{"a file outside its schema", "fixtures/limits.json", `"level"`, `"Level": "core", "level"`, `fixtures/limits.json: the file: the fixture file holds the key "Level"`},
		{"a catalogue outside its schema", "defects.json", `"defects"`, `"version": "4.1.0", "defects"`, `defects.json: the file: the catalogue holds the key "version"`},
	} {
		t.Run("refuses "+c.name, func(t *testing.T) {
			dir := unpacked(t)
			rewrite(t, dir, c.file, c.old, c.replacement)

			if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("got %v, want an error holding: %s", err, c.err)
			}
		})
	}

	t.Run("refuses a directory missing a file the set holds", func(t *testing.T) {
		dir := unpacked(t)

		if err := os.Remove(filepath.Join(dir, "defects.json")); err != nil {
			t.Fatal(err)
		}

		if _, err := Load(dir); err == nil {
			t.Error("read a set with no catalogue")
		}
	})

	t.Run("leaves out the cases it names and keeps the rest", func(t *testing.T) {
		set, _ := Vendored()
		without := set.Without("limits/output-at-limit", "grammar/literal-text", "no/such-case")

		if caseCount(without) != caseCount(set)-2 || len(without.Fixtures) != len(set.Fixtures) || len(without.Defects) != len(set.Defects) {
			t.Errorf("got %d cases", caseCount(without))
		}

		if caseCount(set) != 771 {
			t.Error("leaving out changed the set it was taken from")
		}
	})
}
