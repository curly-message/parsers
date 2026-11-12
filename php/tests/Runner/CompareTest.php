<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Runner;

use CurlyMessage\Conformance\Adapter;
use CurlyMessage\Conformance\Performer;
use CurlyMessage\Conformance\Resolution;
use CurlyMessage\Conformance\Runner;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class CompareTest extends TestCase
{
    use Cases;

    private const PLAIN = '[{"id": "a/plain", "description": "D.", "message": "m", "expected": {"output": " text "}}]';

    private const REPORTED = '[{"id": "a/reported", "description": "D.", "message": "m", "messageId": "m", "expected": {"output": "x", "reports": [{"code": "unknown-modifier", "origin": "message", "id": "m"}, {"code": "failed-modifier", "origin": "payload"}]}}]';

    private const NUMBER = '[{"id": "a/number", "description": "D.", "message": "{{n:number}}", "locale": "en", "expected": {"format": {"api": "NumberFormat", "options": {"style": "percent", "maximumFractionDigits": 2}, "input": 1}}}]';

    private const AGO = '[{"id": "a/ago", "description": "D.", "message": "{{n:ago}}", "locale": "en", "expected": {"format": {"api": "RelativeTimeFormat", "options": {}, "input": [-0, "day"]}}}]';

    private const PLURAL = '[{"id": "a/plural", "description": "D.", "message": "{{n:plural}}", "locale": "cs", "expected": {"format": {"api": "PluralRules", "options": {"type": "cardinal"}, "input": 1.5}, "categories": {"zero": "z", "one": "o", "two": "t", "few": "f", "many": "m", "other": "x"}}}]';

    /** @return iterable<string, array{string, mixed, string}> */
    public static function outputs(): iterable
    {
        yield 'the same text' => [self::PLAIN, ['output' => ' text ', 'reports' => []], 'passed'];
        yield 'the text trimmed' => [self::PLAIN, ['output' => 'text', 'reports' => []], 'The output differs.'];
        yield 'the text decomposed' => [self::PLAIN, ['output' => " te\u{301}xt ", 'reports' => []], 'The output differs.'];
        yield 'no reports observed' => [self::PLAIN, ['output' => ' text '], 'unobserved'];
        yield 'reports not a list' => [self::PLAIN, ['output' => ' text ', 'reports' => ['a' => []]], 'The reports are not a list.'];
        yield 'an output not text' => [self::PLAIN, ['output' => 5, 'reports' => []], 'The adapter answered with no resolution.'];
        yield 'no array' => [self::PLAIN, (object) ['output' => ' text '], 'The adapter answered with no resolution.'];
        yield 'nothing' => [self::PLAIN, null, 'The adapter answered with no resolution.'];
    }

    #[DataProvider('outputs')]
    public function testTheOutputIsComparedExactly(string $cases, mixed $answer, string $outcome): void
    {
        self::assertSame($outcome, self::outcome(self::over(self::answering(static fn(): mixed => $answer), [self::file($cases)])));
    }

    public function testARaiseFailsItsCaseAndTheRunGoesOn(): void
    {
        $cases = '[{"id": "a/one", "description": "D.", "message": "one", "expected": {"output": "one"}}, {"id": "a/two", "description": "D.", "message": "two", "expected": {"output": "two"}}]';
        $adapter = self::answering(static fn(Resolution $input): array => $input->message === 'one' ? throw new \RuntimeException('Raised.') : ['output' => 'two', 'reports' => []]);
        $result = self::over($adapter, [self::file($cases)]);

        self::assertSame('The adapter raised.', $result->failed[0]->outcome->reason);
        self::assertSame('Raised.', $result->failed[0]->outcome->actual);
        self::assertSame('a/two', $result->passed[0]->id);
    }

    /** @return iterable<string, array{list<mixed>, string}> */
    public static function reports(): iterable
    {
        $first = ['code' => 'unknown-modifier', 'origin' => 'message', 'id' => 'm'];
        $second = ['code' => 'failed-modifier', 'origin' => 'payload', 'id' => 'm'];

        yield 'as stated' => [[$first, $second], 'passed'];
        yield 'with no field but the code' => [[['code' => 'unknown-modifier'], ['code' => 'failed-modifier']], 'passed'];
        yield 'with fields carried as null' => [[['code' => 'unknown-modifier', 'origin' => null, 'id' => null], $second], 'passed'];
        yield 'with an id no expectation names' => [[['id' => ['m']] + $first, $second], 'passed'];
        yield 'as objects' => [[(object) $first, (object) $second], 'passed'];
        yield 'one fewer' => [[$first], 'Expected 2 reports, got 1 report.'];
        yield 'one more' => [[$first, $second, $second], 'Expected 2 reports, got 3 reports.'];
        yield 'reversed' => [[$second, $first], 'The code of report 1 differs.'];
        yield 'from another origin' => [[['origin' => 'payload'] + $first, $second], 'The origin of report 1 differs.'];
        yield 'naming another message' => [[['id' => 'n'] + $first, $second], 'The id of report 1 differs.'];
        yield 'not a report' => [['unknown-modifier', $second], 'The code of report 1 differs.'];
    }

    /** @param list<mixed> $reports */
    #[DataProvider('reports')]
    public function testReportsAreComparedInOrderWhereTheyAreCarried(array $reports, string $outcome): void
    {
        $adapter = self::answering(static fn(): array => ['output' => 'x', 'reports' => $reports]);

        self::assertSame($outcome, self::outcome(self::over($adapter, [self::file(self::REPORTED)])));
    }

    public function testALimitIsComparedWhereTheReportCarriesOne(): void
    {
        $limits = self::file('[{"id": "limits/over", "description": "D.", "generate": "output-over-limit"}]');
        $carrying = static fn(mixed $limit): \Closure => static fn(): array => ['output' => 'AB', 'reports' => [['code' => 'output-limit', 'origin' => 'limit', 'id' => 'limits', 'limit' => $limit]]];

        self::assertSame('passed', self::outcome(self::over(self::answering($carrying(5)), [$limits])));
        self::assertSame('passed', self::outcome(self::over(self::answering($carrying(5.0)), [$limits])));
        self::assertSame('passed', self::outcome(self::over(self::answering($carrying(null)), [$limits])));
        self::assertSame('The limit of report 1 differs.', self::outcome(self::over(self::answering($carrying(6)), [$limits])));
    }

    /** @return iterable<string, array{string, mixed, string}> */
    public static function requests(): iterable
    {
        $number = ['api' => 'NumberFormat', 'options' => ['maximumFractionDigits' => 2, 'style' => 'percent'], 'input' => 1.0];

        yield 'in another order, by value' => [self::NUMBER, [$number], 'passed'];
        yield 'as an object' => [self::NUMBER, [(object) ['api' => 'NumberFormat', 'options' => (object) ['style' => 'percent', 'maximumFractionDigits' => 2.0], 'input' => 1]], 'passed'];
        yield 'none' => [self::NUMBER, [], 'Expected one formatting request, got 0.'];
        yield 'two' => [self::NUMBER, [$number, $number], 'Expected one formatting request, got 2.'];
        yield 'not a list' => [self::NUMBER, ['a' => $number], 'The formatting requests are not a list.'];
        yield 'of another facility' => [self::NUMBER, [['api' => 'PluralRules'] + $number], 'The facility of the request differs.'];
        yield 'a property more' => [self::NUMBER, [['options' => ['useGrouping' => false] + $number['options']] + $number], 'The properties of the request differ.'];
        yield 'a property of another type' => [self::NUMBER, [['options' => ['maximumFractionDigits' => '2', 'style' => 'percent']] + $number], 'The properties of the request differ.'];
        yield 'another input' => [self::NUMBER, [['input' => 2] + $number], 'The input of the request differs.'];
        yield 'an input of another type' => [self::NUMBER, [['input' => '1'] + $number], 'The input of the request differs.'];
        yield 'no properties for none' => [self::AGO, [['api' => 'RelativeTimeFormat', 'input' => [0, 'day']]], 'passed'];
        yield 'an empty map for none' => [self::AGO, [['api' => 'RelativeTimeFormat', 'options' => [], 'input' => [0.0, 'day']]], 'passed'];
        yield 'a pair as a map' => [self::AGO, [['api' => 'RelativeTimeFormat', 'options' => [], 'input' => ['value' => 0, 'unit' => 'day']]], 'The input of the request differs.'];
        yield 'a pair reversed' => [self::AGO, [['api' => 'RelativeTimeFormat', 'options' => [], 'input' => ['day', 0]]], 'The input of the request differs.'];
    }

    #[DataProvider('requests')]
    public function testAnExposedRequestIsComparedByItsEntries(string $cases, mixed $formats, string $outcome): void
    {
        $adapter = self::answering(static fn(): array => ['output' => 'whatever the host made', 'reports' => [], 'formats' => $formats]);

        self::assertSame($outcome, self::outcome(self::over($adapter, [self::file($cases, 'intl')])));
    }

    /** @return iterable<string, array{mixed, string, string}> */
    public static function selections(): iterable
    {
        yield 'the output of its category' => ['many', 'm', 'passed'];
        yield 'the output of another category' => ['many', 'x', 'The output differs from the one the category many selects.'];
        yield 'no category' => [null, 'm', 'The selection request carries no category the plural rules name.'];
        yield 'a category the rules do not name' => ['several', 'm', 'The selection request carries no category the plural rules name.'];
    }

    #[DataProvider('selections')]
    public function testAnExposedSelectionHoldsTheOutputToItsCategory(mixed $category, string $output, string $outcome): void
    {
        $request = ['api' => 'PluralRules', 'options' => ['type' => 'cardinal'], 'input' => 1.5, 'category' => $category];
        $adapter = self::answering(static fn(): array => ['output' => $output, 'reports' => [], 'formats' => [$request]]);

        self::assertSame($outcome, self::outcome(self::over($adapter, [self::file(self::PLURAL, 'intl')])));
    }

    public function testWithoutRequestsTheRunnerPerformsTheCaseOnItsOwnHost(): void
    {
        $performer = new class implements Performer {
            /** @var list<array{array{api: string, options: \stdClass, input: mixed}, string|null}> */
            public array $asked = [];

            public function perform(array $request, ?string $locale): string
            {
                $this->asked[] = [$request, $locale];

                return $request['api'] === 'PluralRules' ? 'many' : '100%';
            }
        };
        $text = static fn(string $output): \Closure => static fn(): array => ['output' => $output, 'reports' => []];

        self::assertSame('passed', self::outcome(self::over(self::answering($text('100%')), [self::file(self::NUMBER, 'intl')], $performer)));
        self::assertSame('The output differs.', self::outcome(self::over(self::answering($text('1')), [self::file(self::NUMBER, 'intl')], $performer)));
        self::assertSame('passed', self::outcome(self::over(self::answering($text('m')), [self::file(self::PLURAL, 'intl')], $performer)));
        self::assertSame('The output differs from the one the category many selects.', self::outcome(self::over(self::answering($text('o')), [self::file(self::PLURAL, 'intl')], $performer)));
        self::assertSame('NumberFormat', $performer->asked[0][0]['api']);
        self::assertEquals((object) ['style' => 'percent', 'maximumFractionDigits' => 2], $performer->asked[0][0]['options']);
        self::assertSame('en', $performer->asked[0][1]);
        self::assertSame('cs', $performer->asked[2][1]);
    }

    public function testWithoutRequestsOrAPerformerTheRunIsRefused(): void
    {
        $this->expectExceptionMessage('The case a/number states a request the adapter does not expose, and the run was given nothing to perform it with.');

        self::over(self::answering(static fn(): array => ['output' => '100%']), [self::file(self::NUMBER, 'intl')]);
    }

    public function testASelectionNamingNoOutputForACategoryEndsTheRun(): void
    {
        $incomplete = str_replace('"zero": "z", ', '', self::PLURAL);

        $this->expectExceptionMessage('The case a/plural names no output for the category "zero".');

        self::over(self::answering(static fn(): array => ['output' => 'm', 'formats' => []]), [self::file($incomplete, 'intl')]);
    }

    public function testTheInputsAreDecodedAndWhatACaseLeavesOutIsAbsent(): void
    {
        $cases = '[{"id": "a/inputs", "description": "D.", "message": "m", "payload": {"a": {"$curly": "undefined"}, "b": [{}]}, "props": {"p": {"$curly": "undefined"}}, "modifiers": {"up": "upper"}, "expected": {"output": "m"}}]';
        $seen = [];
        $adapter = new Adapter(['core'], Stub::LIMITS, static function (Resolution $input) use (&$seen): array {
            $seen[] = $input;

            return ['output' => 'm'];
        }, absent: 'ABSENT');
        self::over($adapter, [self::file($cases)]);

        self::assertCount(1, $seen);
        self::assertEquals((object) ['a' => 'ABSENT', 'b' => [new \stdClass()]], $seen[0]->payload);
        self::assertEquals((object) ['p' => 'ABSENT'], $seen[0]->props);
        self::assertSame('ABSENT', $seen[0]->locale);
        self::assertSame('ABSENT', $seen[0]->id);
        self::assertSame('ABSENT', $seen[0]->defaults);
        self::assertSame(['up'], array_keys($seen[0]->modifiers));
    }

    public function testAFailureLineIsBoundedAndNamesTheSection(): void
    {
        $long = str_repeat("\u{e9}", 150);
        $result = self::over(self::answering(static fn(): array => ['output' => $long]), [self::file(self::PLAIN)]);
        $line = Runner::summarize($result);

        self::assertStringStartsWith('FAIL a/plain (section 14.3): The output differs. Expected " text ", actual "', $line);
        self::assertStringContainsString('actual "' . str_repeat("\u{e9}", 99) . '...', $line);
        self::assertStringNotContainsString(str_repeat("\u{e9}", 100), $line);
        self::assertSame(1, preg_match('//u', $line));
        self::assertStringEndsWith("D.\n0 passed, 1 failed, 0 skipped", $line);
    }
}
