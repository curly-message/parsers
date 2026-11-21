<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

use CurlyMessage\Props;
use CurlyMessage\Undefined;

/**
 * Reads of what a caller hands over as data: a payload, a wrapper, a layer of
 * props. A container is a PHP array or a `stdClass`, whose entries are all its
 * own; any other object is none, because reading one means `offsetGet` or
 * `__get`, host code, and its declared properties are the class's rather than
 * the caller's. So `__proto__`, `constructor` and `toString` are ordinary keys.
 * One instance serves one resolution.
 *
 * @internal
 */
final class Layers
{
    /** @var \WeakMap<\stdClass, array<array-key, mixed>> the entries of each `stdClass` read by a name beginning with NUL, or as the props a wrapper holds under a modifier's name */
    private \WeakMap $enumerated;

    /**
     * What was read from a container and what was composed of a layer, held
     * until the resolution ends: host code may drop the caller's own
     * reference to an entry while the walk reads it, which leaves the walk
     * holding the last one. That holds of an array too, whose entry may be a
     * reference to a variable host code reassigns. An object is held once,
     * by its id, which no other object takes while it is held; an array has
     * no identity, and is held at each read, which the message bounds.
     *
     * @var array<int, object>
     */
    private array $objects = [];

    /** @var list<array<array-key, mixed>> */
    private array $arrays = [];

    /**
     * What `props` composed of the layers every placeholder shares, kept
     * by what was composed: the properties under a name, those of them a
     * list names, and what a placeholder with none of its own is handed.
     *
     * @var array<array-key, array<array-key, mixed>>
     */
    private array $composed = [];

    /** @var array<array-key, array<string, array<array-key, mixed>>> */
    private array $named = [];

    /** @var array<array-key, array<string, Props>> */
    private array $handed = [];

    /** @param list<mixed> $shared the layers of props every placeholder of the resolution shares, the lowest first */
    public function __construct(private readonly array $shared = [])
    {
        $this->enumerated = new \WeakMap();
    }

    public static function isContainer(mixed $value): bool
    {
        return is_array($value) || ($value instanceof \stdClass && get_class($value) === \stdClass::class);
    }

    /**
     * The container's entry under `$key`, or `Undefined::Value` where it has
     * none or is no container. A key is looked up as it is: an array finds
     * `null` under it (`isset` would not), and a `stdClass` is asked
     * `property_exists` and then read, which costs one lookup however many
     * entries it holds. A name beginning with NUL is one `property_exists`
     * cannot see, so it alone is looked for among every entry, which a
     * resolution enumerates once for each `stdClass`, as it first reads one
     * so: a message naming such a key again and again costs no more for each
     * entry the payload holds. Reading either
     * runs no host code: a `stdClass` declares no property to hook, and one
     * made lazy is initialized as it is made, its initializer never run.
     */
    public function own(mixed $container, string|int|null $key): mixed
    {
        if ($key === null) {
            return Undefined::Value;
        }

        if (is_array($container)) {
            return array_key_exists($key, $container) ? $this->held($container[$key]) : Undefined::Value;
        }

        if (!self::isContainer($container)) {
            return Undefined::Value;
        }

        /** @var \stdClass $container */
        $name = (string) $key;

        if (str_starts_with($name, "\0")) {
            $entries = $this->enumerated[$container] ??= get_object_vars($container);

            return array_key_exists($name, $entries) ? $this->held($entries[$name]) : Undefined::Value;
        }

        return property_exists($container, $name) ? $this->held($container->{$name}) : Undefined::Value;
    }

    /**
     * Every entry of a container, or none for anything else.
     *
     * @return array<array-key, mixed>
     */
    public static function entries(mixed $container): array
    {
        if (is_array($container)) {
            return $container;
        }

        /** @var \stdClass $container */
        return self::isContainer($container) ? get_object_vars($container) : [];
    }

    /**
     * The properties each layer holds under `$name`, composed in order: a
     * layer overrides only what it names, and a property it holds
     * `Undefined::Value` under it does not name. Where `$only` names the properties, those alone are read,
     * name by name, so nothing else a layer holds under `$name` is read.
     *
     * @param list<mixed> $layers
     * @param list<string>|null $only
     *
     * @return array<array-key, mixed>
     */
    public function compose(array $layers, string $name, ?array $only = null): array
    {
        $composed = [];

        foreach ($layers as $layer) {
            $slice = $this->own($layer, $name);

            if ($only !== null) {
                foreach ($only as $property) {
                    $value = $this->own($slice, $property);

                    if ($value !== Undefined::Value) {
                        $composed[$property] = $value;
                    }
                }

                continue;
            }

            foreach (self::entries($slice) as $property => $value) {
                if ($value !== Undefined::Value) {
                    $composed[$property] = $value;
                }
            }
        }

        return $this->held($composed);
    }

