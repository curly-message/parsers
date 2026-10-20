<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * ICU's own data, read through `ResourceBundle` where ext-intl has no binding
 * for what uses it: the plural rules, the relative-time fields, the minimum
 * grouping digits, the narrow currency symbols. A bundle is only ever
 * enumerated, never asked for a key it may lack, since a miss is reported
 * through the host's `intl.error_level` and `intl.use_exceptions`, and what
 * is read is kept as PHP arrays, a locale at a time.
 *
 * @internal
 *
 * @phpstan-type Fields array<string, array{relative: array<string, string>, future: array<string, string>, past: array<string, string>}>
 * @phpstan-type LocaleData array{parent: ?string, fields: Fields, grouping: ?int}
 */
final class IcuData
{
    /** The units a relative time is stated in, and so the fields read. */
    public const UNITS = ['second', 'minute', 'hour', 'day', 'week', 'month', 'quarter', 'year'];

    // Each style falls back to the next wider one, as ICU's
    // RelativeDateTimeFormatter falls back.
    private const STYLES = ['narrow' => ['-narrow', '-short', ''], 'short' => ['-short', ''], 'long' => ['']];

    // How far a chain of parents may run; ICU's own run a few steps.
    private const CHAIN = 16;

    /** @var array<string, true>|null */
    private static ?array $available = null;

    private static ?string $fallback = null;

    /** @var array<string, true>|null */
    private static ?array $currencies = null;

    /** @var array{cardinal: array<string, string>, ordinal: array<string, string>, rules: array<string, array<string, string>>}|array{}|null */
    private static ?array $plurals = null;

    /** @var array<string, bool>|null */
    private static ?array $numberingSystems = null;

    /** @var array<string, string>|null */
    private static ?array $timeZones = null;

    /**
     * ICU's plural data: the rule set of each locale for each type, and the
     * rule text of each set by category, in the order ICU evaluates them.
     *
     * @return array{cardinal: array<string, string>, ordinal: array<string, string>, rules: array<string, array<string, string>>}|array{}
     */
    public static function plurals(): array
    {
        if (self::$plurals !== null) {
            return self::$plurals;
        }

        $bundle = \ResourceBundle::create('plurals', null, false);
        $read = ['cardinal' => [], 'ordinal' => [], 'rules' => []];

        if (!$bundle instanceof \ResourceBundle) {
            return self::$plurals = [];
        }

        foreach ($bundle as $key => $table) {
            if (!$table instanceof \ResourceBundle) {
                continue;
            }

            if ($key === 'locales' || $key === 'locales_ordinals') {
                $read[$key === 'locales' ? 'cardinal' : 'ordinal'] = self::strings($table);
            } elseif ($key === 'rules') {
                foreach ($table as $set => $rules) {
                    if (is_string($set) && $rules instanceof \ResourceBundle) {
                        $read['rules'][$set] = self::strings($rules);
                    }
                }
            }
        }

        return self::$plurals = $read;
    }

    /**
     * What one locale's bundle holds of what the facilities read: the parent
     * it names, its relative-time fields and its minimum grouping digits,
     * each null or empty where it holds none.
     *
     * @return LocaleData
     */
    public static function locale(string $id): array
    {
        /** @var LocaleData */
        return Cache::locale('data', $id, static fn(): array => self::read($id));
    }

    /**
     * The locales ICU looks a locale's data up in, most specific first: the
     * locale, the parent its bundle names or its truncation, down to `root`.
     *
     * @return list<string>
     */
    public static function chain(string $id): array
    {
        /** @var list<string> */
        return Cache::locale('chain', $id, static function () use ($id): array {
            $chain = [];
            $at = $id;

            while (count($chain) < self::CHAIN) {
                if (self::exists($at)) {
                    $chain[] = $at;
                }

                if ($at === 'root') {
                    break;
                }

                $parent = self::exists($at) ? self::locale($at)['parent'] : null;
                $cut = strrpos($at, '_');
                $at = $parent ?? ($cut === false ? 'root' : rtrim(substr($at, 0, $cut), '_'));

                if ($at === '') {
                    $at = 'root';
                }
            }

            return $chain;
        });
    }

