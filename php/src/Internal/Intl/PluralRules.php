<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * ECMAScript's `Intl.PluralRules`, performed from ICU's own rules, which
 * ext-intl has no binding for: the rule set ICU's `PluralRules::forLocale`
 * picks for the locale, asked of the number's digits as `Rounding` shows
 * them (SPEC.md section 11.5), so `1.0` is `many` in Czech where `1` is
 * `one`.
 *
 * @internal
 */
final readonly class PluralRules
{
    /** @param array<string, PluralRule> $rules every category's rule but `other`, in the order ICU asks them */
    private function __construct(
        private Rounding $rounding,
        private array $rules,
    ) {}

    /**
     * @param array<array-key, mixed> $options
     */
    public static function build(LocaleTag $tag, array $options): self
    {
        $type = EcmaOptions::string($options, 'type', ['cardinal', 'ordinal'], 'cardinal');
        $rounding = Rounding::read($options, 0, 3);

        return new self($rounding, self::rules($tag->base, $type));
    }

    /** The category the number is in. */
    public function select(int|float $number): string
    {
        $rounding = $this->rounding->isSettled() ? $this->rounding : $this->rounding->settled($number);
        [$integer, $fraction] = Digits::parts(Digits::shown($number, $rounding));

        foreach ($this->rules as $category => $rule) {
            if ($rule->holds($integer, $fraction)) {
                return $category;
            }
        }

        return 'other';
    }

    /**
     * The rules of the set ICU gives the locale, found as ICU finds it: the
     * locale, then each truncation of it; a locale none names has `other`
     * alone. `other` is what no other rule holds for, so it is never asked.
     *
     * @return array<string, PluralRule>
     */
    private static function rules(string $base, string $type): array
    {
        $plurals = IcuData::plurals();
        $sets = $plurals === [] ? [] : ($type === 'ordinal' ? $plurals['ordinal'] : $plurals['cardinal']);
        $set = null;

        for ($at = $base; $at !== ''; $at = rtrim(substr($at, 0, max(0, (int) strrpos($at, '_'))), '_')) {
            if (isset($sets[$at])) {
                $set = $sets[$at];

                break;
            }

            if (!str_contains($at, '_')) {
                break;
            }
        }

        if ($set === null || $plurals === []) {
            return [];
        }

        $texts = $plurals['rules'][$set] ?? [];

        /** @var array<string, PluralRule> */
        return Cache::locale('rules', $set, static function () use ($texts): array {
            $rules = [];

            foreach ($texts as $category => $text) {
                $rule = $category === 'other' ? null : PluralRule::parse($text);

                if ($rule !== null) {
                    $rules[$category] = $rule;
                }
            }

            return $rules;
        });
    }
}
