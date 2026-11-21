#!/bin/sh
# Builds the wheel and installs it with the `intl` extra into `.bench-venv`,
# which `bench/rows.mjs` reads. A benchmark workflow runs this from `python/`
# on each side of a comparison, in place of npm.

exec uv run --no-project tasks.py bench-venv
