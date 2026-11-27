"""``cst``: the tree a message is described by (CST.md)."""

from __future__ import annotations

from typing import Literal, cast

from ._types import (
    CstEscape,
    CstMessage,
    CstName,
    CstOptionValue,
    CstPlaceholder,
    CstPunctuation,
    CstSpace,
    CstText,
)
from ._utils import Scanner, cancels, separated, trimmed, unesc

_Contents = list[CstText | CstEscape | CstPlaceholder]

# The option values described but not yet filled, each with the range it is
# filled from. Nesting is not otherwise limited (section 6, note 10), so a value
# is filled off this stack rather than by a call per level.
_Pending = list[tuple[_Contents, int, int]]


def _spelled(message: str, from_: int, to: int) -> list[CstText | CstEscape]:
    """Text with every escape sequence in it marked.

    Section 7's rule is uniform over the message, so the same reading serves a
    message, a name and an option value. A backslash with nothing left to
    consume denotes itself and is text.
    """
    nodes: list[CstText | CstEscape] = []
    start = from_
    index = message.find("\\", from_, to)
    while 0 <= index < to - 1:
        if index > start:
            nodes.append({"type": "text", "start": start, "end": index})
        end = index + 2
        nodes.append(
            {"type": "escape", "start": index, "end": end, "cancels": cancels(message[index + 1])}
        )
        start = end
        index = message.find("\\", end, to)
    if to > start:
        nodes.append({"type": "text", "start": start, "end": to})
    return nodes


def _space(from_: int, to: int) -> CstSpace:
    return {"type": "space", "start": from_, "end": to}


def _separator(at: int) -> CstPunctuation:
    return {"type": "separator", "start": at, "end": at + 1}


_Parts = list[CstPunctuation | CstSpace | CstName | CstOptionValue]


def _named(
    message: str,
    kind: Literal["key", "modifier", "option-key"],
    from_: int,
    to: int,
    nodes: _Parts,
) -> None:
    """A name and the padding it is read without, which are nodes of their own."""
    start, end = trimmed(message, from_, to)
    if start > from_:
        nodes.append(_space(from_, start))
    nodes.append(
        {
            "type": kind,
            "start": start,
            "end": end,
            "name": unesc(message[start:end]),
            "nodes": _spelled(message, start, end),
        }
    )
    if to > end:
        nodes.append(_space(end, to))


def _valued(
    message: str,
    from_: int,
    to: int,
    nodes: _Parts,
    pending: _Pending,
) -> None:
    """An option value and its padding; what it holds is left on ``pending``."""
    start, end = trimmed(message, from_, to)
    contents: _Contents = []
    pending.append((contents, start, end))
    if start > from_:
        nodes.append(_space(from_, start))
    nodes.append({"type": "option-value", "start": start, "end": end, "nodes": contents})
    if to > end:
        nodes.append(_space(end, to))


def _placeholder(
    message: str, scan: Scanner, open_: int, close: int, pending: _Pending
) -> CstPlaceholder:
    nodes: _Parts = [{"type": "open", "start": open_, "end": open_ + 2}]
    # A `;` a nested placeholder encloses divides that placeholder and not this
    # one (section 6, note 5).
    declaration, *options = separated(message, open_ + 2, close - 2, ";", scan)
    # The selector's colon is the first one no escape claims; a later colon is
    # part of the modifier name (note 3).
    key, *modifier = separated(message, declaration[0], declaration[1], ":")
    _named(message, "key", key[0], key[1], nodes)
    if modifier:
        nodes.append(_separator(key[1]))
        _named(message, "modifier", key[1] + 1, declaration[1], nodes)
    for from_, to in options:
        # The `;` that opened this segment sits immediately before it.
        nodes.append(_separator(from_ - 1))
        option_key, *value = separated(message, from_, to, ":", scan)
        _named(message, "option-key", option_key[0], option_key[1], nodes)
        # A segment stating no value stands for its own key (section 9.4), so
        # there is no value to describe.
        if value:
            nodes.append(_separator(option_key[1]))
            _valued(message, option_key[1] + 1, to, nodes, pending)
    nodes.append({"type": "close", "start": close - 2, "end": close})
    return {"type": "placeholder", "start": open_, "end": close, "nodes": nodes}


def _contents(
    message: str, scan: Scanner, nodes: _Contents, from_: int, to: int, *, pending: _Pending
) -> None:
    """What a range of the message holds: its text, its escapes and its placeholders."""
    at = from_
    found = scan.next(at, to)
    while found is not None:
        open_, close = found
        nodes.extend(_spelled(message, at, open_))
        nodes.append(_placeholder(message, scan, open_, close, pending))
        at = close
        found = scan.next(at, to)
    nodes.extend(_spelled(message, at, to))


def cst(message: object) -> CstMessage:
    """Describe a message: its placeholders, what each is made of, and every escape.

    Spans count code points. The placeholders are the ones resolution finds,
    because the same scanner finds them; a construct that derives nothing is
    text. A message that is not a ``str`` has no characters to describe and
    carries no parts. Like the rest of the tree, the answer is plain dicts and
    lists, so ``json.dumps(cst(message))`` is the interchange encoding of
    CST.md, section 8.
    """
    if not issubclass(type(message), str):
        return {"type": "message", "start": 0, "end": 0, "nodes": []}
    # A subclass is read through `str`'s own storage, so no override runs.
    text = str.__str__(cast(str, message))
    scan = Scanner(text)
    nodes: _Contents = []
    pending: _Pending = [(nodes, 0, len(text))]
    while pending:
        into, from_, to = pending.pop()
        _contents(text, scan, into, from_, to, pending=pending)
    return {"type": "message", "start": 0, "end": len(text), "nodes": nodes}
