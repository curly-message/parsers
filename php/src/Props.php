<?php

declare(strict_types=1);

namespace CurlyMessage;

/**
 * The properties a modifier is handed (SPEC.md section 11): those the
 * layers hold under its name, composed, a layer overriding only what it
 * names. It is read as an array is, by name, by `isset`, by `foreach` and by
 * `count`, and cannot be written: one resolution composes the layers every
 * placeholder shares once for each name, and every placeholder naming that
 * modifier is handed the same properties, so a write would reach them all.
 * Writing into it throws, which fails the modifier that tried as any raise
 * of its own does.
 *
 * The properties a wrapper holds lie over the shared ones without either
 * being copied, so reading one by name costs the same however many there
 * are. `toArray`, `foreach` and `count` walk them all, and what that costs
 * is theirs who ask.
 *
 * A property's name is a string or an integer, as an array's key is, and an
 * integer written as a string names the same property. Anything else names
 * none.
 *
 * @implements \ArrayAccess<array-key, mixed>
 * @implements \IteratorAggregate<array-key, mixed>
 */
final class Props implements \ArrayAccess, \IteratorAggregate, \Countable
{
    /**
     * @internal the parser builds one; host code reads it
     *
     * @param list<array<array-key, mixed>> $layers the uppermost first, each holding `Undefined::Value` under a name it does not name; the lowest holds none
     * @param array<array-key, mixed> $pinned what lies over every layer
     * @param array<array-key, true> $dropped the names no layer is read for
     */
    public function __construct(
        private readonly array $layers = [],
        private readonly array $pinned = [],
        private readonly array $dropped = [],
    ) {}

    /** Whether a layer names the property, whatever it holds under it, `null` included. */
    public function has(mixed $name): bool
    {
        return (is_string($name) || is_int($name)) && $this->find($name) !== Undefined::Value;
    }

    /** The property, or `$default` where no layer names it. */
    public function get(mixed $name, mixed $default = null): mixed
    {
        $found = is_string($name) || is_int($name) ? $this->find($name) : Undefined::Value;

        return $found === Undefined::Value ? $default : $found;
    }

    /**
     * Every property, as an array of the caller's own: those of the lowest
     * layer first, each where the lowest layer naming it put it, behind
     * what a modifier of the format pinned over them.
     *
     * @return array<array-key, mixed>
     */
    public function toArray(): array
    {
        $composed = $this->layers === [] ? [] : $this->layers[count($this->layers) - 1];

        for ($at = count($this->layers) - 2; $at >= 0; $at -= 1) {
            foreach ($this->layers[$at] as $name => $value) {
                if ($value !== Undefined::Value) {
                    $composed[$name] = $value;
                }
            }
        }

        if ($this->dropped !== []) {
            $composed = array_diff_key($composed, $this->dropped);
        }

        return $this->pinned + $composed;
    }

    /**
     * These properties with `$pinned` over them and without those `$dropped`
     * names, neither copied: what a modifier of the format asks its facility
     * with.
     *
     * @internal
     *
     * @param array<array-key, mixed> $pinned
     * @param list<string> $dropped
     */
    public function with(array $pinned, array $dropped = []): self
    {
        if ($dropped === []) {
            return new self($this->layers, $this->pinned === [] ? $pinned : $pinned + $this->pinned, $this->dropped);
        }

        $names = array_fill_keys($dropped, true);

        return new self($this->layers, $pinned + array_diff_key($this->pinned, $names), $this->dropped + $names);
    }

    /**
     * Those of the `$names` properties a layer names, in the order of
     * `$names`, each looked for once: what it costs is the names asked
     * for, however many properties there are.
     *
     * @internal
     *
     * @param list<string> $names
     *
     * @return array<string, mixed>
     */
    public function only(array $names): array
    {
        $found = [];
        $pinned = $this->pinned;
        $dropped = $this->dropped;
        $layers = $this->layers;

        // Most placeholders are handed no property but one a modifier of
        // the format pinned.
        if ($layers === [] && count($pinned) < 2) {
            $name = array_key_first($pinned);

            return is_string($name) && in_array($name, $names, true) ? [$name => $pinned[$name]] : [];
        }

        foreach ($names as $name) {
            if ($pinned !== [] && \array_key_exists($name, $pinned)) {
                $found[$name] = $pinned[$name];

                continue;
            }

            if ($layers === [] || ($dropped !== [] && isset($dropped[$name]))) {
                continue;
            }

            foreach ($layers as $layer) {
                if (\array_key_exists($name, $layer) && $layer[$name] !== Undefined::Value) {
                    $found[$name] = $layer[$name];

                    break;
                }
            }
        }

        return $found;
    }

    /** Whether the property is named and holds anything but `null`, as `isset` asks of an array. */
    public function offsetExists(mixed $offset): bool
    {
        return $this->get($offset) !== null;
    }

    /** The property, or `null` where no layer names it. */
    public function offsetGet(mixed $offset): mixed
    {
        return $this->get($offset);
    }

    /** @throws \LogicException always */
    public function offsetSet(mixed $offset, mixed $value): never
    {
        throw new \LogicException('The properties a modifier is handed cannot be written.');
    }

    /** @throws \LogicException always */
    public function offsetUnset(mixed $offset): never
    {
        throw new \LogicException('The properties a modifier is handed cannot be written.');
    }

    /** @return \ArrayIterator<array-key, mixed> */
    public function getIterator(): \ArrayIterator
    {
        return new \ArrayIterator($this->toArray());
    }

    public function count(): int
    {
        return count($this->layers) === 1 && $this->pinned === [] && $this->dropped === [] ? count($this->layers[0]) : count($this->toArray());
    }

    private function find(string|int $name): mixed
    {
        if ($this->pinned !== [] && \array_key_exists($name, $this->pinned)) {
            return $this->pinned[$name];
        }

        if ($this->dropped !== [] && isset($this->dropped[$name])) {
            return Undefined::Value;
        }

        foreach ($this->layers as $layer) {
            if (\array_key_exists($name, $layer) && $layer[$name] !== Undefined::Value) {
                return $layer[$name];
            }
        }

        return Undefined::Value;
    }
}
