<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\Parser;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/**
 * The serializer is held to `json_encode` byte for byte, over every shape it
 * accepts, and to refusing where `json_encode` refuses.
 */
final class ConversionTest extends TestCase
{
    use Resolves;

    private const FLAGS = JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_LINE_TERMINATORS;

    /** @return iterable<string, array{array<array-key, mixed>|\stdClass}> */
    public static function shapes(): iterable
    {
        yield 'a list' => [[1, 2, 3]];
        yield 'a map' => [['a' => 1, 'b' => [2]]];
        yield 'sparse keys' => [[0 => 'a', 2 => 'b']];
        yield 'mixed keys' => [['x' => 1, 0 => 2]];
        yield 'an empty array' => [[]];
        yield 'an empty stdClass' => [new \stdClass()];
        yield 'a nested stdClass' => [(object) ['a' => (object) ['b' => []], 'c' => [new \stdClass()]]];
        yield 'strings' => [['a/b', "\u{E9}", "\u{2028}\u{2029}", "\n\t\"\\", "\x7F\x00"]];
        yield 'ints' => [[0, -1, PHP_INT_MAX, PHP_INT_MIN]];
        yield 'floats' => [[0.1, 1e21, -0.0, 1.0, 0.1 + 0.2, 1e-7]];
        yield 'booleans and null' => [[true, false, null]];
        yield 'a backed enum' => [[Backed::Int, Backed::String]];
        yield 'serializable, answering a scalar' => [[new Serialized(5)]];
        yield 'serializable, answering an array' => [[new Serialized(['a' => [1]])]];
        yield 'serializable, answering serializable' => [[new Serialized(new Serialized('inner'))]];
        yield 'serializable, answering itself' => [[new Itself()]];
        yield 'a date' => [[new \DateTimeImmutable('@0')]];
        yield 'a time zone' => [[new \DateTimeZone('Europe/Prague')]];
        yield 'an interval' => [[new \DateInterval('P1D')]];
        yield 'an array object' => [[new \ArrayObject([1, 2])]];
        yield 'an exception' => [[new \RuntimeException('m')]];
        yield 'a closure' => [[static fn(): int => 1]];
        yield 'properties of every visibility' => [[new Visible()]];
        yield 'one object twice, side by side' => [(static fn(object $o): array => [$o, $o])(new \stdClass())];
        yield 'integer-like property names' => [[(object) ['1' => 'a', '01' => 'b']]];
        // Each answer is a new object, dropped once it is written, so the
        // next answer may be made where it was.
        yield 'serializable, answering new objects in turn' => [['list' => new Fresh(['items' => [new Fresh(['id' => 1]), new Fresh(['id' => 2])]])]];
        yield 'serializable, answering new objects nested' => [['v' => new Fresh(['b' => [new Fresh(['d' => new Fresh(['e' => 1])])]])]];
    }

    /** @param array<array-key, mixed>|\stdClass $value */
    #[DataProvider('shapes')]
    public function testWritesWhatJsonEncodeWrites(array|\stdClass $value): void
    {
        $expected = json_encode($value, self::FLAGS | JSON_THROW_ON_ERROR);

        self::assertSame($expected, $this->resolve('{{v}}', ['v' => $value]));
        self::assertSame([], $this->codes());
    }

    /** @return iterable<string, array{array<array-key, mixed>|\stdClass}> */
    public static function refused(): iterable
    {
        $cycle = new \stdClass();
        $cycle->self = [$cycle];

        yield 'a cycle' => [$cycle];
        yield 'a pure enum' => [[Pure::Case]];
        yield 'invalid UTF-8' => [["\xFF"]];
        yield 'invalid UTF-8 in a key' => [["\xFF" => 1]];
        yield 'an infinity' => [[INF]];
        yield 'not a number' => [[NAN]];
        yield 'serializable, answering a cycle' => [[new Serialized($cycle)]];
    }

    /** @param array<array-key, mixed>|\stdClass $value */
    #[DataProvider('refused')]
    public function testRefusesWhatJsonEncodeRefuses(array|\stdClass $value): void
    {
        self::assertFalse(json_encode($value, self::FLAGS));
        self::assertSame('D', $this->resolve('{{v; default:D}}', ['v' => $value]));
        self::assertSame(['unserializable-value'], $this->codes());
    }

    public function testRefusesAValueWhoseSerializationRaises(): void
    {
        self::assertSame('D', $this->resolve('{{v; default:D}}', ['v' => [new Raising()]]));
        self::assertSame(['unserializable-value'], $this->codes());
    }

