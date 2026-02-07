# Where: e2e/runner/ui.py
# What: Plain reporter for E2E output (Rich removed).
# Why: Keep output deterministic and avoid terminal flicker.
from __future__ import annotations

import os
import sys
import time

from e2e.runner.events import (
    EVENT_ENV_END,
    EVENT_ENV_START,
    EVENT_MESSAGE,
    EVENT_PHASE_END,
    EVENT_PHASE_PROGRESS,
    EVENT_PHASE_SKIP,
    EVENT_PHASE_START,
    EVENT_REGISTRY_READY,
    EVENT_SUITE_END,
    EVENT_SUITE_START,
    PHASE_COMPOSE,
    PHASE_DEPLOY,
    PHASE_RESET,
    PHASE_TEST,
    STATUS_FAILED,
    STATUS_PASSED,
    STATUS_SKIPPED,
    Event,
)
from e2e.runner.live_display import LiveDisplay
from e2e.runner.logging import safe_print

_COLOR_RESET = "\033[0m"
_COLOR_GREEN = "\033[32m"
_COLOR_RED = "\033[31m"
_COLOR_YELLOW = "\033[33m"
_COLOR_BLUE = "\033[34m"
_COLOR_GRAY = "\033[90m"


def _resolve_feature(flag: bool | None, default: bool) -> bool:
    if flag is None:
        return default
    return bool(flag)


def _format_duration(seconds: float) -> str:
    if seconds < 60:
        return f"{seconds:.1f}s"
    total = int(seconds)
    mins = total // 60
    secs = total % 60
    return f"{mins}m{secs:02d}s"


class Reporter:
    def start(self) -> None:
        return None

    def emit(self, event: Event) -> None:
        raise NotImplementedError

    def close(self) -> None:
        return None


