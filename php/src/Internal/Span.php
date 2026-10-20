<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

/**
 * A span of the message being rendered on an explicit stack: where it has
 * reached, and what it has rendered so far.
 *
 * @internal
 */
final class Span
{
    public string $out = '';

    public function __construct(
        public int $at,
        public readonly int $to,
        public readonly int $depth,
    ) {}
}
