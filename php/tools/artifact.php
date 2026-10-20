<?php

declare(strict_types=1);

// What `composer test:dist` runs: the archive a release ships, tested as a
// consumer installs it. PHP has no build, so the artifact is the archive
// Packagist serves, which is `git archive` of this directory under its
// `.gitattributes`. It is built from the working tree rather than from HEAD,
// since the suite runs before a change is committed; installed into a scratch
// project as Composer installs a release, by copy; and then the unit and
// conformance suites, the surface probe and the `php -n` smoke run all run
// against that copy.

$home = dirname(__DIR__);

// What the archive may hold, and what it must: a file outside the first list
// is one export-ignore missed, and one missing from the second is a release
// that cannot be installed or read.
const SHIPPED = ['composer.json', 'README.md', 'CHANGELOG.md', 'LICENSE'];
const REQUIRED = ['composer.json', 'README.md', 'CHANGELOG.md', 'LICENSE', 'src/Parser.php'];

/**
 * Runs a command, its output passed through, and stops the script where it
 * fails.
 *
 * @param list<string> $command
 * @param array<string, string> $environment added to this process's own
 */
function command(array $command, string $directory, array $environment = []): string
{
    $process = proc_open($command, [1 => ['pipe', 'w']], $pipes, $directory, [...getenv(), ...$environment]);

    if (!is_resource($process)) {
        throw new RuntimeException('Could not start ' . $command[0] . '.');
    }

    $output = (string) stream_get_contents($pipes[1]);
    fclose($pipes[1]);
    $status = proc_close($process);

    if ($status !== 0) {
        fwrite(STDOUT, $output);

        throw new RuntimeException(implode(' ', $command) . " exited with {$status}.");
    }

    return $output;
}

/** @return list<string> every file under a directory, by its path from there, with forward slashes */
function listing(string $directory): array
{
    $files = [];
    $entries = new RecursiveIteratorIterator(new RecursiveDirectoryIterator($directory, FilesystemIterator::SKIP_DOTS));

    foreach ($entries as $entry) {
        if ($entry instanceof SplFileInfo && !$entry->isDir()) {
            $files[] = str_replace('\\', '/', substr($entry->getPathname(), strlen($directory) + 1));
        }
    }

    sort($files);

    return $files;
}

function erase(string $path): void
{
    if (is_link($path) || is_file($path)) {
        unlink($path);

        return;
    }

    if (!is_dir($path)) {
        return;
    }

    $names = scandir($path);

    foreach ($names === false ? [] : $names as $name) {
        if ($name !== '.' && $name !== '..') {
            erase("{$path}/{$name}");
        }
    }

    rmdir($path);
}

$scratch = sys_get_temp_dir() . '/curly-message-artifact-' . bin2hex(random_bytes(6));
$composer = getenv('COMPOSER_BINARY');
// Composer names itself to the scripts it runs; run on its own, this finds it
// on the PATH.
$composer = is_string($composer) && $composer !== '' ? [PHP_BINARY, $composer] : ['composer'];

try {
    mkdir($scratch);
    $top = trim(command(['git', 'rev-parse', '--show-toplevel'], $home));
    $prefix = trim(command(['git', 'rev-parse', '--show-prefix'], $home), "/\n");

    // The working tree as the index would hold it, honouring .gitignore, in
    // an index of its own, so the repository's index is left as it is.
    $index = ['GIT_INDEX_FILE' => "{$scratch}/index"];
    command(['git', 'add', '--all', '--', $prefix], $top, $index);
    $tree = trim(command(['git', 'write-tree'], $top, $index));
    command(['git', 'archive', '--format=tar', "--output={$scratch}/archive.tar", "{$tree}:{$prefix}"], $top);

    mkdir("{$scratch}/archive");
    command(['tar', '-xf', "{$scratch}/archive.tar", '-C', "{$scratch}/archive"], $scratch);
    $listed = listing("{$scratch}/archive");
    $stray = array_values(array_filter($listed, static fn(string $file): bool => !str_starts_with($file, 'src/') && !in_array($file, SHIPPED, true)));
    // Every file of src/ ships, so a class the archive lacks is caught here
    // rather than by a consumer's autoloader.
    $sources = array_map(static fn(string $file): string => "src/{$file}", array_filter(explode("\n", command(['git', 'ls-tree', '-r', '--name-only', "{$tree}:{$prefix}/src"], $top)), static fn(string $file): bool => $file !== ''));
    $missing = array_values(array_diff([...REQUIRED, ...$sources], $listed));

    if ($stray !== [] || $missing !== []) {
        throw new RuntimeException('The archive holds ' . json_encode($stray) . ' it should not, and lacks ' . json_encode($missing) . '.');
    }

    fwrite(STDOUT, 'The archive holds ' . count($listed) . " files.\n");

    // A path repository copies the package in, as an install from the
    // registry unpacks it; the version is the one a tag would give it, since
    // the archive carries none, as the published one does not.
    mkdir("{$scratch}/project");
    $manifest = [
        'name' => 'curly-message/artifact-check',
        'type' => 'project',
        'require' => ['curly-message/parser' => '1.0.0'],
        'repositories' => [
            ['type' => 'path', 'url' => "{$scratch}/archive", 'options' => ['symlink' => false, 'versions' => ['curly-message/parser' => '1.0.0']]],
            ['packagist.org' => false],
        ],
    ];
    file_put_contents("{$scratch}/project/composer.json", json_encode($manifest, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR));
    command([...$composer, 'install', '--no-interaction', '--no-progress', '--working-dir=' . "{$scratch}/project"], $scratch);
    $installed = "{$scratch}/project/vendor/curly-message/parser";

    if (is_link($installed) || listing($installed) !== $listed) {
        throw new RuntimeException('The project did not install a copy of the archive.');
    }

    $package = ['CURLY_MESSAGE_PACKAGE' => "{$scratch}/project"];
    fwrite(STDOUT, command([PHP_BINARY, 'vendor/bin/phpunit', '--testsuite', 'unit,conformance'], $home, $package));

    // The public surface as a consumer's analysis reads it: from the project,
    // whose autoloader PHPStan loads and whose installed package it reads.
    $config = "{$scratch}/project/phpstan.neon";
    file_put_contents($config, implode("\n", [
        'parameters:',
        '    level: max',
        '    phpVersion: 80200',
        '    tmpDir: ' . json_encode("{$scratch}/phpstan"),
        '    paths:',
        '        - ' . json_encode("{$home}/tests/Types/surface.php"),
        '',
    ]));
    fwrite(STDOUT, command([PHP_BINARY, "{$home}/tools/qa/vendor/bin/phpstan", 'analyse', '--no-progress', '--memory-limit=1G', "--configuration={$config}"], "{$scratch}/project"));

    fwrite(STDOUT, command([PHP_BINARY, '-n', "{$home}/tools/smoke.php", "{$scratch}/project"], $home));
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage() . "\n");
    $failed = true;
} finally {
    erase($scratch);
}

exit(isset($failed) ? 1 : 0);
