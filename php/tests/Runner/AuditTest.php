<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Runner;

use CurlyMessage\Conformance\Adapter;
use CurlyMessage\Conformance\Audited;
use CurlyMessage\Conformance\DefectEntry;
use CurlyMessage\Conformance\Defects;
use CurlyMessage\Conformance\Fixture;
use CurlyMessage\Conformance\Generators;
use CurlyMessage\Conformance\Options;
use CurlyMessage\Conformance\Resolution;
use CurlyMessage\Conformance\Runner;
use PHPUnit\Framework\TestCase;

final class AuditTest extends TestCase
{
    public function testTheStubPassesTheSmallSet(): void
    {
        $result = Runner::run(Stub::adapter(), new Options(Stub::fixtures()));

        self::assertSame([], $result->failed);
        self::assertCount(7, $result->passed);
        self::assertSame([], $result->unobserved);
    }

    public function testTheRunnerCarriesTheWholeCatalogue(): void
    {
        $catalogue = array_map(static fn(DefectEntry $entry): string => $entry->id, Runner::pinned()->defects);

        self::assertSame(Defects::IDS, $catalogue);
    }

    public function testEveryDefectTheSmallSetReachesIsCaughtOnAnAdapterThatObservesEverything(): void
    {
        $audited = Runner::audit(Stub::adapter(), new Options(Stub::fixtures()));
        // The small set states no request a conditional entry covers and
        // builds no construction of the conversion limit, of kept reads or of
        // resolutions one inside another, and the stub states no bound on
        // counted reads.
        $unreached = ['unexpressible-conditional-declared', 'conversion-limit-overstated', 'conversion-limit-understated', 'reentry-limit-overstated', 'reentry-limit-understated', 'resolutions-overstated'];
        $expected = [];

        foreach (Defects::IDS as $id) {
            $expected[$id] = in_array($id, $unreached, true) ? 'unreachable' : 'caught';
        }

        self::assertSame($expected, self::outcomes($audited));
    }

