<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

use CurlyMessage\FormatRequest;
use CurlyMessage\Internal\Intl\Available;
use CurlyMessage\Internal\Intl\Modifiers;
use CurlyMessage\ModifierInput;
use CurlyMessage\Option;
use CurlyMessage\Parser;
use CurlyMessage\ReentryLimit;
use CurlyMessage\Report;
use CurlyMessage\ReportCode;
use CurlyMessage\Undefined;

/**
 * One resolution of one message (SPEC.md section 5): the walk, the two
 * budgets it spends, and what it has recorded. The message's own text is
 * syntax and is scanned for placeholders; what a placeholder resolves to is
 * data and is never scanned again.
 *
 * @internal
 */
final class Resolution
{
    // The entries a payload reserves for a value's own configuration.
    private const WRAPPED = ['value', 'default', 'props'];

    // The selections the format defines: a placeholder naming one with no
    // options was asked to select from nothing.
    private const SELECTIONS = ['eq', 'ne', 'lt', 'gt', 'lte', 'gte', 'plural', 'ordinal'];

    private readonly Scanner $scanner;

    private readonly Layers $layers;

    // What the output carries and what the payload is read for are budgets of
    // the resolution rather than of a placeholder: what one placeholder spends
    // is gone for every placeholder after it (section 13).
    private int $output = Parser::MAX_OUTPUT_LENGTH;

    private int $read = Parser::MAX_READ_LENGTH;

    // Once the `resolve` that made it returned, nothing resolves, reports or
    // runs host code on the resolution's account (SPEC.md section 11.3): not
    // a text host code kept and reads later, and not a rendering a fiber left
    // suspended in it and resumes.
    private bool $closed = false;

    /**
     * @param mixed $id the id reports carry, null where none
     */
    public function __construct(
        private readonly string $message,
        private readonly mixed $payload,
        private readonly mixed $props,
        private readonly ?string $locale,
        private readonly mixed $id,
        private readonly mixed $customModifiers,
        private readonly mixed $modifierDefaults,
        private readonly ?\Closure $onReport,
        private readonly ?\Closure $onRequest,
        private readonly bool $recognizeWrappers,
        private readonly Conversion $conversion,
    ) {
        $this->scanner = new Scanner($message);
        $this->layers = new Layers();
    }

    public function run(): string
    {
        try {
            return $this->render(0, strlen($this->message), 1);
        } finally {
            $this->closed = true;
            $this->release();
        }
    }

    /**
     * A span of the message: the text it writes itself, its escape sequences
     * resolved, and the placeholders it writes resolved. The message is one
     * such span, and so is each option value a modifier asks for.
     */
    public function render(int $from, int $to, int $depth): string
    {
        $out = '';
        $at = $from;

        while (($match = $this->scanner->next($at, $to)) !== null) {
            [$open, $close] = $match;
            $out .= Text::unesc(substr($this->message, $at, $open - $at));
            $resolved = $this->placeholder($open, $close, $depth);

            // Only what the output carries is counted, and a nested result
            // reaches it through the one around it, so a result is charged
            // where the walk meets the message itself, once. The message's own
            // text is the caller's and always renders. A level below the
            // message holds what it places up to the longest text the parser
            // writes.
            if ($depth > 1) {
                if (strlen($resolved) <= Parser::MAX_TEXT_LENGTH - strlen($out)) {
                    $out .= $resolved;
                } else {
                    $this->report(ReportCode::OutputLimit, $open, $close, Parser::MAX_TEXT_LENGTH);
                }
            } elseif (strlen($resolved) <= $this->output) {
                $this->output -= strlen($resolved);
                $out .= $resolved;
            } else {
                $this->report(ReportCode::OutputLimit, $open, $close);
            }

            $at = $close;
        }

        return $out . Text::unesc(substr($this->message, $at, $to - $at));
    }

    /**
     * Reports a condition met at the placeholder spelled from `$open` to
     * `$close`, whose spelling the report carries as an excerpt.
     *
     * @param int|null $limit the limit reached, where it is not the one the code names
     */
    public function report(ReportCode $code, int $open, int $close, ?int $limit = null): void
    {
        $onReport = $this->onReport;

        if ($onReport === null || $this->closed) {
            return;
        }

        $report = new Report($code, $code->origin(), $code->message(), $this->id, $limit ?? $code->limit(), Excerpt::of($this->message, $open, $close));

        // Reporting is an observation, not a step of the resolution: a host
        // whose handler fails, or answers what fails as it is let go, still
        // gets its message back.
        Contained::run(static function () use ($onReport, $report): null {
            $onReport($report);

            return null;
        }, null);
    }

