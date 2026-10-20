<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/**
 * Serializes to a new link of its chain, until the chain is as long as it was
 * made to be, and counts how often it is asked. Every link stays reachable,
 * so no two are ever the same object.
 */
final class Chain implements \JsonSerializable
{
    public int $calls = 0;

    /** @var list<self> */
    private array $links = [];

    public function __construct(private readonly int $length, private readonly ?self $first = null) {}

    public function jsonSerialize(): mixed
    {
        $first = $this->first ?? $this;
        $first->calls += 1;

        if (count($first->links) >= $first->length) {
            return 'end';
        }

        $link = new self($first->length, $first);
        $first->links[] = $link;

        return $link;
    }
}
