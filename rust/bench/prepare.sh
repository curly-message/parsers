#!/bin/sh
# Builds, from rust/, what bench/rows.mjs reads, into bench/dist/: `hello`, a
# program resolving one message without `intl`, and `hello-intl`, one going
# through every Intl modifier, whose stripped binaries the size rows weigh;
# `curly-message-bench`, the program that runs every other row; and the
# `.crate` a release ships.
set -e

metadata=$(cargo metadata --format-version 1 --no-deps --locked)
target=$(printf '%s' "$metadata" | node -p "JSON.parse(require('fs').readFileSync(0, 'utf8')).target_directory")
version=$(printf '%s' "$metadata" | node -p "JSON.parse(require('fs').readFileSync(0, 'utf8')).packages.find((p) => p.name === 'curly-message-parser').version")

# The size rows weigh stripped binaries, and a profile written in Cargo.toml
# would ship in the package.
export CARGO_PROFILE_RELEASE_STRIP=true

rm -rf bench/dist
mkdir bench/dist

# Without `intl` first: a build with it would link `hello` against a parser
# compiled with the feature.
cargo build --release --locked -p curly-message-bench --bin hello
cp "$target/release/hello" bench/dist/hello

cargo build --release --locked -p curly-message-bench --features intl --bin hello-intl --bin curly-message-bench
cp "$target/release/hello-intl" "$target/release/curly-message-bench" bench/dist/

# The tests verify the package; this weighs it. A local tree may be dirty.
cargo package --locked --no-verify --allow-dirty -p curly-message-parser
cp "$target/package/curly-message-parser-$version.crate" bench/dist/curly-message-parser.crate
