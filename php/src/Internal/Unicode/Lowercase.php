<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Unicode;

use CurlyMessage\Internal\Text;

/**
 * Unicode's toLowercase (Default Case Conversion, language-insensitive):
 * what ECMAScript's `toLowerCase` is defined as, and what SPEC.md section
 * 11.1 compares by. PHP's own `strtolower` maps ASCII only, and the mapping
 * the optional mbstring extension carries changes with the PHP version, so
 * the mapping is a table of its own (`Table`), generated from a pinned Unicode
 * version.
 * A byte that begins no well-formed sequence is kept as it is.
 *
 * @internal
 */
final class Lowercase
{
    private const SIGMA = "\u{3A3}";

    private static ?string $ascii = null;

    public static function of(string $text): string
    {
        self::$ascii ??= implode('', array_map('chr', range(0, 0x7F)));
        $length = strlen($text);

        if (strspn($text, self::$ascii) === $length) {
            return strtolower($text);
        }

        $out = '';

        for ($at = 0; $at < $length;) {
            $run = strspn($text, self::$ascii, $at);

            if ($run > 0) {
                $out .= strtolower(substr($text, $at, $run));
                $at += $run;

                continue;
            }

            $size = Text::charLength($text, $at, $length);
            $character = substr($text, $at, $size);

            if ($character === self::SIGMA) {
                $out .= self::isFinal($text, $at, $at + $size) ? "\u{3C2}" : "\u{3C3}";
            } else {
                $out .= Table::LOWER[$character] ?? $character;
            }

            $at += $size;
        }

        return $out;
    }

    /**
     * Final_Sigma (Unicode Table 3-17): a cased letter before the sigma and
     * none after it, each looked for past the case-ignorable characters
     * between. A character that is both is passed over, as ICU, which
     * ECMAScript's hosts lower case with, passes it over. A byte that begins
     * no well-formed sequence is neither.
     */
    private static function isFinal(string $text, int $start, int $end): bool
    {
        $before = false;

        for ($at = $start; $at > 0;) {
            $at = self::previous($text, $at);
            $codePoint = Text::codePoint($text, $at);

            if (!self::in($codePoint, Table::IGNORABLE)) {
                $before = self::in($codePoint, Table::CASED);

                break;
            }
        }

        if (!$before) {
            return false;
        }

        $length = strlen($text);

        for ($at = $end; $at < $length; $at += Text::charLength($text, $at, $length)) {
            $codePoint = Text::codePoint($text, $at);

            if (self::in($codePoint, Table::IGNORABLE)) {
                continue;
            }

            return !self::in($codePoint, Table::CASED);
        }

        return true;
    }

    // Where the character ending at `$end` begins: a well-formed sequence of
    // up to four bytes ending there, or the byte before it.
    private static function previous(string $text, int $end): int
    {
        for ($size = 2; $size <= 4 && $end - $size >= 0; $size += 1) {
            if (Text::charLength($text, $end - $size, $end) === $size) {
                return $end - $size;
            }
        }

        return $end - 1;
    }

    /** @param list<array{int, int}> $ranges */
    private static function in(int $codePoint, array $ranges): bool
    {
        $low = 0;
        $high = count($ranges) - 1;

        while ($low <= $high) {
            $middle = intdiv($low + $high, 2);

            if ($codePoint < $ranges[$middle][0]) {
                $high = $middle - 1;
            } elseif ($codePoint > $ranges[$middle][1]) {
                $low = $middle + 1;
            } else {
                return true;
            }
        }

        return false;
    }
}
