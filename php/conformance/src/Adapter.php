<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

use CurlyMessage\Undefined;

/**
 * The implementation's side of the contract (SPEC.md section 14.3): the levels
 * it claims, the limits it documents, what its facilities cannot express, the
 * tree it offers, and the call that resolves one case's inputs.
 *
 * The claims are typed `mixed` on purpose: the runner holds them to the
 * format's vocabulary before anything runs, and a defect makes them wrong.
 *
 * `resolve` answers an array: `output`, the text; `reports`, a list of arrays
 * each with a `code` and, where the implementation carries them, an `origin`,
 * an `id` and a `limit`; `formats`, a list of the requests the resolution
 * made, each an `api`, its `options`, its `input` and, for a selection, its
 * `category`. A key left out is one the implementation does not observe.
 */
final readonly class Adapter
{
    /**
     * @param \Closure(Resolution): mixed $resolve
     * @param mixed $absent what an input the case leaves out, or tags `undefined`, is handed over as
     */
    public function __construct(
        public mixed $levels,
        public mixed $limits,
        public \Closure $resolve,
        public mixed $unexpressible = null,
        public ?CstOffer $cst = null,
        public mixed $absent = Undefined::Value,
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
    ): self {
        return new self(
            $levels === Keep::Claim ? $this->levels : $levels,
            $limits === Keep::Claim ? $this->limits : $limits,
            $resolve === Keep::Claim ? $this->resolve : $resolve,
            $unexpressible === Keep::Claim ? $this->unexpressible : $unexpressible,
            $cst === Keep::Claim ? $this->cst : $cst,
            $this->absent,
        );
    }

    /** What the adapter answers for one case's inputs. */
    public function resolve(Resolution $input): mixed
    {
        return ($this->resolve)($input);
    }
}
