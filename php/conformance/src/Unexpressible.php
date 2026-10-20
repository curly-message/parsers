<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * An entry of what an adapter declares it cannot express (section 11.2): a
 * property name, which stands for every value of the property in every
 * request, or a map naming the `property` and narrowing it to the `values`
 * it lists, to the requests whose properties named in `when` hold the values
 * given there, or to both.
 */
final class Unexpressible
{
    private const KEYS = ['property', 'values', 'when'];

    /**
     * Whether an entry is one: neither its values nor its condition empty,
     * since an empty list of values would claim nothing and an empty
     * condition would read as one while narrowing nothing.
     */
    public static function valid(mixed $entry): bool
    {
        if (is_string($entry)) {
            return true;
        }

        $fields = self::fields($entry);

        if ($fields === null || array_diff(array_map('strval', array_keys($fields)), self::KEYS) !== [] || !is_string($fields['property'] ?? null)) {
            return false;
        }

        $values = $fields['values'] ?? null;
        $when = $fields['when'] ?? null;

        return (!array_key_exists('values', $fields) || (is_array($values) && array_is_list($values) && $values !== []))
            && (!array_key_exists('when', $fields) || ((self::fields($when) ?? []) !== []));
    }

    /**
     * The reason an entry gives a request it covers, or null where it covers
     * none: the property must be one the request reads, among the values the
     * entry lists, in a request meeting its condition. A value is compared
     * as a request's are.
     */
    public static function covers(mixed $entry, string $api, \stdClass $options): ?string
    {
        $fields = is_string($entry) ? ['property' => $entry] : (self::fields($entry) ?? []);
        $property = is_string($fields['property'] ?? null) ? $fields['property'] : '';
        $values = is_array($fields['values'] ?? null) ? $fields['values'] : null;
        $when = self::fields($fields['when'] ?? null);
        $held = get_object_vars($options);

        if (!array_key_exists($property, $held)) {
            return null;
        }

        if ($values !== null && !self::any($values, static fn(mixed $value): bool => Canonical::equal(self::object($value), $held[$property]))) {
            return null;
        }

        foreach ($when ?? [] as $name => $value) {
            if (!array_key_exists((string) $name, $held) || !Canonical::equal(self::object($value), $held[(string) $name])) {
                return null;
            }
        }

        $value = $values === null ? '' : 'the value ' . Canonical::text($held[$property]) . ' of ';
        $condition = [];

        foreach ($when ?? [] as $name => $holds) {
            $condition[] = "{$name} is " . Canonical::text($holds);
        }

        return "The adapter cannot express {$value}the {$property} property of a {$api} request" . ($condition === [] ? '' : ' whose ' . implode(' and ', $condition)) . '.';
    }

    /**
     * The members of a map, an array that is not a list or an object; null
     * where the value is neither.
     *
     * @return array<array-key, mixed>|null
     */
    private static function fields(mixed $value): ?array
    {
        if ($value instanceof \stdClass) {
            return get_object_vars($value);
        }

        return is_array($value) && ($value === [] || !array_is_list($value)) ? $value : null;
    }

    // An adapter writes a map as an array, and the set writes one as an
    // object, which is the side `Canonical::equal` reads as expected.
    private static function object(mixed $value): mixed
    {
        if (is_array($value) && $value !== [] && !array_is_list($value)) {
            $object = new \stdClass();

            foreach ($value as $key => $entry) {
                $object->{$key} = self::object($entry);
            }

            return $object;
        }

        return is_array($value) ? array_map(self::object(...), $value) : $value;
    }

    /**
     * @param array<array-key, mixed> $values
     * @param \Closure(mixed): bool $holds
     */
    private static function any(array $values, \Closure $holds): bool
    {
        foreach ($values as $value) {
            if ($holds($value)) {
                return true;
            }
        }

        return false;
    }
}