    /**
     * The first steps every placeholder takes, a refused one included: the
     * read budget tested before it reads, its declaration, and its value
     * read. Null where the budget is spent, and the placeholder resolves to
     * the empty string.
     */
    public function declared(int $open, int $close): ?Declared
    {
        if ($this->closed) {
            return null;
        }

        // The budget is tested before this placeholder reads, so the one that
        // spent it past the limit resolved all the same, and this one pays.
        if ($this->read < 0) {
            $this->report(ReportCode::ReadLimit, $open, $close);

            return null;
        }

        $placeholder = Placeholder::parse($this->message, $open, $close, $this->scanner);
        $raised = fn() => $this->report(ReportCode::UnserializableValue, $open, $close);
        $key = $placeholder->key;
        $entry = $this->layers->own($this->payload, $key);
        // The payload's own `default` is the fallback itself, never
        // configuration, and a caller that turned recognition off hands over
        // entries it did not write, so an entry shaped like a wrapper is a
        // value there.
        $wrapper = $this->recognizeWrappers && $key !== 'default' && $this->isWrapper($entry) ? $entry : null;
        $value = $wrapper === null ? $entry : $this->layers->own($wrapper, 'value');
        // The payload's `default` is read at one place, whether a placeholder
        // names it or falls back to it.
        $place = match (true) {
            $wrapper !== null => 'wv:' . $key,
            $key === 'default' => 'pd',
            default => 'v:' . $key,
        };
        $valueText = $this->payloadText($value, $place, $raised);

        return new Declared($open, $close, $placeholder, $wrapper, $valueText, $raised);
    }

    /**
     * The fallback chain's links the payload holds (section 10): the
     * wrapper's own default, then the payload's. A placeholder naming
     * `default` itself has read the payload's as its value, and one entry is
     * read once.
     */
    public function payloadDefault(Declared $declared): ?string
    {
        if ($this->closed) {
            return null;
        }

        $key = $declared->placeholder->key;
        $own = $declared->wrapper === null ? null : $this->payloadText($this->layers->own($declared->wrapper, 'default'), 'wd:' . $key, $declared->raised);

        if ($own !== null || $this->closed) {
            return $own;
        }

        return $key === 'default' ? $declared->valueText : $this->payloadText($this->layers->own($this->payload, 'default'), 'pd', $declared->raised);
    }

    private function placeholder(int $open, int $close, int $depth): string
    {
        // Past the limit a placeholder resolves only its fallback chain, and
        // the walk down that chain is a loop rather than a call per level.
        if ($depth > Parser::MAX_NESTING) {
            return (new Refusal($this, $this->scanner, $this->message))->run($open, $close, $depth);
        }

        $declared = $this->declared($open, $close);

        // Reading the value may suspend the fiber it runs in, which may be
        // resumed once the resolution returned.
        if ($declared === null || $this->closed) {
            return '';
        }

        return $this->resolved($declared, $open, $close, $depth);
    }

