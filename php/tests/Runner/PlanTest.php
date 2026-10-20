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
        $entries = 'The adapter must name what it cannot express of a NumberFormat request as a list of property names, or of entries each naming a property with the values or the request it cannot be expressed in.';

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
        yield 'properties not a list' => [$stub->with(unexpressible: ['NumberFormat' => 'style']), $set, $entries];
        yield 'a property not a name' => [$stub->with(unexpressible: ['NumberFormat' => ['style', 3]]), $set, $entries];
        yield 'an entry naming no property' => [$stub->with(unexpressible: ['NumberFormat' => [['values' => [2]]]]), $set, $entries];
        yield 'an entry listing no value' => [$stub->with(unexpressible: ['NumberFormat' => [['property' => 'style', 'values' => []]]]), $set, $entries];
        yield 'an entry stating an empty condition' => [$stub->with(unexpressible: ['NumberFormat' => [['property' => 'style', 'when' => new \stdClass()]]]), $set, $entries];
        yield 'an entry holding another member' => [$stub->with(unexpressible: ['NumberFormat' => [['property' => 'style', 'unless' => ['a' => 1]]]]), $set, $entries];
        yield 'a negative reentry' => [$stub->with(reentry: -1), $set, 'The adapter must state its bound on counted reads, reentry, as a non-negative integer.'];
        yield 'a reentry not an integer' => [$stub->with(reentry: 1.0), $set, 'The adapter must state its bound on counted reads, reentry, as a non-negative integer.'];
        yield 'keeps not a boolean' => [$stub->with(keeps: 0), $set, 'The adapter must state whether a modifier can keep an input past its call, keeps, as true or false.'];
        yield 'resolutions not a boolean' => [$stub->with(reentry: 3, resolutions: 'true'), $set, 'The adapter must state whether its bound counts a resolution asked for inside another, resolutions, as true or false.'];
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

    public function testAnEntryNarrowedByValueOrRequestLeavesOutOnlyTheRequestsItCovers(): void
    {
        $format = static fn(string $options): string => '{"message": "{{n:number}}", "payload": {"n": 1}, "expected": {"format": {"api": "NumberFormat", "options": ' . $options . ', "input": 1}}}';
        $fixtures = [self::file('[' . implode(', ', [
            '{"id": "two", "description": "Two digits.", ' . substr($format('{"maximumFractionDigits": 2, "useGrouping": false}'), 1),
            '{"id": "grouped", "description": "Grouped.", ' . substr($format('{"maximumFractionDigits": 2, "useGrouping": true}'), 1),
            '{"id": "three", "description": "Three digits.", ' . substr($format('{"maximumFractionDigits": 3, "useGrouping": false}'), 1),
            '{"id": "negative-zero", "description": "A negative zero.", ' . substr($format('{"maximumFractionDigits": -0.0}'), 1),
            '{"id": "zero", "description": "A zero.", ' . substr($format('{"maximumFractionDigits": 0}'), 1),
        ]) . ']', 'intl')];
        $plan = Runner::plan(Stub::adapter()->with(unexpressible: ['NumberFormat' => [
            ['property' => 'maximumFractionDigits', 'values' => [2], 'when' => ['useGrouping' => false]],
            ['property' => 'maximumFractionDigits', 'values' => [-0.0]],
        ]]), new Options($fixtures));

        self::assertSame(['grouped', 'three', 'zero'], self::ids($plan->cases));
        self::assertSame([
            'two' => 'The adapter cannot express the value 2 of the maximumFractionDigits property of a NumberFormat request whose useGrouping is false.',
            'negative-zero' => 'The adapter cannot express the value -0 of the maximumFractionDigits property of a NumberFormat request.',
        ], self::reasons($plan->skipped));
    }

    public function testACaseWhoseInputsRefuseToBeReadIsLeftOut(): void
    {
        $fixtures = [self::file('[
            {"id": "refusing", "description": "Props that refuse.", "message": "{{v:e}}", "payload": {"v": "x"}, "props": {"e": {"$curly": "refusing", "members": ["a"]}}, "expected": {"output": "x"}},
            {"id": "plain", "description": "Props that do not.", "message": "{{v:e}}", "payload": {"v": "x"}, "props": {"e": {"a": 1}}, "expected": {"output": "x"}}
        ]')];
        $plan = Runner::plan(Stub::adapter(), new Options($fixtures));

        self::assertSame(['plain'], self::ids($plan->cases));
        self::assertSame(['refusing' => 'PHP has no value the implementation reads whose entries raise where they are read.'], self::reasons($plan->skipped));
    }

    public function testACaseOfKeptReadsIsLeftOutWhereTheAdapterStatesItKeepsNoneOrNoBound(): void
    {
        $written = static fn(string $id, string $keeping): string => '{"id": "' . $id . '", "description": "Kept.", "message": "{{x:a}}", "payload": {"x": ""}, "keeping": {"a": "' . $keeping . '"}, "expected": {"output": ""}}';
        $generated = static fn(string $name): string => '{"id": "' . $name . '", "description": "Built.", "generate": "' . $name . '"}';
        $fixtures = [self::file('[' . implode(', ', [
            $written('keep', 'keep'), $written('first', 'first'), $written('take', 'take'), $written('nest', 'nest'), $written('keep-nest', 'keep-nest'), $written('keep-nest-ne', 'keep-nest-ne'),
            $generated('reentry-at-limit'), $generated('reentry-own-reads-free'), $generated('reentry-own-read-in-a-nested-resolution'), $generated('reentry-over-limit-beneath-a-defined-modifier'), $generated('reentry-resolutions-over-limit'), $generated('output-at-limit'),
        ]) . ']', 'extensions')];
        $plan = static fn(Adapter $adapter): array => self::reasons(Runner::plan($adapter, new Options($fixtures))->skipped);
        $keeps = 'The adapter states that a modifier cannot keep an input past its call.';
        $unbound = 'The adapter states no bound on counted reads.';
        $zero = 'The adapter states a bound of 0 on counted reads, so ';
        $nested = $zero . 'the resolution a modifier asks for inside another may be refused.';
        $uncounted = 'The adapter does not state that its bound counts a resolution asked for inside another.';

        $unbounded = ['reentry-at-limit' => $unbound, 'reentry-own-reads-free' => $unbound, 'reentry-own-read-in-a-nested-resolution' => $unbound, 'reentry-over-limit-beneath-a-defined-modifier' => $unbound, 'reentry-resolutions-over-limit' => $unbound];

        self::assertSame($unbounded, $plan(Stub::adapter()));
        self::assertSame($unbounded, $plan(Stub::adapter()->with(resolutions: true)));
        self::assertSame(['reentry-resolutions-over-limit' => $uncounted], $plan(Stub::adapter()->with(reentry: 1)));
        self::assertSame(['reentry-resolutions-over-limit' => $uncounted], $plan(Stub::adapter()->with(reentry: 1, resolutions: false)));
        self::assertSame([], $plan(Stub::adapter()->with(reentry: 1, resolutions: true)));
        // The stub's output limit of 5 holds the brackets of two resolutions
        // and the text between them, not of four.
        self::assertSame(
            ['reentry-resolutions-over-limit' => 'The output limit the adapter states does not hold the brackets of as many resolutions as its bound lets stand.'],
            $plan(Stub::adapter()->with(reentry: 3, resolutions: true)),
        );
        // Built from resolutions, the two framed constructions read nothing
        // kept after its modifier returned.
        self::assertSame([
            'keep' => $keeps, 'first' => $keeps, 'reentry-at-limit' => $keeps, 'reentry-own-reads-free' => $keeps,
        ], $plan(Stub::adapter()->with(reentry: 1, keeps: false, resolutions: true)));
        self::assertSame([
            'keep' => $keeps, 'first' => $keeps, 'reentry-at-limit' => $keeps, 'reentry-own-reads-free' => $keeps,
            'reentry-own-read-in-a-nested-resolution' => $keeps, 'reentry-over-limit-beneath-a-defined-modifier' => $keeps, 'reentry-resolutions-over-limit' => $uncounted,
        ], $plan(Stub::adapter()->with(reentry: 1, keeps: false)));
        self::assertSame([
            'nest' => $nested,
            'keep-nest' => $nested,
            'keep-nest-ne' => $nested,
            'reentry-own-reads-free' => $zero . 'no counted read stands for own reads to be made inside.',
            'reentry-own-read-in-a-nested-resolution' => $nested,
            'reentry-over-limit-beneath-a-defined-modifier' => $nested,
        ], $plan(Stub::adapter()->with(reentry: 0, resolutions: true)));
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
