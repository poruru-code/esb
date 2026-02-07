# Where: e2e/runner/live_display.py
# What: Live multi-line display for parallel E2E progress.
# Why: Keep per-environment status on fixed lines in TTY output.
from __future__ import annotations

import threading

from e2e.runner.logging import safe_print, write_raw


class LiveDisplay:
    def __init__(self, profiles: list[str], *, label_width: int = 0) -> None:
        self._profiles = list(profiles)
        self._label_width = max(label_width, 0)
        self._lines: dict[str, str] = {profile: "" for profile in self._profiles}
        self._index = {profile: idx for idx, profile in enumerate(self._profiles)}
        self._lock = threading.Lock()
        self._started = False
        self._cursor_below_block = True

    def start(self) -> None:
        if not self._profiles:
            return
        with self._lock:
            if self._started:
                return
            for profile in self._profiles:
                line = self._lines.get(profile) or self._placeholder(profile)
                self._lines[profile] = line
                write_raw(f"{line}\n")
            self._started = True
            self._cursor_below_block = True

    def stop(self) -> None:
        with self._lock:
            if not self._started:
                return
            if not self._cursor_below_block:
                write_raw("\n")
            self._started = False

    def update_line(self, profile: str, line: str) -> None:
        if profile not in self._index:
            return
        with self._lock:
            self._lines[profile] = line
            if not self._started:
                return
            self._rewrite_line(profile, line)
            self._cursor_below_block = True

    def log_line(self, message: str) -> None:
        if not self._started:
            safe_print(message)
            return
        with self._lock:
            line_count = len(self._profiles)
            if line_count == 0:
                safe_print(message)
                return
            write_raw(f"\x1b[{line_count}A\r\x1b[J")
            write_raw(f"{message}\n")
            for profile in self._profiles:
                line = self._lines.get(profile) or self._placeholder(profile)
                write_raw(f"{line}\n")
            self._cursor_below_block = True

    def _rewrite_line(self, profile: str, line: str) -> None:
        line_count = len(self._profiles)
        if line_count == 0:
            safe_print(line)
            return
        idx = self._index[profile]
        up = line_count - idx
        write_raw(f"\x1b[{up}A\r\x1b[2K{line}\x1b[{up}B\r")

    def _placeholder(self, profile: str) -> str:
        label = self._format_label(profile)
        return f"[{label}] waiting"

    def _format_label(self, profile: str) -> str:
        if self._label_width <= 0:
            return profile
        return profile.ljust(self._label_width)
