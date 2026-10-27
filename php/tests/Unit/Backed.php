<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

enum Backed: string
{
    case Int = '1';
    case String = 'two';
}
