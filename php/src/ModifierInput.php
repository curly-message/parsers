<?php

declare(strict_types=1);

namespace CurlyMessage;

/**
 * What a modifier is called with (SPEC.md section 11). One is built for each
 * call and dropped after it, so a modifier may sort `options` or write into
 * `props` in place: neither reaches the placeholder after it, nor the caller.
 */
final class ModifierInput
{
    /**
     * @param string $value the value's text
     * @param list<Option> $options the options the placeholder declares, in source order
     * @param array<array-key, mixed> $props the properties the layers hold under the modifier's name, composed
     * @param string|null $locale the locale the resolution was given, null where none
     * @param \Closure(): string $default the fallback chain
     */
    public function __construct(
        public string $value,
        public array $options,
        public array $props,
        public ?string $locale,
        private readonly \Closure $default,
    ) {}

    /**
     * The fallback chain (SPEC.md section 10), walked on the first call that
     * is not refused and not before: a modifier that never asks leaves it
     * unread, so a link nobody consulted is never reported as missing. Every
     * call after that walk finished answers its text.
     *
     * @throws ReentryLimit where `Parser::MAX_REENTRY` walks are in progress on this thread of execution, or, once nothing of the input's resolution is in progress, `Parser::MAX_IN_PROGRESS` resolutions in the process; and where a walk of the chain began and has not finished, whether it is in progress or a fiber was let go in the middle of it
     */
    public function default(): string
    {
        return ($this->default)();
    }
}
