<?php

declare(strict_types=1);

namespace CurlyMessage\Conformance;

/**
 * What a request produces on this host (RUNNER.md, "A formatting request"):
 * the text a formatting request makes, or the category a selection request
 * answers. The runner needs one only for an adapter that exposes no requests.
 */
interface Performer
{
    /**
     * @param array{api: string, options: \stdClass, input: mixed} $request the request as the case states it
     */
    public function perform(array $request, ?string $locale): string;
}
