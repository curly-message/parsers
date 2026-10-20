<?php

declare(strict_types=1);

// Measures what the README's Memory table states: for each shape it names,
// the least size whose resolution a memory limit ends, found by halving
// between a size that resolves and one the limit ends, each run apart under
// `php -n` at that limit. `php tools/memory.php` measures every shape at
// 128M, and `php tools/memory.php <limit> [shape]` at another limit, or one
// shape. A run at one size is `php tools/memory.php <limit> <shape> <size>
// run`, which exits 0 where the resolution returned.

use CurlyMessage\ModifierInput;
use CurlyMessage\Option;
use CurlyMessage\Parser;

require dirname(__DIR__) . '/vendor/autoload.php';

const MIB = 1_048_576;

/**
 * Each shape: what it is, the unit of its size, the size halving begins
 * from, and whether it needs ext-intl.
 *
 * @var array<string, array{string, string, int, bool}>
 */
const SHAPES = [
    'options' => ['one placeholder declaring many options', 'options', 1_000, false],
    'json-shared' => ['a value whose JSON is one string of 1 MiB written many times', 'MiB of JSON', 8, false],
    'json-key' => ['a value whose one key is control characters, each written as six', 'KiB of key', 1_024, false],
    'answer-around' => ['an answer a level below the message places with text around it', 'MiB of answer', 8, false],
    'value-around' => ['a value a level below the message places with text around it', 'MiB of value', 8, false],
    'answer-alone' => ['an answer a level below the message places alone', 'MiB of answer', 8, false],
    'include-modifier' => ['a message that includes itself, each level in a fiber, through a host modifier it hands its options to, as many levels as the host begins', 'levels', 1_000, false],
    'include-value' => ['a message that includes itself, each level in a fiber, through a value that resolves it again, as many levels as the host begins', 'levels', 1_000, false],
    'kept' => ['options a host modifier kept, each read from inside the one before, while the resolution is in progress', 'options', 1_000, false],
    'kept-fibers' => ['options a host modifier kept, each read in a fiber of its own from inside the one before, while the resolution is in progress', 'options', 1_000, false],
    'fibers' => ['fibers the host keeps suspended, outside any resolution', 'fibers', 1_000, false],
    'props-list' => ['a props list an Intl modifier reads as text, one string of 1 MiB written many times', 'MiB of list', 8, true],
];

/** Resolves the shape at the size, in this process. */
function run(string $shape, int $size): string
{
    $holder = new class {
        public ?Parser $parser = null;

        public int $levels = 0;
    };
    /** @var list<Option> $kept the options a modifier kept, the next to read first */
    $kept = [];
    $step = static function () use (&$kept): string {
        $next = array_shift($kept);

        if ($next === null) {
            return 'end';
        }

        $fiber = new Fiber(static fn(): string => $next->value());
        $fiber->start();
        $text = $fiber->getReturn();

        return is_string($text) ? $text : '';
    };
    $stepHere = static function () use (&$kept): string {
        $next = array_shift($kept);

        return $next === null ? 'end' : $next->value();
    };
    $modifiers = [
        'read' => static fn(ModifierInput $in): string => (string) strlen($in->options[0]->value()),
        'big' => static fn(ModifierInput $in): string => str_repeat('a', $size * MIB),
        'fan' => static function (ModifierInput $in) use (&$kept, $step): string {
            $kept = $in->options;

            return $step();
        },
        'step' => $step,
        'keep' => static function (ModifierInput $in) use (&$kept, $stepHere): string {
            $kept = $in->options;

            return $stepHere();
        },
        'next' => $stepHere,
        'include' => static function (ModifierInput $in) use ($holder, $size): string {
            if ($holder->levels === $size) {
                return 'E';
            }

            $holder->levels += 1;
            $fiber = new Fiber(static fn(): string => $holder->parser?->resolve(message('include-modifier', $size), ['a' => 1]) ?? '');
            $fiber->start();
            $text = $fiber->getReturn();

            return '[' . (is_string($text) ? $text : '') . ']';
        },
    ];
    $parser = $holder->parser = new Parser(customModifiers: $modifiers);
    $level = new class ($parser, message($shape, $size), $size) implements Stringable {
        private int $levels = 0;

        public function __construct(private readonly Parser $parser, private readonly string $message, private readonly int $size) {}

        public function __toString(): string
        {
            if ($this->levels === $this->size) {
                return 'E';
            }

            $this->levels += 1;
            $fiber = new Fiber(fn(): string => $this->parser->resolve($this->message, ['a' => 1, 'x' => $this]));
            $fiber->start();
            $text = $fiber->getReturn();

            return is_string($text) ? $text : '';
        }
    };
    $shared = str_repeat('a', MIB);

    return match ($shape) {
        'options', 'answer-around', 'answer-alone', 'include-modifier', 'kept-fibers' => $parser->resolve(message($shape, $size), ['a' => 1]),
        // Each read nests inside the one before on one stack, so one the
        // stack refused would answer otherwise than the last option does.
        'kept' => $parser->resolve(message($shape, $size), ['a' => 1]) === 'end' ? 'end' : throw new RuntimeException('A read was refused.'),
        'json-shared' => $parser->resolve('{{v}}', ['v' => array_fill(0, $size, $shared)]),
        'json-key' => $parser->resolve('{{v}}', ['v' => [str_repeat("\x01", $size * 1_024) => 1]]),
        'value-around' => $parser->resolve(message($shape, $size), ['a' => 1, 'v' => str_repeat('a', $size * MIB)]),
        'include-value' => $parser->resolve(message($shape, $size), ['a' => 1, 'x' => $level]),
        'fibers' => (static function () use ($size): string {
            $fibers = [];

            for ($at = 0; $at < $size; $at += 1) {
                $fiber = new Fiber(static function (): void {
                    Fiber::suspend();
                });
                $fiber->start();
                $fibers[] = $fiber;
            }

            return (string) count($fibers);
        })(),
        'props-list' => $parser->resolve('{{n:number}}', ['n' => ['value' => 5, 'props' => ['number' => ['style' => array_fill(0, $size, $shared)]]]], locale: 'en'),
        default => throw new ValueError("No shape is named {$shape}."),
    };
}

