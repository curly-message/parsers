<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\Origin;
use CurlyMessage\Parser;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class LimitsTest extends TestCase
{
    use Apart;
    use Resolves;

    public function testCarriesAResultThatFillsTheOutputExactly(): void
    {
        $value = str_repeat('a', Parser::MAX_OUTPUT_LENGTH);

        self::assertSame("<{$value}>", $this->resolve('<{{v}}>', ['v' => $value]));
        self::assertSame([], $this->codes());
    }

    public function testDropsAResultPastTheOutputLimitAndCarriesTheNextThatFits(): void
    {
        $value = str_repeat('a', Parser::MAX_OUTPUT_LENGTH - 1);
        $output = $this->resolve('{{v}}<{{s; a:bc}}>{{s; a:d}}', ['v' => $value, 's' => 'a']);

        self::assertTrue($output === $value . '<>d', 'the result with no room is dropped, and the next is carried');
        self::assertSame(['output-limit'], $this->codes());
        self::assertSame(Origin::Limit, $this->reports[0]->origin);
        self::assertSame(Parser::MAX_OUTPUT_LENGTH, $this->reports[0]->limit);
        self::assertSame('{{s; a:bc}}', $this->reports[0]->text);
    }

    public function testCountsWhatTheOutermostWalkPlacesOnce(): void
    {
        $value = str_repeat('a', Parser::MAX_OUTPUT_LENGTH);

        self::assertSame($value, $this->resolve('{{v; a:{{w}}}}', ['v' => 'a', 'w' => $value]));
        self::assertSame([], $this->codes());
    }

    public function testResolvesThePlaceholderThatSpendsTheReadBudgetAndRefusesTheNext(): void
    {
        $payload = ['v' => str_repeat('a', Parser::MAX_READ_LENGTH + 1), 'w' => 'W'];

        self::assertSame('D', $this->resolve('{{v; b:B; default:D}}{{w; default:E}}', $payload));
        self::assertSame(['read-limit'], $this->codes());
        self::assertSame(Parser::MAX_READ_LENGTH, $this->reports[0]->limit);
        self::assertSame('{{w; default:E}}', $this->reports[0]->text);
    }

    public function testReadsUpToTheReadLimitExactly(): void
    {
        $payload = ['v' => str_repeat('a', Parser::MAX_READ_LENGTH - 1), 'w' => 'W', 'x' => 'X'];

        self::assertSame('DWX', $this->resolve('{{v; b:B; default:D}}{{w}}{{x}}', $payload));
        self::assertSame([], $this->codes());
        $payload['v'] .= 'a';
        self::assertSame('DW', $this->resolve('{{v; b:B; default:D}}{{w}}{{x}}', $payload));
        self::assertSame(['read-limit'], $this->codes());
    }

    public function testSerializesUpToTheConversionLimit(): void
    {
        // The list is one node and each element another. The value is read
        // and compared, never carried, so no other limit is reached.
        $list = array_fill(0, Parser::MAX_CONVERSION_NODES - 1, 0);

        self::assertSame('D', $this->resolve('{{v; x:X; default:D}}', ['v' => $list]));
        self::assertSame([], $this->codes());
        $list[] = 0;
        self::assertSame('D', $this->resolve('{{v; x:X; default:D}}', ['v' => $list]));
        self::assertSame(['unserializable-value'], $this->codes());
    }

    /** @return iterable<string, array{\Closure(int): mixed, int}> */
    public static function wide(): iterable
    {
        // Telling whether a list with a hole is a list walks it up to the
        // hole, and listing a `stdClass` walks every property.
        yield 'an array' => [static function (int $count): array {
            $array = range(0, $count);
            unset($array[$count - 1]);

            return $array;
        }, Parser::MAX_CONVERSION_NODES - 1];
        yield 'a stdClass in a list' => [static function (int $count): array {
            $object = new \stdClass();

            for ($at = 0; $at < $count; $at += 1) {
                $object->{"p{$at}"} = $at;
            }

            return [$object];
        }, Parser::MAX_CONVERSION_NODES - 2];
    }

    // A container past the limit is refused at the same bound wherever it
    // is read, however many places read it, the second time as the first.
    // One that fits is read whole, which spends the read budget.
    /** @param \Closure(int): mixed $build */
    #[DataProvider('wide')]
    public function testRefusesAContainerPastTheConversionLimitAtEveryPlaceItIsRead(\Closure $build, int $fits): void
    {
        self::assertSame('D', $this->resolve('{{a; x:X; default:D}}', ['a' => $build($fits)]));
        self::assertSame([], $this->codes());
        $value = $build($fits + 1);
        self::assertSame('DDD', $this->resolve('{{a; x:X; default:D}}{{b; x:X; default:D}}{{a; x:X; default:D}}', ['a' => $value, 'b' => $value]));
        self::assertSame(['unserializable-value', 'unserializable-value', 'unserializable-value'], $this->codes());
    }

    public function testResolvesEveryLevelUpToTheNestingLimit(): void
    {
        $message = str_repeat('{{v; a:', Parser::MAX_NESTING - 1) . '{{w}}' . str_repeat('}}', Parser::MAX_NESTING - 1);

        self::assertSame('W', $this->resolve($message, ['v' => 'a', 'w' => 'W']));
        self::assertSame([], $this->codes());
    }

    public function testRefusesALevelPastTheNestingLimitToItsChain(): void
    {
        $message = str_repeat('{{v; a:', Parser::MAX_NESTING) . '{{w; default:I}}' . str_repeat('}}', Parser::MAX_NESTING);

        self::assertSame('I', $this->resolve($message, ['v' => 'a', 'w' => 'W']));
        self::assertSame(['nesting-limit'], $this->codes());
        self::assertSame(Origin::Message, $this->reports[0]->origin);
        self::assertSame(Parser::MAX_NESTING, $this->reports[0]->limit);
        self::assertSame('P', $this->resolve($message, ['v' => 'a', 'w' => ['default' => 'P']]));
        self::assertSame('Q', $this->resolve($message, ['v' => 'a', 'default' => 'Q']));
    }

    /** @return iterable<string, array{int}> */
    public static function depths(): iterable
    {
        foreach ([1350, 1400, 3000, 50000] as $depth) {
            yield "{$depth} levels" => [$depth];
        }
    }

    // Each refused level reports and takes its inline default, which holds
    // the next: a walk that recursed per level would run out of stack here.
    #[DataProvider('depths')]
    public function testWalksADefaultChainNestedPastTheLimitWithoutRecursing(int $depth): void
    {
        $message = str_repeat('{{a; default:', $depth) . 'x' . str_repeat('}}', $depth);
        $parser = $this->parser();

        self::assertSame('x', $parser->resolve($message, []));
        self::assertSame(['nesting-limit'], array_values(array_unique($this->codes())));
        self::assertCount($depth - Parser::MAX_NESTING, $this->reports);
    }

    /** @return iterable<string, array{int}> */
    public static function counts(): iterable
    {
        yield 'two thousand levels' => [2000];
        yield 'four thousand levels' => [4000];
    }

    // What a placeholder declares is asked of memory before it is parsed,
    // read off the `;` it writes itself: each byte is read for the one
    // placeholder it belongs to, however deep the chain nests, not again for
    // every level it is nested in. Run apart, since what is counted is
    // PHP's own function, shadowed in the package's namespace there.
    #[DataProvider('counts')]
    public function testReadsEachByteOfADefaultChainOnceForWhatItDeclares(int $depth): void
    {
        $answered = self::apart('counting', (string) $depth, '128M');

        self::assertIsArray($answered);
        self::assertSame('D', $answered['output'] ?? null);
        self::assertIsInt($answered['message'] ?? null);
        self::assertLessThanOrEqual($answered['message'], $answered['counted'] ?? null);
    }

    // A key beginning with NUL is looked for among every entry of a
    // `stdClass`, which one resolution enumerates once, however often its
    // message names such a key. Run apart, since what is counted is PHP's
    // own function, shadowed in the package's namespace there.
    public function testEnumeratesAPayloadOnceForTheKeysBeginningWithNulItIsAskedFor(): void
    {
        $answered = self::apart('enumerating', '10000', '128M');

        self::assertSame(['output' => 'D', 'enumerated' => 10000], $answered);
    }

    // A refused placeholder renders none of its options, so the levels an
    // option value nests past the limit are never walked.
    public function testWalksNoOptionValueNestedPastTheLimit(): void
    {
        $depth = 50000;
        $message = str_repeat('{{v; a:', $depth) . 'x' . str_repeat('}}', $depth);

        self::assertSame('', $this->resolve($message, ['v' => 'a']));
        self::assertSame(['nesting-limit'], $this->codes());
    }

    public function testUnescapesTheTextOfARefusedLevel(): void
    {
        $message = str_repeat('{{a; default:\;', 10) . 'x' . str_repeat('}}', 10);

        self::assertSame(str_repeat(';', 10) . 'x', $this->resolve($message, []));
        self::assertCount(2, $this->reports);
    }

}
