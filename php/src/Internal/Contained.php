<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

/**
 * Runs what can run host code, so that nothing it raises leaves the
 * resolution. Host code runs where the parser asks it something
 * (`__toString`, `jsonSerialize`, a modifier, a handler) and also where the
 * parser lets go of what it answered, since a `__destruct` runs there. So the
 * work answers only text or a flag, and everything else it held is let go as
 * its frame ends, which is inside the `try` here. A raise is host code as it
 * is let go too: its own `__destruct`, and those of the arguments its trace
 * holds. It is let go inside a `try` as well, and so is each raise that
 * raises in turn, for as long as the host's code raises one.
 *
 * @internal
 */
final class Contained
{
    /**
     * What `$work` answers, or `$raised` where anything in it raised.
     *
     * @template T
     * @template R
     *
     * @param \Closure(): T $work answering nothing host code made but text
     * @param R $raised
     *
     * @return T|R
     */
    public static function run(\Closure $work, mixed $raised): mixed
    {
        try {
            return $work();
        } catch (\Throwable $thrown) {
        }

        self::release($thrown);

        return $raised;
    }

    /**
     * Lets go of each of `$held` inside a `try`, and of whatever that raises
     * in turn, so that no `__destruct` it runs leaves the caller.
     */
    public static function release(mixed &...$held): void
    {
        foreach ($held as &$one) {
            while (true) {
                try {
                    $one = null;

                    break;
                } catch (\Throwable $again) {
                    $one = $again;
                    unset($again);
                }
            }
        }
    }
}
