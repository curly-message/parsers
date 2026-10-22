#!/bin/sh
# The check of `php/`, run by the pre-commit hook from inside `php/`: the
# formatting and source checks of the Lint job of `tests-php.yml`
# (`composer lint`). PHPStan and the suite are left to the workflow.

if ! command -v php > /dev/null 2>&1; then
  echo 'php: php is not on the PATH: install PHP 8.2 or later (https://www.php.net/downloads), then run composer install in php/.' >&2
  exit 75
fi

if ! command -v composer > /dev/null 2>&1; then
  echo 'php: composer is not on the PATH: install Composer (https://getcomposer.org/download/), then run composer install in php/.' >&2
  exit 75
fi

if [ ! -d vendor ] || [ ! -d tools/qa/vendor ]; then
  echo 'php: php/vendor or php/tools/qa/vendor is missing: run composer install in php/.' >&2
  exit 75
fi

exec composer lint
