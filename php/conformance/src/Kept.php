<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * One list of what the behaviours of `Behaviours::keeping` kept, and which of
 * it a `take` has claimed: each input is the call that reads the default its
 * modifier was handed.
 */
final class Kept
{
    /** @var list<\Closure(): string> */
    private array $inputs = [];

    /** @var array<int, true> */
    private array $claimed = [];

    /** @param \Closure(): string $read */
    public function keep(\Closure $read): void
    {
        $this->inputs[] = $read;
    }

    /**
     * Reads the input kept last that no `take` has claimed, claimed before it
     * is read, so a read that reaches `take` again finds the one kept before
     * it; `none` where every input is claimed or none is kept.
     */
    public function take(): string
    {
        for ($index = count($this->inputs) - 1; $index >= 0; $index -= 1) {
            if (!isset($this->claimed[$index])) {
                $this->claimed[$index] = true;

                return self::shown(($this->inputs[$index])());
            }
        }

        return 'none';
    }

    /** Reads the input kept first, claiming nothing; `none` where nothing is kept. */
    public function first(): string
    {
        return $this->inputs === [] ? 'none' : self::shown(($this->inputs[0])());
    }

    // A read that answered the empty string, as a modifier that goes on with
    // it answers: never empty, so a modifier that did not go on shows.
    private static function shown(string $text): string
    {
        return $text === '' ? 'empty' : $text;
    }
}
