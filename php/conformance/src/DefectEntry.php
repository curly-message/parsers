<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** One defect of the catalogue `defects.json` ships: one way an adapter can be wrong, and what a correct runner answers where it is. */
final readonly class DefectEntry
{
    /**
     * @param string|null $document the document `section` is a heading of; SPEC.md where null
     * @param 'error'|'fail'|'skip'|'unobserved' $expects
     */
    public function __construct(
        public string $id,
        public string $description,
        public ?string $document,
        public ?string $section,
        public string $expects,
    ) {}
}
