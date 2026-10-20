<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\ModifierInput;
use CurlyMessage\Option;
use CurlyMessage\Origin;
use CurlyMessage\Parser;
use CurlyMessage\ReentryLimit;
use CurlyMessage\Undefined;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\Attributes\RequiresPhpExtension;
use PHPUnit\Framework\TestCase;

final class ModifiersTest extends TestCase
{
    use Apart;
    use Resolves;

    public function testCallsAClosureOrAnInvokableObjectWithWhatThePlaceholderDeclares(): void
    {
        $seen = [];
        $parser = $this->parser(customModifiers: [
            'show' => static function (ModifierInput $in) use (&$seen): string {
                $seen[] = [$in->value, array_map(static fn(Option $option): string => $option->key . '=' . $option->value(), $in->options), $in->locale];

                return 'shown';
            },
            'up' => new class {
                public function __invoke(ModifierInput $in): string
                {
                    return strtoupper($in->value);
                }
            },
        ]);

        self::assertSame('shown', $parser->resolve('{{v:show; a:A; b:{{w}}}}', ['v' => 1, 'w' => 'W'], locale: 'cs'));
        self::assertSame('shown', $parser->resolve('{{v:show}}', ['v' => 'x']));
        self::assertSame('ADA', $parser->resolve('{{v:up}}', ['v' => 'ada']));
        self::assertSame([['1', ['a=A', 'b=W'], 'cs'], ['x', [], null]], $seen);
        self::assertSame([], $this->codes());
    }

    public function testRegistersOnlyAnEntryThatIsACallableObject(): void
    {
        $parser = $this->parser(customModifiers: ['up' => 'strtoupper', 'join' => [new Spelled('x'), '__toString'], 'eq' => 'nope']);

        self::assertSame('D', $parser->resolve('{{v:up; default:D}}', ['v' => 'a']));
        self::assertSame('D', $parser->resolve('{{v:join; default:D}}', ['v' => 'a']));
        self::assertSame(['unknown-modifier', 'unknown-modifier'], $this->codes());
        $this->reports = [];
        // An entry that is no modifier shadows nothing: the format's own
        // answers under its name.
        self::assertSame('A', $parser->resolve('{{v:eq; a:A}}', ['v' => 'a']));
        self::assertSame('A', $parser->resolve('{{v; a:A}}', ['v' => 'a']));
        self::assertSame([], $this->codes());
    }

    public function testReadsARegistryFromItsOwnEntries(): void
    {
        $parser = $this->parser(customModifiers: (object) ['__proto__' => static fn(ModifierInput $in): string => 'P', 'constructor' => static fn(ModifierInput $in): string => 'C']);

        self::assertSame('PC', $parser->resolve('{{v:__proto__}}{{v:constructor}}', ['v' => 'a']));
        self::assertSame('D', $parser->resolve('{{v:toString; default:D}}', ['v' => 'a']));
        self::assertSame(['unknown-modifier'], $this->codes());
    }

    public function testLetsAHostReplaceASelectionThatThenAnswersFromNoOptions(): void
    {
        $parser = $this->parser(customModifiers: ['eq' => static fn(ModifierInput $in): string => 'host:' . count($in->options)]);

        self::assertSame('host:0', $parser->resolve('{{v:eq}}', ['v' => 'a']));
        // A placeholder naming no modifier selects with whatever answers to `eq`.
        self::assertSame('host:1', $parser->resolve('{{v; a:A}}', ['v' => 'a']));
        self::assertSame([], $this->codes());
        // The selections it did not replace still select from nothing.
        self::assertSame('', $parser->resolve('{{v:ne}}', ['v' => 'a']));
        self::assertSame(['missing-options'], $this->codes());
    }

    public function testTakesTheChainAndReportsAModifierThatRaises(): void
    {
        $parser = $this->parser(customModifiers: [
            'fail' => static fn(ModifierInput $in): never => throw new \RuntimeException('fail'),
            'typed' => static fn(int $in): string => 'never',
        ]);

        self::assertSame('D', $parser->resolve('{{v:fail; default:D}}', ['v' => 'a']));
        self::assertSame(['failed-modifier'], $this->codes());
        self::assertSame(Origin::Payload, $this->reports[0]->origin);
        self::assertSame('{{v:fail; default:D}}', $this->reports[0]->text);
        self::assertSame('P', $parser->resolve('{{v:typed}}', ['v' => 'a', 'default' => 'P']));
        self::assertSame(['failed-modifier', 'failed-modifier'], $this->codes());
    }

    public function testReadsTheChainOnceForAModifierThatReadItBeforeRaising(): void
    {
        $parser = $this->parser(customModifiers: ['fail' => static function (ModifierInput $in): never {
            $in->default();

            throw new \RuntimeException('fail');
        }]);

        self::assertSame('', $parser->resolve('{{v:fail; default:{{x:nope}}}}', ['v' => 'a']));
        self::assertSame(['unknown-modifier', 'failed-modifier'], $this->codes());
    }

    // The placeholder holds the answer until it has its text, so what the
    // answer runs as it is released is contained with the modifier: one
    // that throws is a modifier that failed. One that reads again the chain
    // still rendering around it runs once its modifier returned, which is
    // then asked no longer: the read is refused, and fails nothing.
    public function testContainsWhatAnAnswerRunsAsItIsReleased(): void
    {
        $kept = null;
        $parser = $this->parser(customModifiers: [
            'keep' => static function (ModifierInput $in) use (&$kept): null {
                $kept = $in;

                return null;
            },
            'reread' => static function (ModifierInput $in) use (&$kept): Releasing {
                return new Releasing(static function () use (&$kept): void {
                    if ($kept instanceof ModifierInput) {
                        $kept->default();
                    }
                });
            },
            'raise' => static fn(ModifierInput $in): Releasing => new Releasing(static fn(): never => throw new \RuntimeException('released')),
        ]);

        self::assertSame('[D]', $parser->resolve('[{{x:raise; default:D}}]', ['x' => 'y']));
        self::assertSame(['failed-modifier'], $this->codes());
        $this->reports = [];
        self::assertSame('[<b>]', $parser->resolve('[{{a:keep; default:<{{x:reread}}>}}]', ['a' => 'v', 'x' => 'y']));
        self::assertSame([], $this->codes());
    }

    // An array answer is let go inside the call that answered it too, though
    // its modifier answers again at a later placeholder.
    public function testContainsWhatAnArrayAnswerRunsAsItIsReleased(): void
    {
        $parser = $this->parser(customModifiers: [
            'raise' => static fn(ModifierInput $in): array => [new Releasing(static fn(): never => throw new \RuntimeException('released'))],
        ]);

        self::assertSame('A|B|C', $parser->resolve('{{a:raise; default:A}}|{{b:raise; default:B}}|{{c:raise; default:C}}', ['a' => 1, 'b' => 2, 'c' => 3]));
        self::assertSame(['failed-modifier', 'failed-modifier', 'failed-modifier'], $this->codes());
    }

    /** @return iterable<string, array{string, int|string, list<array{string, int}>}> */
    public static function letGo(): iterable
    {
        yield 'an option a placeholder reads, then lets go, forty times' => ['separate', 320, []];
        yield 'a long text answered, past the output, thirty times' => ['texts', '', array_fill(0, 30, ['output-limit', Parser::MAX_OUTPUT_LENGTH])];
        yield 'a list holding a long string answered, past the output, thirty times' => ['lists', '', array_fill(0, 30, ['output-limit', Parser::MAX_OUTPUT_LENGTH])];
    }

    // What a placeholder made and let go is no longer held, so placeholders
    // whose texts together pass what memory holds, each within it, resolve
    // as if each were alone. Run apart, at PHP's default memory limit.
    /** @param list<array{string, int}> $reports */
    #[DataProvider('letGo')]
    public function testHoldsNoTextAnEarlierPlaceholderLetGo(string $shape, int|string $output, array $reports): void
    {
        self::assertSame(['output' => $output, 'reports' => $reports], self::apart('holding', $shape, '128M'));
    }

