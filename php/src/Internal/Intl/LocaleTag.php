<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * A language tag a caller supplied, checked and resolved before any facility
 * is built from it. ICU's constructors answer a tag they cannot use each in
 * their own way (one formats at `root`, another at the process default,
 * another leaves an object that cannot format), so the tag is first held to
 * the structure of a `unicode_locale_id` (UTS #35), which is what ECMAScript
 * checks too, and a tag that fails is one the host rejects. A well-formed tag
 * is then resolved to the locale ICU has data for, its `-u-` keywords kept
 * for the facilities that read them.
 *
 * @internal
 */
final readonly class LocaleTag
{
    /**
     * @param string $base the ICU locale the tag resolved to, without keywords
     * @param array<string, string> $keywords the tag's `-u-` keywords, by key, the first of each
     */
    public function __construct(
        public string $base,
        public array $keywords,
    ) {}

    /** The tag resolved, or null where it is not a well-formed tag. */
    public static function of(string $tag): ?self
    {
        $resolved = Cache::locale('tag', $tag, static function () use ($tag): self|false {
            $parsed = self::parse($tag);

            if ($parsed === null) {
                return false;
            }

            $base = self::resolve($parsed[0]);

            return $base === null ? false : new self($base, $parsed[1]);
        });

        return $resolved === false ? null : $resolved;
    }

    /**
     * The tag's structure: the ICU spelling of its language, script, region
     * and variants, and its `-u-` keywords; null where it is not a
     * `unicode_locale_id`. Read subtag by subtag, with no pattern.
     *
     * @return array{string, array<string, string>}|null
     */
    public static function parse(string $tag): ?array
    {
        if ($tag === '') {
            return null;
        }

        $subtags = [];
        $start = 0;
        $length = strlen($tag);

        for ($at = 0; $at <= $length; $at += 1) {
            if ($at < $length && $tag[$at] !== '-' && $tag[$at] !== '_') {
                $byte = ord($tag[$at]);

                if (!(($byte >= 0x30 && $byte <= 0x39) || ($byte >= 0x41 && $byte <= 0x5A) || ($byte >= 0x61 && $byte <= 0x7A))) {
                    return null;
                }

                continue;
            }

            if ($at === $start || $at - $start > 8) {
                return null;
            }

            $subtags[] = strtolower(substr($tag, $start, $at - $start));
            $start = $at + 1;
        }

        $count = count($subtags);
        $index = 0;
        $language = $subtags[$index];

        if (!self::isLanguage($language) && $language !== 'root') {
            return null;
        }

        $base = $language;
        $index += 1;

        if ($index < $count && self::isScript($subtags[$index])) {
            $base .= '_' . ucfirst($subtags[$index]);
            $index += 1;
        }

        $region = $index < $count && self::isRegion($subtags[$index]) ? strtoupper($subtags[$index]) : '';
        $index += $region === '' ? 0 : 1;
        $variants = [];

        while ($index < $count && self::isVariant($subtags[$index])) {
            if (isset($variants[$subtags[$index]])) {
                return null;
            }

            $variants[$subtags[$index]] = true;
            $index += 1;
        }

        // A variant follows a region in ICU's spelling, so one with no region
        // leaves the region's place empty.
        if ($region !== '' || $variants !== []) {
            $base .= '_' . $region;
        }

        if ($variants !== []) {
            $base .= '_' . strtoupper(implode('_', array_keys($variants)));
        }

        $keywords = [];
        $singletons = [];

        while ($index < $count) {
            $singleton = $subtags[$index];

            if (strlen($singleton) !== 1 || isset($singletons[$singleton])) {
                return null;
            }

            $singletons[$singleton] = true;
            $index += 1;

            $read = match ($singleton) {
                'x' => self::privateUse($subtags, $index),
                'u' => self::unicode($subtags, $index, $keywords),
                't' => self::transformed($subtags, $index),
                default => self::other($subtags, $index),
            };

            if ($read === null) {
                return null;
            }

            $index = $read;

            if ($singleton === 'x') {
                break;
            }
        }

        return $index === $count ? [$base, $keywords] : null;
    }

    /**
     * The numbering system a request names, as ICU calls it: the
     * `numberingSystem` property where one is named, else the tag's `nu`
     * keyword. A name ICU does not know, or an algorithmic system ECMAScript
     * does not take, leaves the locale's own, as V8 leaves it; a name that is
     * no `type` is refused.
     *
     * @param array<array-key, mixed> $options
     */
    public function numbering(array $options): ?string
    {
        $named = EcmaOptions::string($options, 'numberingSystem', null, null);

        if ($named !== null && !EcmaOptions::isType($named)) {
            throw new \ValueError('numberingSystem is not a type');
        }

        $system = $named === null ? ($this->keywords['nu'] ?? null) : strtolower($named);

        return $system !== null && (IcuData::numberingSystems()[$system] ?? true) === false ? $system : null;
    }

    /**
     * The tag with ICU keywords added, as ICU's constructors read them.
     *
     * @param array<string, string|null> $keywords
     */
    public function icu(array $keywords): string
    {
        $spelled = [];

        foreach ($keywords as $key => $value) {
            if ($value !== null) {
                $spelled[] = $key . '=' . $value;
            }
        }

        return $spelled === [] ? $this->base : $this->base . '@' . implode(';', $spelled);
    }

    /**
     * The locale ICU has data for that answers for `$base`: the one its
     * formatters are actually built at, which follows ICU's own aliases and
     * parents (`zh_TW` is `zh_Hant_TW`, `iw` is `he`) and falls back to the
     * process default where nothing matches, as ECMAScript falls back to its
     * default locale. Where nothing matches ICU builds at its own default,
     * which is not PHP's where `intl.default_locale` is set, so a locale with
     * no data of its own that lands on ICU's default has matched nothing.
     */
    private static function resolve(string $base): ?string
    {
        try {
            $actual = IcuData::actual($base);
            $unmatched = in_array($actual, ['', 'root'], true) || ($actual === IcuData::fallback() && !IcuData::covers($base));
        } catch (\Throwable) {
            return null;
        }

        return $unmatched ? \Locale::getDefault() : $actual;
    }

    private static function isLanguage(string $subtag): bool
    {
        return self::alpha($subtag) && in_array(strlen($subtag), [2, 3, 5, 6, 7, 8], true);
    }

    private static function isScript(string $subtag): bool
    {
        return strlen($subtag) === 4 && self::alpha($subtag);
    }

    private static function isRegion(string $subtag): bool
    {
        return (strlen($subtag) === 2 && self::alpha($subtag)) || (strlen($subtag) === 3 && self::digits($subtag));
    }

    private static function isVariant(string $subtag): bool
    {
        $length = strlen($subtag);

        return ($length >= 5 && $length <= 8) || ($length === 4 && self::digits($subtag[0]));
    }

    /** @param list<string> $subtags */
    private static function privateUse(array $subtags, int $index): ?int
    {
        $start = $index;

        while ($index < count($subtags)) {
            $index += 1;
        }

        return $index > $start ? $index : null;
    }

    /**
     * @param list<string> $subtags
     * @param array<string, string> $keywords
     */
    private static function unicode(array $subtags, int $index, array &$keywords): ?int
    {
        $start = $index;
        $count = count($subtags);

        while ($index < $count && strlen($subtags[$index]) >= 3) {
            $index += 1;
        }

        while ($index < $count && strlen($subtags[$index]) === 2) {
            $key = $subtags[$index];

            if (!self::alpha($key[1])) {
                return null;
            }

            $index += 1;
            $types = [];

            while ($index < $count && strlen($subtags[$index]) >= 3) {
                $types[] = $subtags[$index];
                $index += 1;
            }

            $keywords[$key] ??= $types === [] ? 'true' : implode('-', $types);
        }

        return $index > $start ? $index : null;
    }

    /** @param list<string> $subtags */
    private static function transformed(array $subtags, int $index): ?int
    {
        $start = $index;
        $count = count($subtags);

        if ($index < $count && self::isLanguage($subtags[$index])) {
            $index += 1;

            if ($index < $count && self::isScript($subtags[$index])) {
                $index += 1;
            }

            if ($index < $count && self::isRegion($subtags[$index])) {
                $index += 1;
            }

            while ($index < $count && self::isVariant($subtags[$index])) {
                $index += 1;
            }
        }

        while ($index < $count && strlen($subtags[$index]) === 2 && self::alpha($subtags[$index][0]) && self::digits($subtags[$index][1])) {
            $index += 1;
            $values = $index;

            while ($index < $count && strlen($subtags[$index]) >= 3) {
                $index += 1;
            }

            if ($index === $values) {
                return null;
            }
        }

        return $index > $start ? $index : null;
    }

    /** @param list<string> $subtags */
    private static function other(array $subtags, int $index): ?int
    {
        $start = $index;

        while ($index < count($subtags) && strlen($subtags[$index]) >= 2) {
            $index += 1;
        }

        return $index > $start ? $index : null;
    }

    // The subtags are lower case by now, so a letter is one of 26 bytes.
    private static function alpha(string $text): bool
    {
        return strspn($text, 'abcdefghijklmnopqrstuvwxyz') === strlen($text);
    }

    private static function digits(string $text): bool
    {
        return strspn($text, '0123456789') === strlen($text);
    }
}
