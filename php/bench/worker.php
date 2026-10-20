<?php

declare(strict_types=1);

// What `bench/rows.mjs` measures, run in PHP. Started as
//
//   php -d opcache.enable_cli=1 bench/worker.php <row> <times>
//
// it builds the row's input once, writes a line naming the configuration it
// runs under, and then, for each line it reads, runs the row's operation
// `<times>` times and answers with a line, until its input ends. Started as
//
//   php bench/worker.php --count <row>
//
// it writes the row's count and exits, and started with `--configuration`,
// what PHP it runs on.

use CurlyMessage\Cst;
use CurlyMessage\FormatRequest;
use CurlyMessage\Internal\Intl\Cache;
use CurlyMessage\ModifierInput;
use CurlyMessage\Parser;

require dirname(__DIR__) . '/vendor/autoload.php';

// Messages as a catalogue holds them: interpolation, a selection, a plural,
// a placeholder an option value writes, and a formatted number and date.
const MESSAGES = [
    'Hello, {{name}}!',
    '{{name}} has {{count:number}} new {{count:plural; one:message; other:messages}}.',
    '{{gender; male:He; female:She; default:They}} replied to {{thread}}.',
    '{{count:plural; one:{{count}} file; few:{{count}} files; other:{{count}} files}} in {{folder; :the root; default:{{folder}}}}.',
    'Total: {{amount:number}}, due {{due:date}}.',
];

/** @return array<string, mixed> */
function payload(): array
{
    return ['name' => 'Ada', 'count' => 3, 'gender' => 'female', 'thread' => 'Plans', 'folder' => 'docs', 'amount' => 1234.5, 'due' => '2026-01-31'];
}

function nested(int $depth): string
{
    return str_repeat('{{v; a:', $depth) . 'x' . str_repeat('}}', $depth);
}

function flat(int $count): string
{
    return implode(' ', array_map(static fn(int $index): string => "{{p{$index}}}", range(0, $count - 1)));
}

