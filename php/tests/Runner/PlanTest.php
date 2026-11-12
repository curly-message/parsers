<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Runner;

use CurlyMessage\Conformance\Adapter;
use CurlyMessage\Conformance\CstOffer;
use CurlyMessage\Conformance\Options;
use CurlyMessage\Conformance\Planned;
use CurlyMessage\Conformance\Runner;
use CurlyMessage\Conformance\Skipped;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class PlanTest extends TestCase
{
    use Cases;

    /** @return iterable<string, array{Adapter, Options, string}> */
    public static function refused(): iterable
    {
        $stub = Stub::adapter();
        $set = new Options(Stub::fixtures());

        yield 'levels not a list' => [$stub->with(levels: 'core'), $set, 'The adapter must list the levels it claims.'];
        yield 'levels a map' => [$stub->with(levels: ['a' => 'core']), $set, 'The adapter must list the levels it claims.'];
        yield 'an unknown level' => [$stub->with(levels: ['core', 'ecmascript']), $set, 'The adapter names the level "ecmascript"; the levels are core, intl and extensions.'];
        yield 'no core' => [$stub->with(levels: ['intl']), $set, 'The adapter must claim the core level.'];
        yield 'an unknown level to run' => [$stub, new Options(Stub::fixtures(), ['Core']), 'The levels option names the level "Core"; the levels are core, intl and extensions.'];
        yield 'an unclaimed level to run' => [$stub->with(levels: ['core']), new Options(Stub::fixtures(), ['intl']), 'The adapter does not claim the intl level.'];
        yield 'a limit missing' => [$stub->with(limits: ['output' => 1, 'read' => 1, 'conversion' => 1]), $set, 'The adapter must declare its nesting limit as a positive integer.'];
        yield 'a limit of zero' => [$stub->with(limits: ['nesting' => 0] + Stub::LIMITS), $set, 'The adapter must declare its nesting limit as a positive integer.'];
        yield 'a limit not an integer' => [$stub->with(limits: ['read' => 4.0] + Stub::LIMITS), $set, 'The adapter must declare its read limit as a positive integer.'];
        yield 'limits not a map' => [$stub->with(limits: 5), $set, 'The adapter must declare its output limit as a positive integer.'];
        yield 'unexpressible not a map' => [$stub->with(unexpressible: 'NumberFormat'), $set, 'The adapter must name the properties it cannot express by the request that reads them.'];
        yield 'an unknown facility' => [$stub->with(unexpressible: ['Collator' => []]), $set, 'The adapter names the facility "Collator"; the facilities are NumberFormat, DateTimeFormat, RelativeTimeFormat, PluralRules.'];
        yield 'properties not a list' => [$stub->with(unexpressible: ['NumberFormat' => 'style']), $set, 'The adapter must name what it cannot express of a NumberFormat request as a list of property names.'];
        yield 'a property not a name' => [$stub->with(unexpressible: ['NumberFormat' => ['style', 3]]), $set, 'The adapter must name what it cannot express of a NumberFormat request as a list of property names.'];
        yield 'an unknown unit' => [$stub->with(cst: new CstOffer('utf-32', static fn(): array => [])), $set, 'The adapter counts its spans in "utf-32"; the units are utf-8, utf-16, code-point.'];
        yield 'no unit' => [$stub->with(cst: new CstOffer(null, static fn(): array => [])), $set, 'The adapter counts its spans in null; the units are utf-8, utf-16, code-point.'];
    }

    #[DataProvider('refused')]
    public function testAClaimOutsideTheVocabularyRefusesTheRun(Adapter $adapter, Options $options, string $message): void
    {
        $this->expectException(\RuntimeException::class);
        $this->expectExceptionMessage($message);

        Runner::plan($adapter, $options);
    }

    public function testAClaimInsideTheVocabularyPlansEveryCase(): void
    {
        $plan = Runner::plan(Stub::adapter()->with(unexpressible: ['DateTimeFormat' => ['era']]), new Options(Stub::fixtures()));

        self::assertSame(['core/edges', 'core/two-reports', 'intl/number', 'intl/plural', 'limits/output-over-limit', 'limits/nesting-over-limit', 'tree/escaped-key'], self::ids($plan->cases));
        self::assertSame([], $plan->skipped);
    }

    public function testEachCaseLeftOutIsListedWithItsReason(): void
    {
        $plan = Runner::plan(Stub::adapter(tree: false, levels: ['core', 'intl'])->with(unexpressible: ['NumberFormat' => ['maximumFractionDigits']]), new Options(Stub::fixtures()));

        self::assertSame(['core/edges', 'core/two-reports', 'intl/plural', 'limits/output-over-limit', 'limits/nesting-over-limit'], self::ids($plan->cases));
        self::assertSame([
            'intl/number' => 'The adapter cannot express the maximumFractionDigits property of a NumberFormat request.',
            'tree/escaped-key' => 'The adapter offers no concrete syntax tree.',
        ], self::reasons($plan->skipped));
    }

    public function testALevelUnclaimedOrNotRunIsLeftOut(): void
    {
        $unclaimed = Runner::plan(Stub::adapter(levels: ['core']), new Options(Stub::fixtures()));
        $unrun = Runner::plan(Stub::adapter(), new Options(Stub::fixtures(), ['core', 'extensions']));

        self::assertSame(['intl/number' => 'The adapter does not claim the intl level.', 'intl/plural' => 'The adapter does not claim the intl level.'], self::reasons($unclaimed->skipped));
        self::assertSame(['intl/number' => 'The intl level is not among the levels being run.', 'intl/plural' => 'The intl level is not among the levels being run.'], self::reasons($unrun->skipped));
    }

    public function testATreeCaseRunsWhateverLevelsAreRun(): void
    {
        $plan = Runner::plan(Stub::adapter(), new Options(Stub::fixtures(), ['core']));

        self::assertContains('tree/escaped-key', self::ids($plan->cases));
    }

    public function testAFileOfAnotherFormatIsRefused(): void
    {
        $file = self::file('[{"id": "a/b", "description": "D.", "message": "m", "expected": {"output": "m"}}]');
        $file->file->format = 'curly-message-2';

        $this->expectExceptionMessage('The fixture file cases.json targets the format "curly-message-2"; this runner reads curly-message-3.');

        Runner::plan(Stub::adapter(), new Options([$file]));
    }

    public function testAnIdentityNamesTheDocumentWhereItIsNotTheSpecification(): void
    {
        $plan = Runner::plan(Stub::adapter(), new Options(Stub::fixtures()));
        $where = [];

        foreach ($plan->cases as $case) {
            $where[$case->id] = [$case->file, $case->level, $case->where()];
        }

        self::assertSame(['core.json', 'core', 'section 8'], $where['core/edges']);
        self::assertSame(['core.json', 'core', 'section 14.3'], $where['core/two-reports']);
        self::assertSame(['tree.json', null, 'CST.md section 6'], $where['tree/escaped-key']);
    }

    /**
     * @param list<Planned> $cases
     * @return list<string>
     */
    private static function ids(array $cases): array
    {
        return array_map(static fn(Planned $case): string => $case->id, $cases);
    }

    /**
     * @param list<Skipped> $skipped
     * @return array<string, string>
     */
    private static function reasons(array $skipped): array
    {
        $reasons = [];

        foreach ($skipped as $case) {
            $reasons[$case->id] = $case->reason;
        }

        return $reasons;
    }
}
