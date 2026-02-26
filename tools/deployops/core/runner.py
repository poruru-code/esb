"""Command execution helpers for deployops commands."""

from __future__ import annotations

import os
import shlex
import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Mapping, Sequence


@dataclass(frozen=True)
class CompletedCommand:
    """Normalized command execution result."""

    cmd: tuple[str, ...]
    returncode: int
    stdout: str = ""
    stderr: str = ""


class RunnerError(RuntimeError):
    """Raised when a command execution fails."""


class CommandRunner:
    """Thin subprocess wrapper with dry-run support and deterministic logging."""

    def __init__(
        self,
        *,
        dry_run: bool = False,
        printer: Callable[[str], None] | None = None,
    ) -> None:
        self.dry_run = dry_run
        self._printer = printer or print

    def format_cmd(self, cmd: Sequence[str]) -> str:
        return "$ " + " ".join(shlex.quote(str(token)) for token in cmd)

    def emit(self, message: str) -> None:
        self._printer(message)

    def which(self, command: str) -> str | None:
        resolved = shutil.which(command)
        if resolved is None:
            return None
        return str(Path(resolved).resolve())

    def require_command(self, command: str) -> str:
        resolved = self.which(command)
        if resolved is None:
            raise RunnerError(f"required command not found: {command}")
        return resolved

    def run(
        self,
        cmd: Sequence[str],
        *,
        cwd: Path | None = None,
        env: Mapping[str, str] | None = None,
        capture_output: bool = False,
        check: bool = True,
        stream_output: bool = False,
        on_line: Callable[[str], None] | None = None,
        run_in_dry_run: bool = False,
    ) -> CompletedCommand:
        rendered = self.format_cmd(cmd)
        if self.dry_run and not run_in_dry_run:
            self.emit(f"[dry-run] {rendered}")
            return CompletedCommand(tuple(str(token) for token in cmd), 0, "", "")

        run_env = os.environ.copy()
        if env:
            run_env.update({str(key): str(value) for key, value in env.items()})

        if stream_output:
            self.emit(rendered)
            proc = subprocess.Popen(
                [str(token) for token in cmd],
                cwd=str(cwd) if cwd else None,
                env=run_env,
                stdin=subprocess.DEVNULL,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                bufsize=1,
                errors="replace",
            )
            assert proc.stdout is not None
            captured: list[str] = []
            for raw_line in proc.stdout:
                line = raw_line.rstrip("\n")
                captured.append(line)
                if on_line:
                    on_line(line)
                self.emit(line)
            rc = proc.wait()
            stdout = "\n".join(captured)
            if check and rc != 0:
                raise RunnerError(f"command failed with exit code {rc}: {rendered}")
            return CompletedCommand(tuple(str(token) for token in cmd), rc, stdout, "")

        completed = subprocess.run(
            [str(token) for token in cmd],
            cwd=str(cwd) if cwd else None,
            env=run_env,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE if capture_output else None,
            stderr=subprocess.PIPE if capture_output else None,
            text=True,
            check=False,
            errors="replace",
        )

        if check and completed.returncode != 0:
            stderr = (completed.stderr or "").strip()
            stdout = (completed.stdout or "").strip()
            detail = stderr or stdout
            if detail:
                raise RunnerError(
                    f"command failed with exit code {completed.returncode}: {rendered}\n{detail}"
                )
            raise RunnerError(f"command failed with exit code {completed.returncode}: {rendered}")

        return CompletedCommand(
            tuple(str(token) for token in cmd),
            completed.returncode,
            completed.stdout or "",
            completed.stderr or "",
        )
