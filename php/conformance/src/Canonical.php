<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * Two values compared by their entries, never by the text a serialization
 * makes of them: a request's property order is the host's (RUNNER.md, "A
 * formatting request"). The walk follows the expected value, which is the
 * set's, so what it costs is bounded by the case and not by the answer.
 */
final class Canonical
{
    /**
     * A value written as a request's are compared, for a reason or a failure
     * line: a map by its sorted entries, and a negative zero as `-0`.
     */
    public static function text(mixed $value): string
    {
        if (is_float($value) && self::negative($value)) {
            return '-0';
        }

        $entries = $value instanceof \stdClass || (is_array($value) && !array_is_list($value)) ? self::map($value) : null;

        if ($entries !== null) {
            ksort($entries, SORT_STRING);
            $written = [];

            foreach ($entries as $key => $entry) {
                $written[] = json_encode((string) $key, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_INVALID_UTF8_SUBSTITUTE) . ':' . self::text($entry);
            }

            return '{' . implode(',', $written) . '}';
        }

        if (is_array($value)) {
            return '[' . implode(',', array_map(self::text(...), $value)) . ']';
        }

        $json = json_encode($value, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_INVALID_UTF8_SUBSTITUTE);

        return $json === false ? get_debug_type($value) : $json;
    }

    public static function equal(mixed $expected, mixed $actual): bool
    {
        if ($expected instanceof \stdClass) {
            $entries = self::map($actual);

            if ($entries === null) {
                return false;
            }

            $wanted = get_object_vars($expected);
            $keys = array_map('strval', array_keys($wanted));
            $held = array_map('strval', array_keys($entries));
            sort($keys, SORT_STRING);
            sort($held, SORT_STRING);

            if ($keys !== $held) {
                return false;
            }

            foreach ($wanted as $key => $value) {
                if (!self::equal($value, $entries[$key])) {
                    return false;
                }
            }

            return true;
        }

        if (is_array($expected)) {
            if (!is_array($actual) || !array_is_list($actual) || count($actual) !== count($expected)) {
                return false;
            }

            foreach (array_values($expected) as $index => $value) {
                if (!self::equal($value, $actual[$index])) {
                    return false;
                }
            }

            return true;
        }

        return self::scalar($expected, $actual);
    }

    /**
     * Two scalars alike: numbers by value and by the sign of zero, so `1` is
     * `1.0` and `-0.0` is not `0`, since a relative time reads -0 as the past
     * and 0 as the future; anything else as itself.
     */
    public static function scalar(mixed $expected, mixed $actual): bool
    {
        if ((is_int($expected) || is_float($expected)) && (is_int($actual) || is_float($actual))) {
            return is_int($expected) && is_int($actual)
                ? $expected === $actual
                : (float) $expected === (float) $actual && self::negative($expected) === self::negative($actual);
        }

        return $expected === $actual;
    }

    // Whether a number is a negative zero, which compares equal to zero.
    private static function negative(int|float $number): bool
    {
        return is_float($number) && $number === 0.0 && fdiv(1, $number) < 0;
    }

    /**
     * The entries of a value read as a map: an object's own properties, or an
     * array's entries where the array is not a list of values. An empty array
     * is the empty map, since PHP spells both alike.
     *
     * @return array<array-key, mixed>|null
     */
    private static function map(mixed $value): ?array
    {
        if ($value instanceof \stdClass) {
            return get_object_vars($value);
        }

        if (is_array($value) && ($value === [] || !array_is_list($value))) {
            return $value;
        }

        return null;
    }
}
