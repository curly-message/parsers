<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

use CurlyMessage\Parser;

/**
 * Counts the walks in progress on each thread of execution: each
 * resolution, and each text host code asks the parser to render. Host code
 * the parser calls may begin either again, and PHP ends a recursion that
 * never stops at its memory limit, with a fatal error no `catch` sees, so
 * the count stands in for the host's own limit (SPEC.md section 13). A
 * fiber has a call stack of its own, so it is a thread of execution of its
 * own and keeps a count of its own: what one holds says nothing of what
 * another may begin, whichever of them is suspended. A frame runs on one
 * stack from its first line to its last, so `leave` finds the count `enter`
 * raised by asking which fiber is running, as `enter` did.
 *
 * A host that begins each level of a recursion in a fiber of its own, as an
 * async host awaiting each call does, starts every level at a count of 0,
 * so the resolutions in progress in the process are counted too: that count
 * bounds such a recursion, and what fibers left suspended in resolutions
 * hold, which nothing else does. A resolution is in progress until the call
 * that began it returned and no text of it host code asked for is
 * rendering, in any fiber, and holds one place in the process all that
 * while: it renders each of its texts at most once, so what host code reads
 * of one, at once or one inside another, in fibers of their own or not, is
 * bounded by its message. The last of its walks to end gives the place
 * back, and a text asked for once none is in progress takes it again, as a
 * resolution begun does.
 *
 * @internal
 */
final class Guard
{
    private static int $main = 0;

    private static int $inProcess = 0;

    /** @var \WeakMap<object, int>|null */
    private static ?\WeakMap $fibers = null;

    /**
     * Takes a place on this thread of execution, and one in the process too
     * where `$process` asks for it, or answers why it refuses one, before
     * anything is read. The caller raises the refusal, once it has let go of
     * what the raise's trace would hold.
     */
    public static function enter(bool $process): ?string
    {
        $fiber = \Fiber::getCurrent();
        $fibers = null;
        $count = $fiber === null ? self::$main : ($fibers = self::$fibers ??= new \WeakMap())[$fiber] ?? 0;

        if ($count >= Parser::MAX_REENTRY) {
            return 'A resolution or a text host code asked for was begun while ' . Parser::MAX_REENTRY . ' were in progress on its thread of execution.';
        }

        if ($process) {
            if (self::$inProcess >= Parser::MAX_IN_PROGRESS) {
                return 'A resolution, or a text host code asked for once nothing of its resolution was in progress, was begun while ' . Parser::MAX_IN_PROGRESS . ' resolutions were in progress in the process.';
            }

            self::$inProcess += 1;
        }

        if ($fibers === null) {
            self::$main = $count + 1;
        } else {
            $fibers[$fiber] = $count + 1;
        }

        return null;
    }

    public static function leave(bool $process): void
    {
        if ($process) {
            self::$inProcess -= 1;
        }

        $fiber = \Fiber::getCurrent();

        if ($fiber === null) {
            self::$main -= 1;

            return;
        }

        $fibers = self::$fibers ??= new \WeakMap();
        $count = ($fibers[$fiber] ?? 1) - 1;

        if ($count > 0) {
            $fibers[$fiber] = $count;
        } else {
            unset($fibers[$fiber]);
        }
    }
}
