<?php

declare(strict_types=1);

// The public surface as a consumer's analysis reads it. PHPStan analyses this
// file and runs none of it: `composer analyse` against the source, and
// `composer test:dist` against the package a project installed from the
// archive a release ships, so that a signature, a type in a doc comment, or a
// file the archive leaves out that the two disagree on fails one of them.

namespace CurlyMessage\Tests\Types;

use CurlyMessage\Cst;
use CurlyMessage\Cst\NodeType;
use CurlyMessage\Facility;
use CurlyMessage\FormatRequest;
use CurlyMessage\ModifierInput;
use CurlyMessage\Option;
use CurlyMessage\Origin;
use CurlyMessage\Parser;
use CurlyMessage\ReentryLimit;
use CurlyMessage\Report;
use CurlyMessage\ReportCode;
use CurlyMessage\Undefined;

use function PHPStan\Testing\assertType;

function parser(): void
{
    $parser = new Parser(
        customModifiers: ['upper' => static fn(ModifierInput $in): string => strtoupper($in->value), 'none' => static function (ModifierInput $in): void {}],
        modifierDefaults: (object) ['number' => ['maximumFractionDigits' => 1]],
        onReport: static function (Report $report): void {},
        onRequest: static function (FormatRequest $request): void {},
        recognizeWrappers: false,
    );

    assertType('string', $parser->resolve());
    assertType('string', $parser->resolve('{{name}}', payload: ['name' => 'Ada'], props: (object) [], locale: 'en', id: 'greeting'));
    assertType('string', $parser->resolve(Undefined::Value, 1, 'props', 42, Undefined::Value));
    assertType("list<'core'|'extensions'|'intl'>", Parser::levels());
    assertType('array<string, list<string>>', Parser::unexpressible());
    assertType('100000', Parser::MAX_OUTPUT_LENGTH);
    assertType('100000', Parser::MAX_READ_LENGTH);
    assertType('100000', Parser::MAX_CONVERSION_NODES);
    assertType('8', Parser::MAX_NESTING);
    assertType('32', Parser::MAX_REENTRY);
    assertType('512', Parser::MAX_IN_PROGRESS);
    assertType('536870888', Parser::MAX_TEXT_LENGTH);
    assertType('CurlyMessage\Parser', new Parser());

    // A string names a callable PHP may autoload to resolve, which is host
    // code at configuration time, so a handler is a closure.
    /** @phpstan-ignore argument.type */
    $refused = new Parser(onReport: 'var_dump');
}

function modifierInput(ModifierInput $in, Option $option): void
{
    assertType('string', $in->value);
    assertType('list<CurlyMessage\Option>', $in->options);
    assertType('array<mixed>', $in->props);
    assertType('string|null', $in->locale);
    assertType('string', $in->default());
    assertType('string', $option->key);
    assertType('string', $option->value());

    // One is built per call, so a modifier may reorder what it was given.
    usort($in->options, static fn(Option $a, Option $b): int => strcmp($a->key, $b->key));
    $in->props['seen'] = true;
}

function report(Report $report, string $code): void
{
    assertType('CurlyMessage\ReportCode', $report->code);
    assertType('CurlyMessage\Origin', $report->origin);
    assertType('string', $report->message);
    assertType('mixed', $report->id);
    assertType('int|null', $report->limit);
    assertType('string', $report->text);
    assertType('CurlyMessage\Origin', $report->code->origin());
    assertType('int|null', $report->code->limit());
    assertType('string', $report->code->message());
    assertType("'failed-modifier'|'missing-locale'|'missing-options'|'nesting-limit'|'output-limit'|'read-limit'|'unknown-modifier'|'unserializable-value'", $report->code->value);
    assertType("'limit'|'message'|'payload'", $report->origin->value);
    assertType('CurlyMessage\ReportCode', ReportCode::from($code));
    assertType('CurlyMessage\Origin', Origin::from($code));
}

function request(FormatRequest $request, string $api): void
{
    assertType('CurlyMessage\Facility', $request->api);
    assertType('array<mixed>', $request->options);
    assertType('array{float|int, string}|float|int', $request->input);
    assertType('string|null', $request->category);
    assertType("'DateTimeFormat'|'NumberFormat'|'PluralRules'|'RelativeTimeFormat'", $request->api->value);
    assertType('CurlyMessage\Facility', Facility::from($api));
}

function tree(string $type): void
{
    $node = Cst::parse('Hi {{name}}');

    assertType('CurlyMessage\Cst\Node', $node);
    assertType("'utf-8'", Cst::UNIT);
    assertType('CurlyMessage\Cst\NodeType', $node->type);
    assertType('int', $node->start);
    assertType('int', $node->end);
    assertType('list<CurlyMessage\Cst\Node>|null', $node->nodes);
    assertType('string|null', $node->name);
    assertType('bool|null', $node->cancels);
    assertType('array<string, mixed>', $node->toArray());
    assertType('CurlyMessage\Cst\NodeType', NodeType::from($type));
}

function absence(): void
{
    assertType('CurlyMessage\Undefined::Value', Undefined::Value);
    assertType('CurlyMessage\ReentryLimit', new ReentryLimit());
    thrown(new ReentryLimit());
}

function thrown(\Error $error): void {}
