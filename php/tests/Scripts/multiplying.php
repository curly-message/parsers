<?php

declare(strict_types=1);

// Resolves ten placeholders over a property padded with as many blanks as the
// first argument names: a `currency` at a `ratio` of `2` behind no-break
// spaces, or with `digits` as the second argument, a `number` at a
// `maximumFractionDigits` of `1` between spaces and no-break spaces. With
// `blanks`, that `1` sits between medium mathematical spaces, which lead with
// the byte fourteen other blanks do: a sixth as many a side as the first
// argument names, rounded down, of three bytes each, so as many bytes in all
// less the remainder of that division. With `alternating`, those blanks
// pad a `minimumFractionDigits` of `1` instead, and the placeholders take
// turns between two wrappers with props of their own. With `listed`, they
// pad the `maximumFractionDigits` in a list of one. With `quoted`, the
// option is that many letters in curly quotes. With `number` or `date`, one
// placeholder of that modifier reads a value of `1.5` with as many no-break
// spaces a side as the first argument names. With `shared`, `texts` or
// `singles`, as many placeholders as the first argument names each read a
// wrapper whose `minimumFractionDigits` is a list repeating one text of
// 1 MiB twenty times then the placeholder's number, a text of 1 MiB with
// that number, or a list of that one text. Writes as JSON whether ext-intl
// is loaded, what it answered, the characters the package's text rules read
// meanwhile, the bytes their replacements passed over and, for those three,
// the peak memory of the resolution above what was in use before it.

namespace CurlyMessage\Internal {
    // Counts each character the package's text rules read, which PHP's own
    // function then reads; a namespaced function is looked up before the
    // global one.
    function ord(string $character): int
    {
        \Multiplied::$characters += 1;

        return \ord($character);
    }

    /** @param string|array<string, string> $from */
    function strtr(string $string, string|array $from, ?string $to = null): string
    {
        \Multiplied::$marked += strlen($string);

        return is_string($from) ? \strtr($string, $from, (string) $to) : \strtr($string, $from);
    }

    /**
     * Counts a pass over the subject for each string searched for, which PHP's
     * own function replaces in turn; `strtr` reads it once for all of them.
     *
     * @param string|list<string> $search
     * @param string|list<string> $replace
     */
    function str_replace(string|array $search, string|array $replace, string $subject): string
    {
        \Multiplied::$marked += strlen($subject) * (is_array($search) ? count($search) : 1);

        return \str_replace($search, $replace, $subject);
    }
}

namespace {
    use CurlyMessage\Parser;

    require dirname(__DIR__) . '/autoload.php';

    final class Multiplied
    {
        public static int $characters = 0;

        public static int $marked = 0;

        public static int $base = 0;
    }

    $spaces = (int) ($argv[1] ?? 0);
    $padding = str_repeat(" \u{A0}", $spaces);
    $blanks = str_repeat("\u{205F}", intdiv($spaces, 6));
    $digits = match ($argv[2] ?? '') {
        'digits' => $padding . '1' . $padding,
        'blanks' => $blanks . '1' . $blanks,
        'quoted' => "\u{201C}" . str_repeat('a', $spaces) . "\u{201D}",
        default => null,
    };
    $wrapped = static fn(bool $grouping): array => ['value' => 1.56, 'props' => ['number' => ['useGrouping' => $grouping]]];
    $value = str_repeat("\u{A0}", $spaces) . '1.5' . str_repeat("\u{A0}", $spaces);
    $placeholders = static fn(int $count, string $text): array => [
        implode('', array_map(static fn(int $i): string => "{{p$i:number; default:D}}", range(1, $count))),
        array_combine(array_map(static fn(int $i): string => "p$i", range(1, $count)), array_map(static fn(int $i): array => ['value' => 1.56, 'props' => ['number' => ['minimumFractionDigits' => match ($argv[2] ?? '') {
            'shared' => [...array_fill(0, 20, $text), (string) $i],
            'texts' => $text . $i,
            default => [$text . $i],
        }]]], range(1, $count))),
    ];
    // The resolution alone is measured: one of a single placeholder first
    // loads what it runs, so the peak counts no compiled code, and the peak is
    // taken from what is in use once the payload is built.
    $measured = static function (array $warm, array $shared): string {
        (new Parser())->resolve($warm[0], $warm[1], [], 'en');
        memory_reset_peak_usage();
        Multiplied::$base = memory_get_usage();

        return (new Parser())->resolve($shared[0], $shared[1], [], 'en');
    };
    $output = match ($argv[2] ?? '') {
        'alternating' => (new Parser())->resolve(str_repeat('{{a:number; default:D}}{{b:number; default:D}}', 5), ['a' => $wrapped(false), 'b' => $wrapped(true)], ['number' => ['minimumFractionDigits' => $blanks . '1' . $blanks]], 'en'),
        'listed' => (new Parser())->resolve(str_repeat('{{n:number; default:D}}', 10), ['n' => 1.56], ['number' => ['maximumFractionDigits' => [$blanks . '1' . $blanks]]], 'en'),
        'number' => (new Parser())->resolve('{{v:number; default:D}}', ['v' => $value], [], 'en'),
        'date' => (new Parser())->resolve('{{v:date; default:D}}', ['v' => $value], ['date' => ['timeZone' => 'UTC', 'dateStyle' => 'short']], 'en'),
        'shared', 'texts', 'singles' => $measured($placeholders(1, 'a'), $placeholders($spaces, str_repeat('a', 1 << 20))),
        default => $digits !== null
            ? (new Parser())->resolve(str_repeat('{{n:number; default:D}}', 10), ['n' => 1.56], ['number' => ['maximumFractionDigits' => $digits]], 'en')
            : (new Parser())->resolve(str_repeat('{{n:currency; default:D}}', 10), ['n' => 25], ['currency' => ['currency' => 'EUR', 'ratio' => str_repeat("\u{A0}", $spaces) . '2']], 'en'),
    };

    echo json_encode(['intl' => extension_loaded('intl'), 'output' => $output, 'read' => Multiplied::$characters, 'marked' => Multiplied::$marked, 'memory' => memory_get_peak_usage() - Multiplied::$base]);
}