    // A level below the message holds what it places up to the longest text
    // the parser writes: a result that brings it to that length is placed,
    // and one that would carry it past is not, and reports the limit it
    // reached. Run apart, with room for a text that long.
    public function testHoldsALevelBelowTheMessageToTheLongestTextTheParserWrites(): void
    {
        self::assertSame(['output' => (string) Parser::MAX_TEXT_LENGTH, 'reports' => []], self::apart('holding', 'longest', '1G'));
        self::assertSame(['output' => (string) (Parser::MAX_TEXT_LENGTH - 1), 'reports' => [['output-limit', Parser::MAX_TEXT_LENGTH]]], self::apart('holding', 'past', '1G'));
    }

    /** @return iterable<string, array{\Closure(ModifierInput): mixed}> */
    public static function hanging(): iterable
    {
        yield 'an answer hung on the options' => [static function (ModifierInput $in): Releasing {
            $answer = new Releasing(static fn(): never => throw new \RuntimeException('released'));
            // PHP holds the list to no type of element, so a host may.
            /** @phpstan-ignore assign.propertyType */
            $in->options[] = $answer;

            return $answer;
        }];
        yield 'a raise that raises as it is let go' => [static fn(ModifierInput $in): never => throw new Rethrowing(1)];
    }

    // The input is the modifier's to write into, so what it hangs there is let
    // go with the input, inside the modifier's call; and so is a raise, which
    // is host code as it is let go too.
    /** @param \Closure(ModifierInput): mixed $modifier */
    #[DataProvider('hanging')]
    public function testContainsWhatAModifierHangsOnItsInputAsItIsLetGo(\Closure $modifier): void
    {
        $parser = $this->parser(customModifiers: ['hang' => $modifier]);

        self::assertSame('[D]', $parser->resolve('[{{x:hang; default:D}}]', ['x' => 'y']));
        self::assertSame(['failed-modifier'], $this->codes());
    }

    // A raise holds the arguments of each call it passed through, which are
    // let go with it, where the host keeps them.
    public function testContainsWhatAModifiersRaiseHoldsAsItIsLetGo(): void
    {
        $ignored = ini_set('zend.exception_ignore_args', '0');
        $raise = static function (Releasing $held): never {
            throw new \RuntimeException('raised');
        };
        $parser = $this->parser(customModifiers: ['raise' => static fn(ModifierInput $in): never => $raise(new Releasing(static fn(): never => throw new \RuntimeException('released')))]);

        try {
            self::assertSame('[D]', $parser->resolve('[{{x:raise; default:D}}]', ['x' => 'y']));
            self::assertSame(['failed-modifier'], $this->codes());
        } finally {
            ini_set('zend.exception_ignore_args', (string) $ignored);
        }
    }

    /** @return iterable<string, array{\Closure(): string, string}> */
    public static function droppedByAModifier(): iterable
    {
        $released = static fn(): never => throw new \RuntimeException('released');

        yield 'a wrapper the modifier drops from the payload' => [static function () use ($released): string {
            $payload = (object) ['v' => (object) ['value' => new Releasing($released)]];
            $parser = new Parser(customModifiers: ['once' => static function (ModifierInput $in) use ($payload): string {
                unset($payload->v);

                return $in->value;
            }]);

            return $parser->resolve('[{{v:once}}] after', $payload);
        }, '[b] after'];
        yield 'a modifier that drops itself from those the parser was given' => [static function () use ($released): string {
            $modifiers = new \stdClass();
            $modifiers->once = new class ($modifiers, $released) {
                /** @param \Closure(): never $released */
                public function __construct(private readonly \stdClass $modifiers, private readonly \Closure $released) {}

                public function __invoke(ModifierInput $in): string
                {
                    unset($this->modifiers->once);

                    return 'm';
                }

                public function __destruct()
                {
                    ($this->released)();
                }
            };

            return (new Parser(customModifiers: $modifiers))->resolve('[{{v:once}}] after', ['v' => 1]);
        }, '[m] after'];
        yield 'a modifier in a reference slot of those the parser was given, which drops itself' => [static function () use ($released): string {
            $held = new Releasing($released);
            $once = static function (ModifierInput $in) use (&$once, $held): string {
                $once = null;

                return 'm' . $held;
            };
            unset($held);

            return (new Parser(customModifiers: ['once' => &$once]))->resolve('[{{v:once}}] after', ['v' => 1]);
        }, '[mb] after'];
        // The resolution lets go of what it read as it ends, though host
        // code keeps what it was handed, and with it the resolution, longer.
        yield 'an entry its conversion drops from the payload, beside a modifier that keeps its input' => [static function () use ($released): string {
            $kept = null;
            $payload = new \stdClass();
            $payload->v = new Dropping($payload, 'v', $released);
            $payload->k = 1;
            $parser = new Parser(customModifiers: ['keep' => static function (ModifierInput $in) use (&$kept): string {
                $kept = $in;

                return 'k';
            }]);
            $resolved = $parser->resolve('[{{v}}{{k:keep}}] after', $payload);
            $kept = null;

            return $resolved;
        }, '[xk] after'];
        yield 'an array entry the modifier drops from the payload, beside a modifier that keeps its input' => [static function () use ($released): string {
            $kept = null;
            $payload = new \stdClass();
            $payload->v = [new Releasing($released)];
            $payload->k = 1;
            $parser = new Parser(customModifiers: ['keep' => static function (ModifierInput $in) use (&$kept, $payload): string {
                $kept = $in;
                unset($payload->v);

                return 'k';
            }]);
            $resolved = $parser->resolve('[{{v}}{{k:keep}}] after', $payload);
            $kept = null;

            return $resolved;
        }, '[[{}]k] after'];
    }

    // What a modifier drops of what the parser reads leaves the parser
    // holding the last reference to it, which it lets go of where nothing
    // that runs leaves the resolution.
    #[DataProvider('droppedByAModifier')]
    public function testContainsWhatAModifierDropsAsTheParserLetsGoOfIt(\Closure $resolve, string $resolved): void
    {
        self::assertSame($resolved, $resolve());
    }

    /** @return iterable<string, array{string, \Closure(Suspending): array<string, mixed>, string, int}> */
    public static function convertedInFibers(): iterable
    {
        yield 'an object, read in a fiber each' => ['{{k:all; 1:{{o}}; 2:{{o}}}} {{o}}', static fn(Suspending $value): array => ['k' => 1, 'o' => $value], 't1, t1', 1];
        yield 'an array, read in a fiber each by fifty placeholders' => [
            '{{k:all' . str_repeat('; o:{{a:len}}', 50) . '}}',
            static fn(Suspending $value): array => ['k' => 1, 'a' => [...range(1, 1_000), $value]],
            strlen((string) json_encode([...range(1, 1_000), 1])) . str_repeat(',', 49),
            49,
        ];
        yield 'an object, read in a fiber let go while it converts, then again' => ['{{k:drop; 1:{{o}}; 2:{{o}}}} {{o}}', static fn(Suspending $value): array => ['k' => 1, 'o' => $value], ' ', 2];
    }

    // A fiber's read of a value that suspends as it converts leaves the
    // conversion begun; a read in another fiber meets it there, and has no
    // text rather than convert the value again, so a value is converted
    // once however many fibers read it, and every read after the
    // conversion finished answers its text. A conversion a fiber was let
    // go in the middle of never finishes, and is not begun again.
    /** @param \Closure(Suspending): array<string, mixed> $payload */
    #[DataProvider('convertedInFibers')]
    public function testConvertsAValueOnceThoughFibersReadItWhileItConverts(string $message, \Closure $payload, string $resolved, int $refused): void
    {
        $value = new Suspending();
        $parser = $this->parser(customModifiers: [
            'all' => static function (ModifierInput $in): string {
                $fibers = [];

                foreach ($in->options as $option) {
                    $fiber = new \Fiber(static fn(): string => $option->value());
                    $fiber->start();
                    $fibers[] = $fiber;
                }

                $texts = [];

                foreach ($fibers as $fiber) {
                    while (!$fiber->isTerminated()) {
                        $fiber->resume();
                    }

                    $text = $fiber->getReturn();
                    $texts[] = is_string($text) ? $text : '';
                }

                return implode(',', $texts);
            },
            'drop' => static function (ModifierInput $in): string {
                $option = $in->options[0];
                $fiber = new \Fiber(static fn(): string => $option->value());
                $fiber->start();
                unset($fiber);

                return $in->options[1]->value();
            },
            'len' => static fn(ModifierInput $in): int => strlen($in->value),
        ]);

        self::assertSame($resolved, $parser->resolve($message, $payload($value)));
        self::assertSame(1, $value->calls);
        self::assertSame(array_fill(0, $refused, 'unserializable-value'), $this->codes());
    }

