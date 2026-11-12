<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

use CurlyMessage\Internal\Text;
use CurlyMessage\Parser;

/**
 * Options read as ECMA-402 reads them, since a request is stated in its
 * vocabulary (SPEC.md section 11.2): `GetOption` with ECMAScript's own
 * coercions, written over PHP values. A PHP list is an ECMAScript array and
 * an associative array or a `stdClass` an ordinary object; any other object
 * has no ECMAScript counterpart and is refused. What ECMAScript answers with a
 * `RangeError` throws `\ValueError` here, and a `TypeError` `\TypeError`: both
 * are a request the host refuses.
 *
 * @internal
 */
final class EcmaOptions
{
    /**
     * `GetOption` of type string.
     *
     * @param array<array-key, mixed> $options
     * @param list<string>|null $values the values allowed, or null for any
     *
     * @return ($fallback is null ? string|null : string)
     */
    public static function string(array $options, string $name, ?array $values, ?string $fallback): ?string
    {
        if (!array_key_exists($name, $options)) {
            return $fallback;
        }

        $value = self::toString($options[$name]);

        if ($values !== null && !in_array($value, $values, true)) {
            throw new \ValueError(sprintf('%s is out of range', $name));
        }

        return $value;
    }

    /**
     * `GetOption` of type boolean.
     *
     * @param array<array-key, mixed> $options
     */
    public static function boolean(array $options, string $name): ?bool
    {
        return array_key_exists($name, $options) ? self::toBoolean($options[$name]) : null;
    }

    /**
     * `GetNumberOption`: an integer from `$minimum` to `$maximum`.
     *
     * @param array<array-key, mixed> $options
     *
     * @return ($fallback is null ? int|null : int)
     */
    public static function number(array $options, string $name, int $minimum, int $maximum, ?int $fallback): ?int
    {
        return array_key_exists($name, $options) ? self::integer($options[$name], $minimum, $maximum, $name) : $fallback;
    }

    /** `DefaultNumberOption` over a value that is present. */
    public static function integer(mixed $value, int $minimum, int $maximum, string $name): int
    {
        $number = self::toNumber($value);

        if (is_nan($number) || $number < $minimum || $number > $maximum) {
            throw new \ValueError(sprintf('%s is out of range', $name));
        }

        return (int) floor($number);
    }

    /**
     * `GetBooleanOrStringNumberFormatOption` as `useGrouping` asks it: true
     * is `$true`, anything falsy is false, `"true"` and `"false"` are the
     * fallback, and any other text must be one of `$values`.
     *
     * @param array<array-key, mixed> $options
     * @param list<string> $values
     */
    public static function booleanOrString(array $options, string $name, array $values, string $true, string $fallback): string|false
    {
        if (!array_key_exists($name, $options)) {
            return $fallback;
        }

        $value = $options[$name];

        if ($value === true) {
            return $true;
        }

        if (!self::toBoolean($value)) {
            return false;
        }

        $text = self::toString($value);

        if ($text === 'true' || $text === 'false') {
            return $fallback;
        }

        if (!in_array($text, $values, true)) {
            throw new \ValueError(sprintf('%s is out of range', $name));
        }

        return $text;
    }

    /** `ToBoolean`. */
    public static function toBoolean(mixed $value): bool
    {
        return match (true) {
            $value === null, $value === false, $value === '' => false,
            is_int($value) => $value !== 0,
            is_float($value) => $value !== 0.0 && !is_nan($value),
            default => true,
        };
    }

    /**
     * `ToString`.
     *
     * @param int $length the bytes a list's text holds at most; a test writes to a shorter one
     */
    public static function toString(mixed $value, int $length = Parser::MAX_TEXT_LENGTH): string
    {
        if (is_array($value) && array_is_list($value)) {
            return self::join($value, $length);
        }

        return self::primitive($value);
    }

    /** `ToNumber`. */
    public static function toNumber(mixed $value): float
    {
        return match (true) {
            is_int($value), is_float($value) => (float) $value,
            $value === null, $value === false => 0.0,
            $value === true => 1.0,
            is_string($value) => self::stringToNumber($value),
            is_array($value) && array_is_list($value) => self::stringToNumber(self::join($value, Parser::MAX_TEXT_LENGTH)),
            is_array($value), $value instanceof \stdClass && get_class($value) === \stdClass::class => NAN,
            default => throw new \TypeError('The value has no ECMAScript counterpart'),
        };
    }

    /**
     * ECMAScript's `Number::toString`: the shortest decimal that reads back
     * as the same number, in plain notation from 1e-7 to below 1e21 and in
     * exponent notation outside it.
     */
    public static function numberToString(float $number): string
    {
        if (is_nan($number)) {
            return 'NaN';
        }

        if (is_infinite($number)) {
            return $number > 0 ? 'Infinity' : '-Infinity';
        }

        if ($number === 0.0) {
            return '0';
        }

        $sign = $number < 0 ? '-' : '';
        $magnitude = abs($number);

        // Seventeen significant digits name every double, so the loop ends.
        $precision = 0;

        do {
            $written = strtr(sprintf('%.' . $precision . 'e', $magnitude), ',', '.');
            $precision += 1;
        } while ($precision < 17 && (float) $written !== $magnitude);

        [$mantissa, $exponent] = explode('e', $written);
        $digits = rtrim(str_replace('.', '', $mantissa), '0');
        $digits = $digits === '' ? '0' : $digits;
        $count = strlen($digits);
        $point = (int) $exponent + 1;

        if ($count <= $point && $point <= 21) {
            return $sign . $digits . str_repeat('0', $point - $count);
        }

        if (0 < $point && $point <= 21) {
            return $sign . substr($digits, 0, $point) . '.' . substr($digits, $point);
        }

        if (-6 < $point && $point <= 0) {
            return $sign . '0.' . str_repeat('0', -$point) . $digits;
        }

        $shown = $point - 1;

        return $sign . $digits[0] . ($count > 1 ? '.' . substr($digits, 1) : '') . 'e' . ($shown < 0 ? '-' : '+') . abs($shown);
    }

