<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

/**
 * One call of a host modifier, which the texts it is handed carry, and
 * whether a read was refused while it was the one being asked (SPEC.md
 * section 13).
 *
 * @internal
 */
final class Asked
{
    public bool $refused = false;
}