    /**
     * A relative-time pattern or phrase as ICU's RelativeDateTimeFormatter
     * finds it: the style asked for through the locale's chain, then each
     * wider style. `root` speaks only for the long style, since its narrower
     * ones are aliases to the requested locale's own wider ones.
     *
     * @param 'relative'|'future'|'past' $part
     */
    public static function field(string $locale, string $unit, string $style, string $part, string $key): ?string
    {
        foreach (self::STYLES[$style] ?? [''] as $suffix) {
            foreach (self::chain($locale) as $id) {
                if ($suffix !== '' && $id === 'root') {
                    continue;
                }

                $found = self::locale($id)['fields'][$unit . $suffix][$part][$key] ?? null;

                if ($found !== null && $found !== '') {
                    return $found;
                }
            }
        }

        return null;
    }

    /** The locale's minimum grouping digits, ICU's default of 1 where its chain names none. */
    public static function minimumGrouping(string $locale): int
    {
        foreach (self::chain($locale) as $id) {
            $grouping = self::locale($id)['grouping'];

            if ($grouping !== null) {
                return $grouping;
            }
        }

        return 1;
    }

    /** The currency's narrow symbol in the locale, null where its chain has none. */
    public static function narrowSymbol(string $locale, string $currency): ?string
    {
        foreach (self::chain($locale) as $id) {
            /** @var array<string, string> $symbols */
            $symbols = Cache::locale('narrow', $id, static fn(): array => self::narrow($id));

            if (isset($symbols[$currency])) {
                return $symbols[$currency];
            }
        }

        return null;
    }

    /**
     * Every numbering system ICU knows, by name, true where it is
     * algorithmic, which ECMAScript does not take.
     *
     * @return array<string, bool>
     */
    public static function numberingSystems(): array
    {
        if (self::$numberingSystems !== null) {
            return self::$numberingSystems;
        }

        $found = [];
        $bundle = \ResourceBundle::create('numberingSystems', null, false);

        foreach ($bundle instanceof \ResourceBundle ? $bundle : [] as $key => $table) {
            if ($key !== 'numberingSystems' || !$table instanceof \ResourceBundle) {
                continue;
            }

            foreach ($table as $name => $system) {
                $algorithmic = true;

                foreach ($system instanceof \ResourceBundle ? $system : [] as $field => $value) {
                    if ($field === 'algorithmic') {
                        $algorithmic = $value !== 0;
                    }
                }

                if (is_string($name)) {
                    $found[$name] = $algorithmic;
                }
            }
        }

        return self::$numberingSystems = $found;
    }

    /**
     * Every time zone ICU knows, by its name in ASCII lower case, since
     * ECMAScript matches a zone's name without regard to case.
     *
     * @return array<string, string>
     */
    public static function timeZones(): array
    {
        if (self::$timeZones !== null) {
            return self::$timeZones;
        }

        $found = [];

        foreach (\IntlTimeZone::createEnumeration() as $zone) {
            if (is_string($zone)) {
                $found[strtolower($zone)] = $zone;
            }
        }

        return self::$timeZones = $found;
    }

    /**
     * The locale ICU builds a formatter at for `$id`, or '' for a language
     * ICU lists no code for, which PHP 8.4 and later refuse to build at.
     */
    public static function actual(string $id): string
    {
        try {
            return (new \NumberFormatter($id, \NumberFormatter::DECIMAL))->getLocale(\Locale::ACTUAL_LOCALE);
        } catch (\ValueError) {
            return '';
        }
    }

