<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

use CurlyMessage\Internal\Unicode\Lowercase;
use CurlyMessage\ModifierInput;
use CurlyMessage\Option;

/**
 * The comparison modifiers (SPEC.md section 11.1). A selection that matched
 * answers with its option's value, empty or not; one that matched nothing
 * answers with the fallback chain, which is read only then.
 *
 * @internal
 */
final class Comparisons
{
    public const NAMES = ['eq', 'ne', 'lt', 'gt', 'lte', 'gte'];

    public static function call(string $name, ModifierInput $input): string
    {
        return match ($name) {
            'eq' => self::selected(self::equal($input), $input),
            'ne' => self::selected(self::differing($input), $input),
            'lt' => self::selected(self::below($input), $input),
            'gt' => self::selected(self::above($input), $input),
            'lte' => self::selected(self::equal($input) ?? self::below($input), $input),
            'gte' => self::selected(self::equal($input) ?? self::above($input), $input),
            default => $input->default(),
        };
    }

    /**
     * The number a text is, by PHP's numeric strings: `is_numeric`, then the
     * number it reads as. A text that is none compares false, never as zero.
     * The unary plus keeps the sign of `-0.0`, which adding to zero drops.
     */
    public static function number(string $text): int|float|null
    {
        return is_numeric($text) ? +$text : null;
    }

    private static function selected(?Option $option, ModifierInput $input): string
    {
        return $option === null ? $input->default() : $option->value();
    }

    // The value is lowered once, ahead of the options, so what that costs adds
    // to the options' cost rather than multiplying it.
    private static function equal(ModifierInput $input): ?Option
    {
        if (!self::comparable($input)) {
            return null;
        }

        $needle = Lowercase::of($input->value);

        foreach ($input->options as $option) {
            if (Lowercase::of($option->key) === $needle) {
                return $option;
            }
        }

        return null;
    }

    private static function differing(ModifierInput $input): ?Option
    {
        if (!self::comparable($input)) {
            return $input->options[0] ?? null;
        }

        $needle = Lowercase::of($input->value);

        foreach ($input->options as $option) {
            if (Lowercase::of($option->key) !== $needle) {
                return $option;
            }
        }

        return null;
    }

    /**
     * Whether the value is short enough to lower to any key's lowering. No
     * character lowers to less than a third of its bytes nor to more than
     * half again as many, so a value more than four and a half times as long
     * as the longest key equals none, and is never lowered: its lowering,
     * which may be half again as long as the value, is the one text a
     * comparison would make that the message does not bound.
     */
    private static function comparable(ModifierInput $input): bool
    {
        $longest = 0;

        foreach ($input->options as $option) {
            $longest = max($longest, strlen($option->key));
        }

        return 2 * strlen($input->value) <= 9 * $longest;
    }

    private static function below(ModifierInput $input): ?Option
    {
        $value = self::number($input->value);

        foreach (self::ordered($input->options, 1) as [$key, $option]) {
            if ($value !== null && $value < $key) {
                return $option;
            }
        }

        return null;
    }

    private static function above(ModifierInput $input): ?Option
    {
        $value = self::number($input->value);

        foreach (self::ordered($input->options, -1) as [$key, $option]) {
            if ($value !== null && $value > $key) {
                return $option;
            }
        }

        return null;
    }

    /**
     * The options a numeric comparison can order, by key, on a copy: a key
     * that is no number is never selected by one, and the list is the
     * modifier's input, which a comparison leaves as it found it. `usort` is
     * stable, so keys of one number keep their source order.
     *
     * @param list<Option> $options
     *
     * @return list<array{int|float, Option}>
     */
    private static function ordered(array $options, int $direction): array
    {
        $numbered = [];

        foreach ($options as $option) {
            $key = self::number($option->key);

            if ($key !== null) {
                $numbered[] = [$key, $option];
            }
        }

        usort($numbered, static fn(array $a, array $b): int => $direction * ($a[0] <=> $b[0]));

        return $numbered;
    }
}
