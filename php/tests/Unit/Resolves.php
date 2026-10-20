<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\Parser;
use CurlyMessage\Report;

/** Resolves a message and keeps what it reported. */
trait Resolves
{
    /** @var list<Report> */
    private array $reports = [];

    private function parser(bool $recognizeWrappers = true): Parser
    {
        $this->reports = [];

        return new Parser(onReport: function (Report $report): void {
            $this->reports[] = $report;
        }, recognizeWrappers: $recognizeWrappers);
    }

    private function resolve(mixed $message, mixed $payload = null, mixed $id = null): string
    {
        return $this->parser()->resolve($message, $payload, id: $id);
    }

    /** @return list<string> */
    private function codes(): array
    {
        return array_map(static fn(Report $report): string => $report->code->value, $this->reports);
    }
}
