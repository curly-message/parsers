<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** A run's outcomes, in file then case order. */
final readonly class Result
{
    /**
     * @param list<Planned> $passed
     * @param list<Failed> $failed
     * @param list<Skipped> $skipped
     * @param list<Planned> $unobserved the passed cases whose reports went unchecked
     */
    public function __construct(
        public array $passed,
        public array $failed,
        public array $skipped,
        public array $unobserved,
    ) {}
}
