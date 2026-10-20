<?php

declare(strict_types=1);

namespace CurlyMessage\Tests\Unit;

/** Properties of every visibility, typed and uninitialized among them. */
final class Visible
{
    public string $public = 'public';

    public int $uninitialized;

    public ?Visible $nested = null;

    protected string $protected = 'protected';

    private string $private = 'private';

    public function hidden(): string
    {
        return $this->protected . $this->private;
    }
}
