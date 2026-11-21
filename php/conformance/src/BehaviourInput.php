<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** What a behaviour of the catalogue reads: section 11's inputs to a modifier. */
final readonly class BehaviourInput
{
    /**
     * @param list<BehaviourOption> $options
     * @param array<array-key, mixed> $props
     * @param \Closure(): string $default
     */
    public function __construct(
        public string $value,
        public array $options,
        public array $props,
        public ?string $locale,
        private \Closure $default,
    ) {}

    /** The fallback chain, walked where the behaviour asks for it. */
    public function default(): string
    {
        return ($this->default)();
    }
}
