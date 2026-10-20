<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * The characters of a PHP string, read as UTF-8. A byte that begins no
 * well-formed sequence is a character of its own, so every string has a
 * reading and none is refused.
 */
final class Utf8
{
    /** The byte length of the character that starts at a byte offset. */
    public static function width(string $text, int $at): int
    {
        $lead = ord($text[$at]);
        $width = match (true) {
            $lead >= 0xC2 && $lead <= 0xDF => 2,
            $lead >= 0xE0 && $lead <= 0xEF => 3,
            $lead >= 0xF0 && $lead <= 0xF4 => 4,
            default => 1,
        };

        return $width > 1 && preg_match('//u', substr($text, $at, $width)) !== 1 ? 1 : $width;
    }

    /** The text without its last character: a well-formed sequence of up to four bytes, or else one byte. */
    public static function dropLast(string $text): string
    {
        $length = strlen($text);

        for ($width = 4; $width > 1; $width -= 1) {
            if ($length >= $width && self::width($text, $length - $width) === $width) {
                return substr($text, 0, $length - $width);
            }
        }

        return substr($text, 0, max(0, $length - 1));
    }

    /** At most `$bytes` bytes of the text, cut where a character ends. */
    public static function cut(string $text, int $bytes): string
    {
        if (strlen($text) <= $bytes) {
            return $text;
        }

        $at = 0;

        while ($at < $bytes) {
            $width = self::width($text, $at);

            if ($at + $width > $bytes) {
                break;
            }

            $at += $width;
        }

        return substr($text, 0, $at);
    }
}
