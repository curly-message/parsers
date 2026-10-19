"""What the wheel a release ships carries.

``tasks.py test`` names the wheel it built in ``CURLY_MESSAGE_WHEEL``; run any
other way, there is no wheel to read and these tests skip.
"""

from __future__ import annotations

import os
import zipfile
from email.parser import Parser
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[2]

PACKAGE = ROOT / "src" / "curly_message"


@pytest.fixture(scope="module")
def wheel() -> zipfile.ZipFile:
    location = os.environ.get("CURLY_MESSAGE_WHEEL")
    if not location:
        pytest.skip("no wheel was built for this run")
    return zipfile.ZipFile(location)


def _metadata(wheel: zipfile.ZipFile) -> str:
    (name,) = [name for name in wheel.namelist() if name.endswith(".dist-info/METADATA")]
    return wheel.read(name).decode()


def test_wheel_carries_every_module_and_the_marker(wheel: zipfile.ZipFile) -> None:
    names = set(wheel.namelist())
    modules = {
        f"curly_message/{path.relative_to(PACKAGE).as_posix()}" for path in PACKAGE.rglob("*.py")
    }
    assert "curly_message/py.typed" in names
    assert modules <= names
    shipped = {name for name in names if name.startswith("curly_message/")}
    assert shipped == modules | {"curly_message/py.typed"}


def test_wheel_carries_no_tests(wheel: zipfile.ZipFile) -> None:
    assert not [name for name in wheel.namelist() if name.startswith("tests/")]


def test_wheel_depends_on_nothing(wheel: zipfile.ZipFile) -> None:
    metadata = Parser().parsestr(_metadata(wheel))
    assert metadata.get_all("Requires-Dist") is None
    assert metadata["Name"] == "curly-message"
    assert metadata["Requires-Python"] == ">=3.11"
