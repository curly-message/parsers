<?php

declare(strict_types=1);

namespace CurlyMessage;

/**
 * A condition one resolution met, handed to `onReport` where the walk meets
 * it. Everything it carries is the message's or the caller's: `text` is the
 * placeholder that reported, or the message that held it, never a payload
 * value.
 */
final readonly class Report
{
    /**
     * @param mixed $id the id the resolution was given, null where none
     * @param int|null $limit the documented limit, for the three codes that reach one; for `output-limit` below the message, `Parser::MAX_TEXT_LENGTH`
     * @param string $text the placeholder's spelling, cut to 120 bytes and with every line terminator escaped
     */
    public function __construct(
        public ReportCode $code,
        public Origin $origin,
        public string $message,
        public mixed $id,
        public ?int $limit,
        public string $text,
    ) {}
}
