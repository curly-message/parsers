<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** A case that ran and failed, with its outcome. */
final readonly class Failed
{
    public function __construct(
        public Planned $case,
        public Outcome $outcome,
    ) {}
}
