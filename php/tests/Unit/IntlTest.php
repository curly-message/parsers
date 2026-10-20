<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\Facility;
use CurlyMessage\FormatRequest;
use CurlyMessage\Internal\Intl\Cache;
use CurlyMessage\ModifierInput;
use CurlyMessage\Parser;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\Attributes\RequiresPhpExtension;
use PHPUnit\Framework\TestCase;

#[RequiresPhpExtension('intl')]
final class IntlTest extends TestCase
{
    use Resolves;

    private string $locale = '';

    private string $zone = '';

    protected function setUp(): void
    {
        // A locale with no data formats at the process's default, and a
        // local time is read in the process's zone, so both are pinned.
        $this->locale = \Locale::getDefault();
        $this->zone = date_default_timezone_get();
        \Locale::setDefault('en_US');
        date_default_timezone_set('UTC');
        Cache::clear();
    }

    protected function tearDown(): void
    {
        \Locale::setDefault($this->locale);
        date_default_timezone_set($this->zone);
        Cache::clear();
    }

    public function testExposesEachRequestWithTheCategoryASelectionAnswered(): void
    {
        $parser = $this->parser();

        self::assertSame('1,234.5 ONE', $parser->resolve('{{n:number}} {{c:plural; one:ONE; other:OTHER}}', ['n' => 1234.5, 'c' => 1], locale: 'en'));
        self::assertCount(2, $this->requests);
        [$number, $plural] = $this->requests;
        self::assertSame([Facility::NumberFormat, ['maximumFractionDigits' => 2], 1234.5, null], [$number->api, $number->options, $number->input, $number->category]);
        self::assertSame([Facility::PluralRules, ['type' => 'cardinal', 'maximumFractionDigits' => 2], 1, 'one'], [$plural->api, $plural->options, $plural->input, $plural->category]);
        self::assertSame([], $this->codes());
    }

    public function testObservesARefusedRequestWithNoCategory(): void
    {
        $parser = $this->parser();

        self::assertSame('D', $parser->resolve('{{c:plural; one:ONE; default:D}}', ['c' => 1], ['plural' => ['maximumFractionDigits' => 200]], 'en'));
        self::assertCount(1, $this->requests);
        self::assertSame(Facility::PluralRules, $this->requests[0]->api);
        self::assertNull($this->requests[0]->category);
        self::assertSame(['failed-modifier'], $this->codes());
    }

    public function testMakesNoRequestWhereTheLocaleTheValueOrAKeyDecidesFirst(): void
    {
        $parser = $this->parser();

        self::assertSame('[]', $parser->resolve('[{{n:number; default:D}}]', ['n' => 1]));
        self::assertSame('[]', $parser->resolve('[{{n:number; default:D}}]', ['n' => 1], locale: ''));
        self::assertSame(['missing-locale', 'missing-locale'], $this->codes());
        $this->reports = [];

        self::assertSame('D', $parser->resolve('{{n:number; default:D}}', ['n' => ' '], locale: 'en'));
        self::assertSame('D', $parser->resolve('{{n:number; default:D}}', ['n' => '12px'], locale: 'en'));
        self::assertSame('D', $parser->resolve('{{n:number; default:D}}', ['n' => '1e400'], locale: 'en'));
        self::assertSame('D', $parser->resolve('{{n:ordinal; one:st; default:D}}', ['n' => 1.5], locale: 'en'));
        self::assertSame(['failed-modifier', 'failed-modifier', 'failed-modifier', 'failed-modifier'], $this->codes());
        $this->reports = [];

        // A key equal to the number selects before any category is asked.
        self::assertSame('EXACT', $parser->resolve('{{n:plural; one:ONE; 1.0:EXACT}}', ['n' => '1'], locale: 'en'));
        self::assertSame([], $this->requests);
        self::assertSame([], $this->codes());
    }

    public function testContainsAnObserverThatThrows(): void
    {
        $parser = new Parser(onRequest: static function (FormatRequest $request): never {
            throw new \RuntimeException('observer');
        });

        self::assertSame('1.5', $parser->resolve('{{n:number}}', ['n' => 1.5], locale: 'en'));
        $parser = new Parser(onRequest: static function (FormatRequest $request): never {
            throw new Rethrowing(1);
        });

        self::assertSame('1.5', $parser->resolve('{{n:number}}', ['n' => 1.5], locale: 'en'));
    }

