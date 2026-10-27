<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Runner;

use CurlyMessage\Conformance\Audited;
use CurlyMessage\Conformance\DefectEntry;
use CurlyMessage\Conformance\Defects;
use CurlyMessage\Conformance\Options;
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

    public function testEveryDefectIsCaughtOnAnAdapterThatObservesEverything(): void
    {
        $audited = Runner::audit(Stub::adapter(), new Options(Stub::fixtures()));

        self::assertSame(array_fill_keys(Defects::IDS, 'caught'), self::outcomes($audited));
    }

    public function testADefectNothingCarriesIsUnreachableRatherThanMissed(): void
    {
        $audited = Runner::audit(Stub::adapter(reports: false, formats: false, tree: false, levels: ['core']), new Options(Stub::fixtures()));
        $caught = ['output-truncated', 'output-trimmed', 'answers-nothing', 'raises', 'claims-no-core', 'claims-unknown-level', 'claims-no-limits'];
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
