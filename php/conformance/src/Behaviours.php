<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** The catalogue of the set's README.md, each behaviour a function of section 11's inputs. */
final class Behaviours
{
    public const NAMES = ['upper', 'echo', 'empty', 'nothing', 'raise', 'default', 'options', 'props', 'locale', 'object'];

    /** @return \Closure(BehaviourInput): mixed */
    public static function named(string $name): \Closure
    {
        return match ($name) {
            // ASCII letters only, as the catalogue states it.
            'upper' => static fn(BehaviourInput $in): string => strtoupper($in->value),
            'echo' => static fn(BehaviourInput $in): string => $in->value,
            'empty' => static fn(BehaviourInput $in): string => '',
            // PHP's nothing is null, as ECMAScript's is undefined.
            'nothing' => static fn(BehaviourInput $in): mixed => null,
            'raise' => static fn(BehaviourInput $in): never => throw new \RuntimeException('The raise behaviour raised.'),
            'default' => static fn(BehaviourInput $in): string => $in->default(),
            'options' => static fn(BehaviourInput $in): string => implode(',', array_map(
                static fn(BehaviourOption $option): string => "{$option->key}={$option->value()}",
                $in->options,
            )),
            'props' => static function (BehaviourInput $in): string {
                $props = $in->props;
                ksort($props, SORT_STRING);
                $written = [];

                foreach ($props as $name => $value) {
                    $written[] = $name . '=' . json_encode($value, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_THROW_ON_ERROR);
                }

                return implode(',', $written);
            },
            'locale' => static fn(BehaviourInput $in): string => $in->locale ?? 'none',
            'object' => static fn(BehaviourInput $in): array => ['answer' => $in->value],
            default => throw new \RuntimeException('The set names no behaviour ' . json_encode($name) . '.'),
        };
    }
}
