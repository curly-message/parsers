<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * The constructions of the set's README.md, "Generated cases", built from the
 * limits the adapter documents rather than from section 13's minima, or from
 * the bound it states on counted reads.
 */
final class Generators
{
    public const NAMES = [
        'output-at-limit', 'output-over-limit', 'output-over-limit-continues',
        'read-at-limit', 'read-over-limit',
        'conversion-at-limit', 'conversion-over-limit', 'conversion-over-limit-continues',
        'nesting-at-limit', 'nesting-over-limit', 'nesting-over-limit-converts', 'nesting-over-limit-reads', 'nesting-over-limit-read-spent',
        'nesting-over-limit-unknown-modifier', 'nesting-over-limit-missing-options', 'nesting-chain-over-limit',
        ...self::COUNTED, self::RESOLUTIONS,
    ];

    /** The constructions of kept reads, built from the bound on counted reads (section 13) rather than from the limits. */
    public const COUNTED = ['reentry-at-limit', 'reentry-over-limit', 'reentry-refused-not-recorded', 'reentry-own-reads-free', 'reentry-over-limit-by-a-conversion', 'reentry-own-read-in-a-nested-resolution', 'reentry-over-limit-beneath-a-defined-modifier'];

    /**
     * Those of them that make their read in a resolution a modifier asks for
     * with the bound reached, and are built from resolutions one inside
     * another where the adapter states that its bound counts them, which
     * keeps no input past the call it was handed to.
     */
    public const FRAMED = ['reentry-own-read-in-a-nested-resolution', self::DEFINED];

    /** The construction whose read is refused beneath a modifier of the format. */
    public const DEFINED = 'reentry-over-limit-beneath-a-defined-modifier';

    /**
     * The construction built from that bound where the adapter states that
     * it counts a resolution asked for inside another, which keeps no input.
     */
    public const RESOLUTIONS = 'reentry-resolutions-over-limit';

    /** Every generated case reports under this id. */
    public const ID = 'limits';

    // A value of the stated length the placeholder selects nothing from: the
    // read budget is spent whole and two characters reach the output.
    private const READS = '{{v:eq; nomatch:X; default:ok;}}';

    // A value converted whole and compared against a key its text cannot
    // equal: the option is chosen only where the value was converted, so a
    // conversion abandoned shows in the output as the default.
    private const CONVERTS = '{{v:ne; nomatch:ok; default:refused;}}';

    // How many levels past the nesting limit the chain construction nests.
    private const CHAIN = 20000;

    // A placeholder that reads the input kept last that no `take` has
    // claimed. Its inline default stands for a `take` that failed.
    private const TAKE = '{{x:take; default:F}}';

    // A placeholder whose modifier answers a value that reads the input kept
    // last that no `take` has claimed, as it is converted.
    private const TAKING = '{{x:taking; default:F}}';

    // A placeholder whose modifier keeps its default unread and asks for a
    // resolution in which the conversion of an answer reads it.
    private const KEEP_NEST = '{{x:keep-nest; default:z}}';

    // One whose modifier does the same, of a resolution in which that
    // conversion runs while a modifier the format defines produces the option
    // it selected, between brackets. Its default holds another: where the
    // bound counts the resolution, the read of the first default is one past
    // it; where it counts reads alone, that read takes the last place and
    // produces the second placeholder, whose default is then read one past
    // the bound. Either read is refused, and the text is the same.
    private const KEEP_NEST_NE = '<{{x:keep-nest-ne; default:{{x:keep-nest-ne; default:z}}}}>';

