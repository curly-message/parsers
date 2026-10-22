<?php

declare(strict_types=1);

namespace CurlyMessage;

use CurlyMessage\Cst\Node;
use CurlyMessage\Cst\NodeType;
use CurlyMessage\Internal\Scanner;
use CurlyMessage\Internal\Text;

/**
 * Describes a message as it is written (CST.md): where its placeholders are,
 * what each is made of, and where every escape sequence falls. The
 * placeholders are the ones resolution finds, because the same scan finds
 * them.
 */
final class Cst
{
    /** The unit every span is in: a byte of the PHP string, read as UTF-8. */
    public const UNIT = 'utf-8';

    /** @var list<NodeType> */
    private array $types = [];

    /** @var list<int> */
    private array $starts = [];

    /** @var list<int> */
    private array $ends = [];

    /** @var array<int, string> */
    private array $names = [];

    /** @var array<int, bool> */
    private array $cancels = [];

    /** @var array<int, list<int>> each node that holds children, by index, with theirs */
    private array $children = [];

    /**
     * The option values described but not yet filled, each with the span it
     * is filled from. Nesting is not otherwise limited (SPEC.md section 6,
     * note 10), so a value is filled off this stack rather than by a call per
     * level.
     *
     * @var list<array{int, int, int}>
     */
    private array $pending = [];

    private readonly Scanner $scanner;

    private function __construct(private readonly string $message)
    {
        $this->scanner = new Scanner($message);
    }

    public static function parse(string $message): Node
    {
        $builder = new self($message);
        $root = $builder->add(NodeType::Message, 0, strlen($message), holds: true);
        $builder->pending[] = [$root, 0, strlen($message)];

        while (($work = array_pop($builder->pending)) !== null) {
            $builder->contents(...$work);
        }

        return $builder->freeze();
    }

    // A node is added before anything it holds, so every child's index is
    // greater than its parent's.
    private function add(NodeType $type, int $start, int $end, ?int $parent = null, bool $holds = false): int
    {
        $index = count($this->types);
        $this->types[] = $type;
        $this->starts[] = $start;
        $this->ends[] = $end;

        if ($holds) {
            $this->children[$index] = [];
        }

        if ($parent !== null) {
            $this->children[$parent][] = $index;
        }

        return $index;
    }

    // Children are built before the node that holds them, highest index
    // first, so the tree is frozen in one loop and no call nests per level.
    // A node is let go of once its parent holds it, and the builder's record
    // before the message is built, since the message gathers every node below
    // it.
    private function freeze(): Node
    {
        /** @var array<int, Node> $built */
        $built = [];

        for ($index = count($this->types) - 1; $index > 0; $index -= 1) {
            $built[$index] = new Node(
                $this->types[$index],
                $this->starts[$index],
                $this->ends[$index],
                $this->held($index, $built),
                $this->names[$index] ?? null,
                $this->cancels[$index] ?? null,
            );
        }

        $nodes = $this->held(0, $built);
        $end = $this->ends[0];
        $this->types = $this->starts = $this->ends = $this->names = $this->cancels = [];

        return new Node(NodeType::Message, 0, $end, $nodes);
    }

    /**
     * @param array<int, Node> $built
     *
     * @return list<Node>|null
     */
    private function held(int $index, array &$built): ?array
    {
        if (!isset($this->children[$index])) {
            return null;
        }

        $nodes = [];

        foreach ($this->children[$index] as $child) {
            $nodes[] = $built[$child];
            unset($built[$child]);
        }

        unset($this->children[$index]);

        return $nodes;
    }

