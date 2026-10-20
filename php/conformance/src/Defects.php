<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * The catalogue's defects as this runner carries them: each wraps a
 * conforming adapter into one that is wrong in exactly one way, and says
 * whether that way ever changed what the adapter answered.
 */
final class Defects
{
    public const IDS = [
        'output-truncated', 'output-trimmed', 'answers-nothing', 'raises',
        'reports-dropped', 'reports-extra', 'reports-reversed', 'report-code-changed', 'report-origin-changed', 'report-id-changed', 'report-limit-changed',
        'formats-wrong', 'reports-unobserved',
        'claims-core-only', 'unexpressible-declared', 'unexpressible-conditional-declared', 'claims-no-core', 'claims-unknown-level', 'claims-no-limits',
        'conversion-limit-overstated', 'conversion-limit-understated', 'reentry-limit-overstated', 'reentry-limit-understated', 'claims-negative-reentry',
        'resolutions-overstated', 'claims-non-boolean-resolutions',
        'tree-unoffered', 'tree-node-dropped', 'tree-node-retyped', 'tree-span-shifted', 'tree-name-raw', 'tree-cancels-inverted', 'tree-unit-changed', 'tree-unit-unknown',
    ];

    private const OTHER_ID = 'an id no case names';

    // A request the set states nowhere, so a case compared on one fails on it.
    private const OTHER_REQUEST = ['api' => 'NumberFormat', 'options' => ['style' => 'percent'], 'input' => 0];

    public static function carries(string $id): bool
    {
        return in_array($id, self::IDS, true);
    }

