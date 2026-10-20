<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * What a case expects: the output it states, or the request it states and,
 * for a selection, the output each category selects; and the reports, each
 * an array of the fields the case expects something of.
 */
final readonly class Expectation
{
    /** @param list<array<array-key, mixed>> $reports */
    public function __construct(
        public ?string $output,
        public ?\stdClass $format,
        public ?\stdClass $categories,
        public array $reports,
        public ?string $locale = null,
    ) {}
}
