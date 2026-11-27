"""The adapter of section 14.3: ``curly_message_parser`` presented to the conformance runner."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any, cast

from runner import OMIT, Adapter, Call, Cst, Resolution, Resolved
from runner.model import Behaviour

import curly_message_parser
from curly_message_parser import (
    MAX_CONVERSION_NODES,
    MAX_NESTING,
    MAX_OUTPUT_LENGTH,
    MAX_READ_LENGTH,
    MAX_REENTRY,
    UNDEFINED,
    ModifierInput,
    Report,
    create_parser,
)
from curly_message_parser._requests import UNEXPRESSIBLE, record


def _modifier(behaviour: Behaviour) -> curly_message_parser.Modifier:
    # The options pass through as they are, so their values stay unread until
    # a behaviour reads them.
    def modifier(given: ModifierInput) -> object:
        return behaviour(
            Call(
                value=given.value,
                options=given.options,
                props=given.props,
                locale=given.locale,
                default=lambda: given.default,
            )
        )

    return modifier


def _report(report: Report) -> Mapping[str, object]:
    shown: dict[str, object] = {"code": report.code, "origin": report.origin}
    if report.message_id is not None:
        shown["id"] = report.message_id
    if report.limit is not None:
        shown["limit"] = report.limit
    return shown


def _resolve(resolution: Resolution) -> Resolved:
    reports: list[Report] = []
    modifiers = resolution.modifiers or {}
    parser = create_parser(
        custom_modifiers={name: _modifier(behaviour) for name, behaviour in modifiers.items()},
        modifier_defaults=None if resolution.defaults is OMIT else cast("Any", resolution.defaults),
        on_report=reports.append,
    )
    given: dict[str, Any] = {
        name: value
        for name, value in (
            ("props", resolution.props),
            ("locale", resolution.locale),
            ("message_id", resolution.id),
        )
        if value is not OMIT
    }
    payload = None if resolution.payload is OMIT else cast("Any", resolution.payload)
    message = UNDEFINED if resolution.message is OMIT else resolution.message
    with record() as requests:
        output = parser.resolve(message, payload, **given)
    return Resolved(
        output=output, reports=[_report(report) for report in reports], formats=list(requests)
    )


def _adapter() -> Adapter:
    claimed = curly_message_parser.levels()
    return Adapter(
        levels=tuple(sorted(claimed)),
        limits={
            "output": MAX_OUTPUT_LENGTH,
            "read": MAX_READ_LENGTH,
            "conversion": MAX_CONVERSION_NODES,
            "nesting": MAX_NESTING,
        },
        resolve=_resolve,
        unexpressible=cast("Mapping[str, tuple[str, ...]]", UNEXPRESSIBLE)
        if "intl" in claimed
        else None,
        cst=Cst(unit="code-point", parse=curly_message_parser.cst),
        undefined=UNDEFINED,
        # ``MAX_REENTRY`` bounds the resolutions in progress and the counted
        # reads together, and the resolution a read is made in holds one of
        # its places: the bound on counted reads alone is one less.
        reentry=MAX_REENTRY - 1,
        # A resolution asked for inside another takes a place on its thread
        # as a counted read does, and one past the bound raises
        # ``ReentryLimitError`` into the modifier that asked.
        resolutions=True,
    )


adapter = _adapter()
"""What this installation satisfies: the levels are read from the package, never recomputed."""