    /**
     * A defect realized for the run an audit makes: the fixture files it
     * reads, and the levels whose cases it runs.
     *
     * @param list<Fixture> $set
     * @param list<string> $run
     */
    public static function apply(string $id, Adapter $adapter, array $set, array $run): Mutation
    {
        /** @var list<string> $levels */
        $levels = $adapter->levels;
        /** @var array<string, int> $limits */
        $limits = $adapter->limits;
        $cst = $adapter->cst;

        return match ($id) {
            'output-truncated' => self::answering($adapter, static fn(array $resolved): ?array => is_string($resolved['output'] ?? null) && $resolved['output'] !== ''
                ? ['output' => Utf8::dropLast($resolved['output'])] + $resolved
                : null),
            // trim()'s own set, and only where it changed something.
            'output-trimmed' => self::answering($adapter, static fn(array $resolved): ?array => is_string($resolved['output'] ?? null) && trim($resolved['output']) !== $resolved['output']
                ? ['output' => trim($resolved['output'])] + $resolved
                : null),
            'answers-nothing' => self::instead($adapter, static fn(Resolution $input): mixed => null),
            'raises' => self::instead($adapter, static fn(Resolution $input): never => throw new \RuntimeException('The raises defect raised.')),
            'reports-dropped' => self::reporting($adapter, static fn(array $reports): ?array => $reports === [] ? null : []),
            'reports-extra' => self::reporting($adapter, static fn(array $reports): array => [...$reports, ['code' => 'unknown-modifier']]),
            'reports-reversed' => self::reporting($adapter, static function (array $reports): ?array {
                $reversed = array_reverse($reports);

                return self::alike($reports, $reversed) ? null : $reversed;
            }),
            'report-code-changed' => self::reporting($adapter, static fn(array $reports): ?array => $reports === [] ? null : array_map(
                static fn(mixed $report): mixed => self::set($report, 'code', Compare::field($report, 'code') === 'unknown-modifier' ? 'failed-modifier' : 'unknown-modifier'),
                $reports,
            )),
            'report-origin-changed' => self::reporting($adapter, static fn(array $reports): ?array => self::some($reports, 'origin', static fn(mixed $origin): bool => $origin !== null)
                ? array_map(static fn(mixed $report): mixed => Compare::field($report, 'origin') === null ? $report : self::set($report, 'origin', Compare::field($report, 'origin') === 'message' ? 'payload' : 'message'), $reports)
                : null),
            'report-id-changed' => self::reporting($adapter, static fn(array $reports): ?array => self::some($reports, 'id', static fn(mixed $id): bool => $id !== null)
                ? array_map(static fn(mixed $report): mixed => Compare::field($report, 'id') === null ? $report : self::set($report, 'id', self::OTHER_ID), $reports)
                : null),
            'report-limit-changed' => self::reporting($adapter, static fn(array $reports): ?array => self::some($reports, 'limit', static fn(mixed $limit): bool => is_int($limit) || is_float($limit))
                ? array_map(static function (mixed $report): mixed {
                    $limit = Compare::field($report, 'limit');

                    return is_int($limit) || is_float($limit) ? self::set($report, 'limit', $limit + 1) : $report;
                }, $reports)
                : null),
            // The one defect that answers what the implementation produced and
            // states something else about it: the output stands, and only the
            // request is wrong, so a runner that ignores the request answers
            // nothing at all.
            'formats-wrong' => new Mutation(
                $adapter->with(resolve: static function (Resolution $input) use ($adapter): mixed {
                    $resolved = $adapter->resolve($input);

                    return is_array($resolved) ? ['formats' => [self::OTHER_REQUEST]] + $resolved : $resolved;
                }),
                static fn(): bool => in_array('intl', $levels, true),
            ),
            'reports-unobserved' => self::answering($adapter, static function (array $resolved): ?array {
                if (!array_key_exists('reports', $resolved)) {
                    return null;
                }

                unset($resolved['reports']);

                return $resolved;
            }),
            'claims-core-only' => new Mutation($adapter->with(levels: ['core']), static fn(): bool => count($levels) > 1),
            'unexpressible-declared' => self::declaring($adapter, 'maximumFractionDigits', $set, $run),
            'unexpressible-conditional-declared' => self::declaring($adapter, ['property' => 'maximumFractionDigits', 'values' => [2], 'when' => ['useGrouping' => false]], $set, $run),
            'claims-no-core' => new Mutation($adapter->with(levels: array_values(array_filter($levels, static fn(string $level): bool => $level !== 'core'))), static fn(): bool => true),
            'claims-unknown-level' => new Mutation($adapter->with(levels: [...$levels, 'ecmascript']), static fn(): bool => true),
            'claims-no-limits' => new Mutation($adapter->with(limits: ['nesting' => 0] + $limits), static fn(): bool => true),
            // Only the construction at the limit visits a node more than the
            // limit the implementation imposes; past it, either limit refuses.
            'conversion-limit-overstated' => self::converting($adapter, 1, ['conversion-at-limit'], $set, $run),
            // Only the constructions past the limit then visit no node more
            // than the implementation's limit; at it, either lets it through.
            'conversion-limit-understated' => self::converting($adapter, -1, ['conversion-over-limit', 'conversion-over-limit-continues'], $set, $run),
            // Every construction that stands at the bound then asks for one
            // read more than the implementation lets stand: at the bound it
            // pins what that read answers, and past it which `take` the
            // refusal reaches.
            'reentry-limit-overstated' => self::reentering($adapter, 1, ['reentry-at-limit', 'reentry-over-limit', 'reentry-refused-not-recorded', 'reentry-own-reads-free', 'reentry-over-limit-by-a-conversion', 'reentry-own-read-in-a-nested-resolution', Generators::DEFINED], $set, $run),
            // Only the constructions past the bound then let one read through
            // that the case expects refused; at it, either bound lets every
            // read through.
            'reentry-limit-understated' => self::reentering($adapter, -1, ['reentry-over-limit', 'reentry-refused-not-recorded', 'reentry-over-limit-by-a-conversion', Generators::DEFINED], $set, $run),
            'claims-negative-reentry' => new Mutation($adapter->with(reentry: -1), static fn(): bool => true),
            // The construction of resolutions one inside another then runs
            // over an implementation that refuses none of them, and the one
            // whose read is refused beneath a modifier of the format is built
            // from resolutions that take no place, so above a bound of 1 its
            // read is let through. An adapter that states the claim itself
            // carries nothing to alter; the first is left out all the same
            // where it states no bound or its output limit does not hold it,
            // and the second shows nothing where it states none, 0 or 1.
            'resolutions-overstated' => new Mutation(
                $adapter->with(resolutions: true),
                static fn(): bool => $adapter->resolutions !== true && is_int($adapter->reentry)
                    && self::holds($set, $run, static fn(\stdClass $case): bool => match ($case->generate ?? null) {
                        Generators::RESOLUTIONS => strlen(Generators::renested($adapter->reentry)) <= $limits['output'],
                        Generators::DEFINED => $adapter->reentry > 1,
                        default => false,
                    }),
            ),
            'claims-non-boolean-resolutions' => new Mutation($adapter->with(resolutions: 'true'), static fn(): bool => true),
            'tree-unoffered' => new Mutation($adapter->with(cst: null), static fn(): bool => $cst !== null),
            'tree-node-dropped' => self::parsing($adapter, static function (mixed $tree): mixed {
                $children = Tree::children($tree);

                return $children === [] ? null : self::set($tree, 'nodes', array_slice(array_values($children), 0, -1));
            }),
            'tree-node-retyped' => self::everywhere($adapter, static fn(mixed $node, string $message): mixed => Compare::field($node, 'type') === 'separator' ? self::set($node, 'type', 'text') : $node),
            'tree-span-shifted' => self::everywhere($adapter, static function (mixed $node, string $message): mixed {
                $start = Compare::field($node, 'start');

                return Compare::field($node, 'type') === 'placeholder' && is_int($start) ? self::set($node, 'start', $start + 1) : $node;
            }),
            'tree-name-raw' => self::everywhere($adapter, static function (mixed $node, string $message) use ($cst): mixed {
                if ($cst === null || !in_array(Compare::field($node, 'type'), Tree::NAMED, true)) {
                    return $node;
                }

                $spelled = Tree::spelling($message, $cst->unit, $node);

                return $spelled === null || $spelled === Compare::field($node, 'name') ? $node : self::set($node, 'name', $spelled);
            }),
            'tree-cancels-inverted' => self::everywhere($adapter, static fn(mixed $node, string $message): mixed => Compare::field($node, 'type') === 'escape' ? self::set($node, 'cancels', Compare::field($node, 'cancels') !== true) : $node),
            // A declaration of the adapter's own unit would alter nothing, so
            // a code-point adapter is misdeclared as UTF-16 and every other
            // as code points.
            'tree-unit-changed' => new Mutation(
                $cst === null ? $adapter : $adapter->with(cst: $cst->counting($cst->unit === 'code-point' ? 'utf-16' : 'code-point')),
                static fn(): bool => $cst !== null,
            ),
            'tree-unit-unknown' => new Mutation(
                $cst === null ? $adapter : $adapter->with(cst: $cst->counting('utf-32')),
                static fn(): bool => $cst !== null,
            ),
            default => throw new \RuntimeException('The catalogue names a defect this runner does not carry: ' . json_encode($id) . '.'),
        };
    }

