<?php

declare(strict_types=1);

namespace CurlyMessage;

/**
 * What a modifier is called with (SPEC.md section 11). One is built for each
 * call and dropped after it, so a modifier may sort `options` in place,
 * which reaches neither the placeholder after it nor the caller. `props` is
 * read-only: every placeholder naming the modifier shares what the layers
 * hold under its name, and a modifier that writes into it fails.
 */
final class ModifierInput
{
    /**
     * @param string $value the value's text
     * @param list<Option> $options the options the placeholder declares, in source order
     * @param Props $props the properties the layers hold under the modifier's name, composed, read as an array is and never written
     * @param string|null $locale the locale the resolution was given, null where none
     * @param \Closure(): string $default the fallback chain
     */
    public function __construct(
        public string $value,
        public array $options,
        public Props $props,
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
     * A call the parser refuses walks nothing and answers the empty string,
     * and the host modifier being asked on that thread of execution then
     * fails whatever it answers (SPEC.md section 13). One is refused at once
     * where, while the input's resolution is in progress, a walk of the
     * chain began and has not finished, whether it is in progress or a
     * fiber was let go in the middle of it. One made by anything but the
     * modifier the input was handed to, while that modifier is being asked,
     * takes a place among the resolutions in progress while it walks
     * (`Parser::MAX_REENTRY`, `Parser::MAX_IN_PROGRESS`), and is refused
     * where there is none, which leaves the chain to be walked by a later
     * call.
     */
    public function default(): string
    {
        return ($this->default)();
    }
}
