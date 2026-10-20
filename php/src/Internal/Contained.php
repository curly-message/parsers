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
 * Each thread of execution keeps the host code the parser is running on it,
 * the innermost on top, so that a text host code asks for can tell whether
 * the modifier it was handed to is the one asking, and can fail the host
 * code that asked where it is refused.
 *
 * @internal
 */
final class Contained
{
    private static ?HostFrame $mainTop = null;

    /** @var \WeakMap<object, HostFrame>|null */
    private static ?\WeakMap $fiberTops = null;

    /**
     * What `$work` answers, or `$raised` where anything in it raised, or
     * where a text it asked for was refused, whatever it answered after.
     * The host code runs on top of what its thread of execution runs, as
     * `$frame` where it is given one.
     *
     * @template T
     * @template R
     *
     * @param \Closure(): T $work answering nothing host code made but text
     * @param R $raised
     *
     * @return T|R
     */
    public static function run(\Closure $work, mixed $raised, ?HostFrame $frame = null): mixed
    {
        $frame ??= new HostFrame();
        $fiber = \Fiber::getCurrent();
        $main = $fiber === null;

        if ($main) {
            $frame->below = self::$mainTop;
            self::$mainTop = $frame;
        } else {
            $tops = self::$fiberTops ??= new \WeakMap();
            $frame->below = $tops[$fiber] ?? null;
            $tops[$fiber] = $frame;
            // What the fiber's stack holds of the fiber itself would keep
            // it alive once the host let go of it.
            unset($fiber, $tops);
        }

        try {
            try {
                $answer = $work();

                return $frame->refused ? $raised : $answer;
            } catch (\Throwable $thrown) {
            }

            self::release($thrown);

            return $raised;
        } finally {
            if ($main) {
                self::$mainTop = $frame->below;
            } else {
                self::pop($frame);
            }
        }
    }

    /** The host code the parser runs innermost on this thread of execution, if any. */
    public static function top(): ?HostFrame
    {
        $fiber = \Fiber::getCurrent();

        return $fiber === null ? self::$mainTop : self::$fiberTops[$fiber] ?? null;
    }

    /**
     * Fails the host code the parser runs innermost on this thread of
     * execution, whatever it answers after. On a thread where it runs none,
     * as in a fiber host code started, what fails is the innermost it runs
     * on the thread that started or resumed this one and is waiting on it,
     * or on the one beneath that, down to the main thread; where it runs
     * none on any of them, nothing fails.
     */
    public static function fail(): void
    {
        $fiber = \Fiber::getCurrent();

        if ($fiber === null) {
            self::failMain();

            return;
        }

        $tops = self::$fiberTops ??= new \WeakMap();
        $frame = $tops[$fiber] ?? null;

        if ($frame !== null) {
            $frame->refuse();

            return;
        }

        // A thread waiting on a fiber is switched out at its call to that
        // fiber's `start`, `resume` or `throw`, and this fiber's trace goes
        // on into the frames of each thread waiting beneath it: so those
        // calls name this fiber and then each fiber waiting beneath it, the
        // innermost first, and the main thread waits beneath the last.
        foreach (debug_backtrace(\DEBUG_BACKTRACE_PROVIDE_OBJECT | \DEBUG_BACKTRACE_IGNORE_ARGS) as $call) {
            $called = ($call['class'] ?? null) === \Fiber::class ? $call['object'] ?? null : null;
            $frame = $called instanceof \Fiber ? $tops[$called] ?? null : null;

            if ($frame !== null) {
                $frame->refuse();

                return;
            }
        }

        self::failMain();
    }

    private static function failMain(): void
    {
        self::$mainTop?->refuse();
    }

    /** Takes `$frame` off the top of the fiber it runs on. */
    private static function pop(HostFrame $frame): void
    {
        $fiber = \Fiber::getCurrent();

        if ($fiber === null || self::$fiberTops === null) {
            return;
        }

        if ($frame->below === null) {
            unset(self::$fiberTops[$fiber]);
        } else {
            self::$fiberTops[$fiber] = $frame->below;
        }
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