    /**
     * A defect of what the adapter declares it cannot express: one entry more
     * than the adapter declared itself, so the gaps it documents stay left
     * out. It reaches the runner where a case of the run states a request the
     * entry covers and no entry of the adapter's own covers: a case only a
     * runner that reads the entry leaves out.
     *
     * @param list<Fixture> $set
     * @param list<string> $run
     */
    private static function declaring(Adapter $adapter, mixed $entry, array $set, array $run): Mutation
    {
        $declared = is_array($adapter->unexpressible) ? $adapter->unexpressible : [];
        $own = is_array($declared['NumberFormat'] ?? null) ? array_values($declared['NumberFormat']) : [];
        $leaves = static function (\stdClass $options) use ($entry, $own): bool {
            if (Unexpressible::covers($entry, 'NumberFormat', $options) === null) {
                return false;
            }

            foreach ($own as $declared) {
                if (Unexpressible::covers($declared, 'NumberFormat', $options) !== null) {
                    return false;
                }
            }

            return true;
        };

        return new Mutation(
            $adapter->with(unexpressible: ['NumberFormat' => [...$own, $entry]] + $declared),
            static fn(): bool => self::holds($set, $run, static function (\stdClass $case) use ($leaves): bool {
                $expected = $case->expected ?? null;
                $format = $expected instanceof \stdClass ? ($expected->format ?? null) : null;

                if (property_exists($case, 'generate') || !$format instanceof \stdClass || ($format->api ?? null) !== 'NumberFormat') {
                    return false;
                }

                $options = $format->options ?? null;

                return $leaves($options instanceof \stdClass ? $options : new \stdClass());
            }),
        );
    }

