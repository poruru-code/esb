# Where: e2e/runner/lifecycle.py
# What: Lifecycle operations (reset/up/down) for E2E environments.
# Why: Separate environment orchestration from scenario execution logic.
from __future__ import annotations

import os
import shutil
import time
from pathlib import Path
from typing import Callable

import requests
import urllib3

from e2e.runner import constants, infra
from e2e.runner.cleanup import cleanup_managed_images, isolate_external_network, thorough_cleanup
from e2e.runner.env import discover_ports
from e2e.runner.logging import LogSink, run_and_stream
from e2e.runner.models import RunContext, Scenario
from e2e.runner.utils import (
    E2E_STATE_ROOT,
    PROJECT_ROOT,
    env_key,
    resolve_env_file_path,
)


def resolve_compose_file(scenario: Scenario) -> Path:
    env_dir = scenario.env_dir
    if env_dir:
        compose_path = PROJECT_ROOT / env_dir / "docker-compose.yml"
        if compose_path.exists():
            return compose_path
        raise FileNotFoundError(f"Compose file not found in env_dir: {compose_path}")
    return PROJECT_ROOT / f"docker-compose.{scenario.mode}.yml"


def reset_environment(
    ctx: RunContext,
    *,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    env_name = ctx.scenario.env_name
    project_label = ctx.compose_project
    log.write_line(f"Resetting environment: {env_name}")
    if printer:
        printer(f"Resetting environment: {env_name}")

    if ctx.compose_file.exists():
        down_cmd = _compose_base_cmd(
            project_name=project_label,
            compose_file=ctx.compose_file,
            env_file=ctx.env_file,
        )
        down_cmd.extend(["down", "--volumes", "--remove-orphans"])
        run_and_stream(
            down_cmd,
            cwd=PROJECT_ROOT,
            env=_compose_env(ctx),
            log=log,
            printer=printer,
        )

    thorough_cleanup(project_label, env_name, log=log.write_line, printer=printer)
    cleanup_managed_images(
        env_name,
        ctx.project_name,
        log=log.write_line,
        printer=printer,
    )
    isolate_external_network(project_label, log=log.write_line, printer=printer)

    env_state_dir = E2E_STATE_ROOT / env_name
    if env_state_dir.exists():
        log.write_line(f"  - Cleaning artifact directory: {env_state_dir}")
        if printer:
            printer(f"  - Cleaning artifact directory: {env_state_dir}")
        shutil.rmtree(env_state_dir)


def compose_up(
    ctx: RunContext,
    *,
    build: bool,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> dict[str, int]:
    if not ctx.compose_file.exists():
        raise FileNotFoundError(f"Compose file not found: {ctx.compose_file}")

    compose_cmd = _compose_base_cmd(
        project_name=ctx.compose_project,
        compose_file=ctx.compose_file,
        env_file=ctx.env_file,
    )
    compose_cmd.extend(["up", "--detach"])
    if build:
        compose_cmd.append("--build")

    run_and_stream(
        compose_cmd,
        cwd=PROJECT_ROOT,
        env=_compose_env(ctx),
        log=log,
        printer=printer,
    )

    infra.connect_registry_to_network(ctx.runtime_env.get(constants.ENV_NETWORK_EXTERNAL, ""))
    return discover_ports(ctx.compose_project, ctx.compose_file, env_file=ctx.env_file)


def compose_down(
    ctx: RunContext,
    *,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    down_cmd = _compose_base_cmd(
        project_name=ctx.compose_project,
        compose_file=ctx.compose_file,
        env_file=ctx.env_file,
    )
    down_cmd.append("down")
    run_and_stream(
        down_cmd,
        cwd=PROJECT_ROOT,
        env=_compose_env(ctx),
        log=log,
        printer=printer,
    )


def wait_for_gateway(
    env_name: str,
    *,
    ports: dict[str, int],
    timeout: float = 60.0,
    interval: float = 1.0,
) -> None:
    gw_port = ports.get(env_key(constants.PORT_GATEWAY_HTTPS))
    if not gw_port:
        return

    url = f"https://localhost:{gw_port}/health"
    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

    deadline = time.time() + timeout
    last_err = None
    while time.time() < deadline:
        try:
            with requests.Session() as session:
                session.trust_env = False
                response = session.get(url, timeout=2.0, verify=False)
            if response.status_code == 200:
                return
            last_err = f"Status code {response.status_code}"
        except requests.exceptions.RequestException as exc:
            last_err = str(exc)
        time.sleep(interval)
    raise RuntimeError(
        f"Gateway failed to start in time ({timeout}s) for {env_name}. Last error: {last_err}"
    )


def _compose_env(ctx: RunContext) -> dict[str, str]:
    compose_env = os.environ.copy()
    compose_env.update(ctx.runtime_env)
    compose_env.setdefault(constants.ENV_PROJECT_NAME, ctx.compose_project)
    return {**compose_env}


def _compose_base_cmd(
    *,
    project_name: str,
    compose_file: Path,
    env_file: str | None,
) -> list[str]:
    cmd = [
        "docker",
        "compose",
        "--project-name",
        project_name,
        "--file",
        str(compose_file),
    ]
    env_file_path = resolve_env_file_path(env_file)
    if env_file_path:
        cmd.extend(["--env-file", env_file_path])
    return cmd
