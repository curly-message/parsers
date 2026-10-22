<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * Runs a resolution case and compares what the adapter answered, in the order
 * RUNNER.md, "Executing a case", states: the answer's shape, the output or
 * the request, then the reports. The first mismatch is the outcome.
 */
final class Compare
{
    public const CATEGORIES = ['zero', 'one', 'two', 'few', 'many', 'other'];

    private const COMPARED = ['origin', 'id', 'limit'];

    /** Runs a written-out case: decodes its inputs, reads its expectation, and compares. */
    public static function concrete(Adapter $adapter, \stdClass $case, ?Performer $performer): Outcome
    {
        return self::execute($adapter, self::prepare($adapter, $case), $performer, self::id($case));
    }

    /** Runs a generated case: builds it from the adapter's limits, and compares. */
    public static function generated(Adapter $adapter, \stdClass $case, ?Performer $performer): Outcome
    {
        $name = $case->generate ?? null;

        if (!is_string($name) || !in_array($name, Generators::NAMES, true)) {
            throw new \RuntimeException('The case ' . self::id($case) . ' names no construction: ' . json_encode($name) . '.');
        }

        /** @var array{output: int, read: int, conversion: int, nesting: int} $limits */
        $limits = $adapter->limits;

        return self::execute($adapter, Generators::build($name, $limits, $adapter->absent), $performer, self::id($case));
    }

    /** A field of a report, a request or a node, held as an array or an object; null where it holds none. */
    public static function field(mixed $value, string $name): mixed
    {
        if (is_array($value)) {
            return $value[$name] ?? null;
        }

        if ($value instanceof \stdClass) {
            return $value->{$name} ?? null;
        }

        return null;
    }

    private static function id(\stdClass $case): string
    {
        $id = $case->id ?? null;

        return is_string($id) ? $id : '';
    }

    private static function prepare(Adapter $adapter, \stdClass $case): Prepared
    {
        $absent = $adapter->absent;
        $expected = $case->expected ?? null;

        if (!$expected instanceof \stdClass) {
            throw new \RuntimeException('The case ' . self::id($case) . ' states no expectation.');
        }

        $format = $expected->format ?? null;
        $format = $format instanceof \stdClass ? $format : null;
        $categories = null;

        if (($format->api ?? null) === 'PluralRules') {
            $categories = self::categories($case, $expected->categories ?? null);
        } elseif ($format === null && !is_string($expected->output ?? null)) {
            throw new \RuntimeException('The case ' . self::id($case) . ' expects neither an output nor a format.');
        }

        $reports = [];

        foreach (is_array($expected->reports ?? null) ? $expected->reports : [] as $report) {
            if (!$report instanceof \stdClass) {
                throw new \RuntimeException('The case ' . self::id($case) . ' expects a report that is not an object.');
            }

            $reports[] = get_object_vars($report);
        }

        $locale = self::input($case, 'locale', $absent);
        $modifiers = [];

        $registered = $case->modifiers ?? null;

        foreach ($registered instanceof \stdClass ? get_object_vars($registered) : [] as $name => $behaviour) {
            $modifiers[(string) $name] = Behaviours::named(is_string($behaviour) ? $behaviour : '');
        }

        return new Prepared(
            new Resolution(
                self::input($case, 'message', $absent),
                self::input($case, 'payload', $absent),
                self::input($case, 'props', $absent),
                $locale,
                self::input($case, 'messageId', $absent),
                $modifiers,
                self::input($case, 'defaults', $absent),
            ),
            new Expectation(
                $format === null ? (is_string($expected->output ?? null) ? $expected->output : null) : null,
                $format,
                $categories,
                $reports,
                is_string($locale) ? $locale : null,
            ),
        );
    }

    private static function input(\stdClass $case, string $name, mixed $absent): mixed
    {
        return property_exists($case, $name) ? Decode::value($case->{$name}, $absent) : $absent;
    }

    // A selection case names the output of every category. One that leaves a
    // category out is the set's defect and not the implementation's, whichever
    // reading it is measured on, so it ends the run before anything runs.
    private static function categories(\stdClass $case, mixed $named): \stdClass
    {
        foreach (self::CATEGORIES as $category) {
            if (!$named instanceof \stdClass || !property_exists($named, $category) || !is_string($named->{$category})) {
                throw new \RuntimeException('The case ' . self::id($case) . ' names no output for the category ' . json_encode($category) . '.');
            }
        }

        return $named;
    }

