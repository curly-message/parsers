<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit\Internal\Intl;

use CurlyMessage\Internal\Intl\DateText;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class DateTextTest extends TestCase
{
    private string $zone = '';

    protected function setUp(): void
    {
        // A date and a time with no offset are local, so the zone is pinned.
        $this->zone = date_default_timezone_get();
        date_default_timezone_set('Europe/Prague');
    }

    protected function tearDown(): void
    {
        date_default_timezone_set($this->zone);
    }

    /** @return iterable<string, array{string, int|null}> */
    public static function texts(): iterable
    {
        // Each instant is what `Date.parse` answers in Europe/Prague.
        yield 'a year, at UTC' => ['2024', 1704067200000];
        yield 'a month, at UTC' => ['2024-03', 1709251200000];
        yield 'a day, at UTC' => ['2024-03-05', 1709596800000];
        yield 'a six-digit year' => ['+002024-03-05', 1709596800000];
        yield 'a local time' => ['2024-03-05T10:00', 1709629200000];
        yield 'a time at UTC' => ['2024-03-05T10:00Z', 1709632800000];
        yield 'a fraction and an offset' => ['2024-03-05T10:00:00.5+01:00', 1709629200500];
        yield 'a day past the month, carried' => ['2024-02-30', 1709251200000];
        yield 'the midnight that ends a day' => ['2024-03-05T24:00', 1709679600000];
        yield 'a date with its own weekday' => ['Tue, 05 Mar 2024 10:00:00 +0000', 1709632800000];
        yield 'a date and a time PHP reads' => ['2024-03-05 10:00', 1709629200000];
        yield 'a thirteenth month' => ['2024-13', null];
        yield 'a minute past the end of a day' => ['2024-03-05T24:01', null];
        yield 'a weekday the date is not' => ['Wed, 05 Mar 2024 10:00:00 +0000', null];
        yield 'the present' => ['now', null];
        yield 'a span' => ['+1 day', null];
        yield 'a military zone alone' => ['a', null];
        yield 'a day named by its distance' => ['tomorrow', null];
        yield 'nothing' => ['', null];
    }

    #[DataProvider('texts')]
    public function testReadsTheDatesEcmaScriptReadsAndNamesNoOther(string $text, ?int $instant): void
    {
        self::assertSame($instant, DateText::parse($text));
    }
}
