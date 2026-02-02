# Where: e2e/runner/ui.py
# What: Plain reporter for E2E output (Rich removed).
# Why: Keep output deterministic and avoid terminal flicker.
from __future__ import annotations

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
    Event,
)
from e2e.runner.logging import safe_print


class Reporter:
    def start(self) -> None:
        return None

    def emit(self, event: Event) -> None:
        raise NotImplementedError

    def close(self) -> None:
        return None


class PlainReporter(Reporter):
    def __init__(self, *, verbose: bool) -> None:
        self._verbose = verbose
        self._last_progress: dict[str, tuple[int, float]] = {}

    def emit(self, event: Event) -> None:
        if event.event_type == EVENT_MESSAGE and event.message:
            safe_print(event.message)
            return
        if event.event_type == EVENT_SUITE_START:
            safe_print("[suite] started")
            return
        if event.event_type == EVENT_REGISTRY_READY:
            safe_print("[infra] registry ready")
            return
        if event.event_type == EVENT_SUITE_END:
            safe_print("[suite] finished")
            return
        if event.event_type == EVENT_ENV_START and event.env:
            safe_print(f"[{event.env}] started")
        if event.event_type == EVENT_ENV_END and event.env:
            status = event.data.get("status", "")
            safe_print(f"[{event.env}] {status}")
        if event.event_type == EVENT_PHASE_START and event.env and event.phase:
            safe_print(f"[{event.env}] {event.phase} start")
        if event.event_type == EVENT_PHASE_END and event.env and event.phase:
            status = event.data.get("status", "")
            safe_print(f"[{event.env}] {event.phase} {status}")
        if event.event_type == EVENT_PHASE_SKIP and event.env and event.phase:
            safe_print(f"[{event.env}] {event.phase} skipped")
        if (
            event.event_type == EVENT_PHASE_PROGRESS
            and event.env
            and event.phase
            and not self._verbose
        ):
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
            safe_print(f"[{event.env}] {event.phase} progress {current}/{total_str}")
