<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * ECMAScript's `Intl.NumberFormat`, performed on ICU's NumberFormatter: the
 * options read as ECMA-402 reads them, the digits as `Rounding` settles
 * them, and the grouping ECMAScript's `auto` asks for emulated, since ICU's
 * minimum grouping digits cannot be set from PHP.
 *
 * @internal
 */
final class NumberFormat
{
    // UNumberFormatStyle values PHP names no constant for.
    private const CURRENCY_ISO = 10;

    private const CURRENCY_PLURAL = 11;

    private const CURRENCY_STANDARD = 16;

    private static ?\NumberFormatter $currencies = null;

    /**
     * @param int $grouping the integer digits from which grouping shows, 0 where it never does
     */
    private function __construct(
        private readonly \NumberFormatter $formatter,
        private readonly Rounding $rounding,
        private readonly int $grouping,
        private readonly bool $percent,
    ) {}

    /**
     * @param array<array-key, mixed> $options
     */
    public static function build(LocaleTag $tag, array $options): self
    {
        $numbering = $tag->numbering($options);
        $style = EcmaOptions::string($options, 'style', ['decimal', 'percent', 'currency', 'unit'], 'decimal');
        $currency = EcmaOptions::string($options, 'currency', null, null);

        if ($currency !== null && !EcmaOptions::isCurrency($currency)) {
            throw new \ValueError('currency is not a currency code');
        }

        if ($currency === null && $style === 'currency') {
            throw new \TypeError('The currency style needs a currency');
        }

        $currency = $currency === null ? null : strtoupper($currency);
        $display = EcmaOptions::string($options, 'currencyDisplay', ['code', 'symbol', 'narrowSymbol', 'name'], 'symbol');

        [$minimum, $maximum] = match (true) {
            $style === 'currency' && $currency !== null => array_fill(0, 2, self::currencyDigits($currency)),
            $style === 'percent' => [0, 0],
            default => [0, 3],
        };

        $rounding = Rounding::read($options, $minimum, $maximum);
        $grouping = EcmaOptions::booleanOrString($options, 'useGrouping', ['min2', 'auto', 'always'], 'always', 'auto');

        $formatter = new \NumberFormatter($tag->icu(['numbers' => $numbering]), match (true) {
            $style === 'percent' => \NumberFormatter::PERCENT,
            $style !== 'currency' => \NumberFormatter::DECIMAL,
            $display === 'code' => self::CURRENCY_ISO,
            $display === 'name' => self::CURRENCY_PLURAL,
            default => self::CURRENCY_STANDARD,
        });

        if ($style === 'currency' && $currency !== null) {
            // The code resets the digits to the currency's, so it is set first.
            $formatter->setTextAttribute(\NumberFormatter::CURRENCY_CODE, $currency);
            $narrow = $display === 'narrowSymbol' ? IcuData::narrowSymbol($tag->base, $currency) : null;

            if ($narrow !== null) {
                $formatter->setSymbol(\NumberFormatter::CURRENCY_SYMBOL, $narrow);
            }
        }

        if ($rounding->isSettled()) {
            $rounding->apply($formatter);
        }

        // ICU shows a separator once the integer digits pass the group's
        // size; ECMAScript's `auto` waits for the locale's minimum grouping
        // digits beyond it, and `min2` for two.
        $size = $formatter->getAttribute(\NumberFormatter::GROUPING_SIZE);
        $least = match ($grouping) {
            false => 0,
            'always' => 1,
            'min2' => 2,
            default => IcuData::minimumGrouping($tag->base),
        };

        $formatter->setAttribute(\NumberFormatter::GROUPING_USED, $least === 0 ? 0 : 1);

        return new self($formatter, $rounding, $least > 1 && $size > 0 ? $size + $least : 0, $style === 'percent');
    }

    public function format(int|float $number): string
    {
        $rounding = $this->rounding;

        if (!$rounding->isSettled()) {
            $rounding = $rounding->settled($number, $this->percent);
            $rounding->apply($this->formatter);
        }

        if ($this->grouping > 0) {
            // The digits that decide are the ones shown, after rounding: 9999.6
            // at no fraction digits is shown with five.
            $integer = Digits::parts(Digits::shown($number, $rounding, $this->percent))[0];
            $this->formatter->setAttribute(\NumberFormatter::GROUPING_USED, strlen($integer) >= $this->grouping ? 1 : 0);
        }

        $written = $this->formatter->format($number);

        if (!is_string($written)) {
            throw new \ValueError('The number cannot be formatted');
        }

        return $written;
    }

    /** The digits ICU gives a currency, which are ISO 4217's minor units, 2 for a code it does not know. */
    private static function currencyDigits(string $currency): int
    {
        self::$currencies ??= new \NumberFormatter('en', \NumberFormatter::CURRENCY);
        self::$currencies->setTextAttribute(\NumberFormatter::CURRENCY_CODE, $currency);

        return self::$currencies->getAttribute(\NumberFormatter::MAX_FRACTION_DIGITS);
    }
}
