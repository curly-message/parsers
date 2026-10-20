<?php

declare(strict_types=1);

namespace CurlyMessage;

use CurlyMessage\Internal\Contained;
use CurlyMessage\Internal\Conversion;
use CurlyMessage\Internal\Guard;
use CurlyMessage\Internal\Resolution;

/**
 * Resolves messages of the Curly Message Format (SPEC.md), built once with
 * its options and asked for any number of messages.
 */
final class Parser
{
    /** Bytes the placeholders of one resolution place in its output. */
    public const MAX_OUTPUT_LENGTH = 100_000;

    /** Bytes of value text one resolution reads from its payload. */
    public const MAX_READ_LENGTH = 100_000;

    /** Values one serialization visits. */
    public const MAX_CONVERSION_NODES = 100_000;

    /** Levels of placeholder one resolution resolves, the outermost first. */
    public const MAX_NESTING = 8;

    /**
     * Bytes of one text the parser writes: a value's JSON, and what a level
     * below the message holds of the results it places. It counts bytes of
     * UTF-8, where V8, whose longest string is this many UTF-16 code units,
     * counts units, so text outside ASCII meets it at fewer characters than
     * the JavaScript parser does; section 13 asks a host to hold at least
     * 100 000 units of its own.
     */
    public const MAX_TEXT_LENGTH = 536_870_888;

    /**
     * Walks in progress on one thread of execution, past which `resolve`, or
     * a text host code asks to be rendered, throws `ReentryLimit`: each
     * resolution, and each option or fallback chain host code asks for, as
     * it renders or after. A fiber is a thread of execution of its own.
     */
    public const MAX_REENTRY = 32;

    /**
     * Resolutions in progress in the process, every fiber's included,
     * suspended or not, past which `resolve`, or a text host code asks for
     * once nothing of its resolution is in progress, throws `ReentryLimit`.
     * It bounds a recursion that begins each level in a fiber of its own,
     * which `MAX_REENTRY` counts afresh at each level, and what fibers left
     * suspended in resolutions hold. A resolution is in progress until its
     * `resolve` returned and no text of it host code asked for is rendering,
     * in any fiber, and takes one place all that while: it renders each of
     * its texts at most once, so however many of them host code reads, at
     * once or one inside another, in fibers of their own or not, during the
     * resolution or after it, they are bounded by the texts the message
     * declares rather than by either count, and each resolution host code
     * begins takes a place of its own. A text asked for once nothing of its
     * resolution is in progress takes the place again, as a resolution does.
     */
    public const MAX_IN_PROGRESS = 512;

    /**
     * @param array<array-key, mixed>|\stdClass $customModifiers the modifiers the host defines, by name, each a `\Closure` or an object with `__invoke`; an entry of any other kind registers nothing and shadows nothing
     * @param array<array-key, mixed>|\stdClass $modifierDefaults the props each modifier is configured with, by its name
     * @param (\Closure(Report): mixed)|null $onReport called with each report, in walk order
     * @param bool $recognizeWrappers whether an entry holding only `value`, `default` and `props` configures its value (section 4.1)
     */
    public function __construct(
        private readonly array|\stdClass $customModifiers = [],
        private readonly array|\stdClass $modifierDefaults = [],
        private readonly ?\Closure $onReport = null,
        private readonly bool $recognizeWrappers = true,
    ) {}

    /**
     * The text a message resolves to. It never throws, but for `ReentryLimit`
     * where a resolution begins while `MAX_REENTRY` walks are in progress on
     * its thread of execution, or `MAX_IN_PROGRESS` resolutions in the
     * process, a resolution being in progress until its `resolve` returned
     * and no text of it host code asked for is rendering. A `__destruct` PHP's cycle collector runs while it resolves is
     * the host's garbage rather than code the parser called, and what it
     * throws leaves wherever the collector ran, here as in any code: from
     * `resolve`, or, where the collector ran inside what the parser contains
     * of a placeholder, as that placeholder's failure.
     *
     * @param mixed $message the message; `Undefined::Value` is none, and resolves to ''
     * @param mixed $payload an array or a `stdClass`; anything else is no payload
     * @param mixed $props an array or a `stdClass` of props by modifier name; anything else is none
     * @param mixed $locale a language tag; anything but a string is none
     * @param mixed $id what each report carries to name the message; `Undefined::Value` is none
     */
    public function resolve(
        mixed $message = Undefined::Value,
        mixed $payload = null,
        mixed $props = null,
        mixed $locale = null,
        mixed $id = null,
    ): string {
        $refused = Guard::enter(true);

        if ($refused !== null) {
            // A raise keeps the arguments of the frames it is made in where
            // the host has PHP keep them (`zend.exception_ignore_args` off),
            // and the caller would let go of them as it lets go of the
            // refusal: so they are let go first, inside the containment.
            Contained::release($message, $payload, $props, $locale, $id);

            throw new ReentryLimit($refused);
        }

        $conversion = null;
        $resolution = null;

        try {

            // One record for the resolution, which the message's own
            // conversion enters first: a value the payload holds as well
            // converts once.
            $conversion = new Conversion();
            // A message nothing describes is a message nobody wrote, and
            // nothing behind it is read on its account.
            $text = $conversion->text($message, 'message');

            if ($text === null) {
                return '';
            }

            $resolution = new Resolution(
                $text,
                $payload,
                $props,
                is_string($locale) ? $locale : null,
                $id === Undefined::Value ? null : $id,
                $this->customModifiers,
                $this->modifierDefaults,
                $this->onReport,
                $this->recognizeWrappers,
                $conversion,
            );

            return $resolution->run();
        } finally {
            // The place in the process is the resolution's while a text of it
            // host code asked for renders, and the last of them gives it back.
            $last = $resolution === null || $resolution->end();
            // An argument, or what the call made and recorded as it read, may
            // hold the last reference to what the caller handed over, which
            // is let go where nothing it runs leaves the call.
            Contained::release($message, $payload, $props, $locale, $id, $resolution, $conversion);
            Guard::leave($last);
        }
    }
}
