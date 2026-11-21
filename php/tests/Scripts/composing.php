<?php

declare(strict_types=1);

// Resolves a message of as many placeholders as the first argument names,
// half reading wrapped values and half a plain one, each handing `eq` the
// props three layers hold under its name, and writes as JSON what it
// answered and how many entries the package enumerated meanwhile. The two
// layers every placeholder shares hold as many entries as the second
// argument names. The wrapped values lie under as many keys as the third
// names, each wrapper holding as many entries of its own as the fourth
// names. Every layer is a `stdClass`, whose enumeration is counted, or an
// array where a fifth is 1, which costs nothing to enumerate and is run
// for what the process holds. The arguments are written as one, with an
// `x` between them.

namespace CurlyMessage\Internal {
    // Counts the entries each enumeration the package asks for copies, which
    // PHP's own function then makes; a namespaced function is looked up
    // before the global one.
    /** @return array<array-key, mixed> */
    function get_object_vars(object $object): array
    {
        $entries = \get_object_vars($object);
        \Composed::$entries += count($entries);

        return $entries;
    }
}

namespace {
    use CurlyMessage\Parser;

    require dirname(__DIR__) . '/autoload.php';

    final class Composed
    {
        public static int $entries = 0;
    }

    [$placeholders, $width, $keys, $own, $arrays] = array_map(intval(...), explode('x', $argv[1] ?? '') + [0, 0, 1, 0, 0]);
    $slice = static function (int $width) use ($arrays): array|stdClass {
        $slice = [];

        for ($at = 0; $at < $width; $at += 1) {
            $slice["p{$at}"] = $at;
        }

        return $arrays === 1 ? $slice : (object) $slice;
    };
    $payload = ['v' => 1];
    $message = '';

    for ($at = 0; $at < $keys; $at += 1) {
        $payload["w{$at}"] = ['value' => 1, 'props' => ['eq' => $slice($own)]];
    }

    for ($at = 0; $at < intdiv($placeholders, 2); $at += 1) {
        $message .= '{{w' . $at % $keys . ':eq; 1:a}}{{v:eq; 1:b}}';
    }

    $parser = new Parser(modifierDefaults: ['eq' => $slice($width)]);
    $output = $parser->resolve($message, $payload, ['eq' => $slice($width)]);

    echo json_encode(['output' => $output, 'enumerated' => Composed::$entries]);
}
