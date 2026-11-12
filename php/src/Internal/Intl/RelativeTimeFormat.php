<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * ECMAScript's `Intl.RelativeTimeFormat`, which ext-intl has no binding for,
 * performed from ICU's own relative-time fields the way ICU's
 * `RelativeDateTimeFormatter` (what V8 calls) performs it: a phrase where
 * `numeric` is `auto` and the locale has one for the offset, and otherwise
 * the pattern for the direction and the count's plural category, with the
 * count formatted as the locale's decimal format writes it.
 *
 * @internal
 */
final readonly class RelativeTimeFormat
{
    // The offsets ICU's relative phrases are written for, by a hundredth of
    // the offset rounded half away from zero, as ICU allows them a 1% margin.
    private const PHRASES = [-200 => '-2', -100 => '-1', 0 => '0', 100 => '1', 200 => '2'];

    private function __construct(
        private string $locale,
        private string $style,
        private bool $auto,
        private NumberFormat $number,
        private PluralRules $plural,
    ) {}

    /**
     * @param array<array-key, mixed> $options
     */
    public static function build(LocaleTag $tag, array $options): self
    {
        $numbering = $tag->numbering($options);
        $style = EcmaOptions::string($options, 'style', ['long', 'short', 'narrow'], 'long');
        $numeric = EcmaOptions::string($options, 'numeric', ['always', 'auto'], 'always');
        // ICU's own decimal format: at most three fraction digits, rounded
        // half to even, and the locale's grouping.
        $digits = ['maximumFractionDigits' => 3, 'roundingMode' => 'halfEven'];

        return new self(
            $tag->base,
            $style,
            $numeric === 'auto',
            NumberFormat::build($tag, $digits + ($numbering === null ? [] : ['numberingSystem' => $numbering])),
            PluralRules::build($tag, $digits),
        );
    }

    public function format(int|float $count, string $unit): string
    {
        if (!is_finite((float) $count)) {
            throw new \ValueError('The count is not finite');
        }

        if (!in_array($unit, IcuData::UNITS, true)) {
            throw new \ValueError('The unit is not one a relative time is stated in');
        }

        if ($this->auto && $count > -2.1 && $count < 2.1) {
            $hundredths = $count * 100;
            $rounded = (int) ($hundredths < 0 ? $hundredths - 0.5 : $hundredths + 0.5);
            $offset = self::PHRASES[$rounded] ?? null;

            // A second has a phrase for the present alone, ICU's "now".
            if ($offset !== null && ($unit !== 'second' || $offset === '0')) {
                $phrase = IcuData::field($this->locale, $unit, $this->style, 'relative', $offset);

                if ($phrase !== null) {
                    return $phrase;
                }
            }
        }

        // The sign bit decides, so negative zero is in the past.
        $past = $count < 0 || (is_float($count) && $count === 0.0 && fdiv(1.0, $count) < 0);
        $magnitude = abs($count);
        $category = $this->plural->select($magnitude);
        $direction = $past ? 'past' : 'future';
        $pattern = IcuData::field($this->locale, $unit, $this->style, $direction, $category)
            ?? IcuData::field($this->locale, $unit, $this->style, $direction, 'other');

        if ($pattern === null) {
            throw new \ValueError('The locale has no pattern for the unit');
        }

        return self::apply($pattern, $this->number->format($magnitude));
    }

    /**
     * A pattern of ICU's SimpleFormatter with its one argument in place: an
     * apostrophe before a brace quotes up to the next apostrophe, two
     * apostrophes are one, and any other apostrophe is itself.
     */
    private static function apply(string $pattern, string $argument): string
    {
        $out = '';
        $length = strlen($pattern);
        $quoted = false;

        for ($at = 0; $at < $length; $at += 1) {
            $byte = $pattern[$at];

            if ($byte === "'") {
                $next = $pattern[$at + 1] ?? '';

                if ($next === "'") {
                    $out .= "'";
                    $at += 1;
                } elseif ($quoted) {
                    $quoted = false;
                } elseif ($next === '{' || $next === '}') {
                    $quoted = true;
                } else {
                    $out .= "'";
                }

                continue;
            }

            if (!$quoted && $byte === '{') {
                $close = strpos($pattern, '}', $at);

                if ($close !== false && $close > $at + 1 && strspn($pattern, '0123456789', $at + 1, $close - $at - 1) === $close - $at - 1) {
                    $out .= $argument;
                    $at = $close;

                    continue;
                }
            }

            $out .= $byte;
        }

        return $out;
    }
}
