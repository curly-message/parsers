"""The formatting requests a resolution makes, for a caller that asks to see them.

Private. The conformance set compares the request an Intl modifier makes
rather than the text the supplier renders for it (section 11.2), so its
adapter records them here. A request is not a diagnostic: diagnostics leave
through ``on_report`` alone, and this seam is no public channel.
"""

from __future__ import annotations

import contextlib
from collections.abc import Iterator
from contextvars import ContextVar
from types import MappingProxyType
from typing import Final

Request = dict[str, object]
"""``{"api", "options", "input"}``, and ``"category"`` for a selection the supplier answered."""

_RECORDER: Final[ContextVar[list[Request] | None]] = ContextVar(
    "curly_message_requests", default=None
)

UNEXPRESSIBLE: Final = MappingProxyType(
    {
        # The supplier carries no digits for any numbering system but Latin's.
        "NumberFormat": ("numberingSystem",),
        # The count is a NumberFormat rendering.
        "RelativeTimeFormat": ("numberingSystem",),
        # Gregorian only, Latin digits only, and no data to place a zone name
        # or fractional seconds in a pattern the options compose.
        "DateTimeFormat": (
            "calendar",
            "numberingSystem",
            "timeZoneName",
            "fractionalSecondDigits",
        ),
    }
)
"""The request properties the supplier cannot express, by facility (section 11.2).

A property listed here is not a failure: the request is formatted with the
rest of its properties.
"""


@contextlib.contextmanager
def record() -> Iterator[list[Request]]:
    """Collect, in order, every formatting request made while the block runs."""
    made: list[Request] = []
    token = _RECORDER.set(made)
    try:
        yield made
    finally:
        _RECORDER.reset(token)


def offer(api: str, options: dict[str, object], value: object) -> Request | None:
    """Record a request where a recorder is active, and answer the record to complete."""
    recorder = _RECORDER.get()
    if recorder is None:
        return None
    request: Request = {"api": api, "options": dict(options), "input": value}
    recorder.append(request)
    return request