    /**
     * Text with every escape sequence in it marked. A backslash with nothing
     * left to consume denotes itself and is text.
     */
    private function spelled(int $parent, int $from, int $to): void
    {
        $message = $this->message;
        $start = $from;

        for ($at = $from + strcspn($message, '\\', $from, $to - $from); $at + 1 < $to; $at += strcspn($message, '\\', $at, $to - $at)) {
            if ($at > $start) {
                $this->add(NodeType::Text, $start, $at, $parent);
            }

            $end = Text::escapeEnd($message, $at, $to);
            $escape = $this->add(NodeType::Escape, $at, $end, $parent);
            $this->cancels[$escape] = Text::cancels(substr($message, $at + 1, $end - $at - 1));
            $at = $end;
            $start = $end;
        }

        if ($to > $start) {
            $this->add(NodeType::Text, $start, $to, $parent);
        }
    }

    /**
     * What a span of the message itself holds: its text, its escape sequences
     * and the placeholders it writes. The message is one such span and so is
     * each option value.
     */
    private function contents(int $parent, int $from, int $to): void
    {
        $at = $from;

        while (($match = $this->scanner->next($at, $to)) !== null) {
            [$open, $close] = $match;
            $this->spelled($parent, $at, $open);
            $this->placeholder($parent, $open, $close);
            $at = $close;
        }

        $this->spelled($parent, $at, $to);
    }

    // A name and the padding it is read without, as nodes of their own, so the
    // parts still tile the message.
    private function named(int $parent, NodeType $type, int $from, int $to): void
    {
        [$start, $end] = Text::trimmed($this->message, $from, $to);

        if ($start > $from) {
            $this->add(NodeType::Space, $from, $start, $parent);
        }

        $name = $this->add($type, $start, $end, $parent, holds: true);
        $this->names[$name] = Text::unesc(substr($this->message, $start, $end - $start));
        $this->spelled($name, $start, $end);

        if ($to > $end) {
            $this->add(NodeType::Space, $end, $to, $parent);
        }
    }

    // An option value and the padding it is read without; what it holds is
    // left on the pending stack.
    private function valued(int $parent, int $from, int $to): void
    {
        [$start, $end] = Text::trimmed($this->message, $from, $to);

        if ($start > $from) {
            $this->add(NodeType::Space, $from, $start, $parent);
        }

        $value = $this->add(NodeType::OptionValue, $start, $end, $parent, holds: true);
        $this->pending[] = [$value, $start, $end];

        if ($to > $end) {
            $this->add(NodeType::Space, $end, $to, $parent);
        }
    }

    private function placeholder(int $parent, int $open, int $close): void
    {
        $message = $this->message;
        $placeholder = $this->add(NodeType::Placeholder, $open, $close, $parent, holds: true);
        $this->add(NodeType::Open, $open, $open + 2, $placeholder);
        // A `;` a nested placeholder encloses divides that placeholder and not
        // this one (SPEC.md section 6, note 5).
        $options = Text::separated($message, $open + 2, $close - 2, ';', $this->scanner);
        $declaration = array_shift($options);
        // The selector's colon is the first one no escape sequence claims; a
        // second colon is part of the modifier name (note 3).
        $selector = Text::separated($message, $declaration[0], $declaration[1], ':');
        $key = $selector[0];
        $this->named($placeholder, NodeType::Key, $key[0], $key[1]);

        if (count($selector) > 1) {
            $this->add(NodeType::Separator, $key[1], $key[1] + 1, $placeholder);
            $this->named($placeholder, NodeType::Modifier, $key[1] + 1, $declaration[1]);
        }

        foreach ($options as [$from, $to]) {
            // The `;` that opened this segment sits immediately before it.
            $this->add(NodeType::Separator, $from - 1, $from, $placeholder);
            $segment = Text::separated($message, $from, $to, ':', $this->scanner);
            $optionKey = $segment[0];
            $this->named($placeholder, NodeType::OptionKey, $optionKey[0], $optionKey[1]);

            // A segment that states no value stands for its own key (section
            // 9.4), so there is no value to describe.
            if (count($segment) > 1) {
                $this->add(NodeType::Separator, $optionKey[1], $optionKey[1] + 1, $placeholder);
                $this->valued($placeholder, $optionKey[1] + 1, $to);
            }
        }

        $this->add(NodeType::Close, $close - 2, $close, $placeholder);
    }
}
