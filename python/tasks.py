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
from collections.abc import Callable
from pathlib import Path

ROOT = Path(__file__).resolve().parent
DIST = ROOT / "dist"
WHEEL_VENV = ROOT / ".venv-dist"


def _run(*args: str | Path, env: dict[str, str] | None = None) -> None:
    command = [str(arg) for arg in args]
    print("+", " ".join(command), flush=True)
    if subprocess.run(command, cwd=ROOT, env=env, check=False).returncode:
        raise SystemExit(1)


def _env(**extra: str) -> dict[str, str]:
    env = dict(os.environ)
    env["UV_PROJECT_ENVIRONMENT"] = str(ROOT / ".venv")
    env.update(extra)
    return env


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


def sync() -> None:
    _run("uv", "sync", "--locked", env=_env())


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


def _wheel_venv() -> None:
    shutil.rmtree(WHEEL_VENV, ignore_errors=True)
    _run("uv", "venv", WHEEL_VENV, env=_env())
    requirements = WHEEL_VENV / "requirements.txt"
    _run(
        "uv",
        "export",
        "--locked",
        "--only-group",
        "test",
        "--no-emit-project",
        "--output-file",
        requirements,
        "--quiet",
        env=_env(),
    )
    python = _python(WHEEL_VENV)
    _run("uv", "pip", "install", "--python", python, "--require-hashes", "-r", requirements)
    _run("uv", "pip", "install", "--python", python, "--no-deps", _wheel())


def test() -> None:
    """What CI runs: build, lint, typecheck, then the suite on the source and on the wheel."""
    build()
    lint()
    typecheck()
    wheel = str(_wheel())
    _tool("pytest", env=_env(CURLY_MESSAGE_WHEEL=wheel))
    _wheel_venv()
    _run(
        _python(WHEEL_VENV),
        "-m",
        "pytest",
        env=_env(CURLY_MESSAGE_DIST="1", CURLY_MESSAGE_WHEEL=wheel),
    )


TASKS: dict[str, Callable[[], None]] = {
    "build": build,
    "lint": lint,
    "lint-fix": lint_fix,
    "test": test,
    "typecheck": typecheck,
}


def main(argv: list[str]) -> None:
    if len(argv) != 1 or argv[0] not in TASKS:
        raise SystemExit(f"usage: tasks.py {{{','.join(TASKS)}}}")
    TASKS[argv[0]]()


if __name__ == "__main__":
    main(sys.argv[1:])