    /** A placeholder within the nesting limit, once it has read its value. */
    private function resolved(Declared $declared, int $open, int $close, int $depth): string
    {
        $placeholder = $declared->placeholder;
        // The wrapper speaks for its own value, the payload for every key it
        // does not carry, and the message only for what neither says. A link
        // waits for the one before it to come back empty, and the chain waits
        // for a reader: the parser, or host code.
        $chain = null;
        $default = function (bool $hosted = false) use (&$chain, $declared, $placeholder, $depth): string {
            return $this->once($chain, fn(): string => $this->payloadDefault($declared)
                ?? ($placeholder->inlineDefault === null ? '' : $this->render($placeholder->inlineDefault[0], $placeholder->inlineDefault[1], $depth + 1)), $hosted);
        };

        $modifier = $placeholder->modifier;
        $name = $modifier === '' ? 'eq' : $modifier;
        $hosted = $this->hosted($name);
        // The Intl level's modifiers are the format's only where the host
        // satisfies it; elsewhere nobody registered them.
        $intl = $hosted === null && in_array($name, Modifiers::NAMES, true) && Available::satisfied();

        // A modifier nobody registered is a defect in the message, not a
        // selection: running `eq` in its place would answer a question the
        // message never asked.
        if ($modifier !== '' && $hosted === null && !$intl && !in_array($modifier, Comparisons::NAMES, true)) {
            $this->report(ReportCode::UnknownModifier, $open, $close);

            return $default();
        }

        // A selection declaring no options was asked to select from nothing,
        // which is how the placeholder is written, so it is reported before
        // the value is asked anything. One naming no key compares nothing and
        // is no selection, and neither is a host's own modifier under a
        // selection's name, which may answer from nothing.
        if ($placeholder->key !== null && $placeholder->options === [] && $hosted === null && in_array($modifier, self::SELECTIONS, true)) {
            $this->report(ReportCode::MissingOptions, $open, $close);

            return $default();
        }

        // An absent value is nothing to compare against, whatever the
        // modifier asks.
        if ($declared->valueText === null) {
            return $default();
        }

        if ($modifier === '' && $placeholder->options === []) {
            return $declared->valueText;
        }

        $options = [];

        foreach ($placeholder->options as [$optionKey, [$from, $to]]) {
            $options[] = $this->option($optionKey, $from, $to, $depth + 1, $hosted !== null);
        }

        $layers = [$this->modifierDefaults, $this->props, $declared->wrapper === null ? null : $this->layers->own($declared->wrapper, 'props')];
        $props = $this->layers->compose($layers, $name);

        // The format's own `plural` takes its category from the number
        // `number` shows, so the digits composed under `number` lie beneath
        // its own.
        if ($intl && $name === 'plural') {
            $props = array_replace($this->layers->compose($layers, 'number', Modifiers::SHOWN_DIGITS), $props);
        }

        if ($hosted === null) {
            $input = new ModifierInput($declared->valueText, $options, $props, $this->locale, $default);

            return $intl ? $this->intl($name, $input, $declared, $default) : Comparisons::call($name, $input);
        }

        return $this->call($hosted, fn(): ModifierInput => new ModifierInput($declared->valueText, $options, $props, $this->locale, fn(): string => $default(true)), $declared, $default);
    }

    /**
     * An Intl modifier's answer. One that could not answer reports why, and
     * takes the fallback chain, but for a missing locale: a declared default
     * stands in for a value the modifier cannot read, never for the locale
     * it would have formatted in.
     *
     * @param \Closure(): string $default
     */
    private function intl(string $name, ModifierInput $input, Declared $declared, \Closure $default): string
    {
        $answer = Modifiers::call($name, $input, $this->observe(...));

        if (!$answer instanceof ReportCode) {
            return $answer;
        }

        $this->report($answer, $declared->open, $declared->close);

        return $answer === ReportCode::MissingLocale ? '' : $default();
    }

    /**
     * Hands a request to the caller's observer. Observing is not a step of
     * the resolution: a handler that fails, or answers what fails as it is
     * let go, changes nothing it resolves to.
     */
    private function observe(FormatRequest $request): void
    {
        $onRequest = $this->onRequest;

        if ($onRequest === null || $this->closed) {
            return;
        }

        Contained::run(static function () use ($onRequest, $request): null {
            $onRequest($request);

            return null;
        }, null);
    }

    /**
     * The modifier the host registered under `$name`, or null where its entry
     * is none: a modifier is a `\Closure` or an object with `__invoke`, and
     * telling either runs no host code. A string or an array is not asked,
     * since asking whether one names a method can load a class.
     *
     * @return ((callable(ModifierInput): mixed)&object)|null
     */
    private function hosted(string $name): ?object
    {
        $entry = $this->layers->own($this->customModifiers, $name);

        return is_object($entry) && is_callable($entry) ? $entry : null;
    }

    /**
     * A host modifier's answer, which converts as a payload entry does: what
     * it builds stays structured rather than becoming PHP's word for it. A
     * modifier that answers nothing, as a PHP function does by answering
     * `null`, or answers what nothing describes, or raises, has not answered,
     * and the placeholder takes the fallback chain; containing the raise is
     * what keeps it out of the caller's render path, and the report is what
     * keeps the caller hearing of it. The answer is this placeholder's
     * result rather than text it read, so it is bounded by the output limit
     * and spends nothing of the read budget.
     *
     * @param (callable(ModifierInput): mixed)&object $modifier
     * @param \Closure(): ModifierInput $input
     * @param \Closure(): string $default
     */
    private function call(object $modifier, \Closure $input, Declared $declared, \Closure $default): string
    {
        // The input is built and the answer converted inside the containment,
        // and both are let go as it ends, with whatever the modifier hung on
        // either: what they run as they go is the modifier's, contained as
        // its raise is.
        $text = Contained::run(function () use ($modifier, $input): string|false|null {
            $answer = $modifier($input());

            // The modifier may have suspended the fiber it runs in, and been
            // resumed once the resolution returned.
            return $answer === null || $answer === Undefined::Value || $this->closed ? false : $this->conversion->text($answer);
        }, true);

        if ($text === true) {
            $this->report(ReportCode::FailedModifier, $declared->open, $declared->close);

            return $default();
        }

        if ($text === false) {
            return $default();
        }

        if ($text === null) {
            ($declared->raised)();

            return $default();
        }

        return $text;
    }

