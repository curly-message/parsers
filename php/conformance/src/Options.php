<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/** How a run is made: over which fixtures, at which levels, and performing requests how. */
final readonly class Options
{
    /**
     * @param list<Fixture>|null $fixtures the fixture files to run, the pinned set where none
     * @param list<string>|null $levels the levels to run, every level the adapter claims where none
     * @param Performer|null $performer what performs a case's request where the adapter exposes none
     */
    public function __construct(
        public ?array $fixtures = null,
        public ?array $levels = null,
        public ?Performer $performer = null,
    ) {}
}
