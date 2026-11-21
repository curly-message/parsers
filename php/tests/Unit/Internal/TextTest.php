<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit\Internal;

use CurlyMessage\Internal\Text;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class TextTest extends TestCase
{
    public function testEnumeratesTheTwentyFiveMembersOfTheWhitespaceClass(): void
    {
        self::assertCount(25, array_unique(Text::WHITESPACE));

        foreach (Text::LINE_TERM as $terminator) {
            self::assertContains($terminator, Text::WHITESPACE);
        }
    }

    /** @return iterable<string, array{string, bool}> */
    public static function blanks(): iterable
    {
        yield 'empty' => ['', true];
        yield 'every member' => [implode('', Text::WHITESPACE), true];
        yield 'a letter' => [' a ', false];
        yield 'a next line' => ["\u{0085}", false];
        yield 'a Mongolian vowel separator' => ["\u{180E}", false];
        yield 'a zero width space' => ["\u{200B}", false];
        yield 'a lead byte alone' => ["\xE2\x80", false];
        yield 'a continuation byte alone' => ["\xA0", false];
    }

    #[DataProvider('blanks')]
    public function testReadsBlankTextByTheFormatsClass(string $text, bool $blank): void
    {
        self::assertSame($blank, Text::isBlank($text));
    }

    /** @return iterable<string, array{string, string}> */
    public static function escaped(): iterable
    {
        yield 'no backslash' => ['plain', 'plain'];
        yield 'cancelled delimiters' => ['\\{\\{a\\}\\}', '{{a}}'];
        yield 'a cancelled colon and semicolon' => ['a\\:b\;c', 'a:b;c'];
        yield 'a cancelled backslash' => ['a\\\\b', 'a\\b'];
        yield 'a backslash that denotes itself' => ['C:\\path\\n', 'C:\\path\\n'];
        yield 'cancelled whitespace' => ["\\ \\\u{3000}", " \u{3000}"];
        yield 'a backslash before an astral character' => ["\\\u{1F600}", "\\\u{1F600}"];
        yield 'a trailing backslash' => ['a\\', 'a\\'];
        yield 'a backslash before an ill-formed byte' => ["\\\x80\x80", "\\\x80\x80"];
    }

    #[DataProvider('escaped')]
    public function testResolvesTheEscapeSequencesTextHolds(string $text, string $expected): void
    {
        self::assertSame($expected, Text::unesc($text));
    }

    /** @return iterable<string, array{string, array{int, int}}> */
    public static function padded(): iterable
    {
        yield 'no padding' => ['abc', [0, 3]];
        yield 'ASCII padding' => [" \t abc \n", [3, 6]];
        yield 'padding outside ASCII' => ["\u{3000}abc\u{00A0}\u{FEFF}", [3, 6]];
        yield 'all padding' => ["  \u{2028}", [0, 0]];
        yield 'an escaped trailing blank' => ['abc\\  ', [0, 5]];
        yield 'an escaped trailing blank outside ASCII' => ["abc\\\u{3000}\u{3000}", [0, 7]];
        yield 'an escaped backslash before a blank' => ['abc\\\\ ', [0, 5]];
        yield 'a near miss of a blank' => ["\u{2027}a\u{202A}", [0, 7]];
    }

    /** @param array{int, int} $expected */
    #[DataProvider('padded')]
    public function testDropsBlankPaddingButNotWhatAnEscapeClaims(string $text, array $expected): void
    {
        self::assertSame($expected, Text::trimmed($text, 0, strlen($text)));
    }

    /** @return iterable<string, array{string, string}> */
    public static function values(): iterable
    {
        yield 'no padding' => ['1', '1'];
        yield 'padding of every member' => [implode('', Text::WHITESPACE) . '1' . implode('', array_reverse(Text::WHITESPACE)), '1'];
        yield 'blanks inside kept' => ["\u{A0} 1\u{3000}2 \u{A0}", "1\u{3000}2"];
        yield 'all padding' => ["\u{A0} \u{FEFF}", ''];
        yield 'the empty text' => ['', ''];
        yield 'a NUL byte' => ["\0 1 \0", "\0 1 \0"];
        yield 'a near miss of a blank' => ["\u{85}\u{200B}1\u{180E}\u{2027}", "\u{85}\u{200B}1\u{180E}\u{2027}"];
        yield 'a blank cut short' => ["\xC2 1 \xE3\x80", "\xC2 1 \xE3\x80"];
        yield 'a stray lead byte before a blank' => ["\xE2\u{2000}1\xE2\u{2000}", "\xE2\u{2000}1\xE2"];
    }

    /**
     * A blank, or a blank cut short, split across each edge at which the
     * padding is read a slice at a time from either end: slices of 64 bytes,
     * twice the last up to 64 KiB, then two more of 64 KiB.
     *
     * @return iterable<string, array{string, string}>
     */
    public static function sliceEdges(): iterable
    {
        for ($edge = 0, $size = 64; $edge < 3 * 65536; $size = min($size * 2, 65536)) {
            $edge += $size;

            foreach (["\u{A0}" => 1, "\u{3000}" => 2, "\u{2028}" => 1] as $blank => $before) {
                $front = str_repeat(' ', $edge - $before);
                $name = sprintf('%s at %d bytes', bin2hex($blank), $edge);

                yield "$name, at the front" => [$front . $blank . '1 ', '1'];
                yield "$name, at the back" => [' 1' . $blank . strrev($front), '1'];
                yield "$name cut short, at the front" => [$front . substr($blank, 0, -1) . '1', substr($blank, 0, -1) . '1'];
                yield "$name cut short, at the back" => ['1' . substr($blank, 1) . strrev($front), '1' . substr($blank, 1)];
            }
        }
    }

    #[DataProvider('values')]
    #[DataProvider('sliceEdges')]
    public function testDropsTheWhitespaceClassFromAValue(string $text, string $expected): void
    {
        self::assertSame($expected, Text::unpadded($text));
    }

    public function testSeparatesPassingOverEscapesAndNestedPlaceholders(): void
    {
        $text = 'a;b\;c;{{x;y}};d';
        $scanner = new \CurlyMessage\Internal\Scanner($text);

        self::assertSame([[0, 1], [2, 6], [7, 14], [15, 16]], Text::separated($text, 0, strlen($text), ';', $scanner));
        self::assertSame([[0, 1], [2, 6], [7, 10], [11, 14], [15, 16]], Text::separated($text, 0, strlen($text), ';'));
    }

    /** @return iterable<string, array{string, int}> */
    public static function characters(): iterable
    {
        yield 'ASCII' => ['a', 1];
        yield 'two bytes' => ["\u{00E9}", 2];
        yield 'three bytes' => ["\u{20AC}", 3];
        yield 'four bytes' => ["\u{10FFFF}", 4];
        yield 'a continuation byte' => ["\x80", 1];
        yield 'an overlong encoding' => ["\xE0\x80\x80", 1];
        yield 'an encoded surrogate' => ["\xED\xBF\xBF", 1];
        yield 'past the last code point' => ["\xF4\x90\x80\x80", 1];
        yield 'a truncated sequence' => ["\xF0\x9F\x98", 1];
    }

    #[DataProvider('characters')]
    public function testReadsOneCharacterAsAWellFormedSequenceOrOneByte(string $text, int $length): void
    {
        self::assertSame($length, Text::charLength($text, 0, strlen($text)));
    }
}
