<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

/**
 * What a placeholder declares and what it read for its value: the steps
 * every placeholder takes, a refused one included, before anything resolves.
 *
 * @internal
 */
final readonly class Declared
{
    /**
     * @param int $open where the placeholder's spelling begins
     * @param int $close where it ends
     * @param array<array-key, mixed>|\stdClass|null $wrapper the entry configuring the value, null where none does
     * @param string|null $valueText null where the value is absent or nothing describes it
     * @param \Closure(): void $raised reports that a link refused to be read
     */
    public function __construct(
        public int $open,
        public int $close,
        public Placeholder $placeholder,
        public array|\stdClass|null $wrapper,
        public ?string $valueText,
        public \Closure $raised,
    ) {}
}
