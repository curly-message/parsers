<?php

declare(strict_types=1);

namespace CurlyMessage\Internal;

/**
 * Host code the parser runs, above the host code it runs beneath on the
 * same thread of execution, if any, and whether a text it asked for was
 * refused. The call to a host modifier runs as the frame the texts its
 * placeholder hands out name it by, and so does the conversion of what it
 * answered, which is the call's own code.
 *
 * @internal
 */
final class HostFrame
{
    public bool $refused = false;

    public ?HostFrame $below = null;

    /** The call to a host modifier whose answer this frame converts, if any. */
    public ?HostFrame $call = null;

    /** Fails this frame, and the call it runs as. */
    public function refuse(): void
    {
        $this->refused = true;

        if ($this->call !== null) {
            $this->call->refused = true;
        }
    }
}
