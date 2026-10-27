<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

use CurlyMessage\Facility as Api;
use CurlyMessage\FormatRequest;
use CurlyMessage\Internal\Comparisons;
use CurlyMessage\Internal\Text;
use CurlyMessage\ModifierInput;
use CurlyMessage\ReportCode;

/**
 * The modifiers of the Intl level (SPEC.md sections 11.2 and 11.5). Each
 * reads the locale first, then the value, and then makes one request of its
 * facility in ECMAScript's vocabulary, composed as the format's reference
 * implementation composes it. A modifier that cannot answer says why with the
 * code the resolution reports.
 *
 * @internal
 */
final class Modifiers
{
    public const NAMES = ['number', 'date', 'ago', 'currency', 'plural', 'ordinal'];

    /**
     * The properties that decide which digits a number is shown with, which
     * `plural` takes from what the layers hold under `number`, so a category
     * is asked of the number as `number` shows it.
     */
    public const SHOWN_DIGITS = [
        'minimumIntegerDigits', 'minimumFractionDigits', 'maximumFractionDigits', 'minimumSignificantDigits',
        'maximumSignificantDigits', 'roundingIncrement', 'roundingMode', 'roundingPriority', 'trailingZeroDisplay',
    ];

    // The rungs `ago` climbs, each with how many of the rung below it make one.
    private const LADDER = ['second' => 1000, 'minute' => 60, 'hour' => 60, 'day' => 24, 'week' => 7, 'month' => 13 / 3, 'year' => 12];

    /**
     * The modifier's text, or the code of the condition that kept it from
     * answering.
     *
     * @param \Closure(FormatRequest): void $observe told of every request made, once its facility answered or refused
     */
    public static function call(string $name, ModifierInput $input, \Closure $observe): string|ReportCode
    {
        $locale = $input->locale;

        if ($locale === null || $locale === '') {
            return ReportCode::MissingLocale;
        }

        try {
            return match ($name) {
                'number' => self::number($input, $locale, $observe),
                'date' => self::date($input, $locale, $observe),
                'ago' => self::ago($input, $locale, $observe),
                'currency' => self::currency($input, $locale, $observe),
                'plural' => self::byCategory($input, $locale, $observe, 'cardinal'),
                default => self::byCategory($input, $locale, $observe, 'ordinal'),
            };
        } catch (\Throwable) {
            return ReportCode::FailedModifier;
        }
    }

    /**
     * A value as a number to format: its text read as 3.4 reads it, with
     * blank text and a number past the finite refused, as section 11.2
     * refuses them.
     */
    private static function formattable(string $text): int|float
    {
        $number = Text::isBlank($text) ? null : Comparisons::number($text);

        if ($number === null || !is_finite((float) $number)) {
            throw new \ValueError('The value is not a number to format');
        }

        return $number;
    }

    /** @param \Closure(FormatRequest): void $observe */
    private static function number(ModifierInput $input, string $locale, \Closure $observe): string
    {
        $number = self::formattable($input->value);

        return self::ask(Api::NumberFormat, $locale, self::shownDigits($input->props), $number, $observe);
    }

    /** @param \Closure(FormatRequest): void $observe */
    private static function date(ModifierInput $input, string $locale, \Closure $observe): string
    {
        $text = $input->value;
        $number = Text::isBlank($text) ? null : Comparisons::number($text);
        $instant = $number !== null && is_finite((float) $number) ? $number : DateText::parse($text);

        if ($instant === null) {
            throw new \ValueError('The value is not a date');
        }

        return self::ask(Api::DateTimeFormat, $locale, $input->props, $instant, $observe);
    }

    /** @param \Closure(FormatRequest): void $observe */
    private static function currency(ModifierInput $input, string $locale, \Closure $observe): string
    {
        $amount = self::formattable($input->value);
        $props = $input->props;
        $ratio = array_key_exists('ratio', $props) ? self::ratio($props['ratio']) : 1;
        $product = $amount * $ratio;

        if (!is_finite((float) $product)) {
            throw new \ValueError('The amount at its ratio is not a number to format');
        }

        unset($props['ratio']);

        // The style is what the modifier is rather than a property it
        // layers, so it is pinned over every layer.
        return self::ask(Api::NumberFormat, $locale, ['style' => 'currency'] + $props, $product, $observe);
    }

    /**
     * A ratio as PHP's arithmetic reads an operand: a number as it is, a
     * boolean and null as numbers, numeric text as its number, and nothing
     * else.
     */
    private static function ratio(mixed $ratio): int|float
    {
        return match (true) {
            is_int($ratio), is_float($ratio) => $ratio,
            is_bool($ratio) => (int) $ratio,
            $ratio === null => 0,
            is_string($ratio) && is_numeric($ratio) => +$ratio,
            default => throw new \TypeError('The ratio is not a number'),
        };
    }

    /** @param \Closure(FormatRequest): void $observe */
    private static function ago(ModifierInput $input, string $locale, \Closure $observe): string
    {
        $milliseconds = self::formattable($input->value);
        $props = $input->props;
        $numeric = array_key_exists('numeric', $props) ? $props['numeric'] : 'auto';
        $format = array_key_exists('format', $props) ? EcmaOptions::toString($props['format']) : 'auto';

        // The ladder is the whole of what `ago` counts in, so a format naming
        // anything else is one the modifier cannot process.
        if ($format !== 'auto' && self::rung($format) === null) {
            throw new \ValueError('The format names no rung of the ladder');
        }

        unset($props['format']);
        $props['numeric'] = $numeric;

        return self::ask(Api::RelativeTimeFormat, $locale, $props, self::climbed((float) $milliseconds, $format), $observe);
    }

