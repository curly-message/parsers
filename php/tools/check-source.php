<?php

declare(strict_types=1);

// The source policy the formatter cannot state: every byte is ASCII, the
// library writes nowhere itself, and Core calls nothing an optional extension
// or a dev dependency's polyfill defines. Tokens are read, not lines matched,
// so a name in a string or a comment is never mistaken for a call.

/**
 * Whether a name names one of a list, spelled whole or as a prefix.
 *
 * @param list<string> $whole
 * @param list<string> $prefixes
 */
function matches(string $name, array $whole, array $prefixes): bool
{
    $name = strtolower(ltrim($name, '\\'));

    if (in_array($name, $whole, true)) {
        return true;
    }

    foreach ($prefixes as $prefix) {
        if (str_starts_with($name, $prefix) && !str_contains($name, '\\')) {
            return true;
        }
    }

    return false;
}

/**
 * Every PHP file under a directory of the package, by its path from the root.
 *
 * @return list<string>
 */
function files(string $root, string $directory): array
{
    $path = $root . '/' . $directory;

    if (!is_dir($path)) {
        return [];
    }

    $found = [];

    foreach (new RecursiveIteratorIterator(new RecursiveDirectoryIterator($path, FilesystemIterator::SKIP_DOTS)) as $file) {
        if (!$file instanceof SplFileInfo || !$file->isFile()) {
            continue;
        }

        $relative = str_replace('\\', '/', substr($file->getPathname(), strlen($root) + 1));

        if (str_starts_with($relative, 'tools/qa/')) {
            continue;
        }

        $found[] = $relative;
    }

    sort($found);

    return $found;
}

$root = dirname(__DIR__);

$ascii = ['src', 'tests', 'tools', 'conformance/src', 'conformance/bin', 'bench'];

$output = ['echo', 'print', 'exit', 'die', 'eval', 'var_dump', 'print_r', 'printf', 'vprintf', 'error_log', 'trigger_error', 'user_error', 'debug_zval_dump', 'debug_print_backtrace'];

$optional = ['mb_', 'iconv', 'ctype_', 'grapheme_', 'idn_'];

$intlClasses = ['numberformatter', 'intldateformatter', 'intldatepatterngenerator', 'locale', 'resourcebundle', 'messageformatter', 'collator', 'normalizer', 'transliterator', 'spoofchecker', 'uconverter'];

$intlPrefixes = ['intl', 'numfmt_', 'datefmt_', 'locale_', 'resourcebundle_', 'msgfmt_', 'collator_', 'normalizer_', 'transliterator_'];

$intlHome = 'src/Internal/Intl/';

/** @var list<string> $problems */
$problems = [];

foreach ($ascii as $directory) {
    foreach (files($root, $directory) as $relative) {
        $content = (string) file_get_contents($root . '/' . $relative);
        $at = strcspn($content, implode('', array_map('chr', range(0x80, 0xFF))));

        if ($at < strlen($content)) {
            $line = substr_count($content, "\n", 0, $at) + 1;
            $problems[] = sprintf('%s:%d: a byte outside ASCII; write the character as an escape', $relative, $line);
        }
    }
}

$skipped = [T_WHITESPACE, T_COMMENT, T_DOC_COMMENT];

foreach (files($root, 'src') as $relative) {
    if (!str_ends_with($relative, '.php')) {
        continue;
    }

    $tokens = token_get_all((string) file_get_contents($root . '/' . $relative));
    $count = count($tokens);

    for ($index = 0; $index < $count; $index += 1) {
        $token = $tokens[$index];

        if (!is_array($token)) {
            continue;
        }

        [$id, $text, $line] = $token;

        if (in_array($id, [T_ECHO, T_PRINT, T_EXIT, T_EVAL, T_INLINE_HTML, T_OPEN_TAG_WITH_ECHO], true)) {
            $problems[] = sprintf('%s:%d: `%s` writes output; a library writes nowhere itself', $relative, $line, trim($text));

            continue;
        }

        if (!in_array($id, [T_STRING, T_NAME_FULLY_QUALIFIED], true)) {
            continue;
        }

        $before = $index - 1;

        while ($before >= 0 && is_array($tokens[$before]) && in_array($tokens[$before][0], $skipped, true)) {
            $before -= 1;
        }

        $after = $index + 1;

        while ($after < $count && is_array($tokens[$after]) && in_array($tokens[$after][0], $skipped, true)) {
            $after += 1;
        }

        $previous = $before >= 0 ? $tokens[$before] : null;
        $following = $after < $count ? $tokens[$after] : null;

        // A member, a declaration or a named argument spells a name of its
        // own, not the global one.
        if (is_array($previous) && in_array($previous[0], [T_OBJECT_OPERATOR, T_NULLSAFE_OBJECT_OPERATOR, T_DOUBLE_COLON, T_FUNCTION, T_CONST, T_CASE], true)) {
            continue;
        }

        if ($following === ':') {
            continue;
        }

        if ($following === '(' && matches($text, $output, [])) {
            $problems[] = sprintf('%s:%d: `%s` writes output; a library writes nowhere itself', $relative, $line, $text);
        }

        if ($following === '(' && matches($text, [], $optional)) {
            $problems[] = sprintf('%s:%d: `%s` is an optional extension\'s, which Core never calls', $relative, $line, $text);
        }

        if (!str_starts_with($relative, $intlHome) && matches($text, $intlClasses, $intlPrefixes)) {
            $problems[] = sprintf('%s:%d: `%s` is ext-intl\'s, reached only from %s', $relative, $line, $text, $intlHome);
        }
    }
}

foreach ($problems as $problem) {
    fwrite(STDERR, $problem . "\n");
}

exit($problems === [] ? 0 : 1);
