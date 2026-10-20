#!/bin/sh
# The check of `js/`, run by the pre-commit hook from inside `js/`.

if ! command -v npm > /dev/null 2>&1; then
  echo 'js: npm is not on the PATH: install Node.js 22 or later (https://nodejs.org), then run npm ci in js/.' >&2
  exit 75
fi

if [ ! -d node_modules ]; then
  echo 'js: js/node_modules is missing: run npm ci in js/.' >&2
  exit 75
fi

exec npm run lint
