<?php

declare(strict_types=1);

// Writes src/Internal/Unicode/Table.php, the table Unicode's toLowercase
// is read from (Default Case Conversion, with Final_Sigma), from the Unicode
// Character Database files pinned below. `--check` writes nothing and fails
// where the table on disk is not the one these files make. The files are read
// from the directory named on the command line, or fetched from unicode.org.

const VERSION = '17.0.0';

const FILES = [
    'UnicodeData.txt' => '2e1efc1dcb59c575eedf5ccae60f95229f706ee6d031835247d843c11d96470c',
    'SpecialCasing.txt' => 'efc25faf19de21b92c1194c111c932e03d2a5eaf18194e33f1156e96de4c9588',
    'DerivedCoreProperties.txt' => '24c7fed1195c482faaefd5c1e7eb821c5ee1fb6de07ecdbaa64b56a99da22c08',
];

const TABLE = __DIR__ . '/../src/Internal/Unicode/Table.php';

// Its lower case depends on what surrounds it, so it is decided where it is
// read and has no entry of its own.
const CAPITAL_SIGMA = 0x03A3;

/** @param list<string> $arguments */
function main(array $arguments): int
{
    $check = in_array('--check', $arguments, true);
    $directories = array_values(array_filter($arguments, static fn(string $argument): bool => $argument !== '--check'));
    $sources = [];

    foreach (FILES as $name => $digest) {
        $text = read($directories[0] ?? null, $name);

        if ($text === null || hash('sha256', $text) !== $digest) {
            fwrite(STDERR, "{$name} is not the pinned UCD " . VERSION . " file.\n");

            return 1;
        }

        $sources[$name] = $text;
    }

    $table = table($sources['UnicodeData.txt'], $sources['SpecialCasing.txt'], $sources['DerivedCoreProperties.txt']);

    if ($check) {
        if (@file_get_contents(TABLE) !== $table) {
            fwrite(STDERR, 'src/Internal/Unicode/Table.php is not the table UCD ' . VERSION . " makes; run composer unicode.\n");

            return 1;
        }

        return 0;
    }

    file_put_contents(TABLE, $table);

    return 0;
}

function read(?string $directory, string $name): ?string
{
    $path = $directory === null ? 'https://www.unicode.org/Public/' . VERSION . "/ucd/{$name}" : "{$directory}/{$name}";
    $text = @file_get_contents($path);

    return is_string($text) ? $text : null;
}

/**
 * Each line's fields, comments and blank lines dropped.
 *
 * @return list<list<string>>
 */
function records(string $text): array
{
    $records = [];

    foreach (explode("\n", $text) as $line) {
        $data = trim(explode('#', $line, 2)[0]);

        if ($data !== '') {
            $records[] = array_map('trim', explode(';', $data));
        }
    }

    return $records;
}

/** @return list<int> */
function codePoints(string $field): array
{
    $fields = preg_split('/\s+/', trim($field), -1, PREG_SPLIT_NO_EMPTY);

    return array_map('hexdec_int', $fields === false ? [] : $fields);
}

function hexdec_int(string $hex): int
{
    return (int) hexdec($hex);
}

/**
 * A PHP string literal: single-quoted where every character is printable
 * ASCII, as the formatting contract writes one, and escaped otherwise.
 *
 * @param list<int> $codePoints
 */
function literal(array $codePoints): string
{
    $plain = '';
    $escaped = '';

    foreach ($codePoints as $codePoint) {
        $printable = $codePoint >= 0x20 && $codePoint < 0x7F;

        if (!$printable || in_array(chr($codePoint), ['\'', '\\'], true)) {
            $plain = null;
        } elseif ($plain !== null) {
            $plain .= chr($codePoint);
        }

        $escaped .= $printable && !in_array(chr($codePoint), ['"', '\\', '$'], true) ? chr($codePoint) : sprintf('\u{%X}', $codePoint);
    }

    return $plain === null ? "\"{$escaped}\"" : "'{$plain}'";
}

/**
 * A property's ranges, adjacent ones merged.
 *
 * @return list<array{int, int}>
 */
function ranges(string $derived, string $property): array
{
    $ranges = [];

    foreach (records($derived) as $record) {
        if (($record[1] ?? null) !== $property) {
            continue;
        }

        $bounds = explode('..', $record[0]);
        $start = hexdec_int($bounds[0]);
        $ranges[] = [$start, isset($bounds[1]) ? hexdec_int($bounds[1]) : $start];
    }

    usort($ranges, static fn(array $a, array $b): int => $a[0] <=> $b[0]);
    $merged = [];

    foreach ($ranges as $range) {
        $last = count($merged) - 1;

        if ($last >= 0 && $merged[$last][1] + 1 >= $range[0]) {
            $merged[$last] = [$merged[$last][0], max($merged[$last][1], $range[1])];
        } else {
            $merged[] = $range;
        }
    }

    return $merged;
}

function table(string $unicodeData, string $specialCasing, string $derived): string
{
    /** @var array<int, list<int>> $lower */
    $lower = [];

    foreach (records($unicodeData) as $record) {
        if (($record[13] ?? '') !== '') {
            $lower[hexdec_int($record[0])] = codePoints($record[13]);
        }
    }

    // The full mappings that hold unconditionally replace the simple ones;
    // a mapping with a condition is a language's or a context's.
    foreach (records($specialCasing) as $record) {
        if (($record[4] ?? '') === '') {
            $codePoint = hexdec_int($record[0]);
            $lower[$codePoint] = codePoints($record[1]);
        }
    }

    ksort($lower);
    $lines = [];

    foreach ($lower as $codePoint => $mapped) {
        if ($codePoint < 0x80 || $codePoint === CAPITAL_SIGMA || $mapped === [$codePoint]) {
            continue;
        }

        $lines[] = '        ' . literal([$codePoint]) . ' => ' . literal($mapped) . ',';
    }

    $sets = '';

    foreach (['CASED' => 'Cased', 'IGNORABLE' => 'Case_Ignorable'] as $name => $property) {
        $sets .= "\n    public const {$name} = [\n";

        foreach (ranges($derived, $property) as [$start, $end]) {
            $sets .= sprintf("        [0x%04X, 0x%04X],\n", $start, $end);
        }

        $sets .= "    ];\n";
    }

    return "<?php\n\ndeclare(strict_types=1);\n\n"
        . "namespace CurlyMessage\\Internal\\Unicode;\n\n"
        . "/**\n"
        . ' * GENERATED by tools/unicode.php from UCD ' . VERSION . "; do not edit.\n"
        . " *\n"
        . " * LOWER: each character outside ASCII whose lower case differs, but\n"
        . " * U+03A3, which depends on its context. CASED, IGNORABLE: the code point\n"
        . " * ranges of Cased and Case_Ignorable, which that context is read by.\n"
        . " *\n"
        . " * @internal\n"
        . " */\n"
        . "final class Table\n{\n    public const LOWER = [\n" . implode("\n", $lines) . "\n    ];\n" . $sets . "}\n";
}

exit(main(isset($argv) ? array_slice($argv, 1) : []));
