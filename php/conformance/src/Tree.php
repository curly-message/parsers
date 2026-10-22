<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * Runs a tree case in the seven steps of RUNNER.md, "Executing a tree case":
 * the tree's shape, its span, each node's kind and span, the tiling, the
 * case's own expectation, determinism, and agreement with resolution.
 */
final class Tree
{
    /** The units CST.md section 4 lets an implementation count its spans in. */
    public const UNITS = ['utf-8', 'utf-16', 'code-point'];

    public const TYPES = ['message', 'placeholder', 'open', 'close', 'separator', 'space', 'key', 'modifier', 'option-key', 'option-value', 'text', 'escape'];

    /**
     * The kinds that carry a name (CST.md section 8). A kind that must carry
     * a field and does not has answered less than the document requires; a
     * field a kind does not take is not policed, because that table describes
     * the interchange encoding and an adapter hands back a host value.
     */
    public const NAMED = ['key', 'modifier', 'option-key'];

    /**
     * Where each boundary of the message falls, as a byte offset into it,
     * indexed by the unit the adapter declared. A boundary inside a code point
     * has no offset: a character wider than one unit contributes one entry for
     * its start and null for each position within it.
     *
     * @return list<int|null>
     */
    public static function boundaries(string $message, mixed $unit): array
    {
        $offsets = [];
        $length = strlen($message);

        for ($at = 0; $at < $length; $at += $width) {
            $width = Utf8::width($message, $at);
            $offsets[] = $at;
            $units = match ($unit) {
                'utf-8' => $width,
                'utf-16' => $width === 4 ? 2 : 1,
                default => 1,
            };

            for ($within = 1; $within < $units; $within += 1) {
                $offsets[] = null;
            }
        }

        $offsets[] = $length;

        return $offsets;
    }

    /** The text a node's span covers, in the declared unit; null where the span is not one. */
    public static function spelling(string $message, mixed $unit, mixed $node): ?string
    {
        $offsets = self::boundaries($message, $unit);
        $start = Compare::field($node, 'start');
        $end = Compare::field($node, 'end');
        $from = is_int($start) ? ($offsets[$start] ?? null) : null;
        $to = is_int($end) ? ($offsets[$end] ?? null) : null;

        return $from === null || $to === null || $to < $from ? null : substr($message, $from, $to - $from);
    }

    /**
     * The children a node holds: none where it holds no list.
     *
     * @return array<array-key, mixed>
     */
    public static function children(mixed $node): array
    {
        $nodes = Compare::field($node, 'nodes');

        return is_array($nodes) ? $nodes : [];
    }

    public static function execute(Adapter $adapter, \stdClass $case): Outcome
    {
        $cst = $adapter->cst;
        $id = is_string($case->id ?? null) ? $case->id : '';
        $message = $case->message ?? null;

        if ($cst === null) {
            throw new \RuntimeException("The case {$id} was planned against an adapter that offers no tree.");
        }

        if (!is_string($message)) {
            throw new \RuntimeException("The case {$id} describes no message.");
        }

        $offsets = self::boundaries($message, $cst->unit);
        $units = count($offsets) - 1;
        $tree = self::answer($cst, $message);

        if ($tree instanceof Outcome) {
            return $tree;
        }

        $start = Compare::field($tree, 'start');
        $end = Compare::field($tree, 'end');

        if ($start !== 0 || $end !== $units) {
            return Outcome::failed('The tree does not span the whole message.', [0, $units], [$start, $end]);
        }

        $walked = self::walk($tree, $message, $offsets);

        if ($walked instanceof Outcome) {
            return $walked;
        }

        $wrong = self::tiling($walked, $message, $units)
            ?? self::compare(is_array($case->expected ?? null) ? $case->expected : [], self::children($tree), $message, $offsets, 'the message');

        if ($wrong !== null) {
            return $wrong;
        }

        $again = self::answer($cst, $message);
        $twice = $again instanceof Outcome ? $again : self::walk($again, $message, $offsets);

        if ($twice instanceof Outcome) {
            return $twice;
        }

        if (self::shape($twice) !== self::shape($walked)) {
            return Outcome::failed('The tree is not a function of the message alone: two parses of it differ.', self::shape($walked), self::shape($twice));
        }

        if (!property_exists($case, 'resolves')) {
            return Outcome::passed();
        }

        $absent = $adapter->absent;

        try {
            $resolved = $adapter->resolve(new Resolution($message, $absent, $absent, $absent, $absent, [], $absent));
        } catch (\Throwable $error) {
            return Outcome::failed('The adapter raised resolving the message the tree describes.', $case->resolves, $error->getMessage());
        }

        $output = Compare::field($resolved, 'output');

        return $output === $case->resolves
            ? Outcome::passed()
            : Outcome::failed('The tree and the resolution do not read the same placeholders.', $case->resolves, $output);
    }

