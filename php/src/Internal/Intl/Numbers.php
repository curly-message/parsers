<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

use CurlyMessage\Parser;

/**
 * The numbers one resolution's properties convert to, as `ToNumber` converts
 * them. A text, or a list of that one text, which joins to it, is converted
 * once however many placeholders read it, since converting reads its
 * padding, which no limit counts. The table is keyed by the texts the layers
 * hold already, one entry for each, so it holds no text of its own; any
 * other list joins to a text of its own, and is converted at each request.
 * The table goes with the resolution.
 *
 * @internal
 */
final class Numbers
{
    /** @var array<array-key, float> */
    private array $converted = [];

    public function of(mixed $value): float
    {
        $one = is_array($value) && array_is_list($value) && count($value) === 1 ? $value[0] : null;
        $text = is_string($one) && strlen($one) <= Parser::MAX_TEXT_LENGTH ? $one : $value;

        return is_string($text) ? $this->converted[$text] ??= EcmaOptions::toNumber($text) : EcmaOptions::toNumber($text);
    }
}