    /**
     * The properties a placeholder hands its modifier: those the layers
     * every placeholder shares hold under `$name`, and over them those the
     * wrapper of the placeholder's value holds, `$own` being that wrapper's
     * props. Where `$beneath` names another modifier and properties of it,
     * those lie beneath all of them, composed the same way.
     *
     * Composing walks every property a layer holds under the name, and how
     * many those are is the payload's to say, not the message's. So one
     * resolution composes the shared layers once for each name, and no
     * placeholder copies them: one whose wrapper holds nothing of its own
     * is handed the very properties every other such placeholder is, and
     * one whose wrapper does is handed that wrapper's own laid over the
     * shared ones, each read where it is. A wrapper's array is laid as it
     * is, and a `stdClass` is enumerated once in a resolution, however many
     * placeholders read it. What a placeholder costs is then what it
     * reads, and nothing kept grows with the placeholders or the keys of a
     * message times what a layer holds. What is composed is kept until
     * the resolution ends and no longer, so a layer host code changes
     * while the resolution is in progress is read as it was when a
     * placeholder first composed it.
     *
     * What is kept is kept by what was composed, the name and what lies
     * beneath, never by who asked: a modifier a host registers while the
     * resolution is in progress is handed what its own asking composes.
     *
     * @param array{string, list<string>}|null $beneath
     */
    public function props(mixed $own, string $name, ?array $beneath = null): Props
    {
        // Most placeholders hold no wrapper, and nothing lies beneath.
        if ($own === null && $beneath === null) {
            return $this->handed[$name][''] ??= new Props($this->lower($name, null, ''));
        }

        $slice = $this->slice($own, $name);
        $under = $beneath === null || $own === null ? [] : $this->compose([$own], $beneath[0], $beneath[1]);
        $variant = $beneath === null ? '' : $beneath[0] . "\0" . implode("\0", $beneath[1]);

        if ($slice === [] && $under === []) {
            return $this->handed[$name][$variant] ??= new Props($this->lower($name, $beneath, $variant));
        }

        $layers = $this->lower($name, $beneath, $variant);

        if ($under !== []) {
            array_splice($layers, $this->composed[$name] === [] ? 0 : 1, 0, [$under]);
        }

        if ($slice !== []) {
            // A wrapper's own may hold `Undefined::Value`, which the lowest
            // layer of what is handed over never does.
            array_unshift($layers, $slice, ...($layers === [] ? [[]] : []));
        }

        return new Props($layers);
    }

    /**
     * Lets go of what the resolution held, where nothing a `__destruct` runs
     * leaves it.
     */
    public function release(): void
    {
        if ($this->objects === [] && $this->arrays === [] && count($this->enumerated) === 0) {
            return;
        }

        $held = [$this->objects, $this->arrays, $this->enumerated, $this->composed, $this->named, $this->handed];
        $this->objects = [];
        $this->arrays = [];
        $this->composed = [];
        $this->named = [];
        $this->handed = [];
        $this->enumerated = new \WeakMap();
        Contained::release($held);
    }

    /**
     * What the shared layers hold under `$name`, and beneath it what they
     * hold of `$beneath`, each composed once.
     *
     * @param array{string, list<string>}|null $beneath
     *
     * @return list<array<array-key, mixed>>
     */
    private function lower(string $name, ?array $beneath, string $variant): array
    {
        $props = $this->composed[$name] ??= $this->compose($this->shared, $name);
        $lower = $props === [] ? [] : [$props];

        if ($beneath === null) {
            return $lower;
        }

        $named = $this->named[$beneath[0]][$variant] ??= $this->compose($this->shared, $beneath[0], $beneath[1]);

        return $named === [] ? $lower : [...$lower, $named];
    }

    /**
     * The entries a wrapper's props hold under `$name`, as they are: an
     * array is the caller's own, uncopied, and a `stdClass` is enumerated
     * once in a resolution.
     *
     * @return array<array-key, mixed>
     */
    private function slice(mixed $own, string $name): array
    {
        $slice = $this->own($own, $name);

        if (is_array($slice)) {
            return $slice;
        }

        if (!self::isContainer($slice)) {
            return [];
        }

        /** @var \stdClass $slice */
        return $this->enumerated[$slice] ??= get_object_vars($slice);
    }

    /**
     * @template T
     *
     * @param T $value
     *
     * @return T
     */
    private function held(mixed $value): mixed
    {
        if (is_object($value)) {
            $this->objects[spl_object_id($value)] ??= $value;
        } elseif (is_array($value) && $value !== []) {
            $this->arrays[] = $value;
        }

        return $value;
    }
}
