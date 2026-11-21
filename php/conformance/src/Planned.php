<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** A case the plan runs, ready to execute. */
final readonly class Planned extends Identity
{
    /** @param \Closure(): Outcome $execute */
    public function __construct(Identity $identity, private \Closure $execute)
    {
        parent::__construct($identity->id, $identity->file, $identity->level, $identity->document, $identity->section, $identity->description);
    }

    public function execute(): Outcome
    {
        return ($this->execute)();
    }
}