    public function testADefectOfADeclaredBoundIsReachedWhereTheRunBuildsACaseFromIt(): void
    {
        $file = static fn(string $level, string ...$names): Fixture => new Fixture('built.json', (object) ['format' => 'curly-message-3', 'level' => $level, 'section' => '13', 'cases' => array_map(
            static fn(string $name): \stdClass => (object) ['id' => $name, 'description' => 'Built.', 'generate' => $name],
            $names,
        )]);
        $all = Generators::COUNTED;
        $only = static fn(bool $over, bool $under, bool $reentryOver, bool $reentryUnder, bool $resolutionsOver = false): array => [
            'conversion-limit-overstated' => $over, 'conversion-limit-understated' => $under,
            'reentry-limit-overstated' => $reentryOver, 'reentry-limit-understated' => $reentryUnder, 'claims-negative-reentry' => true,
            'resolutions-overstated' => $resolutionsOver, 'claims-non-boolean-resolutions' => true,
        ];

        self::assertSame($only(true, false, false, false), self::bounds([$file('core', 'conversion-at-limit')]));
        self::assertSame($only(false, true, false, false), self::bounds([$file('core', 'conversion-over-limit')]));
        self::assertSame($only(false, true, false, false), self::bounds([$file('core', 'conversion-over-limit-continues')]));
        self::assertSame($only(false, false, true, false), self::bounds([$file('extensions', 'reentry-at-limit')]));
        self::assertSame($only(false, false, true, true), self::bounds([$file('extensions', 'reentry-over-limit')]));
        self::assertSame($only(false, false, true, true), self::bounds([$file('extensions', 'reentry-refused-not-recorded')]));
        self::assertSame($only(false, false, true, false), self::bounds([$file('extensions', 'reentry-own-reads-free')]));
        self::assertSame($only(false, false, true, true), self::bounds([$file('extensions', 'reentry-over-limit-by-a-conversion')]));
        // Built from kept reads, it stands a place short of a bound that
        // counts reads alone, so neither bound stated one off alters what it
        // answers; built from resolutions, it shows one stated higher only
        // at a stated bound of 1, through the reports the adapter observes.
        $nested = [$file('extensions', 'reentry-own-read-in-a-nested-resolution')];
        self::assertSame($only(false, false, false, false), self::bounds($nested));
        self::assertSame($only(false, false, false, false), self::bounds($nested, Stub::adapter()->with(reentry: 3, resolutions: true)));
        $observed = static function (Adapter $adapter) use ($nested): bool {
            $mutation = Defects::apply('reentry-limit-overstated', $adapter, $nested, ['core', 'extensions']);
            $before = $mutation->reached();
            $mutation->adapter->resolve(new Resolution('edges', new \stdClass(), $adapter->absent, $adapter->absent, $adapter->absent, [], $adapter->absent));

            return !$before && $mutation->reached();
        };
        self::assertTrue($observed(Stub::adapter()->with(reentry: 0, resolutions: true)));
        self::assertFalse($observed(Stub::adapter(reports: false)->with(reentry: 0, resolutions: true)));
        self::assertFalse($observed(Stub::adapter()->with(reentry: 0)));
        // Beneath a modifier of the format, the read past the bound shows a
        // bound stated one lower however the case is built, one stated one
        // higher built from resolutions, and a `resolutions` overstated above
        // a bound of 1.
        $defined = [$file('extensions', Generators::DEFINED)];
        self::assertSame($only(false, false, false, true, true), self::bounds($defined));
        self::assertSame($only(false, false, false, false), self::bounds($defined, Stub::adapter()->with(reentry: 1)));
        self::assertSame($only(false, false, true, true), self::bounds($defined, Stub::adapter()->with(reentry: 2, resolutions: true)));
        self::assertSame($only(false, false, true, false), self::bounds($defined, Stub::adapter()->with(reentry: 1, resolutions: true)));
        self::assertSame($only(false, false, false, false, true), self::bounds($defined, Stub::adapter()->with(reentry: 2, keeps: false)));
        self::assertSame($only(false, false, true, true), self::bounds($defined, Stub::adapter()->with(reentry: 2, keeps: false, resolutions: true)));
        // The stub's output limit of 5 holds two resolutions' brackets, not
        // four, and what is stated of keeping does not reach the case.
        $resolutions = $file('extensions', Generators::RESOLUTIONS);
        self::assertSame($only(false, false, false, false, true), self::bounds([$resolutions], Stub::adapter()->with(reentry: 1)));
        self::assertSame($only(false, false, false, false, true), self::bounds([$resolutions], Stub::adapter()->with(reentry: 1, keeps: false)));
        self::assertSame($only(false, false, false, false), self::bounds([$resolutions], Stub::adapter()->with(reentry: 1, resolutions: true)));
        self::assertSame($only(false, false, false, false), self::bounds([$resolutions], Stub::adapter()->with(reentry: 3)));
        self::assertSame($only(false, false, false, false), self::bounds([$resolutions], Stub::adapter()));
        self::assertSame($only(false, false, false, false), self::bounds([$resolutions], Stub::adapter()->with(reentry: 1), ['core']));
        self::assertSame($only(false, false, true, false), self::bounds([$file('extensions', ...$all)], Stub::adapter()->with(reentry: 0)));
        self::assertSame($only(false, false, false, false), self::bounds([$file('extensions', ...$all)], Stub::adapter()));
        self::assertSame($only(false, false, false, false, true), self::bounds([$file('extensions', ...$all)], Stub::adapter()->with(reentry: 3, keeps: false)));
        self::assertSame($only(false, false, false, false), self::bounds([$file('extensions', 'conversion-at-limit', ...$all)], null, ['core']));
    }

