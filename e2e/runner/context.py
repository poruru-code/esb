# Where: e2e/runner/context.py
# What: Context and environment assembly helpers for E2E runner execution.
# Why: Keep runtime context setup separate from orchestration flow.
from __future__ import annotations

import os
from pathlib import Path

from e2e.runner import constants, infra
from e2e.runner.buildx import ensure_buildx_builder
from e2e.runner.env import (
    apply_gateway_env_from_container,
    apply_proxy_defaults,
    calculate_runtime_env,
    calculate_staging_dir,
    read_env_file,
)
from e2e.runner.lifecycle import resolve_compose_file
from e2e.runner.models import RunContext, Scenario
from e2e.runner.utils import (
    BRAND_SLUG,
    E2E_STATE_ROOT,
    PROJECT_ROOT,
    build_unique_tag,
    default_e2e_deploy_templates,
    env_key,
)

_CREDENTIAL_KEYS = {
    constants.ENV_AUTH_USER,
    constants.ENV_AUTH_PASS,
    constants.ENV_JWT_SECRET_KEY,
    constants.ENV_X_API_KEY,
    constants.ENV_RUSTFS_ACCESS_KEY,
    constants.ENV_RUSTFS_SECRET_KEY,
}


def _prepare_context(
    scenario: Scenario,
    port_overrides: dict[str, str] | None = None,
) -> RunContext:
    env_name = scenario.env_name
    project_name = scenario.project_name or BRAND_SLUG
    compose_project = f"{project_name}-{env_name}"
    env_file = _resolve_env_file(scenario.env_file)

    compose_file = resolve_compose_file(scenario)
    templates = _resolve_templates(scenario)
    template_path = templates[0]

    runtime_env = calculate_runtime_env(
        project_name,
        env_name,
        scenario.mode,
        env_file,
        template_path=str(template_path),
    )

    state_env = _load_state_env(env_name)
    for key in _CREDENTIAL_KEYS:
        if key in state_env:
            runtime_env[key] = state_env[key]

    runtime_env.update(scenario.env_vars)
    apply_proxy_defaults(runtime_env)
    _apply_port_overrides(runtime_env, port_overrides)
    runtime_env[env_key("PROJECT")] = project_name
    runtime_env[env_key("ENV")] = env_name
    runtime_env[env_key("INTERACTIVE")] = "0"
    runtime_env[env_key("HOME")] = str((E2E_STATE_ROOT / env_name).absolute())
    runtime_env[constants.ENV_PROJECT_NAME] = compose_project

    staging_config_dir = calculate_staging_dir(
        compose_project,
        env_name,
        template_path=str(template_path),
    )
    runtime_env[constants.ENV_CONFIG_DIR] = str(staging_config_dir)
    staging_config_dir.mkdir(parents=True, exist_ok=True)

    tag_key = env_key(constants.ENV_TAG)
    tag_override = scenario.env_vars.get(tag_key)
    if tag_override:
        runtime_env[tag_key] = tag_override
    else:
        current_tag = runtime_env.get(tag_key, "").strip()
        if current_tag in ("", "latest"):
            runtime_env[tag_key] = build_unique_tag(env_name)

    host_addr, service_addr = infra.get_registry_config()
    runtime_registry = host_addr if scenario.mode.lower() == "docker" else service_addr
    runtime_env["HOST_REGISTRY_ADDR"] = host_addr
    runtime_env[constants.ENV_CONTAINER_REGISTRY] = runtime_registry
    runtime_env["REGISTRY"] = f"{runtime_registry}/"

    deploy_env = os.environ.copy()
    deploy_env.update(runtime_env)
    deploy_env["PROJECT_NAME"] = compose_project
    deploy_env[constants.ENV_META_REUSE] = "1"
    deploy_env.update(scenario.env_vars)

    pytest_env = os.environ.copy()
    pytest_env.update(runtime_env)
    pytest_env.update(scenario.env_vars)

    ensure_buildx_builder(
        runtime_env.get("BUILDX_BUILDER", ""),
        config_path=runtime_env.get(constants.ENV_BUILDKITD_CONFIG, ""),
        proxy_source=runtime_env,
    )

    return RunContext(
        scenario=scenario,
        project_name=project_name,
        compose_project=compose_project,
        compose_file=compose_file,
        env_file=env_file,
        runtime_env=runtime_env,
        deploy_env=deploy_env,
        pytest_env=pytest_env,
    )


def _resolve_env_file(env_file: str | None) -> str | None:
    if not env_file:
        return None
    path = Path(env_file)
    if not path.is_absolute():
        path = PROJECT_ROOT / path
    return str(path.absolute())


def _resolve_templates(scenario: Scenario) -> list[Path]:
    if scenario.deploy_templates:
        return [_resolve_template_path(Path(template)) for template in scenario.deploy_templates]
    return default_e2e_deploy_templates()


def _resolve_template_path(path: Path) -> Path:
    if path.is_absolute():
        return path
    return (PROJECT_ROOT / path).resolve()


def _apply_ports_to_env_dict(ports: dict[str, int], env: dict[str, str]) -> None:
    for key, value in ports.items():
        env[key] = str(value)

    gw_key = env_key(constants.PORT_GATEWAY_HTTPS)
    if gw_key in ports:
        gw_port = ports[gw_key]
        env[gw_key] = str(gw_port)
        env[constants.ENV_GatewayPort] = str(gw_port)
        env[constants.ENV_GatewayURL] = f"https://localhost:{gw_port}"

    vl_key = env_key(constants.PORT_VICTORIALOGS)
    if vl_key in ports:
        vl_port = ports[vl_key]
        env[vl_key] = str(vl_port)
        env[constants.ENV_VictoriaLogsPort] = str(vl_port)
        env[constants.ENV_VictoriaLogsURL] = f"http://localhost:{vl_port}"

    agent_key = env_key(constants.PORT_AGENT_GRPC)
    if agent_key in ports:
        agent_port = ports[agent_key]
        env[agent_key] = str(agent_port)
        env[constants.ENV_AgentGrpcAddress] = f"localhost:{agent_port}"

    agent_metrics_key = env_key(constants.PORT_AGENT_METRICS)
    if agent_metrics_key in ports:
        metrics_port = ports[agent_metrics_key]
        env[agent_metrics_key] = str(metrics_port)
        env["AGENT_METRICS_PORT"] = str(metrics_port)
        env["AGENT_METRICS_URL"] = f"http://localhost:{metrics_port}"


def _state_env_path(env_name: str) -> Path:
    return E2E_STATE_ROOT / env_name / "config" / ".env"


def _load_state_env(env_name: str) -> dict[str, str]:
    path = _state_env_path(env_name)
    if not path.exists():
        return {}
    return read_env_file(str(path))


def _persist_state_env(env_name: str, runtime_env: dict[str, str]) -> None:
    path = _state_env_path(env_name)
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = []
    for key in sorted(_CREDENTIAL_KEYS):
        value = runtime_env.get(key)
        if value:
            lines.append(f"{key}={value}")
    path.write_text("\n".join(lines) + ("\n" if lines else ""), encoding="utf-8")


def _sync_gateway_env(ctx: RunContext) -> None:
    apply_gateway_env_from_container(ctx.pytest_env, ctx.compose_project)
    _persist_state_env(ctx.scenario.env_name, ctx.pytest_env)


def _apply_port_overrides(runtime_env: dict[str, str], overrides: dict[str, str] | None) -> None:
    if not overrides:
        return
    for key, value in overrides.items():
        current = runtime_env.get(key)
        if not current or current == "0":
            runtime_env[key] = value
