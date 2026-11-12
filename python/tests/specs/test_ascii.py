"""Every source file is ASCII: a character outside it is written as an escape."""

from __future__ import annotations

from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[2]

SOURCES = sorted(
    [
        *ROOT.joinpath("src").rglob("*.py"),
        *ROOT.joinpath("tests").rglob("*.py"),
        *ROOT.joinpath("bench").rglob("*.py"),
        *ROOT.joinpath("bench").rglob("*.mjs"),
        ROOT / "tasks.py",
    ]
)


@pytest.mark.parametrize("path", SOURCES, ids=lambda path: str(path.relative_to(ROOT)))
def test_source_is_ascii(path: Path) -> None:
    data = path.read_bytes()
    outside = [(index, byte) for index, byte in enumerate(data) if byte > 0x7F]
    assert not outside, f"{path.name} holds non-ASCII bytes at {outside[:5]}"


def test_sources_are_found() -> None:
    assert ROOT.joinpath("src", "curly_message", "__init__.py") in SOURCES
    assert ROOT.joinpath("tasks.py") in SOURCES
    assert ROOT.joinpath("bench", "rows.mjs") in SOURCES
