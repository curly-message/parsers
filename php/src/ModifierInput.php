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
     * The fallback chain (SPEC.md section 10), walked on the first call and
     * not before: a modifier that never asks leaves it unread, so a link
     * nobody consulted is never reported as missing. Every call after that
     * walk finished answers its text. Once the input's resolution returned,
     * a call answers the text a call during the resolution walked, and the
     * empty string where none finished one: nothing is walked, reported or
     * run then (SPEC.md section 11.3).
     *
     * A call is refused as `Option::value()` refuses one, and answers the
     * same: called by anything but the modifier the input was handed to,
     * from its own call or the conversion of what that call answered, the
     * walk takes a place among the resolutions in
     * progress on its thread of execution, and where that place is not to
     * be had, or a walk of the chain began and has not finished, the call
     * answers the empty string and fails the host code the parser called
     * that made it, or, where none runs on its thread, the host code the
     * parser called that waits on that thread.
     */
    public function default(): string
    {
        return ($this->default)();
    }
}
