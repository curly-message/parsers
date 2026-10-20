<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

final class Raising implements \JsonSerializable
{
    public function jsonSerialize(): never
    {
        throw new \RuntimeException('raising');
    }
}
