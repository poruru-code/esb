# Where: e2e/runner/planner.py
# What: Build execution scenarios from the E2E matrix.
# Why: Keep planning logic separate from execution and UI.
from __future__ import annotations

from typing import Any

from e2e.runner.branding import resolve_project_name
from e2e.runner.config import build_env_scenarios
from e2e.runner.models import Scenario


def build_plan(
    matrix: list[dict[str, Any]],
    suites: dict[str, Any],
    *,
    profile_filter: str | None = None,
) -> dict[str, Scenario]:
    raw = build_env_scenarios(matrix, suites, profile_filter=profile_filter)
    return {env: _to_scenario(data) for env, data in raw.items()}


def apply_test_target(
    scenarios: dict[str, Scenario],
    *,
    env_name: str,
    target: str,
) -> dict[str, Scenario]:
    scenario = scenarios.get(env_name)
    if not scenario:
        return {}
    updated = Scenario(
        name=scenario.name,
        env_name=scenario.env_name,
        mode=scenario.mode,
        env_file=scenario.env_file,
        env_dir=scenario.env_dir,
        targets=[target],
        exclude=[],
        project_name=scenario.project_name,
        extra=scenario.extra,
    )
    return {env_name: updated}


def _to_scenario(data: dict[str, Any]) -> Scenario:
    env_file = data.get("env_file") or None
    known_keys = {
        "name",
        "env",
        "mode",
        "env_file",
        "env_dir",
        "targets",
        "exclude",
        "esb_project",
    }
    return Scenario(
        name=data.get("name", ""),
        env_name=data.get("env", ""),
        mode=data.get("mode", "docker"),
        env_file=env_file,
        env_dir=data.get("env_dir"),
        targets=data.get("targets", []) or [],
        exclude=data.get("exclude", []) or [],
        project_name=resolve_project_name(data.get("esb_project")),
        extra={k: v for k, v in data.items() if k not in known_keys},
    )
