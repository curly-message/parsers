<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * ECMAScript's `Intl.DateTimeFormat`, performed on ICU's IntlDateFormatter:
 * the options read in ECMA-402's order, a style or the components turned
 * into ICU's pattern, the hour cycle written into it as V8 writes it, and the
 * instant formatted on a proleptic Gregorian calendar, as ECMAScript's time
 * values are.
 *
 * @internal
 */
final class DateTimeFormat
{
    private const STYLES = ['full' => \IntlDateFormatter::FULL, 'long' => \IntlDateFormatter::LONG, 'medium' => \IntlDateFormatter::MEDIUM, 'short' => \IntlDateFormatter::SHORT];

    // Each component's values, and the skeleton letters each asks ICU for.
    private const COMPONENTS = [
        'weekday' => ['narrow' => 'EEEEE', 'short' => 'EEE', 'long' => 'EEEE'],
        'era' => ['narrow' => 'GGGGG', 'short' => 'G', 'long' => 'GGGG'],
        'year' => ['2-digit' => 'yy', 'numeric' => 'y'],
        'month' => ['2-digit' => 'MM', 'numeric' => 'M', 'narrow' => 'MMMMM', 'short' => 'MMM', 'long' => 'MMMM'],
        'day' => ['2-digit' => 'dd', 'numeric' => 'd'],
        'dayPeriod' => ['narrow' => 'BBBBB', 'short' => 'B', 'long' => 'BBBB'],
        'hour' => ['2-digit' => 'jj', 'numeric' => 'j'],
        'minute' => ['2-digit' => 'mm', 'numeric' => 'm'],
        'second' => ['2-digit' => 'ss', 'numeric' => 's'],
        'fractionalSecondDigits' => [],
        'timeZoneName' => ['short' => 'z', 'long' => 'zzzz', 'shortOffset' => 'O', 'longOffset' => 'OOOO', 'shortGeneric' => 'v', 'longGeneric' => 'vvvv'],
    ];

    // What an hour cycle writes the hour with.
    private const HOURS = ['h11' => 'K', 'h12' => 'h', 'h23' => 'H', 'h24' => 'k'];

    private const LIMIT = 8.64e15;

    /** @var list<string>|null */
    private static ?array $calendars = null;

    private function __construct(
        private readonly \IntlDateFormatter $formatter,
        private readonly \IntlCalendar $calendar,
    ) {}

    /**
     * @param array<array-key, mixed> $options
     */
    public static function build(LocaleTag $tag, array $options): self
    {
        $calendar = EcmaOptions::string($options, 'calendar', null, null);

        if ($calendar !== null && !EcmaOptions::isType($calendar)) {
            throw new \ValueError('calendar is not a type');
        }

        $numbering = $tag->numbering($options);
        $hour12 = EcmaOptions::boolean($options, 'hour12');
        $hourCycle = EcmaOptions::string($options, 'hourCycle', array_keys(self::HOURS), null);
        $zone = self::zone($options);
        $skeleton = '';
        $components = [];

        foreach (self::COMPONENTS as $name => $letters) {
            if ($name === 'fractionalSecondDigits') {
                $digits = EcmaOptions::number($options, $name, 1, 3, null);
                $components[$name] = $digits;
                $skeleton .= $digits === null ? '' : str_repeat('S', $digits);

                continue;
            }

            $value = EcmaOptions::string($options, $name, array_keys($letters), null);
            $components[$name] = $value;
            $skeleton .= $value === null ? '' : ($letters[$value] ?? '');
        }

        $dateStyle = EcmaOptions::string($options, 'dateStyle', array_keys(self::STYLES), null);
        $timeStyle = EcmaOptions::string($options, 'timeStyle', array_keys(self::STYLES), null);
        $styled = $dateStyle !== null || $timeStyle !== null;

        if ($styled && array_filter($components, static fn(mixed $value): bool => $value !== null) !== []) {
            throw new \TypeError('A style cannot be combined with components');
        }

        $locale = $tag->icu(['calendar' => self::calendar($calendar ?? ($tag->keywords['ca'] ?? null)), 'numbers' => $numbering]);
        $hourCycle = $hour12 === null ? ($hourCycle ?? self::keyword($tag)) : self::cycle($locale, $hour12);

        if ($styled) {
            $pattern = (new \IntlDateFormatter($locale, $dateStyle === null ? \IntlDateFormatter::NONE : self::STYLES[$dateStyle], $timeStyle === null ? \IntlDateFormatter::NONE : self::STYLES[$timeStyle], $zone))->getPattern();
        } else {
            // A request naming no date or time component asks for the date,
            // as ECMAScript's `ToDateTimeOptions` asks for it.
            $dated = array_filter(array_diff_key($components, ['era' => true, 'timeZoneName' => true]), static fn(mixed $value): bool => $value !== null) !== [];
            $skeleton = $dated ? $skeleton : $skeleton . 'yMd';

            if ($hourCycle !== null) {
                $skeleton = str_replace('j', self::HOURS[$hourCycle], $skeleton);
            }

            $pattern = self::generator($locale)->getBestPattern($skeleton);
        }

        if (!is_string($pattern)) {
            throw new \ValueError('ICU has no pattern for the request');
        }

        $instant = \IntlCalendar::createInstance($zone, $locale);

        // ECMAScript's time values are on the proleptic Gregorian calendar.
        if ($instant instanceof \IntlGregorianCalendar) {
            $instant->setGregorianChange(-INF);
        }

        $pattern = $hourCycle === null ? $pattern : self::cycled($pattern, self::HOURS[$hourCycle]);

        return new self(new \IntlDateFormatter($locale, \IntlDateFormatter::NONE, \IntlDateFormatter::NONE, $zone, $instant, $pattern), $instant);
    }

