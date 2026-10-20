<?php

declare(strict_types=1);

// Run as `php -n tools/smoke.php <project>`, over a project that installed the
// archive a release ships. `-n` reads no php.ini, so no shared extension is
// loaded, and the project's autoloader holds the package alone: a call the
// package makes to a function an optional extension or a dev dependency
// defines fails here, on every path the set's Core and Extensions cases and
// its tree cases reach.
// The runner's classes come from this checkout; the set is read unvalidated,
// since the validator is a dev dependency and the suite validated it.

use CurlyMessage\Conformance\Adapter;
use CurlyMessage\Conformance\Options;
use CurlyMessage\Conformance\Runner;
use CurlyMessage\Conformance\Set;
use CurlyMessage\Parser;
use CurlyMessage\Tests\Conformance\Pinned;

$project = $argv[1] ?? null;

if (!is_string($project) || realpath($project) === false || !is_file("{$project}/vendor/autoload.php")) {
    fwrite(STDERR, "Usage: php -n tools/smoke.php <project that installed the archive>\n");

    exit(2);
}

$project = realpath($project);

require "{$project}/vendor/autoload.php";

spl_autoload_register(static function (string $class): void {
    $home = dirname(__DIR__);

    foreach (['CurlyMessage\\Conformance\\' => "{$home}/conformance/src/", 'CurlyMessage\\Tests\\' => "{$home}/tests/"] as $prefix => $directory) {
        if (str_starts_with($class, $prefix)) {
            $file = $directory . str_replace('\\', '/', substr($class, strlen($prefix))) . '.php';

            if (is_file($file)) {
                require $file;
            }

            return;
        }
    }
});

$file = (new ReflectionClass(Parser::class))->getFileName();

if ($file === false || !str_starts_with($file, $project . DIRECTORY_SEPARATOR)) {
    fwrite(STDERR, 'The package loads from ' . var_export($file, true) . ", not from the project at {$project}.\n");

    exit(1);
}

try {
    $adapter = require dirname(__DIR__) . '/tests/Conformance/adapter.php';

    if (!$adapter instanceof Adapter) {
        throw new LogicException('adapter.php returns no adapter.');
    }

    $result = Runner::run($adapter, new Options(fixtures: Pinned::without(Set::pinned(false)), levels: ['core', 'extensions']));
} catch (Throwable $error) {
    fwrite(STDERR, $error::class . ': ' . $error->getMessage() . "\n");

    exit(1);
}

$ok = $result->failed === [] && $result->passed !== [];
$summary = explode("\n", Runner::summarize($result));

// The cases the two levels leave out are the Intl and tree cases, by the
// hundred; the counts say how many.
fwrite($ok ? STDOUT : STDERR, 'The archive under php -n (' . implode(', ', get_loaded_extensions()) . '): ' . ($ok ? end($summary) : implode("\n", $summary)) . "\n");

exit($ok ? 0 : 1);
