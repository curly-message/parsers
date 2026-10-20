<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * The eight constructions of the set's README.md, "Generated cases", built
 * from the limits the adapter documents rather than from section 13's minima.
 */
final class Generators
{
    public const NAMES = ['output-at-limit', 'output-over-limit', 'output-over-limit-continues', 'read-at-limit', 'read-over-limit', 'conversion-over-limit', 'nesting-at-limit', 'nesting-over-limit'];

    /** Every generated case reports under this id. */
    public const ID = 'limits';

    // A value of the stated length the placeholder selects nothing from: the
    // read budget is spent whole and two characters reach the output.
    private const READS = '{{v:eq; nomatch:X; default:ok;}}';

    /** @param array{output: int, read: int, conversion: int, nesting: int} $limits */
    public static function build(string $name, array $limits, mixed $absent): Prepared
    {
        $input = static fn(string $message, \stdClass $payload): Resolution => new Resolution($message, $payload, $absent, $absent, self::ID, [], $absent);
        $output = $limits['output'];
        $read = $limits['read'];
        $nesting = $limits['nesting'];

        return match ($name) {
            'output-at-limit' => new Prepared(
                $input(self::writes($output), (object) ['v' => 'a']),
                new Expectation(str_repeat('x', $output), null, null, []),
            ),
            // The placeholder resolves to the empty string and the message's
            // own text still renders: that text is the caller's, and is
            // counted against no limit.
            'output-over-limit' => new Prepared(
                $input('A' . self::writes($output + 1) . 'B', (object) ['v' => 'a']),
                new Expectation('AB', null, null, [self::limit('output-limit', 'limit', $output)]),
            ),
            // A result the output has no room for spends nothing, so the
            // placeholder after it is resolved and carried.
            'output-over-limit-continues' => new Prepared(
                $input(self::writes($output + 1) . '{{w}}', (object) ['v' => 'a', 'w' => 'tail']),
                new Expectation('tail', null, null, [self::limit('output-limit', 'limit', $output)]),
            ),
            'read-at-limit' => new Prepared(
                $input(self::READS, (object) ['v' => str_repeat('x', $read)]),
                new Expectation('ok', null, null, []),
            ),
            // The budget is tested before a placeholder reads, so the one
            // that spent it past the limit still resolves and the one after
            // it pays.
            'read-over-limit' => new Prepared(
                $input(self::READS . '{{w}}', (object) ['v' => str_repeat('x', $read + 1), 'w' => 'tail']),
                new Expectation('ok', null, null, [self::limit('read-limit', 'limit', $read)]),
            ),
            'conversion-over-limit' => new Prepared(
                $input('{{v; default:D}}', (object) ['v' => Decode::nodes($limits['conversion'] + 1)]),
                new Expectation('D', null, null, [['code' => 'unserializable-value', 'origin' => 'payload', 'id' => self::ID]]),
            ),
            'nesting-at-limit' => new Prepared(
                $input(self::nest($nesting), (object) ['v' => 'a']),
                new Expectation('settled', null, null, []),
            ),
            // The innermost placeholder is the one refused, and a nesting
            // limit is a defect of the message: it takes its fallback chain,
            // and every level around it selects the option that holds it.
            'nesting-over-limit' => new Prepared(
                $input(self::nest($nesting + 1), (object) ['v' => 'a']),
                new Expectation('fallback', null, null, [self::limit('nesting-limit', 'message', $nesting)]),
            ),
            default => throw new \RuntimeException('The set names no construction ' . json_encode($name) . '.'),
        };
    }

    // An output of the stated length written entirely by the message: the
    // placeholder selects an option, and an option value is the message's own
    // text, so what reaches the output is the placeholder's result while the
    // payload was read for one character.
    private static function writes(int $length): string
    {
        return '{{v; a:' . str_repeat('x', $length) . ';}}';
    }

    // A message nesting the stated number of levels, each selecting the option
    // that holds the next. The innermost declares a fallback, so a level the
    // nesting limit refuses is visible as the chain it takes.
    private static function nest(int $levels): string
    {
        $message = '{{v; a:settled; default:fallback;}}';

        for ($level = 2; $level <= $levels; $level += 1) {
            $message = "{{v; a:{$message};}}";
        }

        return $message;
    }

    /** @return array{code: string, origin: string, id: string, limit: int} */
    private static function limit(string $code, string $origin, int $declared): array
    {
        return ['code' => $code, 'origin' => $origin, 'id' => self::ID, 'limit' => $declared];
    }
}