    /** @return iterable<string, array{string, array<array-key, mixed>}> */
    public static function refusals(): iterable
    {
        yield 'a minimum above the maximum' => ['number', ['minimumFractionDigits' => 3, 'maximumFractionDigits' => 2]];
        yield 'an increment that is none' => ['number', ['roundingIncrement' => 3]];
        yield 'an increment over unequal digits' => ['number', ['roundingIncrement' => 5, 'maximumFractionDigits' => 2]];
        yield 'an increment with significant digits' => ['number', ['roundingIncrement' => 5, 'maximumSignificantDigits' => 2]];
        yield 'a grouping of no allowed value' => ['number', ['useGrouping' => 'yes']];
        yield 'a numbering system that is no type' => ['number', ['numberingSystem' => 'f']];
        yield 'a style of no allowed value' => ['number', ['style' => 'fancy']];
        yield 'a currency style with no currency' => ['currency', []];
        yield 'a currency that is no code' => ['currency', ['currency' => 'EURO']];
        yield 'an object as a digit count' => ['number', ['maximumFractionDigits' => new \ArrayObject()]];
        yield 'a style and a component' => ['date', ['dateStyle' => 'short', 'hour' => 'numeric']];
        yield 'a zone nobody knows' => ['date', ['timeZone' => 'Mars/Olympus']];
        yield 'a calendar that is no type' => ['date', ['calendar' => 'x']];
        yield 'a relative style of no allowed value' => ['ago', ['style' => 'tiny']];
        yield 'a null numeric' => ['ago', ['numeric' => null]];
    }

    /** @param array<array-key, mixed> $props */
    #[DataProvider('refusals')]
    public function testRefusesWhatEcmaScriptRefuses(string $modifier, array $props): void
    {
        $parser = $this->parser();

        self::assertSame('D', $parser->resolve("{{n:{$modifier}; default:D}}", ['n' => 1709632800000], [$modifier => $props], 'en'));
        self::assertSame(['failed-modifier'], $this->codes());
        self::assertCount(1, $this->requests);
    }

    public function testReadsOptionsAsEcmaScriptConvertsThem(): void
    {
        $parser = $this->parser();

        // ToNumber(null) is 0, and "true" is the fallback of a grouping.
        self::assertSame('2', $parser->resolve('{{n:number}}', ['n' => 1.5], ['number' => ['maximumFractionDigits' => null]], 'en'));
        self::assertSame('1,234.5', $parser->resolve('{{n:number}}', ['n' => 1234.5], ['number' => ['useGrouping' => 'true']], 'en'));
        self::assertSame('1234.5', $parser->resolve('{{n:number}}', ['n' => 1234.5], ['number' => ['useGrouping' => false]], 'en'));
        self::assertSame('1,234.50', $parser->resolve('{{n:number}}', ['n' => 1234.5], ['number' => ['minimumFractionDigits' => '2']], 'en'));
        // An option ECMAScript does not name is ignored.
        self::assertSame('1.5', $parser->resolve('{{n:number}}', ['n' => 1.5], ['number' => ['shape' => 'round']], 'en'));
        self::assertSame([], $this->codes());
    }

    /** @return iterable<string, array{string, string, string}> */
    public static function locales(): iterable
    {
        yield 'not a tag' => ['not a locale', 'D', 'D'];
        yield 'not a character of one' => ['x!!', 'D', 'D'];
        yield 'a language with no data' => ['zz', '1,234.5', '3/5/2024'];
        yield 'a language ICU lists a code for and has no data' => ['tlh', '1,234.5', '3/5/2024'];
        yield 'a language reserved for local use' => ['qaa', '1,234.5', '3/5/2024'];
        yield 'an underscore' => ['en_US', '1,234.5', '3/5/2024'];
        yield 'a hyphen' => ['en-US', '1,234.5', '3/5/2024'];
        yield 'a numbering keyword' => ['en-US-u-nu-arab', "\u{0661}\u{066C}\u{0662}\u{0663}\u{0664}\u{066B}\u{0665}", "\u{0663}/\u{0665}/\u{0662}\u{0660}\u{0662}\u{0664}"];
        yield 'an ICU keyword' => ['de@collation=phonebook', 'D', 'D'];
        yield 'a grandfathered tag' => ['i-klingon', 'D', 'D'];
        yield 'a singleton twice' => ['en-a-x-a-y', 'D', 'D'];
        yield 'a variant twice' => ['de-1996-1996', 'D', 'D'];
    }

    #[DataProvider('locales')]
    public function testChecksTheLocaleAlikeInEveryFacility(string $locale, string $number, string $date): void
    {
        $parser = $this->parser();

        self::assertSame($number, $parser->resolve('{{n:number; default:D}}', ['n' => 1234.5], locale: $locale));
        self::assertSame($date, $parser->resolve('{{n:date; default:D}}', ['n' => 1709632800000], ['date' => ['timeZone' => 'UTC']], $locale));
        self::assertSame($number === 'D' ? ['failed-modifier', 'failed-modifier'] : [], $this->codes());
    }