    /**
     * An option whose value is rendered where it is first asked for and not
     * again; one the modifier passes over is never rendered at all. Host
     * code asks for what a host modifier is handed (`$hosted`), and the
     * parser for what its own modifiers are.
     */
    private function option(string $key, int $from, int $to, int $depth, bool $hosted): Option
    {
        $rendered = null;

        return new Option($key, function () use (&$rendered, $from, $to, $depth, $hosted): string {
            return $this->once($rendered, fn(): string => $this->render($from, $to, $depth), $hosted);
        });
    }

    /**
     * Text rendered on the first read, at most once, and the text that one
     * rendering made on every read after it finished. `$state` is that text
     * once there is one; before that, true once the rendering began, and
     * null where it did not. A rendering a fiber left suspended, or let go
     * of while it was suspended, leaves it true, and no read renders the
     * text again: one that did would, where a modifier reads its text in
     * such a fiber at each level of a message, double what each level
     * walks, however the host schedules its fibers.
     *
     * What the parser reads is bounded by the nesting limit, and never
     * throws: its read of a text whose rendering began and has not finished
     * is the empty string. What host code asks for, it may ask for from
     * inside the text's own rendering, or from inside another text it kept
     * from elsewhere in the message, each read inside the one before. Each
     * text renders at most once, so such reads nest no deeper than the texts
     * the message declares, and they are the walk of the resolution, which
     * takes no place among the resolutions in progress (SPEC.md section 13).
     * A text asked for once a rendering of it began and before one finished
     * is refused: one asked for beneath its own rendering would render
     * inside itself, and one whose rendering a fiber left suspended may
     * never finish. The refusal throws into the host code that asked, which
     * the placeholder that called that code contains.
     *
     * Once the resolution returned, a read answers the text a rendering
     * finished during it, and the empty string where none did, rendering,
     * reporting and running nothing (section 11.3). A rendering a fiber left
     * suspended and resumes then resolves nothing more, and answers the
     * empty string too.
     *
     * @param string|true|null $state
     * @param \Closure(): string $render
     *
     * @param-out string|true|null $state
     */
    private function once(string|true|null &$state, \Closure $render, bool $hosted): string
    {
        if (is_string($state)) {
            return $state;
        }

        if ($this->closed) {
            return '';
        }

        if ($state !== null) {
            if ($hosted) {
                throw new ReentryLimit('A text was asked for while it was rendering.');
            }

            return '';
        }

        $state = true;
        $text = $render();

        return $this->closed ? '' : $state = $text;
    }

    /**
     * Lets go of what the resolution read and recorded, where nothing a
     * `__destruct` runs leaves it.
     */
    private function release(): void
    {
        $this->layers->release();
        $this->conversion->release();
    }

    /**
     * Value text is what a placeholder takes from the payload, and every byte
     * of it is spent whether or not any of it reaches the output. Converting
     * it may run host code that suspends the fiber it runs in.
     *
     * @phpstan-impure
     */
    private function payloadText(mixed $declared, string $place, \Closure $raised): ?string
    {
        if ($declared === Undefined::Value) {
            return null;
        }

        $text = $this->conversion->text($declared, $place);

        if ($text === null) {
            $raised();

            return null;
        }

        $this->read -= strlen($text);

        return $text;
    }

    /**
     * Whether an entry configures its value (section 4.1): it owns at least
     * one of the reserved keys and nothing else. An array has its count at
     * hand; a `stdClass` is enumerated no further than one key past the
     * three reserved, so a large one costs no more to ask than a small one,
     * and asking it again costs no more than recording the answer would.
     *
     * @phpstan-assert-if-true array<array-key, mixed>|\stdClass $entry
     */
    private function isWrapper(mixed $entry): bool
    {
        if (is_array($entry)) {
            $count = count($entry);

            if ($count < 1 || $count > count(self::WRAPPED)) {
                return false;
            }
        } elseif (!$entry instanceof \stdClass || get_class($entry) !== \stdClass::class) {
            return false;
        }

        $count = 0;

        foreach ($entry as $key => $_) {
            $count += 1;

            if ($count > count(self::WRAPPED) || !in_array($key, self::WRAPPED, true)) {
                return false;
            }
        }

        return $count > 0;
    }
}
