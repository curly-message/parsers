<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

use CurlyMessage\ReportCode;

/**
 * The walk past the nesting limit (SPEC.md section 13). A refused
 * placeholder calls no modifier and renders no option: what it can still
 * reach is its fallback chain, whose last link is message text holding
 * placeholders that are refused in turn. That walk is a loop over an explicit
 * stack of spans, so however deep the message nests its defaults, the host's
 * call stack holds the same frames.
 *
 * @internal
 */
final class Refusal
{
    public function __construct(
        private readonly Resolution $resolution,
        private readonly Scanner $scanner,
        private readonly string $message,
    ) {}

    public function run(int $open, int $close, int $depth): string
    {
        $first = $this->refuse($open, $close);

        if (is_string($first)) {
            return $first;
        }

        /** @var list<Span> $spans */
        $spans = [new Span($first[0], $first[1], $depth + 1)];

        while (($span = end($spans)) !== false) {
            $match = $this->scanner->next($span->at, $span->to);

            if ($match === null) {
                array_pop($spans);
                $done = $span->out . Text::unesc(substr($this->message, $span->at, $span->to - $span->at));
                $parent = end($spans);

                // A refused placeholder resolves to its inline default as
                // that span renders, so the span's text is its result.
                if ($parent === false) {
                    return $done;
                }

                $parent->out .= $done;

                continue;
            }

            [$inner, $innerClose] = $match;
            $span->out .= Text::unesc(substr($this->message, $span->at, $inner - $span->at));
            $span->at = $innerClose;
            $next = $this->refuse($inner, $innerClose);

            if (is_string($next)) {
                $span->out .= $next;
            } else {
                $spans[] = new Span($next[0], $next[1], $span->depth + 1);
            }
        }

        return '';
    }

    /**
     * One refused placeholder: the steps every placeholder takes, the report,
     * then the links of the chain the payload holds. Its result, or the span
     * of its inline default where neither link answers and it has one.
     *
     * @return string|array{int, int}
     */
    private function refuse(int $open, int $close): string|array
    {
        $declared = $this->resolution->declared($open, $close);

        if ($declared === null) {
            return '';
        }

        $this->resolution->report(ReportCode::NestingLimit, $open, $close);

        return $this->resolution->payloadDefault($declared) ?? $declared->placeholder->inlineDefault ?? '';
    }
}
