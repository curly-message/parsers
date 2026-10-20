<?php

declare(strict_types=1);

// Resolves a message whose modifier keeps what an outer placeholder handed
// it, and reads it again from a placeholder nested inside, in the shape the
// argument names: an option's value (`option`) or the fallback chain
// (`default`) read while it is still being rendered; each of the options in
// turn, one from inside the one before (`distinct`, as many as the second
// argument names), the same with each answer marked and a payload default
// to fall back to (`chain`); a text kept from one placeholder and read
// from inside another nested three deep, itself nested six deep (`nested`);
// a modifier that reads its own option in a fiber its option suspends,
// eight times, beside an ordinary message nested eight deep (`suspended`),
// or as many times as the second argument names, before a text kept and
// read where another placeholder nests the read (`parked`); texts kept at
// each depth from two to eight, read one inside another, each one level
// deeper than the one before but for every seventh (`cycling`); a text a
// fiber left suspended while rendering it, read twice from inside itself
// (`twice`); or an option a fiber begins, read again before the fiber
// resumes, then once more after (`resumed`); each of the options in turn,
// read by the report handler from inside the one before (`reported`), by
// the value each option places, a value of its own (`converted`) or one
// value they all place (`reconverted`), as many as the second argument
// names; options kept from as many resolutions, each read from inside the
// one before once they all returned (`across`); or a text read twice by each rendering of it, each time in a
// fiber the reading starts and leaves suspended, kept by the host
// (`fanned`) or let go (`dropped`); an option or the fallback chain, as the
// second argument names, read in two fibers that its rendering suspends,
// then by the parser after a modifier that raises or answers nothing, or by
// the modifier from its own call, as the third names (`forked`); or texts
// nested seven deep, each read again from inside its own rendering, around
// as many placeholders as the second argument names (`stacked`); or, at each
// of seven levels, an option a modifier reads in a fiber it lets go while
// the option suspends, then reads again itself, around a placeholder that
// resolves the whole message again, as many levels deep as the second
// argument names, before it suspends (`abandoned`); the same with the
// fallback chain, which the modifier answers nothing after, so the parser
// reads it (`forsaken`), and with each fiber kept and resumed by the
// placeholder in the middle before it suspends, as an event loop resumes
// what it waits on (`scheduled`); or a fallback chain a fiber left
// suspended, read by the parser after its modifier raises, then by the
// fiber a later placeholder resumes, then by the one after it
// (`stranded`).
// Writes what the resolution answered as JSON.

use CurlyMessage\ModifierInput;
use CurlyMessage\Option;
use CurlyMessage\Parser;
use CurlyMessage\Report;

require dirname(__DIR__, 2) . '/vendor/autoload.php';

$reports = [];
$kept = null;
$calls = 0;
$fibers = [];
$towers = [];
/** @var Closure|null $hook */
$hook = null;
$renders = 0;
/** @var Parser|null $parser */
$parser = null;
$count = (int) ($argv[2] ?? 50_000);
$entered = 0;
$abandoned = str_repeat('{{a:abandon; o:[', 7) . '{{a:enter}}' . str_repeat(']}}', 7);
$forsaken = str_repeat('{{a:forsake; default:[', 7) . '{{a:enter}}' . str_repeat(']}}', 7);
$scheduled = str_repeat('{{a:schedule; default:[', 7) . '{{a:enter}}' . str_repeat(']}}', 7);
$reentered = match ($argv[1] ?? '') {
    'forsaken' => $forsaken,
    'scheduled' => $scheduled,
    default => $abandoned,
};

