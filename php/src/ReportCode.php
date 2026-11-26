<?php

declare(strict_types=1);

namespace CurlyMessage;

/** The conditions a resolution reports (SPEC.md section 14.2). */
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
}
