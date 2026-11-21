<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** The catalogue of the set's README.md, each behaviour a function of section 11's inputs. */
final class Behaviours
{
    public const NAMES = ['upper', 'echo', 'empty', 'nothing', 'raise', 'default', 'options', 'props', 'locale', 'object'];

    /** The behaviours over one list of what was kept, which a case registers under `keeping`. */
    public const KEEPING = ['keep', 'keep-default', 'take', 'first', 'taking', 'nest', 'keep-nest', 'keep-nest-ne'];

    /** The behaviours over that list that ask for a resolution inside another. */
    public const NESTS = ['nest', 'keep-nest', 'keep-nest-ne'];

    /** The message `descend` asks a resolution of while it has further to go. */
    public const DESCEND = '{{x:descend; default:F}}';

    /** The message `renest` asks a resolution of: the one it is registered in. */
    public const RENESTED = '[{{x:renest; default:F}}]';

    /**
     * The behaviours that read a kept input only after the modifier that kept
     * it returned: `keep` reads nothing it keeps, so only a later modifier
     * does, and `first` reads only what an earlier one kept.
     */
    public const LATER = ['keep', 'first'];

    // The message `nest` asks a resolution of, the one `keep-nest` does, and
    // the one `keep-nest-ne` does, whose `ne` is the format's own: `x` holds
    // the empty string, which differs from `b`, so it selects that option and
    // produces it during its call.
    private const NESTED = '{{x:take; default:F}}';

    private const NESTED_TAKING = '{{x:taking; default:F}}';

    private const NESTED_DEFINED = '{{x:ne; b:{{x:taking; default:F}}}}';

    /** @return \Closure(BehaviourInput): mixed */
    public static function named(string $name): \Closure
    {
        return match ($name) {
            // ASCII letters only, as the catalogue states it.
            'upper' => static fn(BehaviourInput $in): string => strtoupper($in->value),
            'echo' => static fn(BehaviourInput $in): string => $in->value,
            'empty' => static fn(BehaviourInput $in): string => '',
            // PHP's nothing is null, as ECMAScript's is undefined.
            'nothing' => static fn(BehaviourInput $in): mixed => null,
            'raise' => static fn(BehaviourInput $in): never => throw new \RuntimeException('The raise behaviour raised.'),
            'default' => static fn(BehaviourInput $in): string => $in->default(),
            'options' => static fn(BehaviourInput $in): string => implode(',', array_map(
                static fn(BehaviourOption $option): string => "{$option->key}={$option->value()}",
                $in->options,
            )),
            'props' => static function (BehaviourInput $in): string {
                $props = $in->props;
                ksort($props, SORT_STRING);
                $written = [];

                foreach ($props as $name => $value) {
                    $written[] = $name . '=' . json_encode($value, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_THROW_ON_ERROR);
                }

                return implode(',', $written);
            },
            'locale' => static fn(BehaviourInput $in): string => $in->locale ?? 'none',
            'object' => static fn(BehaviourInput $in): array => ['answer' => $in->value],
            default => throw new \RuntimeException('The set names no behaviour ' . json_encode($name) . '.'),
        };
    }

    /**
     * The behaviours of the catalogue over one list of what was kept. Each
     * call builds them afresh, so what one resolution kept is read by no
     * other but the one a `nest`, a `keep-nest` or a `keep-nest-ne` among
     * them asks `$adapter` for while its call is in progress, handing it
     * these same behaviours: a runner calls it once per resolution it asks
     * for, with the adapter it asks.
     *
     * @return array<string, \Closure(BehaviourInput): mixed>
     */
    public static function keeping(Adapter $adapter): array
    {
        return self::over($adapter, null);
    }

    /**
     * Those behaviours with the `descend` of the catalogue beside them, which
     * asks `$adapter` for `$hops` resolutions one inside another, each handed
     * these same behaviours and `descend`, the innermost of them of `$inner`.
     *
     * @return array<string, \Closure(BehaviourInput): mixed>
     */
    public static function descending(Adapter $adapter, int $hops, string $inner): array
    {
        return self::over($adapter, [$hops, $inner]);
    }

