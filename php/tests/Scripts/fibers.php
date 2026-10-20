<?php

declare(strict_types=1);

// Resolves a message whose value resolves it again, each time in a fiber of
// its own, and writes what the outermost resolution answered as JSON. The
// first argument names how many levels of placeholder the message nests the
// value in, none where it names none; a second, `stack`, resolves it again
// on the same call stack instead.

use CurlyMessage\Parser;
use CurlyMessage\Report;

require dirname(__DIR__) . '/autoload.php';

$reports = [];
$parser = new Parser(onReport: static function (Report $report) use (&$reports): void {
    $reports[] = $report->code->value;
});

$nesting = (int) ($argv[1] ?? 0);
$message = str_repeat('{{a; 1:<', $nesting) . '{{x; default:F}}' . str_repeat('>}}', $nesting);

$level = new class ($parser, $message, ($argv[2] ?? '') === 'stack') {
    public int $levels = 0;

    public function __construct(private readonly Parser $parser, private readonly string $message, private readonly bool $stack) {}

    public function __toString(): string
    {
        $this->levels += 1;

        if ($this->stack) {
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
