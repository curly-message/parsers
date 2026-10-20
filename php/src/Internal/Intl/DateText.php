<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * Text read as a date (SPEC.md section 11.2), for a value that is no
 * timestamp, in two steps, the first that reads it deciding. First
 * ECMAScript's own date time string format (ECMA-262, "Date Time String
 * Format"), read the way `Date.parse` reads it: PHP's parser reads `2024` as
 * a time of day today and `2024-03` in the local zone, where ECMAScript reads
 * a year and a month at UTC. Then PHP's own parser, narrowed to text that
 * names a date: `now`, `tomorrow` or a lone `a` (a military time zone) name
 * none, and a weekday that disagrees with its date would move it.
 *
 * @internal
 */
final class DateText
{
    private const DAY = 86_400_000;

    /** The instant the text names, in milliseconds since the epoch, or null where it names none. */
    public static function parse(string $text): int|float|null
    {
        return self::iso($text) ?? self::named($text);
    }

    /**
     * `YYYY`, `YYYY-MM` or `YYYY-MM-DD` (or a signed six-digit year), then
     * optionally `THH:mm`, `:ss`, `.sss` and `Z` or `+HH:mm`. A date alone is
     * UTC, and a date and a time with no offset are local time. Read byte by
     * byte, with no pattern.
     */
    private static function iso(string $text): int|float|null
    {
        $length = strlen($text);
        $at = 0;
        $sign = 1;

        if ($length > 0 && ($text[0] === '+' || $text[0] === '-')) {
            $sign = $text[0] === '-' ? -1 : 1;
            $year = self::digits($text, 1, 6);

            // Year zero has one spelling, and it is not negative.
            if ($year === null || ($sign === -1 && $year === 0)) {
                return null;
            }

            $at = 7;
        } else {
            $year = self::digits($text, 0, 4);

            if ($year === null) {
                return null;
            }

            $at = 4;
        }

        $year *= $sign;
        $month = 1;
        $day = 1;

        if ($at < $length && $text[$at] === '-') {
            $month = self::digits($text, $at + 1, 2);

            if ($month === null || $month < 1 || $month > 12) {
                return null;
            }

            $at += 3;

            if ($at < $length && $text[$at] === '-') {
                $day = self::digits($text, $at + 1, 2);

                // V8 takes any day to 31 and lets it run into the next month.
                if ($day === null || $day < 1 || $day > 31) {
                    return null;
                }

                $at += 3;
            }
        }

        $days = self::daysFromCivil($year, $month, 1) + $day - 1;

        if ($at === $length) {
            return $days * self::DAY;
        }

        if ($text[$at] !== 'T') {
            return null;
        }

        $hour = self::digits($text, $at + 1, 2);
        $minute = ($text[$at + 3] ?? '') === ':' ? self::digits($text, $at + 4, 2) : null;

        if ($hour === null || $minute === null) {
            return null;
        }

        $at += 6;
        $second = 0;
        $millisecond = 0;

        if ($at < $length && $text[$at] === ':') {
            $second = self::digits($text, $at + 1, 2);

            if ($second === null) {
                return null;
            }

            $at += 3;

            if ($at < $length && $text[$at] === '.') {
                $run = strspn($text, '0123456789', $at + 1);

                if ($run === 0) {
                    return null;
                }

                $millisecond = (int) str_pad(substr($text, $at + 1, min($run, 3)), 3, '0');
                $at += 1 + $run;
            }
        }

        $midnight = $hour === 24 && $minute === 0 && $second === 0 && $millisecond === 0;

        if (($hour > 23 && !$midnight) || $minute > 59 || $second > 59) {
            return null;
        }

        $time = $days * self::DAY + (($hour * 60 + $minute) * 60 + $second) * 1000 + $millisecond;

        if ($at === $length) {
            return self::fromLocal($time);
        }

        if ($text[$at] === 'Z' && $at + 1 === $length) {
            return $time;
        }

        if (($text[$at] === '+' || $text[$at] === '-') && $at + 6 === $length && $text[$at + 3] === ':') {
            $hours = self::digits($text, $at + 1, 2);
            $minutes = self::digits($text, $at + 4, 2);

            if ($hours === null || $minutes === null || $hours > 23 || $minutes > 59) {
                return null;
            }

            $offset = ($hours * 60 + $minutes) * 60_000;

            return $text[$at] === '+' ? $time - $offset : $time + $offset;
        }

        return null;
    }

    /**
     * PHP's parser, where what it read is an absolute date: no error or
     * warning, a year, a month and a day, and nothing relative but a weekday
     * that is the date's own, as RFC 2822 and HTTP dates carry one.
     */
    private static function named(string $text): ?int
    {
        $parsed = date_parse($text);

        if ($parsed['error_count'] > 0 || $parsed['warning_count'] > 0 || !is_int($parsed['year']) || !is_int($parsed['month']) || !is_int($parsed['day'])) {
            return null;
        }

        $relative = $parsed['relative'] ?? null;

        try {
            if (is_array($relative)) {
                foreach ($relative as $unit => $amount) {
                    if ($unit !== 'weekday' && $amount !== 0) {
                        return null;
                    }
                }

                $own = (int) (new \DateTimeImmutable(sprintf('%04d-%02d-%02d', $parsed['year'], $parsed['month'], $parsed['day']), new \DateTimeZone('UTC')))->format('w');

                if (!isset($relative['weekday']) || !is_int($relative['weekday']) || $relative['weekday'] % 7 !== $own) {
                    return null;
                }
            }

            $instant = new \DateTimeImmutable($text);
        } catch (\Throwable) {
            return null;
        }

        return $instant->getTimestamp() * 1000 + intdiv((int) $instant->format('u'), 1000);
    }

    /**
     * A local time in PHP's default zone as an instant, as ECMAScript reads
     * one in the host's: at the offset in force then.
     */
    private static function fromLocal(int|float $local): int|float|null
    {
        try {
            $zone = new \DateTimeZone(date_default_timezone_get());
            $offset = $zone->getOffset(new \DateTimeImmutable('@' . intdiv((int) $local, 1000))) * 1000;
            $settled = $zone->getOffset(new \DateTimeImmutable('@' . intdiv((int) ($local - $offset), 1000))) * 1000;
        } catch (\Throwable) {
            return null;
        }

        return $local - $settled;
    }

    /** Exactly `$count` ASCII digits at `$at`, as a number. */
    private static function digits(string $text, int $at, int $count): ?int
    {
        return $at + $count <= strlen($text) && strspn($text, '0123456789', $at, $count) === $count ? (int) substr($text, $at, $count) : null;
    }

    /** Days from 1970-01-01 to a date of the proleptic Gregorian calendar. */
    private static function daysFromCivil(int $year, int $month, int $day): int
    {
        $year -= $month <= 2 ? 1 : 0;
        $era = intdiv($year >= 0 ? $year : $year - 399, 400);
        $yearOfEra = $year - $era * 400;
        $dayOfYear = intdiv(153 * ($month > 2 ? $month - 3 : $month + 9) + 2, 5) + $day - 1;
        $dayOfEra = $yearOfEra * 365 + intdiv($yearOfEra, 4) - intdiv($yearOfEra, 100) + $dayOfYear;

        return $era * 146097 + $dayOfEra - 719468;
    }
}
