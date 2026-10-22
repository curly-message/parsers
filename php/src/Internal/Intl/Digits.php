<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * A number's digits as `number` would show them, written in ASCII with no
 * separators: what the plural rules are read over (SPEC.md section 11.5), and
 * what decides the roundings that weigh one result against another. ICU
 * itself rounds them, at a locale whose digits are ASCII, so they are the
 * digits every locale shows in its own script.
 *
 * @internal
 */
final class Digits
{
    /** @var array<int, \NumberFormatter> */
    private static array $formatters = [];

    /**
     * The shown digits, signed, such as `-1.50`. A percentage is shown as
     * ICU scales it, in decimal, which multiplying the number first would
     * not reproduce: 0.12345 times 100 is 12.344999999999999.
     */
    public static function shown(int|float $number, Rounding $rounding, bool $percent = false): string
    {
        $style = $percent ? \NumberFormatter::PERCENT : \NumberFormatter::DECIMAL;

        if (!isset(self::$formatters[$style])) {
            $formatter = new \NumberFormatter('en@numbers=latn', $style);
            $formatter->setAttribute(\NumberFormatter::GROUPING_USED, 0);
            self::$formatters[$style] = $formatter;
        }

        $formatter = self::$formatters[$style];
        $rounding->apply($formatter);
        $written = $formatter->format($number);

        if (!is_string($written)) {
            throw new \ValueError('The number cannot be shown');
        }

        $kept = '';
        $length = strlen($written);

        for ($at = 0; $at < $length; $at += 1) {
            $byte = $written[$at];

            if (($byte >= '0' && $byte <= '9') || $byte === '.' || $byte === '-') {
                $kept .= $byte;
            }
        }

        return $kept;
    }

    /**
     * The power of ten of the shown number's leading digit, 0 for zero: what
     * ECMA-402 measures a rounding's magnitude from.
     */
    public static function exponent(string $shown): int
    {
        [$integer, $fraction] = self::parts($shown);

        if ($integer !== '0') {
            return strlen($integer) - 1;
        }

        $zeros = strspn($fraction, '0');

        return $zeros === strlen($fraction) ? 0 : -($zeros + 1);
    }

    /** Whether the shown number has no fraction but zeros. */
    public static function isInteger(string $shown): bool
    {
        return trim(self::parts($shown)[1], '0') === '';
    }

    /**
     * The shown number's integer digits, with no leading zero but for zero
     * itself, and its fraction digits as shown, trailing zeros included: the
     * operands the plural rules are written over, unsigned.
     *
     * @return array{string, string}
     */
    public static function parts(string $shown): array
    {
        $unsigned = ltrim($shown, '-');
        $point = strpos($unsigned, '.');
        $integer = $point === false ? $unsigned : substr($unsigned, 0, $point);
        $fraction = $point === false ? '' : substr($unsigned, $point + 1);
        $integer = ltrim($integer, '0');

        return [$integer === '' ? '0' : $integer, $fraction];
    }
}
