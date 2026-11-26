<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

use CurlyMessage\Internal\Intl\Request;
use CurlyMessage\Internal\Requests;
use CurlyMessage\Parser;
use CurlyMessage\Report;

/** Resolves a message and keeps what it reported. */
trait Resolves
{
    /** @var list<Report> */
    private array $reports = [];

    /** @var list<Request> */
    private array $requests = [];

    /**
     * @param array<array-key, mixed>|\stdClass $customModifiers
     * @param array<array-key, mixed>|\stdClass $modifierDefaults
     */
    private function parser(bool $recognizeWrappers = true, array|\stdClass $customModifiers = [], array|\stdClass $modifierDefaults = []): Parser
    {
        $this->reports = [];
        $this->requests = [];

        return Requests::observe(new Parser(
            $customModifiers,
            $modifierDefaults,
            onReport: function (Report $report): void {
                $this->reports[] = $report;
            },
            recognizeWrappers: $recognizeWrappers,
        ), function (Request $request): void {
            $this->requests[] = $request;
        });
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