    // Once the resolution returned, nothing resolves on its account: a
    // rendering a fiber left suspended in it and resumes then reports
    // nothing and answers the empty string, and so does a read of the text
    // it was rendering, which converts nothing. The fiber's conversion asks
    // nothing more of the value then and is not recorded, so the resolution,
    // which a kept text keeps, holds nothing the conversion read.
    public function testResolvesNothingInARenderingAFiberResumesOnceTheResolutionReturned(): void
    {
        $value = new Suspending();
        $released = 0;
        $payload = new \stdClass();
        $payload->v = 1;
        $after = new Serialized(2);
        $payload->list = [$value, $after, new Releasing(static function () use (&$released): void {
            $released += 1;
        })];
        $fiber = null;
        $kept = null;
        $parser = $this->parser(customModifiers: ['start' => static function (ModifierInput $in) use (&$fiber, &$kept): string {
            $kept = $option = $in->options[0];
            $fiber = new \Fiber(static fn(): string => $option->value());
            $fiber->start();

            return 's';
        }]);

        self::assertSame('s', $parser->resolve('{{v:start; o:<{{list}}>}}', $payload));
        self::assertInstanceOf(Option::class, $kept);
        self::assertSame('', $kept->value());
        unset($payload->list);
        self::assertInstanceOf(\Fiber::class, $fiber);
        $fiber->resume();
        self::assertTrue($fiber->isTerminated());
        self::assertSame('', $fiber->getReturn());
        self::assertSame(1, $value->calls);
        self::assertSame(0, $after->calls);
        self::assertSame(1, $released);
        self::assertSame([], $this->codes());
    }

    /** @return iterable<string, array{string, \Closure(Spelled): array<string, mixed>}> */
    public static function resumedOnceReturned(): iterable
    {
        $wrapped = static fn(Spelled $host): array => ['v' => 1, 'w' => [new Suspending()], 'obj' => $host];

        yield 'an answer, which converts nothing' => ['{{v:start; o:<{{v:susp}}>}}', $wrapped];
        yield 'a value, whose modifier is not called' => ['{{v:start; o:<{{w:host}}>}}', $wrapped];
        yield 'the placeholder after it, which reads nothing' => ['{{v:start; o:<{{v:susp}}{{obj}}>}}', $wrapped];
        yield 'a wrapper\'s default, after which the payload\'s is not read' => [
            '{{v:start; o:<{{w}}>}}',
            static fn(Spelled $host): array => ['v' => 1, 'w' => ['value' => Undefined::Value, 'default' => [new Suspending(), new Serialized(1)]], 'default' => $host],
        ];
        yield 'a value past the nesting limit, whose wrapper\'s default is not read' => [
            '{{v:start; o:<' . str_repeat('{{m; default:', 7) . '{{w}}' . str_repeat('}}', 7) . '>}}',
            static fn(Spelled $host): array => ['v' => 1, 'w' => ['value' => [new Suspending()], 'default' => $host]],
        ];
    }

    // Each step a rendering takes once what it read suspended the fiber it
    // runs in asks whether the resolution returned meanwhile, and runs no
    // host code past it where it did: not to convert what it read, nor to
    // call a modifier, nor to read on.
    /** @param \Closure(Spelled): array<string, mixed> $payload */
    #[DataProvider('resumedOnceReturned')]
    public function testRunsNoHostCodeInARenderingAFiberResumesOnceTheResolutionReturned(string $message, \Closure $payload): void
    {
        $host = new Spelled('h');
        $fiber = null;
        $parser = $this->parser(customModifiers: [
            'start' => static function (ModifierInput $in) use (&$fiber): string {
                $option = $in->options[0];
                $fiber = new \Fiber(static fn(): string => $option->value());
                $fiber->start();

                return 's';
            },
            'susp' => static function () use ($host): Spelled {
                \Fiber::suspend();

                return $host;
            },
            'host' => static fn(): string => (string) $host,
        ]);

        self::assertSame('s', $parser->resolve($message, $payload($host)));
        self::assertInstanceOf(\Fiber::class, $fiber);
        $fiber->resume();
        self::assertTrue($fiber->isTerminated());
        self::assertSame('', $fiber->getReturn());
        self::assertSame(0, $host->calls);
        self::assertSame([], $this->codes());
    }

    /** @return iterable<string, array{mixed}> */
    public static function nothing(): iterable
    {
        yield 'null' => [null];
        yield 'Undefined::Value' => [Undefined::Value];
    }

    #[DataProvider('nothing')]
    public function testTakesTheChainWithoutAReportForAModifierThatAnswersNothing(mixed $answer): void
    {
        $parser = $this->parser(customModifiers: [
            'none' => static fn(ModifierInput $in): mixed => $answer,
            'void' => static function (ModifierInput $in): void {},
        ]);

        self::assertSame('D', $parser->resolve('{{v:none; default:D}}', ['v' => 'a']));
        self::assertSame('D', $parser->resolve('{{v:void; default:D}}', ['v' => 'a']));
        self::assertSame([], $this->codes());
    }

    /** @return iterable<string, array{mixed, string}> */
    public static function answers(): iterable
    {
        yield 'an array' => [['a' => 1, 'b' => [true, null]], '{"a":1,"b":[true,null]}'];
        yield 'a list' => [[1, 'x'], '[1,"x"]'];
        yield 'a stdClass' => [(object) ['a' => '/'], '{"a":"/"}'];
        yield 'an object with text' => [new Spelled('spelled'), 'spelled'];
        yield 'a serializable object, in an array' => [[new Serialized(['s' => 1])], '[{"s":1}]'];
        yield 'an int' => [3, '3'];
        yield 'false' => [false, ''];
        yield 'the empty string' => ['', ''];
    }

    #[DataProvider('answers')]
    public function testConvertsAnAnswerAsAPayloadEntryConverts(mixed $answer, string $text): void
    {
        $parser = $this->parser(customModifiers: ['m' => static fn(ModifierInput $in): mixed => $answer]);

        self::assertSame($text, $parser->resolve('{{v:m; default:D}}', ['v' => 'a']));
        self::assertSame([], $this->codes());
    }

    // An answer converts inside the call that answered it, and is let go
    // there, so an array a modifier answers again, the same or another,
    // converts again at each placeholder it answers at.
    public function testConvertsAnArrayAnswerAtEachPlaceholderItAnswersAt(): void
    {
        $serialized = new Serialized('s');
        $held = [$serialized];
        $parser = $this->parser(customModifiers: [
            'same' => static fn(): array => $held,
            'fresh' => static fn(ModifierInput $in): array => [$serialized, $in->value],
        ]);

        self::assertSame('["s"] ["s"] ["s"]', $parser->resolve('{{a:same}} {{a:same}} {{b:same}}', ['a' => 1, 'b' => 2]));
        self::assertSame(3, $serialized->calls);
        self::assertSame('["s","1"] ["s","2"] ["s","1"]', $parser->resolve('{{a:fresh}} {{b:fresh}} {{a:fresh}}', ['a' => 1, 'b' => 2]));
        self::assertSame(6, $serialized->calls);
        self::assertSame([], $this->codes());
    }

    /** @return iterable<string, array{mixed}> */
    public static function undescribed(): iterable
    {
        yield 'a pure enum' => [Pure::Case];
        yield 'an object with no text' => [new \ArrayObject()];
        yield 'a serializable object, alone' => [new Serialized(['s' => 1])];
        yield 'an object whose text raises' => [new Spelled(null)];
        yield 'a cycle' => [self::cycle()];
    }

