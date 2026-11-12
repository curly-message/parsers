<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * The runner of RUNNER.md: holds an adapter's claims to the format's
 * vocabulary, plans the set against them, runs it, and audits itself against
 * the catalogue of defects.
 */
final class Runner
{
    public const LEVELS = ['core', 'intl', 'extensions'];

    public const LIMITS = ['output', 'read', 'conversion', 'nesting'];

    public const APIS = ['NumberFormat', 'DateTimeFormat', 'RelativeTimeFormat', 'PluralRules'];

    // A failure line carries text derived from the payload, and an output can
    // be as long as the output limit, so what is shown is bounded.
    private const SHOWN = 200;

    private static ?Set $pinned = null;

    /** The set `conformance/set.json` pins, read once per process. */
    public static function pinned(): Set
    {
        return self::$pinned ??= Set::pinned();
    }

    /** One entry per case the adapter's levels require, each ready to run, beside the cases left out and why. */
    public static function plan(Adapter $adapter, Options $options = new Options()): Plan
    {
        self::claims($adapter, $options);
        $set = $options->fixtures ?? self::pinned()->fixtures;

        foreach ($set as $fixture) {
            if (($fixture->file->format ?? null) !== Set::FORMAT) {
                throw new \RuntimeException("The fixture file {$fixture->name} targets the format " . json_encode($fixture->file->format ?? null) . '; this runner reads ' . Set::FORMAT . '.');
            }
        }

        $cases = [];
        $skipped = [];

        foreach ($set as $fixture) {
            foreach (self::entries($adapter, $options, $fixture) as [$identity, $execute, $reason]) {
                if ($reason === null) {
                    $cases[] = new Planned($identity, $execute);
                } else {
                    $skipped[] = new Skipped($identity, $reason);
                }
            }
        }

        return new Plan($cases, $skipped);
    }

    /** Executes a plan and sorts the outcomes. */
    public static function run(Adapter $adapter, Options $options = new Options()): Result
    {
        $plan = self::plan($adapter, $options);
        $passed = [];
        $failed = [];
        $unobserved = [];

        foreach ($plan->cases as $case) {
            $outcome = $case->execute();

            if (!$outcome->ok) {
                $failed[] = new Failed($case, $outcome);

                continue;
            }

            $passed[] = $case;

            if ($outcome->unobserved !== null) {
                $unobserved[] = $case;
            }
        }

        return new Result($passed, $failed, $plan->skipped, $unobserved);
    }

    /** Runs the set and throws where anything failed, listing every failure. */
    public static function check(Adapter $adapter, Options $options = new Options()): void
    {
        $result = self::run($adapter, $options);

        if ($result->failed !== []) {
            $total = count($result->passed) + count($result->failed);

            throw new \RuntimeException(count($result->failed) . " of {$total} conformance cases failed:\n" . implode("\n", array_map(self::failure(...), $result->failed)));
        }
    }

    /** The text the command prints: one line per failure and per skipped case, then the counts. */
    public static function summarize(Result $result): string
    {
        $counts = count($result->passed) . ' passed, ' . count($result->failed) . ' failed, ' . count($result->skipped) . ' skipped';
        $unobserved = $result->unobserved === [] ? '' : ', ' . count($result->unobserved) . ' passed with reports unobserved';
        $lines = [
            ...array_map(self::failure(...), $result->failed),
            ...array_map(static fn(Skipped $case): string => "SKIP {$case->id} ({$case->where()}): {$case->reason}", $result->skipped),
            $counts . $unobserved,
        ];

        return implode("\n", $lines);
    }

