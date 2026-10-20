<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

use CurlyMessage\Facility as Api;

/**
 * Performs a request of section 11.2 on ext-intl: the locale checked as a
 * tag before anything is built, the facility built once per request whose
 * locale and properties it reads are spelled the same way and kept, and the
 * number formatted or its category selected.
 * A request the host refuses throws, as ECMAScript's `Intl` throws.
 *
 * @internal
 */
final class Facility
{
    private const DIGITS = ['minimumIntegerDigits', 'minimumFractionDigits', 'maximumFractionDigits', 'minimumSignificantDigits', 'maximumSignificantDigits', 'roundingIncrement', 'roundingMode', 'roundingPriority', 'trailingZeroDisplay'];

    // The properties each facility reads, as ECMA-402 reads them; it ignores
    // every other name, and so does the facility, which is handed none.
    private const READS = [
        'NumberFormat' => ['numberingSystem', 'style', 'currency', 'currencyDisplay', 'useGrouping', ...self::DIGITS],
        'DateTimeFormat' => ['calendar', 'numberingSystem', 'hour12', 'hourCycle', 'timeZone', 'weekday', 'era', 'year', 'month', 'day', 'dayPeriod', 'hour', 'minute', 'second', 'fractionalSecondDigits', 'timeZoneName', 'dateStyle', 'timeStyle'],
        'RelativeTimeFormat' => ['numberingSystem', 'style', 'numeric'],
        'PluralRules' => ['type', ...self::DIGITS],
    ];

    /**
     * The text the request formats to, or for `PluralRules` the category it
     * selects.
     *
     * @param array<array-key, mixed> $options
     * @param int|float|array{int|float, string} $input
     */
    public static function perform(Api $api, string $locale, array $options, int|float|array $input): string
    {
        $tag = LocaleTag::of($locale) ?? throw new \ValueError('The locale is not a well-formed tag');
        // Kept by what it reads alone, so names a payload adds keep nothing
        // more and share the facility built without them.
        $options = array_intersect_key($options, array_flip(self::READS[$api->value]));
        $head = Cache::key(['api' => $api->value, 'locale' => $locale]);
        $rest = Cache::key($options);
        $key = $head === null || $rest === null ? null : $head . $rest;

        $built = Cache::formatter($key, static fn(): NumberFormat|DateTimeFormat|RelativeTimeFormat|PluralRules => match ($api) {
            Api::NumberFormat => NumberFormat::build($tag, $options),
            Api::DateTimeFormat => DateTimeFormat::build($tag, $options),
            Api::RelativeTimeFormat => RelativeTimeFormat::build($tag, $options),
            Api::PluralRules => PluralRules::build($tag, $options),
        });

        if (is_array($input)) {
            if (!$built instanceof RelativeTimeFormat) {
                throw new \TypeError('Only a relative time takes a count and a unit');
            }

            return $built->format($input[0], $input[1]);
        }

        return match (true) {
            $built instanceof PluralRules => $built->select($input),
            $built instanceof RelativeTimeFormat => throw new \TypeError('A relative time takes a count and a unit'),
            default => $built->format($input),
        };
    }
}
