<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** A conforming adapter made wrong in one way, and whether that way ever changed an answer. */
final readonly class Mutation
{
    /** @param \Closure(): bool $reached */
    public function __construct(
        public Adapter $adapter,
        private \Closure $reached,
    ) {}

    public function reached(): bool
    {
        return ($this->reached)();
    }
}
