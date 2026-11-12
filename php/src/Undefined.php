<?php

declare(strict_types=1);

namespace CurlyMessage;

/**
 * The host's undefined, which PHP does not have: a payload entry, a wrapper's
 * value or default, a layer property or a message holding it is one nobody
 * supplied. `null` stays the host's null, a value that is present.
 */
enum Undefined
{
    case Value;
}