/** @return Closure(): mixed the operation a time row times */
function operation(string $row): Closure
{
    $parser = new Parser();
    $payload = payload();

    return match ($row) {
        'catalogue' => static function () use ($parser, $payload): void {
            foreach (MESSAGES as $message) {
                $parser->resolve($message, $payload, locale: 'en');
            }
        },
        'placeholders' => (static function () use ($parser): Closure {
            $message = flat(5000);

            return static fn(): string => $parser->resolve($message, [], locale: 'en');
        })(),
        'nested' => (static function () use ($parser): Closure {
            $message = nested(10000);

            return static fn(): string => $parser->resolve($message, ['v' => 'a'], locale: 'en');
        })(),
        'chain' => (static function () use ($parser): Closure {
            $message = str_repeat('{{v; default:', 10000) . 'x' . str_repeat('}}', 10000);

            return static fn(): string => $parser->resolve($message, [], locale: 'en');
        })(),
        'blanks' => (static function () use ($parser): Closure {
            $message = '{{' . str_repeat(' ', 10000) . '}}';

            return static fn(): string => $parser->resolve($message, [], locale: 'en');
        })(),
        'key-blanks' => (static function () use ($parser): Closure {
            $message = '{{a' . str_repeat(' ', 10000) . 'b}}';

            return static fn(): string => $parser->resolve($message, [], locale: 'en');
        })(),
        'unclosed' => (static function () use ($parser): Closure {
            $message = str_repeat('{{a; c:', 1000);

            return static fn(): string => $parser->resolve($message, [], locale: 'en');
        })(),
        'options' => (static function () use ($parser): Closure {
            $message = '{{a' . str_repeat('; a', 5000) . '}}';

            return static fn(): string => $parser->resolve($message, ['a' => 'a'], locale: 'en');
        })(),
        'option-key' => (static function () use ($parser): Closure {
            $message = '{{a; ' . str_repeat('x', 50000) . ':v}}';

            return static fn(): string => $parser->resolve($message, ['a' => 'a'], locale: 'en');
        })(),
        'wrapper' => (static function (): Closure {
            $parser = new Parser(customModifiers: ['test' => static fn(ModifierInput $in): string => $in->value]);
            $props = [];

            for ($index = 0; $index < 5000; $index += 1) {
                $props["p{$index}"] = $index;
            }

            $payload = ['v' => ['value' => 'a', 'props' => ['test' => $props]]];

            return static fn(): string => $parser->resolve('{{v:test}}', $payload, ['test' => ['q' => 1]], 'en');
        })(),
        // Each placeholder names a key beginning with NUL, which a
        // `stdClass` is searched for among all its entries.
        'nul-keys' => (static function () use ($parser): Closure {
            $payload = new stdClass();

            for ($index = 0; $index < 5000; $index += 1) {
                $payload->{(string) $index} = 'v';
            }

            $message = str_repeat("{{\0}}", 5000);

            return static fn(): string => $parser->resolve($message, $payload, locale: 'en');
        })(),
        // Each placeholder compares text outside ASCII, which goes through
        // the case table rather than through `strtolower`.
        'comparisons' => (static function () use ($parser): Closure {
            $message = str_repeat("{{v:eq; \u{00c9}cole:a; \u{0130}stanbul:b; default:c}}", 1000);

            return static fn(): string => $parser->resolve($message, ['v' => "\u{00c9}COLE"], locale: 'en');
        })(),
        // A list of records, each a map holding a list, written as JSON.
        'serialized' => (static function () use ($parser): Closure {
            $records = [];

            for ($index = 0; $index < 1000; $index += 1) {
                $records[] = ['id' => $index, 'name' => "item {$index}", 'tags' => ['a', 'b'], 'meta' => (object) ['on' => true]];
            }

            return static fn(): string => $parser->resolve('{{v; x:y; default:D}}', ['v' => $records], locale: 'en');
        })(),
        // One list read at each of 1 000 placeholders, a list past what a
        // conversion visits, so no conversion describes it and each read
        // compares it with what the first recorded.
        'reread' => (static function () use ($parser): Closure {
            $message = str_repeat('{{v; default:D}}', 1000);
            $payload = ['v' => range(0, Parser::MAX_CONVERSION_NODES)];

            return static fn(): string => $parser->resolve($message, $payload, locale: 'en');
        })(),
        // One list with a hole near its end, which telling whether it is a
        // list walks up to, past what a conversion visits, read at each of
        // 1 000 keys: what refusing it costs at each place.
        'holed' => (static function () use ($parser): Closure {
            $list = range(0, 4 * Parser::MAX_CONVERSION_NODES);
            unset($list[4 * Parser::MAX_CONVERSION_NODES - 1]);
            $payload = [];

            for ($index = 0; $index < 1000; $index += 1) {
                $payload["k{$index}"] = $list;
            }

            $message = implode('', array_map(static fn(int $index): string => "{{k{$index}; default:D}}", range(0, 999)));

            return static fn(): string => $parser->resolve($message, $payload, locale: 'en');
        })(),
        // One `stdClass` past what a conversion visits, in a list read at
        // each of 100 keys, so each key converts the list afresh: what
        // refusing the object costs at each place.
        'wide' => (static function () use ($parser): Closure {
            $object = new stdClass();

            for ($index = 0; $index <= Parser::MAX_CONVERSION_NODES; $index += 1) {
                $object->{"p{$index}"} = $index;
            }

            $payload = [];

            for ($index = 0; $index < 100; $index += 1) {
                $payload["k{$index}"] = [$object];
            }

            $message = implode('', array_map(static fn(int $index): string => "{{k{$index}; default:D}}", range(0, 99)));

            return static fn(): string => $parser->resolve($message, $payload, locale: 'en');
        })(),
        // A host modifier that reads an option and the fallback chain at
        // each placeholder, resolved in a fiber: what a read of a text host
        // code asks for costs.
        'hosted-reads' => (static function (): Closure {
            $parser = new Parser(customModifiers: ['pick' => static fn(ModifierInput $in): string => $in->options[0]->value() . $in->default()]);
            $message = str_repeat('{{a:pick; o:x{{b}}; default:d}}', 5000);

            return static function () use ($parser, $message): string {
                $fiber = new Fiber(static fn(): string => $parser->resolve($message, ['a' => 1, 'b' => 2], locale: 'en'));
                $fiber->start();
                $output = $fiber->getReturn();

                return is_string($output) ? $output : '';
            };
        })(),
        // Resolved in a fiber while 250 resolutions are suspended in fibers
        // of their own, in a modifier of a placeholder declaring options,
        // each holding its place in the process: what the counts of the
        // threads of execution and of the process cost a resolution beside
        // them.
        'parked' => (static function () use ($parser): Closure {
            $parker = new Parser(customModifiers: ['park' => static function (): string {
                Fiber::suspend();

                return 'r';
            }]);
            $parked = [];

            for ($index = 0; $index < 250; $index += 1) {
                $parked[] = $fiber = new Fiber(static fn(): string => $parker->resolve('{{v:park; o:p; q:r}}', ['v' => 'a']));
                $fiber->start();
            }

            $message = str_repeat('{{v; a:b}}', 5000);

            // What it answers names the resolutions suspended, which are kept
            // as long as the row runs.
            return static function () use ($parser, $message, $parked): string {
                $fiber = new Fiber(static fn(): string => $parser->resolve($message, ['v' => 'a'], locale: 'en'));
                $fiber->start();
                $output = $fiber->getReturn();

                return count($parked) . (is_string($output) ? $output : '');
            };
        })(),
        'cst-catalogue' => static function (): void {
            foreach (MESSAGES as $message) {
                Cst::parse($message);
            }
        },
        'cst-placeholders' => (static function (): Closure {
            $message = flat(5000);

            return static fn(): Cst\Node => Cst::parse($message);
        })(),
        'cst-nested' => (static function (): Closure {
            $message = nested(250);

            return static fn(): Cst\Node => Cst::parse($message);
        })(),
        'intl-warm' => static function () use ($parser, $payload): void {
            foreach (MESSAGES as $message) {
                $parser->resolve($message, $payload, locale: 'cs');
            }
        },
        // What a request pays under FPM or mod_php, where what the formatters
        // keep lasts one request.
        'intl-cold' => static function () use ($parser, $payload): void {
            Cache::clear();

            foreach (MESSAGES as $message) {
                $parser->resolve($message, $payload, locale: 'cs');
            }
        },
        'number-cold' => static function () use ($parser): string {
            Cache::clear();

            return $parser->resolve('{{n:number}}', ['n' => 1234.5], locale: 'cs');
        },
        'date-cold' => static function () use ($parser, $payload): string {
            Cache::clear();

            return $parser->resolve('{{due:date}}', $payload, locale: 'cs');
        },
        'ago-cold' => static function () use ($parser): string {
            Cache::clear();

            return $parser->resolve('{{n:ago}}', ['n' => -5400000], locale: 'cs');
        },
        default => throw new InvalidArgumentException("No row is named `{$row}`."),
    };
}

