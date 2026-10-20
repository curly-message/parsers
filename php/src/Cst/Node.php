<?php

declare(strict_types=1);

namespace CurlyMessage\Cst;

/**
 * One node of the tree a message is described by (CST.md section 6). Its span
 * is in bytes of the PHP string, so `substr($message, $node->start,
 * $node->end - $node->start)` is what it spans.
 */
final readonly class Node
{
    /**
     * @param list<Node>|null $nodes the children of a message, a placeholder, a name and an option value, and null on every other kind
     * @param string|null $name a name's span unescaped, and null on every other kind
     * @param bool|null $cancels whether an escape cancels a structural meaning, and null on every other kind
     */
    public function __construct(
        public NodeType $type,
        public int $start,
        public int $end,
        public ?array $nodes = null,
        public ?string $name = null,
        public ?bool $cancels = null,
    ) {
        $this->retained = $type === NodeType::Message ? self::descendants($this) : [];
    }

    /**
     * PHP frees a value by recursing into what it holds, so a tree nested
     * deep enough would take the process down when it is dropped. A message
     * also holds every node below it, parents before children, declared after
     * its children so that it releases them first: that list then releases
     * each node while the list still holds its children, one level at a time.
     *
     * @var list<Node>
     * @held
     */
    private array $retained;

    /**
     * CST.md section 8's interchange encoding, as PHP arrays: each node
     * carries the fields its kind takes and no others. It is built without
     * recursing, so a tree of any depth answers; `json_encode` of it recurses
     * and needs a `depth` of `4 * nesting + 8` or more.
     *
     * @return array<string, mixed>
     */
    public function toArray(): array
    {
        /** @var list<Node> $order */
        $order = [$this];
        /** @var list<int> $parents */
        $parents = [-1];

        for ($index = 0; $index < count($order); $index += 1) {
            foreach ($order[$index]->nodes ?? [] as $child) {
                $order[] = $child;
                $parents[] = $index;
            }
        }

        // Children come after their parent in that order, so walking it from
        // the end finishes every child before the node that holds it.
        /** @var array<int, list<array<string, mixed>>> $children */
        $children = [];
        $root = [];

        for ($index = count($order) - 1; $index >= 0; $index -= 1) {
            $node = $order[$index];
            $encoded = ['type' => $node->type->value, 'start' => $node->start, 'end' => $node->end];

            if ($node->name !== null) {
                $encoded['name'] = $node->name;
            }

            if ($node->nodes !== null) {
                $encoded['nodes'] = array_reverse($children[$index] ?? []);
                unset($children[$index]);
            }

            if ($node->cancels !== null) {
                $encoded['cancels'] = $node->cancels;
            }

            if ($parents[$index] < 0) {
                $root = $encoded;
            } else {
                $children[$parents[$index]][] = $encoded;
            }
        }

        return $root;
    }

    /** @return list<Node> */
    private static function descendants(self $node): array
    {
        $order = $node->nodes ?? [];

        for ($index = 0; $index < count($order); $index += 1) {
            foreach ($order[$index]->nodes ?? [] as $child) {
                $order[] = $child;
            }
        }

        return $order;
    }
}
