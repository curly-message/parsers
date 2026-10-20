<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

/**
 * Reads one message for the placeholders it derives. It is scoped to that
 * message because it records what it has decided, and a verdict belongs to a
 * message and a position.
 *
 * A backslash consumes the character after it, so a brace an escape claimed
 * is text rather than half of a delimiter, and a placeholder holds no line
 * terminator in any position. A verdict is final (SPEC.md section 6, note
 * 11): whether a placeholder derives at a position is a function of the
 * message and that position alone, so an answer is recorded and never
 * recomputed. That record is what keeps the scan linear in the length of the
 * message: note 7 has the scan resume one character past a brace that opened
 * nothing, so the attempts overlap, and without the record a message that
 * nests deeply costs time growing faster than any polynomial in its length.
 *
 * @internal
 */
final class Scanner
{
    // Which part of a placeholder a frame is inside. Only a value admits a
    // nested placeholder (note 10), so the part decides whether a `{{` opens
    // one or refuses the construct.
    private const KEY = 0;
    private const MODIFIER = 1;
    private const OPTION_KEY = 2;
    private const OPTION_VALUE = 3;

    // Every byte that can mean something inside a placeholder. U+2028 and
    // U+2029 both begin with 0xE2, which is then read whole.
    private const STOPS = "\\{}:;\n\r\xE2";

    /**
     * How many times the scan has stepped, over every call: what the tests
     * hold to the length of the message.
     */
    public int $steps = 0;

    private readonly int $length;

    /** @var array<int, int> a position's close, or -1 where it derives nothing */
    private array $verdict = [];

    public function __construct(private readonly string $text)
    {
        $this->length = strlen($text);
    }

    /** Where a complete placeholder opening at `$open` ends, or null. */
    public function end(int $open): ?int
    {
        if (isset($this->verdict[$open])) {
            $known = $this->verdict[$open];

            return $known < 0 ? null : $known;
        }

        $text = $this->text;
        $length = $this->length;

        // An explicit stack rather than recursion: nesting is not otherwise
        // limited (note 10), and a message nested deep enough must not take
        // the host's call stack down with it. The construct being read is
        // held apart; the stack holds each one it is nested in, waiting.
        /** @var list<array{int, int}> $waiting open, part */
        $waiting = [];
        $index = $open + 2;
        $part = self::KEY;

        while (true) {
            $this->steps += 1;

            if ($index < $length) {
                $index += strcspn($text, self::STOPS, $index);
            }

            if ($index >= $length) {
                return $this->refuse($open, $waiting);
            }

            $character = $text[$index];

            if ($character === '\\') {
                if (Text::lineTermAt($text, $index + 1) > 0) {
                    return $this->refuse($open, $waiting);
                }

                $index = Text::escapeEnd($text, $index, $length);

                continue;
            }

            if ($character === "\n" || $character === "\r") {
                return $this->refuse($open, $waiting);
            }

            if ($character === "\xE2") {
                if (Text::lineTermAt($text, $index) > 0) {
                    return $this->refuse($open, $waiting);
                }

                $index += 1;

                continue;
            }

            if ($character === ';') {
                $index += 1;
                $part = self::OPTION_KEY;

                continue;
            }

            if ($character === ':') {
                // The first colon of a part is its separator and a later one
                // is content (notes 3 and 4), so a part only ever moves on.
                if ($part === self::KEY) {
                    $part = self::MODIFIER;
                } elseif ($part === self::OPTION_KEY) {
                    $part = self::OPTION_VALUE;
                }

                $index += 1;

                continue;
            }

            $pair = $index + 1 < $length && $text[$index + 1] === $character;

            if ($character === '{' && $pair) {
                // A `{{` in a value must open a complete placeholder or the
                // construct around it does not derive at all; anywhere else it
                // opens none, so the construct around it does not derive
                // either (note 10).
                if ($part !== self::OPTION_VALUE) {
                    return $this->refuse($open, $waiting);
                }

                if (isset($this->verdict[$index])) {
                    if ($this->verdict[$index] < 0) {
                        return $this->refuse($open, $waiting);
                    }

                    $index = $this->verdict[$index];

                    continue;
                }

                $waiting[] = [$open, $part];
                $open = $index;
                $index += 2;
                $part = self::KEY;

                continue;
            }

            if ($character === '}' && $pair) {
                $index += 2;
                $this->verdict[$open] = $index;
                $resumed = array_pop($waiting);

                if ($resumed === null) {
                    return $index;
                }

                [$open, $part] = $resumed;

                continue;
            }

            $index += 1;
        }
    }

    /**
     * Where the next placeholder opening at or after `$from` opens and ends,
     * or null. The search stops at `$to`, and both braces belong to the span:
     * a placeholder that derives inside one closes inside it.
     *
     * @return array{int, int}|null
     */
    public function next(int $from, int $to): ?array
    {
        $text = $this->text;

        for ($index = $from; $index < $to;) {
            $this->steps += 1;
            $index += strcspn($text, '\\{', $index, $to - $index);

            if ($index >= $to) {
                return null;
            }

            if ($text[$index] === '\\') {
                $index = Text::escapeEnd($text, $index, $to);

                continue;
            }

            if ($index + 1 < $to && $text[$index + 1] === '{') {
                $close = $this->end($index);

                if ($close !== null) {
                    return [$index, $close];
                }
            }

            // The braces a refused construct opened are text, and the scan
            // resumes at the very next character rather than past the pair
            // (note 7).
            $index += 1;
        }

        return null;
    }

    /**
     * Every construct still open when the scan dies is one whose closing pair
     * never arrived, so each of them derives nothing.
     *
     * @param list<array{int, int}> $waiting
     */
    private function refuse(int $open, array $waiting): null
    {
        $this->verdict[$open] = -1;

        foreach ($waiting as [$nested]) {
            $this->verdict[$nested] = -1;
        }

        return null;
    }
}
