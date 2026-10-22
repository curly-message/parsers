<?php

declare(strict_types=1);

namespace CurlyMessage;

/**
 * Thrown into host code that begins a walk while `Parser::MAX_REENTRY` are in
 * progress on its thread of execution, a `resolve`, or an option's `value()`
 * or an input's `default()` that renders the text it names; or a `resolve`,
 * or a `value()` or `default()` asked for once nothing of its resolution is
 * in progress, while `Parser::MAX_IN_PROGRESS` resolutions are in progress in
 * the process. PHP ends a recursion that never stops with a fatal error no
 * code can catch, so this stands in for the host's own limit SPEC.md section
 * 13 has re-entry end at. A fiber is a thread of execution of its own, and
 * the count of the process bounds a recursion that begins each level in a
 * fiber of its own, and what fibers left suspended in resolutions hold. A `value()` or `default()` also throws this
 * where a rendering of the same text began and none finished, on any thread
 * of execution. It is thrown before anything is read, and the placeholder
 * that called the host code contains it; a `resolve` no placeholder called
 * fails, and nothing else.
 */
final class ReentryLimit extends \Error {}