    /**
     * The count and unit a span of milliseconds is stated in: each rung
     * divides what the one below it counted, rounded, and the climb stops at
     * the format's rung, or under `auto` at the last rung that still counts
     * one or more.
     *
     * @return array{int|float, string}
     */
    private static function climbed(float $value, string $format): array
    {
        $unit = '';
        $previous = -1;
        $index = 0;

        foreach (self::LADDER as $key => $multiplier) {
            if ($unit !== '' && self::rung($format) === $unit) {
                break;
            }

            if ($unit === '' || $index === $previous + 1) {
                $step = self::step($value / $multiplier);

                if ($unit === '' || abs($step) >= 1 || $format !== 'auto') {
                    $value = $step;
                    $unit = $key;
                    $previous = $index;
                }
            }

            $index += 1;
        }

        return [self::count($value), $unit];
    }

    /** The rung a format names, singular or plural, or null where it names none. */
    private static function rung(string $format): ?string
    {
        foreach (self::LADDER as $key => $_) {
            if ($format === $key || $format === $key . 's') {
                return $key;
            }
        }

        return null;
    }

    /**
     * A step rounded on its magnitude, half away from zero, and given its
     * sign back, so a span and its negation climb alike. The rounding is
     * ECMAScript's `Math.round` written exactly: adding a half first would
     * carry 0.49999999999999994 to 1.
     */
    private static function step(float $value): float
    {
        $magnitude = abs($value);
        $floor = floor($magnitude);
        $rounded = $magnitude - $floor >= 0.5 ? $floor + 1 : $floor;

        return $value < 0 || ($value === 0.0 && fdiv(1.0, $value) < 0) ? -$rounded : $rounded;
    }

    /**
     * A count as an integer where it is one PHP can hold. The upper bound is
     * exclusive: 2^63 is a float PHP's integers stop short of, and casting it
     * would turn the future into the past. Negative zero stays a float, since
     * it is a past an integer cannot write.
     */
    private static function count(float $value): int|float
    {
        $integral = floor($value) === $value && !($value === 0.0 && fdiv(1.0, $value) < 0);

        return $integral && $value >= (float) PHP_INT_MIN && $value < (float) PHP_INT_MAX ? (int) $value : $value;
    }

    /**
     * A selection by the category the locale's rules put the number in. A
     * key that is a number selects the value it equals before any category
     * is asked for, so the facility is asked only where none does.
     *
     * @param \Closure(FormatRequest): void $observe
     * @param 'cardinal'|'ordinal' $type
     */
    private static function byCategory(ModifierInput $input, string $locale, \Closure $observe, string $type): string
    {
        $number = self::formattable($input->value);

        // An ordinal of a fraction names no position.
        if ($type === 'ordinal' && floor((float) $number) !== (float) $number) {
            throw new \ValueError('An ordinal is a whole number');
        }

        foreach ($input->options as $option) {
            $key = Text::isBlank($option->key) ? null : Comparisons::number($option->key);

            if ($key !== null && (float) $key === (float) $number) {
                return $option->value();
            }
        }

        $props = $type === 'cardinal' ? self::shownDigits($input->props) : $input->props;
        $category = self::ask(Api::PluralRules, $locale, ['type' => $type] + $props, $number, $observe);

        foreach ($input->options as $option) {
            if ($option->key === $category) {
                return $option->value();
            }
        }

        return $input->default();
    }

    /**
     * The digits `number` shows where no layer names a maximum: two fraction
     * digits, widened to a larger minimum a layer names, as ECMAScript
     * widens its own default.
     *
     * @param array<array-key, mixed> $props
     *
     * @return array<array-key, mixed>
     */
    private static function shownDigits(array $props): array
    {
        if (array_key_exists('maximumFractionDigits', $props)) {
            return $props;
        }

        $minimum = 0.0;

        // A minimum that is no number widens nothing; the facility is handed
        // it all the same, and refuses it there.
        try {
            $minimum = array_key_exists('minimumFractionDigits', $props) ? EcmaOptions::toNumber($props['minimumFractionDigits']) : 0.0;
        } catch (\TypeError) {
        }

        $maximum = is_nan($minimum) || $minimum < 2 ? 2 : $minimum;
        $props['maximumFractionDigits'] = is_finite($maximum) && floor($maximum) === $maximum && abs($maximum) < 2 ** 53 ? (int) $maximum : $maximum;

        return $props;
    }

    /**
     * The facility's answer to one request, observed once it answered or
     * refused; the refusal goes on to the modifier.
     *
     * @param array<array-key, mixed> $options
     * @param int|float|array{int|float, string} $request
     * @param \Closure(FormatRequest): void $observe
     */
    private static function ask(Api $api, string $locale, array $options, int|float|array $request, \Closure $observe): string
    {
        try {
            $answer = Facility::perform($api, $locale, $options, $request);
        } catch (\Throwable $refusal) {
            $observe(new FormatRequest($api, $options, $request));

            throw $refusal;
        }

        $observe(new FormatRequest($api, $options, $request, $api === Api::PluralRules ? $answer : null));

        return $answer;
    }
}
