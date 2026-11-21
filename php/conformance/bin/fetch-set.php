<?php

declare(strict_types=1);

// Installs the set conformance/set.json pins, verified against its digest:
// what `composer fetch-set` runs, and a no-op where it is installed already.

use CurlyMessage\Conformance\Fetch;

require dirname(__DIR__, 2) . '/vendor/autoload.php';

$home = dirname(__DIR__);
$pin = Fetch::pin("{$home}/set.json");
$into = "{$home}/.set";

try {
    if (Fetch::installed($into, $pin['version'], $pin['sha256'])) {
        fwrite(STDOUT, "The conformance set {$pin['version']} is installed.\n");

        exit(0);
    }

    $root = Fetch::install(Fetch::download($pin['url']), $pin['sha256'], $pin['version'], $into);
    fwrite(STDOUT, "The conformance set {$pin['version']} is installed at {$root}.\n");
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage() . "\n");

    exit(1);
}
