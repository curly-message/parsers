<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * What one case came to: passed, passed with its reports unobserved, or
 * failed at its first mismatch, beside what was expected and what came back.
 */
final readonly class Outcome
{
    private function __construct(
        public bool $ok,
        public ?string $unobserved,
        public string $reason,
        public mixed $expected,
        public mixed $actual,
    ) {}

    public static function passed(): self
    {
        return new self(true, null, '', null, null);
    }

    /** A pass whose reports went unchecked, because the adapter observes none. */
    public static function unobserved(): self
    {
        return new self(true, 'reports', '', null, null);
    }

    public static function failed(string $reason, mixed $expected, mixed $actual): self
    {
        return new self(false, null, $reason, $expected, $actual);
    }
}