$modifiers = [
    'option' => static function (ModifierInput $in) use (&$kept, &$calls): string {
        $calls += 1;
        $kept = $in->options === [] ? $kept : $in->options[0];

        return $kept instanceof Option ? $kept->value() : 'none';
    },
    'default' => static function (ModifierInput $in) use (&$kept, &$calls): string {
        $calls += 1;
        $kept ??= $in;

        return $kept instanceof ModifierInput ? $kept->default() : 'none';
    },
    'distinct' => static function (ModifierInput $in) use (&$kept, &$calls): string {
        $calls += 1;
        $kept ??= $in->options;

        return is_array($kept) && isset($kept[$calls - 1]) ? $kept[$calls - 1]->value() : 'end';
    },
    'chain' => static function (ModifierInput $in) use (&$kept, &$calls): string {
        $calls += 1;
        $kept ??= $in->options;

        return (is_array($kept) && isset($kept[$calls - 1]) ? $kept[$calls - 1]->value() : 'end') . 'x';
    },
    'keep' => static function (ModifierInput $in) use (&$kept): string {
        $kept = $in->options;

        return 'K';
    },
    'read' => static function () use (&$kept, &$calls): string {
        $calls += 1;

        return is_array($kept) && isset($kept[0]) ? $kept[0]->value() : 'none';
    },
    'pick' => static fn(ModifierInput $in): string => $in->options[0]->value(),
    'aside' => static function (ModifierInput $in) use (&$fibers): string {
        $fiber = new Fiber(static fn(): string => $in->options[0]->value());
        $fiber->start();
        $fibers[] = $fiber;

        return 'h';
    },
    'suspend' => static function (): string {
        Fiber::suspend();

        return 's';
    },
    'tower' => static function (ModifierInput $in) use (&$towers): string {
        $towers[(int) $in->value] = array_slice($in->options, 0, -1);

        return 'T' . $in->options[count($in->options) - 1]->value();
    },
    'walk' => static function () use (&$towers, &$calls): string {
        $at = $calls;
        $calls += 1;
        $text = $towers[$at % 7 + 1][intdiv($at, 7)] ?? null;

        return $text instanceof Option ? $text->value() : 'end';
    },
    'hold' => static function (ModifierInput $in) use (&$kept, &$fibers): string {
        $kept = $in->options[0];
        $fiber = new Fiber(static fn(): string => $in->options[0]->value());
        $fiber->start();
        $fibers[] = $fiber;

        return 'k';
    },
    'pause' => static function (): string {
        if (Fiber::getCurrent() !== null) {
            Fiber::suspend();
        }

        return 'p';
    },
    'twice' => static function () use (&$kept, &$calls): string {
        $calls += 1;

        return $calls > 1_000 || !$kept instanceof Option ? 'stop' : (string) strlen($kept->value() . $kept->value());
    },
    'race' => static function (ModifierInput $in): string {
        $option = $in->options[0];
        $fiber = new Fiber(static fn(): string => $option->value());
        $fiber->start();
        $main = $option->value();
        $fiber->resume();
        $resumed = $fiber->getReturn();

        return $main . ',' . (is_string($resumed) ? $resumed : '') . ',' . $option->value();
    },
    'first' => static function (ModifierInput $in) use (&$kept): string {
        $kept = $in->options;

        return $in->options[0]->value();
    },
    'store' => static function (ModifierInput $in) use (&$towers): string {
        $towers[] = $in->options[0];

        return 's';
    },
    'next' => static function () use (&$towers, &$calls): string {
        $calls += 1;
        $text = $towers[$calls - 1] ?? null;

        return $text instanceof Option ? $text->value() : 'end';
    },
    'spawn' => static function () use (&$kept, &$fibers, &$renders, $argv): string {
        $renders += 1;

        foreach ([1, 2] as $_) {
            $option = $kept;
            $fiber = new Fiber(static fn(): string => $option instanceof Option ? $option->value() : '');
            $fiber->start();

            if (($argv[1] ?? '') === 'fanned') {
                $fibers[] = $fiber;
            }
        }

        return 'f';
    },
    'fork' => static function (ModifierInput $in) use (&$fibers, &$calls, $argv): ?string {
        $calls += 1;

        foreach ([1, 2] as $_) {
            $fiber = new Fiber(static fn(): string => ($argv[2] ?? '') === 'option' ? $in->options[0]->value() : $in->default());
            $fiber->start();
            $fibers[] = $fiber;
        }

        return match ($argv[3] ?? '') {
            'raise' => throw new RuntimeException('Refused.'),
            'nothing' => null,
            default => $in->options[0]->value(),
        };
    },
    'stack' => static function (ModifierInput $in) use (&$towers): string {
        $towers[] = $in->options[0];

        try {
            return $in->options[0]->value();
        } finally {
            array_pop($towers);
        }
    },
    'again' => static function () use (&$towers): string {
        $text = end($towers);

        return $text instanceof Option ? $text->value() : '';
    },
    'tick' => static function () use (&$renders): string {
        $renders += 1;

        return '';
    },
    'count' => static function () use (&$calls): string {
        $calls += 1;
        $call = $calls;

        if (Fiber::getCurrent() !== null) {
            Fiber::suspend();
        }

        return 'r' . $call;
    },
    'abandon' => static function (ModifierInput $in) use (&$calls): string {
        $calls += 1;
        $option = $in->options[0];
        $fiber = new Fiber(static fn(): string => $option->value());
        $fiber->start();
        unset($fiber);

        return $option->value();
    },
    'forsake' => static function (ModifierInput $in) use (&$calls): null {
        $calls += 1;
        $fiber = new Fiber(static fn(): string => $in->default());
        $fiber->start();
        unset($fiber);

        return null;
    },
    'schedule' => static function (ModifierInput $in) use (&$calls, &$fibers): null {
        $calls += 1;
        $fiber = new Fiber(static fn(): string => $in->default());
        $fibers[] = $fiber;
        $fiber->start();

        return null;
    },
    'strand' => static function (ModifierInput $in) use (&$kept, &$fibers): never {
        $kept = $in;
        $fiber = new Fiber(static fn(): string => $in->default());
        $fiber->start();
        $fibers[] = $fiber;

        throw new RuntimeException('Stranded.');
    },
    'wake' => static function () use (&$fibers): string {
        $fiber = $fibers[0];
        $fiber->resume();
        $read = $fiber->getReturn();

        return is_string($read) ? $read : '';
    },
    'reread' => static function () use (&$kept): string {
        return $kept instanceof ModifierInput ? $kept->default() : '';
    },
    'enter' => static function () use (&$calls, &$parser, &$entered, &$fibers, $count, $reentered, $argv): string {
        $calls += 1;

        if ($entered < $count && $parser instanceof Parser) {
            $entered += 1;

            try {
                return $parser->resolve($reentered, ['a' => 1]);
            } finally {
                $entered -= 1;
            }
        }

        if (($argv[1] ?? '') === 'scheduled') {
            $current = Fiber::getCurrent();

            foreach ($fibers as $at => $fiber) {
                if ($fiber !== $current && $fiber->isSuspended()) {
                    unset($fibers[$at]);
                    $fiber->resume();
                }
            }

            unset($current, $fiber);
        }

        // A frame that held its own fiber as it suspends would keep the
        // fiber from being destroyed where the modifier lets go of it.
        if (Fiber::getCurrent() !== null) {
            Fiber::suspend();
        }

        return 'x';
    },
];

