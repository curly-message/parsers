<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/** Serializes to a new object holding what it was given, as a data object answers. */
final class Fresh implements \JsonSerializable
{
    /** @param array<string, mixed> $members */
    public function __construct(private readonly array $members) {}

    public function jsonSerialize(): \stdClass
    {
        return (object) $this->members;
    }
}
