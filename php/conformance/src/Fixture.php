<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** A fixture file as it was read, objects as `stdClass` and arrays as lists, together with the name it is shipped under. */
final readonly class Fixture
{
    public function __construct(
        public string $name,
        public \stdClass $file,
    ) {}
}
