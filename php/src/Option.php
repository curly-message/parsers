<?php

declare(strict_types=1);

namespace CurlyMessage;

/**
 * One option a placeholder declares, in the order it declares them (SPEC.md
 * section 9.4). Its value is message text, so it is rendered rather than
 * read, and only where it is asked for: an option a modifier passes over is
 * never rendered, however deeply it nests.
 */
final readonly class Option
{
    /** @param \Closure(): string $read renders the value, once */
    public function __construct(
        public string $key,
        private \Closure $read,
    ) {}

    /**
     * The option's value, rendered on the first call, and the same text on
     * every call after that rendering finished. Once the option's resolution
     * returned, a call answers the text a call during the resolution
     * rendered, and the empty string where none finished one: nothing is
     * rendered, reported or run then (SPEC.md section 11.3).
     *
     * Called by anything but the modifier the option was handed to, from
     * its own call or the conversion of what that call answered, the
     * rendering takes a place among the resolutions in
     * progress on its thread of execution (`Parser::MAX_REENTRY`, SPEC.md
     * section 13). A call is refused where that place is not to be had, or
     * where a rendering of the value began and has not finished, whether it
     * is in progress or a fiber was let go in the middle of it: it records
     * nothing, answers the empty string, and throws nothing: the host code
     * the parser called that made it fails, whatever it answers, so a
     * modifier's placeholder reports `failed-modifier` and takes its
     * fallback chain. Made where no host code the parser called runs on its
     * thread, as in a fiber host code started, it fails the host code the
     * parser called that waits on that fiber, having started or resumed it,
     * and fails nothing where none does.
     */
    public function value(): string
    {
        return ($this->read)();
    }
}
