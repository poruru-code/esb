# Where: e2e/runner/models.py
# What: Dataclasses for E2E runner planning and execution context.
# Why: Keep execution inputs explicit and avoid implicit global state.
from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


@dataclass(frozen=True)
class Scenario:
    name: str
    env_name: str
    mode: str
    env_file: str | None
    env_dir: str | None
    env_vars: dict[str, str]
    targets: list[str]
    exclude: list[str]
    deploy_templates: list[str] = field(default_factory=list)
    project_name: str | None = None
    extra: dict[str, Any] = field(default_factory=dict)


@dataclass
class RunContext:
    scenario: Scenario
    project_name: str
    compose_project: str
    compose_file: Path
    env_file: str | None
    runtime_env: dict[str, str]
    deploy_env: dict[str, str]
    pytest_env: dict[str, str]
    ports: dict[str, int] = field(default_factory=dict)
