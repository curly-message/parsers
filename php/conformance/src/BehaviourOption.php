<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** One option a placeholder declares, with its value rendered on asking (SPEC.md section 11). */
final readonly class BehaviourOption
{
    /** @param \Closure(): string $read */
    public function __construct(
        public string $key,
        private \Closure $read,
    ) {}

    public function value(): string
    {
        return ($this->read)();
    }
}
