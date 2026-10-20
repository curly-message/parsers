<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Runner;

use CurlyMessage\Conformance\CstOffer;
use CurlyMessage\Conformance\Tree;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class TreeTest extends TestCase
{
    use Cases;

    // One character of each width: one, two, three and four bytes.
    private const MESSAGE = "a\u{e9}\u{20ac}\u{1f600}";

    private const CASE = '[{"id": "tree/text", "description": "D.", "message": "ab", "expected": [{"type": "text", "text": "ab"}]}]';

    /** @return iterable<string, array{string, list<int|null>}> */
    public static function units(): iterable
    {
        yield 'utf-8' => ['utf-8', [0, 1, null, 3, null, null, 6, null, null, null, 10]];
        yield 'utf-16' => ['utf-16', [0, 1, 3, 6, null, 10]];
        yield 'code-point' => ['code-point', [0, 1, 3, 6, 10]];
    }

    /** @param list<int|null> $offsets */
    #[DataProvider('units')]
    public function testEachBoundaryIsAByteOffsetAndNoneFallsInsideACodePoint(string $unit, array $offsets): void
    {
        self::assertSame($offsets, Tree::boundaries(self::MESSAGE, $unit));
    }

    public function testAByteThatBeginsNoSequenceIsACharacterOfItsOwn(): void
    {
        self::assertSame([0, 1, 2], Tree::boundaries("\xC3a", 'code-point'));
        self::assertSame([0, 1, 2], Tree::boundaries("\xE0\x80", 'utf-8'));
    }

    /** @return iterable<string, array{mixed, string}> */
    public static function trees(): iterable
    {
        $text = ['type' => 'text', 'start' => 0, 'end' => 2];
        $root = static fn(array $nodes): array => ['type' => 'message', 'start' => 0, 'end' => 2, 'nodes' => $nodes];

        yield 'the text' => [$root([$text]), 'passed'];
        yield 'as objects' => [(object) ['type' => 'message', 'start' => 0, 'end' => 2, 'nodes' => [(object) $text]], 'passed'];
        yield 'no children as null' => [$root([['nodes' => null] + $text]), 'passed'];
        yield 'no tree' => [['type' => 'text', 'start' => 0, 'end' => 2], 'The adapter answered with no tree.'];
        yield 'short of the message' => [['end' => 1] + $root([$text]), 'The tree does not span the whole message.'];
        yield 'a child not a node' => [$root(['text']), 'The tree holds something that is not a node.'];
        yield 'a kind nobody names' => [$root([['type' => 'word'] + $text]), 'The tree holds a node of no kind this document names.'];
        yield 'a span not of offsets' => [$root([['end' => '2'] + $text]), 'The span of the text node is not a pair of offsets.'];
        yield 'a span reversed' => [$root([['start' => 2, 'end' => 0] + $text]), 'The text node ends before it starts.'];
        yield 'a span outside the message' => [$root([['end' => 3] + $text]), 'The text node lies outside the message.'];
        yield 'children not a list' => [$root([['nodes' => 'none'] + $text]), 'The children of the text node are not a list.'];
        yield 'a gap' => [$root([['end' => 1] + $text]), 'The leaves do not reach the end of the message.'];
        yield 'an overlap' => [$root([['end' => 1] + $text, $text]), 'The leaves do not tile the message: a "text" node starts where the one before it did not end.'];
        yield 'a key with no name' => [$root([['type' => 'key'] + $text]), 'The key node carries no name.'];
        yield 'an escape that takes no reading' => [$root([['type' => 'escape'] + $text]), 'The escape node does not say which reading it takes.'];
        yield 'another kind than expected' => [$root([['type' => 'space'] + $text]), 'Node 1 of the message is of another kind.'];
        yield 'more nodes than expected' => [$root([['end' => 1] + $text, ['start' => 1] + $text]), 'The children of the message are 2 nodes rather than 1.'];
    }

    #[DataProvider('trees')]
    public function testATreeIsHeldToTheDocumentBeforeTheCase(mixed $tree, string $outcome): void
    {
        $adapter = self::answering(static fn(): array => ['output' => 'ab'], new CstOffer('utf-8', static fn(): mixed => $tree));

        self::assertSame($outcome, self::outcome(self::over($adapter, [self::treeFile(self::CASE)])));
    }

    public function testATreeThatHoldsItselfFailsWithinTheBudget(): void
    {
        $visited = 0;
        $parse = static function () use (&$visited): array {
            $node = ['type' => 'message', 'start' => 0, 'end' => 2];
            $node['nodes'] = [&$node];
            $visited += 1;

            return $node;
        };
        $adapter = self::answering(static fn(): array => ['output' => 'ab'], new CstOffer('utf-8', $parse));
        $result = self::over($adapter, [self::treeFile(self::CASE)]);

        self::assertSame('The tree holds more nodes than the message can spell.', self::outcome($result));
        self::assertSame('at most 72 nodes', $result->failed[0]->outcome->expected);
        self::assertSame(1, $visited);
    }

    public function testATreeIsAFunctionOfTheMessageAlone(): void
    {
        $calls = 0;
        $parse = static function () use (&$calls): array {
            $calls += 1;

            return ['type' => 'message', 'start' => 0, 'end' => 2, 'nodes' => [['type' => $calls === 1 ? 'text' : 'space', 'start' => 0, 'end' => 2]]];
        };
        $cases = '[{"id": "tree/text", "description": "D.", "message": "ab", "expected": [{"type": "text", "text": "ab"}, {"type": "text", "text": ""}]}]';
        $adapter = self::answering(static fn(): array => ['output' => 'ab'], new CstOffer('utf-8', $parse));

        self::assertSame('The tree is not a function of the message alone: two parses of it differ.', self::outcome(self::over($adapter, [self::treeFile(self::CASE)])));
        self::assertSame(2, $calls);
        self::assertSame('The children of the message are 1 nodes rather than 2.', self::outcome(self::over($adapter, [self::treeFile($cases)])));
    }

    public function testARaiseFailsTheCase(): void
    {
        $adapter = self::answering(static fn(): array => ['output' => 'ab'], new CstOffer('utf-8', static fn(): never => throw new \RuntimeException('No tree.')));

        self::assertSame('The adapter raised rather than answering with a tree.', self::outcome(self::over($adapter, [self::treeFile(self::CASE)])));
    }

    public function testTheTreeAndTheResolutionReadTheSameMessage(): void
    {
        $cases = '[{"id": "tree/text", "description": "D.", "message": "ab", "expected": [{"type": "text", "text": "ab"}], "resolves": "ab"}]';
        $tree = new CstOffer('utf-8', static fn(): array => ['type' => 'message', 'start' => 0, 'end' => 2, 'nodes' => [['type' => 'text', 'start' => 0, 'end' => 2]]]);

        self::assertSame('passed', self::outcome(self::over(self::answering(static fn(): array => ['output' => 'ab'], $tree), [self::treeFile($cases)])));
        self::assertSame('The tree and the resolution do not read the same placeholders.', self::outcome(self::over(self::answering(static fn(): array => ['output' => 'a'], $tree), [self::treeFile($cases)])));
        self::assertSame('The adapter raised resolving the message the tree describes.', self::outcome(self::over(self::answering(static fn(): never => throw new \RuntimeException('No.'), $tree), [self::treeFile($cases)])));
    }

    public function testASpanInsideACodePointFails(): void
    {
        $cases = '[{"id": "tree/text", "description": "D.", "message": "\u00e9", "expected": [{"type": "text", "text": "\u00e9"}]}]';
        $tree = new CstOffer('utf-8', static fn(): array => ['type' => 'message', 'start' => 0, 'end' => 2, 'nodes' => [['type' => 'text', 'start' => 0, 'end' => 1], ['type' => 'text', 'start' => 1, 'end' => 2]]]);

        self::assertSame('The span of the text node falls inside a code point.', self::outcome(self::over(self::answering(static fn(): array => ['output' => ''], $tree), [self::treeFile($cases)])));
    }
}
