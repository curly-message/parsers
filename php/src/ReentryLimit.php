<?php

declare(strict_types=1);

namespace CurlyMessage;

/**
 * Thrown into host code that begins a resolution while `Parser::MAX_REENTRY`
 * are in progress on its thread of execution. PHP ends a recursion that
 * never stops with a fatal error no code can catch, so this stands in for
 * the host's own limit SPEC.md section 13 has re-entry end at. A fiber is a
 * thread of execution of its own, and a resolution begun on one where none
 * is in progress is never refused, so a recursion that begins its levels in
 * fibers of their own is the host's to bound. It is thrown before anything is
 * read, and the placeholder that called the host code contains it; a
 * `resolve` no placeholder called fails, and nothing else. An option's
 * `value()` and an input's `default()` never throw it: a read the parser
 * refuses answers the empty string.
 */
final class ReentryLimit extends \Error {}