    public static function build(string $name, Adapter $adapter): Prepared
    {
        /** @var array{output: int, read: int, conversion: int, nesting: int} $limits */
        $limits = $adapter->limits;
        $absent = $adapter->absent;
        $bound = is_int($adapter->reentry) ? $adapter->reentry : 0;
        $input = static fn(string $message, \stdClass $payload): Resolution => new Resolution($message, $payload, $absent, $absent, self::ID, [], $absent);
        $output = $limits['output'];
        $read = $limits['read'];
        $conversion = $limits['conversion'];
        $nesting = $limits['nesting'];
        $unserializable = static fn(): mixed => Decode::value((object) [Decode::TAG => 'unserializable'], $absent);

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
            'conversion-at-limit' => new Prepared(
                $input(self::CONVERTS, (object) ['v' => self::visiting($conversion, $absent)]),
                new Expectation('ok', null, null, []),
            ),
            'conversion-over-limit' => new Prepared(
                $input('{{v; default:D}}', (object) ['v' => self::visiting($conversion + 1, $absent)]),
                new Expectation('D', null, null, [self::unserializable()]),
            ),
            // One node past the limit, each element long enough that the text
            // a finished serialization would write runs past the read budget:
            // a refused serialization produces no text, so it spends none,
            // and the placeholder after it is resolved and carried.
            'conversion-over-limit-continues' => new Prepared(
                $input('{{v; default:D}}{{w}}', (object) ['v' => array_fill(0, $conversion, str_repeat('x', intdiv($read + $conversion - 1, $conversion) + 1)), 'w' => 'tail']),
                new Expectation('Dtail', null, null, [self::unserializable()]),
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
            // Section 9 decides the nesting limit after the value is looked
            // up and converted, so a placeholder nested too deep still
            // converts its value, reports what the conversion reports and
            // spends the read budget, and one reached with the budget spent
            // resolves to the empty string however deeply it nests.
            'nesting-over-limit-converts' => new Prepared(
                $input(self::nest($nesting + 1, '{{w; default:fallback;}}'), (object) ['v' => 'a', 'w' => $unserializable()]),
                new Expectation('fallback', null, null, [self::unserializable(), self::limit('nesting-limit', 'message', $nesting)]),
            ),
            'nesting-over-limit-reads' => new Prepared(
                $input(self::nest($nesting + 1, '{{w; default:fallback;}}') . '{{v}}', (object) ['v' => 'a', 'w' => str_repeat('x', $read + 1)]),
                new Expectation('fallback', null, null, [self::limit('nesting-limit', 'message', $nesting), self::limit('read-limit', 'limit', $read)]),
            ),
            'nesting-over-limit-read-spent' => new Prepared(
                $input(self::nest($nesting + 1, '{{w:eq; nomatch:X;}}{{v; default:fallback;}}'), (object) ['v' => 'a', 'w' => str_repeat('x', $read + 1)]),
                new Expectation('', null, null, [self::limit('nesting-limit', 'message', $nesting), self::limit('read-limit', 'limit', $read)]),
            ),
            // The nesting limit is the first of the message errors, and a
            // placeholder that meets one meets none after it.
            'nesting-over-limit-unknown-modifier' => new Prepared(
                $input(self::nest($nesting + 1, '{{w:zz; default:fallback;}}'), (object) ['v' => 'a', 'w' => 'x']),
                new Expectation('fallback', null, null, [self::limit('nesting-limit', 'message', $nesting)]),
            ),
            'nesting-over-limit-missing-options' => new Prepared(
                $input(self::nest($nesting + 1, '{{w:eq; default:fallback;}}'), (object) ['v' => 'a', 'w' => 'x']),
                new Expectation('fallback', null, null, [self::limit('nesting-limit', 'message', $nesting)]),
            ),
            // A refused placeholder takes its chain, whose last link is its
            // inline default, so each level past the limit is refused and
            // reported once and the walk carries on to the innermost text.
            'nesting-chain-over-limit' => new Prepared(
                $input(str_repeat('{{v; default:', $nesting + self::CHAIN) . 'x' . str_repeat('}}', $nesting + self::CHAIN), new \stdClass()),
                new Expectation('x', null, null, array_fill(0, self::CHAIN, self::limit('nesting-limit', 'message', $nesting))),
            ),
            // As many reads as the bound lets stand at once: the innermost
            // `take` finds nothing left to claim.
            'reentry-at-limit' => self::reentering($adapter, self::keeps($bound) . self::TAKE, 'none'),
            // One read more: it is refused, produces nothing and answers the
            // empty string, and the `take` that made it goes on with it.
            'reentry-over-limit' => self::reentering($adapter, self::refusing($bound), $bound > 0 ? '[empty]' : 'empty'),
            // The refused read was not recorded, so a read of the same input
            // afterwards, with no other read in progress, produces it. Where
            // the bound is 0, that read is refused as well.
            'reentry-refused-not-recorded' => self::reentering($adapter, self::refusing($bound) . '{{x:first; default:F}}', $bound > 0 ? '[empty]none' : 'emptyempty'),
            // The innermost of as many counted reads as the bound lets stand
            // produces a stack of own reads as deep as the nesting limit
            // leaves room for: they are the walk's and count toward no bound.
            'reentry-own-reads-free' => self::reentering($adapter, self::keeps(1, self::own(max($nesting - 1, 1))) . self::keeps($bound - 1) . self::TAKE, 'z'),
            // One read more, made by the conversion of an answer rather than
            // by a modifier: it is refused, and the conversion goes on with
            // the empty string.
            'reentry-over-limit-by-a-conversion' => self::reentering($adapter, self::refusing($bound, self::TAKING), $bound > 0 ? '[empty]' : 'empty', ['keep', 'take', 'first', 'taking']),
            // With the bound reached, a modifier keeps its default and asks for
            // a resolution, where the conversion of an answer reads that
            // default. The modifier is current there, so the read is its own:
            // a counted read there would be one past the bound.
            'reentry-own-read-in-a-nested-resolution' => self::standing($adapter, $bound, self::KEEP_NEST, 'keep-nest', 'z'),
            // The same, with that conversion run while a modifier the format
            // defines produces the option it selected: that modifier is the
            // current one, so the read is no modifier's own, is counted one
            // past the bound and is refused, and the conversion goes on with
            // the empty string.
            self::DEFINED => self::standing($adapter, $bound, self::KEEP_NEST_NE, 'keep-nest-ne', '<empty>'),
            // One resolution more than the bound lets stand inside the one
            // asked for: it is refused, the modifier that asked fails in the
            // innermost resolution standing, and reports there. Only where
            // the bound is 0 is that the resolution the case asks for.
            self::RESOLUTIONS => new Prepared(
                new Resolution(Behaviours::RENESTED, (object) ['x' => ''], $absent, $absent, self::ID, ['renest' => Behaviours::renesting($adapter, $bound)], $absent),
                new Expectation(self::renested($bound), null, null, $bound > 0 ? [] : [['code' => 'failed-modifier', 'origin' => 'payload', 'id' => self::ID]]),
            ),
            default => throw new \RuntimeException('The set names no construction ' . json_encode($name) . '.'),
        };
    }

