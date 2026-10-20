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
        'claims-core-only', 'unexpressible-declared', 'claims-no-core', 'claims-unknown-level', 'claims-no-limits',
        'tree-unoffered', 'tree-node-dropped', 'tree-node-retyped', 'tree-span-shifted', 'tree-name-raw', 'tree-cancels-inverted', 'tree-unit-changed', 'tree-unit-unknown',
    ];

    private const OTHER_ID = 'an id no case names';

    // A request the set states nowhere, so a case compared on one fails on it.
    private const OTHER_REQUEST = ['api' => 'NumberFormat', 'options' => ['style' => 'percent'], 'input' => 0];

    public static function carries(string $id): bool
    {
        return in_array($id, self::IDS, true);
    }

    public static function apply(string $id, Adapter $adapter): Mutation
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
            'unexpressible-declared' => new Mutation(
                $adapter->with(unexpressible: ['NumberFormat' => ['maximumFractionDigits']]),
                static fn(): bool => in_array('intl', $levels, true),
            ),
            'claims-no-core' => new Mutation($adapter->with(levels: array_values(array_filter($levels, static fn(string $level): bool => $level !== 'core'))), static fn(): bool => true),
            'claims-unknown-level' => new Mutation($adapter->with(levels: [...$levels, 'ecmascript']), static fn(): bool => true),
            'claims-no-limits' => new Mutation($adapter->with(limits: ['nesting' => 0] + $limits), static fn(): bool => true),
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
            'tree-unit-changed' => new Mutation(
                $cst === null ? $adapter : $adapter->with(cst: $cst->counting('code-point')),
                static fn(): bool => $cst !== null && $cst->unit !== 'code-point',
            ),
            'tree-unit-unknown' => new Mutation(
                $cst === null ? $adapter : $adapter->with(cst: $cst->counting('utf-32')),
                static fn(): bool => $cst !== null,
            ),
            default => throw new \RuntimeException('The catalogue names a defect this runner does not carry: ' . json_encode($id) . '.'),
        };
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
