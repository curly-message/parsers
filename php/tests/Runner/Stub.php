<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Runner;

use CurlyMessage\Conformance\Adapter;
use CurlyMessage\Conformance\CstOffer;
use CurlyMessage\Conformance\Fixture;
use CurlyMessage\Conformance\Generators;
use CurlyMessage\Conformance\Resolution;
use CurlyMessage\Conformance\Runner;

/**
 * An adapter that answers the small set beside this file from a table, as a
 * conforming implementation would: no parser behind it, so what the runner
 * makes of it is the runner's alone.
 */
final class Stub
{
    public const LIMITS = ['output' => 5, 'read' => 4, 'conversion' => 6, 'nesting' => 3];

    public const TREE = "\u{e9}{{k\\;:m}}";

    /** @return list<Fixture> */
    public static function fixtures(): array
    {
        return Runner::pinned()->load(__DIR__ . '/set');
    }

    /** @param list<string> $levels */
    public static function adapter(bool $reports = true, bool $formats = true, bool $tree = true, array $levels = ['core', 'intl', 'extensions']): Adapter
    {
        $table = self::table();

        return new Adapter(
            $levels,
            self::LIMITS,
            static function (Resolution $input) use ($table, $reports, $formats): array {
                $message = is_string($input->message) ? $input->message : '';
                $answer = $table[$message] ?? throw new \LogicException("The stub has no answer for {$message}.");

                if (!$reports) {
                    unset($answer['reports']);
                }

                if (!$formats) {
                    unset($answer['formats']);
                }

                return $answer;
            },
            null,
            $tree ? new CstOffer('utf-8', static fn(string $message): array => $message === self::TREE ? self::tree() : throw new \LogicException("The stub has no tree for {$message}.")) : null,
        );
    }

    /**
     * The answer to each message of the small set, by message.
     *
     * @return array<string, array{output: string, reports: list<array<array-key, mixed>>, formats: list<array<string, mixed>>}>
     */
    private static function table(): array
    {
        $table = [
            self::TREE => ['output' => "\u{e9}", 'reports' => [], 'formats' => []],
            'edges' => ['output' => ' both ends ', 'reports' => [], 'formats' => []],
            'two' => ['output' => 'two', 'reports' => [
                ['code' => 'unknown-modifier', 'origin' => 'message', 'id' => 'm'],
                ['code' => 'unserializable-value', 'origin' => 'payload', 'id' => 'm'],
            ], 'formats' => []],
            '{{n:number}}' => ['output' => '', 'reports' => [], 'formats' => [['api' => 'NumberFormat', 'options' => ['maximumFractionDigits' => 2], 'input' => 1.5]]],
            '{{n:plural; one:single; other:many}}' => ['output' => 'single', 'reports' => [], 'formats' => [['api' => 'PluralRules', 'options' => ['type' => 'cardinal'], 'input' => 1, 'category' => 'one']]],
        ];

        foreach (['output-over-limit', 'nesting-over-limit'] as $name) {
            $prepared = Generators::build($name, self::LIMITS, null);
            $message = $prepared->input->message;
            $table[is_string($message) ? $message : throw new \LogicException("The {$name} construction holds no message.")] = ['output' => (string) $prepared->expected->output, 'reports' => $prepared->expected->reports, 'formats' => []];
        }

        return $table;
    }

    /** @return array<string, mixed> */
    private static function tree(): array
    {
        $text = static fn(int $start, int $end): array => ['type' => 'text', 'start' => $start, 'end' => $end];

        return ['type' => 'message', 'start' => 0, 'end' => 11, 'nodes' => [
            $text(0, 2),
            ['type' => 'placeholder', 'start' => 2, 'end' => 11, 'nodes' => [
                ['type' => 'open', 'start' => 2, 'end' => 4],
                ['type' => 'key', 'start' => 4, 'end' => 7, 'name' => 'k;', 'nodes' => [$text(4, 5), ['type' => 'escape', 'start' => 5, 'end' => 7, 'cancels' => true]]],
                ['type' => 'separator', 'start' => 7, 'end' => 8],
                ['type' => 'modifier', 'start' => 8, 'end' => 9, 'name' => 'm', 'nodes' => [$text(8, 9)]],
                ['type' => 'close', 'start' => 9, 'end' => 11],
            ]],
        ]];
    }
}
