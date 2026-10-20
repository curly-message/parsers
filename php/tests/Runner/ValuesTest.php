<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Runner;

use CurlyMessage\Conformance\BehaviourInput;
use CurlyMessage\Conformance\BehaviourOption;
use CurlyMessage\Conformance\Behaviours;
use CurlyMessage\Conformance\Canonical;
use CurlyMessage\Conformance\Decode;
use CurlyMessage\Conformance\Generators;
use CurlyMessage\Conformance\Utf8;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/** What the runner makes of the set's values: decoded inputs, built cases, compared entries, behaviours. */
final class ValuesTest extends TestCase
{
    public function testATagIsReplacedAtEveryDepth(): void
    {
        $value = json_decode('{"a": {"$curly": "undefined"}, "b": [{"c": {"$curly": "undefined"}}], "d": {}, "e": [], "f": null}', false, 512, JSON_THROW_ON_ERROR);
        $decoded = Decode::value($value, 'ABSENT');

        self::assertEquals((object) ['a' => 'ABSENT', 'b' => [(object) ['c' => 'ABSENT']], 'd' => new \stdClass(), 'e' => [], 'f' => null], $decoded);
        self::assertInstanceOf(\stdClass::class, $decoded);
        self::assertNotSame($value, $decoded);
        self::assertSame('ABSENT', Decode::value(json_decode('{"$curly": "undefined"}'), 'ABSENT'));
    }

    public function testAnUnserializableValueHoldsItself(): void
    {
        $value = Decode::value(json_decode('{"$curly": "unserializable"}'), null);

        self::assertInstanceOf(\stdClass::class, $value);
        self::assertSame($value, $value->self);
        self::assertFalse(json_encode($value));
    }

    /** @return iterable<string, array{int}> */
    public static function counts(): iterable
    {
        foreach ([1, 2, 3, 4, 7, 8, 100, 100001] as $count) {
            yield (string) $count => [$count];
        }
    }

    #[DataProvider('counts')]
    public function testANodesValueVisitsAtLeastItsCountAndLessThanTwice(int $count): void
    {
        $value = Decode::value(json_decode('{"$curly": "nodes", "count": ' . $count . '}'), null);
        $visits = self::visits($value);

        self::assertIsArray($value);
        self::assertGreaterThanOrEqual($count, $visits);
        self::assertLessThan(2 * $count + 1, $visits);
    }

    public function testAnUnknownTagIsRefused(): void
    {
        $this->expectExceptionMessage('Unknown tag "nan".');

        Decode::value(json_decode('{"$curly": "nan"}'), null);
    }

    public function testANodesTagWithoutACountIsRefused(): void
    {
        $this->expectExceptionMessage('A nodes tag needs a numeric count.');

        Decode::value(json_decode('{"$curly": "nodes", "count": "3"}'), null);
    }

    public function testEachConstructionIsBuiltFromTheDeclaredLimits(): void
    {
        $limits = ['output' => 3, 'read' => 2, 'conversion' => 4, 'nesting' => 2];
        $built = [];

        foreach (Generators::NAMES as $name) {
            $prepared = Generators::build($name, $limits, 'ABSENT');
            $built[$name] = [$prepared->input->message, $prepared->input->payload, $prepared->expected->output, $prepared->expected->reports];
            self::assertSame(['limits', 'ABSENT', 'ABSENT', 'ABSENT', []], [$prepared->input->id, $prepared->input->props, $prepared->input->locale, $prepared->input->defaults, $prepared->input->modifiers]);
        }

        $limit = static fn(string $code, string $origin, int $limit): array => ['code' => $code, 'origin' => $origin, 'id' => 'limits', 'limit' => $limit];

        self::assertEquals([
            'output-at-limit' => ['{{v; a:xxx;}}', (object) ['v' => 'a'], 'xxx', []],
            'output-over-limit' => ['A{{v; a:xxxx;}}B', (object) ['v' => 'a'], 'AB', [$limit('output-limit', 'limit', 3)]],
            'output-over-limit-continues' => ['{{v; a:xxxx;}}{{w}}', (object) ['v' => 'a', 'w' => 'tail'], 'tail', [$limit('output-limit', 'limit', 3)]],
            'read-at-limit' => ['{{v:eq; nomatch:X; default:ok;}}', (object) ['v' => 'xx'], 'ok', []],
            'read-over-limit' => ['{{v:eq; nomatch:X; default:ok;}}{{w}}', (object) ['v' => 'xxx', 'w' => 'tail'], 'ok', [$limit('read-limit', 'limit', 2)]],
            'conversion-over-limit' => ['{{v; default:D}}', (object) ['v' => Decode::nodes(5)], 'D', [['code' => 'unserializable-value', 'origin' => 'payload', 'id' => 'limits']]],
            'nesting-at-limit' => ['{{v; a:{{v; a:settled; default:fallback;}};}}', (object) ['v' => 'a'], 'settled', []],
            'nesting-over-limit' => ['{{v; a:{{v; a:{{v; a:settled; default:fallback;}};}};}}', (object) ['v' => 'a'], 'fallback', [$limit('nesting-limit', 'message', 2)]],
        ], $built);
    }

    public function testANestingOfOneLevelIsThePlaceholderAlone(): void
    {
        self::assertSame('{{v; a:settled; default:fallback;}}', Generators::build('nesting-at-limit', ['output' => 1, 'read' => 1, 'conversion' => 1, 'nesting' => 1], null)->input->message);
    }

