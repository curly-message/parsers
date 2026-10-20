// Command pack builds a module's zip as the Go module proxy builds it: from
// a revision of the repository, through git archive, so that what is not
// committed at that revision, build output and untracked files among it,
// never enters it. Run from go/dev:
//
//	go run ./pack -module go -version v1.0.0 [-repo ../..] [-rev HEAD] [-proxy <dir>] [-hash]
//
// -proxy writes the module's .info, .mod and .zip into <dir> laid out as a
// GOPROXY=file://<dir> serves them, so a consumer can go get the version as
// it would from the proxy; -hash prints the zip's h1: hash, the Sum the
// proxy and the checksum database record for that version.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	"golang.org/x/mod/zip"
)

func git(repo string, args ...string) ([]byte, error) {
	var stderr bytes.Buffer

	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	cmd.Stderr = &stderr

	out, err := cmd.Output()

	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return out, nil
}

func run() error {
	repo := flag.String("repo", "../..", "the repository's root")
	rev := flag.String("rev", "HEAD", "the revision to pack")
	dir := flag.String("module", "", "the module's directory in the repository, as go or go/intl")
	version := flag.String("version", "", "the version to pack it as")
	proxy := flag.String("proxy", "", "a directory to write the module into, laid out as a module proxy")
	hash := flag.Bool("hash", false, "print the zip's h1: hash")

	flag.Parse()

	if *dir == "" || *version == "" || flag.NArg() > 0 || (*proxy == "" && !*hash) {
		flag.Usage()

		return errors.New("-module, -version and one of -proxy and -hash are required")
	}

	root, err := filepath.Abs(*repo)

	if err != nil {
		return err
	}

	subdir := strings.Trim(filepath.ToSlash(*dir), "/")
	gomod, err := git(root, "show", *rev+":"+subdir+"/go.mod")

	if err != nil {
		return err
	}

	path := modfile.ModulePath(gomod)

	if path == "" {
		return fmt.Errorf("%s/go.mod at %s names no module", subdir, *rev)
	}

	m := module.Version{Path: path, Version: *version}

	if err := module.Check(m.Path, m.Version); err != nil {
		return err
	}

	stamp, err := git(root, "log", "-1", "--format=%ct", *rev)

	if err != nil {
		return err
	}

	var seconds int64

	if _, err := fmt.Sscan(string(stamp), &seconds); err != nil {
		return fmt.Errorf("the commit time of %s: %w", *rev, err)
	}

	var archive bytes.Buffer

	if err := zip.CreateFromVCS(&archive, m, root, *rev, subdir); err != nil {
		return err
	}

	target := *proxy

	if target == "" {
		temporary, err := os.MkdirTemp("", "pack")

		if err != nil {
			return err
		}

		defer os.RemoveAll(temporary)

		target = temporary
	}

	escapedPath, err := module.EscapePath(m.Path)

	if err != nil {
		return err
	}

	escapedVersion, err := module.EscapeVersion(m.Version)

	if err != nil {
		return err
	}

	at := filepath.Join(target, filepath.FromSlash(escapedPath), "@v")

	if err := os.MkdirAll(at, 0o755); err != nil {
		return err
	}

	info, err := json.Marshal(map[string]string{"Version": m.Version, "Time": time.Unix(seconds, 0).UTC().Format(time.RFC3339)})

	if err != nil {
		return err
	}

	base := filepath.Join(at, escapedVersion)

	for _, file := range []struct {
		name string
		data []byte
	}{{base + ".info", info}, {base + ".mod", gomod}, {base + ".zip", archive.Bytes()}} {
		if err := os.WriteFile(file.name, file.data, 0o644); err != nil {
			return err
		}
	}

	if *proxy != "" {
		list, err := os.OpenFile(filepath.Join(at, "list"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)

		if err != nil {
			return err
		}

		if _, err := fmt.Fprintln(list, m.Version); err != nil {
			list.Close()

			return err
		}

		if err := list.Close(); err != nil {
			return err
		}
	}

	if *hash {
		sum, err := dirhash.HashZip(base+".zip", dirhash.Hash1)

		if err != nil {
			return err
		}

		fmt.Println(sum)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pack:", err)
		os.Exit(1)
	}
}
