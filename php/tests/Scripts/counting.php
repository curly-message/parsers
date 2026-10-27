<?php

declare(strict_types=1);

// Resolves, with no payload, a fallback chain nested as many levels deep as
// the argument names, each level declaring an option, and writes as JSON what
// it answered, the bytes of the message, and the bytes the package counted
// characters in meanwhile.

namespace CurlyMessage\Internal {
    // Counts the bytes each count the package asks reads, which PHP's own
    // function then makes; a namespaced function is looked up before the
    // global one.
    function substr_count(string $haystack, string $needle, int $offset = 0, ?int $length = null): int
    {
        \Counted::$bytes += $length ?? strlen($haystack) - $offset;

        return \substr_count($haystack, $needle, $offset, $length);
    }
}

namespace {
    use CurlyMessage\Parser;

    require dirname(__DIR__) . '/autoload.php';

    final class Counted
    {
        public static int $bytes = 0;
    }

    $levels = (int) ($argv[1] ?? 0);
    $message = str_repeat('{{x; default:', $levels) . 'D' . str_repeat('}}', $levels);
    $output = (new Parser())->resolve($message);

    echo json_encode(['output' => $output, 'message' => strlen($message), 'counted' => Counted::$bytes]);
}
