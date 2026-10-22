<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\Cst;
use CurlyMessage\Cst\Node;
use CurlyMessage\Cst\NodeType;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

final class CstTest extends TestCase
{
    public function testStatesItsUnit(): void
    {
        self::assertSame('utf-8', self::unit(Cst::class));
    }

    public function testEncodesAMessageAsCstMdDoes(): void
    {
        self::assertSame([
            'type' => 'message', 'start' => 0, 'end' => 7,
            'nodes' => [
                ['type' => 'escape', 'start' => 0, 'end' => 2, 'cancels' => true],
                ['type' => 'text', 'start' => 2, 'end' => 7],
            ],
        ], Cst::parse('\\{world')->toArray());
    }

    public function testDescribesAPlaceholderInsideAnother(): void
    {
        $tree = Cst::parse('{{count:gt; 0:{{count:number;}}; default:no;}}');
        $outer = self::only($tree, NodeType::Placeholder);

        self::assertSame([0, 46], [$outer->start, $outer->end]);
        self::assertSame(['count', 'gt', '0', 'default', ''], self::names($outer));

        $value = self::all($outer, NodeType::OptionValue)[0];
        self::assertSame([14, 31], [$value->start, $value->end]);

        $inner = self::only($value, NodeType::Placeholder);
        self::assertSame([14, 31], [$inner->start, $inner->end]);
    }

    /** @return iterable<string, array{string, list<array{string, int, int}>}> */
    public static function consumedBraces(): iterable
    {
        yield 'escaped opening brace' => ['\\{{v}}', [['escape', 0, 2], ['text', 2, 6]]];
        yield 'escaped closing brace' => ['{{v\\}}', [['text', 0, 3], ['escape', 3, 5], ['text', 5, 6]]];
    }

    /** @param list<array{string, int, int}> $leaves */
    #[DataProvider('consumedBraces')]
    public function testDescribesABraceABackslashConsumedAsText(string $message, array $leaves): void
    {
        self::assertSame($leaves, array_map(static fn(Node $node): array => [$node->type->value, $node->start, $node->end], Cst::parse($message)->nodes ?? []));
    }

    public function testReadsTheBraceLeftOverAsTheKeysEnd(): void
    {
        $placeholder = self::only(Cst::parse('{{v\\}}}'), NodeType::Placeholder);

        self::assertSame([0, 7], [$placeholder->start, $placeholder->end]);
        self::assertSame(['v}'], self::names($placeholder));
    }

    public function testReadsALaterColonAsContent(): void
    {
        $placeholder = self::only(Cst::parse('{{a:b:c; d:e:f;}}'), NodeType::Placeholder);

        self::assertSame(['a', 'b:c', 'd', ''], self::names($placeholder));

        $value = self::only($placeholder, NodeType::OptionValue);
        self::assertSame('e:f', substr('{{a:b:c; d:e:f;}}', $value->start, $value->end - $value->start));
    }

    public function testDescribesPaddingBesideTheNames(): void
    {
        $placeholder = self::only(Cst::parse('{{ v : number ;}}'), NodeType::Placeholder);

        self::assertSame([
            ['open', 0, 2], ['space', 2, 3], ['key', 3, 4], ['space', 4, 5], ['separator', 5, 6], ['space', 6, 7],
            ['modifier', 7, 13], ['space', 13, 14], ['separator', 14, 15], ['option-key', 15, 15], ['close', 15, 17],
        ], array_map(static fn(Node $node): array => [$node->type->value, $node->start, $node->end], $placeholder->nodes ?? []));
        self::assertSame(['v', 'number', ''], self::names($placeholder));
    }

    public function testAnchorsANameThatIsAllPaddingAtItsStart(): void
    {
        $placeholder = self::only(Cst::parse('{{  }}'), NodeType::Placeholder);

        self::assertSame(
            [['open', 0, 2], ['key', 2, 2], ['space', 2, 4], ['close', 4, 6]],
            array_map(static fn(Node $node): array => [$node->type->value, $node->start, $node->end], $placeholder->nodes ?? []),
        );
    }