    /** The output of `reentry-resolutions-over-limit` at a bound: one resolution more than the bound stands, each writing its brackets. */
    public static function renested(int $bound): string
    {
        return str_repeat('[', $bound + 1) . 'F' . str_repeat(']', $bound + 1);
    }

    // An output of the stated length written entirely by the message: the
    // placeholder selects an option, and an option value is the message's own
    // text, so what reaches the output is the placeholder's result while the
    // payload was read for one character.
    private static function writes(int $length): string
    {
        return '{{v; a:' . str_repeat('x', $length) . ';}}';
    }

    /**
     * A value whose serialization visits the stated number of nodes and
     * writes little text. Section 13 counts a member the walk visits and then
     * omits, and a value reached twice twice: the root holds `s` references
     * to one object of `s - 1` members holding the host's undefined, each
     * reference visiting `s` nodes, and enough members holding undefined
     * itself to make up the rest.
     */
    private static function visiting(int $nodes, mixed $absent): \stdClass
    {
        $side = (int) floor(sqrt($nodes - 1));
        $shared = new \stdClass();
        $root = new \stdClass();

        for ($index = 0; $index < $side - 1; $index += 1) {
            $shared->{"k{$index}"} = $absent;
        }

        for ($index = 0; $index < $side; $index += 1) {
            $root->{"s{$index}"} = $shared;
        }

        for ($index = 0; $index < $nodes - 1 - $side * $side; $index += 1) {
            $root->{"u{$index}"} = $absent;
        }

        return $root;
    }