    /**
     * @param array{int, string}|null $descent where a `descend` leads: through so many resolutions, the innermost of that message
     *
     * @return array<string, \Closure(BehaviourInput): mixed>
     */
    private static function over(Adapter $adapter, ?array $descent): array
    {
        $kept = new Kept();
        $absent = $adapter->absent;
        $record = [
            'keep' => static function (BehaviourInput $in) use ($kept): string {
                $kept->keep($in->default(...));

                return '';
            },
            'keep-default' => static function (BehaviourInput $in) use ($kept): string {
                $kept->keep($in->default(...));

                return $in->default();
            },
            'take' => static fn(BehaviourInput $in): string => $kept->take(),
            'first' => static fn(BehaviourInput $in): string => $kept->first(),
            // A `Stringable`, which the host's ordinary string conversion
            // (section 4) converts by running `__toString`, so the read is
            // made by the conversion of the answer, after the call returned.
            'taking' => static fn(BehaviourInput $in): \Stringable => new class ($kept) implements \Stringable {
                public function __construct(private readonly Kept $kept) {}

                public function __toString(): string
                {
                    return $this->kept->take();
                }
            },
        ];
        // The output of a resolution of `$message` asked of the adapter, over
        // these same behaviours.
        $nested = static function (string $message) use ($adapter, &$record, $absent): string {
            $resolved = $adapter->resolve(new Resolution($message, (object) ['x' => ''], $absent, $absent, $absent, $record, $absent));
            $output = is_array($resolved) ? ($resolved['output'] ?? null) : null;

            return is_string($output) ? $output : throw new \RuntimeException('The resolution nest asked for answered no output.');
        };
        $record['nest'] = static fn(BehaviourInput $in): string => $nested(self::NESTED);
        $record['keep-nest'] = static function (BehaviourInput $in) use ($kept, $nested): string {
            $kept->keep($in->default(...));

            return $nested(self::NESTED_TAKING);
        };
        $record['keep-nest-ne'] = static function (BehaviourInput $in) use ($kept, $nested): string {
            $kept->keep($in->default(...));

            return $nested(self::NESTED_DEFINED);
        };

        if ($descent !== null) {
            [$hops, $inner] = $descent;
            $descending = 0;
            // With fewer calls in progress than the descent has hops, the next
            // hop; with as many, the message the descent leads to.
            $record['descend'] = static function (BehaviourInput $in) use ($nested, $hops, $inner, &$descending): string {
                $descending += 1;

                try {
                    return $nested($descending < $hops ? self::DESCEND : $inner);
                } finally {
                    $descending -= 1;
                }
            };
        }

        return $record;
    }

    /**
     * The `renest` of the set's README.md, which asks `$adapter` for a
     * resolution of the message it is registered in, with itself registered,
     * and answers its output. It raises where that resolution answered none,
     * which is how an adapter shows one its implementation refused. One level
     * past what `$bound` lets stand it asks for nothing and answers
     * `unrefused`, so an implementation that refuses nothing fails the case
     * in place of nesting until its host ends it.
     *
     * @return \Closure(BehaviourInput): string
     */
    public static function renesting(Adapter $adapter, int $bound): \Closure
    {
        $absent = $adapter->absent;
        $standing = 0;
        $renest = static function (BehaviourInput $in) use ($adapter, $bound, $absent, &$standing, &$renest): string {
            if ($standing > $bound) {
                return 'unrefused';
            }

            $standing += 1;

            try {
                $resolved = $adapter->resolve(new Resolution(self::RENESTED, (object) ['x' => ''], $absent, $absent, $absent, ['renest' => $renest], $absent));
                $output = is_array($resolved) ? ($resolved['output'] ?? null) : null;

                return is_string($output) ? $output : throw new \RuntimeException('The resolution renest asked for answered no output.');
            } finally {
                $standing -= 1;
            }
        };

        return $renest;
    }
}