    /** @return iterable<string, array{mixed, mixed, bool}> */
    public static function entries(): iterable
    {
        yield 'a map in another order' => [(object) ['a' => 1, 'b' => [1, 2]], ['b' => [1, 2], 'a' => 1], true];
        yield 'a map with a key more' => [(object) ['a' => 1], ['a' => 1, 'b' => 2], false];
        yield 'a map with a key fewer' => [(object) ['a' => 1, 'b' => 2], ['a' => 1], false];
        yield 'the empty map as an empty array' => [new \stdClass(), [], true];
        yield 'a map as a list' => [(object) ['0' => 'a'], ['a'], false];
        yield 'a map of numeric names' => [(object) ['1' => 'a'], [1 => 'a'], true];
        yield 'a list in another order' => [[1, 2], [2, 1], false];
        yield 'a list as a map' => [[1, 2], [1 => 2, 0 => 1], false];
        yield 'the empty list as the empty map' => [[], new \stdClass(), false];
        yield 'an integer as a float' => [1, 1.0, true];
        yield 'negative zero as zero' => [0, -0.0, true];
        yield 'a number as text' => [1, '1', false];
        yield 'true as one' => [true, 1, false];
        yield 'null as nothing' => [null, null, true];
        yield 'nested maps' => [(object) ['a' => (object) ['b' => 2.5]], ['a' => (object) ['b' => 2.5]], true];
        yield 'nested maps differing' => [(object) ['a' => (object) ['b' => 2.5]], ['a' => ['b' => 2.4]], false];
    }

    #[DataProvider('entries')]
    public function testValuesAreComparedByTheirEntries(mixed $expected, mixed $actual, bool $equal): void
    {
        self::assertSame($equal, Canonical::equal($expected, $actual));
    }

    /** @return iterable<string, array{string, string}> */
    public static function truncations(): iterable
    {
        yield 'ASCII' => ['abc', 'ab'];
        yield 'two bytes' => ["a\u{e9}", 'a'];
        yield 'three bytes' => ["a\u{20ac}", 'a'];
        yield 'four bytes' => ["a\u{1f600}", 'a'];
        yield 'a lone continuation byte' => ["a\x80", 'a'];
        yield 'a sequence cut short' => ["a\xE2\x82", "a\xE2"];
        yield 'nothing' => ['', ''];
    }

    #[DataProvider('truncations')]
    public function testTheLastCharacterIsDroppedWhole(string $text, string $dropped): void
    {
        self::assertSame($dropped, Utf8::dropLast($text));
    }

    public function testACutEndsWhereACharacterEnds(): void
    {
        self::assertSame("a\u{e9}", Utf8::cut("a\u{e9}\u{e9}", 4));
        self::assertSame("a\u{e9}", Utf8::cut("a\u{e9}\u{1f600}", 6));
        self::assertSame('abc', Utf8::cut('abc', 3));
        self::assertSame("\xFF\xFF", Utf8::cut("\xFF\xFF\xFF", 2));
    }

    /** @return iterable<string, array{string, string|array{answer: string}|null}> */
    public static function behaviours(): iterable
    {
        yield 'upper' => ['upper', "VAL\u{e9}"];
        yield 'echo' => ['echo', "val\u{e9}"];
        yield 'empty' => ['empty', ''];
        yield 'nothing' => ['nothing', null];
        yield 'default' => ['default', 'chain'];
        yield 'options' => ['options', 'b=1,a=2'];
        yield 'props' => ['props', "a=\"\u{e9}/\",b=1.5,c=[1],d={\"x\":null}"];
        yield 'locale' => ['locale', 'cs'];
        yield 'object' => ['object', ['answer' => "val\u{e9}"]];
    }

    /** @param string|array{answer: string}|null $answer */
    #[DataProvider('behaviours')]
    public function testEachBehaviourIsAFunctionOfItsInputs(string $name, string|array|null $answer): void
    {
        $input = new BehaviourInput(
            "val\u{e9}",
            [new BehaviourOption('b', static fn(): string => '1'), new BehaviourOption('a', static fn(): string => '2')],
            ['d' => ['x' => null], 'b' => 1.5, 'a' => "\u{e9}/", 'c' => [1]],
            'cs',
            static fn(): string => 'chain',
        );

        self::assertSame($answer, Behaviours::named($name)($input));
    }

    public function testTheRaiseBehaviourRaisesAndTheLocaleBehaviourNamesNone(): void
    {
        $input = new BehaviourInput('v', [], [], null, static fn(): string => '');

        self::assertSame('none', Behaviours::named('locale')($input));
        $this->expectExceptionMessage('The raise behaviour raised.');
        Behaviours::named('raise')($input);
    }

    public function testAnOptionTheBehaviourPassesOverIsNeverRendered(): void
    {
        $rendered = 0;
        $input = new BehaviourInput('v', [new BehaviourOption('a', static function () use (&$rendered): string {
            $rendered += 1;

            return 'x';
        })], [], null, static fn(): string => '');

        Behaviours::named('echo')($input);
        self::assertSame(0, $rendered);
    }

    // The nodes a serialization visits: every array once each time it is met.
    private static function visits(mixed $value): int
    {
        if (!is_array($value)) {
            return 1;
        }

        $visits = 1;

        foreach ($value as $entry) {
            $visits += self::visits($entry);
        }

        return $visits;
    }
}
