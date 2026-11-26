<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * Text read as a date (SPEC.md section 11.2), for a value that is no
 * timestamp, by PHP's own date parser narrowed to text that names an absolute
 * date: `date_parse` reads it with no error or warning, with a year, a month
 * and a day, and with nothing relative but a weekday that is the date's own,
 * as RFC 2822 and HTTP dates carry one. So `now`, `tomorrow` and a lone `a`
 * (a military time zone) name none, a weekday that disagrees with its date
 * would move it, and `2024-02-30` and `T24:00`, which PHP warns of, name none
 * either. Text with no offset, a date alone included, is read in PHP's
 * default time zone.
 *
 * @internal
 */
final class DateText
{
    /** The instant the text names, in milliseconds since the epoch, or null where it names none. */
    public static function parse(string $text): ?int
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
}