    /**
     * The locale ICU builds at where nothing matches: its own default, asked
     * of the code for no linguistic content, which ICU lists and no data
     * ever names.
     */
    public static function fallback(): string
    {
        return self::$fallback ??= self::actual('zxx');
    }

    /** Whether ICU's main data has the locale or one of its truncations. */
    public static function covers(string $base): bool
    {
        for ($at = $base; $at !== ''; $at = rtrim(substr($at, 0, (int) strrpos($at, '_')), '_')) {
            if (self::exists($at)) {
                return true;
            }

            if (!str_contains($at, '_')) {
                return false;
            }
        }

        return false;
    }

    private static function exists(string $id): bool
    {
        if (self::$available === null) {
            self::$available = self::names(\ResourceBundle::getLocales(''));
        }

        return isset(self::$available[$id]);
    }

    /** @return LocaleData */
    private static function read(string $id): array
    {
        $data = ['parent' => null, 'fields' => [], 'grouping' => null];
        $bundle = self::exists($id) ? \ResourceBundle::create($id, null, false) : null;

        foreach ($bundle instanceof \ResourceBundle ? $bundle : [] as $key => $value) {
            if ($key === '%%Parent' && is_string($value)) {
                $data['parent'] = $value;
            } elseif ($key === 'fields' && $value instanceof \ResourceBundle) {
                $data['fields'] = self::fields($value);
            } elseif ($key === 'NumberElements' && $value instanceof \ResourceBundle) {
                foreach ($value as $element => $grouping) {
                    if ($element === 'minimumGroupingDigits' && is_numeric($grouping)) {
                        $data['grouping'] = (int) $grouping;
                    }
                }
            }
        }

        return $data;
    }

    /** @return Fields */
    private static function fields(\ResourceBundle $fields): array
    {
        $read = [];

        foreach ($fields as $name => $field) {
            if (!is_string($name) || !in_array(explode('-', $name, 2)[0], self::UNITS, true) || !$field instanceof \ResourceBundle) {
                continue;
            }

            $entry = ['relative' => [], 'future' => [], 'past' => []];

            foreach ($field as $part => $table) {
                if (!$table instanceof \ResourceBundle) {
                    continue;
                }

                if ($part === 'relative') {
                    $entry['relative'] = self::strings($table);
                } elseif ($part === 'relativeTime') {
                    foreach ($table as $direction => $patterns) {
                        if (($direction === 'future' || $direction === 'past') && $patterns instanceof \ResourceBundle) {
                            $entry[$direction] = self::strings($patterns);
                        }
                    }
                }
            }

            $read[$name] = $entry;
        }

        return $read;
    }

    /** @return array<string, string> */
    private static function narrow(string $id): array
    {
        if (self::$currencies === null) {
            self::$currencies = self::names(\ResourceBundle::getLocales('ICUDATA-curr'));
        }

        $bundle = isset(self::$currencies[$id]) ? \ResourceBundle::create($id, 'ICUDATA-curr', false) : null;

        foreach ($bundle instanceof \ResourceBundle ? $bundle : [] as $key => $table) {
            if ($key === 'Currencies%narrow' && $table instanceof \ResourceBundle) {
                return self::strings($table);
            }
        }

        return [];
    }

    /**
     * A table's string entries by key.
     *
     * @return array<string, string>
     */
    private static function strings(\ResourceBundle $table): array
    {
        $read = [];

        foreach ($table as $key => $value) {
            if (is_string($key) && is_string($value)) {
                $read[$key] = $value;
            }
        }

        return $read;
    }

    /**
     * The locales a data tree lists, and `root`, which it holds without
     * listing.
     *
     * @param array<array-key, mixed>|false $locales
     *
     * @return array<string, true>
     */
    private static function names(array|false $locales): array
    {
        $names = ['root' => true];

        foreach ($locales === false ? [] : $locales as $locale) {
            if (is_string($locale)) {
                $names[$locale] = true;
            }
        }

        return $names;
    }
}