    // A message nesting the stated number of levels, each selecting the option
    // that holds the next. The innermost declares a fallback, so a level the
    // nesting limit refuses is visible as the chain it takes.
    private static function nest(int $levels, string $innermost = '{{v; a:settled; default:fallback;}}'): string
    {
        $message = $innermost;

        for ($level = 2; $level <= $levels; $level += 1) {
            $message = "{{v; a:{$message};}}";
        }

        return $message;
    }

    // Placeholders that each keep their default unread, the default taking
    // the input kept before it. A `take` after them claims the last one kept
    // and reads it, and each read takes the one kept before, so the reads
    // stand one inside another as deep as the placeholders kept, while the
    // message nests two levels.
    private static function keeps(int $count, string $body = self::TAKE): string
    {
        return str_repeat("{{x:keep; default:{$body}}}", max($count, 0));
    }

    // One input more than the bound lets be read at once. The read past it is
    // made by the placeholder in the second input's default, written between
    // brackets; where the bound is 0 it is the placeholder after them.
    private static function refusing(int $bound, string $reader = self::TAKE): string
    {
        return $bound > 0
            ? self::keeps(1) . self::keeps(1, "[{$reader}]") . self::keeps($bound - 1) . self::TAKE
            : self::keeps(1) . $reader;
    }

    // The stated number of levels of own reads: each placeholder's modifier
    // reads its own default, which holds the next, and the innermost answers
    // `z`.
    private static function own(int $levels): string
    {
        return str_repeat('{{x:read; default:', $levels) . 'z' . str_repeat('}}', $levels);
    }

    // `$inner` resolved with one place fewer taken than the bound lets
    // stand, so that the resolution its modifier asks for takes the last
    // where the bound counts resolutions. Where the adapter states that it
    // does, the places are resolutions one inside another, the innermost of
    // `$inner`. Otherwise they are counted reads, the innermost producing
    // `$inner`, and the bound may count the resolution or not: the adapter
    // need not say.
    private static function standing(Adapter $adapter, int $bound, string $inner, string $keeper, string $output): Prepared
    {
        if ($adapter->resolutions === true) {
            $absent = $adapter->absent;

            return new Prepared(
                new Resolution($bound > 1 ? Behaviours::DESCEND : $inner, (object) ['x' => ''], $absent, $absent, self::ID, Behaviours::descending($adapter, $bound - 1, $inner), $absent),
                new Expectation($output, null, null, []),
            );
        }

        return self::reentering($adapter, $bound > 1 ? self::keeps(1, $inner) . self::keeps($bound - 2) . self::TAKE : $inner, $output, ['keep', 'take', 'first', 'taking', $keeper]);
    }

    /** @param list<string> $keepers */
    private static function reentering(Adapter $adapter, string $message, string $output, array $keepers = ['keep', 'take', 'first']): Prepared
    {
        $kept = Behaviours::keeping($adapter);
        $absent = $adapter->absent;
        $modifiers = ['read' => Behaviours::named('default')];

        foreach ($keepers as $name) {
            $modifiers[$name] = $kept[$name];
        }

        return new Prepared(
            new Resolution($message, (object) ['x' => ''], $absent, $absent, self::ID, $modifiers, $absent),
            new Expectation($output, null, null, []),
        );
    }

    /** @return array{code: string, origin: string, id: string, limit: int} */
    private static function limit(string $code, string $origin, int $declared): array
    {
        return ['code' => $code, 'origin' => $origin, 'id' => self::ID, 'limit' => $declared];
    }

    /** @return array{code: string, origin: string, id: string} */
    private static function unserializable(): array
    {
        return ['code' => 'unserializable-value', 'origin' => 'payload', 'id' => self::ID];
    }
}