    /**
     * A defect of the conversion limit the adapter declares. The limit is
     * read only by the constructions of section 13 that build from it, so the
     * defect reaches the runner where the run holds one of the stated
     * constructions at a level it runs.
     *
     * @param list<string> $constructions
     * @param list<Fixture> $set
     * @param list<string> $run
     */
    private static function converting(Adapter $adapter, int $by, array $constructions, array $set, array $run): Mutation
    {
        /** @var array<string, int> $limits */
        $limits = $adapter->limits;

        return new Mutation(
            $adapter->with(limits: ['conversion' => $limits['conversion'] + $by] + $limits),
            static fn(): bool => self::generates($set, $run, $constructions),
        );
    }

    /**
     * A defect of the bound on counted reads the adapter states. An adapter
     * that states none, or states 0 where the defect would state one lower,
     * carries nothing to alter. Otherwise the bound is read only by the
     * constructions built from it. Those built from kept reads run where the
     * adapter states that a modifier can keep an input. The two built inside
     * a resolution a modifier asks for run at a bound above 0, and from
     * resolutions, whatever the adapter states of keeping, where it states
     * that its bound counts them. The one whose read is refused shows a
     * bound stated one lower however it is built, by a read let through, and
     * one stated one higher only built from resolutions: built from kept
     * reads it stands a place short of the bound where the bound counts reads
     * alone, which the adapter need not say. The other answers the same text
     * where the resolution is refused, and shows only a bound stated one
     * higher, built from resolutions, in the report of the modifier that
     * asked, which reaches the runner where that modifier sits in the
     * resolution the case asks for, at a stated bound of 1, and the adapter
     * observes reports. So the defect reaches the runner where the run holds,
     * at a level it runs, one of the stated constructions as it is built for
     * this adapter.
     *
     * @param list<string> $constructions
     * @param list<Fixture> $set
     * @param list<string> $run
     */
    private static function reentering(Adapter $adapter, int $by, array $constructions, array $set, array $run): Mutation
    {
        $declared = is_int($adapter->reentry) ? $adapter->reentry + $by : -1;
        $counts = $adapter->resolutions === true;
        $observes = false;
        $resolve = static function (Resolution $input) use ($adapter, &$observes): mixed {
            $resolved = $adapter->resolve($input);
            $observes = $observes || (is_array($resolved) && array_key_exists('reports', $resolved));

            return $resolved;
        };
        $shows = static function (string $generate) use ($constructions, $declared, $counts, $by, $adapter, &$observes): bool {
            if (!in_array($generate, $constructions, true)) {
                return false;
            }

            if (!in_array($generate, Generators::FRAMED, true)) {
                return $adapter->keeps !== false;
            }

            return $declared > 0 && ($generate === Generators::DEFINED
                ? $counts || ($by < 0 && $adapter->keeps !== false)
                : $counts && $by > 0 && $declared === 1 && $observes);
        };

        return new Mutation(
            $declared < 0 ? $adapter->with(resolve: $resolve) : $adapter->with(resolve: $resolve, reentry: $declared),
            static fn(): bool => $declared >= 0 && self::holds($set, $run, static fn(\stdClass $case): bool => is_string($case->generate ?? null) && $shows($case->generate)),
        );
    }

    /**
     * Whether the run holds a case built by one of the constructions.
     *
     * @param list<Fixture> $set
     * @param list<string> $run
     * @param list<string> $constructions
     */
    private static function generates(array $set, array $run, array $constructions): bool
    {
        return self::holds($set, $run, static fn(\stdClass $case): bool => in_array($case->generate ?? null, $constructions, true));
    }

    /**
     * Whether a case of the run, at a level it runs, meets the condition.
     *
     * @param list<Fixture> $set
     * @param list<string> $run
     * @param \Closure(\stdClass): bool $meets
     */
    private static function holds(array $set, array $run, \Closure $meets): bool
    {
        foreach ($set as $fixture) {
            $file = $fixture->file;

            if (($file->kind ?? null) === 'tree' || !in_array($file->level ?? null, $run, true)) {
                continue;
            }

            foreach (is_array($file->cases ?? null) ? $file->cases : [] as $case) {
                if ($case instanceof \stdClass && $meets($case)) {
                    return true;
                }
            }
        }

        return false;
    }

