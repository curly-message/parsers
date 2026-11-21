<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * The tree an adapter offers (CST.md section 4): the unit its spans are
 * counted in, and the call that describes a message, answering CST.md section
 * 8's encoding as nested arrays or objects.
 */
final readonly class CstOffer
{
    /** @param \Closure(string): mixed $parse */
    public function __construct(
        public mixed $unit,
        public \Closure $parse,
    ) {}

    /** A copy with the parse wrapped: what a defect of the tree is. */
    public function parsing(\Closure $parse): self
    {
        return new self($this->unit, $parse);
    }

    public function counting(mixed $unit): self
    {
        return new self($unit, $this->parse);
    }

    public function parse(string $message): mixed
    {
        return ($this->parse)($message);
    }
}