    /** @return iterable<string, array{string}> */
    public static function lineTerminators(): iterable
    {
        yield 'line feed' => ["{{v\n}}"];
        yield 'carriage return' => ["{{v\r}}"];
        yield 'line separator' => ["{{v\u{2028}}}"];
        yield 'paragraph separator' => ["{{v\u{2029}}}"];
        yield 'escaped line feed' => ["{{v\\\n}}"];
        yield 'escaped line separator' => ["{{v\\\u{2028}}}"];
        yield 'in a nested placeholder' => ["{{v; a:{{n\u{2028}}}}}"];
    }

    #[DataProvider('lineTerminators')]
    public function testDerivesNoPlaceholderOverALineTerminator(string $message): void
    {
        self::assertSame([], self::all(Cst::parse($message), NodeType::Placeholder));
    }

    public function testReadsANearMissOfALineTerminatorAsPartOfTheKey(): void
    {
        // U+2027 and U+202A share the lead byte and the first continuation of
        // the two terminators.
        self::assertSame(["v\u{2027}\u{202A}"], self::names(self::only(Cst::parse("{{v\u{2027}\u{202A}}}"), NodeType::Placeholder)));
    }

    /** @return iterable<string, array{string, int, bool}> */
    public static function escapes(): iterable
    {
        yield 'a brace' => ['\\{', 2, true];
        yield 'a no-break space' => ["\\\u{00A0}", 3, true];
        yield 'an ideographic space' => ["\\\u{3000}", 4, true];
        yield 'an astral character' => ["\\\u{1F600}", 5, false];
        yield 'a letter' => ['\\n', 2, false];
        yield 'a byte that begins no sequence' => ["\\\x80\x80", 2, false];
        yield 'a truncated sequence' => ["\\\xE2\x80", 2, false];
        yield 'an overlong encoding' => ["\\\xC0\x80", 2, false];
        yield 'an encoded surrogate' => ["\\\xED\xA0\x80", 2, false];
    }

    #[DataProvider('escapes')]
    public function testClaimsTheWholeCharacterAnEscapeConsumes(string $message, int $end, bool $cancels): void
    {
        $escape = (Cst::parse($message)->nodes ?? [])[0];

        self::assertSame(NodeType::Escape, $escape->type);
        self::assertSame([0, $end], [$escape->start, $escape->end]);
        self::assertSame($cancels, $escape->cancels);
    }

    public function testReadsATrailingBackslashAsText(): void
    {
        self::assertSame([['text', 0, 2]], array_map(static fn(Node $node): array => [$node->type->value, $node->start, $node->end], Cst::parse('a\\')->nodes ?? []));
    }

    public function testKeepsAnOptionValuesTextBesideThePlaceholderInIt(): void
    {
        $message = '{{n:gt; 0:you have {{n}} left; default:none;}}';
        $value = self::all(self::only(Cst::parse($message), NodeType::Placeholder), NodeType::OptionValue)[0];

        self::assertSame([10, 29], [$value->start, $value->end]);
        self::assertSame(
            [['text', 10, 19], ['placeholder', 19, 24], ['text', 24, 29]],
            array_map(static fn(Node $node): array => [$node->type->value, $node->start, $node->end], $value->nodes ?? []),
        );
    }

    public function testReadsTheOuterBracesOfAnUnclosedConstructAsText(): void
    {
        self::assertSame(
            [['text', 0, 7], ['placeholder', 7, 12]],
            array_map(static fn(Node $node): array => [$node->type->value, $node->start, $node->end], Cst::parse('{{a; x:{{b}}')->nodes ?? []),
        );
    }