// Values that count what a resolution asks of them: an object's own text,
// and what one nested in plain data serializes as.
final class Tally
{
    public static int $calls = 0;

    public function __construct(private readonly string $text) {}

    public function __toString(): string
    {
        self::$calls += 1;

        return $this->text;
    }
}

final class Serialized implements JsonSerializable
{
    public function __construct(private readonly string $text) {}

    public function jsonSerialize(): string
    {
        Tally::$calls += 1;

        return $this->text;
    }
}

// The host code a resolution of the catalogue runs: each value's `__toString`
// or `jsonSerialize`.
function host_calls(): int
{
    $payload = [...payload(), 'name' => new Tally('Ada'), 'thread' => ['Plans', new Serialized('Ideas')], 'folder' => new Tally('docs')];
    $parser = new Parser();

    foreach (MESSAGES as $message) {
        $parser->resolve($message, $payload, locale: 'en');
    }

    return Tally::$calls;
}

// The host code that converting what a host modifier answers runs: one list
// it holds, answered at each of 100 placeholders.
function answer_calls(): int
{
    $answer = [new Serialized('s')];
    $parser = new Parser(customModifiers: ['held' => static fn(): array => $answer]);
    $parser->resolve(str_repeat('{{v:held}}', 100), ['v' => 1], locale: 'en');

    return Tally::$calls;
}

// The modifiers a resolution calls: a host modifier on every placeholder of
// three messages, and the facility requests of the catalogue. A branch not
// taken calls none.
function modifier_calls(): int
{
    $calls = 0;
    $parser = new Parser(
        customModifiers: ['tally' => static function (ModifierInput $in) use (&$calls): string {
            $calls += 1;

            return $in->value;
        }],
        onRequest: static function (FormatRequest $request) use (&$calls): void {
            $calls += 1;
        },
    );
    $tallied = [
        'Hello, {{name:tally}}!',
        '{{gender; male:{{name:tally}}; female:{{thread:tally}}; default:{{folder:tally}}}} replied to {{thread:tally}}.',
        '{{count:plural; one:{{count:tally}} file; other:{{count:tally}} files}} in {{folder; :the root; default:{{folder:tally}}}}.',
    ];

    foreach ([...MESSAGES, ...$tallied] as $message) {
        $parser->resolve($message, payload(), locale: 'en');
    }

    return $calls;
}

$arguments = array_slice($argv, 1);

if (($arguments[0] ?? null) === '--configuration') {
    $opcache = function_exists('opcache_get_status') ? opcache_get_status(false) : false;
    $jit = is_array($opcache) && is_array($opcache['jit'] ?? null) ? $opcache['jit'] : [];
    fwrite(STDOUT, json_encode([
        'php' => PHP_VERSION,
        'opcache' => is_array($opcache) && ($opcache['opcache_enabled'] ?? false) === true,
        'jit' => ($jit['on'] ?? false) === true,
        'icu' => defined('INTL_ICU_VERSION') ? INTL_ICU_VERSION : null,
    ], JSON_THROW_ON_ERROR) . "\n");

    exit(0);
}

if (($arguments[0] ?? null) === '--count') {
    $count = match ($arguments[1] ?? '') {
        'host-calls' => host_calls(),
        'modifier-calls' => modifier_calls(),
        'answer-calls' => answer_calls(),
        default => throw new InvalidArgumentException('No count row is named `' . ($arguments[1] ?? '') . '`.'),
    };
    fwrite(STDOUT, "{$count}\n");

    exit(0);
}

$run = operation($arguments[0] ?? '');
$times = max(1, (int) ($arguments[1] ?? 1));

// One run before the worker says it is ready, so a row that fails does so
// before the first sample.
$run();
fwrite(STDOUT, "ready\n");

while (fgets(STDIN) !== false) {
    for ($index = 0; $index < $times; $index += 1) {
        $run();
    }

    fwrite(STDOUT, "done\n");
}
