<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** A case the plan leaves out, with the reason it is left out. */
final readonly class Skipped extends Identity
{
    public function __construct(Identity $identity, public string $reason)
    {
        parent::__construct($identity->id, $identity->file, $identity->level, $identity->document, $identity->section, $identity->description);
    }
}
