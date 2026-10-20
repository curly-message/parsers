<?php

declare(strict_types=1);

namespace CurlyMessage;

/** What a reported condition is a defect of (SPEC.md section 14.2). */
enum Origin: string
{
    case Message = 'message';
    case Payload = 'payload';
    case Limit = 'limit';
}
