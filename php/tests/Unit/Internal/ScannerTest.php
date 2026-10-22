<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit\Internal;

use CurlyMessage\Internal\Scanner;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class ScannerTest extends TestCase
{
    /** @return iterable<string, array{string, list<array{int, int}>}> */
    public static function derivations(): iterable
    {
        yield 'one placeholder' => ['a {{b}} c', [[2, 7]]];
        yield 'an unclosed construct before a placeholder' => ['{{a{{b}}', [[3, 8]]];
        yield 'a brace before a placeholder' => ['{{{{a}}', [[1, 7]]];
        yield 'a placeholder in a value' => ['{{v; a:{{n}}; default:D}}', [[0, 25]]];
        yield 'an unclosed construct around a placeholder' => ['{{v; a:{{n}}', [[7, 12]]];
        yield 'a placeholder in a key' => ['{{a{{b}}}}', [[3, 8]]];
        yield 'a placeholder in a modifier name' => ['{{a:{{b}}}}', [[4, 9]]];
        yield 'a placeholder in an option key' => ['{{a; {{b}}:c}}', [[5, 10]]];
        yield 'escaped braces' => ['\\{\\{a}} {{b\\}}}', [[8, 15]]];
        yield 'a backslash at the end' => ['{{a}}\\', [[0, 5]]];
        yield 'a line terminator' => ["{{a\u{2028}}} {{b}}", [[9, 14]]];
    }

    /** @param list<array{int, int}> $expected */
    #[DataProvider('derivations')]
    public function testFindsThePlaceholdersAMessageDerives(string $message, array $expected): void
    {
        self::assertSame($expected, self::walk(new Scanner($message), strlen($message)));
    }

    public function testOpensNothingAtTheLastByteOfASpan(): void
    {
        $scanner = new Scanner('{{a}}{{b}}');

        self::assertNull($scanner->next(0, 1));
        self::assertNull($scanner->next(5, 6));
        self::assertSame([5, 10], $scanner->next(1, 10));
    }

    /** @return iterable<string, array{callable(int): string}> */
    public static function pathologicalShapes(): iterable
    {
        yield 'an unclosed option value per segment' => [static fn(int $n): string => str_repeat('{{a; c:', $n)];
        yield 'nested and closed' => [static fn(int $n): string => str_repeat('{{a; c:', $n) . 'x' . str_repeat('}}', $n)];
        yield 'nested and closed but the outermost' => [static fn(int $n): string => str_repeat('{{a; c:', $n) . 'x' . str_repeat('}}', $n - 1)];
        yield 'an opening pair per byte' => [static fn(int $n): string => str_repeat('{', $n)];
        yield 'an unclosed key per pair' => [static fn(int $n): string => str_repeat('{{a', $n)];
        yield 'escapes and braces' => [static fn(int $n): string => str_repeat('{{\\{', $n)];
    }

    /** @param callable(int): string $shape */
    #[DataProvider('pathologicalShapes')]
    public function testStepsLinearlyInTheLengthOfTheMessage(callable $shape): void
    {
        foreach ([1_000, 10_000] as $size) {
            $message = $shape($size);
            $scanner = new Scanner($message);
            self::walk($scanner, strlen($message));

            self::assertLessThanOrEqual(4 * strlen($message) + 8, $scanner->steps, sprintf('%d bytes', strlen($message)));
        }
    }

    /** @return list<array{int, int}> */
    private static function walk(Scanner $scanner, int $length): array
    {
        $found = [];
        $at = 0;

        while (($match = $scanner->next($at, $length)) !== null) {
            $found[] = $match;
            $at = $match[1];
        }

        return $found;
    }
}
