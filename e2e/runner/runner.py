# Where: e2e/runner/runner.py
# What: Orchestrates E2E runs per environment and in parallel.
# Why: Separate planning/execution from CLI entrypoint and UI.
from __future__ import annotations

import os
import socket
import subprocess
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Callable

from e2e.runner import constants, infra
from e2e.runner.buildx import ensure_buildx_builder
from e2e.runner.deploy import deploy_templates
from e2e.runner.env import (
    apply_gateway_env_from_container,
    calculate_runtime_env,
    calculate_staging_dir,
    discover_ports,
    read_env_file,
)
from e2e.runner.events import (
    EVENT_ENV_END,
    EVENT_ENV_START,
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
    Event,
)
from e2e.runner.lifecycle import (
    compose_down,
    compose_up,
    reset_environment,
    resolve_compose_file,
    wait_for_gateway,
)
from e2e.runner.logging import LogSink, make_prefix_printer
from e2e.runner.models import RunContext, Scenario
from e2e.runner.tester import run_pytest
from e2e.runner.ui import Reporter
from e2e.runner.utils import (
    BRAND_SLUG,
    E2E_STATE_ROOT,
    PROJECT_ROOT,
    build_unique_tag,
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


def run_parallel(
    scenarios: dict[str, Scenario],
    *,
    reporter: Reporter,
    parallel: bool,
    args,
) -> dict[str, bool]:
    reporter.start()
    reporter.emit(Event(EVENT_SUITE_START))
    results: dict[str, bool] = {}
    try:
        _warmup()
        infra_printer = make_prefix_printer("infra") if args.verbose else None
        infra.ensure_infra_up(str(PROJECT_ROOT), printer=infra_printer)
        reporter.emit(Event(EVENT_REGISTRY_READY))

        if not scenarios:
            return results

        max_workers = len(scenarios) if parallel else 1
        port_plan = _allocate_ports(list(scenarios.keys()))
        lock = threading.Lock()

        def _run(env_name: str, scenario: Scenario) -> bool:
            log_path = PROJECT_ROOT / "e2e" / f".parallel-{env_name}.log"
            log = LogSink(log_path)
            log.open()
            printer = make_prefix_printer(env_name) if args.verbose else None
            try:
                ctx = _prepare_context(scenario, port_plan.get(env_name))
                success = _run_env(ctx, reporter, log, printer, args)
                return success
            finally:
                log.close()

        with ThreadPoolExecutor(max_workers=max_workers) as executor:
            future_to_env = {
                executor.submit(_run, env_name, scenario): env_name
                for env_name, scenario in scenarios.items()
            }
            for future in as_completed(future_to_env):
                env_name = future_to_env[future]
                try:
                    success = future.result()
                except Exception:
                    success = False
                with lock:
                    results[env_name] = success
                if args.fail_fast and not success:
                    for pending in future_to_env:
                        pending.cancel()
                    break
        return results
    finally:
        reporter.emit(Event(EVENT_SUITE_END))
        reporter.close()


def _prepare_context(
    scenario: Scenario,
    port_overrides: dict[str, str] | None = None,
) -> RunContext:
    env_name = scenario.env_name
    project_name = scenario.project_name or BRAND_SLUG
    compose_project = f"{project_name}-{env_name}"
    env_file = _resolve_env_file(scenario.env_file)

    compose_file = resolve_compose_file(scenario)
    template_path = None
    if scenario.deploy_templates:
        template_path = (PROJECT_ROOT / scenario.deploy_templates[0]).resolve()
    else:
        template_path = (PROJECT_ROOT / "e2e" / "fixtures" / "template.yaml").resolve()

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

    runtime_env["ESB_BUILDX_CACHE"] = "0"

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
    deploy_env["ESB_META_REUSE"] = "1"
    deploy_env.update(scenario.env_vars)

    pytest_env = os.environ.copy()
    pytest_env.update(runtime_env)
    pytest_env.update(scenario.env_vars)

    ensure_buildx_builder(
        runtime_env.get("BUILDX_BUILDER", ""),
        config_path=runtime_env.get(constants.ENV_BUILDKITD_CONFIG, ""),
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


def _run_env(
    ctx: RunContext,
    reporter: Reporter,
    log: LogSink,
    printer: Callable[[str], None] | None,
    args,
) -> bool:
    reporter.emit(Event(EVENT_ENV_START, env=ctx.scenario.env_name))
    try:
        if args.test_only:
            reporter.emit(Event(EVENT_PHASE_SKIP, env=ctx.scenario.env_name, phase=PHASE_RESET))
            reporter.emit(Event(EVENT_PHASE_SKIP, env=ctx.scenario.env_name, phase=PHASE_COMPOSE))
            reporter.emit(Event(EVENT_PHASE_SKIP, env=ctx.scenario.env_name, phase=PHASE_DEPLOY))
        else:
            _phase(
                reporter,
                ctx,
                PHASE_RESET,
                log,
                printer,
                lambda: reset_environment(ctx, log=log, printer=printer),
            )

            _phase(
                reporter,
                ctx,
                PHASE_COMPOSE,
                log,
                printer,
                lambda: _compose_and_wait(ctx, log, printer, args),
            )

            _phase(
                reporter,
                ctx,
                PHASE_DEPLOY,
                log,
                printer,
                lambda: _deploy(ctx, log, printer, args),
            )

        if args.build_only:
            reporter.emit(Event(EVENT_PHASE_SKIP, env=ctx.scenario.env_name, phase=PHASE_TEST))
            reporter.emit(
                Event(
                    EVENT_ENV_END,
                    env=ctx.scenario.env_name,
                    data={"status": STATUS_PASSED},
                )
            )
            return True

        if ctx.scenario.targets:
            _phase(
                reporter,
                ctx,
                PHASE_TEST,
                log,
                printer,
                lambda: _test(ctx, reporter, log, printer),
            )
        else:
            reporter.emit(Event(EVENT_PHASE_SKIP, env=ctx.scenario.env_name, phase=PHASE_TEST))

        reporter.emit(
            Event(
                EVENT_ENV_END,
                env=ctx.scenario.env_name,
                data={"status": STATUS_PASSED},
            )
        )
        return True
    except Exception as exc:
        log.write_line(f"[ERROR] {exc}")
        reporter.emit(
            Event(EVENT_ENV_END, env=ctx.scenario.env_name, data={"status": STATUS_FAILED})
        )
        return False
    finally:
        if args.cleanup:
            compose_down(ctx, log=log, printer=printer)


def _compose_and_wait(ctx: RunContext, log: LogSink, printer, args) -> None:
    build = args.build or _needs_compose_build()
    if build and not args.build:
        log.write_line("Auto-enabling compose build (base images missing).")
        if printer:
            printer("Auto-enabling compose build (base images missing).")
    ctx.ports = compose_up(ctx, build=build, log=log, printer=printer)
    if ctx.ports:
        _apply_ports_to_env_dict(ctx.ports, ctx.pytest_env)
    wait_for_gateway(ctx.scenario.env_name, ports=ctx.ports)
    _sync_gateway_env(ctx)


def _deploy(ctx: RunContext, log: LogSink, printer, args) -> None:
    templates = _resolve_templates(ctx.scenario)
    deploy_templates(
        ctx,
        templates,
        no_cache=args.no_cache,
        verbose=args.verbose,
        log=log,
        printer=printer,
    )


def _test(ctx: RunContext, reporter: Reporter, log: LogSink, printer) -> None:
    if not ctx.ports:
        ctx.ports = discover_ports(ctx.compose_project, ctx.compose_file)
        if ctx.ports:
            _apply_ports_to_env_dict(ctx.ports, ctx.pytest_env)
        wait_for_gateway(ctx.scenario.env_name, ports=ctx.ports)
    _sync_gateway_env(ctx)

    def _progress(current: int, total: int | None) -> None:
        reporter.emit(
            Event(
                EVENT_PHASE_PROGRESS,
                env=ctx.scenario.env_name,
                phase=PHASE_TEST,
                data={"current": current, "total": total},
            )
        )

    run_pytest(ctx, log=log, printer=printer, on_progress=_progress)


def _phase(
    reporter: Reporter,
    ctx: RunContext,
    phase: str,
    log: LogSink,
    printer: Callable[[str], None] | None,
    fn: Callable[[], None],
) -> None:
    reporter.emit(Event(EVENT_PHASE_START, env=ctx.scenario.env_name, phase=phase))
    started = time.monotonic()
    try:
        fn()
    except Exception:
        duration = time.monotonic() - started
        reporter.emit(
            Event(
                EVENT_PHASE_END,
                env=ctx.scenario.env_name,
                phase=phase,
                data={"status": STATUS_FAILED, "duration": duration},
            )
        )
        raise
    duration = time.monotonic() - started
    reporter.emit(
        Event(
            EVENT_PHASE_END,
            env=ctx.scenario.env_name,
            phase=phase,
            data={"status": STATUS_PASSED, "duration": duration},
        )
    )


def _warmup() -> None:
    template = PROJECT_ROOT / "e2e" / "fixtures" / "template.yaml"
    if not template.exists():
        raise FileNotFoundError(f"Missing E2E template: {template}")


def _resolve_env_file(env_file: str | None) -> str | None:
    if not env_file:
        return None
    path = Path(env_file)
    if not path.is_absolute():
        path = PROJECT_ROOT / path
    return str(path.absolute())


def _resolve_templates(scenario: Scenario) -> list[Path]:
    if scenario.deploy_templates:
        return [Path(t).resolve() for t in scenario.deploy_templates]
    return [PROJECT_ROOT / "e2e" / "fixtures" / "template.yaml"]


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


def _needs_compose_build() -> bool:
    images = [
        f"{BRAND_SLUG}-os-base:latest",
        f"{BRAND_SLUG}-python-base:latest",
    ]
    for image in images:
        result = subprocess.run(
            ["docker", "image", "inspect", image],
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            return True
    return False


def _allocate_ports(env_names: list[str]) -> dict[str, dict[str, str]]:
    base = constants.E2E_PORT_BASE
    block = constants.E2E_PORT_BLOCK
    offsets = {
        constants.PORT_GATEWAY_HTTPS: 0,
        constants.PORT_GATEWAY_HTTP: 1,
        constants.PORT_AGENT_GRPC: 2,
        constants.PORT_AGENT_METRICS: 3,
        constants.PORT_VICTORIALOGS: 4,
        constants.PORT_DATABASE: 5,
        constants.PORT_S3: 6,
        constants.PORT_S3_MGMT: 7,
    }
    env_names_sorted = sorted(env_names)

    def _port_available(port: int) -> bool:
        # Bind to 0.0.0.0 so we catch conflicts with services bound to all interfaces.
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            try:
                sock.bind(("0.0.0.0", port))
            except OSError:
                return False
        return True

    # Prefer stable port blocks, but move the whole block-window up if any port
    # is already in use on the host.
    group_size = len(env_names_sorted)
    for shift in range(0, 200):
        bases: dict[str, int] = {}
        ok = True
        for idx, env_name in enumerate(env_names_sorted):
            env_base = base + (idx + shift * group_size) * block
            if env_base + max(offsets.values()) >= 65535:
                ok = False
                break
            ports = [env_base + offset for offset in offsets.values()]
            if not all(_port_available(port) for port in ports):
                ok = False
                break
            bases[env_name] = env_base
        if not ok:
            continue

        plan: dict[str, dict[str, str]] = {}
        for env_name, env_base in bases.items():
            env_ports: dict[str, str] = {}
            for key, offset in offsets.items():
                env_ports[env_key(key)] = str(env_base + offset)
            plan[env_name] = env_ports
        return plan

    raise RuntimeError(
        "Failed to allocate a free host port block for E2E. "
        f"base={base} block={block} envs={env_names_sorted}"
    )


def _apply_port_overrides(runtime_env: dict[str, str], overrides: dict[str, str] | None) -> None:
    if not overrides:
        return
    for key, value in overrides.items():
        current = runtime_env.get(key)
        if not current or current == "0":
            runtime_env[key] = value
