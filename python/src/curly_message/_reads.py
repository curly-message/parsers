"""How a read of an option's value or of a default renders, and what it counts (section 13)."""

from __future__ import annotations

import threading
from collections.abc import Generator
from typing import Any, Final

MAX_REENTRY: Final[int] = 16
"""The most resolutions in progress at once on one thread.

A read stands three frames on the interpreter's stack while it renders: the
modifier that made it, ``read`` and the step of the walk that called the
modifier. CPython 3.11 counts a fourth toward its limit, the call that resumes
the walk. A resolution in progress can stand under ``MAX_NESTING - 1`` reads a
modifier makes of what it was handed, and costs at most seven frames more than
a read itself, so sixteen keep the deepest chain the two bounds allow within
16 * (7 * 4 + 7) = 560 frames: under the 600 a default interpreter still has
where its caller stands 400 deep. Frames host code stands beneath itself are
that code's.
"""

Job = Generator["Job", "str | Text", "str | Text"]
"""A step of the walk that renders nested text: it yields the job it waits on and gets its text."""


class Text:
    """Rendered text held as the pieces it is made of, and made a ``str`` where it is read.

    A level that only passes a nested result on would otherwise copy it, so a
    message nested n levels deep would copy its innermost text n times.
    """

    __slots__ = ("length", "pieces")

    def __init__(self, pieces: list[str | Text], length: int) -> None:
        self.pieces = pieces
        self.length = length


def drive(top: Job) -> str | Text:
    """Run a job and every job it waits on, on a stack of suspended generators.

    However deep a message nests, the host's call stack holds this loop and
    the one generator it is resuming: a nested render is a job pushed here,
    never a call.
    """
    stack = [top]
    sent: Any = None
    while True:
        try:
            job = stack[-1].send(sent)
        except StopIteration as finished:
            stack.pop()
            if not stack:
                result: str | Text = finished.value
                return result
            sent = finished.value
            continue
        stack.append(job)
        sent = None


def text(found: str | Text) -> str:
    """The text as one ``str``, its pieces collected on a stack rather than by recursion."""
    if isinstance(found, str):
        return found
    pieces: list[str] = []
    stack = [iter(found.pieces)]
    while stack:
        piece = next(stack[-1], None)
        if piece is None:
            stack.pop()
        elif isinstance(piece, str):
            pieces.append(piece)
        else:
            stack.append(iter(piece.pieces))
    return "".join(pieces)


class Asked:
    """A host-defined modifier the walk is asking, which a refused read fails."""

    __slots__ = ("failed", "walk")

    host: Final = True

    def __init__(self, walk: Any) -> None:
        self.walk = walk
        self.failed = False


class Now:
    """What one thread has in progress.

    ``resolving`` counts the resolutions in progress there, and the reads that
    count as one. ``asked`` is the host-defined modifier being asked there, and
    nothing while the walk runs: under a modifier, the walk runs again only to
    render what host code asked it for.
    """

    resolving = 0
    asked: Asked | None = None


class _Thread(threading.local):
    # Made where a thread first resolves, by the code that asks: neither class
    # has an initializer, so reading this runs no frame of its own on a stack
    # that may have room for none.
    now: Now | None = None


THREAD: Final = _Thread()

_FINISHED: Final = threading.Condition()
_WAITING: Final = [0]


def _wait(holder: Any, now: Now) -> None:
    # Another thread is producing this text and runs while this one waits, so
    # the wait ends; one host code keeps from ending is that code's.
    with _FINISHED:
        _WAITING[0] += 1
        try:
            while holder._busy is not None and holder._busy is not now:
                _FINISHED.wait()
        finally:
            _WAITING[0] -= 1


def read(holder: Any) -> str:
    """What ``Option.value`` and ``ModifierInput.default`` answer, rendered on the first read.

    It is rendered on a trampoline of its own: a modifier is host code the
    walk called, so the render it asks for cannot be a job of the walk around
    it. The loop of ``drive`` is written out here, since a call would stand
    one more frame on the interpreter's stack for every read in progress.

    A modifier the format defines reads only what it was handed, which is the
    walk, one level deeper. So is a host-defined modifier reading what it was
    handed while it is the one being asked. Any other read renders at the
    depth of the placeholder that wrote it, whoever asks, so it counts as a
    resolution in progress on its thread while it renders; one past the bound
    renders nothing and fails the modifier being asked there. A read of what
    its own thread is already rendering is refused the same way at once, and
    one of what another thread is rendering waits for it.
    """
    found: str | None = holder._text
    if found is not None:
        return found
    call = holder._call
    if call is None:
        # Host code made this one, around a callable of its own.
        found = holder._produce()
        if found is None:
            return ""
        holder._text = found
        holder._produce = None
        return found
    now: Now | None = None
    asked: Asked | None = None
    counted = False
    if call.host:
        if call.walk.closed:
            return ""
        now = THREAD.now
        if now is None:
            now = THREAD.now = Now()
        asked = now.asked
        while True:
            busy = holder._busy
            if busy is None:
                break
            if busy is now:
                if asked is not None:
                    asked.failed = True
                return ""
            _wait(holder, now)
            found = holder._text
            if found is not None:
                return found
        if asked is not call:
            if now.resolving >= MAX_REENTRY:
                if asked is not None:
                    asked.failed = True
                return ""
            counted = True
            now.resolving += 1
        holder._busy = now
        now.asked = None
    elif call.closed:
        return ""
    try:
        result = holder._produce()
        if not isinstance(result, str):
            stack = [result]
            sent: Any = None
            while True:
                try:
                    job = stack[-1].send(sent)
                except StopIteration as finished:
                    stack.pop()
                    sent = finished.value
                    if not stack:
                        break
                    continue
                stack.append(job)
                sent = None
            result = text(sent)
    finally:
        if now is not None:
            now.asked = asked
            if counted:
                now.resolving -= 1
            holder._busy = None
            if _WAITING[0]:
                with _FINISHED:
                    _FINISHED.notify_all()
    holder._text = result
    holder._produce = holder._call = None
    return result