    #[DataProvider('undescribed')]
    public function testTakesTheChainAndReportsAnAnswerNothingDescribes(mixed $answer): void
    {
        $parser = $this->parser(customModifiers: ['m' => static fn(ModifierInput $in): mixed => $answer]);

        self::assertSame('D', $parser->resolve('{{v:m; default:D}}', ['v' => 'a']));
        self::assertSame(['unserializable-value'], $this->codes());
    }

    public function testHandsAModifierThePropsComposedUnderItsOwnName(): void
    {
        $seen = [];
        $parser = $this->parser(
            customModifiers: ['m' => static function (ModifierInput $in) use (&$seen): string {
                $seen[] = $in->props->toArray();

                return 'M';
            }],
            modifierDefaults: ['m' => ['a' => 1, 'b' => 1, 'c' => 1], 'eq' => ['other' => 1]],
        );
        $payload = [
            'v' => ['value' => 'x', 'props' => ['m' => (object) ['c' => 3, 'd' => Undefined::Value]]],
            'u' => ['value' => 'x', 'props' => ['m' => ['e' => 5, 'b' => Undefined::Value, 'a' => null]]],
            'w' => 'y',
        ];

        self::assertSame('MMM', $parser->resolve('{{v:m}}{{w:m}}{{u:m}}', $payload, ['m' => ['b' => 2, 'c' => 2]]));
        self::assertSame([['a' => 1, 'b' => 2, 'c' => 3], ['a' => 1, 'b' => 2, 'c' => 2], ['a' => null, 'b' => 2, 'c' => 2, 'e' => 5]], $seen);
    }

    // The properties are read as an array is, a wrapper's own lying over
    // those every placeholder shares, and a property a layer holds the
    // host's undefined under is one it does not name.
    public function testReadsThePropsAModifierIsHandedAsAnArrayIsRead(): void
    {
        $seen = [];
        $parser = $this->parser(
            customModifiers: ['m' => static function (ModifierInput $in) use (&$seen): string {
                $props = $in->props;
                $seen[] = [
                    $props['a'], $props['c'], $props['missing'] ?? 'none', $props->get('missing', 'none'), $props->get('n', 'none'),
                    isset($props['c']), isset($props['n']), isset($props['d']), $props->has('n'), $props->has('d'), $props->has(1.5),
                    count($props), iterator_to_array($props), $props['1'], $props[1],
                ];

                return 'M';
            }],
            modifierDefaults: ['m' => ['a' => 1, 'c' => 1, 'n' => null, 1 => 'one']],
        );
        $payload = ['v' => ['value' => 'x', 'props' => ['m' => ['c' => 3, 'd' => Undefined::Value, 'a' => Undefined::Value]]], 'w' => 'y'];

        self::assertSame('MM', $parser->resolve('{{v:m}}{{w:m}}', $payload));
        self::assertSame([
            [1, 3, 'none', 'none', null, true, false, false, true, false, false, 4, ['a' => 1, 'c' => 3, 'n' => null, 1 => 'one'], 'one', 'one'],
            [1, 1, 'none', 'none', null, true, false, false, true, false, false, 4, ['a' => 1, 'c' => 1, 'n' => null, 1 => 'one'], 'one', 'one'],
        ], $seen);
    }

    // A property a wrapper holds `Undefined::Value` under is one it does not
    // name, where no layer beneath it names any either.
    public function testNamesNoPropertyAWrapperAloneLeavesUndefined(): void
    {
        $seen = [];
        $parser = $this->parser(customModifiers: ['m' => static function (ModifierInput $in) use (&$seen): string {
            $seen[] = [count($in->props), $in->props->toArray(), iterator_to_array($in->props), $in->props->has('d'), $in->props['d'] ?? 'none'];

            return 'M';
        }]);
        $payload = ['v' => ['value' => 'x', 'props' => ['m' => ['d' => Undefined::Value, 'c' => 3]]], 'w' => ['value' => 'y', 'props' => ['m' => ['d' => Undefined::Value]]]];

        self::assertSame('MM', $parser->resolve('{{v:m}}{{w:m}}', $payload));
        self::assertSame([[1, ['c' => 3], ['c' => 3], false, 'none'], [0, [], [], false, 'none']], $seen);
    }

    /** @return iterable<string, array{\Closure(ModifierInput): mixed, string, list<string>}> */
    public static function propsWriters(): iterable
    {
        yield 'a property set' => [static function (ModifierInput $in): string {
            $in->props['written'] = true;

            return 'M';
        }, 'DDD', ['failed-modifier', 'failed-modifier', 'failed-modifier']];
        yield 'a property removed' => [static function (ModifierInput $in): string {
            unset($in->props['a']);

            return 'M';
        }, 'DDD', ['failed-modifier', 'failed-modifier', 'failed-modifier']];
        yield 'a property written by one placeholder alone' => [static function (ModifierInput $in): string {
            if ($in->value === 'x') {
                $in->props['a'] = 2;
            }

            return 'M' . json_encode($in->props['a']);
        }, 'DM1M1', ['failed-modifier']];
        // The near misses: an array of the modifier's own is its to write,
        // and so is what it was handed in place of the properties.
        yield 'a copy written' => [static function (ModifierInput $in): string {
            $props = $in->props->toArray();
            $props['a'] = 2;

            return 'M' . $props['a'] . json_encode($in->props['a']);
        }, 'M21M21M21', []];
    }

    // Every placeholder naming a modifier is handed the properties the
    // shared layers hold under its name, the same ones, so nothing writes
    // into them: a modifier that tries fails as one that raises does, and
    // what the others read is what the layers hold.
    /** @param list<string> $codes */
    #[DataProvider('propsWriters')]
    public function testFailsAModifierThatWritesIntoItsProps(\Closure $modifier, string $resolved, array $codes): void
    {
        $parser = $this->parser(customModifiers: ['m' => $modifier], modifierDefaults: ['m' => ['a' => 1]]);
        $payload = ['v' => 'x', 'w' => 'y', 'u' => ['value' => 'z', 'props' => ['m' => ['b' => 1]]]];

        self::assertSame($resolved, $parser->resolve('{{v:m; default:D}}{{w:m; default:D}}{{u:m; default:D}}', $payload));
        self::assertSame($codes, $this->codes());
    }

    // What is composed is kept by what was composed, not by who asked: a
    // host that registers its own `plural` while a resolution is in
    // progress is handed what the layers hold under `plural`, without the
    // digits of `number` that lie beneath the format's own.
    #[RequiresPhpExtension('intl')]
    public function testHandsAModifierRegisteredMeanwhileThePropsOfItsOwnName(): void
    {
        $registry = new \stdClass();
        $seen = [];
        $registry->reg = static function () use ($registry, &$seen): string {
            $registry->plural = static function (ModifierInput $in) use (&$seen): string {
                $seen[] = $in->props->toArray();

                return 'P';
            };

            return 'R';
        };
        $parser = $this->parser(customModifiers: $registry, modifierDefaults: ['number' => ['minimumFractionDigits' => 2], 'plural' => ['a' => 1]]);
        $payload = ['n' => 1, 'w' => ['value' => 1, 'props' => ['number' => ['minimumFractionDigits' => 3], 'plural' => ['b' => 2]]]];

        self::assertSame('bbRPP', $parser->resolve('{{n:plural; one:a; other:b}}{{w:plural; one:a; other:b}}{{n:reg}}{{n:plural; one:a; other:b}}{{w:plural; one:a; other:b}}', $payload, null, 'en'));
        self::assertSame([['a' => 1], ['a' => 1, 'b' => 2]], $seen);
    }

    public function testKeepsWhatAModifierReordersToItsOwnCall(): void
    {
        $seen = [];
        $parser = $this->parser(customModifiers: ['m' => static function (ModifierInput $in) use (&$seen): string {
            $seen[] = implode(',', array_map(static fn(Option $option): string => $option->key, $in->options));
            $in->options = array_reverse($in->options);

            return $in->options[0]->value();
        }]);

        self::assertSame('BB', $parser->resolve('{{v:m; a:A; b:B}}{{v:m; a:A; b:B}}', ['v' => 'x']));
        self::assertSame(['a,b', 'a,b'], $seen);
    }

