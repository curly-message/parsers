<?php

declare(strict_types=1);

namespace CurlyMessage\Internal\Intl;

/**
 * One request a locale-dependent modifier made of its facility (SPEC.md
 * section 11.2), as an observer `Requests::observe` registered is handed it:
 * after the facility answered or refused it, in walk order. It carries the
 * number the request was made with and nothing else of the payload.
 *
 * @internal
 */
final readonly class Request
{
    /**
     * @param Api $api the facility asked
     * @param Options $options the options, in ECMAScript's vocabulary, as the layers composed them
     * @param int|float|array{int|float, string} $input the number, a timestamp in milliseconds for `DateTimeFormat`, or a count and its unit for `RelativeTimeFormat`
     * @param string|null $category the category a `PluralRules` request answered, null for every other request and for one refused
     */
    public function __construct(
        public Api $api,
        public Options $options,
        public int|float|array $input,
        public ?string $category = null,
    ) {}
}
