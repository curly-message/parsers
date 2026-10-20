<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\Origin;
use CurlyMessage\Parser;
use CurlyMessage\Report;
use CurlyMessage\ReportCode;
use CurlyMessage\Undefined;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class ParserTest extends TestCase
{
    use Apart;
    use Resolves;

    public function testSubstitutesAValueAndUnescapesTheMessagesOwnText(): void
    {
        self::assertSame('Hi Ada {{', $this->resolve('Hi {{name}} \{{', ['name' => 'Ada']));
        self::assertSame([], $this->codes());
    }

    /** @return iterable<string, array{mixed, string}> */
    public static function values(): iterable
    {
        yield 'a string' => ['Ada', 'Ada'];
        yield 'an int' => [42, '42'];
        yield 'a float' => [1.5, '1.5'];
        yield 'true' => [true, '1'];
        yield 'false' => [false, ''];
        yield 'null, which is present' => [null, ''];
        yield 'a list' => [[1, 'a/b', "\u{E9}"], "[1,\"a/b\",\"\u{E9}\"]"];
        yield 'a map' => [['a' => 1, 0 => 2], '{"a":1,"0":2}'];
        yield 'an empty array' => [[], '[]'];
        yield 'an empty stdClass' => [new \stdClass(), '{}'];
        yield 'a stdClass' => [(object) ['a' => [true, null]], '{"a":[true,null]}'];
        yield 'an object with __toString' => [new Spelled('spelled'), 'spelled'];
    }

    #[DataProvider('values')]
    public function testConvertsAValueToText(mixed $value, string $text): void
    {
        self::assertSame($text, $this->resolve('{{v; default:D}}', ['v' => $value]));
        self::assertSame([], $this->codes());
    }

    /** @return iterable<string, array{mixed}> */
    public static function undescribed(): iterable
    {
        yield 'an object without __toString' => [new \ArrayObject([1])];
        yield 'a __toString that throws' => [new Spelled(null)];
        yield 'a stdClass holding itself' => [self::cycle()];
        yield 'a pure enum in a list' => [[Undefined::Value, Pure::Case]];
        yield 'invalid UTF-8 in a list' => [["\xC3"]];
        yield 'an infinity in a list' => [[INF]];
    }

    #[DataProvider('undescribed')]
    public function testTakesTheChainAndReportsAValueNothingDescribes(mixed $value): void
    {
        self::assertSame('D', $this->resolve('{{v; default:D}}', ['v' => $value], 'the.id'));
        self::assertSame(['unserializable-value'], $this->codes());
        self::assertSame(Origin::Payload, $this->reports[0]->origin);
        self::assertSame('the.id', $this->reports[0]->id);
    }

    public function testReadsUndefinedAsNobodySupplyingIt(): void
    {
        self::assertSame('D', $this->resolve('{{v; default:D}}', ['v' => Undefined::Value]));
        self::assertSame('P', $this->resolve('{{v}}', ['v' => ['value' => Undefined::Value, 'default' => Undefined::Value], 'default' => 'P']));
        self::assertSame('', $this->resolve(Undefined::Value, ['v' => 'x']));
        self::assertSame('[null]', $this->resolve('{{v}}', ['v' => [Undefined::Value]]));
        self::assertSame('{"b":1}', $this->resolve('{{v}}', ['v' => ['a' => Undefined::Value, 'b' => 1]]));
        self::assertSame([], $this->codes());
    }

    public function testFindsAPresentNullAndAKeyMissingFromEitherContainer(): void
    {
        self::assertSame('', $this->resolve('{{v; default:D}}', ['v' => null]));
        self::assertSame('', $this->resolve('{{v; default:D}}', (object) ['v' => null]));
        self::assertSame('D', $this->resolve('{{w; default:D}}', (object) ['v' => null]));
        self::assertSame('D', $this->resolve('{{w; default:D}}', ['v' => null]));
    }

    /** @return iterable<string, array{string}> */
    public static function prototypeKeys(): iterable
    {
        yield '__proto__' => ['__proto__'];
        yield 'constructor' => ['constructor'];
        yield 'toString' => ['toString'];
    }

    #[DataProvider('prototypeKeys')]
    public function testReadsAPrototypeNameAsAnOrdinaryKey(string $key): void
    {
        self::assertSame('D', $this->resolve("{{{$key}; default:D}}", []));
        self::assertSame('own', $this->resolve("{{{$key}}}", [$key => 'own']));
        self::assertSame('own', $this->resolve("{{{$key}}}", (object) [$key => 'own']));
    }

    public function testFindsAKeyBeginningWithNulOnAStdClass(): void
    {
        self::assertSame('own', $this->resolve("{{\0k}}", (object) ["\0k" => 'own']));
        self::assertSame('D', $this->resolve("{{\0j; default:D}}", (object) ["\0k" => 'own']));
    }

    public function testReadsNoOtherObjectAsAPayload(): void
    {
        self::assertSame('D', $this->resolve('{{v; default:D}}', new \ArrayObject(['v' => 'x'])));
        self::assertSame('D', $this->resolve('{{v; default:D}}', new Spelled('x')));
        self::assertSame([], $this->codes());
    }

    public function testNeverThrowsOnInputsOfTheWrongShape(): void
    {
        $parser = new Parser();

        self::assertSame('', $parser->resolve());
        self::assertSame('123', $parser->resolve(123));
        self::assertSame('["a"]', $parser->resolve(['a']));
        self::assertSame('', $parser->resolve(null));
        self::assertSame('', $parser->resolve(new \ArrayObject()));
        self::assertSame('x', $parser->resolve('{{v; default:x}}', 'payload', 5, [], Undefined::Value));
        self::assertSame('x', $parser->resolve('{{v; default:x}}', 7, (object) [], 1.5, new \stdClass()));
    }

    public function testConvertsAMessageAndAValueItIsOnce(): void
    {
        $spelled = new Spelled('{{x}}');

        self::assertSame('{{x}}', $this->resolve($spelled, ['x' => $spelled]));
        self::assertSame(1, $spelled->calls);
    }

    public function testConvertsAValueOnceHoweverManyPlaceholdersReadIt(): void
    {
        $spelled = new Spelled('s');

        self::assertSame(str_repeat('s', 1000), $this->resolve(str_repeat('{{x}}', 1000), ['x' => $spelled]));
        self::assertSame(1, $spelled->calls);
        // A record is the resolution's: the next one asks again.
        $this->resolve('{{x}}', ['x' => $spelled]);
        self::assertSame(2, $spelled->calls);
    }

    public function testReadsALinkOfTheChainOnlyWhereTheChainIsRead(): void
    {
        $payload = ['v' => 'a', 'default' => self::cycle()];

        self::assertSame('A', $this->resolve('{{v; a:A}}', $payload));
        self::assertSame([], $this->codes());
        self::assertSame('', $this->resolve('{{v; b:B}}', $payload));
        self::assertSame(['unserializable-value'], $this->codes());
    }

    public function testConvertsAnArrayReadAtOnePlaceOnce(): void
    {
        $serialized = new Serialized('s');
        $payload = ['x' => [$serialized], 'y' => [$serialized]];

        self::assertSame('["s"]["s"]["s"]', $this->resolve('{{x}}{{x}}{{y}}', $payload));
        // Arrays have no identity, so each place an array is read at is one.
        self::assertSame(2, $serialized->calls);
    }

    // An array is recorded beside the place it was read at, so one host code
    // put there since, as a value's `__toString` may into a `stdClass`
    // payload, is a new value and converts, as a new object does; one left
    // there converts once, whether anything describes it or not.
    public function testConvertsAnArrayHostCodePutWhereOneWasRead(): void
    {
        $kept = new Serialized('s');
        $refused = new Serialized("\xFF");
        $payload = new \stdClass();
        $payload->list = [$kept];
        $payload->bad = [$refused];
        $payload->obj = (object) ['k' => 'a'];
        $payload->swap = new Converting(static function () use ($payload): void {
            $payload->list = ['b'];
            $payload->bad = ['ok'];
            $payload->obj = (object) ['k' => 'b'];
        }, static fn(): null => null);

        self::assertSame(
            '["s"]["s"]|--|{"k":"a"}|x|["b"]|["ok"]|{"k":"b"}',
            $this->resolve('{{list}}{{list}}|{{bad; default:-}}{{bad; default:-}}|{{obj}}|{{swap}}|{{list}}|{{bad; default:-}}|{{obj}}', $payload),
        );
        self::assertSame(1, $kept->calls);
        self::assertSame(1, $refused->calls);
        self::assertSame(['unserializable-value', 'unserializable-value'], $this->codes());
    }

    /** @return iterable<string, array{float, string, int}> */
    public static function zeroesPut(): iterable
    {
        yield 'a zero of the other sign' => [-0.0, '["s",0]x["s",-0]', 2];
        yield 'a zero of the same sign' => [0.0, '["s",0]x["s",0]', 1];
    }

    // JSON writes a float zero with its sign, which `===` does not tell, so
    // an array host code put where one was read, alike to it but for the
    // sign of a zero, converts again, and one alike to it, sign and all,
    // answers the text it converted to.
    #[DataProvider('zeroesPut')]
    public function testConvertsAnArrayHostCodePutWhereOneWasReadByTheSignOfAZero(float $put, string $output, int $calls): void
    {
        $serialized = new Serialized('s');
        $payload = new \stdClass();
        $payload->zero = [$serialized, 0.0];
        $payload->swap = new Converting(static function () use ($payload, $serialized, $put): void {
            $payload->zero = [$serialized, $put];
        }, static fn(): null => null);

        self::assertSame($output, $this->resolve('{{zero}}{{swap}}{{zero}}', $payload));
        self::assertSame($calls, $serialized->calls);
    }

    /** @return iterable<string, array{float, string}> */
    public static function deepZeroesPut(): iterable
    {
        yield 'a zero of the other sign' => [-0.0, '-0'];
        yield 'a zero of the same sign' => [0.0, '0'];
    }

    // An array nested deeper than `===` is asked of is compared entry by
    // entry, where a float zero is alike only to one of its sign too.
    #[DataProvider('deepZeroesPut')]
    public function testConvertsADeepArrayHostCodePutWhereOneWasReadByTheSignOfAZero(float $put, string $zero): void
    {
        $nested = static function (float $leaf): array {
            $level = [$leaf];

            for ($at = 1; $at < 1_100; $at += 1) {
                $level = [$level];
            }

            return $level;
        };
        $payload = new \stdClass();
        $payload->deep = $nested(0.0);
        $payload->swap = new Converting(static function () use ($payload, $nested, $put): void {
            $payload->deep = $nested($put);
        }, static fn(): null => null);
        $deep = static fn(string $leaf): string => str_repeat('[', 1_100) . $leaf . str_repeat(']', 1_100);

        self::assertSame($deep('0') . 'x' . $deep($zero), $this->resolve('{{deep}}{{swap}}{{deep}}', $payload));
    }

    /** @return iterable<string, array{int}> */
    public static function readsBeforeAZeroIsPut(): iterable
    {
        yield 'past what the re-reads compare' => [120];
        yield 'within it' => [5];
    }

    // An array holding a float zero is compared entry by entry once `===`
    // tells it alike, so one host code put there that `===` tells apart
    // converts however much the re-reads of an array nothing describes
    // compared before it.
    #[DataProvider('readsBeforeAZeroIsPut')]
    public function testConvertsAnArrayHoldingAZeroHostCodePutAfterLongComparisons(int $reads): void
    {
        $payload = new \stdClass();
        $payload->nan = [NAN, ...range(1, 1_000)];
        $payload->zero = [0.0, 'a'];
        $payload->swap = new Converting(static function () use ($payload): void {
            $payload->zero = [0.0, 'b'];
        }, static fn(): null => null);

        self::assertSame('[0,"a"]' . str_repeat('-', $reads) . 'x[0,"b"]', $this->resolve('{{zero}}' . str_repeat('{{nan; default:-}}', $reads) . '{{swap}}{{zero}}', $payload));
    }

    // An array nothing describes may share one child at every level, and a
    // copy built apart is compared with it no further than a conversion
    // visits, where comparing the two whole would walk every path through
    // them.
    public function testComparesASharedArrayPutAgainNoFurtherThanAConversionVisits(): void
    {
        $shared = static function (): array {
            $level = ['x'];

            for ($at = 0; $at < 40; $at += 1) {
                $level = [$level, $level];
            }

            return $level;
        };
        $payload = new \stdClass();
        $payload->dag = $shared();
        $payload->swap = new Converting(static function () use ($payload, $shared): void {
            $payload->dag = $shared();
        }, static fn(): null => null);

        self::assertSame('-x-', $this->resolve('{{dag; default:-}}{{swap}}{{dag; default:-}}', $payload));
        self::assertSame(['unserializable-value', 'unserializable-value'], $this->codes());
    }

    // A float that is not a number is no entry of JSON, and compared with
    // itself as the same array's entry, it is alike: the array converts once.
    public function testConvertsAnArrayHoldingNanOnceHoweverOftenItIsRead(): void
    {
        $serialized = new Serialized('s');

        self::assertSame('---', $this->resolve('{{a; default:-}}{{a; default:-}}{{a; default:-}}', ['a' => [$serialized, NAN]]));
        self::assertSame(1, $serialized->calls);
        self::assertSame(['unserializable-value', 'unserializable-value', 'unserializable-value'], $this->codes());
    }

    /** @return iterable<string, array{string, string, int}> */
    public static function comparedPast(): iterable
    {
        yield 'read once before host code puts another there' => ['{{big; default:-}}{{swap}}{{big; default:-}}', '-x-', 2];
        yield 'read twice before, past what a conversion visits' => ['{{big; default:-}}{{big; default:-}}{{swap}}{{big; default:-}}', '--x-', 1];
    }

    // What a resolution's re-reads compare entry by entry is held, together,
    // to what one conversion visits: past it, a re-read answers its record,
    // so reading an array at every placeholder costs no more than converting
    // it once.
    #[DataProvider('comparedPast')]
    public function testComparesArraysReadAgainNoFurtherInAResolutionThanAConversionVisits(string $message, string $output, int $calls): void
    {
        $serialized = new Serialized('s');
        $payload = new \stdClass();
        $payload->big = [$serialized, ...range(1, 60_000), NAN];
        $payload->swap = new Converting(static function () use ($payload, $serialized): void {
            $payload->big = [$serialized, ...range(1, 60_000), INF];
        }, static fn(): null => null);

        self::assertSame($output, $this->resolve($message, $payload));
        self::assertSame($calls, $serialized->calls);
    }

    /** @return iterable<string, array{string, string}> */
    public static function replacedDeep(): iterable
    {
        yield 'an equal copy' => ['copy', '1'];
        yield 'a copy whose innermost entry differs' => ['changed', '2'];
        yield 'none, the list left in place' => ['kept', '1'];
    }

    // An array nested deeper than PHP's `===` walks on a fiber's stack, put
    // again where one was read, is compared on a stack of the parser's own,
    // and converts again only where it differs. Run apart, since a stack
    // `===` overflows ends the process.
    #[DataProvider('replacedDeep')]
    public function testComparesADeepArrayPutAgainInAFiberOnAStackOfItsOwn(string $shape, string $innermost): void
    {
        $first = str_repeat('[', 20_000) . '1' . str_repeat(']', 20_000);
        $second = str_repeat('[', 20_000) . $innermost . str_repeat(']', 20_000);

        self::assertSame(['output' => "{$first}|{$second}", 'calls' => 1], self::apart('replacing', $shape, '128M'));
    }

    public function testConvertsThePayloadsDefaultOnceWhetherNamedOrFallenBackTo(): void
    {
        $serialized = new Serialized('s');

        self::assertSame('["s"]|["s"]|["s"]', $this->resolve('{{default}}|{{x}}|{{y}}', ['default' => [$serialized]]));
        self::assertSame('["s"]|["s"]', $this->resolve('{{x}}|{{default}}', (object) ['default' => [$serialized]]));
        // The payload's `default` is one place, however a placeholder reaches it.
        self::assertSame(2, $serialized->calls);
    }

    public function testRecognizesAWrapperByItsReservedKeysAlone(): void
    {
        self::assertSame('W', $this->resolve('{{v}}', ['v' => ['value' => 'W', 'default' => 'D']]));
        self::assertSame('W', $this->resolve('{{v}}', ['v' => (object) ['value' => 'W']]));
        self::assertSame('WD', $this->resolve('{{v; default:I}}', ['v' => ['default' => 'WD'], 'default' => 'P']));
        self::assertSame('', $this->resolve('{{v; default:I}}', ['v' => (object) ['value' => null, 'props' => []]]));
        self::assertSame('{"value":"W","unit":"kg"}', $this->resolve('{{v}}', ['v' => ['value' => 'W', 'unit' => 'kg']]));
        self::assertSame('{"value":"W","default":"D","props":[],"x":1}', $this->resolve('{{v}}', ['v' => (object) ['value' => 'W', 'default' => 'D', 'props' => [], 'x' => 1]]));
        self::assertSame('{"value":"W"}', $this->parser(recognizeWrappers: false)->resolve('{{v}}', ['v' => ['value' => 'W']]));
    }

    public function testReadsThePayloadsOwnDefaultAsAValueNeverAsAWrapper(): void
    {
        self::assertSame('{"value":"W"}', $this->resolve('{{default}}', ['default' => ['value' => 'W']]));
        self::assertSame('{"value":"W"}', $this->resolve('{{v}}', ['default' => ['value' => 'W']]));
    }

    public function testTakesTheChainInOrderAndReadsALinkOnlyWhenTheOneBeforeIsEmpty(): void
    {
        $payload = ['v' => ['default' => 'W'], 'default' => new Spelled('P')];

        self::assertSame('W', $this->resolve('{{v; default:I}}', $payload));
        self::assertSame(0, $payload['default']->calls);
        self::assertSame('P', $this->resolve('{{u; default:I}}', $payload));
        self::assertSame('I', $this->resolve('{{u; default:I}}', []));
        self::assertSame('', $this->resolve('{{u}}', []));
    }

    public function testReportsAModifierNobodyRegistered(): void
    {
        self::assertSame('D', $this->resolve('{{v:nope; a:A; default:D}}', ['v' => 'a']));
        self::assertSame(['unknown-modifier'], $this->codes());
        self::assertSame(Origin::Message, $this->reports[0]->origin);
        self::assertSame('{{v:nope; a:A; default:D}}', $this->reports[0]->text);
    }

    public function testReportsASelectionGivenNoOptionsBeforeReadingItsValue(): void
    {
        self::assertSame('', $this->resolve('{{v:eq}}', ['v' => 'a']));
        self::assertSame(['missing-options'], $this->codes());
        self::assertSame('', $this->resolve('{{v:gt}}', []));
        self::assertSame(['missing-options'], $this->codes());
        // A selection naming no key compares nothing, and is no selection.
        self::assertSame('', $this->resolve('{{:eq}}', ['' => 'a']));
        self::assertSame([], $this->codes());
    }

    public function testRendersAnOptionOnlyWhereTheModifierAsksForIt(): void
    {
        self::assertSame('B', $this->resolve('{{v; a:{{x:nope}}; b:B}}', ['v' => 'b']));
        self::assertSame([], $this->codes());
        self::assertSame('', $this->resolve('{{v; a:{{x:nope}}; b:B}}', ['v' => 'a']));
        self::assertSame(['unknown-modifier'], $this->codes());
    }

    public function testCarriesTheReportsDeclarations(): void
    {
        $this->resolve('{{v:nope}}', [], 42);
        $report = $this->reports[0];

        self::assertSame(ReportCode::UnknownModifier, $report->code);
        self::assertSame(ReportCode::UnknownModifier->message(), $report->message);
        self::assertSame(42, $report->id);
        self::assertNull($report->limit);
        $this->resolve('{{v:nope}}', [], Undefined::Value);
        self::assertNull($this->reports[0]->id);
    }

    public function testContainsAReportHandlerThatThrows(): void
    {
        $parser = new Parser(onReport: static function (Report $report): never {
            throw new \RuntimeException('handler');
        });

        self::assertSame('DE', $parser->resolve('{{v:nope; default:D}}{{w:nope; default:E}}'));
        $parser = new Parser(onReport: static function (Report $report): never {
            throw new Rethrowing(1);
        });

        self::assertSame('DE', $parser->resolve('{{v:nope; default:D}}{{w:nope; default:E}}'));
    }

    /** @return iterable<string, array{mixed}> */
    public static function lettingGo(): iterable
    {
        yield 'a __toString raising what raises as it is let go' => [new class implements \Stringable {
            public function __toString(): string
            {
                throw new Rethrowing(1);
            }
        }];
        yield 'a __toString raising what raises again, many times over, as it is let go' => [new class implements \Stringable {
            public function __toString(): string
            {
                throw new Rethrowing(100);
            }
        }];
        yield 'a jsonSerialize raising what raises as it is let go' => [[new class implements \JsonSerializable {
            public function jsonSerialize(): never
            {
                throw new Rethrowing(1);
            }
        }]];
        yield 'a jsonSerialize answering what raises as it is let go' => [[new class implements \JsonSerializable {
            public function jsonSerialize(): Releasing
            {
                return new Releasing(static fn(): never => throw new \RuntimeException('released'));
            }
        }]];
    }

    // Host code runs where a value is asked for its text, and again where
    // what it answered or raised is let go; neither leaves the resolution.
    #[DataProvider('lettingGo')]
    public function testContainsWhatAValueRunsAsWhatItAnsweredIsLetGo(mixed $value): void
    {
        self::assertSame('D', $this->resolve('{{v; default:D}}', ['v' => $value]));
        self::assertSame(['unserializable-value'], $this->codes());
    }

    // A raise holds the arguments of each call it passed through, which are
    // let go with it, where the host keeps them.
    public function testContainsWhatARaisesTraceRunsAsItIsLetGo(): void
    {
        $ignored = ini_set('zend.exception_ignore_args', '0');
        $value = new class implements \Stringable {
            public function __toString(): string
            {
                self::raise(new Releasing(static fn(): never => throw new \RuntimeException('released')));
            }

            private static function raise(Releasing $held): never
            {
                throw new \RuntimeException('raised');
            }
        };

        try {
            self::assertSame('D', $this->resolve('{{v; default:D}}', ['v' => $value]));
            self::assertSame(['unserializable-value'], $this->codes());
        } finally {
            ini_set('zend.exception_ignore_args', (string) $ignored);
        }
    }

    /** @return iterable<string, array{\Closure(): string, string}> */
    public static function heldLast(): iterable
    {
        $released = static fn(): never => throw new \RuntimeException('released');
        $nothing = static fn(): null => null;

        yield 'a value its conversion drops from the payload' => [static function () use ($released): string {
            $payload = new \stdClass();
            $payload->v = new Dropping($payload, 'v', $released);

            return (new Parser())->resolve('[{{v}}] after', $payload);
        }, '[x] after'];
        yield 'a value its conversion drops from its wrapper' => [static function () use ($released): string {
            $wrapper = new \stdClass();
            $wrapper->value = new Dropping($wrapper, 'value', $released);

            return (new Parser())->resolve('[{{v}}] after', ['v' => $wrapper]);
        }, '[x] after'];
        yield 'the payload\'s default its conversion drops from the payload' => [static function () use ($released): string {
            $payload = new \stdClass();
            $payload->default = new Dropping($payload, 'default', $released);

            return (new Parser())->resolve('[{{v}}] after', $payload);
        }, '[x] after'];
        yield 'an entry read among every entry of the payload, which a value drops from it' => [static function () use ($released, $nothing): string {
            $payload = (object) ["\0k" => 'a', 'v' => new Releasing($released)];
            $payload->w = new Dropping($payload, 'v', $nothing);

            return (new Parser())->resolve("[{{\0k}}{{w}}] after", $payload);
        }, '[ax] after'];
        yield 'a value in a reference slot of the payload, which its conversion reassigns' => [static function () use ($released): string {
            $entry = new Converting(static function () use (&$entry): void {
                $entry = null;
            }, $released);

            return (new Parser())->resolve('[{{v}}] after', ['v' => &$entry]);
        }, '[x] after'];
        yield 'a prop a value drops from its layer' => [static function () use ($released, $nothing): string {
            $slice = (object) ['k' => new Releasing($released)];

            return (new Parser())->resolve('[{{v; 1:{{w}}}}] after', ['v' => 1, 'w' => new Dropping($slice, 'k', $nothing)], ['eq' => $slice]);
        }, '[x] after'];
        yield 'the props a wrapper holds, which a value drops from it' => [static function () use ($released, $nothing): string {
            $wrapper = (object) ['value' => 1, 'props' => (object) ['ne' => ['k' => new Releasing($released)]]];

            return (new Parser())->resolve('[{{v; 1:{{w}}}}] after', ['v' => $wrapper, 'w' => new Dropping($wrapper, 'props', $nothing)]);
        }, '[x] after'];
        yield 'an array the conversion recorded, which a value replaces in the payload' => [static function () use ($released, $nothing): string {
            $payload = new \stdClass();
            $payload->v = [new Releasing($released)];
            $payload->w = new Converting(static function () use ($payload): void {
                $payload->v = 'y';
            }, $nothing);

            return (new Parser())->resolve('[{{v}}{{w}}] after', $payload);
        }, '[[{}]x] after'];
        yield 'an array the conversion recorded, which a value replaces in its wrapper' => [static function () use ($released, $nothing): string {
            $wrapper = (object) ['value' => [new Releasing($released)]];
            $converting = new Converting(static function () use ($wrapper): void {
                $wrapper->value = 'y';
            }, $nothing);

            return (new Parser())->resolve('[{{v}}{{w}}] after', ['v' => $wrapper, 'w' => $converting]);
        }, '[[{}]x] after'];
        yield 'a payload the caller holds no reference to' => [static fn(): string => (new Parser())->resolve('[{{v}}] after', ['v' => new Releasing($released)]), '[b] after'];
    }

    // Host code may drop the caller's own reference to what the parser is
    // reading, which leaves the parser holding the last one, and the caller
    // may hand over what nothing else holds: what either runs as the parser
    // lets go of it does not leave the resolution.
    #[DataProvider('heldLast')]
    public function testContainsWhatAnEntryRunsWhereTheParserLetsGoOfItLast(\Closure $resolve, string $resolved): void
    {
        self::assertSame($resolved, $resolve());
    }

    public function testComparesByTheFormatsCaseFolding(): void
    {
        self::assertSame('Y', $this->resolve("{{v; \u{212A}:Y; default:N}}", ['v' => 'k']));
        self::assertSame('Y', $this->resolve("{{v; \u{3C3}\u{3C2}:Y; default:N}}", ['v' => "\u{3A3}\u{3A3}"]));
        self::assertSame('N', $this->resolve("{{v; stra\u{DF}e:Y; default:N}}", ['v' => 'STRASSE']));
    }

    /** @return iterable<string, array{string, mixed, string}> */
    public static function comparisons(): iterable
    {
        yield 'eq, the first equal key' => ['{{v:eq; a:1; A:2; default:D}}', 'a', '1'];
        yield 'eq without a name' => ['{{v; a:1; b:2; default:D}}', 'B', '2'];
        yield 'ne, the first different key' => ['{{v:ne; a:1; b:2; default:D}}', 'a', '2'];
        yield 'ne, none different' => ['{{v:ne; a:1; default:D}}', 'A', 'D'];
        yield 'lt, the least greater key' => ['{{v:lt; 10:ten; 5:five; x:X; default:D}}', 4, 'five'];
        yield 'gt, the greatest lesser key' => ['{{v:gt; 1:one; 3:three; default:D}}', 5, 'three'];
        yield 'gt, an equal key' => ['{{v:gt; 5:five; default:D}}', 5, 'D'];
        yield 'lte, an equal key first' => ['{{v:lte; 9:nine; 5:five; default:D}}', 5, 'five'];
        yield 'gte, an equal key first' => ['{{v:gte; 1:one; 5:five; default:D}}', '5', 'five'];
        yield 'gte, equal by text alone' => ['{{v:gte; 1:one; 5:five; default:D}}', '5.0', 'one'];
        yield 'lt, keys of one number in source order' => ['{{v:lt; 2:first; 2.0:second; default:D}}', 1, 'first'];
        yield 'lt over a value that is no number' => ['{{v:lt; 2:two; default:D}}', '1px', 'D'];
        yield 'lt over a hexadecimal value' => ['{{v:lt; 30:yes; default:D}}', '0x1A', 'D'];
        yield 'lt over padded numbers' => ['{{v:lt; 1e3:yes; default:D}}', ' 12 ', 'yes'];
        yield 'an empty option, selected' => ['{{v; a:; default:D}}', 'a', ''];
        yield 'eq, a value lowering to a third of its bytes' => ['{{v; kkk:Y; default:D}}', "\u{212A}\u{212A}\u{212A}", 'Y'];
        yield 'eq, a value too long to equal any key' => ['{{v; a:1; aa:2; default:D}}', str_repeat('a', 10), 'D'];
        yield 'ne, a value too long to equal any key' => ['{{v:ne; a:1; aa:2; default:D}}', str_repeat('a', 10), '1'];
    }

    #[DataProvider('comparisons')]
    public function testSelectsAnOptionByTheModifiersComparison(string $message, mixed $value, string $output): void
    {
        self::assertSame($output, $this->resolve($message, ['v' => $value]));
        self::assertSame([], $this->codes());
    }

    public function testComposesTheLayersAModifierReadsWithoutThrowing(): void
    {
        $parser = new Parser(modifierDefaults: ['eq' => ['a' => 1]]);

        self::assertSame('A', $parser->resolve('{{v; a:A}}', ['v' => ['value' => 'a', 'props' => ['eq' => 'scalar']]], ['eq' => (object) ['b' => 2]]));
    }

    private static function cycle(): \stdClass
    {
        $cycle = new \stdClass();
        $cycle->self = $cycle;

        return $cycle;
    }
}
