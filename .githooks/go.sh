#!/bin/sh
# The check of `go/`, run by the pre-commit hook from inside `go/`: the
# formatting, vet and ASCII checks of the Lint job of `tests-go.yml`, over
# every module under `go/`. Staticcheck and `go mod tidy -diff`, which fetch
# modules, are left to that job.

if ! command -v go > /dev/null 2>&1; then
  echo 'go: go is not on the PATH: install Go 1.26 or later (https://go.dev/dl/).' >&2
  exit 75
fi

export GOWORK=off

# A Go older than the one go.mod requires, where the go command may not
# download that one, is a missing toolchain rather than a finding.
if ! error=$(go list -m 2>&1 > /dev/null); then
  case $error in
    *'requires go >='* | *'download go'*)
      echo "$error" >&2
      echo 'go: the Go go.mod requires is not installed: install Go 1.26 or later (https://go.dev/dl/), or let the go command download it (GOTOOLCHAIN=auto).' >&2
      exit 75
      ;;
  esac
  echo "$error" >&2
  exit 1
fi

# The gofmt of the Go the module selects, which may not be the one on the PATH.
goroot=$(go env GOROOT) || exit 1
unformatted=$("$goroot/bin/gofmt" -l .) || exit 1
if [ -n "$unformatted" ]; then
  echo 'go: gofmt would rewrite these files; gofmt -w . in go/ does:' >&2
  echo "$unformatted" >&2
  exit 1
fi

# Listed from the top of the work tree: git runs a hook with GIT_DIR set, so
# from here it would take `go/` for the top.
for module in $(git -C .. ls-files go/go.mod 'go/*/go.mod' | sed -e 's#^go/##' -e 's#/\{0,1\}go.mod$##' -e 's#^$#.#'); do
  go -C "$module" vet ./... || exit 1
done

# A character outside ASCII is written as an escape.
if files=$(LC_ALL=C grep -rl --include='*.go' "$(printf '[\200-\377]')" .); then
  echo 'go: these Go files hold a character outside ASCII; write it as an escape:' >&2
  echo "$files" >&2
  exit 1
fi
