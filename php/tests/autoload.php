<?php

declare(strict_types=1);

// The suite's classes, and the package it tests: the source, or, where
// CURLY_MESSAGE_PACKAGE names a project that installed the archive a release
// ships (`composer test:dist`), the package as that project installed it.
// Composer prepends the autoloader registered last, so the project's copy of
// the package is the one every test loads; and the suite's own autoloader is
// told to serve none of the package, so a class the archive lacks is one no
// test can load, rather than one the source quietly stands in for.

$suite = require dirname(__DIR__) . '/vendor/autoload.php';

if (!$suite instanceof Composer\Autoload\ClassLoader) {
    throw new RuntimeException('The suite is installed with no Composer autoloader.');
}

(static function (Composer\Autoload\ClassLoader $suite): void {
    $project = getenv('CURLY_MESSAGE_PACKAGE');

    if ($project === false || $project === '') {
        return;
    }

    $suite->setPsr4('CurlyMessage\\', []);

    $root = realpath($project);

    if ($root === false || !is_file("{$root}/vendor/autoload.php")) {
        throw new RuntimeException("CURLY_MESSAGE_PACKAGE names {$project}, which holds no installed project.");
    }

    require "{$root}/vendor/autoload.php";

    $file = (new ReflectionClass(CurlyMessage\Parser::class))->getFileName();

    if ($file === false || !str_starts_with($file, $root . DIRECTORY_SEPARATOR)) {
        throw new RuntimeException('The package loads from ' . var_export($file, true) . ", not from the project at {$root}.");
    }
})($suite);
