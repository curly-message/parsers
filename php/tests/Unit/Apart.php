<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use PHPUnit\Framework\Assert;

/** Runs a script of `tests/Scripts` in a process of its own, where what it holds could end the process. */
trait Apart
{
    /** What the script answers for the shape, decoded, run under the memory limit and any other settings named. */
    private static function apart(string $script, string $shape, string $limit, string ...$settings): mixed
    {
        $process = proc_open(
            [PHP_BINARY, '-n', '-d', "memory_limit={$limit}", ...array_merge(...array_map(static fn(string $setting): array => ['-d', $setting], $settings)), dirname(__DIR__) . "/Scripts/{$script}.php", $shape],
            [1 => ['pipe', 'w'], 2 => ['pipe', 'w']],
            $pipes,
        );

        Assert::assertIsResource($process);
        $answer = stream_get_contents($pipes[1]);
        $errors = stream_get_contents($pipes[2]);

        Assert::assertSame(0, proc_close($process), $answer . $errors);

        return json_decode((string) $answer, true);
    }
}
