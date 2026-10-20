<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** The cases a run executes, beside the cases it leaves out and why. */
final readonly class Plan
{
    /**
     * @param list<Planned> $cases
     * @param list<Skipped> $skipped
     */
    public function __construct(
        public array $cases,
        public array $skipped,
    ) {}
}
