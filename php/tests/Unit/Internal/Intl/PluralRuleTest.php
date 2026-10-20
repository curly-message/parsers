<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit\Internal\Intl;

use CurlyMessage\Internal\Intl\PluralRule;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class PluralRuleTest extends TestCase
{
    /** @return iterable<string, array{string, string, string, bool}> */
    public static function rules(): iterable
    {
        yield 'one, an integer' => ['i = 1 and v = 0 @integer 1', '1', '', true];
        yield 'one, shown with a fraction digit' => ['i = 1 and v = 0 @integer 1', '1', '0', false];
        yield 'a remainder' => ['n % 10 = 1 and n % 100 != 11', '21', '', true];
        yield 'a remainder excluded' => ['n % 10 = 1 and n % 100 != 11', '11', '', false];
        yield 'a remainder of a long number' => ['n % 10 = 1 and n % 100 != 11', '123456789012345678901', '', true];
        yield 'fraction digits shown' => ['v != 0', '1', '0', true];
        yield 'none shown' => ['v != 0', '1', '', false];
        yield 'a list' => ['i = 0,1', '1', '5', true];
        yield 'n with trailing zeros' => ['n = 1', '1', '00', true];
        yield 'n with a fraction' => ['n = 1', '1', '5', false];
        yield 'a range holds integers' => ['n = 0..2', '1', '5', false];
        yield 'within holds fractions' => ['n within 0..2', '1', '5', true];
        yield 'either condition' => ['n = 2 or n = 3', '3', '', true];
        yield 'a value past every bound' => ['n = 1', '100000000000000000000', '', false];
        yield 'the empty rule' => ['', '7', '', true];
    }

    #[DataProvider('rules')]
    public function testHoldsForTheDigitsShown(string $rule, string $integer, string $fraction, bool $holds): void
    {
        $parsed = PluralRule::parse($rule);

        self::assertNotNull($parsed);
        self::assertSame($holds, $parsed->holds($integer, $fraction));
    }

    /** @return iterable<string, array{string}> */
    public static function malformed(): iterable
    {
        yield 'an unknown operand' => ['x = 1'];
        yield 'no value' => ['n = '];
        yield 'a modulus of zero' => ['n % 0 = 1'];
        yield 'a dangling conjunction' => ['n = 1 and'];
        yield 'a character outside the syntax' => ['n = 1; v = 0'];
        yield 'a value too long to read' => ['n = 1234567890123456789'];
    }

    #[DataProvider('malformed')]
    public function testRefusesWhatTheSyntaxDoesNotAdmit(string $rule): void
    {
        self::assertNull(PluralRule::parse($rule));
    }
}
