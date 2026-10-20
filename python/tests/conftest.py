"""Holds the suite to the artifact it was asked to test.

The source layout keeps ``src/`` off the import path, so the package imported
here is the one installed: the editable project in ``.venv``, or the wheel in
``.venv-dist`` where ``CURLY_MESSAGE_DIST=1``. A suite that ran against the
other one would pass for the wrong artifact.
"""

from __future__ import annotations

import importlib.util
import os
import sysconfig
from pathlib import Path

import pytest

import curly_message

ROOT = Path(__file__).resolve().parents[1]

DIST = os.environ.get("CURLY_MESSAGE_DIST") == "1"


@pytest.fixture(scope="session")
def expect_intl() -> bool:
    """Whether this installation is meant to satisfy the Intl level.

    Each leg of ``tasks.py`` states it in ``CURLY_MESSAGE_EXPECT_INTL``, so a
    supplier that failed to install fails the leg rather than skipping its
    tests. Run any other way, the supplier's presence decides.
    """
    stated = os.environ.get("CURLY_MESSAGE_EXPECT_INTL")
    if stated is not None:
        return stated == "1"
    return importlib.util.find_spec("babel") is not None


def _expected() -> Path:
    if DIST:
        return Path(sysconfig.get_paths()["purelib"]).resolve()
    return (ROOT / "src").resolve()


def pytest_configure(config: pytest.Config) -> None:
    location = Path(curly_message.__file__).resolve()
    if not location.is_relative_to(_expected()):
        raise pytest.UsageError(f"curly_message was imported from {location}, not {_expected()}")


def pytest_report_header() -> str:
    return f"curly_message: {Path(curly_message.__file__).parent} ({'wheel' if DIST else 'source'})"
