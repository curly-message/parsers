<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * One plural rule of CLDR's syntax (UTS #35, Part 3, "Plural rules syntax"),
 * as ICU's data writes it: `i = 1 and v = 0 @integer 1`. It is read once by a
 * hand-written loop and kept, and asked of a number's shown digits, so a
 * number as long as a hundred fraction digits is read as written rather than
 * as a float.
 *
 * @internal
 *
 * @phpstan-type Relation array{operand: string, modulus: int, negated: bool, within: bool, ranges: list<array{int, int}>}
 */
final readonly class PluralRule
{
    /** @param list<list<Relation>> $conditions any of them, each all of its relations */
    private function __construct(
        private array $conditions,
    ) {}

    /** The rule written before its samples, or null where it is not one the syntax admits. */
    public static function parse(string $text): ?self
    {
        $at = strpos($text, '@');
        $tokens = self::tokens($at === false ? $text : substr($text, 0, $at));

        if ($tokens === null) {
            return null;
        }

        if ($tokens === []) {
            return new self([[]]);
        }

        $conditions = [];
        $relations = [];
        $count = count($tokens);
        $index = 0;

        while (true) {
            $relation = self::relation($tokens, $index);

            if ($relation === null) {
                return null;
            }

            [$relations[], $index] = $relation;

            if ($index === $count) {
                $conditions[] = $relations;

                return new self($conditions);
            }

            if ($tokens[$index] === 'or') {
                $conditions[] = $relations;
                $relations = [];
            } elseif ($tokens[$index] !== 'and') {
                return null;
            }

            $index += 1;
        }
    }

    /**
     * Whether the rule holds for a number shown with these digits, unsigned.
     */
    public function holds(string $integer, string $fraction): bool
    {
        foreach ($this->conditions as $relations) {
            $all = true;

            foreach ($relations as $relation) {
                if (!self::satisfied($relation, $integer, $fraction)) {
                    $all = false;

                    break;
                }
            }

            if ($all) {
                return true;
            }
        }

        return false;
    }

    /** @param Relation $relation */
    private static function satisfied(array $relation, string $integer, string $fraction): bool
    {
        $trimmed = rtrim($fraction, '0');
        // An operand's value: its integer digits, and whether a fraction
        // follows them, which only `n` carries.
        [$digits, $fractional] = match ($relation['operand']) {
            'n' => [$integer, $trimmed !== ''],
            'i' => [$integer, false],
            'v' => [(string) strlen($fraction), false],
            'w' => [(string) strlen($trimmed), false],
            'f' => [$fraction === '' ? '0' : $fraction, false],
            't' => [$trimmed === '' ? '0' : $trimmed, false],
            default => ['0', false],
        };

        if ($relation['modulus'] > 0) {
            $remainder = 0;
            $length = strlen($digits);

            for ($at = 0; $at < $length; $at += 1) {
                $remainder = ($remainder * 10 + ord($digits[$at]) - 0x30) % $relation['modulus'];
            }

            $digits = (string) $remainder;
        }

        $digits = ltrim($digits, '0');
        // Past eighteen digits a value is above every bound the rules name.
        $value = $digits === '' ? 0 : (strlen($digits) > 18 ? PHP_INT_MAX : (int) $digits);
        $found = false;

        foreach ($relation['ranges'] as [$low, $high]) {
            $inside = $relation['within']
                ? $value >= $low && ($value < $high || ($value === $high && !$fractional))
                : !$fractional && $value >= $low && $value <= $high;

            if ($inside) {
                $found = true;

                break;
            }
        }

        return $found !== $relation['negated'];
    }

    /**
     * @param list<string> $tokens
     *
     * @return array{Relation, int}|null
     */
    private static function relation(array $tokens, int $index): ?array
    {
        $count = count($tokens);
        $operand = $tokens[$index] ?? '';

        if (!in_array($operand, ['n', 'i', 'f', 't', 'v', 'w', 'c', 'e'], true)) {
            return null;
        }

        $index += 1;
        $modulus = 0;

        if ($index < $count && ($tokens[$index] === 'mod' || $tokens[$index] === '%')) {
            $divisor = $tokens[$index + 1] ?? '';

            if (!self::isValue($divisor) || (int) $divisor === 0) {
                return null;
            }

            $modulus = (int) $divisor;

            $index += 2;
        }

        $negated = false;
        $within = false;
        $word = $tokens[$index] ?? '';
        $index += 1;

        if ($word === 'is') {
            if (($tokens[$index] ?? '') === 'not') {
                $negated = true;
                $index += 1;
            }
        } elseif ($word === '!=') {
            $negated = true;
        } elseif ($word === 'not') {
            $negated = true;
            $word = $tokens[$index] ?? '';
            $index += 1;
            $within = $word === 'within';

            if ($word !== 'in' && $word !== 'within') {
                return null;
            }
        } elseif ($word === 'within') {
            $within = true;
        } elseif ($word !== '=' && $word !== 'in') {
            return null;
        }

        $ranges = [];

        while (true) {
            $low = $tokens[$index] ?? '';

            if (!self::isValue($low)) {
                return null;
            }

            $index += 1;
            $high = $low;

            if (($tokens[$index] ?? '') === '..') {
                $high = $tokens[$index + 1] ?? '';

                if (!self::isValue($high)) {
                    return null;
                }

                $index += 2;
            }

            $ranges[] = [(int) $low, (int) $high];

            if (($tokens[$index] ?? '') !== ',' || $word === 'is') {
                break;
            }

            $index += 1;
        }

        return [['operand' => $operand, 'modulus' => $modulus, 'negated' => $negated, 'within' => $within, 'ranges' => $ranges], $index];
    }

    /**
     * The rule's words, numbers and symbols, or null at a character the
     * syntax has no place for.
     *
     * @return list<string>|null
     */
    private static function tokens(string $text): ?array
    {
        $tokens = [];
        $length = strlen($text);
        $at = 0;

        while ($at < $length) {
            $byte = $text[$at];

            if ($byte === ' ' || $byte === "\t" || $byte === "\n" || $byte === "\r") {
                $at += 1;

                continue;
            }

            $run = strspn($text, 'abcdefghijklmnopqrstuvwxyz', $at);

            if ($run === 0) {
                $run = strspn($text, '0123456789', $at);
            }

            if ($run === 0) {
                $run = match (true) {
                    substr($text, $at, 2) === '..', substr($text, $at, 2) === '!=' => 2,
                    $byte === '=' || $byte === ',' || $byte === '%' => 1,
                    default => 0,
                };
            }

            if ($run === 0) {
                return null;
            }

            $tokens[] = substr($text, $at, $run);
            $at += $run;
        }

        return $tokens;
    }

    // A value the syntax writes, short enough to be an integer here.
    private static function isValue(string $token): bool
    {
        $length = strlen($token);

        return $length > 0 && $length <= 18 && strspn($token, '0123456789') === $length;
    }
}
