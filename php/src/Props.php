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
 * In the properties the parser hands over, those a wrapper holds lie over
 * the shared ones without either being copied, and properties a host builds
 * copy each layer once, as they are built; either way reading one by name
 * costs the same however many there are. `toArray` and `count` walk every
 * layer above the lowest, `foreach` walks them all, and what that costs is
 * theirs who ask.
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
    /** @var list<array<array-key, mixed>> the uppermost first; the lowest holds no `Undefined::Value` */
    private array $layers;

    /**
     * Properties composed of `$layers`, the uppermost first, each overriding
     * what lies beneath it only where it names a property: `Undefined::Value`
     * under a name is no property, so the layer beneath decides it. The
     * layers are read in the order they are handed, whatever their keys,
     * and anything but an array among them is no layer. Each is copied once,
     * by value, so the props hold what a reference in it held then.
     *
     * @param array<mixed> $layers
     */
    public function __construct(array $layers = [])
    {
        $kept = [];

        foreach ($layers as $layer) {
            if (is_array($layer)) {
                $copied = [];

                foreach ($layer as $name => $value) {
                    if ($value !== Undefined::Value) {
                        $copied[$name] = $value;
                    }
                }

                $kept[] = $copied;
            }
        }

        $this->layers = $kept;
    }

    /**
     * Properties composed of layers the parser composed, whose lowest holds
     * no `Undefined::Value`, taken as they are.
     *
     * @internal the parser builds one; host code reads it
     *
     * @param list<array<array-key, mixed>> $layers
     */
    public static function composed(array $layers): self
    {
        $props = new self();
        $props->layers = $layers;

        return $props;
    }

    /**
     * Those of the `$names` properties a layer names, in the order of
     * `$names`, each looked for once: what it costs is the names asked
     * for, however many properties there are.
     *
     * @internal a modifier of the format reads its facility's options with it
     *
     * @param list<string> $names
     *
     * @return array<string, mixed>
     */
    public function only(array $names): array
    {
        $found = [];

        // Most placeholders are handed no property.
        if ($this->layers === []) {
            return $found;
        }

        foreach ($names as $name) {
            foreach ($this->layers as $layer) {
                if (\array_key_exists($name, $layer) && $layer[$name] !== Undefined::Value) {
                    $found[$name] = $layer[$name];

                    break;
                }
            }
        }

        return $found;
    }

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
     * layer first, each where the lowest layer naming it put it.
     *
     * @return array<array-key, mixed>
     */
    public function toArray(): array
    {
        $at = count($this->layers) - 1;
        $composed = $this->layers[$at] ?? [];

        for ($at -= 1; $at >= 0; $at -= 1) {
            foreach ($this->layers[$at] as $name => $value) {
                if ($value !== Undefined::Value) {
                    $composed[$name] = $value;
                }
            }
        }

        return $composed;
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
        return count($this->layers) === 1 ? count($this->layers[0]) : count($this->toArray());
    }

    private function find(string|int $name): mixed
    {
        foreach ($this->layers as $layer) {
            if (\array_key_exists($name, $layer) && $layer[$name] !== Undefined::Value) {
                return $layer[$name];
            }
        }

        return Undefined::Value;
    }
}
