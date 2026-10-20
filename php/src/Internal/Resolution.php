<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

use CurlyMessage\ModifierInput;
use CurlyMessage\Option;
use CurlyMessage\Parser;
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

    /**
     * @param mixed $id the id reports carry, null where none
     */
    public function __construct(
        private readonly string $message,
        private readonly mixed $payload,
        private readonly mixed $props,
        private readonly ?string $locale,
        private readonly mixed $id,
        private readonly mixed $modifierDefaults,
        private readonly ?\Closure $onReport,
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
            $this->layers->release();
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

        if ($onReport === null) {
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
        $key = $declared->placeholder->key;
        $own = $declared->wrapper === null ? null : $this->payloadText($this->layers->own($declared->wrapper, 'default'), 'wd:' . $key, $declared->raised);

        if ($own !== null) {
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

        if ($declared === null) {
            return '';
        }

        return $this->resolved($declared, $open, $close, $depth);
    }

    /** A placeholder within the nesting limit, once it has read its value. */
    private function resolved(Declared $declared, int $open, int $close, int $depth): string
    {
        $placeholder = $declared->placeholder;
        $resolvedDefault = null;
        // The wrapper speaks for its own value, the payload for every key it
        // does not carry, and the message only for what neither says. A link
        // waits for the one before it to come back empty, and the chain waits
        // for a reader.
        $default = function () use (&$resolvedDefault, $declared, $placeholder, $depth): string {
            return $resolvedDefault ??= $this->payloadDefault($declared)
                ?? ($placeholder->inlineDefault === null ? '' : $this->render($placeholder->inlineDefault[0], $placeholder->inlineDefault[1], $depth + 1));
        };

        $modifier = $placeholder->modifier;

        // A modifier nobody registered is a defect in the message, not a
        // selection: running `eq` in its place would answer a question the
        // message never asked.
        if ($modifier !== '' && !in_array($modifier, Comparisons::NAMES, true)) {
            $this->report(ReportCode::UnknownModifier, $open, $close);

            return $default();
        }

        // A selection declaring no options was asked to select from nothing,
        // which is how the placeholder is written, so it is reported before
        // the value is asked anything. One naming no key compares nothing and
        // is no selection.
        if ($placeholder->key !== null && $placeholder->options === [] && in_array($modifier, self::SELECTIONS, true)) {
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

        $name = $modifier === '' ? 'eq' : $modifier;
        $options = [];

        foreach ($placeholder->options as [$optionKey, [$from, $to]]) {
            $options[] = $this->option($optionKey, $from, $to, $depth + 1);
        }

        $props = $this->layers->compose([$this->modifierDefaults, $this->props, $declared->wrapper === null ? null : $this->layers->own($declared->wrapper, 'props')], $name);

        return Comparisons::call($name, new ModifierInput($declared->valueText, $options, $props, $this->locale, $default));
    }

    /**
     * An option whose value is rendered where it is first asked for and not
     * again; one the modifier passes over is never rendered at all. Each is
     * built in a scope of its own, so no two share what they recorded.
     */
    private function option(string $key, int $from, int $to, int $depth): Option
    {
        $rendered = null;

        return new Option($key, function () use (&$rendered, $from, $to, $depth): string {
            return $rendered ??= $this->render($from, $to, $depth);
        });
    }

    /**
     * Value text is what a placeholder takes from the payload, and every byte
     * of it is spent whether or not any of it reaches the output.
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
