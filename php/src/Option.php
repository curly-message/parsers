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
     * A call the parser refuses renders nothing and answers the empty
     * string, and the host modifier being asked on that thread of execution
     * then fails whatever it answers (SPEC.md section 13). One is refused at
     * once where, while the option's resolution is in progress, a rendering
     * of the value began and has not finished, whether it is in progress or
     * a fiber was let go in the middle of it. One made by anything but the
     * modifier the option was handed to, while that modifier is being
     * asked, takes a place among the resolutions in progress while it
     * renders (`Parser::MAX_REENTRY`, `Parser::MAX_IN_PROGRESS`), and is
     * refused where there is none, which leaves the value to be rendered by
     * a later call.
     */
    public function value(): string
    {
        return ($this->read)();
    }
}
