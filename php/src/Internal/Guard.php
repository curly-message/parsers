<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

use CurlyMessage\Parser;

/**
 * Counts the resolutions in progress on each thread of execution. Host code
 * the parser calls may begin one again, and PHP ends a recursion that never
 * stops at its memory limit, with a fatal error no `catch` sees, so the
 * count stands in for the host's own limit (SPEC.md section 13). A fiber
 * has a call stack of its own, so it is a thread of execution of its own
 * and keeps a count of its own: what one holds says nothing of what another
 * may begin, whichever of them is suspended. A frame runs on one stack from
 * its first line to its last, so `leave` finds the count `enter` raised by
 * asking which fiber is running, as `enter` did.
 *
 * A resolution asked for on a thread of execution where none is in
 * progress is never refused, as section 13 asks, so a host that begins each
 * level of a recursion in a fiber of its own, as an async host awaiting
 * each call does, begins every level at a count of 0, and the recursion is
 * the host's to bound, as the fibers it starts are. A resolution begun
 * beneath another on its thread takes a place in the process too: those
 * places add up the parts of such a recursion that each stay on one stack
 * past their first level, and what fibers left suspended in such
 * resolutions hold.
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
     * where a resolution is in progress on the thread already, before
     * anything is read: whether it took one in the process, or why it
     * refuses either. The caller raises the refusal, once it has let go of
     * what the raise's trace would hold.
     */
    public static function enter(): string|bool
    {
        $fiber = \Fiber::getCurrent();
        $fibers = null;
        $count = $fiber === null ? self::$main : ($fibers = self::$fibers ??= new \WeakMap())[$fiber] ?? 0;

        if ($count >= Parser::MAX_REENTRY) {
            return 'A resolution was begun while ' . Parser::MAX_REENTRY . ' were in progress on its thread of execution.';
        }

        $placed = $count > 0;

        if ($placed) {
            if (self::$inProcess >= Parser::MAX_IN_PROGRESS) {
                return 'A resolution was begun beneath another on its thread of execution while ' . Parser::MAX_IN_PROGRESS . ' resolutions so begun were in progress in the process.';
            }

            self::$inProcess += 1;
        }

        if ($fibers === null) {
            self::$main = $count + 1;
        } else {
            $fibers[$fiber] = $count + 1;
        }

        return $placed;
    }

    public static function leave(bool $placed): void
    {
        if ($placed) {
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