    public function testGroupsByTheDigitsShownAfterRounding(): void
    {
        $parser = $this->parser();
        $props = ['number' => ['maximumFractionDigits' => 0]];

        self::assertSame("10\u{00A0}000", $parser->resolve('{{n:number}}', ['n' => 9999.6], $props, 'pl'));
        self::assertSame('9999', $parser->resolve('{{n:number}}', ['n' => 9999.4], $props, 'pl'));
        self::assertSame("-12\u{00A0}345", $parser->resolve('{{n:number}}', ['n' => -12345], $props, 'pl'));
        self::assertSame('1234', $parser->resolve('{{n:number}}', ['n' => 1234], $props, 'pl'));
        self::assertSame("1\u{00A0}234", $parser->resolve('{{n:number}}', ['n' => 1234], ['number' => ['useGrouping' => 'always']], 'pl'));
    }

    public function testRoundsHalfAwayFromZeroByDefault(): void
    {
        $parser = $this->parser();
        $whole = ['number' => ['maximumFractionDigits' => 0]];

        self::assertSame('0.13', $parser->resolve('{{n:number}}', ['n' => 0.125], locale: 'en'));
        self::assertSame('3', $parser->resolve('{{n:number}}', ['n' => 2.5], $whole, 'en'));
        self::assertSame('-3', $parser->resolve('{{n:number}}', ['n' => -2.5], $whole, 'en'));
        self::assertSame('2', $parser->resolve('{{n:number}}', ['n' => 2.5], ['number' => ['maximumFractionDigits' => 0, 'roundingMode' => 'halfEven']], 'en'));
        self::assertSame('12.35%', $parser->resolve('{{n:number}}', ['n' => 0.12345], ['number' => ['style' => 'percent']], 'en'));
    }

    public function testClimbsTheLadderAlikeIntoThePastAndTheFuture(): void
    {
        $parser = $this->parser();
        $always = ['ago' => ['numeric' => 'always']];

        self::assertSame('in 1 hour', $parser->resolve('{{n:ago}}', ['n' => 1800000], $always, 'en'));
        self::assertSame('1 hour ago', $parser->resolve('{{n:ago}}', ['n' => -1800000], $always, 'en'));
        self::assertSame('in 2 hours', $parser->resolve('{{n:ago}}', ['n' => 5400000], $always, 'en'));
        self::assertSame('2 hours ago', $parser->resolve('{{n:ago}}', ['n' => -5400000], $always, 'en'));
        self::assertSame('in 0 seconds', $parser->resolve('{{n:ago}}', ['n' => 0.5], $always, 'en'));
        self::assertSame('0 seconds ago', $parser->resolve('{{n:ago}}', ['n' => -0.5], $always, 'en'));
        self::assertSame('now', $parser->resolve('{{n:ago}}', ['n' => -0.5], locale: 'en'));
        self::assertSame('in 90 minutes', $parser->resolve('{{n:ago}}', ['n' => 5400000], ['ago' => ['format' => 'minutes']], 'en'));

        // A step that rounds to nothing in the past keeps its sign.
        $past = $this->requests[5]->input;
        self::assertIsArray($past);
        self::assertSame('second', $past[1]);
        self::assertIsFloat($past[0]);
        self::assertLessThan(0, fdiv(1.0, $past[0]));
        self::assertSame(['numeric' => 'always'], $this->requests[5]->options);
        self::assertSame(['numeric' => 'auto'], $this->requests[6]->options);
        self::assertSame([90, 'minute'], $this->requests[7]->input);
        self::assertSame([], $this->codes());
    }

    public function testKeepsTheSignOfANegativeZero(): void
    {
        $props = ['ago' => ['numeric' => 'always', 'format' => 'second']];

        self::assertSame('-0|-0|0 seconds ago', $this->parser()->resolve('{{v:number}}|{{w:number}}|{{v:ago}}', ['v' => '-0.0', 'w' => '-0e0'], $props, 'en'));
        self::assertSame('0|in 0 seconds', $this->parser()->resolve('{{v:number}}|{{v:ago}}', ['v' => '0.0'], $props, 'en'));
    }

