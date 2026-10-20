<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\ModifierInput;
use CurlyMessage\Option;
use CurlyMessage\Parser;
use CurlyMessage\ReentryLimit;
use CurlyMessage\Report;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class ReentryTest extends TestCase
{
    use Apart;
    use Resolves;

    public function testContainsAResolutionBegunPastTheLimitOfOneStack(): void
    {
        $parser = $this->parser();
        $reentering = new Reentering($parser);

        self::assertSame('F', $parser->resolve('{{x; default:F}}', ['x' => $reentering]));
        self::assertSame(Parser::MAX_REENTRY, $reentering->calls);
        self::assertInstanceOf(ReentryLimit::class, $reentering->refused);
        self::assertSame(['unserializable-value'], $this->codes());
    }

    /** @return iterable<string, array{bool}> */
    public static function threads(): iterable
    {
        yield 'in the main code' => [false];
        yield 'in a fiber' => [true];
    }

    // One resolution begun beneath every other the bound allows, on the
    // same thread, resolves as any other does.
    #[DataProvider('threads')]
    public function testResolvesEveryResolutionBegunWithinTheLimitOfOneThread(bool $fibered): void
    {
        $parser = $this->parser();
        $nesting = new class ($parser) implements \Stringable {
            public int $left = Parser::MAX_REENTRY - 1;

            public function __construct(private readonly Parser $parser) {}

            public function __toString(): string
            {
                if ($this->left === 0) {
                    return 'end';
                }

                $this->left -= 1;

                return '<' . $this->parser->resolve('{{x}}', ['x' => $this]) . '>';
            }
        };
        $resolve = static fn(): string => $parser->resolve('{{x}}', ['x' => $nesting]);
        $depth = Parser::MAX_REENTRY - 1;

        self::assertSame(str_repeat('<', $depth) . 'end' . str_repeat('>', $depth), $fibered ? self::inFiber($resolve) : $resolve());
        self::assertSame(0, $nesting->left);
        self::assertSame([], $this->codes());
    }

    #[DataProvider('threads')]
    public function testContainsAResolutionBegunPastTheLimitOfItsThread(bool $fibered): void
    {
        $parser = $this->parser();
        $reentering = new Reentering($parser);
        $resolve = static fn(): string => $parser->resolve('{{x; default:F}}', ['x' => $reentering]);

        self::assertSame('F', $fibered ? self::inFiber($resolve) : $resolve());
        self::assertSame(Parser::MAX_REENTRY, $reentering->calls);
        self::assertInstanceOf(ReentryLimit::class, $reentering->refused);
        self::assertStringContainsString('on its thread of execution', $reentering->refused->getMessage());
        self::assertSame(['unserializable-value'], $this->codes());
    }

    // A thread that resolves one message after another, as a fiber serving
    // requests in turn does, takes every place it gives back again.
    #[DataProvider('threads')]
    public function testReleasesEveryPlaceItTook(bool $fibered): void
    {
        $parser = $this->parser();
        $rounds = static function () use ($parser): string {
            $calls = [];

            for ($round = 0; $round < 3; $round += 1) {
                $reentering = new Reentering($parser);
                $calls[] = $parser->resolve('{{x; default:F}}', ['x' => $reentering]) . $reentering->calls;
            }

            return implode(',', $calls);
        };
        $each = 'F' . Parser::MAX_REENTRY;

        self::assertSame("{$each},{$each},{$each}", $fibered ? self::inFiber($rounds) : $rounds());
    }

    public function testContainsAReportHandlerThatReenters(): void
    {
        $calls = 0;
        $parser = null;
        $parser = new Parser(onReport: static function (Report $report) use (&$parser, &$calls): void {
            $calls += 1;
            self::assertInstanceOf(Parser::class, $parser);
            $parser->resolve('{{v:nope}}');
        });

        self::assertSame('D', $parser->resolve('{{v:nope; default:D}}'));
        self::assertSame(Parser::MAX_REENTRY, $calls);
    }

    // A fiber begun deep, suspended, and resumed once what it began beneath
    // has ended runs beneath none of it.
    public function testResolvesInAFiberResumedBeneathLessThanItBeganBeneath(): void
    {
        self::assertSame(['begun' => 'begun', 'resumed' => '[<<deepest>>]', 'reports' => []], self::apart('resumed', 'shallow', '128M'));
    }

    // A fiber is a thread of execution of its own: one begun beneath every
    // place but one of the main code's counts its own, and the main code's
    // count is the same once it returns.
    public function testCountsThePlacesOfEachFiberApart(): void
    {
        $parser = $this->parser();
        $inner = new Reentering($parser);
        $outer = new class ($parser, $inner) implements \Stringable {
            public int $left = Parser::MAX_REENTRY - 1;

            public function __construct(private readonly Parser $parser, private readonly Reentering $inner) {}

            public function __toString(): string
            {
                $this->left -= 1;

                if ($this->left > 0) {
                    return $this->parser->resolve('{{o}}', ['o' => $this]);
                }

                $fiber = new \Fiber(fn(): string => $this->parser->resolve('{{x; default:F}}', ['x' => $this->inner]));
                $fiber->start();
                $text = $fiber->getReturn();

                return is_string($text) ? $text : '';
            }
        };

        self::assertSame('F', $parser->resolve('{{o}}', ['o' => $outer]));
        self::assertSame(Parser::MAX_REENTRY, $inner->calls);
        self::assertSame(['unserializable-value'], $this->codes());
    }

    // What fibers left suspended in resolutions hold takes nothing from any
    // other thread, up to the bound of the process, and each resumes where
    // it was. A thread's first resolution takes no place in the process, so
    // those begun beneath it are all a thread adds there.
    public function testResolvesBesideResolutionsSuspendedInFibers(): void
    {
        $parser = $this->parser();
        $fibers = self::park($parser, Parser::MAX_IN_PROGRESS - Parser::MAX_REENTRY + 1);
        $reentering = new Reentering($parser);
        self::assertSame('F', $parser->resolve('{{x; default:F}}', ['x' => $reentering]));
        self::assertSame(Parser::MAX_REENTRY, $reentering->calls);
        self::assertInstanceOf(ReentryLimit::class, $reentering->refused);
        self::assertStringContainsString('on its thread of execution', $reentering->refused->getMessage());
        $fiber = new \Fiber(static fn(): string => $parser->resolve('A'));
        $fiber->start();
        self::assertSame('A', $fiber->getReturn());

        foreach ($fibers as $suspended) {
            $suspended->resume();
            self::assertSame('w', $suspended->getReturn());
        }

        self::assertSame(['unserializable-value'], $this->codes());
    }

    /** @return iterable<string, array{int, string, int, string}> */
    public static function fiberings(): iterable
    {
        yield 'each level in a fiber and the next beneath it on its stack, the value placed by the message itself' => [0, 'paired', 2 * Parser::MAX_IN_PROGRESS + 1, '256M'];
        yield 'each level in a fiber and the next beneath it on its stack, the value nested as deep as the message nests' => [Parser::MAX_NESTING - 1, 'paired', 2 * Parser::MAX_IN_PROGRESS + 1, '256M'];
        yield 'each level on the same stack' => [Parser::MAX_NESTING - 1, 'stack', Parser::MAX_REENTRY, '128M'];
    }

    // A host that re-enters through a new fiber on each level begins each
    // level at a count of its own, and a resolution begun beneath another
    // on its stack takes a place in the process, so the places there end a
    // recursion each of whose fibers resolves two levels. Run apart, since a
    // recursion nothing ended would end the process.
    #[DataProvider('fiberings')]
    public function testContainsARecursionThatResolvesTwoLevelsInEachFiberAtTheBoundOfTheProcess(int $nesting, string $where, int $levels, string $memory): void
    {
        $wrapped = $nesting * $levels;

        self::assertSame(['output' => str_repeat('<', $wrapped) . 'F' . str_repeat('>', $wrapped), 'levels' => $levels, 'reports' => ['unserializable-value']], $this->reenter($memory, (string) $nesting, $where));
    }

    // A resolution begun on a thread of execution where none is in progress
    // is never refused, so a recursion that begins each level in a fiber of
    // its own reaches no bound the parser keeps, past the bound of the
    // process too: the host bounds it, as it bounds the fibers it starts.
    public function testResolvesARecursionThatBeginsEachLevelInAFiberAsDeepAsTheHostLets(): void
    {
        $levels = Parser::MAX_IN_PROGRESS + 1;
        $wrapped = (Parser::MAX_NESTING - 1) * ($levels + 1);

        self::assertSame(['output' => str_repeat('<', $wrapped) . 'E' . str_repeat('>', $wrapped), 'levels' => $levels, 'reports' => []], $this->reenter('128M', (string) (Parser::MAX_NESTING - 1), 'fiber', (string) $levels));
    }

    // Resolutions suspended in fibers are in progress, though nothing runs
    // beneath them, so the count of the process bounds what those begun
    // beneath another hold together, and refuses one begun beneath another
    // past it, wherever it begins; a fiber that ends, or is let go, releases
    // its place, the one let go as PHP destroys it, which no cycle
    // collection needs to run for.
    public function testRefusesAResolutionBegunBeneathAnotherPastTheBoundOfTheProcess(): void
    {
        $parser = $this->parser();
        $fibers = self::park($parser, Parser::MAX_IN_PROGRESS - 1);
        $reentering = new Reentering($parser);

        self::assertSame('F', $parser->resolve('{{x; default:F}}', ['x' => $reentering]));
        self::assertSame(2, $reentering->calls);
        self::assertInstanceOf(ReentryLimit::class, $reentering->refused);
        self::assertStringContainsString('in the process', $reentering->refused->getMessage());
        self::assertSame(['unserializable-value'], $this->codes());

        $fibers = [...$fibers, ...self::park($parser, 1)];

        foreach ([static fn(): string => self::beneath($parser, static fn(): string => $parser->resolve('A')), static fn(): string => self::inFiber(static fn(): string => self::beneath($parser, static fn(): string => $parser->resolve('A')))] as $resolve) {
            self::assertStringContainsString('in the process', $resolve());
        }

        // What a refused resolution was handed is let go inside the
        // containment, so the refusal is all that leaves it, and letting go
        // of the refusal runs nothing it was handed, though its trace keeps
        // the arguments of the frames it was made in.
        $released = static fn(): never => throw new \RuntimeException('released');
        $ignoring = ini_set('zend.exception_ignore_args', '0');

        try {
            foreach ([static fn(): string => $parser->resolve('{{v}}', ['v' => new Releasing($released)]), static fn(): string => $parser->resolve(new Releasing($released))] as $resolve) {
                $outcome = null;
                self::assertSame('x', $parser->resolve('{{v}}', ['v' => new Beneath(static function () use ($resolve, &$outcome): string {
                    try {
                        $resolve();
                        $outcome = 'resolved';
                    } catch (ReentryLimit $refused) {
                        $previous = $refused->getPrevious();
                        unset($refused);
                        $outcome = $previous === null ? 'refused' : 'chained';
                    }

                    return 'x';
                })]));
                self::assertSame('refused', $outcome);
            }
        } finally {
            ini_set('zend.exception_ignore_args', (string) $ignoring);
        }

        $collecting = gc_enabled();
        gc_disable();

        try {
            array_pop($fibers);
            self::assertSame('A', self::beneath($parser, static fn(): string => $parser->resolve('A')));
        } finally {
            if ($collecting) {
                gc_enable();
            }
        }

        $fibers = [...$fibers, ...self::park($parser, 1)];
        $first = array_shift($fibers);
        self::assertInstanceOf(\Fiber::class, $first);
        $first->resume();
        self::assertSame('w', $first->getReturn());
        self::assertSame('A', self::inFiber(static fn(): string => self::beneath($parser, static fn(): string => $parser->resolve('A'))));

        foreach ($fibers as $fiber) {
            $fiber->resume();
            self::assertSame('w', $fiber->getReturn());
        }

        self::assertSame(['unserializable-value'], $this->codes());
    }

    // A resolution asked for on a thread of execution where none is in
    // progress is never refused, however many are in progress in the
    // process: in the main code, at the top of a fiber, or suspended there
    // beside every place the process holds.
    public function testNeverRefusesAResolutionBegunWhereNoneIsInProgressOnItsThread(): void
    {
        $parser = $this->parser();
        $fibers = [...self::park($parser, Parser::MAX_IN_PROGRESS), ...self::park($parser, Parser::MAX_IN_PROGRESS + 1, false)];

        self::assertSame('A', $parser->resolve('A'));
        self::assertSame('A', self::inFiber(static fn(): string => $parser->resolve('A')));
        self::assertStringContainsString('in the process', self::beneath($parser, static fn(): string => $parser->resolve('A')));

        foreach ($fibers as $fiber) {
            $fiber->resume();
            self::assertSame('w', $fiber->getReturn());
        }

        self::assertSame('A', self::beneath($parser, static fn(): string => $parser->resolve('A')));
        self::assertSame([], $this->codes());
    }

    // A fiber resumed inside a resolution runs from its own count, so fibers
    // resumed one inside another are bounded by the fibers the host holds.
    public function testResolvesInFibersResumedOneInsideAnother(): void
    {
        $answer = self::apart('resumed', 'stacked', '128M');

        self::assertIsArray($answer);
        self::assertSame(141, $answer['levels'] ?? null);
        self::assertSame([], $answer['reports'] ?? null);
    }

    /** @return iterable<string, array{string, int, string, list<string>}> */
    public static function keptChains(): iterable
    {
        $placeholders = static fn(int $count): string => str_repeat('{{x:keep; o:{{x:pop}}}}', $count) . '{{x:pop}}';
        $options = static fn(int $count): string => '{{x:keep' . str_repeat('; o:{{x:pop}}', $count) . '}}{{x:pop}}';

        yield 'as many placeholders, each keeping its option, as the bound leaves places' => [$placeholders(Parser::MAX_REENTRY - 1), Parser::MAX_REENTRY - 1, 'e', []];
        yield 'one placeholder more' => [$placeholders(Parser::MAX_REENTRY), Parser::MAX_REENTRY, '', ['failed-modifier']];
        yield 'twenty thousand placeholders' => [$placeholders(20_000), 20_000, '', ['failed-modifier']];
        yield 'one placeholder keeping as many options as the bound leaves places' => [$options(Parser::MAX_REENTRY - 1), 1, 'e', []];
        yield 'one option more' => [$options(Parser::MAX_REENTRY), 1, '', ['failed-modifier']];
    }

    // A modifier may keep what it is handed, and a later placeholder's
    // modifier read it, rendering a text that reads the one kept before it,
    // and so on, at the shallow depth each was kept at. Each such read is a
    // resolution begun inside the one in progress (SPEC.md section 13), and
    // takes a place on its thread as it renders, so the bound ends the
    // chain however long the message: the read past it is the empty string,
    // and the modifier that asked fails.
    /** @param list<string> $codes */
    #[DataProvider('keptChains')]
    public function testBoundsAChainOfTextsKeptFromEarlierPlaceholders(string $message, int $keeps, string $last, array $codes): void
    {
        $kept = [];
        $parser = $this->parser(customModifiers: [
            'keep' => static function (ModifierInput $in) use (&$kept): string {
                array_push($kept, ...$in->options);

                return 'k';
            },
            'pop' => static function () use (&$kept): string {
                $option = array_pop($kept);

                return $option instanceof Option ? $option->value() : 'e';
            },
        ]);

        self::assertSame(str_repeat('k', $keeps) . $last, $parser->resolve($message, ['x' => 1]));
        self::assertSame($codes, $this->codes());
    }

    // A modifier whose read was refused fails whatever it answers after, a
    // catch around the read included: the refusal is the parser's, not a
    // raise the host code can answer for.
    public function testFailsAModifierWhoseReadWasRefusedWhateverItAnswers(): void
    {
        $kept = null;
        $parser = $this->parser(customModifiers: [
            'k2' => static function (ModifierInput $in) use (&$kept): string {
                $kept = $in->options[0];

                return $kept->value();
            },
            'c' => static function () use (&$kept): string {
                try {
                    return $kept instanceof Option ? 'read:' . $kept->value() : 'none';
                } catch (\Throwable) {
                    return 'caught';
                }
            },
        ]);

        self::assertSame('[D]', $parser->resolve('{{x:k2; o:[{{x:c; default:D}}]}}', ['x' => 1]));
        self::assertSame(['failed-modifier'], $this->codes());
    }

    /** @return iterable<string, array{string, bool, string}> */
    public static function fibersReadingARenderingText(): iterable
    {
        foreach (['on the main thread' => false, 'in a fiber' => true] as $where => $inFiber) {
            yield "a fiber the modifier starts, {$where}" => ['{{a:hold; o:[{{a:peek; default:D}}]}}', $inFiber, '[D]'];
            // The modifier that resumes the fiber is the one waiting on it,
            // not the one that started it, which has returned.
            yield "a fiber another modifier started, resumed by the modifier, {$where}" => ['{{a:start; default:SD}}{{a:hold; o:[{{a:resume; default:RD}}]}}', $inFiber, 'S[RD]'];
            yield "a fiber started in a fiber the modifier starts, {$where}" => ['{{a:hold; o:[{{a:through; default:D}}]}}', $inFiber, '[D]'];
            // The innermost host code beneath the fiber is the modifier of a
            // resolution another fiber began, not the one that began it.
            yield "a fiber a modifier starts in a resolution a fiber began, {$where}" => ['{{a:hold; o:[{{a:outer; default:OD}}]}}', $inFiber, '[o:ID]'];
        }
    }

    // A fiber host code starts runs none of the host code the parser called,
    // so a read its own code makes of a text while it is rendering is
    // refused as any is: it answers the empty string, and raises nothing
    // the fiber's code could catch. What fails is the host code the parser
    // called that is waiting on the fiber, beneath it.
    #[DataProvider('fibersReadingARenderingText')]
    public function testFailsTheHostCodeBeneathAFiberWhoseReadWasRefused(string $message, bool $inFiber, string $resolved): void
    {
        /** @var Option|null $kept */
        $kept = null;
        $parked = null;
        /** @var list<string> $reads */
        $reads = [];
        $parser = null;
        $read = static function () use (&$kept, &$reads): string {
            try {
                $text = $kept instanceof Option ? $kept->value() : 'none';
            } catch (\Throwable $thrown) {
                $text = $thrown::class;
            }

            $reads[] = $text;

            return $text;
        };
        $started = static function (\Closure $work): string {
            $fiber = new \Fiber($work);
            $fiber->start();
            $text = $fiber->getReturn();

            return is_string($text) ? $text : '';
        };
        $parser = $this->parser(customModifiers: [
            'hold' => static function (ModifierInput $in) use (&$kept): string {
                $kept = $in->options[0];

                return $kept->value();
            },
            'peek' => static fn(): string => 'p:' . $started($read),
            'start' => static function () use (&$parked, $read): string {
                $parked = new \Fiber(static function () use ($read): string {
                    \Fiber::suspend();

                    return $read();
                });
                $parked->start();

                return 'S';
            },
            'resume' => static function () use (&$parked): string {
                self::assertInstanceOf(\Fiber::class, $parked);
                $parked->resume();
                $text = $parked->getReturn();

                return 'r:' . (is_string($text) ? $text : '');
            },
            'through' => static fn(): string => 't:' . $started(static fn(): string => $started($read)),
            'outer' => static function () use (&$parser, $started): string {
                return 'o:' . $started(static function () use (&$parser): string {
                    self::assertInstanceOf(Parser::class, $parser);

                    return $parser->resolve('{{a:peek; default:ID}}', ['a' => 1]);
                });
            },
        ]);
        $resolve = static fn(): string => $parser->resolve($message, ['a' => 1]);

        self::assertSame($resolved, $inFiber ? $started($resolve) : $resolve());
        self::assertSame([''], $reads);
        self::assertSame(['failed-modifier'], $this->codes());
    }

    // On a thread where the parser runs no host code, and none beneath it,
    // as in main code a fiber the resolution runs in suspended into, a
    // refused read answers the empty string and fails nothing: no host
    // code the parser called is waiting on it.
    public function testAnswersNothingToARefusedReadNoHostCodeTheParserCalledIsBeneath(): void
    {
        $kept = null;
        $parser = $this->parser(customModifiers: [
            'hold' => static function (ModifierInput $in) use (&$kept): string {
                $kept = $in->options[0];

                return $kept->value();
            },
            'wait' => static function (): string {
                \Fiber::suspend();

                return 'w';
            },
        ]);
        $fiber = new \Fiber(static fn(): string => $parser->resolve('{{a:hold; o:[{{a:wait}}]}}', ['a' => 1]));
        $fiber->start();

        self::assertInstanceOf(Option::class, $kept);
        self::assertSame('', $kept->value());
        $fiber->resume();
        self::assertSame('[w]', $fiber->getReturn());
        self::assertSame([], $this->codes());
    }

    /** @return iterable<string, array{string, \Closure(Option): mixed, string, list<string>}> */
    public static function readsAtTheBoundOfTheThread(): iterable
    {
        $nested = str_repeat('{{x:own; o:', Parser::MAX_NESTING - 1) . '{{x:own; o:X}}' . str_repeat('}}', Parser::MAX_NESTING - 1);
        $shown = str_repeat('<', Parser::MAX_NESTING) . 'X' . str_repeat('>', Parser::MAX_NESTING);

        // The innermost resolution is the last the bound lets begin.
        yield 'one more resolution' => ['{{x:again; default:F}}', static fn(): array => ['x' => 1], 'F|O', ['failed-modifier']];
        // What a modifier reads of its own options renders where the message
        // nests it, and takes no place, however deep.
        yield 'options each read by their own modifier, nested as deep as the nesting limit' => [$nested, static fn(): array => ['x' => 1], $shown . '|O', []];
        // A text kept from elsewhere takes a place, and is refused; it
        // records nothing, so its own modifier reads it afterwards.
        yield 'a kept option, read by another modifier' => ['{{x:read; default:F}}', static fn(): array => ['x' => 1], 'F|O', ['failed-modifier']];
        // A fiber of its own is a thread where none is in progress.
        yield 'a kept option, read by another modifier in a fiber of its own' => ['{{x:fork; default:F}}', static fn(): array => ['x' => 1], 'r:O|O', []];
        // A value whose conversion asked for it has no text, and the
        // modifier around it answers.
        yield 'a kept option, read by a value as it converts' => ['{{x:show; o:[{{v; default:F}}]}}', static fn(Option $lent): array => ['x' => 1, 'v' => new Beneath(static fn(): string => $lent->value())], '[F]|O', ['unserializable-value']];
        // What converts a modifier's answer is the modifier's own call.
        yield 'an option, read by its own modifier\'s answer as it converts' => ['{{x:answer; o:A; default:F}}', static fn(): array => ['x' => 1], '<A>|O', []];
        yield 'an option, read by its own modifier\'s answer as it serializes' => ['{{x:serialized; o:A; default:F}}', static fn(): array => ['x' => 1], '["A"]|O', []];
        yield 'a fallback chain, read by its own modifier\'s answer as it converts' => ['{{x:chain; default:D}}', static fn(): array => ['x' => 1], '<D>|O', []];
        yield 'a kept option, read by another modifier\'s answer as it converts' => ['{{x:readAnswer; default:F}}', static fn(): array => ['x' => 1], 'F|O', ['failed-modifier']];
    }

    // A modifier lends its option, then begins resolutions one inside
    // another up to the bound of its thread, the innermost of which
    // resolves `$message`; then it reads the option itself.
    /**
     * @param \Closure(Option): mixed $payload
     * @param list<string> $codes
     */
    #[DataProvider('readsAtTheBoundOfTheThread')]
    public function testReadsTextsWhereTheResolutionsInProgressReachTheBound(string $message, \Closure $payload, string $resolved, array $codes): void
    {
        $lent = null;
        $levels = 0;
        $parser = null;
        $parser = $this->parser(customModifiers: [
            'lend' => static function (ModifierInput $in) use (&$lent, &$parser): string {
                $lent = $in->options[0];
                self::assertInstanceOf(Parser::class, $parser);

                return $parser->resolve('{{x:deep}}', ['x' => 1]) . '|' . $in->options[0]->value();
            },
            'deep' => static function () use (&$lent, &$levels, &$parser, $message, $payload): string {
                $levels += 1;
                self::assertInstanceOf(Parser::class, $parser);
                self::assertInstanceOf(Option::class, $lent);

                // The first is the second resolution in progress.
                return $levels + 2 < Parser::MAX_REENTRY ? $parser->resolve('{{x:deep}}', ['x' => 1]) : $parser->resolve($message, $payload($lent));
            },
            'again' => static function () use (&$parser): string {
                self::assertInstanceOf(Parser::class, $parser);

                return $parser->resolve('A');
            },
            'own' => static fn(ModifierInput $in): string => '<' . $in->options[0]->value() . '>',
            'read' => static function () use (&$lent): string {
                return $lent instanceof Option ? 'r:' . $lent->value() : 'none';
            },
            'fork' => static function () use (&$lent): string {
                $fiber = new \Fiber(static fn(): string => $lent instanceof Option ? 'r:' . $lent->value() : 'none');
                $fiber->start();
                $read = $fiber->getReturn();

                return is_string($read) ? $read : '';
            },
            'show' => static fn(ModifierInput $in): string => $in->options[0]->value(),
            'answer' => static fn(ModifierInput $in): Beneath => new Beneath(static fn(): string => '<' . $in->options[0]->value() . '>'),
            'serialized' => static fn(ModifierInput $in): array => [new class ($in) implements \JsonSerializable {
                public function __construct(private readonly ModifierInput $in) {}

                public function jsonSerialize(): string
                {
                    return $this->in->options[0]->value();
                }
            }],
            'chain' => static fn(ModifierInput $in): Beneath => new Beneath(static fn(): string => '<' . $in->default() . '>'),
            'readAnswer' => static function () use (&$lent): Beneath {
                return new Beneath(static fn(): string => $lent instanceof Option ? 'r:' . $lent->value() : 'none');
            },
        ]);

        self::assertSame($resolved, $parser->resolve('{{x:lend; o:O}}', ['x' => 1]));
        self::assertSame(Parser::MAX_REENTRY - 2, $levels);
        self::assertSame($codes, $this->codes());
    }

    /**
     * What `tests/Scripts/fibers.php` answers for its arguments, run apart
     * under `$memory`.
     *
     * @return array<array-key, mixed>
     */
    private function reenter(string $memory, string ...$arguments): array
    {
        $process = proc_open(
            [PHP_BINARY, '-n', '-d', "memory_limit={$memory}", dirname(__DIR__) . '/Scripts/fibers.php', ...array_values($arguments)],
            [1 => ['pipe', 'w'], 2 => ['pipe', 'w']],
            $pipes,
        );

        self::assertIsResource($process);
        $output = stream_get_contents($pipes[1]);
        $errors = stream_get_contents($pipes[2]);

        self::assertSame(0, proc_close($process), (string) $errors . (string) $output);
        $answered = json_decode((string) $output, true);
        self::assertIsArray($answered);

        return $answered;
    }

    /**
     * Fibers each left suspended in a resolution: one begun beneath another
     * on the fiber's stack, holding one place in the process, or, where
     * `$beneath` is false, the fiber's first, holding none.
     *
     * @return list<\Fiber<mixed, mixed, mixed, mixed>>
     */
    private static function park(Parser $parser, int $count, bool $beneath = true): array
    {
        $waiting = new class implements \Stringable {
            public function __toString(): string
            {
                \Fiber::suspend();

                return 'w';
            }
        };
        $resolve = static fn(): string => $parser->resolve('{{v}}', ['v' => $waiting]);
        $fibers = [];

        for ($at = 0; $at < $count; $at += 1) {
            $fiber = new \Fiber($beneath ? static fn(): string => $parser->resolve('{{v}}', ['v' => new Beneath($resolve)]) : $resolve);
            $fiber->start();
            $fibers[] = $fiber;
        }

        return $fibers;
    }

    /** What `$resolve` answers, or the message of the refusal it raises, run beneath a resolution on this thread. */
    private static function beneath(Parser $parser, \Closure $resolve): string
    {
        return $parser->resolve('{{v}}', ['v' => new Beneath(static function () use ($resolve): string {
            try {
                $text = $resolve();

                return is_string($text) ? $text : '';
            } catch (ReentryLimit $refused) {
                return $refused->getMessage();
            }
        })]);
    }

    /** What `$resolve` answers, run in a fiber of its own. */
    private static function inFiber(\Closure $resolve): string
    {
        $fiber = new \Fiber($resolve);
        $fiber->start();
        $text = $fiber->getReturn();

        return is_string($text) ? $text : '';
    }
}
