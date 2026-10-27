<?php

declare(strict_types=1);

namespace CurlyMessage;

/** The conditions a resolution reports (SPEC.md section 14.2), each with what it declares. */
enum ReportCode: string
{
    case UnknownModifier = 'unknown-modifier';
    case FailedModifier = 'failed-modifier';
    case MissingOptions = 'missing-options';
    case UnserializableValue = 'unserializable-value';
    case MissingLocale = 'missing-locale';
    case NestingLimit = 'nesting-limit';
    case OutputLimit = 'output-limit';
    case ReadLimit = 'read-limit';

    /**
     * What the condition is a defect of. It is the code's to declare, so no
     * site that reports one chooses it: a modifier that could not answer was
     * handed the caller's value, props and locale, or is the caller's own, so
     * what it reports is the payload's.
     */
    public function origin(): Origin
    {
        return match ($this) {
            self::UnknownModifier, self::MissingOptions, self::NestingLimit => Origin::Message,
            self::FailedModifier, self::UnserializableValue, self::MissingLocale => Origin::Payload,
            self::OutputLimit, self::ReadLimit => Origin::Limit,
        };
    }

    /** The limit the condition names, for the three that reach one. */
    public function limit(): ?int
    {
        return match ($this) {
            self::NestingLimit => Parser::MAX_NESTING,
            self::OutputLimit => Parser::MAX_OUTPUT_LENGTH,
            self::ReadLimit => Parser::MAX_READ_LENGTH,
            default => null,
        };
    }

    /** An English sentence saying what happened. */
    public function message(): string
    {
        return match ($this) {
            self::UnknownModifier => 'A placeholder named a modifier this parser does not know.',
            self::FailedModifier => 'A modifier could not produce a result, so the placeholder took its fallback chain.',
            self::MissingOptions => 'A selection was given no options to select from, so the placeholder took its fallback chain.',
            self::UnserializableValue => 'A value could not become text, so resolution read it as missing.',
            self::MissingLocale => 'A modifier that depends on a locale was given none, so the placeholder resolved to the empty string.',
            self::NestingLimit => 'A placeholder was nested deeper than ' . Parser::MAX_NESTING . ' levels, so it took its fallback chain.',
            self::OutputLimit => 'A placeholder resolved to text this resolution has no room for: in the message, past ' . Parser::MAX_OUTPUT_LENGTH . ' bytes of output, and below it, past ' . Parser::MAX_TEXT_LENGTH . ' bytes of the text of its level, so it resolved to the empty string.',
            self::ReadLimit => 'Resolution read more than ' . Parser::MAX_READ_LENGTH . ' bytes of value text, so this placeholder resolved to the empty string.',
        };
    }
}
