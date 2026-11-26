<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * The locale-dependent facility a request is made of (SPEC.md section 11.2),
 * named as ECMAScript's `Intl` names it, since a request is stated in that
 * vocabulary whatever performs it.
 *
 * @internal
 */
enum Api: string
{
    case NumberFormat = 'NumberFormat';
    case DateTimeFormat = 'DateTimeFormat';
    case RelativeTimeFormat = 'RelativeTimeFormat';
    case PluralRules = 'PluralRules';
}
