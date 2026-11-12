<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/**
 * Text, and JSON, that suspends the fiber asking for it, where a fiber asks,
 * and counts how often it is asked: the text of the nth ask is `tn`.
 */
final class Suspending implements \Stringable, \JsonSerializable
{
    public int $calls = 0;

    public function __toString(): string
    {
        return 't' . $this->ask();
    }

    public function jsonSerialize(): int
    {
        return $this->ask();
    }

    private function ask(): int
    {
        $this->calls += 1;
        $call = $this->calls;

        if (\Fiber::getCurrent() !== null) {
            \Fiber::suspend();
        }

        return $call;
    }
}