/** The message the shape resolves at the size. */
function message(string $shape, int $size): string
{
    return match ($shape) {
        'options' => '{{a' . str_repeat('; k:v', $size) . '}}',
        'answer-around' => '{{a:read; o:a{{a:big}}b}}',
        'answer-alone' => '{{a:read; o:{{a:big}}}}',
        'value-around' => '{{a:read; o:a{{v}}b}}',
        'include-modifier' => '{{a:include; k:v}}',
        'include-value' => '{{x; k:v; default:F}}',
        'kept' => '{{a:keep' . str_repeat('; k:{{a:next}}', $size) . '}}',
        'kept-fibers' => '{{a:fan' . str_repeat('; k:{{a:step}}', $size) . '}}',
        default => '',
    };
}

/** Whether the shape resolves at the size under the limit, run apart; null where it cannot run here. */
function resolves(string $limit, string $shape, int $size): ?bool
{
    $intl = SHAPES[$shape][3] ? ['-d', 'extension=intl'] : [];
    // What the run writes to stderr goes to a file: from 8.5 on, a fatal
    // error writes a backtrace, more than a pipe holds before its writer
    // waits for a reader.
    $log = tempnam(sys_get_temp_dir(), 'memory');

    if ($log === false) {
        throw new RuntimeException('Could not make a file for a run to write to.');
    }

    $process = proc_open([PHP_BINARY, '-n', '-d', 'display_errors=stderr', '-d', "memory_limit={$limit}", ...$intl, __FILE__, $limit, $shape, (string) $size, 'run'], [1 => ['pipe', 'w'], 2 => ['file', $log, 'w']], $pipes);

    if (!is_resource($process)) {
        unlink($log);

        throw new RuntimeException('Could not start a run.');
    }

    stream_get_contents($pipes[1]);
    $status = proc_close($process);
    $errors = (string) file_get_contents($log);
    unlink($log);

    if ($status === 0) {
        return true;
    }

    if (str_contains($errors, 'Allowed memory size')) {
        return false;
    }

    if ($status === 3) {
        return null;
    }

    throw new RuntimeException("{$shape} at {$size} ended otherwise than at the limit: {$errors}");
}

/** The least size the limit ends, within a fiftieth of it; null where the shape cannot run here. */
function least(string $limit, string $shape): ?int
{
    $low = 0;
    $high = SHAPES[$shape][2];

    while (true) {
        $resolved = resolves($limit, $shape, $high);

        if ($resolved === null) {
            return null;
        }

        if (!$resolved) {
            break;
        }

        $low = $high;
        $high *= 2;
    }

    while ($high - $low > 1 && ($high - $low) * 50 > $high) {
        $middle = intdiv($low + $high, 2);

        if (resolves($limit, $shape, $middle) === true) {
            $low = $middle;
        } else {
            $high = $middle;
        }
    }

    return $high;
}

$limit = $argv[1] ?? '128M';

if (($argv[4] ?? '') === 'run') {
    if (SHAPES[$argv[2]][3] ?? false) {
        if (!extension_loaded('intl')) {
            exit(3);
        }
    }

    run($argv[2], (int) $argv[3]);

    exit(0);
}

$named = $argv[2] ?? null;

foreach (SHAPES as $shape => [$what, $unit]) {
    if ($named !== null && $named !== $shape) {
        continue;
    }

    $size = least($limit, $shape);
    fwrite(STDOUT, $size === null ? "{$shape}: {$what}: needs ext-intl\n" : "{$shape}: {$what}: ends at {$limit} from about {$size} {$unit}\n");
}
