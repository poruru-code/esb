# Where: e2e/runner/warmup.py
# What: Warmup helpers for E2E runs.
# Why: Perform only buildx preflight needed by artifact-only deploy.
from __future__ import annotations

from pathlib import Path
from typing import Callable

from e2e.runner import constants
from e2e.runner.branding import resolve_project_name
from e2e.runner.buildx import ensure_buildx_builder
from e2e.runner.env import apply_proxy_defaults, calculate_runtime_env
from e2e.runner.models import Scenario
from e2e.runner.utils import PROJECT_ROOT

_PROXY_ENV_KEYS = (
    "HTTP_PROXY",
    "http_proxy",
    "HTTPS_PROXY",
    "https_proxy",
    "NO_PROXY",
    "no_proxy",
)


def _warmup(
    scenarios: dict[str, Scenario],
    *,
    printer: Callable[[str], None] | None = None,
    verbose: bool = False,
) -> None:
    del printer
    del verbose
    _ensure_buildx_builders(scenarios)


def _resolve_env_file(env_file: str | None) -> str | None:
    if not env_file:
        return None
    path = Path(env_file)
    if not path.is_absolute():
        path = PROJECT_ROOT / path
    return str(path.absolute())


def _scenario_runtime_env_for_buildx(scenario: Scenario) -> dict[str, str]:
    runtime_env = calculate_runtime_env(
        resolve_project_name(scenario.project_name),
        scenario.env_name,
        scenario.mode,
        _resolve_env_file(scenario.env_file),
    )
    apply_proxy_defaults(runtime_env)
    return runtime_env


def _ensure_buildx_builders(scenarios: dict[str, Scenario]) -> None:
    seen: set[tuple[str, str, tuple[tuple[str, str], ...]]] = set()
    for scenario in scenarios.values():
        runtime_env = _scenario_runtime_env_for_buildx(scenario)
        builder_name = runtime_env.get("BUILDX_BUILDER", "").strip()
        if not builder_name:
            continue
        config_path = runtime_env.get(constants.ENV_BUILDKITD_CONFIG, "").strip()
        proxy_signature = tuple((key, runtime_env.get(key, "").strip()) for key in _PROXY_ENV_KEYS)
        signature = (builder_name, config_path, proxy_signature)
        if signature in seen:
            continue
        seen.add(signature)
        ensure_buildx_builder(
            builder_name,
            config_path=config_path,
            proxy_source=runtime_env,
        )