    public function testKeepsACountPastPhpIntegersInTheFuture(): void
    {
        $parser = $this->parser();

        self::assertStringStartsWith('in ', $parser->resolve('{{n:ago}}', ['n' => '9223372036854775808000'], ['ago' => ['format' => 'second']], 'en'));
        $input = $this->requests[0]->input;
        self::assertIsArray($input);
        self::assertSame(9223372036854775808.0, $input[0]);
    }

    public function testRefusesAFormatOffTheLadder(): void
    {
        $parser = $this->parser();

        self::assertSame('D', $parser->resolve('{{n:ago; default:D}}', ['n' => 1000], ['ago' => ['format' => 'quarter']], 'en'));
        self::assertSame('D', $parser->resolve('{{n:ago; default:D}}', ['n' => 1000], ['ago' => ['format' => 'Minutes']], 'en'));
        self::assertSame(['failed-modifier', 'failed-modifier'], $this->codes());
        self::assertSame([], $this->requests);
    }

    public function testWritesTheLocalesPhrases(): void
    {
        $parser = $this->parser();

        self::assertSame('yesterday', $parser->resolve('{{n:ago}}', ['n' => -86400000], locale: 'en'));
        self::assertSame('in 2 days', $parser->resolve('{{n:ago}}', ['n' => 172800000], locale: 'en'));
        self::assertSame('za 2 dny', $parser->resolve('{{n:ago}}', ['n' => 172800000], ['ago' => ['numeric' => 'always']], 'cs'));
    }

    public function testSelectsByTheDigitsANumberIsShownWith(): void
    {
        $parser = $this->parser();
        $message = '{{n:plural; one:ONE; many:MANY; other:OTHER}}';

        self::assertSame('ONE', $parser->resolve($message, ['n' => 1], locale: 'cs'));
        self::assertSame('MANY', $parser->resolve($message, ['n' => 1], ['number' => ['minimumFractionDigits' => 1, 'style' => 'percent']], 'cs'));
        // Only the digits `number` is configured with reach `plural`.
        self::assertSame(['type' => 'cardinal', 'minimumFractionDigits' => 1, 'maximumFractionDigits' => 2], $this->requests[1]->options);
        self::assertSame('nd', $parser->resolve('{{n:ordinal; one:st; two:nd; few:rd; other:th}}', ['n' => 22], locale: 'en'));
        self::assertSame('two', $this->requests[2]->category);
        self::assertSame('OTHER', $parser->resolve('{{n:plural; one:ONE; other:OTHER}}', ['n' => 21], locale: 'en'));
    }

    public function testFormatsCurrencyAtItsRatio(): void
    {
        $parser = $this->parser();

        self::assertSame("\u{20AC}25.00", $parser->resolve('{{n:currency}}', ['n' => 25], ['currency' => ['currency' => 'EUR', 'style' => 'percent']], 'en'));
        self::assertSame("\u{20AC}50.00", $parser->resolve('{{n:currency}}', ['n' => 25], ['currency' => ['currency' => 'EUR', 'ratio' => '2']], 'en'));
        self::assertSame(['style' => 'currency', 'currency' => 'EUR'], $this->requests[1]->options);
        self::assertSame(50, $this->requests[1]->input);
        self::assertSame('D', $parser->resolve('{{n:currency; default:D}}', ['n' => 25], ['currency' => ['currency' => 'EUR', 'ratio' => 'x']], 'en'));
        self::assertSame(['failed-modifier'], $this->codes());
        self::assertCount(2, $this->requests);
        // Numeric text is read with its sign, a negative zero included.
        self::assertSame("-\u{20AC}0.00", $parser->resolve('{{n:currency}}', ['n' => 5], ['currency' => ['currency' => 'EUR', 'ratio' => '-0.0']], 'en'));
    }

    public function testFormatsADateTheTextNames(): void
    {
        $parser = $this->parser();
        $props = ['date' => ['dateStyle' => 'medium', 'timeZone' => 'UTC']];

        self::assertSame('Mar 5, 2024', $parser->resolve('{{n:date}}', ['n' => '2024-03-05'], $props, 'en'));
        self::assertSame('Mar 5, 2024', $parser->resolve('{{n:date}}', ['n' => 'Tue, 05 Mar 2024 10:00:00 +0000'], $props, 'en'));
        self::assertSame('D', $parser->resolve('{{n:date; default:D}}', ['n' => 'tomorrow'], $props, 'en'));
        self::assertSame('D', $parser->resolve('{{n:date; default:D}}', ['n' => 8640000000000001], $props, 'en'));
        self::assertSame(['failed-modifier', 'failed-modifier'], $this->codes());
        self::assertSame(1709596800000, $this->requests[0]->input);
    }

