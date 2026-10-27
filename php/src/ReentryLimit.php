<?php

declare(strict_types=1);

namespace CurlyMessage;

/**
 * Thrown into host code that begins a resolution while `Parser::MAX_REENTRY`
 * are in progress on its thread of execution, or begins one beneath another
 * there while `Parser::MAX_IN_PROGRESS` resolutions so begun are in progress
 * in the process. PHP ends a recursion that never stops with a fatal error
 * no code can catch, so this stands in for the host's own limit SPEC.md
 * section 13 has re-entry end at. A fiber is a thread of execution of its
 * own, and a resolution begun on one where none is in progress is never
 * refused, so a recursion that begins each level in a fiber of its own is
 * the host's to bound; the count of the process adds up the parts of one
 * that stay on one stack past their first level, and what fibers left
 * suspended in such resolutions hold. An option's `value()` or an input's
 * `default()` throws this too where, while its resolution is in progress, a
 * rendering of the same text began and none finished, on any thread of
 * execution. It is thrown before anything is read, and the placeholder that
 * called the host code contains it; a `resolve` no placeholder called fails,
 * and nothing else.
 */
final class ReentryLimit extends \Error {}
