"""Fetch a release of the conformance set beside this file, refused unless its digest is stated.

    python tests/conformance/fetch.py 4.2.0 <the SHA-256 the release notes state>

The archive is written here as the release attached it, and nothing else is:
the suite reads it in place. A bump is that archive and the ``PIN`` line of
``runner/archive.py``, which this script prints rather than edits.
"""

from __future__ import annotations

import hashlib
import re
import shutil
import sys
import tempfile
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Final

RELEASES: Final = "https://github.com/curly-message/spec/releases/download"

HERE: Final = Path(__file__).resolve().parent

_VERSION: Final = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+")

_DIGEST: Final = re.compile(r"[0-9a-f]{64}")

_CHUNK: Final = 1 << 16


def address(version: str) -> str:
    """Where a release attaches its archive."""
    return f"{RELEASES}/conformance-v{version}/conformance-{version}.zip"


def fetch(version: str, digest: str, *, source: str | None = None, into: Path = HERE) -> Path:
    """Download the archive into a fresh directory, check it, and only then move it ``into``."""
    if not _VERSION.fullmatch(version):
        raise ValueError(f"{version!r} is not a release version.")
    if not _DIGEST.fullmatch(digest):
        raise ValueError(f"{digest!r} is not a SHA-256 written in lowercase hexadecimal.")
    url = source or address(version)
    if urllib.parse.urlsplit(url).scheme not in ("https", "file"):
        raise ValueError(f"{url!r} is neither an https nor a file address.")
    name = f"conformance-{version}.zip"
    with tempfile.TemporaryDirectory() as scratch:
        downloaded = Path(scratch) / name
        hashed = hashlib.sha256()
        with urllib.request.urlopen(url) as response, downloaded.open("wb") as file:  # noqa: S310
            while chunk := response.read(_CHUNK):
                hashed.update(chunk)
                file.write(chunk)
        if hashed.hexdigest() != digest:
            raise ValueError(f"{url} has the digest {hashed.hexdigest()}, not the stated {digest}.")
        return Path(shutil.move(downloaded, into / name))


def main(arguments: list[str]) -> int:
    if len(arguments) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    version, digest = arguments
    written = fetch(version, digest)
    print(f"Wrote {written}. Set PIN in runner/archive.py to:")
    print(f'PIN: Final = ("{version}", "{digest}")')
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
