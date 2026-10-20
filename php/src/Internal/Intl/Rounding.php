<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * The digits a number is shown with, as ECMA-402's
 * `SetNumberFormatDigitOptions` resolves them, and as ICU's NumberFormatter
 * is told them. `NumberFormat` and `PluralRules` read the same properties the
 * same way, so a count and the word that agrees with it are decided by one
 * reading.
 *
 * @internal
 */
final readonly class Rounding
{
    public const INCREMENTS = [1, 2, 5, 10, 20, 25, 50, 100, 200, 250, 500, 1000, 2000, 2500, 5000];

    public const MODES = ['ceil', 'floor', 'expand', 'trunc', 'halfCeil', 'halfFloor', 'halfExpand', 'halfTrunc', 'halfEven'];

    // ICU's UNumberFormatRoundingMode for each of ECMAScript's modes; the
    // half-ceiling and half-floor ones are ICU 69's.
    private const ICU_MODES = [
        'ceil' => 0, 'floor' => 1, 'trunc' => 2, 'expand' => 3, 'halfEven' => 4,
        'halfTrunc' => 5, 'halfExpand' => 6, 'halfCeil' => 9, 'halfFloor' => 10,
    ];

    /**
     * @param 'fraction'|'significant'|'morePrecision'|'lessPrecision' $type
     */
    public function __construct(
        public int $minimumIntegerDigits,
        public string $type,
        public int $minimumFractionDigits,
        public int $maximumFractionDigits,
        public int $minimumSignificantDigits,
        public int $maximumSignificantDigits,
        public int $increment,
        public string $mode,
        public bool $stripIfInteger,
    ) {}

    /**
     * The digit properties of a request, with the defaults of the facility
     * and style it is made of. An increment leaves the default maximum where
     * it is, as V8 does, so an increment over no fraction digits that
     * resolve equal is refused.
     *
     * @param array<array-key, mixed> $options
     */
    public static function read(array $options, int $fractionMinimum, int $fractionMaximum): self
    {
        $integers = EcmaOptions::number($options, 'minimumIntegerDigits', 1, 21, 1);
        $minimumFraction = $options['minimumFractionDigits'] ?? null;
        $maximumFraction = $options['maximumFractionDigits'] ?? null;
        $hasFraction = array_key_exists('minimumFractionDigits', $options) || array_key_exists('maximumFractionDigits', $options);
        $hasSignificant = array_key_exists('minimumSignificantDigits', $options) || array_key_exists('maximumSignificantDigits', $options);
        $increment = EcmaOptions::number($options, 'roundingIncrement', 1, 5000, 1);

        if (!in_array($increment, self::INCREMENTS, true)) {
            throw new \ValueError('roundingIncrement is out of range');
        }

        $mode = EcmaOptions::string($options, 'roundingMode', self::MODES, 'halfExpand');
        $priority = EcmaOptions::string($options, 'roundingPriority', ['auto', 'morePrecision', 'lessPrecision'], 'auto');
        $strip = EcmaOptions::string($options, 'trailingZeroDisplay', ['auto', 'stripIfInteger'], 'auto') === 'stripIfInteger';
        $needSignificant = $priority !== 'auto' || $hasSignificant;
        $needFraction = $priority !== 'auto' || !$hasSignificant;
        $minimumSignificant = 1;
        $maximumSignificant = 21;

        if ($needSignificant && $hasSignificant) {
            $minimumSignificant = array_key_exists('minimumSignificantDigits', $options) ? EcmaOptions::integer($options['minimumSignificantDigits'], 1, 21, 'minimumSignificantDigits') : 1;
            $maximumSignificant = array_key_exists('maximumSignificantDigits', $options) ? EcmaOptions::integer($options['maximumSignificantDigits'], $minimumSignificant, 21, 'maximumSignificantDigits') : 21;
        }

        $minimum = $fractionMinimum;
        $maximum = $fractionMaximum;

        if ($needFraction && $hasFraction) {
            $minimum = array_key_exists('minimumFractionDigits', $options) ? EcmaOptions::integer($minimumFraction, 0, 100, 'minimumFractionDigits') : null;
            $maximum = array_key_exists('maximumFractionDigits', $options) ? EcmaOptions::integer($maximumFraction, 0, 100, 'maximumFractionDigits') : null;

            if ($minimum === null) {
                $minimum = min($fractionMinimum, (int) $maximum);
            } elseif ($maximum === null) {
                $maximum = max($fractionMaximum, $minimum);
            } elseif ($minimum > $maximum) {
                throw new \ValueError('minimumFractionDigits is above maximumFractionDigits');
            }
        }

        $type = match (true) {
            $priority === 'morePrecision' => 'morePrecision',
            $priority === 'lessPrecision' => 'lessPrecision',
            $needSignificant => 'significant',
            default => 'fraction',
        };

        if ($increment !== 1) {
            if ($type !== 'fraction') {
                throw new \TypeError('roundingIncrement rounds fraction digits only');
            }

            if ($maximum !== $minimum) {
                throw new \ValueError('roundingIncrement needs as many fraction digits at least as at most');
            }
        }

        return new self($integers, $type, $minimum, (int) $maximum, $minimumSignificant, $maximumSignificant, $increment, $mode, $strip);
    }

    /**
     * The one rounding ICU performs for this number: where the properties
     * weigh significant digits against fraction digits, or strip an
     * integer's trailing zeros, the shown digits decide which applies, and
     * what is left is one ICU can be told.
     */
    public function settled(int|float $number, bool $percent = false): self
    {
        $type = $this->type;

        if ($type === 'morePrecision' || $type === 'lessPrecision') {
            $significant = $this->with('significant');
            $fraction = $this->with('fraction');
            $magnitude = Digits::exponent(Digits::shown($number, $significant, $percent)) - $this->maximumSignificantDigits + 1;
            $significantWins = $magnitude <= -$this->maximumFractionDigits;
            $type = ($type === 'morePrecision') === $significantWins ? 'significant' : 'fraction';
        }

        $settled = $this->with($type);

        if (!$this->stripIfInteger || !Digits::isInteger(Digits::shown($number, $settled, $percent))) {
            return $settled;
        }

        return new self($this->minimumIntegerDigits, $type, 0, $this->maximumFractionDigits, 1, $this->maximumSignificantDigits, $this->increment, $this->mode, false);
    }

    /** Tells a formatter these digits; a settled rounding only. */
    public function apply(\NumberFormatter $formatter): void
    {
        $formatter->setAttribute(\NumberFormatter::MIN_INTEGER_DIGITS, $this->minimumIntegerDigits);

        // An ICU before 69 has no half-ceiling or half-floor mode to set.
        if (!$formatter->setAttribute(\NumberFormatter::ROUNDING_MODE, self::ICU_MODES[$this->mode])) {
            throw new \ValueError('ICU has no such rounding mode');
        }

        if ($this->type === 'significant') {
            $formatter->setAttribute(\NumberFormatter::SIGNIFICANT_DIGITS_USED, 1);
            $formatter->setAttribute(\NumberFormatter::MAX_SIGNIFICANT_DIGITS, $this->maximumSignificantDigits);
            $formatter->setAttribute(\NumberFormatter::MIN_SIGNIFICANT_DIGITS, $this->minimumSignificantDigits);

            return;
        }

        $formatter->setAttribute(\NumberFormatter::SIGNIFICANT_DIGITS_USED, 0);
        $formatter->setAttribute(\NumberFormatter::MAX_FRACTION_DIGITS, $this->maximumFractionDigits);
        $formatter->setAttribute(\NumberFormatter::MIN_FRACTION_DIGITS, $this->minimumFractionDigits);

        // A formatter kept for another number may hold an increment; zero is
        // ICU's none.
        $formatter->setAttribute(\NumberFormatter::ROUNDING_INCREMENT, $this->increment === 1 ? 0 : $this->increment / 10 ** $this->maximumFractionDigits);
    }

    /** Whether the properties leave the rounding to the number. */
    public function isSettled(): bool
    {
        return ($this->type === 'fraction' || $this->type === 'significant') && !$this->stripIfInteger;
    }

    /** @param 'fraction'|'significant'|'morePrecision'|'lessPrecision' $type */
    private function with(string $type): self
    {
        return new self($this->minimumIntegerDigits, $type, $this->minimumFractionDigits, $this->maximumFractionDigits, $this->minimumSignificantDigits, $this->maximumSignificantDigits, $this->increment, $this->mode, false);
    }
}
