# Where: e2e/runner/logging.py
# What: Log sinks and subprocess streaming helpers for E2E runs.
# Why: Ensure full logs are always persisted while keeping UI optional.
from __future__ import annotations

import subprocess
import sys
import threading
import urllib.parse
from pathlib import Path
from typing import Callable, TextIO

_OUTPUT_LOCK = threading.Lock()
_PROXY_ENV_KEYS = {
    "HTTP_PROXY",
    "http_proxy",
    "HTTPS_PROXY",
    "https_proxy",
    "ALL_PROXY",
    "all_proxy",
}


def safe_print(message: str = "", *, prefix: str | None = None) -> None:
    with _OUTPUT_LOCK:
        if prefix:
            print(f"{prefix} {message}", flush=True)
        else:
            print(message, flush=True)


def write_raw(message: str) -> None:
    with _OUTPUT_LOCK:
        sys.stdout.write(message)
        sys.stdout.flush()


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
    display_cmd = _redact_cmd(cmd)
    rendered_cmd = f"$ {' '.join(display_cmd)}"
    log.write_line(rendered_cmd)
    if printer:
        printer(rendered_cmd)
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


def _redact_cmd(cmd: list[str]) -> list[str]:
    redacted: list[str] = []
    for token in cmd:
        redacted.append(_redact_proxy_token(token))
    return redacted


def _redact_proxy_token(token: str) -> str:
    if "=" not in token:
        return token
    key, value = token.split("=", 1)
    canonical_key = key.strip("\"'")
    suffix_key = canonical_key.split(".")[-1]
    if canonical_key in _PROXY_ENV_KEYS or suffix_key in _PROXY_ENV_KEYS:
        return f"{key}={_redact_proxy_url(value)}"
    return token


def _redact_proxy_url(raw: str) -> str:
    try:
        parsed = urllib.parse.urlsplit(raw.strip())
    except ValueError:
        return raw
    if not parsed.scheme or not parsed.hostname:
        return raw
    if parsed.username is None:
        return raw
    username = urllib.parse.quote("***", safe="")
    password = urllib.parse.quote("***", safe="") if parsed.password is not None else ""
    auth = username if password == "" else f"{username}:{password}"
    host = parsed.hostname
    if ":" in host and not host.startswith("["):
        host = f"[{host}]"
    if parsed.port is not None:
        host = f"{host}:{parsed.port}"
    netloc = f"{auth}@{host}"
    return urllib.parse.urlunsplit(
        (parsed.scheme, netloc, parsed.path, parsed.query, parsed.fragment)
    )
