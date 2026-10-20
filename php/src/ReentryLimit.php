<?php

declare(strict_types=1);

namespace CurlyMessage;

/**
 * Thrown into host code that begins a resolution while `Parser::MAX_REENTRY`
 * are in progress on its thread of execution, or `Parser::MAX_IN_PROGRESS`
 * in the process. PHP ends a recursion that never stops with a fatal error
 * no code can catch, so this stands in for the host's own limit SPEC.md
 * section 13 has re-entry end at. A fiber is a thread of execution of its
 * own, and the count of the process bounds a recursion that begins each
 * level in a fiber of its own, and what fibers left suspended hold. It is
 * thrown before anything is read, and the placeholder that called the host
 * code contains it; a `resolve` no placeholder called fails, and nothing
 * else.
 */
final class ReentryLimit extends \Error {}