$parser = new Parser(customModifiers: $modifiers, onReport: static function (Report $report) use (&$reports, &$hook): void {
    $reports[] = $report->code->value;

    if ($hook instanceof Closure) {
        $hook();
    }
});
$readNext = static function () use (&$kept, &$calls): string {
    $calls += 1;

    return is_array($kept) && isset($kept[$calls]) ? $kept[$calls]->value() : '';
};
$placing = static fn(): Stringable => new class ($readNext) implements Stringable {
    /** @param Closure(): string $read */
    public function __construct(private readonly Closure $read) {}

    public function __toString(): string
    {
        return ($this->read)();
    }
};
$placed = static function (bool $each) use ($parser, $placing, $count): string {
    $payload = ['a' => 1];
    $value = $placing();

    for ($at = 0; $at < $count; $at += 1) {
        $payload['v' . $at] = $each ? $placing() : $value;
    }

    return $parser->resolve('{{a:first' . implode('', array_map(static fn(int $at): string => '; o:{{v' . $at . '}}', range(0, $count - 1))) . '}}', $payload);
};

$nested = static fn(int $levels, string $inner): string => str_repeat('{{a:pick; o:<', $levels) . $inner . str_repeat('>}}', $levels);
$tower = '-';

for ($level = 7; $level >= 1; $level -= 1) {
    $tower = '{{d' . $level . ':tower' . str_repeat('; o:{{a:walk}}', 10) . '; n:' . $tower . '}}';
}

