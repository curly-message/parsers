"""What the walk costs, asserted by count and never by time."""

from __future__ import annotations

from curly_message._utils import Scanner


def test_the_scanner_settles_each_opening_brace_once() -> None:
    message = "{{a; c:" * 1000
    scan = Scanner(message)
    assert scan.next(0, len(message)) is None
    assert len(scan.verdict) <= message.count("{")
    assert all(verdict == -1 for verdict in scan.verdict.values())


def test_a_settled_verdict_is_answered_from_the_record() -> None:
    message = "{{a; b:" * 50 + "x" + "}}" * 50
    scan = Scanner(message)
    assert scan.end(0) == len(message)
    settled = dict(scan.verdict)
    assert len(settled) == 50
    for open_, close in settled.items():
        assert scan.end(open_) == close
    assert scan.verdict == settled
