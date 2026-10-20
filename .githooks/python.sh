#!/bin/sh
# The check of `python/`, run by the pre-commit hook from inside `python/`:
# the ruff and mypy checks of the Lint job of `tests-python.yml`. The suite is
# left to the workflow.

if ! command -v uv > /dev/null 2>&1; then
  echo 'python: uv is not on the PATH: install uv (https://docs.astral.sh/uv/getting-started/installation/).' >&2
  exit 75
fi

if ! uv run --no-project tasks.py lint; then
  echo 'python: ruff reports what is shown above; uv run --no-project tasks.py lint-fix in python/ fixes what it can.' >&2
  exit 1
fi

uv run --no-project tasks.py typecheck || exit 1
