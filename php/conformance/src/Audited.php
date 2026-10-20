<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * What the runner made of one defect: what the catalogue expects, what the
 * runner observed (`error`, `fail`, `skip`, `unobserved` or `none`), and
 * whether that is `caught`, `missed` or `unreachable`.
 */
final readonly class Audited
{
    public function __construct(
        public DefectEntry $defect,
        public string $observed,
        public string $outcome,
    ) {}
}
