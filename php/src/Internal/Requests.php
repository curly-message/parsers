<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

use CurlyMessage\Internal\Intl\Request;
use CurlyMessage\Parser;

/**
 * The observers of the requests a parser's Intl modifiers make of their
 * facilities (SPEC.md section 11.2), for the conformance adapter, the suite
 * and the benchmark. An observer is called with each request once its
 * facility answered or refused it, in walk order, contained as a report
 * handler is. It lives as long as its parser.
 *
 * @internal
 */
final class Requests
{
    /** @var \WeakMap<Parser, \Closure(Request): mixed>|null */
    private static ?\WeakMap $observers = null;

    /**
     * Has `$onRequest` observe every request a resolution of `$parser` makes,
     * in place of the one it observed before.
     *
     * @param \Closure(Request): mixed $onRequest
     */
    public static function observe(Parser $parser, \Closure $onRequest): Parser
    {
        $observers = self::$observers ??= new \WeakMap();
        $observers[$parser] = $onRequest;

        return $parser;
    }

    /** @return (\Closure(Request): mixed)|null */
    public static function of(Parser $parser): ?\Closure
    {
        return self::$observers === null ? null : self::$observers[$parser] ?? null;
    }
}