$output = match ($argv[1] ?? '') {
    'option' => $parser->resolve('{{a:option; k:[{{b:option}}]; default:D}}', ['a' => 1, 'b' => 2]),
    'default' => $parser->resolve('{{a:default; default:[{{b:default}}]}}', ['a' => 1, 'b' => 2]),
    'distinct' => $parser->resolve('{{a:distinct' . str_repeat('; o:{{a:distinct}}', $count) . '}}', ['a' => 1]),
    'chain' => $parser->resolve('{{a:chain' . str_repeat('; o:{{a:chain}}', $count) . '}}', ['a' => 1, 'default' => 'F']),
    'nested' => $parser->resolve('{{a:keep; o:' . $nested(6, 'X') . '}}|' . $nested(3, '{{a:read}}'), ['a' => 1]),
    'suspended' => $parser->resolve(str_repeat('{{a:aside; o:{{a:suspend}}}}', 8) . '|' . str_repeat('{{a; 1:<', 7) . '{{a; 1:X}}' . str_repeat('>}}', 7), ['a' => 1]),
    'parked' => $parser->resolve(str_repeat('{{a:aside; o:{{a:suspend}}}}', $count) . '|{{a:keep; o:X}}{{a:pick; o:{{a:read}}}}', ['a' => 1]),
    'cycling' => $parser->resolve($tower . '|{{a:walk}}', ['a' => 1, 'd1' => 1, 'd2' => 2, 'd3' => 3, 'd4' => 4, 'd5' => 5, 'd6' => 6, 'd7' => 7]),
    'twice' => $parser->resolve('{{a:hold; o:[{{a:pause}}{{a:twice}}]}}{{a:twice}}', ['a' => 1]),
    'resumed' => $parser->resolve('{{a:race; o:{{a:count}}}}', ['a' => 1]),
    'reported' => (static function () use ($parser, &$hook, $readNext, $count): string {
        $hook = $readNext;

        return $parser->resolve('{{a:first' . str_repeat('; o:{{z:nosuch}}', $count) . '}}', ['a' => 1]);
    })(),
    'converted', 'reconverted' => $placed($argv[1] === 'converted'),
    'across' => (static function () use ($parser, $count): string {
        for ($at = 0; $at < $count; $at += 1) {
            $parser->resolve('{{a:store; o:{{a:next}}}}', ['a' => 1]);
        }

        return $parser->resolve('{{a:next}}', ['a' => 1]);
    })(),
    'fanned', 'dropped' => $parser->resolve('{{a:hold; o:[{{a:spawn}}{{a:pause}}]}}{{a:spawn}}', ['a' => 1]),
    'forked' => $parser->resolve('{{a:fork; o:[{{a:pause}}]; default:[{{a:pause}}]}}', ['a' => 1]),
    'abandoned', 'forsaken', 'scheduled' => $parser->resolve($reentered, ['a' => 1]),
    'stranded' => $parser->resolve('{{a:strand; default:<{{a:count}}>}}|{{a:wake}}|{{a:reread}}', ['a' => 1]),
    'stacked' => $parser->resolve(str_repeat('{{a:stack; o:<{{a:again}}', Parser::MAX_NESTING - 1) . str_repeat('{{a:tick}}', $count) . str_repeat('>}}', Parser::MAX_NESTING - 1), ['a' => '']),
    default => throw new RuntimeException('Name what is read again: option, default, distinct, chain, nested, suspended, parked, cycling, twice, resumed, reported, converted, reconverted, across, fanned, dropped, forked, stacked, abandoned, forsaken, scheduled or stranded.'),
};

echo json_encode(['output' => $output, 'calls' => $calls + $renders, 'reports' => $reports]);
