<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/** Thrown where a value reaches an object it is already inside, naming that object. */
final class Cycle extends \Exception
{
    public function __construct(public readonly \stdClass $object)
    {
        parent::__construct('The value reaches an object it is inside.');
    }
}
