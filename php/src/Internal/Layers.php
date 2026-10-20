<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

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
    /** @var \WeakMap<\stdClass, array<array-key, mixed>> the entries of each `stdClass` read by a name beginning with NUL */
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

    public function __construct()
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
     * `Undefined::Value` under it does not name. What leaves is a fresh array,
     * so a modifier writing into it reaches neither the next placeholder nor
     * the caller.
     *
     * @param list<mixed> $layers
     *
     * @return array<array-key, mixed>
     */
    public function compose(array $layers, string $name): array
    {
        $composed = [];

        foreach ($layers as $layer) {
            foreach (self::entries($this->own($layer, $name)) as $property => $value) {
                if ($value !== Undefined::Value) {
                    $composed[$property] = $value;
                }
            }
        }

        return $this->held($composed);
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

        $held = [$this->objects, $this->arrays, $this->enumerated];
        $this->objects = [];
        $this->arrays = [];
        $this->enumerated = new \WeakMap();
        Contained::release($held);
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
