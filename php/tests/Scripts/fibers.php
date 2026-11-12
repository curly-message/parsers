<?php

declare(strict_types=1);

// Resolves a message whose value resolves it again, and writes what the
// outermost resolution answered as JSON. The first argument names how many
// levels of placeholder the message nests the value in, none where it names
// none; the second names where each level resolves the next: on the same
// call stack (`stack`), in a fiber of its own, for as many levels as the
// third names before the value answers `E` (`fiber`), or on the same call
// stack and in a fiber of its own by turns (`paired`).

use CurlyMessage\Parser;
use CurlyMessage\Report;

require dirname(__DIR__) . '/autoload.php';

$reports = [];
$parser = new Parser(onReport: static function (Report $report) use (&$reports): void {
    $reports[] = $report->code->value;
});

$nesting = (int) ($argv[1] ?? 0);
$message = str_repeat('{{a; 1:<', $nesting) . '{{x; default:F}}' . str_repeat('>}}', $nesting);

$level = new class ($parser, $message, $argv[2] ?? '', (int) ($argv[3] ?? 0)) {
    public int $levels = 0;

    public function __construct(private readonly Parser $parser, private readonly string $message, private readonly string $where, private readonly int $last) {}

    public function __toString(): string
    {
        if ($this->where === 'fiber' && $this->levels === $this->last) {
            return 'E';
        }

        $this->levels += 1;

        if ($this->where === 'stack' || ($this->where === 'paired' && $this->levels % 2 === 1)) {
            return $this->parser->resolve($this->message, ['a' => 1, 'x' => $this]);
        }

        $fiber = new Fiber(fn(): string => $this->parser->resolve($this->message, ['a' => 1, 'x' => $this]));
        $fiber->start();
        $text = $fiber->getReturn();

        return is_string($text) ? $text : '';
    }
};

$output = $parser->resolve($message, ['a' => 1, 'x' => $level]);

echo json_encode(['output' => $output, 'levels' => $level->levels, 'reports' => $reports]);
