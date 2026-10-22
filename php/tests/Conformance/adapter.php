<?php

declare(strict_types=1);

use CurlyMessage\Conformance\Adapter;
use CurlyMessage\Conformance\BehaviourInput;
use CurlyMessage\Conformance\BehaviourOption;
use CurlyMessage\Conformance\CstOffer;
use CurlyMessage\Conformance\Resolution;
use CurlyMessage\Cst;
use CurlyMessage\FormatRequest;
use CurlyMessage\ModifierInput;
use CurlyMessage\Option;
use CurlyMessage\Parser;
use CurlyMessage\Report;

// The adapter of the specification's section 14.3: a fresh parser per case,
// each behaviour a host-defined modifier, every report and request recorded
// field by field, and the tree as `Cst` describes it.
return new Adapter(
    Parser::levels(),
    ['output' => Parser::MAX_OUTPUT_LENGTH, 'read' => Parser::MAX_READ_LENGTH, 'conversion' => Parser::MAX_CONVERSION_NODES, 'nesting' => Parser::MAX_NESTING],
    static function (Resolution $input): array {
        $reports = [];
        $formats = [];
        $modifiers = array_map(
            static fn(Closure $behaviour): Closure => static fn(ModifierInput $in): mixed => $behaviour(new BehaviourInput(
                $in->value,
                array_map(static fn(Option $option): BehaviourOption => new BehaviourOption($option->key, $option->value(...)), $in->options),
                $in->props,
                $in->locale,
                $in->default(...),
            )),
            $input->modifiers,
        );
        $defaults = $input->defaults;
        $parser = new Parser(
            $modifiers,
            is_array($defaults) || $defaults instanceof stdClass ? $defaults : [],
            static function (Report $report) use (&$reports): void {
                $reports[] = array_filter(['code' => $report->code->value, 'origin' => $report->origin->value, 'id' => $report->id, 'limit' => $report->limit], static fn(mixed $field): bool => $field !== null);
            },
            static function (FormatRequest $request) use (&$formats): void {
                $formats[] = array_filter(['api' => $request->api->value, 'options' => $request->options, 'input' => $request->input, 'category' => $request->category], static fn(mixed $field): bool => $field !== null);
            },
        );
        $output = $parser->resolve($input->message, $input->payload, $input->props, $input->locale, $input->id);

        return ['output' => $output, 'reports' => $reports, 'formats' => $formats];
    },
    Parser::unexpressible(),
    new CstOffer(Cst::UNIT, static fn(string $message): array => Cst::parse($message)->toArray()),
);
