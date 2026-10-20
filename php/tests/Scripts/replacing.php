<?php

declare(strict_types=1);

// Resolves, in a fiber, a message reading a list nested 20 000 deep from a
// `stdClass` payload, then a value that puts a list at the same entry, then
// the entry again: an equal copy built apart (`copy`), one whose innermost
// entry differs (`changed`), or none, leaving the list in place (`kept`).
// Each list holds at its innermost one object whose conversion is counted.
// Writes as JSON what the resolution answered and how often that object was
// converted.

use CurlyMessage\Parser;

require dirname(__DIR__, 2) . '/vendor/autoload.php';

$counted = new class implements JsonSerializable {
    public int $calls = 0;

    public function jsonSerialize(): int
    {
        $this->calls += 1;

        return 1;
    }
};

/** @return list<mixed> */
$nested = static function (mixed $innermost): array {
    $list = [$innermost];

    for ($at = 1; $at < 20_000; $at += 1) {
        $list = [$list];
    }

    return $list;
};

$shape = $argv[1] ?? 'kept';
$payload = new stdClass();
$payload->v = $nested($counted);
$payload->w = new class (static function () use ($payload, $shape, $counted, $nested): void {
    if ($shape !== 'kept') {
        $payload->v = $nested($shape === 'copy' ? $counted : 2);
    }
}) implements Stringable {
    /** @param Closure(): void $put */
    public function __construct(private readonly Closure $put) {}

    public function __toString(): string
    {
        ($this->put)();

        return '|';
    }
};
$fiber = new Fiber(static fn(): string => (new Parser())->resolve('{{v}}{{w}}{{v}}', $payload));
$fiber->start();

echo json_encode(['output' => $fiber->getReturn(), 'calls' => $counted->calls]);
