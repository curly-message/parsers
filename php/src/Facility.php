<?php

declare(strict_types=1);

namespace CurlyMessage;

/**
 * The locale-dependent facility a request is made of (SPEC.md section 11.2),
 * named as ECMAScript's `Intl` names it, since a request is stated in that
 * vocabulary whatever performs it.
 */
enum Facility: string
{
    case NumberFormat = 'NumberFormat';
    case DateTimeFormat = 'DateTimeFormat';
    case RelativeTimeFormat = 'RelativeTimeFormat';
    case PluralRules = 'PluralRules';
}
