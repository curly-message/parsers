<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

/**
 * What a placeholder declares: the payload key it names, the modifier it
 * names, the options it carries and the default it states inline. Every name
 * arrives unescaped, because a name answers to itself rather than to the
 * spelling a message needed to write it. Every value arrives unread, as a
 * span: a value is message text, and reading it is work the modifier asks for
 * (SPEC.md section 9.4).
 *
 * @internal
 */
final class Placeholder
{
    /**
     * @param list<array{string, array{int, int}}> $options each key, padding dropped and unescaped, and where its value is written
     * @param array{int, int}|null $inlineDefault
     */
    private function __construct(
        public readonly ?string $key,
        public readonly string $modifier,
        public readonly array $options,
        public readonly ?array $inlineDefault,
    ) {}

    public static function parse(string $message, int $open, int $close, Scanner $scanner): self
    {
        $segments = Text::separated($message, $open + 2, $close - 2, ';', $scanner);
        $declaration = array_shift($segments);
        // The selector's colon is the first one no escape claims, and
        // everything after it is the modifier name (section 6, note 3).
        $declaredKey = Text::separated($message, $declaration[0], $declaration[1], ':')[0];
        [$keyEmpty, $key] = Text::named($message, $declaredKey[0], $declaredKey[1]);
        [, $modifier] = Text::named($message, min($declaredKey[1] + 1, $declaration[1]), $declaration[1]);

        $options = [];
        $inlineDefault = null;

        foreach ($segments as [$from, $to]) {
            $declaredOptionKey = Text::separated($message, $from, $to, ':', $scanner)[0];
            [, $optionKey] = Text::named($message, $declaredOptionKey[0], $declaredOptionKey[1]);
            // The first colon is the separator and every later one is value.
            // An option that names no value stands for itself; one that ends
            // at its colon declares the empty string.
            $declaredValue = $declaredOptionKey[1] < $to ? [$declaredOptionKey[1] + 1, $to] : $declaredOptionKey;
            $value = Text::trimmed($message, $declaredValue[0], $declaredValue[1]);

            if ($optionKey === '') {
                continue;
            }

            // `default` is reserved in lowercase, so a segment is either the
            // inline default or an option.
            if ($optionKey === 'default') {
                $inlineDefault ??= $value;

                continue;
            }

            $options[] = [$optionKey, $value];
        }

        return new self($keyEmpty ? null : $key, $modifier, $options, $inlineDefault);
    }
}
