<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * The host value a fixture's JSON stands for (the set's README.md, "Tagged
 * values"): an object owning `$curly` is replaced, at any depth, and
 * everything else is copied as JSON describes it, so no run shares a value
 * with the set or with another run.
 */
final class Decode
{
    public const TAG = '$curly';

    public static function value(mixed $value, mixed $absent): mixed
    {
        if (is_array($value)) {
            return array_map(static fn(mixed $entry): mixed => self::value($entry, $absent), $value);
        }

        if (!$value instanceof \stdClass) {
            return $value;
        }

        if (property_exists($value, self::TAG)) {
            return self::tagged($value, $absent);
        }

        $copy = new \stdClass();

        foreach (get_object_vars($value) as $key => $entry) {
            $copy->{$key} = self::value($entry, $absent);
        }

        return $copy;
    }

    /**
     * A value whose serialization visits at least `$count` nodes. A shared
     * array is visited again each time it is met, so each level of
     * `[$below, $below]` doubles the visits while holding one array more.
     *
     * @return list<mixed>
     */
    public static function nodes(int|float $count): array
    {
        $tree = [];

        for ($visits = 1; $visits < $count; $visits = $visits * 2 + 1) {
            $tree = [$tree, $tree];
        }

        return $tree;
    }

    private static function tagged(\stdClass $tag, mixed $absent): mixed
    {
        $name = $tag->{self::TAG};

        switch ($name) {
            case 'undefined':
                return $absent;
            case 'unserializable':
                $value = new \stdClass();
                $value->self = $value;

                return $value;
            case 'nodes':
                $count = $tag->count ?? null;

                if (!is_int($count) && !is_float($count)) {
                    throw new \RuntimeException('A nodes tag needs a numeric count.');
                }

                return self::nodes($count);
            default:
                throw new \RuntimeException('Unknown tag ' . json_encode($name) . '.');
        }
    }
}