class PlainReporter(Reporter):
    def __init__(
        self,
        *,
        verbose: bool,
        env_label_width: int = 0,
        color: bool | None = None,
        emoji: bool | None = None,
        live_display: LiveDisplay | None = None,
        show_progress: bool = True,
    ) -> None:
        self._verbose = verbose
        self._env_label_width = max(env_label_width, 0)
        self._tag_width = max(self._env_label_width, len("suite"), len("warmup"), len("infra"))
        self._live_display = live_display
        self._show_progress = show_progress
        is_tty = sys.stdout.isatty()
        term = os.environ.get("TERM", "").lower()
        color_default = is_tty and term != "dumb" and not os.environ.get("NO_COLOR")
        emoji_default = is_tty and term != "dumb" and not os.environ.get("NO_EMOJI")
        self._color = _resolve_feature(color, color_default)
        self._emoji = _resolve_feature(emoji, emoji_default)
        self._last_progress: dict[str, tuple[int, float]] = {}
        self._progress_counts: dict[str, tuple[int, int | None]] = {}
        self._env_started: dict[str, float] = {}
        self._phase_width = max(
            len(PHASE_RESET),
            len(PHASE_COMPOSE),
            len(PHASE_DEPLOY),
            len(PHASE_TEST),
        )

    def _prefix_env(self, env: str) -> str:
        label = env.ljust(self._env_label_width) if self._env_label_width else env
        return f"[{label}]"

    def _prefix_tag(self, tag: str) -> str:
        label = tag.ljust(self._tag_width) if self._tag_width else tag
        return f"[{label}]"

    def _emoji_prefix(self, emoji: str) -> str:
        if not self._emoji or not emoji:
            return ""
        return f"{emoji} "

    def _colorize(self, text: str, color: str) -> str:
        if not self._color:
            return text
        return f"{color}{text}{_COLOR_RESET}"

    def _phase_label(self, phase: str) -> str:
        return phase.ljust(self._phase_width)

    def _status_word(self, status: str) -> str:
        if status == STATUS_PASSED:
            return self._colorize("ok", _COLOR_GREEN)
        if status == STATUS_FAILED:
            return self._colorize("failed", _COLOR_RED)
        if status == STATUS_SKIPPED:
            return self._colorize("skipped", _COLOR_YELLOW)
        return status

    def _env_status(self, status: str) -> str:
        if status == STATUS_PASSED:
            return self._colorize("PASS", _COLOR_GREEN)
        if status == STATUS_FAILED:
            return self._colorize("FAIL", _COLOR_RED)
        if status == STATUS_SKIPPED:
            return self._colorize("SKIP", _COLOR_YELLOW)
        return status

    def _status_icon(self, status: str) -> str:
        if status == STATUS_PASSED:
            return "✅"
        if status == STATUS_FAILED:
            return "❌"
        if status == STATUS_SKIPPED:
            return "⏭️"
        return ""

    def emit(self, event: Event) -> None:
        if event.event_type == EVENT_MESSAGE and event.message:
            self._log_line(event.message)
            return

        if event.event_type == EVENT_SUITE_START:
            self._log_line(f"{self._prefix_tag('suite')} {self._emoji_prefix('🧪')}started")
            return

        if event.event_type == EVENT_REGISTRY_READY:
            self._log_line(f"{self._prefix_tag('infra')} {self._emoji_prefix('🧰')}registry ready")
            return

        if event.event_type == EVENT_SUITE_END:
            status = str(event.data.get("status", "")).strip()
            if status == STATUS_PASSED:
                self._log_line(
                    f"{self._prefix_tag('suite')} {self._emoji_prefix('✅')}"
                    f"[PASSED] ALL MATRIX ENTRIES PASSED!"
                )
                return
            if status == STATUS_FAILED:
                failed = event.data.get("failed_envs")
                if isinstance(failed, list) and failed:
                    failed_text = ", ".join(str(item) for item in failed)
                else:
                    failed_text = "unknown"
                self._log_line(
                    f"{self._prefix_tag('suite')} {self._emoji_prefix('❌')}"
                    f"[FAILED] The following environments failed: "
                    f"{failed_text}"
                )
            return

        if event.event_type == EVENT_ENV_START and event.env:
            self._env_started[event.env] = time.monotonic()
            self._update_env_line(
                event.env,
                f"{self._prefix_env(event.env)} {self._emoji_prefix('🚀')}started",
            )
            return

        if event.event_type == EVENT_ENV_END and event.env:
            status = event.data.get("status", "")
            started = self._env_started.get(event.env)
            duration = _format_duration(time.monotonic() - started) if started else ""
            suffix = f" ({duration})" if duration else ""
            message = (
                f"{self._prefix_env(event.env)} {self._emoji_prefix('🏁')}done ... "
                f"{self._env_status(status)}{suffix}"
            )
            self._update_env_line(event.env, message)
            return

        if event.event_type == EVENT_PHASE_START and event.env and event.phase:
            if self._live_display or self._verbose:
                label = self._phase_label(event.phase)
                message = (
                    f"{self._prefix_env(event.env)} {self._emoji_prefix('⏳')}{label} ... start"
                )
                self._update_env_line(event.env, message)
            return

        if event.event_type == EVENT_PHASE_END and event.env and event.phase:
            status = event.data.get("status", "")
            duration = event.data.get("duration")
            duration_str = _format_duration(duration) if duration is not None else ""
            suffix_parts: list[str] = []
            if event.phase == PHASE_TEST:
                progress = self._progress_counts.get(event.env)
                if progress:
                    current, total = progress
                    total_str = "?" if total is None else str(total)
                    suffix_parts.append(f"{current}/{total_str}")
            if duration_str:
                suffix_parts.append(duration_str)
            suffix = f" ({', '.join(suffix_parts)})" if suffix_parts else ""
            icon = self._status_icon(status)
            icon_prefix = self._emoji_prefix(icon)
            label = self._phase_label(event.phase)
            message = (
                f"{self._prefix_env(event.env)} {icon_prefix}{label} ... "
                f"{self._status_word(status)}{suffix}"
            )
            self._update_env_line(event.env, message)
            return

        if event.event_type == EVENT_PHASE_SKIP and event.env and event.phase:
            label = self._phase_label(event.phase)
            message = f"{self._prefix_env(event.env)} {self._emoji_prefix('⏭️')}{label} ... skipped"
            self._update_env_line(event.env, message)
            return

        if event.event_type == EVENT_PHASE_PROGRESS and event.env and event.phase:
            if self._verbose and not self._live_display:
                return
            current = event.data.get("current")
            total = event.data.get("total")
            if current is None:
                return
            now = time.monotonic()
            last_value, last_ts = self._last_progress.get(event.env, (-1, 0.0))
            if current == last_value and (now - last_ts) < 0.5:
                return
            self._last_progress[event.env] = (int(current), now)
            total_str = "?" if total is None else str(int(total))
            total_value = int(total) if total is not None else None
            self._progress_counts[event.env] = (int(current), total_value)
            if not self._show_progress:
                return
            label = self._phase_label(event.phase)
            message = (
                f"{self._prefix_env(event.env)} {self._emoji_prefix('⏳')}{label} ... "
                f"{current}/{total_str}"
            )
            self._update_env_line(event.env, message)

    def _log_line(self, message: str) -> None:
        if self._live_display:
            self._live_display.log_line(message)
            return
        safe_print(message)

    def _update_env_line(self, env: str, message: str) -> None:
        if self._live_display:
            self._live_display.update_line(env, message)
            return
        safe_print(message)
