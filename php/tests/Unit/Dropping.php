<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/**
 * Text that drops an entry of a `stdClass` as it is converted, and runs host
 * code when the last reference to it is dropped.
 */
final class Dropping implements \Stringable
{
    /** @param \Closure(): mixed $released */
    public function __construct(private readonly \stdClass $container, private readonly string $name, private readonly \Closure $released) {}

    public function __toString(): string
    {
        unset($this->container->{$this->name});

        return 'x';
    }

    public function __destruct()
    {
        ($this->released)();
    }
}
