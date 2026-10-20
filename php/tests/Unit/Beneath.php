<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/** Text that is what host code answers as the value converts, beneath the resolution converting it. */
final class Beneath implements \Stringable
{
    /** @param \Closure(): string $text */
    public function __construct(private readonly \Closure $text) {}

    public function __toString(): string
    {
        return ($this->text)();
    }
}
