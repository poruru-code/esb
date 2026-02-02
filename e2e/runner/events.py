# Where: e2e/runner/events.py
# What: Event and status definitions for E2E runner reporting.
# Why: Provide a stable, decoupled contract between execution and UI.
from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Any

EVENT_SUITE_START = "suite_start"
EVENT_SUITE_END = "suite_end"
EVENT_REGISTRY_READY = "registry_ready"
EVENT_ENV_START = "env_start"
EVENT_ENV_END = "env_end"
EVENT_PHASE_START = "phase_start"
EVENT_PHASE_END = "phase_end"
EVENT_PHASE_SKIP = "phase_skip"
EVENT_PHASE_PROGRESS = "phase_progress"
EVENT_MESSAGE = "message"

STATUS_QUEUED = "queued"
STATUS_RUNNING = "running"
STATUS_PASSED = "passed"
STATUS_FAILED = "failed"
STATUS_SKIPPED = "skipped"

PHASE_RESET = "reset"
PHASE_COMPOSE = "compose"
PHASE_DEPLOY = "deploy"
PHASE_TEST = "test"


@dataclass(frozen=True)
class Event:
    event_type: str
    env: str | None = None
    phase: str | None = None
    message: str | None = None
    data: dict[str, Any] = field(default_factory=dict)
    ts: float = field(default_factory=time.monotonic)