    // What the adapter answered, checked before it is read: a tree it raised
    // over, or one that is not a message node, fails the case rather than
    // ending the run.
    private static function answer(CstOffer $cst, string $message): mixed
    {
        try {
            $tree = $cst->parse($message);
        } catch (\Throwable $error) {
            return Outcome::failed('The adapter raised rather than answering with a tree.', 'a message node', $error->getMessage());
        }

        if (Compare::field($tree, 'type') !== 'message') {
            return Outcome::failed('The adapter answered with no tree.', 'a message node', $tree);
        }

        return $tree;
    }

    // A tree describes the message, so what it can hold follows from the
    // message's length: the densest construction, a run of empty segments,
    // contributes two nodes per character. A structure that holds itself
    // would be walked forever, so the walk is bounded by more than any tree of
    // the message can hold, and a cyclic answer fails the case instead of
    // hanging the run.
    private static function budget(int $units): int
    {
        return $units * 4 + 64;
    }

    /**
     * Every node of the tree, in document order, each with the text its span
     * covers and its depth, or the first thing wrong with one of them in the
     * order CST.md states them. The walk keeps its own stack, and never holds
     * more than the budget.
     *
     * @param list<int|null> $offsets
     * @return list<array{node: mixed, text: string, depth: int}>|Outcome
     */
    private static function walk(mixed $root, string $message, array $offsets): array|Outcome
    {
        $units = count($offsets) - 1;
        $budget = self::budget($units);
        $walked = [];
        /** @var list<array{mixed, int, array{type: string, start: int, end: int}|null}> $stack */
        $stack = [[$root, 0, null]];

        while ($stack !== []) {
            [$node, $depth, $within] = array_pop($stack);

            if (!is_array($node) && !$node instanceof \stdClass) {
                return Outcome::failed('The tree holds something that is not a node.', 'a node', $node);
            }

            $type = Compare::field($node, 'type');
            $start = Compare::field($node, 'start');
            $end = Compare::field($node, 'end');

            if (!is_string($type) || !in_array($type, self::TYPES, true)) {
                return Outcome::failed('The tree holds a node of no kind this document names.', implode(', ', self::TYPES), $type);
            }

            if (!is_int($start) || !is_int($end)) {
                return Outcome::failed("The span of the {$type} node is not a pair of offsets.", 'two integers', [$start, $end]);
            }

            if ($end < $start) {
                return Outcome::failed("The {$type} node ends before it starts.", 'end at or after start', [$start, $end]);
            }

            if ($start < 0 || $end > $units) {
                return Outcome::failed("The {$type} node lies outside the message.", "within [0,{$units})", [$start, $end]);
            }

            if ($within !== null && ($start < $within['start'] || $end > $within['end'])) {
                return Outcome::failed("The {$type} node lies outside the {$within['type']} node that holds it.", [$within['start'], $within['end']], [$start, $end]);
            }

            $from = $offsets[$start];
            $to = $offsets[$end];

            if ($from === null || $to === null) {
                return Outcome::failed("The span of the {$type} node falls inside a code point.", 'a boundary between code points', [$start, $end]);
            }

            if (in_array($type, self::NAMED, true) && !is_string(Compare::field($node, 'name'))) {
                return Outcome::failed("The {$type} node carries no name.", 'the span unescaped', Compare::field($node, 'name'));
            }

            if ($type === 'escape' && !is_bool(Compare::field($node, 'cancels'))) {
                return Outcome::failed('The escape node does not say which reading it takes.', 'true or false', Compare::field($node, 'cancels'));
            }

            $walked[] = ['node' => $node, 'text' => substr($message, $from, $to - $from), 'depth' => $depth];
            $nodes = Compare::field($node, 'nodes');

            if ($nodes === null) {
                continue;
            }

            if (!is_array($nodes) || !array_is_list($nodes)) {
                return Outcome::failed("The children of the {$type} node are not a list.", 'a list of nodes', $nodes);
            }

            if (count($walked) + count($stack) + count($nodes) > $budget + 1) {
                return Outcome::failed('The tree holds more nodes than the message can spell.', "at most {$budget} nodes", 'more');
            }

            $holder = ['type' => $type, 'start' => $start, 'end' => $end];

            for ($index = count($nodes) - 1; $index >= 0; $index -= 1) {
                $stack[] = [$nodes[$index], $depth + 1, $holder];
            }
        }

        return $walked;
    }

