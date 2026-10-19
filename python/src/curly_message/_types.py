"""The public types and constants, with the documentation a caller reads."""

from __future__ import annotations

import enum
from typing import Final


class Undefined(enum.Enum):
    """The host's undefined (specification, section 3).

    ``None`` is the host's null: it is a value, present, and it converts to
    ``"None"``. A value nobody supplied is spelled by leaving the entry out, or
    by ``UNDEFINED``, which means the same: an entry holding it is not named, a
    wrapper whose ``value`` is ``UNDEFINED`` has no value, and a modifier
    answering ``UNDEFINED`` answered nothing.
    """

    UNDEFINED = "UNDEFINED"

    def __repr__(self) -> str:
        return "UNDEFINED"

    def __bool__(self) -> bool:
        return False


UNDEFINED: Final = Undefined.UNDEFINED
"""The one member of :class:`Undefined`."""

MAX_OUTPUT_LENGTH: Final[int] = 100000
"""The most code points a resolution writes from placeholders (section 13)."""

MAX_READ_LENGTH: Final[int] = 100000
"""The most code points of value text a resolution reads (section 13)."""

MAX_CONVERSION_NODES: Final[int] = 100000
"""The most nodes one conversion of a structured value visits (section 13)."""

MAX_NESTING: Final[int] = 8
"""The deepest level at which a placeholder resolves (section 13)."""
