<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit\Internal\Unicode;

use CurlyMessage\Internal\Unicode\Lowercase;
use CurlyMessage\Internal\Unicode\Table;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class LowercaseTest extends TestCase
{
    /** @return iterable<string, array{string, string}> */
    public static function lowered(): iterable
    {
        yield 'ASCII' => ['ABC xyz', 'abc xyz'];
        yield 'Latin-1' => ["\u{C9}T\u{C9}", "\u{E9}t\u{E9}"];
        yield 'the Kelvin sign' => ["\u{212A}", 'k'];
        yield 'the capital sharp s' => ["\u{1E9E}", "\u{DF}"];
        yield 'a sharp s, kept' => ["stra\u{DF}e", "stra\u{DF}e"];
        yield 'a dotted capital I, by its full mapping' => ["\u{130}", "i\u{307}"];
        yield 'a dotless i, kept' => ["\u{131}", "\u{131}"];
        yield 'a sigma alone' => ["\u{3A3}", "\u{3C3}"];
        yield 'a sigma ending a word' => ["\u{391}\u{3A3}", "\u{3B1}\u{3C2}"];
        yield 'a sigma inside a word' => ["\u{391}\u{3A3}\u{391}", "\u{3B1}\u{3C3}\u{3B1}"];
        yield 'a sigma ending a word before a full stop' => ["\u{391}\u{3A3}.", "\u{3B1}\u{3C2}."];
        yield 'a sigma past an ignorable mark' => ["\u{391}\u{301}\u{3A3}", "\u{3B1}\u{301}\u{3C2}"];
        yield 'a sigma before an ignorable mark and a letter' => ["\u{391}\u{3A3}\u{301}\u{391}", "\u{3B1}\u{3C3}\u{301}\u{3B1}"];
        yield 'a sigma after a space' => ["\u{391} \u{3A3}", "\u{3B1} \u{3C3}"];
        yield 'an ill-formed byte, kept' => ["A\xC3B\xFF", "a\xC3b\xFF"];
        yield 'a supplementary letter' => ["\u{10400}", "\u{10428}"];
    }

    #[DataProvider('lowered')]
    public function testLowersByUnicodesDefaultCaseConversion(string $text, string $lower): void
    {
        self::assertSame($lower, Lowercase::of($text));
    }

    // A comparison takes no value as equal to a key it is more than four and
    // a half times as long as, which holds while no character lowers to less
    // than a third of its bytes nor to more than half again as many.
    public function testLowersNoCharacterToLessThanAThirdOrMoreThanHalfAgainOfItsBytes(): void
    {
        $least = 1.0;
        $most = 1.0;

        foreach (Table::LOWER as $character => $lower) {
            $least = min($least, strlen($lower) / strlen($character));
            $most = max($most, strlen($lower) / strlen($character));
        }

        self::assertSame([1 / 3, 1.5], [$least, $most]);
    }
}
