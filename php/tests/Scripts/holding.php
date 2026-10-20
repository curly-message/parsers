<?php

declare(strict_types=1);

// Resolves a message whose placeholders each make and let go of a long text,
// in the shape the argument names, or whose option places a text one byte
// short of the longest the parser writes, then one byte more (`longest`) or
// two (`past`), and writes what the resolution answered as JSON: the output
// where it is short, its length where it is not, and the code and limit of
// each report. The caller sets the memory limit it is resolved under.

use CurlyMessage\ModifierInput;
use CurlyMessage\Parser;
use CurlyMessage\Report;

require dirname(__DIR__, 2) . '/vendor/autoload.php';

$reports = [];
$parser = new Parser(
    customModifiers: [
        'long' => static fn(): string => str_repeat('a', 1_000_000),
        'length' => static fn(ModifierInput $in): int => strlen($in->options[0]->value()),
        'text' => static fn(): Stringable => new class implements Stringable {
            public function __toString(): string
            {
                return str_repeat('a', 1_000_000);
            }
        },
        'list' => static fn(): array => [str_repeat('a', 1_000_000)],
        'longest' => static fn(): string => str_repeat('a', Parser::MAX_TEXT_LENGTH - 1),
        'one' => static fn(): string => 'b',
        'two' => static fn(): string => 'bb',
    ],
    onReport: static function (Report $report) use (&$reports): void {
        $reports[] = [$report->code->value, $report->limit];
    },
);

$output = match ($argv[1] ?? '') {
    'separate' => $parser->resolve(str_repeat('{{x:length; a:{{x:long}};}} ', 40), ['x' => 'a']),
    'texts' => $parser->resolve(str_repeat('{{x:text; default:D;}}', 30), ['x' => 'a']),
    'lists' => $parser->resolve(str_repeat('{{x:list; default:D;}}', 30), ['x' => 'a']),
    'longest' => $parser->resolve('{{x:length; a:{{x:longest}}{{x:one}}}}', ['x' => 'a']),
    'past' => $parser->resolve('{{x:length; a:{{x:longest}}{{x:two}}}}', ['x' => 'a']),
    default => throw new RuntimeException('Name the shape: separate, texts, lists, longest or past.'),
};

echo json_encode(['output' => strlen($output) > 64 ? strlen($output) : $output, 'reports' => $reports]);