    /**
     * Properties 1, 2 and 3 of CST.md section 5, which are what a consumer
     * walks a tree by: the leaves tile the message in order and spell it back.
     *
     * @param list<array{node: mixed, text: string, depth: int}> $walked
     */
    private static function tiling(array $walked, string $message, int $units): ?Outcome
    {
        $at = 0;
        $spelled = '';

        foreach ($walked as ['node' => $node, 'text' => $text]) {
            if (self::children($node) !== []) {
                continue;
            }

            $start = Compare::field($node, 'start');

            if ($start !== $at) {
                return Outcome::failed('The leaves do not tile the message: a ' . json_encode(Compare::field($node, 'type')) . ' node starts where the one before it did not end.', $at, $start);
            }

            $end = Compare::field($node, 'end');
            $at = is_int($end) ? $end : $at;
            $spelled .= $text;
        }

        if ($at !== $units) {
            return Outcome::failed('The leaves do not reach the end of the message.', $units, $at);
        }

        if ($spelled !== $message) {
            return Outcome::failed('The leaves do not spell the message back.', $message, $spelled);
        }

        return null;
    }

    /**
     * The tree as the runner reads it, for comparing one parse with another.
     *
     * @param list<array{node: mixed, text: string, depth: int}> $walked
     */
    private static function shape(array $walked): string
    {
        $parts = [];

        foreach ($walked as ['node' => $node, 'text' => $text, 'depth' => $depth]) {
            $parts[] = implode(':', [$depth, self::text(Compare::field($node, 'type')), self::text($text), self::text(Compare::field($node, 'name')), self::text(Compare::field($node, 'cancels'))]);
        }

        return implode('|', $parts);
    }

    // A field as the shape spells it: a value of a kind the encoding never
    // carries is named by its kind, so spelling it runs nothing of the host's.
    private static function text(mixed $value): string
    {
        return $value === null || is_scalar($value) ? (string) json_encode($value, JSON_UNESCAPED_SLASHES | JSON_INVALID_UTF8_SUBSTITUTE) : get_debug_type($value);
    }

    /**
     * What a case expects of each node, against what the implementation
     * answered. The descent follows the expectation, which is the set's, over
     * nodes the walk already checked.
     *
     * @param array<array-key, mixed> $expected
     * @param array<array-key, mixed> $actual
     * @param list<int|null> $offsets
     */
    private static function compare(array $expected, array $actual, string $message, array $offsets, string $where): ?Outcome
    {
        $types = static fn(array $nodes): array => array_map(static fn(mixed $node): mixed => Compare::field($node, 'type'), $nodes);

        if (count($expected) !== count($actual)) {
            return Outcome::failed("The children of {$where} are " . count($actual) . ' nodes rather than ' . count($expected) . '.', $types($expected), $types($actual));
        }

        $actual = array_values($actual);

        foreach (array_values($expected) as $index => $wanted) {
            $node = $actual[$index];
            $position = 'Node ' . ($index + 1) . " of {$where}";
            $type = Compare::field($node, 'type');

            if ($type !== Compare::field($wanted, 'type')) {
                return Outcome::failed("{$position} is of another kind.", Compare::field($wanted, 'type'), $type);
            }

            $start = Compare::field($node, 'start');
            $end = Compare::field($node, 'end');
            $from = is_int($start) ? $offsets[$start] : null;
            $to = is_int($end) ? $offsets[$end] : null;
            $text = $from === null || $to === null ? null : substr($message, $from, $to - $from);

            if ($text !== Compare::field($wanted, 'text')) {
                return Outcome::failed("{$position} spans other text.", Compare::field($wanted, 'text'), $text);
            }

            $name = Compare::field($wanted, 'name');

            if ($name !== null && Compare::field($node, 'name') !== $name) {
                return Outcome::failed("{$position} is unescaped to something else.", $name, Compare::field($node, 'name'));
            }

            $cancels = Compare::field($wanted, 'cancels');

            if ($cancels !== null && Compare::field($node, 'cancels') !== $cancels) {
                return Outcome::failed("{$position} takes the other reading of section 7.", $cancels, Compare::field($node, 'cancels'));
            }

            $nodes = Compare::field($wanted, 'nodes');

            if (!is_array($nodes)) {
                continue;
            }

            $wrong = self::compare($nodes, self::children($node), $message, $offsets, 'the ' . (is_string($type) ? $type : '') . ' node');

            if ($wrong !== null) {
                return $wrong;
            }
        }

        return null;
    }
}