    public function testRendersEachOptionItsOwnValueOnceAndOnlyWhereItIsAsked(): void
    {
        $parser = $this->parser(customModifiers: ['both' => static fn(ModifierInput $in): string => $in->options[0]->value() . $in->options[1]->value() . $in->options[1]->value() . $in->options[0]->value()]);

        self::assertSame('XYYX', $parser->resolve('{{v:both; a:{{x}}; b:{{y}}; c:{{z:nope}}}}', ['v' => 'v', 'x' => 'X', 'y' => 'Y']));
        self::assertSame([], $this->codes());
        self::assertSame('YY', $parser->resolve('{{v:both; a:; b:{{y:nope; default:Y}}}}', ['v' => 'v']));
        self::assertSame(['unknown-modifier'], $this->codes());
    }

    // Once the resolution returned, a text a modifier kept answers what a
    // read of it during the resolution rendered, and the empty string where
    // nothing read it then: after a resolution returns, nothing resolves,
    // reports or runs host code on its account (SPEC.md section 11.3), and
    // what the text places, an array included, is not converted again.
    public function testAnswersATextKeptPastItsResolutionWithWhatTheResolutionRenderedOfIt(): void
    {
        $kept = [];
        $parser = $this->parser(customModifiers: [
            'read' => static function (ModifierInput $in) use (&$kept): string {
                $kept[] = $in;

                return $in->options[0]->value() . '+' . $in->default();
            },
            'keep' => static function (ModifierInput $in) use (&$kept): string {
                $kept[] = $in;

                return 'now';
            },
        ]);
        $list = new Serialized('l');
        $read = new Spelled('s');
        $unread = new Spelled('u');

        self::assertSame('A["l"]s+Ds|now', $parser->resolve(
            '{{v:read; a:A{{w:zz}}{{list}}{{s}}; b:B{{u}}; default:D{{s}}}}|{{v:keep; a:A{{w:zz}}{{u}}; default:D{{w:zz}}{{u}}}}',
            ['v' => 1, 'list' => [$list], 's' => $read, 'u' => $unread],
        ));
        self::assertSame(['unknown-modifier'], $this->codes());
        self::assertCount(2, $kept);
        self::assertSame(
            ['A["l"]s', 'Ds', '', '', ''],
            [$kept[0]->options[0]->value(), $kept[0]->default(), $kept[0]->options[1]->value(), $kept[1]->options[0]->value(), $kept[1]->default()],
        );
        self::assertSame('A["l"]s', $kept[0]->options[0]->value());
        self::assertSame([1, 1, 0], [$list->calls, $read->calls, $unread->calls]);
        self::assertSame(['unknown-modifier'], $this->codes());
    }

    /** @return iterable<string, array{int, bool}> */
    public static function keptChains(): iterable
    {
        yield 'one text' => [1, true];
        yield 'as many as take the last place' => [Parser::MAX_REENTRY - 1, true];
        yield 'one more' => [Parser::MAX_REENTRY, false];
        yield 'twenty thousand' => [20000, false];
    }

    // A text kept from one placeholder and read by the modifier of another
    // renders at the level it was declared at, which the nesting limit does
    // not count, so a flat message could have each placeholder read a text
    // kept from the one before, each inside the last. Such a rendering takes
    // a place among the resolutions in progress on its thread, where the
    // resolution itself holds the first: the read that finds none renders
    // nothing, and the placeholder whose modifier asked fails.
    #[DataProvider('keptChains')]
    public function testCountsATextReadOutsideTheCallItWasHandedToAmongTheResolutionsInProgress(int $kept, bool $resolves): void
    {
        $texts = [];
        $reading = 0;
        $deepest = 0;
        $parser = $this->parser(customModifiers: [
            'k' => static function (ModifierInput $in) use (&$texts): string {
                $texts[] = $in->options[0];

                return 'k';
            },
            'm' => static function () use (&$texts, &$reading, &$deepest): string {
                $text = array_pop($texts);
                $deepest = max($deepest, $reading);

                if (!$text instanceof Option) {
                    return 'e';
                }

                $reading += 1;

                try {
                    return $text->value();
                } finally {
                    $reading -= 1;
                }
            },
        ]);

        self::assertSame(str_repeat('k', $kept) . ($resolves ? 'e' : ''), $parser->resolve(str_repeat('{{x:k; o:{{x:m}}}}', $kept) . '{{x:m}}', ['x' => 1]));
        self::assertSame($resolves ? [] : ['failed-modifier'], $this->codes());
        self::assertSame(min($kept, Parser::MAX_REENTRY - 1), $deepest);
        self::assertSame('A', $parser->resolve('A'));
    }

    // The bound is on how deep such reads nest, not on how many a message
    // makes: one that returned before the next began holds no place.
    public function testReadsAnyNumberOfKeptTextsOneAfterAnother(): void
    {
        $texts = [];
        $parser = $this->parser(customModifiers: [
            'k' => static function (ModifierInput $in) use (&$texts): string {
                $texts[] = $in;

                return 'k';
            },
            'm' => static function () use (&$texts): string {
                $in = array_shift($texts);

                return $in instanceof ModifierInput ? $in->options[0]->value() . $in->default() : 'e';
            },
        ]);
        $count = 4 * Parser::MAX_REENTRY;

        self::assertSame(
            str_repeat('k', $count) . str_repeat('VD', $count) . 'e',
            $parser->resolve(str_repeat('{{x:k; o:{{v}}; default:D}}', $count) . str_repeat('{{x:m}}', $count + 1), ['x' => 1, 'v' => 'V']),
        );
        self::assertSame([], $this->codes());
    }

    // A modifier reading what its own call was handed renders it where the
    // message nests it, which the nesting limit bounds, so it takes no
    // place: a message nested as deep as the limit allows, each level
    // reading its option and its fallback chain, resolves beneath as many
    // resolutions as one thread holds.
    public function testTakesNoPlaceForATextReadByTheCallItWasHandedTo(): void
    {
        $message = 'X';
        $expected = 'X';

        for ($level = 0; $level < Parser::MAX_NESTING; $level += 1) {
            $message = "{{x:own; o:<{$message}>; default:D}}";
            $expected = "<{$expected}>D";
        }

        $beneath = new class {
            /** @var (\Closure(int): string)|null */
            public ?\Closure $resolve = null;
        };
        $parser = $this->parser(customModifiers: [
            'own' => static fn(ModifierInput $in): string => $in->options[0]->value() . $in->default(),
            'down' => static fn(ModifierInput $in): ?string => $beneath->resolve === null ? null : ($beneath->resolve)((int) $in->value - 1),
        ]);
        $resolve = $beneath->resolve = static fn(int $left): string => $left === 0 ? $parser->resolve($message, ['x' => 1]) : $parser->resolve('{{left:down}}', ['left' => $left]);

        self::assertSame($expected, $resolve(Parser::MAX_REENTRY - 1));
        self::assertSame([], $this->codes());
        self::assertSame('', $resolve(Parser::MAX_REENTRY));
        self::assertSame(['failed-modifier'], $this->codes());
    }

    // A refused read rendered nothing and answered the empty string, and
    // the modifier that made it fails whatever it then answers: its
    // placeholder takes its fallback chain, and the modifiers around it,
    // which made no refused read, read what that chain gave. Nothing was
    // recorded for the text, so a read with a place to take renders it.
    public function testLeavesATextRefusedForWantOfAPlaceToBeReadLater(): void
    {
        $texts = [];
        $refused = null;
        $answered = [];
        $parser = $this->parser(customModifiers: [
            'k' => static function (ModifierInput $in) use (&$texts): string {
                $texts[] = $in->options[0];

                return 'k';
            },
            'm' => static function () use (&$texts, &$refused, &$answered): string {
                $text = array_pop($texts);

                if (!$text instanceof Option) {
                    return 'e';
                }

                $value = $text->value();

                if ($value === '') {
                    $refused ??= $text;
                    $answered[] = 'r';

                    return 'r';
                }

                return $value;
            },
            'last' => static function () use (&$refused): string {
                return $refused instanceof Option ? '|' . $refused->value() : '';
            },
        ]);
        $kept = Parser::MAX_REENTRY;

        self::assertSame(str_repeat('k', $kept) . 'd|e', $parser->resolve(str_repeat('{{x:k; o:{{x:m; default:d}}}}', $kept) . '{{x:m}}{{x:last}}', ['x' => 1]));
        self::assertSame(['r'], $answered);
        self::assertSame(['failed-modifier'], $this->codes());
    }

