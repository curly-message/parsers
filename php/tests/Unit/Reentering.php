<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\Parser;

/** Text that resolves a message holding itself, on the same stack. */
final class Reentering
{
    public int $calls = 0;

    public ?\Throwable $refused = null;

    public function __construct(private readonly Parser $parser) {}

    public function __toString(): string
    {
        $this->calls += 1;

        try {
            return $this->parser->resolve('{{x; default:F}}', ['x' => $this]);
        } catch (\Throwable $refused) {
            $this->refused = $refused;

            throw $refused;
        }
    }
}
