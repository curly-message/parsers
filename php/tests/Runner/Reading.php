<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Runner;

use CurlyMessage\Conformance\Adapter;
use CurlyMessage\Conformance\BehaviourInput;
use CurlyMessage\Conformance\BehaviourOption;
use CurlyMessage\Conformance\Resolution;

/**
 * A reading of section 13 small enough to hold whole in a test, over the
 * placeholders the constructions of kept reads write: a key, a modifier the
 * runner hands or, where it hands none of that name, the format's own `ne`,
 * and options. It counts, toward a bound, each read of a text that is no
 * current modifier's own, and each resolution where `$resolutions` says the
 * bound counts those as well; a modifier stays current through a resolution
 * it asks for. It takes no permission to count a read that is a modifier's
 * own. With
 * `$definedCurrent` false, a modifier the format defines is not current
 * while it runs, as no reading of section 13 has it, so the runner can be
 * held to telling that apart.
 */
final class Reading
{
    private int $places = 0;

    private int $resolving = 0;

    /** @var list<object> */
    private array $current = [];

    private function __construct(
        private readonly int $bound,
        private readonly bool $resolutions,
        private readonly bool $definedCurrent,
    ) {}

    public static function adapter(int $bound, bool $resolutions, bool $definedCurrent = true): Adapter
    {
        $reading = new self($bound, $resolutions, $definedCurrent);

        return new Adapter(['core', 'extensions'], ['output' => 100, 'read' => 100, 'conversion' => 100, 'nesting' => 8], $reading->resolve(...), absent: null, reentry: $bound, resolutions: $resolutions);
    }

    /** @return array{output: ?string} */
    private function resolve(Resolution $input): array
    {
        $counted = $this->resolutions && $this->resolving > 0;

        if ($counted && !$this->enter()) {
            return ['output' => null];
        }

        $this->resolving += 1;

        try {
            $payload = $input->payload instanceof \stdClass ? get_object_vars($input->payload) : [];
            $message = is_string($input->message) ? $input->message : '';

            return ['output' => $this->render($message, $payload, $input->modifiers)];
        } finally {
            $this->resolving -= 1;
            $this->places -= $counted ? 1 : 0;
        }
    }

    // Takes a place toward the bound, where one is left.
    private function enter(): bool
    {
        if ($this->places >= $this->bound) {
            return false;
        }

        $this->places += 1;

        return true;
    }

    /**
     * @param array<array-key, mixed> $payload
     * @param array<array-key, mixed> $modifiers
     */
    private function render(string $text, array $payload, array $modifiers): string
    {
        $out = '';
        $at = 0;

        while (($open = strpos($text, '{{', $at)) !== false) {
            $close = self::closing($text, $open);
            $out .= substr($text, $at, $open - $at) . $this->placeholder(substr($text, $open + 2, $close - $open - 2), $payload, $modifiers);
            $at = $close + 2;
        }

        return $out . substr($text, $at);
    }

    /**
     * @param array<array-key, mixed> $payload
     * @param array<array-key, mixed> $modifiers
     */
    private function placeholder(string $body, array $payload, array $modifiers): string
    {
        $parts = self::split($body);
        [$key, $modifier] = array_pad(explode(':', trim(array_shift($parts)), 2), 2, null);
        $options = [];
        $fallback = '';

        foreach ($parts as $part) {
            [$name, $text] = explode(':', trim($part), 2);

            if ($name === 'default') {
                $fallback = $text;
            } else {
                $options[] = [$name, $text];
            }
        }

        $value = $payload[$key] ?? null;
        $fallen = fn(): string => $this->render($fallback, $payload, $modifiers);

        if (!is_string($value)) {
            return $fallen();
        }

        if ($modifier === null) {
            return $value;
        }

        // A modifier the runner hands is looked up first, as the parser looks
        // up a host's, so a runner that registers `ne` is not read past.
        $behaviour = $modifiers[$modifier] ?? null;

        if ($modifier === 'ne' && !$behaviour instanceof \Closure) {
            $selected = null;

            foreach ($options as [$name, $option]) {
                if ($name !== $value) {
                    $selected = $option;

                    break;
                }
            }

            if ($selected === null) {
                return $fallen();
            }

            return $this->asked(new \stdClass(), $this->definedCurrent, fn(): string => $this->render($selected, $payload, $modifiers));
        }

        if (!$behaviour instanceof \Closure) {
            return $fallen();
        }

        $call = new \stdClass();
        // A text of this call, read once: free while the call is the current
        // modifier, and counted otherwise.
        $once = function (string $text) use ($call, $payload, $modifiers): \Closure {
            $state = null;

            return function () use (&$state, $text, $call, $payload, $modifiers): string {
                if (is_string($state)) {
                    return $state;
                }

                if (end($this->current) === $call) {
                    return $state = $this->render($text, $payload, $modifiers);
                }

                if (!$this->enter()) {
                    return '';
                }

                try {
                    return $state = $this->render($text, $payload, $modifiers);
                } finally {
                    $this->places -= 1;
                }
            };
        };
        $handed = [];

        foreach ($options as [$name, $option]) {
            $handed[] = new BehaviourOption($name, $once($option));
        }

        try {
            $answer = $this->asked($call, true, static fn(): mixed => $behaviour(new BehaviourInput($value, $handed, [], null, $once($fallback))));
        } catch (\RuntimeException) {
            return $fallen();
        }

        return is_string($answer) || $answer instanceof \Stringable ? (string) $answer : $fallen();
    }

    /**
     * What `$run` answers with `$call` the current modifier where `$current`
     * says it is one.
     *
     * @template T
     *
     * @param \Closure(): T $run
     *
     * @return T
     */
    private function asked(object $call, bool $current, \Closure $run): mixed
    {
        if (!$current) {
            return $run();
        }

        $this->current[] = $call;

        try {
            return $run();
        } finally {
            array_pop($this->current);
        }
    }

    // Where the placeholder opened at `$open` closes.
    private static function closing(string $text, int $open): int
    {
        $depth = 0;

        for ($at = $open; $at < strlen($text) - 1; $at += 1) {
            if (substr($text, $at, 2) === '{{') {
                $depth += 1;
                $at += 1;
            } elseif (substr($text, $at, 2) === '}}') {
                $depth -= 1;

                if ($depth === 0) {
                    return $at;
                }

                $at += 1;
            }
        }

        throw new \LogicException('An unclosed placeholder.');
    }

    /** @return non-empty-list<string> the body's parts between semicolons outside any placeholder */
    private static function split(string $body): array
    {
        $parts = [];
        $depth = 0;
        $start = 0;

        for ($at = 0; $at < strlen($body); $at += 1) {
            $pair = substr($body, $at, 2);

            if ($pair === '{{' || $pair === '}}') {
                $depth += $pair === '{{' ? 1 : -1;
                $at += 1;
            } elseif ($body[$at] === ';' && $depth === 0) {
                $parts[] = substr($body, $start, $at - $start);
                $start = $at + 1;
            }
        }

        $last = substr($body, $start);

        if (trim($last) !== '' || $parts === []) {
            $parts[] = $last;
        }

        return $parts;
    }
}