    // ECMAScript asks a value's `toJSON` once; `json_encode` asks an answer
    // that is again serializable in turn, so each answer asked is a value
    // the serialization visits.
    public function testCountsEachSerializableAnswerAskedInTurnAsANode(): void
    {
        $chain = new Chain(2 * Parser::MAX_CONVERSION_NODES);

        self::assertSame('D', $this->resolve('{{v; default:D}}', ['v' => [$chain]]));
        self::assertSame(['unserializable-value'], $this->codes());
        // The list and the chain's first link are two nodes, and the link's
        // first answer stands in for it, so each answer after it is one
        // more: the answer that would be past the limit is never asked for.
        self::assertSame(Parser::MAX_CONVERSION_NODES - 1, $chain->calls);
        self::assertSame('["end"]', $this->resolve('{{v}}', ['v' => [new Chain(Parser::MAX_CONVERSION_NODES - 2)]]));
        self::assertSame([], $this->codes());
        self::assertSame('D', $this->resolve('{{v; default:D}}', ['v' => [new Chain(Parser::MAX_CONVERSION_NODES - 1)]]));
        self::assertSame(['unserializable-value'], $this->codes());
    }

    public function testSerializesAnObjectWithEveryUndefinedMemberLeftOut(): void
    {
        self::assertSame('{"b":null}', $this->resolve('{{v}}', ['v' => (object) ['a' => \CurlyMessage\Undefined::Value, 'b' => null]]));
    }

    public function testReadsAHookedPropertyThroughItsHook(): void
    {
        if (PHP_VERSION_ID < 80400) {
            self::markTestSkipped('Property hooks exist from PHP 8.4 on.');
        }

        // Written as text, since earlier versions cannot parse a hook.
        $hooked = eval('return new class { private int $reads = 0; public int $v { get => 42; } public string $b = "b" { get { $this->reads += 1; return strtoupper($this->b); } } protected int $hidden = 1; public function reads(): int { return $this->reads; } };');

        self::assertIsObject($hooked);
        $expected = json_encode([$hooked], self::FLAGS | JSON_THROW_ON_ERROR);

        self::assertSame('[{"v":42,"b":"B"}]', $expected);
        self::assertSame(str_repeat($expected, 3), $this->resolve('{{v}}{{v}}{{v}}', ['v' => [$hooked]]));
        // Once by `json_encode`, then once for the resolution.
        $reads = [$hooked, 'reads'];
        self::assertIsCallable($reads);
        self::assertSame(2, $reads());
    }

    public function testInitializesALazyObjectAsJsonEncodeDoes(): void
    {
        $class = new \ReflectionClass(Visible::class);

        if (!method_exists($class, 'newLazyGhost')) {
            self::markTestSkipped('Lazy objects exist from PHP 8.4 on.');
        }

        $initialized = 0;
        // An arrow function would capture the count by value.
        $ghost = static function () use ($class, &$initialized): mixed {
            return $class->newLazyGhost(static function (Visible $visible) use (&$initialized): void {
                $initialized += 1;
                $visible->public = 'initialized';
            });
        };
        $expected = '[{"public":"initialized","nested":null}]';

        self::assertSame($expected, json_encode([$ghost()], self::FLAGS));
        self::assertSame($expected . $expected, $this->resolve('{{v}}{{v}}', ['v' => [$ghost()]]));
        self::assertSame(2, $initialized);
    }

    public function testRefusesALazyObjectWhoseInitializerRaises(): void
    {
        $class = new \ReflectionClass(Visible::class);

        if (!method_exists($class, 'newLazyGhost')) {
            self::markTestSkipped('Lazy objects exist from PHP 8.4 on.');
        }

        $ghost = $class->newLazyGhost(static function (): never {
            throw new \RuntimeException('initializer');
        });

        self::assertSame('D', $this->resolve('{{v; default:D}}', ['v' => [$ghost]]));
        self::assertSame(['unserializable-value'], $this->codes());
    }

    // A lazy `stdClass` declares nothing to defer, so it is initialized as it
    // is made and its initializer never runs: a payload made one is read as
    // the empty object it is.
    public function testReadsALazyStdClassAsTheEmptyObjectItIs(): void
    {
        $class = new \ReflectionClass(\stdClass::class);

        if (!method_exists($class, 'newLazyGhost')) {
            self::markTestSkipped('Lazy objects exist from PHP 8.4 on.');
        }

        $payload = $class->newLazyGhost(static function (\stdClass $payload): void {
            $payload->v = 'V';
        });

        self::assertSame('D', $this->resolve('{{v; default:D}}', $payload));
        self::assertSame('{}', $this->resolve('{{v}}', ['v' => $payload]));
        self::assertSame([], $this->codes());
    }
}
