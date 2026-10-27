<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * One case's inputs, decoded: each field the case leaves out holds the
 * adapter's `absent`, and `modifiers` holds the behaviours the case registers
 * by name, none where it registers none.
 */
final readonly class Resolution
{
    /** @param array<string, \Closure(BehaviourInput): mixed> $modifiers */
    public function __construct(
        public mixed $message,
        public mixed $payload,
        public mixed $props,
        public mixed $locale,
        public mixed $id,
        public array $modifiers,
        public mixed $defaults,
    ) {}
}