    // A text asked for from inside its own rendering is refused at once,
    // whatever places there are: the read answers the empty string, and
    // the modifier being asked fails. Where a value's conversion made the
    // read, no modifier is being asked, and nothing fails.
    public function testAnswersTheEmptyStringToAReadFromInsideTheTextsOwnRendering(): void
    {
        /** @var \ArrayObject<int, Option> $kept */
        $kept = new \ArrayObject();
        /** @var \ArrayObject<int, string> $saw */
        $saw = new \ArrayObject();
        $read = static function () use ($kept, $saw): string {
            $saw[] = $text = isset($kept[0]) ? $kept[0]->value() : 'none';

            return 'I' . $text;
        };
        $parser = $this->parser(customModifiers: [
            'outer' => static function (ModifierInput $in) use ($kept): string {
                $kept[0] = $in->options[0];

                return '[' . $kept[0]->value() . ']';
            },
            'inner' => $read,
        ]);

        self::assertSame('[<D>]', $parser->resolve('{{a:outer; o:<{{a:inner; default:D}}>}}', ['a' => 1]));
        self::assertSame([['failed-modifier'], ['']], [$this->codes(), $saw->getArrayCopy()]);
        $this->reports = [];
        $saw->exchangeArray([]);
        self::assertSame('[<I>]', $parser->resolve('{{a:outer; o:<{{s; default:D}}>}}', ['a' => 1, 's' => new Reading($read)]));
        self::assertSame([[], ['']], [$this->codes(), $saw->getArrayCopy()]);
        // The near miss: once the rendering finished, the text is read.
        $this->reports = [];
        $saw->exchangeArray([]);
        self::assertSame('[<>]I<>', $parser->resolve('{{a:outer; o:<>}}{{a:inner}}', ['a' => 1]));
        self::assertSame([[], ['<>']], [$this->codes(), $saw->getArrayCopy()]);
    }

    // A resolution a modifier began asks it no longer while it runs, and
    // gives it back once it returned: a read refused after that fails the
    // modifier as one refused before it does.
    public function testAsksAModifierAgainOnceAResolutionItBeganReturned(): void
    {
        /** @var \ArrayObject<int, Option> $kept */
        $kept = new \ArrayObject();
        $nested = new Parser();
        $parser = $this->parser(customModifiers: [
            'outer' => static function (ModifierInput $in) use ($kept): string {
                $kept[0] = $in->options[0];

                return '[' . $kept[0]->value() . ']';
            },
            'inner' => static fn(ModifierInput $in): string => $nested->resolve('N') . (isset($kept[0]) ? $kept[0]->value() : 'none'),
        ]);

        self::assertSame('[<D>]', $parser->resolve('{{a:outer; o:<{{a:inner; default:D}}>}}', ['a' => 1]));
        self::assertSame(['failed-modifier'], $this->codes());
    }

    /** @return iterable<string, array{bool, int, string, list<string>}> */
    public static function askedNoLonger(): iterable
    {
        yield 'by the answer as it converts, with a place to take' => [false, Parser::MAX_REENTRY - 1, '<T>', []];
        yield 'by the answer as it converts, with none' => [false, Parser::MAX_REENTRY, '<>', []];
        yield 'by a value of a resolution the modifier began, with a place to take' => [true, Parser::MAX_REENTRY - 2, '<T>', []];
        yield 'by a value of a resolution the modifier began, with none' => [true, Parser::MAX_REENTRY - 1, '<>', []];
    }

    // A modifier is being asked while its call is the innermost the parser
    // made into host code: what its answer runs as it converts, and what a
    // resolution it began calls, read its option from outside, so the read
    // takes a place, and where there is none it answers the empty string
    // and fails nothing.
    /** @param list<string> $codes */
    #[DataProvider('askedNoLonger')]
    public function testCountsWhatReadsAModifiersOptionOnceItIsAskedNoLonger(bool $nested, int $resolutions, string $resolved, array $codes): void
    {
        $parser = null;
        $depth = 1;
        $parser = $this->parser(customModifiers: [
            'deep' => static function () use (&$parser, &$depth, $resolutions): string {
                $depth += 1;

                return $parser instanceof Parser ? $parser->resolve($depth < $resolutions ? '{{a:deep}}' : '{{a:m; o:T}}', ['a' => 1]) : '';
            },
            'm' => static function (ModifierInput $in) use (&$parser, $nested): mixed {
                $option = $in->options[0];
                $reading = new Reading(static fn(): string => '<' . $option->value() . '>');

                return $nested && $parser instanceof Parser ? $parser->resolve('{{s}}', ['s' => $reading]) : $reading;
            },
        ]);

        self::assertSame($resolved, $parser->resolve($resolutions > 1 ? '{{a:deep}}' : '{{a:m; o:T}}', ['a' => 1]));
        self::assertSame($codes, $this->codes());
    }

    public function testRendersOneOptionOfAThousandForAModifierThatAsksForOne(): void
    {
        $parser = $this->parser(customModifiers: ['pick' => static fn(ModifierInput $in): string => $in->options[500]->value()]);
        $options = '';

        for ($index = 0; $index < 1000; $index += 1) {
            $options .= "; k{$index}:{{x:nope; default:{$index}}}";
        }

        self::assertSame('500', $parser->resolve("{{v:pick{$options}}}", ['v' => 'v']));
        self::assertSame(['unknown-modifier'], $this->codes());
    }

    public function testReadsNoLinkOfTheChainForAModifierThatNeverAsks(): void
    {
        $parser = $this->parser(customModifiers: ['m' => static fn(ModifierInput $in): string => 'M']);
        $default = new Spelled('P');

        self::assertSame('M', $parser->resolve('{{v:m; default:{{x:nope}}}}', ['v' => 'a', 'default' => $default]));
        self::assertSame(0, $default->calls);
        self::assertSame([], $this->codes());
    }

    public function testBoundsAnAnswerByTheOutputAndSpendsNothingOfTheRead(): void
    {
        $answer = str_repeat('a', Parser::MAX_OUTPUT_LENGTH + 1);
        $parser = $this->parser(customModifiers: [
            'big' => static fn(ModifierInput $in): string => $answer,
            'drop' => static fn(ModifierInput $in): string => strlen($in->options[0]->value()) . ':',
        ]);

        self::assertSame('', $parser->resolve('{{v:big}}', ['v' => 'a']));
        self::assertSame(['output-limit'], $this->codes());
        $this->reports = [];
        self::assertSame(strlen($answer) . ':w', $parser->resolve('{{v:drop; a:{{v:big}}}}{{w}}', ['v' => 'a', 'w' => 'w']));
        self::assertSame([], $this->codes());
    }

    public function testContainsAModifierThatResolvesThroughItselfPastTheLimit(): void
    {
        $calls = 0;
        $parser = null;
        $parser = $this->parser(customModifiers: ['again' => static function (ModifierInput $in) use (&$parser, &$calls): string {
            $calls += 1;
            self::assertInstanceOf(Parser::class, $parser);

            return '[' . $parser->resolve('{{v:again; default:D}}', ['v' => 'a']) . ']';
        }]);
        $depth = Parser::MAX_REENTRY - 1;

        self::assertSame(str_repeat('[', $depth) . 'D' . str_repeat(']', $depth), $parser->resolve('{{v:again; default:D}}', ['v' => 'a']));
        self::assertSame(Parser::MAX_REENTRY, $calls);
        self::assertSame(['failed-modifier'], $this->codes());
    }

