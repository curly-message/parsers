<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit\Internal;

use CurlyMessage\Internal\Placeholder;
use CurlyMessage\Internal\Scanner;
use PHPUnit\Framework\TestCase;

final class PlaceholderTest extends TestCase
{
    public function testReadsWhatAPlaceholderDeclares(): void
    {
        $message = '{{ c\\:o\;unt : g\\:t ; 0 : none ; one ; DEFAULT : x ; default : D ; default: E ; : skipped }}';
        $placeholder = self::parse($message);

        self::assertSame('c:o;unt', $placeholder->key);
        self::assertSame('g:t', $placeholder->modifier);
        self::assertSame(
            [['0', 'none'], ['one', 'one'], ['DEFAULT', 'x']],
            array_map(static fn(array $option): array => [$option[0], self::spanned($message, $option[1])], $placeholder->options),
        );
        self::assertNotNull($placeholder->inlineDefault);
        self::assertSame('D', self::spanned($message, $placeholder->inlineDefault));
    }

    public function testNamesNoKeyAndNoModifierWhereTheSelectorIsBlank(): void
    {
        $placeholder = self::parse('{{ :; a:}}');

        self::assertNull($placeholder->key);
        self::assertSame('', $placeholder->modifier);
        self::assertSame('', self::spanned('{{ :; a:}}', $placeholder->options[0][1]));
        self::assertNull($placeholder->inlineDefault);
    }

    public function testReadsAKeyThatIsZeroAsAKey(): void
    {
        self::assertSame('0', self::parse('{{0}}')->key);
    }

    public function testLeavesASemicolonANestedPlaceholderHoldsToIt(): void
    {
        $message = '{{v; a:{{n; b:B}}; default:{{d}}}}';
        $placeholder = self::parse($message);

        self::assertSame('{{n; b:B}}', self::spanned($message, $placeholder->options[0][1]));
        self::assertNotNull($placeholder->inlineDefault);
        self::assertSame('{{d}}', self::spanned($message, $placeholder->inlineDefault));
    }

    private static function parse(string $message): Placeholder
    {
        $scanner = new Scanner($message);
        $close = $scanner->end(0);
        self::assertNotNull($close);

        return Placeholder::parse($message, 0, $close, $scanner);
    }

    /** @param array{int, int} $span */
    private static function spanned(string $message, array $span): string
    {
        return substr($message, $span[0], $span[1] - $span[0]);
    }
}
