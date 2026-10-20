<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit\Internal;

use CurlyMessage\Internal\Comparisons;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class ComparisonsTest extends TestCase
{
    /** @return iterable<string, array{string, int|float|null}> */
    public static function numbers(): iterable
    {
        yield 'an integer' => ['12', 12];
        yield 'padded' => [' 12 ', 12];
        yield 'signed' => ['+12', 12];
        yield 'an exponent' => ['1e3', 1000.0];
        yield 'a leading point' => ['.5', 0.5];
        yield 'a trailing point' => ['5.', 5.0];
        yield 'a trailing line feed' => ["12\n", 12];
        yield 'past every float' => ['1e400', INF];
        yield 'a unit' => ['12px', null];
        yield 'hexadecimal' => ['0x1A', null];
        yield 'a separator' => ['1_000', null];
        yield 'the word Infinity' => ['Infinity', null];
        yield 'empty' => ['', null];
        yield 'blank' => [' ', null];
        yield 'a no-break space before' => ["\u{A0}12", null];
    }

    #[DataProvider('numbers')]
    public function testReadsANumberByPhpsNumericStrings(string $text, int|float|null $number): void
    {
        self::assertSame($number, Comparisons::number($text));
    }
}
