<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/** Serializes to what it was given, and counts how often it is asked. */
final class Serialized implements \JsonSerializable
{
    public int $calls = 0;

    public function __construct(private readonly mixed $answer) {}

    public function jsonSerialize(): mixed
    {
        $this->calls += 1;

        return $this->answer;
    }
}
