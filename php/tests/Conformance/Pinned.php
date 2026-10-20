<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Conformance;

use CurlyMessage\Conformance\Adapter;
use CurlyMessage\Conformance\Fixture;
use CurlyMessage\Conformance\Performer;
use CurlyMessage\Conformance\Resolution;
use CurlyMessage\Conformance\Runner;
use CurlyMessage\Conformance\Set;
use CurlyMessage\Facility as Api;
use CurlyMessage\Internal\Intl\Facility;

/** The pinned set as this suite runs it, and the adapter it runs the set through. */
final class Pinned
{
    /**
     * The cases the suite leaves out of the pinned set, each with why. The
     * runner decides nothing about a case; the suite passes it the set
     * without these, to every run and to the audit alike.
     */
    public const LEFT_OUT = [
        'comparisons/infinity-compares' => 'The case pins ECMAScript\'s reading of the text "Infinity" as a number, which section 11.2 leaves to the host and PHP does not read; it is left out until a set that does not pin it is released (https://github.com/curly-message/spec/issues/44).',
    ];

    private static ?Adapter $adapter = null;

    /** @var list<Fixture>|null */
    private static ?array $fixtures = null;

    public static function adapter(): Adapter
    {
        if (self::$adapter === null) {
            $adapter = require __DIR__ . '/adapter.php';
            self::$adapter = $adapter instanceof Adapter ? $adapter : throw new \LogicException('adapter.php returns no adapter.');
        }

        return self::$adapter;
    }

    /**
     * The fixture files of the pinned set without the cases left out, read
     * once per process.
     *
     * @return list<Fixture>
     */
    public static function fixtures(): array
    {
        return self::$fixtures ??= self::without(Runner::pinned());
    }

    /**
     * The fixture files of a set without the cases left out. A case left out
     * that the set no longer holds ends the run, so the list cannot outlive
     * the case it names.
     *
     * @return list<Fixture>
     */
    public static function without(Set $set): array
    {
        $found = [];
        $fixtures = [];

        foreach ($set->fixtures as $fixture) {
            $file = clone $fixture->file;
            $cases = [];

            foreach (is_array($file->cases ?? null) ? $file->cases : [] as $case) {
                $id = $case instanceof \stdClass ? ($case->id ?? null) : null;

                if (is_string($id) && array_key_exists($id, self::LEFT_OUT)) {
                    $found[] = $id;

                    continue;
                }

                $cases[] = $case;
            }

            $file->cases = $cases;
            $fixtures[] = new Fixture($fixture->name, $file);
        }

        $stale = array_diff(array_keys(self::LEFT_OUT), $found);

        if ($stale !== []) {
            throw new \LogicException('The suite leaves out cases the set does not hold: ' . implode(', ', $stale) . '.');
        }

        return $fixtures;
    }

    /** The adapter with its requests withheld, so that the runner performs each request a case states. */
    public static function withheld(): Adapter
    {
        $exposed = self::adapter()->resolve(...);

        return self::adapter()->with(resolve: static function (Resolution $input) use ($exposed): mixed {
            $answer = $exposed($input);

            if (is_array($answer)) {
                unset($answer['formats']);
            }

            return $answer;
        });
    }

    /**
     * The facility the parser performs its requests with. A run that reads
     * the text through it checks how a request composes and renders, not
     * that the text is ECMAScript's.
     */
    public static function performer(): Performer
    {
        return new class implements Performer {
            public function perform(array $request, ?string $locale): string
            {
                $input = match (true) {
                    is_int($request['input']), is_float($request['input']) => $request['input'],
                    is_array($request['input']) && array_is_list($request['input']) && count($request['input']) === 2 => self::relative($request['input'][0], $request['input'][1]),
                    default => throw new \LogicException('The request states an input no facility takes.'),
                };

                return Facility::perform(Api::from($request['api']), $locale ?? throw new \LogicException('The request states no locale.'), get_object_vars($request['options']), $input);
            }

            /** @return array{int|float, string} */
            private static function relative(mixed $count, mixed $unit): array
            {
                return (is_int($count) || is_float($count)) && is_string($unit) ? [$count, $unit] : throw new \LogicException('The request states an input no facility takes.');
            }
        };
    }
}
