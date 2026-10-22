<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * Whether this host satisfies the Intl level (SPEC.md section 2): ext-intl
 * loaded, with the classes it is performed on, and ICU's plural rules and
 * relative-time fields readable, since neither has a binding of its own and
 * both are read as data. Asked once per process.
 *
 * @internal
 */
final class Available
{
    private const CLASSES = [
        \NumberFormatter::class,
        \IntlDateFormatter::class,
        \IntlDatePatternGenerator::class,
        \IntlCalendar::class,
        \IntlGregorianCalendar::class,
        \IntlTimeZone::class,
        \Locale::class,
        \ResourceBundle::class,
    ];

    private static ?bool $satisfied = null;

    public static function satisfied(): bool
    {
        return self::$satisfied ??= self::probe();
    }

    /**
     * What the facilities here cannot express (SPEC.md section 11.2), by
     * facility: a property no value of which can be performed. None where
     * there are no facilities.
     *
     * @return array<string, list<string>>
     */
    public static function unexpressible(): array
    {
        if (!self::satisfied()) {
            return [];
        }

        $table = [
            'NumberFormat' => ['notation', 'compactDisplay', 'signDisplay', 'unit', 'unitDisplay', 'currencySign', 'localeMatcher'],
            'DateTimeFormat' => ['formatMatcher', 'localeMatcher'],
            'RelativeTimeFormat' => ['localeMatcher'],
            'PluralRules' => ['localeMatcher'],
        ];

        // The half-ceiling and half-floor modes arrived in ICU 69, and only
        // the facilities that round take a mode.
        if (version_compare(INTL_ICU_VERSION, '69', '<')) {
            $table['NumberFormat'][] = 'roundingMode';
            $table['PluralRules'][] = 'roundingMode';
        }

        return $table;
    }

    private static function probe(): bool
    {
        if (!extension_loaded('intl')) {
            return false;
        }

        foreach (self::CLASSES as $class) {
            if (!class_exists($class, false)) {
                return false;
            }
        }

        try {
            return IcuData::plurals() !== [] && IcuData::locale('root')['fields'] !== [];
        } catch (\Throwable) {
            return false;
        }
    }
}
