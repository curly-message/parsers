<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\Undefined;
use PHPUnit\Framework\TestCase;

final class UndefinedTest extends TestCase
{
    public function testIsAValueThatIsNotNull(): void
    {
        self::assertNotSame(null, self::read(['key' => Undefined::Value], 'key'));
    }

    /** @param array<string, mixed> $from */
    private static function read(array $from, string $key): mixed
    {
        return $from[$key] ?? null;
    }
}