    /** The instant, in milliseconds since the epoch, after ECMAScript's TimeClip. */
    public function format(int|float $milliseconds): string
    {
        if (!is_finite((float) $milliseconds) || abs($milliseconds) > self::LIMIT) {
            throw new \ValueError('The time is outside the range ECMAScript has');
        }

        // TimeClip keeps the integer part, and leaves no negative zero.
        $time = $milliseconds < 0 ? ceil($milliseconds) : floor($milliseconds);
        $this->calendar->setTime($time === 0.0 ? 0.0 : $time);
        $written = $this->formatter->format($this->calendar);

        if (!is_string($written)) {
            throw new \ValueError('The instant cannot be formatted');
        }

        return $written;
    }

    /**
     * The zone a request names, as ICU names it: the process's where it
     * names none, as ECMAScript takes the host's. A name is matched without
     * regard to case, and an offset (`+01:00`) is a zone of its own.
     *
     * @param array<array-key, mixed> $options
     */
    private static function zone(array $options): \IntlTimeZone
    {
        $named = array_key_exists('timeZone', $options) ? EcmaOptions::toString($options['timeZone']) : date_default_timezone_get();
        $id = self::offset($named) ?? match (strtolower($named)) {
            'utc', 'etc/utc', 'gmt', 'etc/gmt' => 'UTC',
            default => IcuData::timeZones()[strtolower($named)] ?? null,
        };

        $zone = $id === null ? null : \IntlTimeZone::createTimeZone($id);

        if (!$zone instanceof \IntlTimeZone || $zone->getID() === 'Etc/Unknown') {
            throw new \ValueError('timeZone is not a time zone');
        }

        return $zone;
    }

    /** `+HH`, `+HHMM` or `+HH:MM` as ICU's custom zone, or null where it is none. */
    private static function offset(string $text): ?string
    {
        $length = strlen($text);

        if ($length < 3 || ($text[0] !== '+' && $text[0] !== '-')) {
            return null;
        }

        $digits = str_replace(':', '', substr($text, 1));

        if (!in_array($length, [3, 5, 6], true) || ($length === 6 && $text[3] !== ':') || ($length === 5 && str_contains($text, ':'))
            || strspn($digits, '0123456789') !== strlen($digits)) {
            return null;
        }

        $hours = (int) substr($digits, 0, 2);
        $minutes = (int) substr($digits . '00', 2, 2);

        return $hours > 23 || $minutes > 59 ? null : sprintf('GMT%s%02d:%02d', $text[0], $hours, $minutes);
    }

    /** A calendar named in ECMAScript's spelling as ICU spells it, null where ICU has none of that name. */
    private static function calendar(?string $named): ?string
    {
        if ($named === null) {
            return null;
        }

        if (self::$calendars === null) {
            self::$calendars = [];

            foreach (\IntlCalendar::getKeywordValuesForLocale('calendar', 'root', false) as $value) {
                if (is_string($value)) {
                    self::$calendars[] = $value;
                }
            }
        }

        $keywords = \Locale::getKeywords(\Locale::canonicalize('und-u-ca-' . strtolower($named)) ?? '');
        $icu = is_array($keywords) ? ($keywords['calendar'] ?? null) : null;

        return is_string($icu) && in_array($icu, self::$calendars, true) ? $icu : null;
    }

    /** The tag's `hc` keyword, where it names an hour cycle. */
    private static function keyword(LocaleTag $tag): ?string
    {
        $named = $tag->keywords['hc'] ?? null;

        return $named !== null && isset(self::HOURS[$named]) ? $named : null;
    }

    /**
     * The hour cycle `hour12` asks for: the locale's own twelve-hour or
     * twenty-four-hour cycle, as its preferred hour is written.
     */
    private static function cycle(string $locale, bool $twelve): string
    {
        $preferred = (string) self::generator($locale)->getBestPattern('j');

        if ($twelve) {
            return str_contains($preferred, 'K') ? 'h11' : 'h12';
        }

        return str_contains($preferred, 'k') ? 'h24' : 'h23';
    }

    /**
     * The pattern with each hour written in the cycle's letter, quoted text
     * aside, as V8 rewrites it.
     */
    private static function cycled(string $pattern, string $letter): string
    {
        $out = '';
        $literal = false;
        $length = strlen($pattern);

        for ($at = 0; $at < $length; $at += 1) {
            $byte = $pattern[$at];

            if ($byte === "'") {
                $literal = !$literal;
            } elseif (!$literal && ($byte === 'H' || $byte === 'h' || $byte === 'K' || $byte === 'k')) {
                $byte = $letter;
            }

            $out .= $byte;
        }

        return $out;
    }

    private static function generator(string $locale): \IntlDatePatternGenerator
    {
        /** @var \IntlDatePatternGenerator */
        return Cache::locale('patterns', $locale, static fn(): \IntlDatePatternGenerator => new \IntlDatePatternGenerator($locale));
    }
}
