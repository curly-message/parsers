"""The conformance set this suite runs: the release archive, checked before it is read.

The archive is vendored beside this package as the release attached it, and
its digest is the one the release notes state. It is read in place, member by
member, and never extracted, so nothing it holds lands where an interpreter
would import it. Each fixture file is validated against the set's own schema
before a case of it is read, as the set's README asks of a runner in another
language.
"""

from __future__ import annotations

import functools
import hashlib
import io
import json
import zipfile
from collections.abc import Mapping
from pathlib import Path
from typing import Final, cast

from jsonschema import Draft202012Validator

from .model import Fixture, SetError

PIN: Final = ("4.1.0", "9011c603dd2a87f563557c60f32a8216f99cb1935dfcf54199f309e31496fbfd")
"""The release this suite runs, and the SHA-256 its release notes state for the archive."""

FORMAT: Final = "curly-message-3"
"""The versioned identifier of the format this runner reads."""

ARCHIVE: Final = Path(__file__).resolve().parents[1] / f"conformance-{PIN[0]}.zip"


@functools.cache
def _members(path: Path = ARCHIVE, pin: tuple[str, str] = PIN) -> Mapping[str, bytes]:
    data = path.read_bytes()
    digest = hashlib.sha256(data).hexdigest()
    if digest != pin[1]:
        raise SetError(f"{path.name} has the digest {digest}, not the pinned {pin[1]}.")
    prefix = f"conformance-{pin[0]}/"
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        return {
            info.filename[len(prefix) :]: archive.read(info)
            for info in archive.infolist()
            if info.filename.startswith(prefix) and not info.is_dir()
        }


def member(name: str) -> object:
    """A JSON member of the pinned archive, by its path under the set's directory."""
    members = _members()
    if name not in members:
        raise SetError(f"The archive holds no {name}.")
    return json.loads(members[name])


@functools.cache
def _validator(schema: str) -> Draft202012Validator:
    document = cast("Mapping[str, object]", member(f"schema/{schema}"))
    Draft202012Validator.check_schema(document)
    return Draft202012Validator(document)


def validated(name: str, file: object, schema: str = "fixture.schema.json") -> Mapping[str, object]:
    """A file of the set, refused where its format is not this runner's or it breaks its schema."""
    if not isinstance(file, dict):
        raise SetError(f"The file {name} is not a JSON object.")
    if file.get("format") != FORMAT:
        raise SetError(
            f"The file {name} targets the format {json.dumps(file.get('format'))}; "
            f"this runner reads {FORMAT}."
        )
    errors = sorted(_validator(schema).iter_errors(file), key=lambda error: list(error.path))
    if errors:
        where = "/".join(str(step) for step in errors[0].absolute_path)
        raise SetError(f"The file {name} does not match {schema} at /{where}: {errors[0].message}")
    return file


@functools.cache
def _shipped() -> tuple[Fixture, ...]:
    names = sorted(
        name[len("fixtures/") :]
        for name in _members()
        if name.startswith("fixtures/") and name.endswith(".json")
    )
    return tuple(Fixture(name, validated(name, member(f"fixtures/{name}"))) for name in names)


def fixtures() -> list[Fixture]:
    """The pinned set's fixture files, sorted by name."""
    return list(_shipped())


def load(directory: Path) -> list[Fixture]:
    """The fixture files of a directory, sorted by name: a set under development."""
    return [
        Fixture(path.name, validated(path.name, json.loads(path.read_text(encoding="utf-8"))))
        for path in sorted(directory.glob("*.json"), key=lambda path: path.name)
    ]
