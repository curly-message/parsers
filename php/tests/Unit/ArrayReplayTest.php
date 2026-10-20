<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\Conformance\Fixture;
use CurlyMessage\Conformance\Options;
use CurlyMessage\Conformance\Planned;
use CurlyMessage\Conformance\Resolution;
use CurlyMessage\Conformance\Runner;
use CurlyMessage\Tests\Conformance\Pinned;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/**
 * The Core and Extensions cases of the set again, each object of their
 * inputs handed over as the associative array PHP callers mostly pass. An
 * object whose array would serialize otherwise stays an object: an empty
 * one, one whose names are 0 to n-1 in order, and one that holds itself.
 * Every case holds over arrays as it does over objects, so the replay leaves
 * out only what the suite leaves out of the set.
 */
final class ArrayReplayTest extends TestCase
{
    /** @var array<string, Planned|string>|null */
    private static ?array $plan = null;

    /** @return iterable<string, array{string}> */
    public static function cases(): iterable
    {
        foreach (array_keys(self::plan()) as $id) {
            yield $id => [$id];
        }
    }

    #[DataProvider('cases')]
    public function testTheCaseResolvesAsTheSetStatesOverArrays(string $id): void
    {
        $case = self::plan()[$id];

        if (is_string($case)) {
            self::markTestSkipped($case);
        }

        $outcome = $case->execute();
        $shown = static function (mixed $value): string {
            $json = json_encode($value, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_PARTIAL_OUTPUT_ON_ERROR);

            return $json === false ? get_debug_type($value) : $json;
        };

        self::assertTrue($outcome->ok, "{$case->where()}: {$outcome->reason} Expected {$shown($outcome->expected)}, actual {$shown($outcome->actual)}.");
    }

    public function testAnObjectIsAnArrayUnlessItsArrayWouldSerializeOtherwise(): void
    {
        $itself = new \stdClass();
        $itself->self = $itself;
        $value = self::arrays((object) ['a' => (object) ['1' => 'x', 'b' => [(object) ['c' => 1]]], 'e' => new \stdClass(), 'l' => (object) ['0' => 'x', '1' => 'y'], 'o' => (object) ['1' => 'x', '0' => 'y'], 'i' => $itself]);

        self::assertIsArray($value);
        self::assertSame(['1' => 'x', 'b' => [['c' => 1]]], $value['a']);
        self::assertEquals(new \stdClass(), $value['e']);
        self::assertEquals((object) ['0' => 'x', '1' => 'y'], $value['l']);
        self::assertSame([1 => 'x', 0 => 'y'], $value['o']);
        self::assertSame($itself, $value['i']);
    }

    /** @return array<string, Planned|string> */
    private static function plan(): array
    {
        if (self::$plan !== null) {
            return self::$plan;
        }

        $fixtures = array_values(array_filter(Pinned::fixtures(), static fn(Fixture $fixture): bool => in_array($fixture->file->level ?? null, ['core', 'extensions'], true)));
        $exposed = Pinned::adapter()->resolve(...);
        $adapter = Pinned::adapter()->with(resolve: static fn(Resolution $input): mixed => $exposed(new Resolution(
            self::arrays($input->message),
            self::arrays($input->payload),
            self::arrays($input->props),
            self::arrays($input->locale),
            self::arrays($input->id),
            $input->modifiers,
            self::arrays($input->defaults),
        )));
        $plan = Runner::plan($adapter, new Options($fixtures, ['core', 'extensions']));
        $cases = Pinned::LEFT_OUT;

        foreach ($plan->cases as $case) {
            $cases[$case->id] = $case;
        }

        foreach ($plan->skipped as $case) {
            $cases[$case->id] = $case->reason;
        }

        return self::$plan = $cases;
    }

    /**
     * The value with each object turned into an array, where the array
     * serializes as the object does. An object on a cycle stays an object,
     * as does every object on that cycle.
     *
     * @param list<\stdClass> $path the objects the value is inside
     */
    private static function arrays(mixed $value, array $path = []): mixed
    {
        if (is_array($value)) {
            return array_map(static fn(mixed $entry): mixed => self::arrays($entry, $path), $value);
        }

        if (!$value instanceof \stdClass) {
            return $value;
        }

        if (in_array($value, $path, true)) {
            throw new Cycle($value);
        }

        $entries = get_object_vars($value);

        if ($entries === [] || array_keys($entries) === range(0, count($entries) - 1)) {
            return $value;
        }

        $inside = [...$path, $value];

        try {
            return array_map(static fn(mixed $entry): mixed => self::arrays($entry, $inside), $entries);
        } catch (Cycle $cycle) {
            if ($cycle->object !== $value) {
                throw $cycle;
            }

            return $value;
        }
    }
}
