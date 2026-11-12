<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/** Text that runs host code as it is converted, and when the last reference to it is dropped. */
final class Converting implements \Stringable
{
    /**
     * @param \Closure(): mixed $converted
     * @param \Closure(): mixed $released
     */
    public function __construct(private readonly \Closure $converted, private readonly \Closure $released) {}

    public function __toString(): string
    {
        ($this->converted)();

        return 'x';
    }

    public function __destruct()
    {
        ($this->released)();
    }
}
