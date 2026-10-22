<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/** Serializes to itself, which is written as its properties. */
final class Itself implements \JsonSerializable
{
    public int $a = 1;

    protected int $b = 2;

    public function jsonSerialize(): self
    {
        return $this;
    }
}
