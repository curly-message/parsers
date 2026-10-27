<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

/**
 * The format's character classes and the text rules over them, read off a
 * PHP string as UTF-8. A byte that begins no well-formed sequence is a
 * character of its own, so nothing here refuses text, and every structural
 * character is ASCII, which UTF-8 never places inside a longer sequence.
 *
 * @internal
 */
final class Text
{
    /**
     * The format's `line-term` production (SPEC.md section 6): the four code
     * points a placeholder holds in no position. Every rule that reads the set
     * reads it from here, so an amendment moves one list.
     */
    public const LINE_TERM = ["\n", "\r", "\u{2028}", "\u{2029}"];

    /** The whitespace class SPEC.md section 6 enumerates: the line terminators and twenty-one code points besides. */
    public const WHITESPACE = [
        ...self::LINE_TERM,
        "\t", "\u{000B}", "\u{000C}", ' ', "\u{00A0}", "\u{1680}",
        "\u{2000}", "\u{2001}", "\u{2002}", "\u{2003}", "\u{2004}", "\u{2005}", "\u{2006}", "\u{2007}", "\u{2008}", "\u{2009}", "\u{200A}",
        "\u{202F}", "\u{205F}", "\u{3000}", "\u{FEFF}",
    ];

    // The characters a backslash cancels besides whitespace (SPEC.md section 7).
    private const RESERVED = ':;{}\\';

    /** @var array<string, true>|null */
    private static ?array $lineTerms = null;

    /** @var array<string, true>|null */
    private static ?array $blanks = null;

    /**
     * How many bytes the character at `$at` spans, reading no byte at or past
     * `$to`: a well-formed UTF-8 sequence (Unicode Table 3-7), or one byte.
     */
    public static function charLength(string $text, int $at, int $to): int
    {
        if ($at >= $to) {
            return 0;
        }

        $lead = ord($text[$at]);

        if ($lead < 0xC2 || $lead > 0xF4) {
            return 1;
        }

        $size = $lead < 0xE0 ? 2 : ($lead < 0xF0 ? 3 : 4);

        if ($at + $size > $to) {
            return 1;
        }

        $second = ord($text[$at + 1]);
        [$low, $high] = match ($lead) {
            0xE0 => [0xA0, 0xBF],
            0xED => [0x80, 0x9F],
            0xF0 => [0x90, 0xBF],
            0xF4 => [0x80, 0x8F],
            default => [0x80, 0xBF],
        };

        if ($second < $low || $second > $high) {
            return 1;
        }

        for ($index = 2; $index < $size; $index += 1) {
            $next = ord($text[$at + $index]);

            if ($next < 0x80 || $next > 0xBF) {
                return 1;
            }
        }

        return $size;
    }

    /**
     * The code point the character at `$at` encodes, or -1 for a byte that
     * begins no well-formed sequence.
     */
    public static function codePoint(string $text, int $at): int
    {
        $size = self::charLength($text, $at, strlen($text));
        $lead = ord($text[$at]);

        if ($size === 1) {
            return $lead < 0x80 ? $lead : -1;
        }

        $codePoint = $lead & (0xFF >> ($size + 1));

        for ($index = 1; $index < $size; $index += 1) {
            $codePoint = ($codePoint << 6) | (ord($text[$at + $index]) & 0x3F);
        }

        return $codePoint;
    }

    /**
     * Where the escape sequence opening at `$at` ends. A backslash consumes
     * the character after it (section 7), every byte of it, so a sequence is
     * claimed whole rather than cut inside a character.
     */
    public static function escapeEnd(string $text, int $at, int $to): int
    {
        return $at + 1 + max(1, self::charLength($text, $at + 1, $to));
    }

    /** How many bytes the line terminator at `$at` spans, or 0 where none begins there. */
    public static function lineTermAt(string $text, int $at): int
    {
        self::$lineTerms ??= array_fill_keys(self::LINE_TERM, true);

        return self::memberAt(self::$lineTerms, $text, $at, strlen($text));
    }

    /** How many bytes the blank at `$at` spans, reading nothing at or past `$to`, or 0 where none begins there. */
    public static function blankAt(string $text, int $at, int $to): int
    {
        self::$blanks ??= array_fill_keys(self::WHITESPACE, true);

        return self::memberAt(self::$blanks, $text, $at, $to);
    }

    /** How many bytes the blank that ends at `$end` spans, reading nothing before `$from`, or 0 where none ends there. */
    public static function blankBefore(string $text, int $end, int $from): int
    {
        self::$blanks ??= array_fill_keys(self::WHITESPACE, true);

        // A member is a whole well-formed sequence, and a continuation byte
        // never leads one, so a suffix that matches a member is that member.
        for ($size = 1; $size <= 3 && $end - $size >= $from; $size += 1) {
            if (isset(self::$blanks[substr($text, $end - $size, $size)])) {
                return $size;
            }
        }

        return 0;
    }

