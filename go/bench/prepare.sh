#!/bin/sh
# Builds, from go/, what bench/rows.mjs reads: the program whose stripped
# binary the size rows weigh, and the program that runs every other row.
set -e

go -C dev build -trimpath -ldflags='-s -w' -o ../dist/size ./size
go -C dev build -trimpath -o ../dist/bench ./bench
