<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/** An object with text of its own, which counts how often it is asked; null throws. */
final class Spelled
{
    public int $calls = 0;

    public function __construct(private readonly ?string $text) {}

    public function __toString(): string
    {
        $this->calls += 1;

        return $this->text ?? throw new \RuntimeException('no text');
    }
}
