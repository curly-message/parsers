<?php

declare(strict_types=1);

namespace CurlyMessage\Cst;

/** The kinds of node CST.md section 6 names, each spelled as it spells them. */
enum NodeType: string
{
    case Message = 'message';
    case Text = 'text';
    case Escape = 'escape';
    case Placeholder = 'placeholder';
    case Open = 'open';
    case Close = 'close';
    case Separator = 'separator';
    case Space = 'space';
    case Key = 'key';
    case Modifier = 'modifier';
    case OptionKey = 'option-key';
    case OptionValue = 'option-value';
}
