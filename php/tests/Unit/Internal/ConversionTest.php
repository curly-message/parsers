<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit\Internal;

use CurlyMessage\Internal\Conversion;
use CurlyMessage\Parser;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/**
 * The length a serialization writes, pinned at lengths a test can hold,
 * shorter than the `Parser::MAX_TEXT_LENGTH` the parser writes to.
 */
final class ConversionTest extends TestCase
{
    private const FLAGS = JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_LINE_TERMINATORS;

    /** @return iterable<string, array{string}> */
    public static function longStrings(): iterable
    {
        $mixed = str_repeat("a/\u{E9}\x01\u{2028}\"\\\u{1F600}", 20_000);

        yield 'ASCII' => [str_repeat('a', 200_000)];
        yield 'escapes and characters of every width' => [$mixed];
        yield 'a four-byte character across a piece' => [str_repeat('a', 65_535) . "\u{1F600}" . str_repeat('b', 70_000)];
        yield 'a two-byte character across a piece' => [str_repeat('a', 65_535) . "\u{E9}" . str_repeat('b', 70_000)];
        yield 'an escape at a piece' => [str_repeat('a', 65_535) . "\x01\"" . str_repeat('b', 70_000)];
    }

    #[DataProvider('longStrings')]
    public function testWritesALongStringAsJsonEncodeDoes(string $string): void
    {
        $conversion = new Conversion(PHP_INT_MAX);

        self::assertSame(json_encode([$string], self::FLAGS | JSON_THROW_ON_ERROR), $conversion->text([$string]));
        self::assertSame(json_encode([$string => 1], self::FLAGS | JSON_THROW_ON_ERROR), $conversion->text([$string => 1]));
    }

    /** @return iterable<string, array{string}> */
    public static function malformed(): iterable
    {
        yield 'a character cut short at a piece' => [str_repeat('a', 65_535) . "\xF0\x9F" . str_repeat('b', 70_000)];
        yield 'a run of continuation bytes across a piece' => [str_repeat('a', 65_530) . str_repeat("\x80", 20) . str_repeat('b', 70_000)];
        yield 'a lone lead byte far into it' => [str_repeat('a', 150_000) . "\xC3"];
    }

    #[DataProvider('malformed')]
    public function testRefusesALongStringThatIsNotUtf8(string $string): void
    {
        $conversion = new Conversion(PHP_INT_MAX);

        self::assertNull($conversion->text([$string]));
        self::assertNull($conversion->text([$string => 1]));
    }

    public function testWritesToTheLengthOfAParsersTextUnlessGivenAnother(): void
    {
        $length = (new \ReflectionMethod(Conversion::class, '__construct'))->getParameters()[0];

        self::assertTrue($length->isDefaultValueAvailable());
        self::assertSame(Parser::MAX_TEXT_LENGTH, $length->getDefaultValue());
    }

    public function testWritesUpToItsLengthExactly(): void
    {
        // `["a...a","b"]` is eight bytes past its long string, `{"a...a":0}`
        // six, and `{"k":["a...a"]}` ten.
        foreach ([10, 200_000] as $size) {
            $long = str_repeat('a', $size);
            self::assertSame("[\"{$long}\",\"b\"]", (new Conversion($size + 8))->text([$long, 'b']));
            self::assertNull((new Conversion($size + 8))->text([$long, 'bc']));
            self::assertSame("{\"{$long}\":0}", (new Conversion($size + 6))->text([$long => 0]));
            self::assertNull((new Conversion($size + 5))->text([$long => 0]));
            self::assertSame("{\"k\":[\"{$long}\"]}", (new Conversion($size + 10))->text((object) ['k' => [$long]]));
            self::assertNull((new Conversion($size + 10))->text((object) ['kk' => [$long]]));
        }
    }

    // A key whose bytes fit and whose escapes do not is refused while it is
    // escaped, before the whole of it is written.
    public function testRefusesAKeyThatEscapesPastItsLength(): void
    {
        $key = str_repeat("\x01", 100_000);

        self::assertNull((new Conversion(200_000))->text([$key => 0]));
        self::assertNull((new Conversion(200_000))->text((object) [$key => 0]));
        self::assertSame(600_006, strlen((string) (new Conversion(600_006))->text([$key => 0])));
        self::assertNull((new Conversion(600_005))->text([$key => 0]));
    }
}
