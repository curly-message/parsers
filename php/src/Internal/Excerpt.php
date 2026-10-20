<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

/**
 * The text a report carries: message text, bounded and escaped, so that a
 * consumer writing a report somewhere cannot have a message forge a line
 * there.
 *
 * @internal
 */
final class Excerpt
{
    public const MAX_LENGTH = 120;

    /** @var array<string, string>|null each line terminator and its escape */
    private static ?array $escapes = null;

    /**
     * The excerpt of `$text` from `$from` to `$to`. Only what the excerpt can
     * hold is copied, so a placeholder spanning most of a long message costs
     * what a short one does.
     */
    public static function of(string $text, int $from, int $to): string
    {
        // The cut reads at most the three bytes past the bound.
        $text = substr($text, $from, min($to - $from, self::MAX_LENGTH + 3));

        if (strlen($text) > self::MAX_LENGTH) {
            $text = substr($text, 0, self::cut($text)) . '...';
        }

        $json = json_encode($text, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_INVALID_UTF8_SUBSTITUTE);
        $escaped = is_string($json) ? substr($json, 1, -1) : '';

        // The serialization escapes every terminator it has a short escape
        // or a flag for, so this pass is the rule rather than an accident of
        // those: every terminator the format holds leaves escaped.
        if (self::$escapes === null) {
            self::$escapes = [];

            foreach (Text::LINE_TERM as $terminator) {
                self::$escapes[$terminator] = self::escape($terminator);
            }
        }

        return strtr($escaped, self::$escapes);
    }

    /**
     * Where the cut falls: before a character the bound would split, found
     * by stepping back at most three bytes, and only over a well-formed
     * sequence. A byte that begins none is a character of its own, so a run
     * of them is cut at the bound rather than walked back.
     */
    private static function cut(string $text): int
    {
        for ($back = 1; $back <= 3; $back += 1) {
            $at = self::MAX_LENGTH - $back;
            $byte = ord($text[$at]);

            if ($byte >= 0x80 && $byte <= 0xBF) {
                continue;
            }

            return Text::charLength($text, $at, strlen($text)) > $back ? $at : self::MAX_LENGTH;
        }

        return self::MAX_LENGTH;
    }

    private static function escape(string $character): string
    {
        return sprintf('\\u%04x', Text::codePoint($character, 0));
    }
}