    // A fiber let go while a text host code asked for renders gives back
    // the place its resolution, begun beneath another, took in the process
    // as PHP destroys the fiber, and no cycle collection needs to run for
    // it. The read took none there, so once one of the fibers that hold
    // every place is let go, one more resolution reading an option fits,
    // and the next is refused.
    public function testReleasesThePlaceOfAResolutionLetGoInARead(): void
    {
        $waiting = new class implements \Stringable {
            public function __toString(): string
            {
                \Fiber::suspend();

                return 'w';
            }
        };
        $parser = $this->parser(customModifiers: ['read' => static fn(ModifierInput $in): string => $in->options[0]->value()]);
        $beneath = static fn(\Closure $resolve): string => $parser->resolve('{{v}}', ['v' => new Beneath($resolve)]);
        $park = static function () use ($parser, $waiting, $beneath): \Fiber {
            $fiber = new \Fiber(static fn(): string => $beneath(static fn(): string => $parser->resolve('{{a:read; o:{{v}}}}', ['a' => 'a', 'v' => $waiting])));
            $fiber->start();
            self::assertTrue($fiber->isSuspended());

            return $fiber;
        };
        $refused = static function () use ($parser, $beneath): void {
            $beneath(static function () use ($parser): string {
                try {
                    $parser->resolve('A');
                    self::fail('A resolution begun past the bound of the process was not refused.');
                } catch (ReentryLimit $refused) {
                    self::assertStringContainsString('in the process', $refused->getMessage());
                }

                return '';
            });
        };
        $fibers = [];

        for ($at = 0; $at < Parser::MAX_IN_PROGRESS; $at += 1) {
            $fibers[] = $park();
        }

        $refused();
        $collecting = gc_enabled();
        gc_disable();

        try {
            array_shift($fibers);
            $fibers[] = $park();
            $refused();
        } finally {
            if ($collecting) {
                gc_enable();
            }
        }

        foreach ($fibers as $fiber) {
            $fiber->resume();
            self::assertSame('w', $fiber->getReturn());
        }

        self::assertSame([], $this->codes());
    }

    // A resolution is in progress until its `resolve` returns, and gives
    // back the place it took in the process then, though a read it began is
    // left suspended in a fiber: the fiber resumed later finishes nothing on
    // its account.
    public function testGivesBackThePlaceOfAResolutionAsItReturnsThoughAReadItBeganIsSuspended(): void
    {
        $waiting = new class implements \Stringable {
            public function __toString(): string
            {
                \Fiber::suspend();

                return 'w';
            }
        };
        $fibers = [];
        $parser = $this->parser(customModifiers: ['fork' => static function (ModifierInput $in) use (&$fibers): string {
            $option = $in->options[0];
            $fiber = new \Fiber(static fn(): string => $option->value());
            $fiber->start();
            $fibers[] = $fiber;

            return 'r';
        }]);
        $beneath = static fn(\Closure $resolve): string => $parser->resolve('{{v}}', ['v' => new Beneath($resolve)]);

        for ($at = 0; $at < Parser::MAX_IN_PROGRESS; $at += 1) {
            self::assertSame('r', $beneath(static fn(): string => $parser->resolve('{{a:fork; o:{{w}}}}', ['a' => 1, 'w' => $waiting])));
        }

        self::assertSame('A', $beneath(static fn(): string => $parser->resolve('A')));
        self::assertCount(Parser::MAX_IN_PROGRESS, $fibers);

        foreach ($fibers as $fiber) {
            $fiber->resume();
            self::assertSame('', $fiber->getReturn());
        }

        self::assertSame([], $this->codes());
    }

    /** @return iterable<string, array{int, string, list<string>}> */
    public static function readsAtTheBound(): iterable
    {
        yield 'a fallback chain asked for where as many resolutions as the bound of its thread holds are in progress' => [Parser::MAX_REENTRY - 2, 'got:F', []];
        yield 'the resolution that would ask for it, begun past that bound' => [Parser::MAX_REENTRY - 1, '', ['unserializable-value']];
    }

    // A text host code asks for is the walk of its resolution and takes no
    // place among the resolutions in progress on its thread, so one asked
    // for while they reach the bound renders: what the bound refuses is a
    // resolution begun past it.
    /** @param list<string> $codes */
    #[DataProvider('readsAtTheBound')]
    public function testRendersATextHostCodeAsksForWhereTheResolutionsInProgressReachTheBound(int $levels, string $resolved, array $codes): void
    {
        $parser = $this->parser(customModifiers: ['ask' => static fn(ModifierInput $in): string => 'got:' . $in->default()]);
        $level = new class ($parser, $levels) implements \Stringable {
            public function __construct(private readonly Parser $parser, private readonly int $left) {}

            public function __toString(): string
            {
                return $this->left === 0
                    ? $this->parser->resolve('{{x:ask; default:F}}', ['x' => 1])
                    : $this->parser->resolve('{{v}}', ['v' => new self($this->parser, $this->left - 1)]);
            }
        };

        self::assertSame($resolved, $parser->resolve('{{v}}', ['v' => $level]));
        self::assertSame($codes, $this->codes());
    }

    // A modifier may read each of its options in a fiber of its own, as an
    // async host awaiting them all at once does. However many it reads so,
    // they take no place in the process, which counts resolutions, so it
    // reads more of them than the process holds resolutions, and a
    // resolution begun while they wait, in the main code or a fiber,
    // resolves.
    public function testReadsOptionsInMoreFibersThanTheProcessHoldsResolutions(): void
    {
        $parser = $this->parser(customModifiers: [
            'all' => static function (ModifierInput $in): string {
                $fibers = [];

                foreach ($in->options as $option) {
                    $fiber = new \Fiber(static fn(): string => $option->value());
                    $fiber->start();
                    $fibers[] = $fiber;
                }

                \Fiber::suspend();
                $texts = [];

                foreach ($fibers as $fiber) {
                    $fiber->resume();
                    $text = $fiber->getReturn();
                    $texts[] = is_string($text) ? $text : '';
                }

                return implode(',', $texts);
            },
            'fetch' => static function (ModifierInput $in): string {
                \Fiber::suspend();

                return 'v' . $in->value;
            },
        ]);
        $count = Parser::MAX_IN_PROGRESS + 1;
        $request = new \Fiber(static fn(): string => $parser->resolve('{{x:all' . str_repeat('; k:{{x:fetch}}', $count) . '; default:F}}', ['x' => 1]));
        $request->start();
        $other = new \Fiber(static fn(): string => $parser->resolve('Hello {{name}}', ['name' => 'C']));
        $other->start();

        self::assertSame('Hello B', $parser->resolve('Hello {{name}}', ['name' => 'B']));
        self::assertSame('Hello C', $other->getReturn());
        $request->resume();
        self::assertSame(implode(',', array_fill(0, $count, 'v1')), $request->getReturn());
        self::assertSame([], $this->codes());
    }

