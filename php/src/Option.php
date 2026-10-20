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
     * The option's value, rendered on the first call that is not refused,
     * and the same text on every call after that rendering finished.
     *
     * @throws ReentryLimit where `Parser::MAX_REENTRY` walks are in progress on this thread of execution, or, once nothing of the option's resolution is in progress, `Parser::MAX_IN_PROGRESS` resolutions in the process; and where a rendering of the value began and has not finished, whether it is in progress or a fiber was let go in the middle of it
     */
    public function value(): string
    {
        return ($this->read)();
    }
}
