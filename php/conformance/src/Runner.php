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
        $set = $options->fixtures ?? self::pinned()->fixtures;
        /** @var list<string> $claimed */
        $claimed = $adapter->levels;
        $levels = $options->levels === null ? $claimed : array_values(array_intersect($claimed, $options->levels));

        foreach ($defects ?? self::pinned()->defects as $entry) {
            if (!Defects::carries($entry->id)) {
                throw new \RuntimeException('The catalogue names a defect this runner does not carry: ' . json_encode($entry->id) . '.');
            }

            $mutation = Defects::apply($entry->id, $adapter, $set, $levels);

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

            foreach ($declared as $api => $entries) {
                if (!in_array($api, self::APIS, true)) {
                    throw new \RuntimeException('The adapter names the facility ' . json_encode($api) . '; the facilities are ' . implode(', ', self::APIS) . '.');
                }

                if (!is_array($entries) || !array_is_list($entries) || array_filter($entries, Unexpressible::valid(...)) !== $entries) {
                    throw new \RuntimeException("The adapter must name what it cannot express of a {$api} request as a list of property names, or of entries each naming a property with the values or the request it cannot be expressed in.");
                }
            }
        }

        // Section 13 sets no minimum for the bound on counted reads, so 0 is
        // one an implementation may keep; a bound below it, or not a count,
        // is no bound.
        $reentry = $adapter->reentry;

        if ($reentry !== null && (!is_int($reentry) || $reentry < 0)) {
            throw new \RuntimeException('The adapter must state its bound on counted reads, reentry, as a non-negative integer.');
        }

        if ($adapter->keeps !== null && !is_bool($adapter->keeps)) {
            throw new \RuntimeException('The adapter must state whether a modifier can keep an input past its call, keeps, as true or false.');
        }

        if ($adapter->resolutions !== null && !is_bool($adapter->resolutions)) {
            throw new \RuntimeException('The adapter must state whether its bound counts a resolution asked for inside another, resolutions, as true or false.');
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
                self::exclusion($level, $adapter, $options) ?? self::unexpressible($case, $adapter) ?? self::refusing($case) ?? self::kept($case, $adapter),
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

    // A case whose request reads what the adapter documented it cannot
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
        $entries = $adapter->unexpressible[$api] ?? null;
        $options = $format->options ?? null;

        foreach (is_array($entries) ? $entries : [] as $entry) {
            $reason = Unexpressible::covers($entry, $api, $options instanceof \stdClass ? $options : new \stdClass());

            if ($reason !== null) {
                return $reason;
            }
        }

        return null;
    }

    // A case whose inputs hold a `refusing` tag asks for a value whose
    // entries raise where they are read, and PHP has none that this runner
    // can hand an implementation that reads only arrays and `stdClass`, as
    // a value model of plain data does (section 4): props that refused
    // nothing would fail the case for the reports it no longer meets rather
    // than pin what it states. The runner builds the value, so it states it.
    private static function refusing(\stdClass $case): ?string
    {
        foreach (['message', 'payload', 'props', 'messageId', 'defaults'] as $input) {
            if (property_exists($case, $input) && Decode::holds($case->{$input}, 'refusing')) {
                return 'PHP has no value the implementation reads whose entries raise where they are read.';
            }
        }

        return null;
    }

    // A case built from the bound on counted reads, or one that reads a kept
    // input after the modifier that kept it returned, measures what the
    // adapter stated it does not do, and is left out the way an unclaimed
    // level is. So is a resolution asked for inside another at a bound of
    // 0, which an implementation whose bound counts resolutions refuses.
    private static function kept(\stdClass $case, Adapter $adapter): ?string
    {
        $keeps = 'The adapter states that a modifier cannot keep an input past its call.';
        $nested = 'The adapter states a bound of 0 on counted reads, so the resolution a modifier asks for inside another may be refused.';

        if (!property_exists($case, 'generate')) {
            $keeping = $case->keeping ?? null;
            $registered = $keeping instanceof \stdClass ? array_values(array_filter(get_object_vars($keeping), is_string(...))) : [];

            if ($adapter->keeps === false && array_intersect($registered, Behaviours::LATER) !== []) {
                return $keeps;
            }

            if ($adapter->reentry === 0 && array_intersect($registered, Behaviours::NESTS) !== []) {
                return $nested;
            }

            return null;
        }

        if ($case->generate === Generators::RESOLUTIONS) {
            return self::counting($adapter);
        }

        if (!in_array($case->generate, Generators::COUNTED, true)) {
            return null;
        }

        $framed = in_array($case->generate, Generators::FRAMED, true);

        // Built from resolutions, the two framed constructions read nothing
        // kept after the modifier that kept it returned.
        if ($adapter->keeps === false && !($framed && $adapter->resolutions === true)) {
            return $keeps;
        }

        if ($adapter->reentry === null) {
            return 'The adapter states no bound on counted reads.';
        }

        if ($case->generate === 'reentry-own-reads-free' && $adapter->reentry === 0) {
            return 'The adapter states a bound of 0 on counted reads, so no counted read stands for own reads to be made inside.';
        }

        if ($framed && $adapter->reentry === 0) {
            return $nested;
        }

        return null;
    }

    // The construction of resolutions one inside another keeps no input, so
    // what the adapter states of keeping does not reach it. It measures a
    // bound that counts resolutions, and writes two brackets for each that
    // stands, which the output limit has to hold.
    private static function counting(Adapter $adapter): ?string
    {
        if (!is_int($adapter->reentry)) {
            return 'The adapter states no bound on counted reads.';
        }

        if ($adapter->resolutions !== true) {
            return 'The adapter does not state that its bound counts a resolution asked for inside another.';
        }

        /** @var array{output: int} $limits */
        $limits = $adapter->limits;

        if (strlen(Generators::renested($adapter->reentry)) > $limits['output']) {
            return 'The output limit the adapter states does not hold the brackets of as many resolutions as its bound lets stand.';
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