    public function testADeclaredEntryIsReachedWhereOnlyItCoversARequestOfTheRun(): void
    {
        $file = static fn(array $options): Fixture => new Fixture('intl.json', (object) ['format' => 'curly-message-3', 'level' => 'intl', 'section' => '11.2', 'cases' => [
            (object) ['id' => 'n', 'description' => 'A number.', 'message' => '{{n:number}}', 'expected' => (object) ['format' => (object) ['api' => 'NumberFormat', 'options' => (object) $options, 'input' => 1]]],
        ]]);
        $reached = static fn(Adapter $adapter, array $options): array => [
            Defects::apply('unexpressible-declared', $adapter, [$file($options)], ['core', 'intl'])->reached(),
            Defects::apply('unexpressible-conditional-declared', $adapter, [$file($options)], ['core', 'intl'])->reached(),
        ];

        self::assertSame([true, true], $reached(Stub::adapter(), ['maximumFractionDigits' => 2, 'useGrouping' => false]));
        self::assertSame([true, false], $reached(Stub::adapter(), ['maximumFractionDigits' => 2, 'useGrouping' => true]));
        self::assertSame([false, false], $reached(Stub::adapter(), ['style' => 'percent']));
        self::assertSame([false, false], $reached(Stub::adapter()->with(unexpressible: ['NumberFormat' => ['maximumFractionDigits']]), ['maximumFractionDigits' => 2, 'useGrouping' => false]));
        self::assertSame(
            ['NumberFormat' => ['style', ['property' => 'maximumFractionDigits', 'values' => [2], 'when' => ['useGrouping' => false]]]],
            Defects::apply('unexpressible-conditional-declared', Stub::adapter()->with(unexpressible: ['NumberFormat' => ['style']]), [], [])->adapter->unexpressible,
        );
    }

    public function testADefectNothingCarriesIsUnreachableRatherThanMissed(): void
    {
        $audited = Runner::audit(Stub::adapter(reports: false, formats: false, tree: false, levels: ['core']), new Options(Stub::fixtures()));
        $caught = ['output-truncated', 'output-trimmed', 'answers-nothing', 'raises', 'claims-no-core', 'claims-unknown-level', 'claims-no-limits', 'claims-negative-reentry', 'claims-non-boolean-resolutions'];
        $expected = [];

        foreach (Defects::IDS as $id) {
            $expected[$id] = in_array($id, $caught, true) ? 'caught' : 'unreachable';
        }

        self::assertSame($expected, self::outcomes($audited));
    }

    public function testAnAnswerOtherThanTheCatalogueStatesIsMissed(): void
    {
        $audited = Runner::audit(Stub::adapter(), new Options(Stub::fixtures()), [new DefectEntry('output-truncated', 'A wrong expectation.', null, '9', 'skip')]);

        self::assertSame('fail', $audited[0]->observed);
        self::assertSame('missed', $audited[0]->outcome);
    }

    public function testADefectTheRunnerDoesNotCarryEndsTheAudit(): void
    {
        $this->expectExceptionMessage('The catalogue names a defect this runner does not carry: "output-doubled".');

        Runner::audit(Stub::adapter(), new Options(Stub::fixtures()), [new DefectEntry('output-doubled', 'Unknown.', null, '9', 'fail')]);
    }

    public function testAnAdapterThatFailsTheSetIsNotAudited(): void
    {
        $failing = Stub::adapter()->with(resolve: static fn(): array => ['output' => 'wrong', 'reports' => [], 'formats' => []]);

        $this->expectExceptionMessage('The audit needs an adapter that passes the set; this one fails');

        Runner::audit($failing, new Options(Stub::fixtures()));
    }

    /**
     * Whether each defect of a declared bound is reached on a run of the set.
     *
     * @param list<Fixture> $set
     * @param list<string> $run
     * @return array<string, bool>
     */
    private static function bounds(array $set, ?Adapter $adapter = null, array $run = ['core', 'intl', 'extensions']): array
    {
        $adapter ??= Stub::adapter()->with(reentry: 3);
        $reached = [];

        foreach (['conversion-limit-overstated', 'conversion-limit-understated', 'reentry-limit-overstated', 'reentry-limit-understated', 'claims-negative-reentry', 'resolutions-overstated', 'claims-non-boolean-resolutions'] as $id) {
            $reached[$id] = Defects::apply($id, $adapter, $set, $run)->reached();
        }

        return $reached;
    }

    /**
     * @param list<Audited> $audited
     * @return array<string, string>
     */
    private static function outcomes(array $audited): array
    {
        $outcomes = [];

        foreach ($audited as $entry) {
            $outcomes[$entry->defect->id] = $entry->outcome;
        }

        return $outcomes;
    }
}
