# Where: e2e/runner/logging.py
# What: Log sinks and subprocess streaming helpers for E2E runs.
# Why: Ensure full logs are always persisted while keeping UI optional.
from __future__ import annotations

import subprocess
import threading
from pathlib import Path
from typing import Callable, TextIO

_OUTPUT_LOCK = threading.Lock()


def safe_print(message: str = "", *, prefix: str | None = None) -> None:
    with _OUTPUT_LOCK:
        if prefix:
            print(f"{prefix} {message}", flush=True)
        else:
            print(message, flush=True)


class LogSink:
    def __init__(self, path: Path) -> None:
        self.path = path
        self._file: TextIO | None = None
        self._lock = threading.Lock()

    def open(self) -> None:
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._file = self.path.open("w", encoding="utf-8")

    def close(self) -> None:
        if self._file:
            self._file.close()
            self._file = None

    def write_line(self, line: str) -> None:
        if self._file is None:
            raise RuntimeError("LogSink is not open")
        with self._lock:
            self._file.write(f"{line}\n")
            self._file.flush()


def make_prefix_printer(
    label: str, phase: str | None = None, *, width: int = 0
) -> Callable[[str], None]:
    formatted = label.ljust(width) if width > 0 else label
    if phase:
        prefix = f"[{formatted}][{phase}] |"
    else:
        prefix = f"[{formatted}]"

    def _printer(line: str) -> None:
        safe_print(line, prefix=prefix)

    return _printer


def run_and_stream(
    cmd: list[str],
    *,
    cwd: Path,
    env: dict[str, str],
    log: LogSink,
    printer: Callable[[str], None] | None = None,
    on_line: Callable[[str], None] | None = None,
) -> int:
    log.write_line(f"$ {' '.join(cmd)}")
    if printer:
        printer(f"$ {' '.join(cmd)}")
    proc = subprocess.Popen(
        cmd,
        cwd=str(cwd),
        env=env,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        bufsize=1,
        errors="replace",
    )
    assert proc.stdout is not None
    for raw_line in proc.stdout:
        line = raw_line.rstrip("\n")
        log.write_line(line)
        if on_line:
            on_line(line)
        if printer:
            printer(line)
    return proc.wait()
