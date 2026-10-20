<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

use CurlyMessage\Undefined;

/**
 * The implementation's side of the contract (SPEC.md section 14.3): the levels
 * it claims, the limits it documents, what its facilities cannot express, the
 * tree it offers, the bound it keeps on counted reads, whether a modifier
 * can keep an input past its call and whether that bound counts a
 * resolution asked for inside another, and the call that resolves one
 * case's inputs.
 *
 * The claims are typed `mixed` on purpose: the runner holds them to the
 * format's vocabulary before anything runs, and a defect makes them wrong.
 *
 * `resolve` answers an array: `output`, the text; `reports`, a list of arrays
 * each with a `code` and, where the implementation carries them, an `origin`,
 * an `id` and a `limit`; `formats`, a list of the requests the resolution
 * made, each an `api`, its `options`, its `input` and, for a selection, its
 * `category`. A key left out is one the implementation does not observe.
 * `resolve` is re-entrant: a `nest`, `keep-nest`, `keep-nest-ne`, `descend` or `renest` behaviour calls
 * it again while a call to it is in progress, and each call answers its own
 * resolution.
 */
final readonly class Adapter
{
    /**
     * @param \Closure(Resolution): mixed $resolve
     * @param mixed $absent what an input the case leaves out, or tags `undefined`, is handed over as
     * @param mixed $reentry the bound on counted reads of section 13, the resolution asked for not counted; null states none
     * @param mixed $keeps false where no modifier can keep an input past its call; null states nothing, which reads as true
     * @param mixed $resolutions true where the bound `$reentry` states counts a resolution asked for inside another as it counts a read; null or false states nothing. The two cases built around a resolution a modifier asks for stand at the bound by resolutions where it is stated, and then run whatever `$keeps` states; they stand by kept reads otherwise
     */
    public function __construct(
        public mixed $levels,
        public mixed $limits,
        public \Closure $resolve,
        public mixed $unexpressible = null,
        public ?CstOffer $cst = null,
        public mixed $absent = Undefined::Value,
        public mixed $reentry = null,
        public mixed $keeps = null,
        public mixed $resolutions = null,
    ) {}

    /**
     * A copy with the named claims changed: what a defect of the claims is.
     *
     * @param (\Closure(Resolution): mixed)|Keep $resolve
     */
    public function with(
        mixed $levels = Keep::Claim,
        mixed $limits = Keep::Claim,
        \Closure|Keep $resolve = Keep::Claim,
        mixed $unexpressible = Keep::Claim,
        CstOffer|Keep|null $cst = Keep::Claim,
        mixed $reentry = Keep::Claim,
        mixed $keeps = Keep::Claim,
        mixed $resolutions = Keep::Claim,
    ): self {
        return new self(
            $levels === Keep::Claim ? $this->levels : $levels,
            $limits === Keep::Claim ? $this->limits : $limits,
            $resolve === Keep::Claim ? $this->resolve : $resolve,
            $unexpressible === Keep::Claim ? $this->unexpressible : $unexpressible,
            $cst === Keep::Claim ? $this->cst : $cst,
            $this->absent,
            $reentry === Keep::Claim ? $this->reentry : $reentry,
            $keeps === Keep::Claim ? $this->keeps : $keeps,
            $resolutions === Keep::Claim ? $this->resolutions : $resolutions,
        );
    }

    /** What the adapter answers for one case's inputs. */
    public function resolve(Resolution $input): mixed
    {
        return ($this->resolve)($input);
    }
}
