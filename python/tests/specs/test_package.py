"""What the wheel a release ships carries, and what a release keeps in step with it.

``tasks.py test`` names the wheel it built in ``CURLY_MESSAGE_WHEEL``; run any
other way, there is no wheel to read and the tests of the wheel skip.
"""

from __future__ import annotations

import json
import os
import tomllib
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


def test_wheel_depends_on_nothing_but_the_intl_extra(wheel: zipfile.ZipFile) -> None:
    metadata = Parser().parsestr(_metadata(wheel))
    assert metadata.get_all("Provides-Extra") == ["intl"]
    assert sorted(metadata.get_all("Requires-Dist") or []) == [
        "babel<3,>=2.14; extra == 'intl'",
        "tzdata>=2024.1; extra == 'intl'",
    ]
    assert metadata["Name"] == "curly-message"
    assert metadata["Requires-Python"] == ">=3.11"


def test_wheel_describes_the_package_with_the_readme(wheel: zipfile.ZipFile) -> None:
    # The registry's page for a release is the description its wheel carries.
    metadata = Parser().parsestr(_metadata(wheel))
    assert metadata["Description-Content-Type"] == "text/markdown"
    assert metadata.get_payload() == ROOT.joinpath("README.md").read_text(encoding="utf-8")


def test_the_benchmark_manifest_names_the_version_the_project_does() -> None:
    # bench/harness.mjs heads its table with package.json's version, so the
    # release writes the one version into both.
    project = tomllib.loads(ROOT.joinpath("pyproject.toml").read_text(encoding="utf-8"))
    manifest = json.loads(ROOT.joinpath("package.json").read_text(encoding="utf-8"))
    assert manifest == {
        "name": "curly-message-python",
        "version": project["project"]["version"],
        "private": True,
    }
