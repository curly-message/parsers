<?php

declare(strict_types=1);

// Leaves resolutions suspended in fibers and resumes them elsewhere, then
// writes what was answered as JSON. Where the argument names `stacked`, a
// hundred and forty are left suspended from the main code, and a message
// whose value resumes the next of them is resolved: each resumed one
// resolves the message again, so each runs beneath every one resumed before
// it. Where it names `shallow`, one is begun beneath all but two of
// the resolutions `Parser::MAX_REENTRY` allows, and resumed from the main
// code once they ended, where it resolves three nested in each other.

use CurlyMessage\Parser;
use CurlyMessage\Report;

require dirname(__DIR__, 2) . '/vendor/autoload.php';

$reports = [];
$parser = new Parser(onReport: static function (Report $report) use (&$reports): void {
    $reports[] = $report->code->value;
});

if (($argv[1] ?? '') === 'shallow') {
    $nested = new class ($parser) implements Stringable {
        public int $left = 3;

        public function __construct(private readonly Parser $parser) {}

        public function __toString(): string
        {
            $this->left -= 1;

            return $this->left > 0 ? $this->parser->resolve('<{{n}}>', ['n' => $this]) : 'deepest';
        }
    };
    $suspending = new class ($parser, $nested) implements Stringable {
        public function __construct(private readonly Parser $parser, private readonly object $nested) {}

        public function __toString(): string
        {
            Fiber::suspend();

            return $this->parser->resolve('[{{n}}]', ['n' => $this->nested]);
        }
    };
    $deep = new class ($parser, $suspending) implements Stringable {
        public int $left = Parser::MAX_REENTRY - 2;

        /** @var Fiber<mixed, mixed, mixed, mixed>|null */
        public ?Fiber $fiber = null;

        public function __construct(private readonly Parser $parser, private readonly object $suspending) {}

        public function __toString(): string
        {
            $this->left -= 1;

            if ($this->left > 0) {
                return $this->parser->resolve('{{d}}', ['d' => $this]);
            }

            $this->fiber = new Fiber(fn(): string => $this->parser->resolve('{{s}}', ['s' => $this->suspending]));
            $this->fiber->start();

            return 'begun';
        }
    };

    $begun = $parser->resolve('{{d}}', ['d' => $deep]);
    $fiber = $deep->fiber;

    if ($fiber === null) {
        throw new LogicException('No fiber was begun.');
    }

    $fiber->resume();

    echo json_encode(['begun' => $begun, 'resumed' => $fiber->getReturn(), 'reports' => $reports]);

    return;
}

$level = new class ($parser) implements Stringable {
    public int $levels = 0;

    /** @var list<Fiber<mixed, mixed, mixed, mixed>> */
    public array $suspended = [];

    public function __construct(private readonly Parser $parser) {}

    public function __toString(): string
    {
        $this->levels += 1;
        $fiber = array_shift($this->suspended);

        if ($fiber === null) {
            return 'end';
        }

        $fiber->resume();
        $text = $fiber->isTerminated() ? $fiber->getReturn() : null;

        return is_string($text) ? $text : '';
    }

    public function resolved(): string
    {
        return $this->parser->resolve('<{{x; default:F}}>', ['x' => $this]);
    }
};
$suspending = new class (static fn(): string => $level->resolved()) implements Stringable {
    /** @param Closure(): string $resumed */
    public function __construct(private readonly Closure $resumed) {}

    public function __toString(): string
    {
        Fiber::suspend();

        return ($this->resumed)();
    }
};

for ($at = 0; $at < 140; $at += 1) {
    $fiber = new Fiber(static fn(): string => $parser->resolve('{{s}}', ['s' => $suspending]));
    $fiber->start();
    $level->suspended[] = $fiber;
}

$output = $level->resolved();

echo json_encode(['output' => $output, 'levels' => $level->levels, 'reports' => array_count_values($reports)]);