    /**
     * Runs the defect catalogue against an adapter that passes the set, and
     * answers what this runner made of each defect: it caught it, it missed
     * it, or the defect never reached it (RUNNER.md, "Auditing the runner").
     *
     * @param list<DefectEntry>|null $defects the catalogue, the pinned set's where none
     * @return list<Audited>
     */
    public static function audit(Adapter $adapter, Options $options = new Options(), ?array $defects = null): array
    {
        $baseline = self::run($adapter, $options);

        if ($baseline->failed !== []) {
            $total = count($baseline->passed) + count($baseline->failed);

            throw new \RuntimeException('The audit needs an adapter that passes the set; this one fails ' . count($baseline->failed) . " of {$total} cases.");
        }

        $audited = [];

        foreach ($defects ?? self::pinned()->defects as $entry) {
            if (!Defects::carries($entry->id)) {
                throw new \RuntimeException('The catalogue names a defect this runner does not carry: ' . json_encode($entry->id) . '.');
            }

            $mutation = Defects::apply($entry->id, $adapter);

            try {
                $observed = self::verdict(self::run($mutation->adapter, $options), $baseline);
            } catch (\Throwable) {
                $observed = 'error';
            }

            $outcome = !$mutation->reached() ? 'unreachable' : ($observed === $entry->expects ? 'caught' : 'missed');
            $audited[] = new Audited($entry, $observed, $outcome);
        }

        return $audited;
    }

    // What the runner answered for a defect, read against what it answered
    // without one: a case that failed, a case left out, or a case that passed
    // with its reports unchecked, where the run without the defect had none.
    private static function verdict(Result $result, Result $baseline): string
    {
        return match (true) {
            count($result->failed) > count($baseline->failed) => 'fail',
            count($result->skipped) > count($baseline->skipped) => 'skip',
            count($result->unobserved) > count($baseline->unobserved) => 'unobserved',
            default => 'none',
        };
    }

    // The adapter's statements about itself, and the levels option, are held
    // to the vocabulary of sections 2, 11.2, 11.5 and 13, and of CST.md
    // section 4, before anything runs: a level that is not one of the three,
    // or a limit that is not a count, is an error, not a skip.
    private static function claims(Adapter $adapter, Options $options): void
    {
        $levels = $adapter->levels;

        if (!is_array($levels) || !array_is_list($levels)) {
            throw new \RuntimeException('The adapter must list the levels it claims.');
        }

        self::known($levels, 'The adapter');

        if (!in_array('core', $levels, true)) {
            throw new \RuntimeException('The adapter must claim the core level.');
        }

        if ($options->levels !== null) {
            self::known($options->levels, 'The levels option');

            foreach ($options->levels as $level) {
                if (!in_array($level, $levels, true)) {
                    throw new \RuntimeException("The adapter does not claim the {$level} level.");
                }
            }
        }

        $limits = $adapter->limits;

        foreach (self::LIMITS as $name) {
            $limit = is_array($limits) ? ($limits[$name] ?? null) : null;

            if (!is_int($limit) || $limit < 1) {
                throw new \RuntimeException("The adapter must declare its {$name} limit as a positive integer.");
            }
        }

        $declared = $adapter->unexpressible;

        if ($declared !== null) {
            if (!is_array($declared)) {
                throw new \RuntimeException('The adapter must name the properties it cannot express by the request that reads them.');
            }

            foreach ($declared as $api => $names) {
                if (!in_array($api, self::APIS, true)) {
                    throw new \RuntimeException('The adapter names the facility ' . json_encode($api) . '; the facilities are ' . implode(', ', self::APIS) . '.');
                }

                if (!is_array($names) || !array_is_list($names) || array_filter($names, is_string(...)) !== $names) {
                    throw new \RuntimeException("The adapter must name what it cannot express of a {$api} request as a list of property names.");
                }
            }
        }

        // A tree whose unit is unstated says nothing about where anything is,
        // so a tree offered without one is refused the way an unknown level is.
        if ($adapter->cst !== null && !in_array($adapter->cst->unit, Tree::UNITS, true)) {
            throw new \RuntimeException('The adapter counts its spans in ' . json_encode($adapter->cst->unit) . '; the units are ' . implode(', ', Tree::UNITS) . '.');
        }
    }

    /** @param list<mixed> $levels */
    private static function known(array $levels, string $where): void
    {
        foreach ($levels as $level) {
            if (!in_array($level, self::LEVELS, true)) {
                throw new \RuntimeException("{$where} names the level " . json_encode($level) . '; the levels are core, intl and extensions.');
            }
        }
    }

