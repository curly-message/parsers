<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Runner;

use CurlyMessage\Conformance\Adapter;
use CurlyMessage\Conformance\CstOffer;
use CurlyMessage\Conformance\Fixture;
use CurlyMessage\Conformance\Options;
use CurlyMessage\Conformance\Performer;
use CurlyMessage\Conformance\Resolution;
use CurlyMessage\Conformance\Result;
use CurlyMessage\Conformance\Runner;

/** Fixture files written in a test, and adapters that answer what the test says. */
trait Cases
{
    /** A fixture file of one level holding the cases given as JSON. */
    private static function file(string $cases, string $level = 'core', string $name = 'cases.json'): Fixture
    {
        $file = json_decode('{"format": "curly-message-3", "level": "' . $level . '", "section": "14.3", "cases": ' . $cases . '}', false, 512, JSON_THROW_ON_ERROR);
        self::assertInstanceOf(\stdClass::class, $file);

        return new Fixture($name, $file);
    }

    /** A tree file holding the cases given as JSON. */
    private static function treeFile(string $cases): Fixture
    {
        $file = json_decode('{"format": "curly-message-3", "kind": "tree", "section": "6", "cases": ' . $cases . '}', false, 512, JSON_THROW_ON_ERROR);
        self::assertInstanceOf(\stdClass::class, $file);

        return new Fixture('tree.json', $file);
    }

    /**
     * An adapter of every level whose resolution answers what `$answer` makes
     * of the inputs.
     *
     * @param \Closure(Resolution): mixed $answer
     */
    private static function answering(\Closure $answer, ?CstOffer $cst = null): Adapter
    {
        return new Adapter(['core', 'intl', 'extensions'], Stub::LIMITS, $answer, null, $cst);
    }

    /** @param list<Fixture> $fixtures */
    private static function over(Adapter $adapter, array $fixtures, ?Performer $performer = null): Result
    {
        return Runner::run($adapter, new Options($fixtures, null, $performer));
    }

    /** The one outcome of a run of one case, as the reason it failed, or `passed` or `unobserved`. */
    private static function outcome(Result $result): string
    {
        if ($result->failed !== []) {
            return $result->failed[0]->outcome->reason;
        }

        return $result->unobserved !== [] ? 'unobserved' : 'passed';
    }
}
