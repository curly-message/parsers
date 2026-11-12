<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Conformance;

use CurlyMessage\Conformance\Audited;
use CurlyMessage\Conformance\Options;
use CurlyMessage\Conformance\Runner;
use PHPUnit\Framework\Attributes\Group;
use PHPUnit\Framework\TestCase;

#[Group('audit')]
final class AuditTest extends TestCase
{
    /**
     * Each deliberately wrong copy of the adapter is caught. Without ext-intl
     * the adapter claims no Intl level, so the two defects only a request or
     * an Intl claim can show have no case to reach them.
     */
    public function testTheRunnerCatchesEveryDefectOfTheCatalogue(): void
    {
        $audited = Runner::audit(Pinned::adapter(), new Options(Pinned::fixtures()));
        $outcomes = [];

        foreach ($audited as $entry) {
            $outcomes[$entry->defect->id] = $entry->outcome;
        }

        $expected = array_fill_keys(array_map(static fn(Audited $entry): string => $entry->defect->id, $audited), 'caught');

        if (!extension_loaded('intl')) {
            $expected = ['formats-wrong' => 'unreachable', 'unexpressible-declared' => 'unreachable'] + $expected;
        }

        ksort($expected);
        ksort($outcomes);

        self::assertCount(26, $audited);
        self::assertSame($expected, $outcomes);
    }
}