    /** @return iterable<string, array{list<string>, string, int, list<string>}> */
    public static function rereadings(): iterable
    {
        // A text read from inside its own rendering is refused, however
        // deep the texts that read it lie: rendering it again would double
        // what each level walks.
        yield 'an option value' => [['option'], '[]', 2, ['failed-modifier']];
        yield 'the fallback chain' => [['default'], '[]', 2, ['failed-modifier']];
        yield 'texts nested seven deep, each read from inside itself' => [['stacked', '100'], str_repeat('<', Parser::MAX_NESTING - 1) . str_repeat('>', Parser::MAX_NESTING - 1), 100, array_fill(0, Parser::MAX_NESTING - 1, 'failed-modifier')];
        // A text renders once: host code asking for it while another
        // fiber's rendering is suspended is refused, and the parser's own
        // read, where the message nests it, is the empty string.
        yield 'the fallback chain, read by the parser after a modifier that raises, beside a fiber suspended in it' => [['forked', 'default', 'raise'], '', 1, ['failed-modifier']];
        // A read refused in a fiber the modifier started is refused on a
        // thread of execution where no modifier is being asked, and fails
        // nothing there.
        yield 'the fallback chain, read by the parser after a modifier that answers nothing, beside a fiber suspended in it' => [['forked', 'default', 'nothing'], '', 1, []];
        yield 'an option, read by its modifier from its own call, beside a fiber suspended in it' => [['forked', 'option'], '[p]', 1, ['failed-modifier']];
        // A text read outside the call it was handed to takes a place among
        // the resolutions in progress on its thread while it renders, so
        // texts read each inside the one before nest as deep as resolutions
        // do: the first modifier reads its own option, which takes none, and
        // the read that finds no place is refused.
        yield 'each option in turn, from inside the one before, as deep as resolutions nest' => [['distinct', (string) Parser::MAX_REENTRY], 'end', Parser::MAX_REENTRY + 1, []];
        yield 'each option in turn, from inside the one before, one deeper' => [['distinct', (string) (Parser::MAX_REENTRY + 1)], '', Parser::MAX_REENTRY + 1, ['failed-modifier']];
        yield 'each of a hundred options in turn, from inside the one before' => [['distinct', '100'], '', Parser::MAX_REENTRY + 1, ['failed-modifier']];
        yield 'each option in turn, each answer marked, beside a payload default, as deep as resolutions nest' => [['chain', (string) Parser::MAX_REENTRY], 'end' . str_repeat('x', Parser::MAX_REENTRY + 1), Parser::MAX_REENTRY + 1, []];
        yield 'each of a hundred options in turn, each answer marked, beside a payload default' => [['chain', '100'], 'F' . str_repeat('x', Parser::MAX_REENTRY), Parser::MAX_REENTRY + 1, ['failed-modifier']];
        yield 'a text nested six deep, kept and read from inside another nested three deep' => [['nested'], 'K|<<<<<<<<<X>>>>>>>>>', 1, []];
        yield 'an option read in a fiber left suspended, beside an ordinary message nested eight deep' => [['suspended'], str_repeat('h', 8) . '|<<<<<<<X>>>>>>>', 0, []];
        // A read on a thread of execution where no resolution is in progress
        // takes no place in the process, so however many fibers a resolution
        // leaves suspended in reads they each began, the rest of it resolves.
        yield 'options read in more fibers left suspended than the process holds resolutions, before a kept text is read' => [['parked', (string) (Parser::MAX_IN_PROGRESS + 1)], str_repeat('h', Parser::MAX_IN_PROGRESS + 1) . '|KX', 1, []];
        // A text kept from another placeholder is out of where the message
        // nests the read, however deep the two lie, so the nesting limit
        // counts none of these and the places they take bound them.
        yield 'texts kept at each depth, each read from inside the one before, mostly one level deeper' => [['cycling'], 'TTTTTTT-|', Parser::MAX_REENTRY, ['failed-modifier']];
        // A text a suspended fiber is rendering is refused here.
        yield 'a text a suspended fiber is rendering, read twice from inside itself' => [['twice'], 'k', 1, ['failed-modifier']];
        yield 'an option a suspended fiber is rendering, read again, then after the fiber ends' => [['resumed'], '', 1, ['failed-modifier']];
        // Host code the parser calls while a text renders is not the call to
        // the modifier the text was handed to, so what it reads of that
        // modifier's options takes a place too: a refused read answers the
        // empty string and fails nothing, a handler's leaving its report
        // delivered and the next option unread, and a value's leaving the
        // value the text it then answers.
        yield 'each option in turn, read by the report handler from inside the one before, as deep as resolutions nest' => [['reported', (string) Parser::MAX_REENTRY], '', Parser::MAX_REENTRY, array_fill(0, Parser::MAX_REENTRY, 'unknown-modifier')];
        yield 'each of a hundred options in turn, read by the report handler from inside the one before' => [['reported', '100'], '', Parser::MAX_REENTRY, array_fill(0, Parser::MAX_REENTRY, 'unknown-modifier')];
        yield 'each option in turn, read by the value the one before places, as deep as resolutions nest' => [['converted', (string) Parser::MAX_REENTRY], '', Parser::MAX_REENTRY, []];
        yield 'each of a hundred options in turn, read by the value the one before places' => [['converted', '100'], '', Parser::MAX_REENTRY, []];
        // A value is converted once: read from inside its own conversion, it
        // has no text there.
        yield 'each option in turn, read by the one value they all place' => [['reconverted', '100'], '', 1, ['unserializable-value']];
        // A text kept from a resolution that returned, which nothing read
        // during it, is the empty string, and renders nothing.
        yield 'options kept from a thousand resolutions, each read from inside the one before once they returned' => [['across', '1000'], '', 1, []];
        // A text read twice by each of its renderings, each time in a new
        // fiber that suspends it, would double at each level and never be
        // kept: past the one rendering in progress it is refused, whether
        // the host keeps the fibers or lets them go. The refused reads are
        // made in fibers, where no modifier is being asked, so the modifier
        // that started them answers.
        yield 'a text read twice by each rendering of it, in fibers kept suspended' => [['fanned'], 'kf', 2, []];
        yield 'a text read twice by each rendering of it, in fibers let go suspended' => [['dropped'], 'kf', 2, []];
        // A rendering a fiber was let go in the middle of never finishes, and
        // the text stays one that began rendering: read again, it is
        // refused rather than rendered afresh, so a modifier doing so at
        // each level walks each level once.
        yield 'an option read in a fiber let go while it renders, then again, at each of seven levels' => [['abandoned', '0'], '[[[[[[]]]]]]', 8, ['failed-modifier']];
        yield 'the same around a placeholder that resolves the message again' => [['abandoned', '1'], str_repeat('[', 13) . str_repeat(']', 13), 16, ['failed-modifier']];
        yield 'the same around placeholders that resolve the message again two deep, for an option' => [['abandoned', '2'], str_repeat('[', 20) . str_repeat(']', 20), 24, ['failed-modifier']];
        // The parser's own read of a chain whose rendering a fiber left
        // unfinished renders it no second time, whether the fiber was let go
        // or is resumed later, so each level is walked once.
        yield 'the fallback chain read in a fiber let go while it renders, then by the parser, at each of seven levels' => [['forsaken', '0'], '[[[[[[]]]]]]', 8, []];
        yield 'the same around placeholders that resolve the message again two deep' => [['forsaken', '2'], str_repeat('[', 20) . str_repeat(']', 20), 24, []];
        yield 'the fallback chain read in a fiber kept and resumed, then by the parser, around placeholders that resolve the message again two deep' => [['scheduled', '2'], str_repeat('[', 20) . str_repeat(']', 20), 24, []];
        // The one rendering's text is what every read after it answers.
        yield 'a fallback chain a fiber left suspended, read by the parser, then by the fiber a later placeholder resumes, then by the one after it' => [['stranded'], '|<r1>|<r1>', 1, ['failed-modifier']];
    }

    // A modifier that kept what an outer placeholder handed it can ask for
    // that text again from inside it, before it has an answer to give, or
    // ask for each other option it kept from inside the one before, and so
    // can host code the parser calls while one renders. A text read by the
    // call it was handed to renders where the message nests it; one read
    // from anywhere else takes a place among the resolutions in progress
    // while it renders, and is refused where there is none. Each text
    // renders at most once, and a text asked for once a rendering of it
    // began and before one finished, in any fiber, is refused, since
    // rendering it again would double what each level walks. A refused
    // read answers the empty string and fails the modifier being asked on
    // its thread of execution, where one is. A text rendering where the
    // message nests it is bounded by the nesting limit.
    // Run apart, at PHP's default memory limit, since a recursion nothing
    // ended would end the process.
    /**
     * @param list<string> $read
     * @param list<string> $codes
     */
    #[DataProvider('rereadings')]
    public function testRefusesTextReadAgainWhileItIsRendered(array $read, string $resolved, int $calls, array $codes): void
    {
        $process = proc_open(
            [PHP_BINARY, '-n', '-d', 'memory_limit=128M', dirname(__DIR__) . '/Scripts/rereading.php', ...$read],
            [1 => ['pipe', 'w'], 2 => ['pipe', 'w']],
            $pipes,
        );

        self::assertIsResource($process);
        $output = stream_get_contents($pipes[1]);
        $errors = stream_get_contents($pipes[2]);

        self::assertSame(0, proc_close($process), (string) $errors);
        self::assertSame(['output' => $resolved, 'calls' => $calls, 'reports' => $codes], json_decode((string) $output, true));
    }

    private static function cycle(): \stdClass
    {
        $cycle = new \stdClass();
        $cycle->self = $cycle;

        return $cycle;
    }
}
