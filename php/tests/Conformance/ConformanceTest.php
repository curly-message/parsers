<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Conformance;

use CurlyMessage\Conformance\Options;
use CurlyMessage\Conformance\Planned;
use CurlyMessage\Conformance\Runner;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/**
 * Every case of the pinned set, one test each, at every level the adapter
 * claims: read off the requests the parser exposes and, where ext-intl
 * performs them, off the text the requests render to.
 */
final class ConformanceTest extends TestCase
{
    /** @var array<string, array<string, Planned|string>> */
    private static array $plans = [];

    /** @return iterable<string, array{string, string}> */
    public static function cases(): iterable
    {
        foreach (self::readings() as $reading) {
            foreach (array_keys(self::plan($reading)) as $id) {
                yield "{$reading} {$id}" => [$reading, $id];
            }
        }

        if (!extension_loaded('intl')) {
            yield 'performed' => ['performed', ''];
        }
    }

    #[DataProvider('cases')]
    public function testTheCaseResolvesAsTheSetStates(string $reading, string $id): void
    {
        if ($id === '') {
            self::markTestSkipped('The requests are performed with ext-intl, which this PHP does not load.');
        }

        $case = self::plan($reading)[$id];

        if (is_string($case)) {
            self::markTestSkipped($case);
        }

        $outcome = $case->execute();
        $shown = static function (mixed $value): string {
            $json = json_encode($value, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_PARTIAL_OUTPUT_ON_ERROR);

            return $json === false ? get_debug_type($value) : $json;
        };

        self::assertTrue($outcome->ok, "{$case->where()}: {$outcome->reason} Expected {$shown($outcome->expected)}, actual {$shown($outcome->actual)}. {$case->description}");
        self::assertNull($outcome->unobserved, "The adapter observes every field, yet the case's {$outcome->unobserved} went unobserved.");
    }

    /** @return list<string> */
    private static function readings(): array
    {
        return extension_loaded('intl') ? ['exposed', 'performed'] : ['exposed'];
    }

    /**
     * The cases of one reading by id, each planned or the reason it is left out.
     *
     * @return array<string, Planned|string>
     */
    private static function plan(string $reading): array
    {
        if (isset(self::$plans[$reading])) {
            return self::$plans[$reading];
        }

        $plan = $reading === 'exposed'
            ? Runner::plan(Pinned::adapter(), new Options(Pinned::fixtures()))
            : Runner::plan(Pinned::withheld(), new Options(Pinned::fixtures(), null, Pinned::performer()));
        $cases = Pinned::LEFT_OUT;

        foreach ($plan->cases as $case) {
            $cases[$case->id] = $case;
        }

        foreach ($plan->skipped as $case) {
            $cases[$case->id] = $case->reason;
        }

        return self::$plans[$reading] = $cases;
    }
}
