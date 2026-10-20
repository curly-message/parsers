<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/** A raise that raises another as it is let go, as many times over as it is told. */
final class Rethrowing extends \RuntimeException
{
    public function __construct(private readonly int $times)
    {
        parent::__construct('rethrowing');
    }

    public function __destruct()
    {
        if ($this->times > 0) {
            throw new self($this->times - 1);
        }
    }
}
