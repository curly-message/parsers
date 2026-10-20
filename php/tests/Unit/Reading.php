<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/** Text that is whatever host code answers as it is converted. */
final class Reading implements \Stringable
{
    /** @param \Closure(): string $read */
    public function __construct(private readonly \Closure $read) {}

    public function __toString(): string
    {
        return ($this->read)();
    }
}
