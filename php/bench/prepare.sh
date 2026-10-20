#!/bin/sh
# What a side of a comparison needs before its rows run: the package's own
# autoloader, installed without dev dependencies and running no script.
set -e
cd "$(dirname "$0")/.."
composer install --no-dev --no-scripts --no-plugins --no-interaction --no-progress