    /**
     * Whether text carries nothing outside the whitespace class, which the
     * empty text does not. The host's own class is no substitute: section 6
     * fixes this one.
     */
    public static function isBlank(string $text): bool
    {
        $length = strlen($text);

        for ($at = 0; $at < $length;) {
            $size = self::blankAt($text, $at, $length);

            if ($size === 0) {
                return false;
            }

            $at += $size;
        }

        return true;
    }

    /**
     * Whether a backslash in front of this character cancels a structural
     * meaning rather than denoting itself (SPEC.md section 7).
     */
    public static function cancels(string $character): bool
    {
        return (strlen($character) === 1 && str_contains(self::RESERVED, $character)) || self::isBlank($character);
    }

    /**
     * Text as the format reads it, with every escape sequence it claims
     * resolved. Text holding no backslash is returned as it is.
     */
    public static function unesc(string $text): string
    {
        $at = strpos($text, '\\');

        if ($at === false) {
            return $text;
        }

        $length = strlen($text);
        $output = substr($text, 0, $at);

        while ($at < $length) {
            if ($at + 1 >= $length) {
                return $output . '\\';
            }

            $size = self::charLength($text, $at + 1, $length);
            $character = substr($text, $at + 1, $size);
            $output .= self::cancels($character) ? $character : '\\' . $character;
            $at += 1 + $size;

            $next = strpos($text, '\\', $at);
            $until = $next === false ? $length : $next;
            $output .= substr($text, $at, $until - $at);
            $at = $until;
        }

        return $output;
    }

    /**
     * The span between `$from` and `$to` with its blank padding dropped, empty
     * at `$from` where the span is padding throughout. Whitespace an escape
     * sequence claims is text, not padding. Only the padding is read, from
     * either end, so a span holding a nested placeholder is not read again at
     * each level.
     *
     * @return array{int, int}
     */
    public static function trimmed(string $text, int $from, int $to): array
    {
        $start = $from;

        while ($start < $to && ($size = self::blankAt($text, $start, $to)) > 0) {
            $start += $size;
        }

        if ($start === $to) {
            return [$from, $from];
        }

        $end = $to;

        // A blank is text where a backslash consumes it, which is where an odd
        // run of them precedes it.
        while ($end > $start && ($size = self::blankBefore($text, $end, $start)) > 0) {
            $run = 0;

            while ($end - $size - 1 - $run >= $from && $text[$end - $size - 1 - $run] === '\\') {
                $run += 1;
            }

            if ($run % 2 === 1) {
                break;
            }

            $end -= $size;
        }

        return [$start, $end];
    }

    /**
     * The spans between `$from` and `$to` that `$separator` divides, passing
     * over every occurrence an escape sequence claims and every one a nested
     * placeholder writes (SPEC.md section 6, note 5). There is always one
     * span. A caller with no scanner passes none: a key, a modifier name and
     * an option key hold no placeholder (note 10).
     *
     * @return non-empty-list<array{int, int}>
     */
    public static function separated(string $text, int $from, int $to, string $separator, ?Scanner $scanner = null): array
    {
        $parts = [];
        $start = $from;
        $stops = '\\' . $separator . ($scanner === null ? '' : '{');
        $length = strlen($text);

        for ($at = $from; $at < $to;) {
            $at += strcspn($text, $stops, $at, $to - $at);

            if ($at >= $to) {
                break;
            }

            $character = $text[$at];

            if ($character === '\\') {
                $at = self::escapeEnd($text, $at, $to);

                continue;
            }

            if ($character === '{') {
                if ($scanner !== null && $at + 1 < $length && $text[$at + 1] === '{') {
                    $close = $scanner->end($at);

                    if ($close !== null && $close <= $to) {
                        $at = $close;

                        continue;
                    }
                }

                $at += 1;

                continue;
            }

            $parts[] = [$start, $at];
            $start = $at + 1;
            $at += 1;
        }

        $parts[] = [$start, $to];

        return $parts;
    }

    /**
     * A span with its blank padding dropped, unescaped: a name as it answers,
     * and whether there was any name at all.
     *
     * @return array{bool, string}
     */
    public static function named(string $text, int $from, int $to): array
    {
        [$start, $end] = self::trimmed($text, $from, $to);

        return [$start === $end, self::unesc(substr($text, $start, $end - $start))];
    }

    /** @param array<string, true> $members */
    private static function memberAt(array $members, string $text, int $at, int $to): int
    {
        if ($at >= $to) {
            return 0;
        }

        $lead = ord($text[$at]);

        if ($lead < 0x80) {
            return isset($members[$text[$at]]) ? 1 : 0;
        }

        $size = self::charLength($text, $at, $to);

        return $size > 1 && isset($members[substr($text, $at, $size)]) ? $size : 0;
    }
}
