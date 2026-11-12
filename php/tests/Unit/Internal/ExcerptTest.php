<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit\Internal;

use CurlyMessage\Internal\Excerpt;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class ExcerptTest extends TestCase
{
    /** @return iterable<string, array{string, string}> */
    public static function excerpts(): iterable
    {
        $bound = Excerpt::MAX_LENGTH;

        yield 'text within the bound, as it is' => ['{{a/b}}', '{{a/b}}'];
        yield 'text of the bound exactly' => [str_repeat('a', $bound), str_repeat('a', $bound)];
        yield 'text past it, cut' => [str_repeat('a', $bound + 1), str_repeat('a', $bound) . '...'];
        yield 'a character the bound would split, cut before' => [str_repeat('a', $bound - 1) . "\u{E9}", str_repeat('a', $bound - 1) . '...'];
        yield 'a four-byte character the bound would split' => [str_repeat('a', $bound - 2) . "\u{1F600}", str_repeat('a', $bound - 2) . '...'];
        yield 'a character ending at the bound, kept' => [str_repeat('a', $bound - 2) . "\u{E9}b", str_repeat('a', $bound - 2) . "\u{E9}..."];
        yield 'a run of continuation bytes, cut at the bound' => ['{{x:nope' . str_repeat("\x80", 300), '{{x:nope' . str_repeat("\u{FFFD}", $bound - 8) . '...'];
        yield 'each line terminator, escaped' => ["a\nb\rc\u{2028}d\u{2029}e", 'a\nb\rc\\u2028d\\u2029e'];
        yield 'quotes and backslashes, escaped' => ['"\\', '\"\\\\'];
    }

    #[DataProvider('excerpts')]
    public function testCutsAndEscapesTheSpelling(string $text, string $excerpt): void
    {
        self::assertSame($excerpt, Excerpt::of($text, 0, strlen($text)));
    }

    public function testExcerptsASpanOfALongerText(): void
    {
        self::assertSame('{{b}}', Excerpt::of('{{a}}{{b}}{{c}}', 5, 10));
    }
}