    /**
     * The cases of one file, each with its identity, how it runs, and why it
     * is left out, null where it runs.
     *
     * @return list<array{Identity, \Closure(): Outcome, string|null}>
     */
    private static function entries(Adapter $adapter, Options $options, Fixture $fixture): array
    {
        $file = $fixture->file;
        $section = is_string($file->section ?? null) ? $file->section : '';
        $entries = [];

        // A tree file declares no level, because CST.md section 2 is not one
        // of them: its cases run where the adapter offers a tree.
        $tree = ($file->kind ?? null) === 'tree';
        $level = is_string($file->level ?? null) ? $file->level : '';

        foreach (is_array($file->cases ?? null) ? $file->cases : [] as $case) {
            if (!$case instanceof \stdClass) {
                throw new \RuntimeException("The fixture file {$fixture->name} holds a case that is not an object.");
            }

            $identity = new Identity(
                is_string($case->id ?? null) ? $case->id : '',
                $fixture->name,
                $tree ? null : $level,
                $tree ? 'CST.md' : null,
                is_string($case->section ?? null) ? $case->section : $section,
                is_string($case->description ?? null) ? $case->description : '',
            );

            if ($tree) {
                $entries[] = [
                    $identity,
                    static fn(): Outcome => Tree::execute($adapter, $case),
                    $adapter->cst === null ? 'The adapter offers no concrete syntax tree.' : null,
                ];

                continue;
            }

            $performer = $options->performer;
            $entries[] = [
                $identity,
                property_exists($case, 'generate')
                    ? static fn(): Outcome => Compare::generated($adapter, $case, $performer)
                    : static fn(): Outcome => Compare::concrete($adapter, $case, $performer),
                self::exclusion($level, $adapter, $options) ?? self::unexpressible($case, $adapter),
            ];
        }

        return $entries;
    }

    // Why a level's cases are left out, or null where they run.
    private static function exclusion(string $level, Adapter $adapter, Options $options): ?string
    {
        /** @var list<string> $claimed */
        $claimed = $adapter->levels;

        if (!in_array($level, $claimed, true)) {
            return "The adapter does not claim the {$level} level.";
        }

        if ($options->levels !== null && !in_array($level, $options->levels, true)) {
            return "The {$level} level is not among the levels being run.";
        }

        return null;
    }

    // A case whose request reads a property the adapter documented it cannot
    // express (section 11.2) measures the host's facility rather than the
    // implementation, so it is left out the way an unclaimed level is.
    private static function unexpressible(\stdClass $case, Adapter $adapter): ?string
    {
        $expected = $case->expected ?? null;
        $format = $expected instanceof \stdClass ? ($expected->format ?? null) : null;

        if (property_exists($case, 'generate') || !$format instanceof \stdClass || !is_array($adapter->unexpressible)) {
            return null;
        }

        $api = is_string($format->api ?? null) ? $format->api : '';
        $cannot = $adapter->unexpressible[$api] ?? null;

        if (!is_array($cannot)) {
            return null;
        }

        $options = $format->options ?? null;

        foreach (array_keys($options instanceof \stdClass ? get_object_vars($options) : []) as $property) {
            if (in_array((string) $property, $cannot, true)) {
                return "The adapter cannot express the {$property} property of a {$api} request.";
            }
        }

        return null;
    }

    private static function failure(Failed $failed): string
    {
        $case = $failed->case;
        $outcome = $failed->outcome;

        return "FAIL {$case->id} ({$case->where()}): {$outcome->reason} Expected " . self::show($outcome->expected) . ', actual ' . self::show($outcome->actual) . ". {$case->description}";
    }

    private static function show(mixed $value): string
    {
        $text = json_encode($value, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_INVALID_UTF8_SUBSTITUTE | JSON_PARTIAL_OUTPUT_ON_ERROR);
        $text = $text === false ? get_debug_type($value) : $text;

        return strlen($text) > self::SHOWN ? Utf8::cut($text, self::SHOWN) . '...' : $text;
    }
}