    /** Whether text is a `type` of UTS #35: one or more subtags of 3 to 8 letters and digits, joined by `-`. */
    public static function isType(string $text): bool
    {
        if ($text === '') {
            return false;
        }

        foreach (explode('-', $text) as $subtag) {
            $length = strlen($subtag);

            if ($length < 3 || $length > 8 || strspn(strtolower($subtag), 'abcdefghijklmnopqrstuvwxyz0123456789') !== $length) {
                return false;
            }
        }

        return true;
    }

    /** `IsWellFormedCurrencyCode`: three ASCII letters. */
    public static function isCurrency(string $text): bool
    {
        return strlen($text) === 3 && strspn(strtolower($text), 'abcdefghijklmnopqrstuvwxyz') === 3;
    }

    /**
     * `ToString` of a value that is no list: what an ECMAScript primitive is
     * written as, and what an ordinary object is.
     */
    private static function primitive(mixed $value): string
    {
        return match (true) {
            is_string($value) => $value,
            is_int($value) => (string) $value,
            is_float($value) => self::numberToString($value),
            is_bool($value) => $value ? 'true' : 'false',
            $value === null => 'null',
            is_array($value), $value instanceof \stdClass && get_class($value) === \stdClass::class => '[object Object]',
            default => throw new \TypeError('The value has no ECMAScript counterpart'),
        };
    }

    /**
     * `Array.prototype.join` over a list, nested lists included, on a stack
     * of its own rather than PHP's, and bounded as a serialization is: in
     * the values it visits, and in its text, which a list sharing one long
     * string would otherwise write out many times over. A text past its
     * length is refused before anything that would carry it there is
     * copied, as the JavaScript parser's engine refuses a string past its
     * longest.
     *
     * @param list<mixed> $list
     */
    private static function join(array $list, int $length): string
    {
        $out = '';
        $stack = [[$list, 0]];
        $visited = 0;

        while ($stack !== []) {
            $top = count($stack) - 1;
            [$items, $index] = $stack[$top];

            if ($index >= count($items)) {
                array_pop($stack);

                continue;
            }

            $stack[$top][1] = $index + 1;
            $item = $items[$index];
            $visited += 1;

            if ($visited > Parser::MAX_CONVERSION_NODES) {
                throw new \ValueError('The list is too large to read');
            }

            $nested = is_array($item) && array_is_list($item);
            $text = $nested || $item === null ? '' : self::primitive($item);

            if (($index > 0 ? 1 : 0) + strlen($text) > $length - strlen($out)) {
                throw new \ValueError('The list is too long to read');
            }

            $out .= $index > 0 ? ',' : '';

            if ($nested) {
                $stack[] = [$item, 0];

                continue;
            }

            $out .= $text;
        }

        return $out;
    }

    /**
     * `StringToNumber`: the whole text, whitespace aside, as a decimal, an
     * infinity, or a binary, octal or hexadecimal integer; anything else is
     * NaN. Read byte by byte, with no pattern.
     */
    private static function stringToNumber(string $text): float
    {
        $from = 0;
        $to = strlen($text);

        while ($from < $to && ($size = Text::blankAt($text, $from, $to)) > 0) {
            $from += $size;
        }

        while ($to > $from && ($size = Text::blankBefore($text, $to, $from)) > 0) {
            $to -= $size;
        }

        $body = substr($text, $from, $to - $from);

        if ($body === '') {
            return 0.0;
        }

        $unsigned = $body[0] === '+' || $body[0] === '-' ? substr($body, 1) : $body;
        $negative = $body[0] === '-';

        if ($unsigned === 'Infinity') {
            return $negative ? -INF : INF;
        }

        if ($unsigned === $body && strlen($body) > 2 && $body[0] === '0') {
            $radix = match ($body[1]) {
                'x', 'X' => 16,
                'o', 'O' => 8,
                'b', 'B' => 2,
                default => 0,
            };

            if ($radix !== 0) {
                return self::radix(substr($body, 2), $radix);
            }
        }

        return self::isDecimal($unsigned) ? (float) $body : NAN;
    }

    private static function radix(string $digits, int $radix): float
    {
        $value = 0.0;
        $alphabet = substr('0123456789abcdef', 0, $radix);

        foreach (str_split(strtolower($digits)) as $digit) {
            $at = strpos($alphabet, $digit);

            if ($at === false) {
                return NAN;
            }

            $value = $value * $radix + $at;
        }

        return $value;
    }

    // StrUnsignedDecimalLiteral: digits with an optional fraction, or a
    // fraction alone, then an optional exponent.
    private static function isDecimal(string $text): bool
    {
        $length = strlen($text);
        $at = strspn($text, '0123456789');
        $whole = $at;
        $fraction = 0;

        if ($at < $length && $text[$at] === '.') {
            $fraction = strspn($text, '0123456789', $at + 1);
            $at += 1 + $fraction;
        }

        if ($whole === 0 && $fraction === 0) {
            return false;
        }

        if ($at < $length && ($text[$at] === 'e' || $text[$at] === 'E')) {
            $at += 1;

            if ($at < $length && ($text[$at] === '+' || $text[$at] === '-')) {
                $at += 1;
            }

            $exponent = strspn($text, '0123456789', $at);

            if ($exponent === 0) {
                return false;
            }

            $at += $exponent;
        }

        return $at === $length;
    }
}