    /** @return iterable<string, array{string}> */
    public static function messages(): iterable
    {
        yield 'empty' => [''];
        yield 'text' => ['Hello'];
        yield 'nested' => ['{{a; b:{{c; d:\;{{e}}}} x}} tail'];
        yield 'escapes everywhere' => ['\\{{ \\: k\;ey : mo\\:d ; o\\:k : v\\}a\\l ;}}\\'];
        yield 'non-ASCII' => ["\u{00E9}{{ \u{00A0}n\u{3000}: gt; \u{1F600}:\u{2028}x}}\u{1F600}"];
        yield 'ill-formed bytes' => ["\x80{{\xFF; \xC3:\xE2\x80}}\\\xF0"];
        yield 'refused constructs' => ['{{{{a}} {{a{{b}} {{a:{{b}}}} {{a;b:{{c}}'];
    }

    #[DataProvider('messages')]
    public function testTilesTheMessageWithLeavesInOrder(string $message): void
    {
        $leaves = [];
        $stack = [Cst::parse($message)];

        while (($node = array_pop($stack)) !== null) {
            if ($node->nodes === null || $node->nodes === []) {
                $leaves[] = $node;

                continue;
            }

            foreach (array_reverse($node->nodes) as $child) {
                $stack[] = $child;
            }
        }

        $at = 0;
        $spelled = '';

        foreach ($leaves as $leaf) {
            self::assertSame($at, $leaf->start);
            self::assertGreaterThanOrEqual($leaf->start, $leaf->end);
            $spelled .= substr($message, $leaf->start, $leaf->end - $leaf->start);
            $at = $leaf->end;
        }

        self::assertSame(strlen($message), $at);
        self::assertSame($message, $spelled);
    }

    // Deep enough that PHP, which frees a value by recursing into what it
    // holds, would crash dropping the tree if it were released level by level.
    public function testDescribesAndReleasesATreeThirtyThousandPlaceholdersDeep(): void
    {
        $depth = 30_000;
        $message = str_repeat('{{a; b:', $depth) . 'x' . str_repeat('}}', $depth);
        $node = Cst::parse($message);
        $found = 0;

        while (($node = self::all($node, NodeType::Placeholder)[0] ?? null) !== null) {
            $found += 1;
            self::assertSame([7 * ($found - 1), strlen($message) - 2 * ($found - 1)], [$node->start, $node->end]);
            $node = self::only($node, NodeType::OptionValue);
        }

        self::assertSame($depth, $found);
    }

    public function testEncodesATreeTenThousandPlaceholdersDeep(): void
    {
        $depth = 10_000;
        $encoded = Cst::parse(str_repeat('{{a; b:', $depth) . 'x' . str_repeat('}}', $depth))->toArray();

        for ($level = 0; $level < $depth; $level += 1) {
            $placeholder = self::child($encoded, 0);
            self::assertSame('placeholder', $placeholder['type']);
            $encoded = self::child($placeholder, 6);
        }

        self::assertSame([['type' => 'text', 'start' => 7 * $depth, 'end' => 7 * $depth + 1]], $encoded['nodes']);
    }

    /**
     * @param array<string, mixed> $node
     * @return array<string, mixed>
     */
    private static function child(array $node, int $index): array
    {
        $children = $node['nodes'];
        self::assertIsArray($children);
        $child = $children[$index];
        self::assertIsArray($child);

        /** @var array<string, mixed> $child */
        return $child;
    }

    /** @return list<Node> */
    private static function all(Node $node, NodeType $type): array
    {
        return array_values(array_filter($node->nodes ?? [], static fn(Node $child): bool => $child->type === $type));
    }

    private static function only(Node $node, NodeType $type): Node
    {
        $found = self::all($node, $type);
        self::assertCount(1, $found);

        return $found[0];
    }

    /** @return list<string|null> */
    private static function names(Node $placeholder): array
    {
        return array_values(array_map(
            static fn(Node $node): ?string => $node->name,
            array_filter($placeholder->nodes ?? [], static fn(Node $node): bool => $node->name !== null),
        ));
    }

    /** @param class-string $class */
    private static function unit(string $class): mixed
    {
        return constant($class . '::UNIT');
    }
}
