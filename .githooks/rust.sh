#!/bin/sh
# The check of `rust/`, run by the pre-commit hook from inside `rust/`: the
# format, clippy and harness checks of the Lint job of `tests-cargo.yml`. The
# docs and the surface check, which that job also runs, and the suite are left
# to the workflow.

if ! command -v cargo > /dev/null 2>&1; then
  echo 'rust: cargo is not on the PATH: install Rust 1.88 or later with rustup (https://rustup.rs).' >&2
  exit 75
fi

# A Rust older than the one Cargo.toml requires is a missing toolchain rather
# than a finding.
required=$(sed -n 's/^rust-version = "\([0-9]*\.[0-9]*\).*"$/\1/p' Cargo.toml)
found=$(rustc --version 2> /dev/null | sed -n 's/^rustc \([0-9]*\.[0-9]*\).*/\1/p')
if [ -z "$found" ] || [ "${found%%.*}" -lt "${required%%.*}" ] ||
  { [ "${found%%.*}" -eq "${required%%.*}" ] && [ "${found#*.}" -lt "${required#*.}" ]; }; then
  echo "rust: Rust $required or later is not installed: rustup toolchain install stable (https://rustup.rs)." >&2
  exit 75
fi

if ! cargo fmt --version > /dev/null 2>&1; then
  echo 'rust: rustfmt is not installed: rustup component add rustfmt.' >&2
  exit 75
fi

if ! cargo clippy --version > /dev/null 2>&1; then
  echo 'rust: clippy is not installed: rustup component add clippy.' >&2
  exit 75
fi

if ! cargo fmt --all --check; then
  echo 'rust: rustfmt would rewrite what is shown above; cargo fmt --all in rust/ does.' >&2
  exit 1
fi

# Once without features and once with every one, so the code each leaves out
# is linted by the other.
cargo clippy --workspace --all-targets --locked --quiet -- -D warnings || exit 1
cargo clippy --workspace --all-targets --all-features --locked --quiet -- -D warnings || exit 1

if ! cmp -s bench/harness.mjs ../js/bench/harness.mjs || ! diff -r bench/check ../js/bench/check > /dev/null; then
  echo "rust: bench/harness.mjs and bench/check/ are the family's: copy them from js/bench/." >&2
  exit 1
fi
