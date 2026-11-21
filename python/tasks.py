"""The package's tasks, run from this directory as ``uv run --no-project tasks.py <task>``.

Every environment is synced from ``uv.lock`` and every build reads the hashed
``build-constraints.txt``, so a task runs the dependencies CI runs. The
interpreter is uv's choice, or ``UV_PYTHON`` where it is set.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tarfile
import tempfile
from collections.abc import Callable
from pathlib import Path

ROOT = Path(__file__).resolve().parent
DIST = ROOT / "dist"
VENV = ROOT / ".venv"
WHEEL_VENV = ROOT / ".venv-dist"
CORE_VENV = ROOT / ".venv-core"
CORE_WHEEL_VENV = ROOT / ".venv-core-dist"
BENCH_VENV = ROOT / ".bench-venv"
LOWEST_BABEL = "babel==2.14.0"
"""The oldest Babel the ``intl`` extra accepts, which ``test-lowest`` runs."""


def _run(*args: str | Path, env: dict[str, str] | None = None) -> None:
    command = [str(arg) for arg in args]
    print("+", " ".join(command), flush=True)
    if subprocess.run(command, cwd=ROOT, env=env, check=False).returncode:
        raise SystemExit(1)


def _env(venv: Path = VENV, **extra: str) -> dict[str, str]:
    env = dict(os.environ)
    env["UV_PROJECT_ENVIRONMENT"] = str(venv)
    env.update(extra)
    return env


def _extras(intl: bool) -> tuple[str, ...]:
    return ("--extra", "intl") if intl else ()


def _tool(*args: str, env: dict[str, str] | None = None) -> None:
    _run("uv", "run", "--locked", "--no-sync", *args, env=env or _env())


def _python(venv: Path) -> Path:
    return venv / ("Scripts/python.exe" if os.name == "nt" else "bin/python")


def _wheel() -> Path:
    wheels = sorted(DIST.glob("*.whl"))
    if len(wheels) != 1:
        raise SystemExit(f"dist/ holds {len(wheels)} wheels, not one")
    return wheels[0]


def build() -> None:
    """The sdist, then the wheel built from it."""
    shutil.rmtree(DIST, ignore_errors=True)
    _run(
        "uv",
        "build",
        "--build-constraints",
        "build-constraints.txt",
        "--require-hashes",
        env=_env(),
    )


def sync(venv: Path = VENV, *, intl: bool = True) -> None:
    _run("uv", "sync", "--locked", *_extras(intl), env=_env(venv))


def lint() -> None:
    """The formatting contract and the lint rules, fixing nothing."""
    sync()
    _tool("ruff", "check")
    _tool("ruff", "format", "--check")


def lint_fix() -> None:
    sync()
    _tool("ruff", "check", "--fix")
    _tool("ruff", "format")


def typecheck() -> None:
    sync()
    _tool("mypy")


def _wheel_venv(venv: Path, *, intl: bool, test: bool = True) -> None:
    shutil.rmtree(venv, ignore_errors=True)
    _run("uv", "venv", venv, env=_env())
    requirements = venv / "requirements.txt"
    _run(
        "uv",
        "export",
        "--locked",
        *_extras(intl),
        "--no-default-groups",
        *(("--group", "test") if test else ()),
        "--no-emit-project",
        "--output-file",
        requirements,
        "--quiet",
        env=_env(),
    )
    python = _python(venv)
    _run("uv", "pip", "install", "--python", python, "--require-hashes", "-r", requirements)
    _run("uv", "pip", "install", "--python", python, "--no-deps", _wheel())


def typecheck_wheel(wheel_venv: Path = WHEEL_VENV) -> None:
    """The type surface checked against the wheel's own ``py.typed``, as a consumer checks it.

    No configuration is read, so ``src/`` is not on the checker's path: the
    package it finds is the one installed in ``wheel_venv``.
    """
    _tool(
        "mypy",
        "--config-file",
        "",
        "--strict",
        "--warn-unused-ignores",
        "--python-version",
        "3.11",
        "--python-executable",
        str(_python(wheel_venv)),
        "tests/types/surface.py",
    )


def _suite(
    venv: Path, wheel_venv: Path, *, intl: bool, lowest: bool = False, surface: bool = False
) -> None:
    # Each leg states whether it expects Intl, so a supplier that failed to
    # install cannot pass as a leg that never asked for one.
    expect = {
        "CURLY_MESSAGE_EXPECT_INTL": os.environ.get(
            "CURLY_MESSAGE_EXPECT_INTL", "1" if intl else "0"
        )
    }
    wheel = str(_wheel())
    if lowest:
        _run("uv", "pip", "install", "--python", _python(venv), LOWEST_BABEL)
    _tool("pytest", env=_env(venv, CURLY_MESSAGE_WHEEL=wheel, **expect))
    _wheel_venv(wheel_venv, intl=intl)
    if lowest:
        _run("uv", "pip", "install", "--python", _python(wheel_venv), LOWEST_BABEL)
    if surface:
        typecheck_wheel(wheel_venv)
    _run(
        _python(wheel_venv),
        "-m",
        "pytest",
        env=_env(venv, CURLY_MESSAGE_DIST="1", CURLY_MESSAGE_WHEEL=wheel, **expect),
    )


def test() -> None:
    """What CI runs: build, lint, typecheck, then the suite on the source and on the wheel.

    The type surface is checked against the wheel before the suite runs on it.
    """
    build()
    lint()
    typecheck()
    _suite(VENV, WHEEL_VENV, intl=True, surface=True)


def test_core() -> None:
    """The suite on the source and on the wheel, installed without the ``intl`` extra.

    The lint and the typecheck read the source, which is the same either
    way; they run in ``test``.
    """
    build()
    sync(CORE_VENV, intl=False)
    _suite(CORE_VENV, CORE_WHEEL_VENV, intl=False)


def test_lowest() -> None:
    """The suite on the source and on the wheel, over the oldest Babel the extra accepts."""
    build()
    sync()
    _suite(VENV, WHEEL_VENV, intl=True, lowest=True)


def test_sdist() -> None:
    """The sdist's own ``test`` task, run in the tree the sdist unpacks to.

    A downstream build runs the suite from the sdist, so what the suite or a
    task reads and the sdist leaves out fails here, where a checkout would
    still hold it.
    """
    build()
    sdists = sorted(DIST.glob("*.tar.gz"))
    if len(sdists) != 1:
        raise SystemExit(f"dist/ holds {len(sdists)} sdists, not one")
    env = dict(os.environ)
    for name in ("UV_PROJECT_ENVIRONMENT", "VIRTUAL_ENV"):
        env.pop(name, None)
    with tempfile.TemporaryDirectory() as scratch:
        with tarfile.open(sdists[0]) as packed:
            packed.extractall(scratch, filter="data")
        (tree,) = Path(scratch).iterdir()
        _run("uv", "run", "--no-project", tree / "tasks.py", "test", env=env)


def bench_venv(*, no_build: bool = False) -> None:
    """The wheel and the ``intl`` extra, installed into ``.bench-venv`` for ``bench/rows.mjs``.

    The wheel is built first, unless ``no_build`` is given and ``dist/``
    already holds one: the release measures the very wheel it publishes.
    """
    if not (no_build and list(DIST.glob("*.whl"))):
        build()
    _wheel_venv(BENCH_VENV, intl=True, test=False)


TASKS: dict[str, Callable[[], None]] = {
    "bench-venv": bench_venv,
    "build": build,
    "lint": lint,
    "lint-fix": lint_fix,
    "test": test,
    "test-core": test_core,
    "test-lowest": test_lowest,
    "test-sdist": test_sdist,
    "typecheck": typecheck,
}


def main(argv: list[str]) -> None:
    if argv == ["bench-venv", "--no-build"]:
        bench_venv(no_build=True)
        return
    if len(argv) != 1 or argv[0] not in TASKS:
        raise SystemExit(f"usage: tasks.py {{{','.join(TASKS)}}} (bench-venv [--no-build])")
    TASKS[argv[0]]()


if __name__ == "__main__":
    main(sys.argv[1:])
