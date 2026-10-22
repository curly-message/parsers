<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * Which case a line is about: its id, the file and level it belongs to, and
 * the section it pins, in the document named wherever that is not the
 * specification.
 */
readonly class Identity
{
    public function __construct(
        public string $id,
        public string $file,
        public ?string $level,
        public ?string $document,
        public string $section,
        public string $description,
    ) {}

    /** Where the sentence the case pins is written. */
    public function where(): string
    {
        return $this->document === null ? "section {$this->section}" : "{$this->document} section {$this->section}";
    }
}
