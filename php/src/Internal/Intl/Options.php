<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

use CurlyMessage\Props;

/**
 * What a modifier of the format asks its facility with: the props it was
 * handed, with what it pinned over them and without the names it dropped,
 * none of them copied.
 *
 * @internal
 */
final readonly class Options
{
    /**
     * @param array<array-key, mixed> $pinned what lies over every property
     * @param array<array-key, true> $dropped the names no property is read for
     */
    public function __construct(
        private Props $props,
        private array $pinned = [],
        private array $dropped = [],
    ) {}

    /**
     * Those of the `$names` options that are set, in the order of `$names`,
     * each looked for once by its name: what it costs is the names asked
     * for, however many properties there are.
     *
     * @param list<string> $names
     *
     * @return array<string, mixed>
     */
    public function only(array $names): array
    {
        $layered = $this->props->only($names);

        if ($this->pinned === [] && $this->dropped === []) {
            return $layered;
        }

        $found = [];

        foreach ($names as $name) {
            if (\array_key_exists($name, $this->pinned)) {
                $found[$name] = $this->pinned[$name];
            } elseif (\array_key_exists($name, $layered) && !isset($this->dropped[$name])) {
                $found[$name] = $layered[$name];
            }
        }

        return $found;
    }

    /**
     * Every option, as an array of the caller's own: the properties, each
     * where the lowest layer naming it put it, behind what was pinned over
     * them.
     *
     * @return array<array-key, mixed>
     */
    public function toArray(): array
    {
        $composed = $this->props->toArray();

        if ($this->dropped !== []) {
            $composed = array_diff_key($composed, $this->dropped);
        }

        return $this->pinned + $composed;
    }
}
