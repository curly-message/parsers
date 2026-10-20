<?php

declare(strict_types=1);

// Resolves a message naming a key that begins with NUL, as many times as the
// argument names, against a `stdClass` payload of as many numbered entries,
// and writes as JSON what it answered and how many entries the package
// enumerated meanwhile.

namespace CurlyMessage\Internal {
    // Counts the entries each enumeration the package asks for copies, which
    // PHP's own function then makes; a namespaced function is looked up
    // before the global one.
    /** @return array<array-key, mixed> */
    function get_object_vars(object $object): array
    {
        $entries = \get_object_vars($object);
        \Enumerated::$entries += count($entries);

        return $entries;
    }
}

namespace {
    use CurlyMessage\Parser;

    require dirname(__DIR__, 2) . '/vendor/autoload.php';

    final class Enumerated
    {
        public static int $entries = 0;
    }

    $count = (int) ($argv[1] ?? 0);
    $payload = new stdClass();

    for ($at = 0; $at < $count; $at += 1) {
        $payload->{(string) $at} = 'v';
    }

    $output = (new Parser())->resolve(str_repeat("{{\0}}", $count) . "{{\0; default:D}}", $payload);

    echo json_encode(['output' => $output, 'enumerated' => Enumerated::$entries]);
}