    public function testKeepsWhatItBuildsWithinItsBounds(): void
    {
        $parser = new Parser();

        for ($at = 0; $at < 300; $at += 1) {
            $parser->resolve('{{n:number}}', ['n' => 1], ['number' => ['minimumIntegerDigits' => 1 + $at % 21, 'maximumFractionDigits' => intdiv($at, 21)]], 'en');
        }

        $kept = Cache::size();
        self::assertLessThan(300, $kept);

        // A request spelled longer than any option is written is not kept.
        self::assertSame('1.0', $parser->resolve('{{n:number}}', ['n' => 1], ['number' => ['minimumFractionDigits' => str_pad('1', 65, '0', STR_PAD_LEFT)]], 'en'));
        self::assertSame($kept, Cache::size());
    }

    // ECMAScript reads the names a facility knows and no other, so a request
    // is kept by those alone, and props a payload names in any number keep
    // nothing more.
    public function testKeepsARequestByTheOptionsItsFacilityReads(): void
    {
        $parser = new Parser();
        $unread = static fn(string $prefix): array => array_fill_keys(array_map(static fn(int $at): string => $prefix . $at, range(1, 100)), str_repeat('x', 60));

        self::assertSame('1.5', $parser->resolve('{{n:number}}', ['n' => ['value' => 1.5, 'props' => ['number' => $unread('a')]]], locale: 'en'));
        $kept = Cache::size();
        self::assertSame('1.5', $parser->resolve('{{n:number}}', ['n' => ['value' => 1.5, 'props' => ['number' => $unread('b')]]], locale: 'en'));
        self::assertSame($kept, Cache::size());
    }

    // A part too long to be kept is measured before it is spelled, so a long
    // one a payload hands over costs the request nothing it would copy; a
    // string of NUL bytes is spelled twelve times as long as it is.
    public function testSpellsNoKeyOfAPartTooLongToKeep(): void
    {
        $parser = new Parser();
        $currency = str_repeat("\0", 1_000_000);
        $payload = ['n' => ['value' => 5, 'props' => ['number' => ['style' => 'currency', 'currency' => $currency]]]];
        memory_reset_peak_usage();
        $before = memory_get_usage();

        self::assertSame('D', $parser->resolve('{{n:number; default:D}}', $payload, locale: 'en'));
        self::assertLessThan(strlen($currency), memory_get_peak_usage() - $before);
    }

    // A request is kept where its locale is no longer than 64 bytes as it is
    // written: a tag that long keeps its formatter and its own reading as
    // `en` does, and one a byte longer keeps neither.
    public function testKeepsARequestByALocaleOfUpTo64Bytes(): void
    {
        $parser = new Parser();
        $long = 'en-x-' . implode('-', str_split(str_repeat('a', 64), 8));
        $parser->resolve('{{n:number}}', ['n' => 1.5], locale: 'en');
        $kept = Cache::size();

        foreach ([64 => $kept, 65 => $kept - 2] as $length => $size) {
            Cache::clear();
            self::assertSame('1.5', $parser->resolve('{{n:number}}', ['n' => 1.5], locale: substr($long, 0, $length)));
            self::assertSame($size, Cache::size());
        }
    }

    public function testYieldsTheNamesToAHostsOwn(): void
    {
        $parser = $this->parser(customModifiers: ['number' => static fn(ModifierInput $in): string => 'host:' . $in->value]);

        self::assertSame('host:1', $parser->resolve('{{n:number}}', ['n' => 1], locale: 'en'));
        self::assertSame([], $this->requests);
        // A host's `plural` is no selection of the format's, so it may
        // answer from no options.
        $parser = $this->parser(customModifiers: ['plural' => static fn(ModifierInput $in): string => 'P']);
        self::assertSame('P', $parser->resolve('{{n:plural}}', ['n' => 1], locale: 'en'));
        self::assertSame([], $this->codes());
    }

    public function testDeclaresTheLevelAndWhatItCannotExpress(): void
    {
        self::assertSame(['core', 'intl', 'extensions'], Parser::levels());

        $unexpressible = Parser::unexpressible();
        self::assertSame(['NumberFormat', 'DateTimeFormat', 'RelativeTimeFormat', 'PluralRules'], array_keys($unexpressible));
        self::assertContains('notation', $unexpressible['NumberFormat']);
        self::assertContains('localeMatcher', $unexpressible['PluralRules']);
        self::assertSame(version_compare(INTL_ICU_VERSION, '69', '<'), in_array('roundingMode', $unexpressible['NumberFormat'], true));
        self::assertNotContains('roundingMode', $unexpressible['DateTimeFormat']);
    }
}
