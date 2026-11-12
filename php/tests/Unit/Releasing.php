<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/** Text that runs host code when the last reference to it is dropped. */
final class Releasing implements \Stringable
{
    /** @param \Closure(): mixed $released */
    public function __construct(private readonly \Closure $released) {}

    public function __toString(): string
    {
        return 'b';
    }

    public function __destruct()
    {
        ($this->released)();
    }
}
