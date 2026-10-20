<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * What the facilities build once and use again: formatters by the request
 * that built them, and what ICU's data says of a locale. Each table is
 * bounded and drops its oldest entry first, and a key longer than a locale
 * or an option is ever written builds its value without keeping it, so what
 * a process holds is bounded by these numbers rather than by its payloads.
 *
 * @internal
 */
final class Cache
{
    private const FORMATTERS = 256;

    private const LOCALES = 64;

    private const KEY = 64;

    /** @var array<string, mixed> */
    private static array $formatters = [];

    /** @var array<string, array<string, mixed>> */
    private static array $locales = [];

    /**
     * The value built for `$key`, built now where none is kept. A null key
     * names a request that cannot be spelled, which is built every time.
     *
     * @template T
     *
     * @param \Closure(): T $build
     *
     * @return T
     */
    public static function formatter(?string $key, \Closure $build): mixed
    {
        if ($key === null) {
            return $build();
        }

        if (array_key_exists($key, self::$formatters)) {
            /** @var T */
            return self::$formatters[$key];
        }

        $built = $build();

        if (count(self::$formatters) >= self::FORMATTERS) {
            unset(self::$formatters[array_key_first(self::$formatters)]);
        }

        self::$formatters[$key] = $built;

        return $built;
    }

    /**
     * What one kind of locale data holds for `$key`, built now where none is
     * kept.
     *
     * @template T
     *
     * @param \Closure(): T $build
     *
     * @return T
     */
    public static function locale(string $kind, string $key, \Closure $build): mixed
    {
        if (strlen($key) > self::KEY) {
            return $build();
        }

        $table = self::$locales[$kind] ?? [];

        if (array_key_exists($key, $table)) {
            /** @var T */
            return $table[$key];
        }

        $built = $build();
        // Building may have read this kind for another key (a chain reads its
        // parents), so the table is read again rather than written over.
        $table = self::$locales[$kind] ?? [];

        if (count($table) >= self::LOCALES) {
            unset($table[array_key_first($table)]);
        }

        $table[$key] = $built;
        self::$locales[$kind] = $table;

        return $built;
    }

    /**
     * Drops everything kept, so what follows pays what a fresh process pays.
     *
     * @internal for the benchmark and the tests
     */
    public static function clear(): void
    {
        self::$formatters = [];
        self::$locales = [];
    }

    /** How many values are kept, for the tests. @internal */
    public static function size(): int
    {
        return count(self::$formatters) + array_sum(array_map(count(...), self::$locales));
    }

    /**
     * The key a value is kept under, spelled from its parts, or null where a
     * part is anything but a scalar or null, or is longer than a locale or an
     * option is ever written. A part is measured as it is written, a string
     * without the quotes and escapes its spelling adds.
     *
     * @param array<array-key, mixed> $parts
     */
    public static function key(array $parts): ?string
    {
        $spelled = '';

        foreach ($parts as $name => $part) {
            // Spelling a string copies it several times over, so a long one
            // is turned away before it is spelled.
            if ((!is_scalar($part) && $part !== null) || strlen((string) $name) > self::KEY || (is_string($part) && strlen($part) > self::KEY)) {
                return null;
            }

            $text = var_export($part, true);

            if (!is_string($part) && strlen($text) > self::KEY) {
                return null;
            }

            $spelled .= var_export($name, true) . '=' . $text . ';';
        }

        return $spelled;
    }
}
