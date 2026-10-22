<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit\Internal\Intl;

use CurlyMessage\Internal\Intl\EcmaOptions;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class EcmaOptionsTest extends TestCase
{
    /** @return iterable<string, array{mixed, float}> */
    public static function numbers(): iterable
    {
        // Each is what ECMAScript's `Number()` answers.
        yield 'null' => [null, 0.0];
        yield 'true' => [true, 1.0];
        yield 'blank around digits' => [" 12\u{3000}", 12.0];
        yield 'hexadecimal' => ['0x10', 16.0];
        yield 'binary' => ['0b11', 3.0];
        yield 'octal' => ['0o17', 15.0];
        yield 'the empty text' => ['', 0.0];
        yield 'an exponent' => ['1e3', 1000.0];
        yield 'a point and no fraction' => ['5.', 5.0];
        yield 'infinity' => ['-Infinity', -INF];
        yield 'a list of one' => [[5], 5.0];
        yield 'the empty list' => [[], 0.0];
    }

    #[DataProvider('numbers')]
    public function testConvertsToANumberAsEcmaScriptDoes(mixed $value, float $number): void
    {
        self::assertSame($number, EcmaOptions::toNumber($value));
    }

    /** @return iterable<string, array{mixed}> */
    public static function notNumbers(): iterable
    {
        yield 'a word' => ['abc'];
        yield 'separated digits' => ['1_000'];
        yield 'a list of two' => [[1, 2]];
        yield 'a map' => [new \stdClass()];
    }

    #[DataProvider('notNumbers')]
    public function testConvertsWhatIsNoNumberToNaN(mixed $value): void
    {
        self::assertNan(EcmaOptions::toNumber($value));
    }

    public function testRefusesAnObjectWithNoJsonCounterpart(): void
    {
        $this->expectException(\TypeError::class);

        EcmaOptions::toNumber(new \ArrayObject());
    }

    /** @return iterable<string, array{float, string}> */
    public static function spellings(): iterable
    {
        // Each is what ECMAScript's `String()` answers.
        yield 'a large integer' => [123456789012345680000.0, '123456789012345680000'];
        yield 'past twenty-one digits' => [1e21, '1e+21'];
        yield 'a tenth' => [0.1, '0.1'];
        yield 'negative zero' => [-0.0, '0'];
        yield 'a small fraction' => [1.5e-6, '0.0000015'];
        yield 'a smaller one' => [1e-7, '1e-7'];
        yield 'a third' => [1 / 3, '0.3333333333333333'];
        yield 'the least' => [5e-324, '5e-324'];
        yield 'the greatest' => [1.7976931348623157e308, '1.7976931348623157e+308'];
    }

    #[DataProvider('spellings')]
    public function testSpellsANumberAsEcmaScriptDoes(float $number, string $text): void
    {
        self::assertSame($text, EcmaOptions::numberToString($number));
    }

    public function testConvertsToTextAsEcmaScriptDoes(): void
    {
        self::assertSame('true', EcmaOptions::toString(true));
        self::assertSame('null', EcmaOptions::toString(null));
        self::assertSame('1', EcmaOptions::toString(1.0));
        self::assertSame('1,2', EcmaOptions::toString([1, 2]));
        self::assertSame('[object Object]', EcmaOptions::toString(new \stdClass()));
    }

    /** @return iterable<string, array{list<mixed>, string}> */
    public static function lists(): iterable
    {
        yield 'strings' => [['abc', 'de'], 'abc,de'];
        yield 'a nested list after a string' => [['ab', ['c', null]], 'ab,c,'];
        yield 'one string shared' => [array_fill(0, 3, 'ab'), 'ab,ab,ab'];
    }

    // A list's text is held to its length, separators included, as a
    // serialization's is: one that would pass it is refused before what
    // would carry it there is copied, and one that reaches it is not.
    /** @param list<mixed> $list */
    #[DataProvider('lists')]
    public function testHoldsAListsTextToItsLength(array $list, string $text): void
    {
        self::assertSame($text, EcmaOptions::toString($list, strlen($text)));

        $this->expectException(\ValueError::class);

        EcmaOptions::toString($list, strlen($text) - 1);
    }
}