    /**
     * A defect that changes what the adapter answers. `$change` answers the
     * altered resolution, or null where this one held nothing to alter, which
     * is what `reached` is read from.
     *
     * @param \Closure(array<array-key, mixed>): (array<array-key, mixed>|null) $change
     */
    private static function answering(Adapter $adapter, \Closure $change): Mutation
    {
        $reached = false;

        return new Mutation(
            $adapter->with(resolve: static function (Resolution $input) use ($adapter, $change, &$reached): mixed {
                $resolved = $adapter->resolve($input);
                $changed = is_array($resolved) ? $change($resolved) : null;

                if ($changed === null) {
                    return $resolved;
                }

                $reached = true;

                return $changed;
            }),
            static function () use (&$reached): bool {
                return $reached;
            },
        );
    }

    /**
     * A defect of the reports, which an implementation need not observe at all.
     *
     * @param \Closure(list<mixed>): (list<mixed>|null) $change
     */
    private static function reporting(Adapter $adapter, \Closure $change): Mutation
    {
        return self::answering($adapter, static function (array $resolved) use ($change): ?array {
            $reports = $resolved['reports'] ?? null;

            if (!is_array($reports) || !array_is_list($reports)) {
                return null;
            }

            $changed = $change($reports);

            return $changed === null ? null : ['reports' => $changed] + $resolved;
        });
    }

    /** A defect that answers nothing of what the implementation produced. */
    private static function instead(Adapter $adapter, \Closure $resolve): Mutation
    {
        return new Mutation($adapter->with(resolve: $resolve), static fn(): bool => true);
    }

    /**
     * A defect of the tree, which an implementation need not offer at all: an
     * adapter that offers none carries nothing to alter, so the defect is
     * unreachable rather than missed.
     *
     * @param \Closure(mixed, string): mixed $change answers the altered tree, or null where it altered nothing
     */
    private static function parsing(Adapter $adapter, \Closure $change): Mutation
    {
        $cst = $adapter->cst;

        if ($cst === null) {
            return new Mutation($adapter, static fn(): bool => false);
        }

        $reached = false;

        return new Mutation(
            $adapter->with(cst: $cst->parsing(static function (string $message) use ($cst, $change, &$reached): mixed {
                $tree = $cst->parse($message);
                $changed = $change($tree, $message);

                if ($changed === null) {
                    return $tree;
                }

                $reached = true;

                return $changed;
            })),
            static function () use (&$reached): bool {
                return $reached;
            },
        );
    }

    /**
     * A defect applied to every node it recognizes, which reaches the runner
     * where it recognized at least one.
     *
     * @param \Closure(mixed, string): mixed $change
     */
    private static function everywhere(Adapter $adapter, \Closure $change): Mutation
    {
        return self::parsing($adapter, static function (mixed $tree, string $message) use ($change): mixed {
            $altered = false;
            $apply = static function (mixed $node) use (&$apply, &$altered, $change, $message): mixed {
                $changed = $change($node, $message);

                if ($changed !== $node) {
                    $altered = true;
                }

                $nodes = Compare::field($changed, 'nodes');

                return is_array($nodes) ? self::set($changed, 'nodes', array_map($apply, $nodes)) : $changed;
            };
            $result = $apply($tree);

            return $altered ? $result : null;
        });
    }

    /** A copy of a node or a report with one field set, as an array or as an object alike. */
    private static function set(mixed $value, string $name, mixed $field): mixed
    {
        if (is_array($value)) {
            $value[$name] = $field;

            return $value;
        }

        if ($value instanceof \stdClass) {
            $copy = clone $value;
            $copy->{$name} = $field;

            return $copy;
        }

        return $value;
    }

    /**
     * @param list<mixed> $reports
     * @param \Closure(mixed): bool $holds
     */
    private static function some(array $reports, string $name, \Closure $holds): bool
    {
        foreach ($reports as $report) {
            if ($holds(Compare::field($report, $name))) {
                return true;
            }
        }

        return false;
    }

    /**
     * @param list<mixed> $reports
     * @param list<mixed> $other
     */
    private static function alike(array $reports, array $other): bool
    {
        foreach ($reports as $index => $report) {
            if (json_encode($report, JSON_PARTIAL_OUTPUT_ON_ERROR) !== json_encode($other[$index], JSON_PARTIAL_OUTPUT_ON_ERROR)) {
                return false;
            }
        }

        return true;
    }
}
