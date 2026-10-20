<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** A case with its inputs built and its expectation read. */
final readonly class Prepared
{
    public function __construct(
        public Resolution $input,
        public Expectation $expected,
    ) {}
}