    private static function execute(Adapter $adapter, Prepared $prepared, ?Performer $performer, string $id): Outcome
    {
        $expected = $prepared->expected;
        $shown = $expected->output ?? $expected->format;

        try {
            $resolved = $adapter->resolve($prepared->input);
        } catch (\Throwable $error) {
            return Outcome::failed('The adapter raised.', $shown, $error->getMessage());
        }

        if (!is_array($resolved) || !is_string($resolved['output'] ?? null)) {
            return Outcome::failed('The adapter answered with no resolution.', $shown, $resolved);
        }

        $output = $resolved['output'];
        $format = $expected->format;

        if ($format !== null && array_key_exists('formats', $resolved)) {
            $produced = self::formats($format, $resolved['formats'])
                ?? (($format->api ?? null) === 'PluralRules' ? self::selection($expected, self::first($resolved['formats']), $output) : null);
        } elseif ($format !== null) {
            $produced = self::performed($expected, $format, $performer, $id, $output);
        } else {
            $produced = $output === $expected->output ? null : Outcome::failed('The output differs.', $expected->output, $output);
        }

        if ($produced !== null) {
            return $produced;
        }

        if (!array_key_exists('reports', $resolved)) {
            return Outcome::unobserved();
        }

        $reports = $resolved['reports'];

        if (!is_array($reports) || !array_is_list($reports)) {
            return Outcome::failed('The reports are not a list.', $expected->reports, $reports);
        }

        return self::reports($expected->reports, $reports) ?? Outcome::passed();
    }

    private static function first(mixed $formats): mixed
    {
        return is_array($formats) ? ($formats[0] ?? null) : null;
    }

    // The text reading: the runner performs the case's request on its own
    // host and compares the text, or for a selection takes the category its
    // host answers and expects the output the case names for it.
    private static function performed(Expectation $expected, \stdClass $format, ?Performer $performer, string $id, string $output): ?Outcome
    {
        if ($performer === null) {
            throw new \RuntimeException("The case {$id} states a request the adapter does not expose, and the run was given nothing to perform it with.");
        }

        $options = $format->options ?? new \stdClass();
        $request = [
            'api' => is_string($format->api ?? null) ? $format->api : '',
            'options' => $options instanceof \stdClass ? $options : new \stdClass(),
            'input' => $format->input ?? null,
        ];
        $text = $performer->perform($request, $expected->locale);

        if ($request['api'] === 'PluralRules') {
            $chosen = $expected->categories->{$text} ?? null;

            return $output === $chosen ? null : Outcome::failed("The output differs from the one the category {$text} selects.", $chosen, $output);
        }

        return $output === $text ? null : Outcome::failed('The output differs.', $text, $output);
    }

    // Section 11.2 lets an implementation expose the request it makes, and an
    // adapter that does is measured on it rather than on the text its own
    // locale data made. A case that states a request writes the placeholder
    // alone, so the resolution it describes makes exactly one.
    private static function formats(\stdClass $expected, mixed $actual): ?Outcome
    {
        if (!is_array($actual) || !array_is_list($actual)) {
            return Outcome::failed('The formatting requests are not a list.', [$expected], $actual);
        }

        if (count($actual) !== 1) {
            return Outcome::failed('Expected one formatting request, got ' . count($actual) . '.', [$expected], $actual);
        }

        $request = $actual[0];

        if (self::field($request, 'api') !== ($expected->api ?? null)) {
            return Outcome::failed('The facility of the request differs.', $expected, $request);
        }

        if (!Canonical::equal($expected->options ?? new \stdClass(), self::field($request, 'options') ?? [])) {
            return Outcome::failed('The properties of the request differ.', $expected, $request);
        }

        if (!Canonical::equal($expected->input ?? null, self::field($request, 'input'))) {
            return Outcome::failed('The input of the request differs.', $expected, $request);
        }

        return null;
    }

    // A selection request exposed on its own says what was asked and not what
    // the host answered, and the answer is what chose the output (section
    // 11.5): the category it carries is read, and the output held to the one
    // the case names for it.
    private static function selection(Expectation $expected, mixed $request, string $output): ?Outcome
    {
        $category = self::field($request, 'category');

        if (!is_string($category) || !in_array($category, self::CATEGORIES, true)) {
            return Outcome::failed('The selection request carries no category the plural rules name.', self::CATEGORIES, $category);
        }

        $chosen = $expected->categories->{$category} ?? null;

        return $output === $chosen ? null : Outcome::failed("The output differs from the one the category {$category} selects.", $chosen, $output);
    }

    /**
     * A field is compared where the adapter's report carries it and the case
     * expects something of it: the specification prescribes a report no shape.
     *
     * @param list<array<array-key, mixed>> $expected
     * @param list<mixed> $actual
     */
    private static function reports(array $expected, array $actual): ?Outcome
    {
        if (count($expected) !== count($actual)) {
            return Outcome::failed('Expected ' . self::count($expected) . ', got ' . self::count($actual) . '.', $expected, $actual);
        }

        foreach ($actual as $index => $report) {
            $wanted = $expected[$index];
            $position = 'report ' . ($index + 1);

            if (self::field($report, 'code') !== ($wanted['code'] ?? null)) {
                return Outcome::failed("The code of {$position} differs.", $wanted, $report);
            }

            // Each field is text or a number, so a report carrying one of
            // another shape carries what no expectation names, and that field
            // goes unobserved.
            foreach (self::COMPARED as $name) {
                $carried = self::field($report, $name);

                if (is_scalar($carried) && array_key_exists($name, $wanted) && !Canonical::scalar($wanted[$name], $carried)) {
                    return Outcome::failed("The {$name} of {$position} differs.", $wanted, $report);
                }
            }
        }

        return null;
    }

    /** @param list<mixed> $reports */
    private static function count(array $reports): string
    {
        return count($reports) . (count($reports) === 1 ? ' report' : ' reports');
    }
}
