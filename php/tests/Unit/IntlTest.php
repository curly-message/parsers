<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\Internal\Intl\Api;
use CurlyMessage\Internal\Intl\Cache;
use CurlyMessage\Internal\Intl\Request;
use CurlyMessage\Internal\Requests;
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
        self::assertSame([Api::NumberFormat, ['maximumFractionDigits' => 2], 1234.5, null], [$number->api, $number->options->toArray(), $number->input, $number->category]);
        self::assertSame([Api::PluralRules, ['type' => 'cardinal', 'maximumFractionDigits' => 2], 1, 'one'], [$plural->api, $plural->options->toArray(), $plural->input, $plural->category]);
        self::assertSame([], $this->codes());
    }

    public function testObservesARefusedRequestWithNoCategory(): void
    {
        $parser = $this->parser();

        self::assertSame('D', $parser->resolve('{{c:plural; one:ONE; default:D}}', ['c' => 1], ['plural' => ['maximumFractionDigits' => 200]], 'en'));
        self::assertCount(1, $this->requests);
        self::assertSame(Api::PluralRules, $this->requests[0]->api);
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
        $parser = Requests::observe(new Parser(), static function (Request $request): never {
            throw new \RuntimeException('observer');
        });

        self::assertSame('1.5', $parser->resolve('{{n:number}}', ['n' => 1.5], locale: 'en'));
        $parser = Requests::observe(new Parser(), static function (Request $request): never {
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
        self::assertSame(['numeric' => 'always'], $this->requests[5]->options->toArray());
        self::assertSame(['numeric' => 'auto'], $this->requests[6]->options->toArray());
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
        self::assertSame(['type' => 'cardinal', 'maximumFractionDigits' => 2, 'minimumFractionDigits' => 1], $this->requests[1]->options->toArray());
        self::assertSame('nd', $parser->resolve('{{n:ordinal; one:st; two:nd; few:rd; other:th}}', ['n' => 22], locale: 'en'));
        self::assertSame('two', $this->requests[2]->category);
        self::assertSame('OTHER', $parser->resolve('{{n:plural; one:ONE; other:OTHER}}', ['n' => 21], locale: 'en'));
    }

    // The digits a wrapper holds under `number` lie over those every
    // placeholder shares, whether or not a layer names anything under
    // `plural` itself.
    public function testSelectsByTheDigitsAWrapperHoldsOverThoseEveryPlaceholderShares(): void
    {
        $parser = $this->parser();
        $message = '{{n:plural; one:ONE; many:MANY; other:OTHER}}';
        $shared = ['number' => ['minimumFractionDigits' => 1]];
        $wrapped = ['n' => ['value' => 1, 'props' => ['number' => ['minimumFractionDigits' => 0]]]];

        self::assertSame('MANY', $parser->resolve($message, ['n' => 1], $shared, 'cs'));
        self::assertSame('ONE', $parser->resolve($message, $wrapped, $shared, 'cs'));
        self::assertSame('ONE', $parser->resolve($message, $wrapped, $shared + ['plural' => ['localeMatcher' => 'lookup']], 'cs'));
        // The near miss: a digit `plural` is configured with lies over both.
        self::assertSame('MANY', $parser->resolve($message, $wrapped, $shared + ['plural' => ['minimumFractionDigits' => 1]], 'cs'));
    }

    public function testFormatsCurrencyAtItsRatio(): void
    {
        $parser = $this->parser();

        self::assertSame("\u{20AC}25.00", $parser->resolve('{{n:currency}}', ['n' => 25], ['currency' => ['currency' => 'EUR', 'style' => 'percent']], 'en'));
        self::assertSame("\u{20AC}50.00", $parser->resolve('{{n:currency}}', ['n' => 25], ['currency' => ['currency' => 'EUR', 'ratio' => '2']], 'en'));
        self::assertSame(['style' => 'currency', 'currency' => 'EUR'], $this->requests[1]->options->toArray());
        self::assertSame(50, $this->requests[1]->input);
        self::assertSame('D', $parser->resolve('{{n:currency; default:D}}', ['n' => 25], ['currency' => ['currency' => 'EUR', 'ratio' => 'x']], 'en'));
        self::assertSame(['failed-modifier'], $this->codes());
        self::assertCount(2, $this->requests);
        // Numeric text is read with its sign, a negative zero included.
        self::assertSame("-\u{20AC}0.00", $parser->resolve('{{n:currency}}', ['n' => 5], ['currency' => ['currency' => 'EUR', 'ratio' => '-0.0']], 'en'));
        // A ratio is a property rather than a value, so it is read as PHP's
        // arithmetic reads an operand: ASCII whitespace around it, which
        // `is_numeric` skips, and none of the rest of section 6's class.
        self::assertSame("\u{20AC}50.00", $parser->resolve('{{n:currency}}', ['n' => 25], ['currency' => ['currency' => 'EUR', 'ratio' => " 2\n"]], 'en'));
        self::assertSame('D', $parser->resolve('{{n:currency; default:D}}', ['n' => 25], ['currency' => ['currency' => 'EUR', 'ratio' => "\u{3000}2"]], 'en'));
    }

    // What a ratio costs to read is not the package's to grow with its
    // length on every placeholder: nothing charges a property to the read
    // limit. So padding no `is_numeric` skips is not walked either, however
    // long. Run apart, since what is counted is PHP's own function, shadowed
    // in the package's namespace there.
    public function testReadsNoCharacterOfARatioItself(): void
    {
        $short = self::multiplied('1');
        $long = self::multiplied('10000');

        self::assertSame(['intl' => true, 'output' => str_repeat('D', 10), 'read' => $short['read'] ?? null], array_diff_key($long, ['marked' => true, 'memory' => true]));
    }

    // A numeric option is converted as ECMAScript's `ToNumber` converts it,
    // section 6's class around it dropped. So its padding is dropped without
    // a step per character: an option just past the key's length reads as
    // many as one far longer.
    public function testWalksNoCharacterOfANumericOptionsPadding(): void
    {
        $short = self::multiplied('30', 'digits');
        $long = self::multiplied('10000', 'digits');

        self::assertSame(['intl' => true, 'output' => str_repeat('1.6', 10), 'read' => $short['read'] ?? null], array_diff_key($long, ['marked' => true, 'memory' => true]));
    }

    // A value is read as a number with section 6's class dropped from its
    // ends, as an option is, so no modifier walks its padding a character at
    // a time: a value padded up to the read limit reads as many characters as
    // one barely padded.
    public function testWalksNoCharacterOfAValuesPadding(): void
    {
        foreach (['number' => '1.5', 'date' => '1/1/70'] as $modifier => $shown) {
            $short = self::multiplied('10', $modifier);
            $long = self::multiplied('24000', $modifier);

            self::assertSame(['intl' => true, 'output' => $shown, 'read' => $short['read'] ?? null], array_diff_key($long, ['marked' => true, 'memory' => true]), $modifier);
        }
    }

    // A resolution remembers the number a property's text converts to by
    // that text, which the payload holds already, so what it keeps does not
    // grow with the placeholders. A list joins to a text of its own, so one
    // of several entries is converted at each request instead: 50 wrappers,
    // each a list repeating one text of 1 MiB twenty times, peak as 10 do,
    // under the memory limit the 20 MiB joins would exhaust if each were
    // kept. A text, or a list of that one text, of its own in each wrapper
    // is remembered by the payload's own string, so it grows the
    // resolution's peak by no copy of it either.
    public function testRemembersNoTextThePayloadDoesNotHold(): void
    {
        $mebibyte = 1 << 20;

        foreach (['shared', 'texts', 'singles'] as $property) {
            $few = self::multiplied('10', $property);
            $many = self::multiplied('50', $property);

            self::assertSame([str_repeat('D', 10), str_repeat('D', 50)], [$few['output'] ?? null, $many['output'] ?? null], $property);
            self::assertIsInt($few['memory'] ?? null);
            self::assertIsInt($many['memory'] ?? null);
            self::assertLessThan($mebibyte, $many['memory'] - $few['memory'], $property);
        }
    }

    // Only the ends of an option are read for its padding: a quoted option
    // marks as many bytes however long it is, and a padded one reads its
    // padding and one slice past it twice, once for the blanks of one byte
    // and once for those of more. A resolution converts each text once, so
    // the ten placeholders read it as one does, whether the request is built
    // for each, read twice for `number`'s own digits, alternates between
    // wrappers, or the text is a list's.
    public function testReadsAnOptionsPaddingAndLittleMore(): void
    {
        $short = self::multiplied('1024', 'quoted');
        $long = self::multiplied('1048576', 'quoted');

        self::assertSame(['intl' => true, 'output' => str_repeat('D', 10), 'marked' => $short['marked'] ?? null], array_diff_key($long, ['read' => true, 'memory' => true]));

        foreach ([1 << 20, 2 << 20] as $padding) {
            foreach (['blanks' => '1.6', 'alternating' => '1.56', 'listed' => '1.6'] as $property => $shown) {
                $padded = self::multiplied((string) $padding, $property);

                self::assertSame(str_repeat($shown, 10), $padded['output'] ?? null, $property);
                self::assertLessThan(2 * (1.01 * $padding + 65536), $padded['marked'] ?? null, $property);
            }
        }
    }

    /**
     * What `tests/Scripts/multiplying.php` answers for the padding and the
     * property, decoded.
     * It runs on this process's binary and ini files rather than under `-n`,
     * so ext-intl is loaded there exactly as it is here: where an
     * installation keeps it, or whether it is built in, differs from one to
     * the next.
     *
     * @return array<array-key, mixed>
     */
    private static function multiplied(string $padding, string $property = 'ratio'): array
    {
        $process = proc_open(
            [PHP_BINARY, '-d', 'display_errors=stderr', '-d', 'memory_limit=128M', dirname(__DIR__) . '/Scripts/multiplying.php', $padding, $property],
            [1 => ['pipe', 'w'], 2 => ['pipe', 'w']],
            $pipes,
        );

        self::assertIsResource($process);
        $answer = (string) stream_get_contents($pipes[1]);
        $errors = (string) stream_get_contents($pipes[2]);

        self::assertSame([0, ''], [proc_close($process), $errors], $answer);
        self::assertJson($answer);
        $decoded = json_decode($answer, true);
        self::assertIsArray($decoded);

        return $decoded;
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

    /** @return iterable<string, array{string, array<string, string>, array<string, string|bool>}> */
    public static function styledCycles(): iterable
    {
        yield 'h23 at en' => ['en', ['timeStyle' => 'short'], ['hourCycle' => 'h23']];
        yield 'hour12 false at en' => ['en', ['timeStyle' => 'short'], ['hour12' => false]];
        yield 'h12 at pt-BR' => ['pt-BR', ['timeStyle' => 'short'], ['hourCycle' => 'h12']];
        yield 'hour12 true at de' => ['de', ['timeStyle' => 'short'], ['hour12' => true]];
        yield 'h23 at zh-Hant' => ['zh-Hant', ['timeStyle' => 'medium'], ['hourCycle' => 'h23']];
        yield 'h11 at ja with a date' => ['ja', ['dateStyle' => 'short', 'timeStyle' => 'short'], ['hourCycle' => 'h11']];
        yield 'h24 at hi with a date' => ['hi', ['dateStyle' => 'full', 'timeStyle' => 'long'], ['hourCycle' => 'h24']];
    }

    /**
     * @param array<string, string> $style
     * @param array<string, string|bool> $cycle
     */
    #[DataProvider('styledCycles')]
    public function testLeavesAnHourCycleUnappliedToAStyle(string $locale, array $style, array $cycle): void
    {
        $parser = $this->parser();

        // A style's pattern keeps the day period its hour cycle writes, so an
        // hour cycle is not applied to it, and the style is formatted as the
        // locale writes it.
        foreach ([1709643900000, 1709600700000] as $instant) {
            $plain = $parser->resolve('{{v:date}}', ['v' => $instant], ['date' => ['timeZone' => 'UTC'] + $style], $locale);
            self::assertSame($plain, $parser->resolve('{{v:date}}', ['v' => $instant], ['date' => ['timeZone' => 'UTC'] + $style + $cycle], $locale));
        }

        self::assertSame($cycle, array_intersect_key($this->requests[1]->options->toArray(), $cycle));
        self::assertSame([], $this->codes());
    }

    public function testExpressesAnHourCycleBesideADateStyleAlone(): void
    {
        $parser = $this->parser();
        $date = ['timeZone' => 'UTC', 'dateStyle' => 'medium'];

        // A date shows no hour, so an hour cycle beside it changes nothing.
        foreach ([['hourCycle' => 'h23'], ['hour12' => true]] as $cycle) {
            self::assertSame(
                $parser->resolve('{{v:date}}', ['v' => 1709643900000], ['date' => $date], 'en'),
                $parser->resolve('{{v:date}}', ['v' => 1709643900000], ['date' => $date + $cycle], 'en'),
            );
            $this->requests = [];
        }

        $declared = Parser::unexpressible()['DateTimeFormat'];
        $covered = static fn(array $options): bool => array_filter($declared, static fn(string|array $entry): bool => is_array($entry)
            && array_key_exists($entry['property'], $options)
            && array_filter($entry['when'], static fn(string $value, string $name): bool => ($options[$name] ?? null) === $value, ARRAY_FILTER_USE_BOTH) === $entry['when']) !== [];

        self::assertFalse($covered($date + ['hourCycle' => 'h23']));
        self::assertTrue($covered($date + ['timeStyle' => 'short', 'hourCycle' => 'h23']));
        self::assertSame([], $this->codes());
    }

    public function testAppliesAnHourCycleToComponentsAndReadsItWithAStyle(): void
    {
        $parser = $this->parser();
        $props = ['timeZone' => 'UTC', 'hour' => 'numeric', 'minute' => '2-digit'];

        self::assertSame('13:05', $parser->resolve('{{v:date}}', ['v' => 1709643900000], ['date' => $props + ['hourCycle' => 'h23']], 'en'));
        self::assertSame('D', $parser->resolve('{{v:date; default:D}}', ['v' => 1709643900000], ['date' => ['timeStyle' => 'short', 'hourCycle' => 'h25']], 'en'));
        self::assertSame(['failed-modifier'], $this->codes());
    }

    public function testWritesTheHourAsWideAsTheLocalesPattern(): void
    {
        $parser = $this->parser();

        // ICU's pattern for the request decides the hour's width, whichever
        // of `numeric` and `2-digit` asked for it.
        foreach (['en', 'de', 'pt-BR', 'ja'] as $locale) {
            foreach ([[], ['minute' => '2-digit']] as $minute) {
                foreach ([[], ['hourCycle' => 'h12'], ['hourCycle' => 'h23']] as $cycle) {
                    $props = ['timeZone' => 'UTC'] + $minute + $cycle;
                    $numeric = $parser->resolve('{{v:date}}', ['v' => 1709600700000], ['date' => $props + ['hour' => 'numeric']], $locale);
                    self::assertSame($numeric, $parser->resolve('{{v:date}}', ['v' => 1709600700000], ['date' => $props + ['hour' => '2-digit']], $locale));
                }
            }
        }

        self::assertSame('01:05', $parser->resolve('{{v:date}}', ['v' => 1709600700000], ['date' => ['timeZone' => 'UTC', 'hour' => 'numeric', 'minute' => '2-digit']], 'de'));
        self::assertSame('1:05', $parser->resolve('{{v:date}}', ['v' => 1709600700000], ['date' => ['timeZone' => 'UTC', 'hour' => '2-digit', 'minute' => '2-digit']], 'ja'));
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

        foreach (['hourCycle', 'hour12'] as $property) {
            foreach (['full', 'long', 'medium', 'short'] as $value) {
                self::assertContains(['property' => $property, 'when' => ['timeStyle' => $value]], $unexpressible['DateTimeFormat']);
                self::assertNotContains(['property' => $property, 'when' => ['dateStyle' => $value]], $unexpressible['DateTimeFormat']);
            }
        }

        self::assertNotContains('hourCycle', $unexpressible['DateTimeFormat']);
        self::assertCount(10, $unexpressible['DateTimeFormat']);
    }
}
