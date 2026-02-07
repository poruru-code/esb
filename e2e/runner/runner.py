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
from datetime import datetime
from pathlib import Path
from typing import Callable

import yaml

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
from e2e.runner.live_display import LiveDisplay
from e2e.runner.logging import LogSink, make_prefix_printer
from e2e.runner.models import RunContext, Scenario
from e2e.runner.tester import run_pytest
from e2e.runner.ui import Reporter
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


def run_parallel(
    scenarios: dict[str, Scenario],
    *,
    reporter: Reporter,
    parallel: bool,
    args,
    env_label_width: int | None = None,
    live_display: LiveDisplay | None = None,
) -> dict[str, bool]:
    reporter.start()
    reporter.emit(Event(EVENT_SUITE_START, data={"wall_time": time.time()}))
    results: dict[str, bool] = {}
    try:
        label_width = env_label_width
        if label_width is None:
            label_width = max((len(env) for env in scenarios.keys()), default=0)

        def format_prefix(label: str, phase: str | None = None) -> str:
            formatted = label.ljust(label_width) if label_width > 0 else label
            if phase:
                return f"[{formatted}][{phase}] |"
            return f"[{formatted}]"

        def make_system_printer(
            label: str,
            phase: str | None = None,
            *,
            always: bool = False,
        ) -> Callable[[str], None] | None:
            if not always and not args.verbose and not live_display:
                return None
            if live_display:
                prefix = format_prefix(label, phase)
                return lambda line: live_display.log_line(f"{prefix} {line}")
            return make_prefix_printer(label, phase, width=label_width)

        def make_env_printer(
            env_name: str,
            phase: str | None = None,
        ) -> Callable[[str], None] | None:
            if not args.verbose and not live_display:
                return None
            if live_display:
                prefix = format_prefix(env_name, phase)
                if args.verbose:
                    return lambda line: live_display.log_line(f"{prefix} {line}")
                if phase == PHASE_TEST:
                    return lambda line: live_display.update_line(env_name, f"{prefix} {line}")
                return lambda line: live_display.update_line(env_name, f"{prefix} {line}")
            return make_prefix_printer(env_name, phase, width=label_width)

        if not args.test_only:
            warmup_printer = make_system_printer("warmup", always=True)
            _warmup(scenarios, printer=warmup_printer, verbose=args.verbose)
        infra_printer = make_system_printer("infra")
        infra.ensure_infra_up(str(PROJECT_ROOT), printer=infra_printer)
        reporter.emit(Event(EVENT_REGISTRY_READY))

        if not scenarios:
            return results

        if live_display:
            live_display.start()

        max_workers = len(scenarios) if parallel else 1
        port_plan = _allocate_ports(list(scenarios.keys()))
        lock = threading.Lock()

        def _run(env_name: str, scenario: Scenario) -> bool:
            log_path = PROJECT_ROOT / "e2e" / f".parallel-{env_name}.log"
            log = LogSink(log_path)
            log.open()

            def phase_printer(phase: str) -> Callable[[str], None] | None:
                return make_env_printer(env_name, phase)

            try:
                ctx = _prepare_context(scenario, port_plan.get(env_name))
                success = _run_env(ctx, reporter, log, phase_printer, args)
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
        reporter.emit(Event(EVENT_SUITE_END, data={"wall_time": time.time()}))
        if live_display:
            live_display.stop()
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
    phase_printer: Callable[[str], Callable[[str], None] | None] | None,
    args,
) -> bool:
    env_name = ctx.scenario.env_name
    start_wall = time.time()
    reporter.emit(Event(EVENT_ENV_START, env=env_name, data={"wall_time": start_wall}))
    log.write_line(f"[{env_name}] started @ {_format_wall_time(start_wall)}")
    try:

        def _printer_for(phase: str) -> Callable[[str], None] | None:
            if phase_printer is None:
                return None
            return phase_printer(phase)

        if args.test_only:
            reporter.emit(Event(EVENT_PHASE_SKIP, env=env_name, phase=PHASE_RESET))
            reporter.emit(Event(EVENT_PHASE_SKIP, env=env_name, phase=PHASE_COMPOSE))
            reporter.emit(Event(EVENT_PHASE_SKIP, env=env_name, phase=PHASE_DEPLOY))
        else:
            reset_printer = _printer_for(PHASE_RESET)
            _phase(
                reporter,
                ctx,
                PHASE_RESET,
                log,
                reset_printer,
                lambda: reset_environment(ctx, log=log, printer=reset_printer),
            )

            compose_printer = _printer_for(PHASE_COMPOSE)
            _phase(
                reporter,
                ctx,
                PHASE_COMPOSE,
                log,
                compose_printer,
                lambda: _compose_and_wait(ctx, log, compose_printer, args),
            )

            deploy_printer = _printer_for(PHASE_DEPLOY)
            _phase(
                reporter,
                ctx,
                PHASE_DEPLOY,
                log,
                deploy_printer,
                lambda: _deploy(ctx, log, deploy_printer, args),
            )

        if args.build_only:
            reporter.emit(Event(EVENT_PHASE_SKIP, env=env_name, phase=PHASE_TEST))
            _emit_env_end(reporter, log, env_name, STATUS_PASSED)
            return True

        if ctx.scenario.targets:
            test_printer = _printer_for(PHASE_TEST)
            _phase(
                reporter,
                ctx,
                PHASE_TEST,
                log,
                test_printer,
                lambda: _test(ctx, reporter, log, test_printer),
            )
        else:
            reporter.emit(Event(EVENT_PHASE_SKIP, env=env_name, phase=PHASE_TEST))

        _emit_env_end(reporter, log, env_name, STATUS_PASSED)
        return True
    except Exception as exc:
        log.write_line(f"[ERROR] {exc}")
        _emit_env_end(reporter, log, env_name, STATUS_FAILED)
        return False
    finally:
        if args.cleanup:
            cleanup_printer = _printer_for("cleanup")
            compose_down(ctx, log=log, printer=cleanup_printer)


def _compose_and_wait(ctx: RunContext, log: LogSink, printer, args) -> None:
    build = args.build or args.build_only or _needs_compose_build()
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


def _warmup(
    scenarios: dict[str, Scenario],
    *,
    printer: Callable[[str], None] | None = None,
    verbose: bool = False,
) -> None:
    templates = _collect_templates(scenarios)
    missing_templates = [template for template in templates if not template.exists()]
    if missing_templates:
        missing = ", ".join(str(template) for template in missing_templates)
        raise FileNotFoundError(f"Missing E2E template(s): {missing}")
    if _uses_java_templates(scenarios):
        _emit_warmup(printer, f"Java fixture warmup ... start @ {_format_wall_time(time.time())}")
        _build_java_fixtures(printer=printer, verbose=verbose)
        _emit_warmup(printer, f"Java fixture warmup ... done  @ {_format_wall_time(time.time())}")


def _uses_java_templates(scenarios: dict[str, Scenario]) -> bool:
    runtime_extensions = PROJECT_ROOT / "runtime" / "java" / "extensions"
    if not runtime_extensions.exists():
        return False
    for template in _collect_templates(scenarios):
        if _template_has_java_runtime(template):
            return True
    return False


def _collect_templates(scenarios: dict[str, Scenario]) -> list[Path]:
    templates: set[Path] = set()
    for scenario in scenarios.values():
        templates.update(_resolve_templates(scenario))
    return sorted(templates)


def _resolve_template_path(path: Path) -> Path:
    if path.is_absolute():
        return path
    return (PROJECT_ROOT / path).resolve()


def _template_has_java_runtime(path: Path) -> bool:
    if not path.exists():
        return False
    try:
        data = yaml.load(path.read_text(encoding="utf-8"), Loader=_YamlIgnoreTagsLoader)
    except (OSError, yaml.YAMLError):
        return False
    if not isinstance(data, dict):
        return False
    if _globals_runtime_is_java(data):
        return True
    resources = data.get("Resources")
    if not isinstance(resources, dict):
        return False
    for resource in resources.values():
        if not isinstance(resource, dict):
            continue
        props = resource.get("Properties")
        if not isinstance(props, dict):
            continue
        runtime = str(props.get("Runtime", "")).lower().strip()
        if runtime.startswith("java"):
            return True
        code_uri = props.get("CodeUri", "")
        if isinstance(code_uri, str):
            if "functions/java/" in code_uri or code_uri.lower().endswith(".jar"):
                return True
    return False


def _globals_runtime_is_java(payload: dict) -> bool:
    globals_section = payload.get("Globals")
    if not isinstance(globals_section, dict):
        return False
    function_globals = globals_section.get("Function")
    if not isinstance(function_globals, dict):
        return False
    runtime = str(function_globals.get("Runtime", "")).lower().strip()
    return runtime.startswith("java")


class _YamlIgnoreTagsLoader(yaml.SafeLoader):
    pass


def _yaml_ignore_unknown_tags(loader, node):
    if isinstance(node, yaml.ScalarNode):
        return loader.construct_scalar(node)
    if isinstance(node, yaml.SequenceNode):
        return loader.construct_sequence(node)
    if isinstance(node, yaml.MappingNode):
        return loader.construct_mapping(node)
    return None


_YamlIgnoreTagsLoader.add_constructor(None, _yaml_ignore_unknown_tags)


def _build_java_fixtures(
    *,
    printer: Callable[[str], None] | None = None,
    verbose: bool = False,
) -> None:
    fixtures_dir = PROJECT_ROOT / "e2e" / "fixtures" / "functions" / "java"
    if not fixtures_dir.exists():
        return

    for project_dir in sorted(p for p in fixtures_dir.iterdir() if p.is_dir()):
        pom = project_dir / "pom.xml"
        if not pom.exists():
            continue
        if printer and verbose:
            printer(f"Building Java fixture: {project_dir.name}")
        _build_java_project(project_dir, verbose=verbose)


def _build_java_project(project_dir: Path, *, verbose: bool = False) -> None:
    cmd = _docker_maven_command(project_dir)
    result = subprocess.run(
        cmd,
        capture_output=not verbose,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        details = ""
        if not verbose:
            details = f"\n{result.stdout}\n{result.stderr}".rstrip()
        raise RuntimeError(f"Java fixture build failed: {project_dir}{details}")

    jar_path = project_dir / "app.jar"
    if not jar_path.exists():
        raise RuntimeError(f"Java fixture jar not found in {project_dir}")


def _docker_maven_command(project_dir: Path) -> list[str]:
    cmd = [
        "docker",
        "run",
        "--rm",
    ]
    getuid = getattr(os, "getuid", None)
    getgid = getattr(os, "getgid", None)
    if callable(getuid) and callable(getgid):
        cmd.extend(["--user", f"{getuid()}:{getgid()}"])
    cmd.extend(["-v", f"{project_dir}:/src:ro", "-v", f"{project_dir}:/out"])
    home_dir = Path.home()
    m2_dir = home_dir / ".m2"
    if m2_dir.exists() and os.access(m2_dir, os.W_OK):
        cmd.extend(["-v", f"{m2_dir}:/tmp/m2"])
    cmd.extend(["-e", "MAVEN_CONFIG=/tmp/m2", "-e", "HOME=/tmp"])
    for key in (
        "HTTP_PROXY",
        "HTTPS_PROXY",
        "NO_PROXY",
        "http_proxy",
        "https_proxy",
        "no_proxy",
        "MAVEN_OPTS",
        "JAVA_TOOL_OPTIONS",
    ):
        value = os.environ.get(key)
        if value:
            cmd.extend(["-e", f"{key}={value}"])
    script = "\n".join(
        [
            "set -euo pipefail",
            "mkdir -p /tmp/work /tmp/m2 /out",
            "cp -a /src/. /tmp/work",
            "cd /tmp/work",
            "mvn -q -DskipTests package",
            "jar=$(ls -S target/*.jar 2>/dev/null | "
            "grep -vE '(-sources|-javadoc)\\.jar$' | head -n 1 || true)",
            'if [ -z "$jar" ]; then echo "jar not found in target" >&2; exit 1; fi',
            'cp "$jar" /out/app.jar',
        ]
    )
    cmd.extend(
        [
            "maven:3.9.6-eclipse-temurin-21",
            "bash",
            "-lc",
            script,
        ]
    )
    return cmd


def _emit_warmup(printer: Callable[[str], None] | None, message: str) -> None:
    if printer:
        printer(message)
    else:
        print(message)


def _format_wall_time(timestamp: float) -> str:
    return datetime.fromtimestamp(timestamp).astimezone().strftime("%Y-%m-%d %H:%M:%S")


def _emit_env_end(
    reporter: Reporter,
    log: LogSink,
    env_name: str,
    status: str,
) -> None:
    end_wall = time.time()
    log.write_line(f"[{env_name}] done ... {status.upper()} @ {_format_wall_time(end_wall)}")
    reporter.emit(
        Event(
            EVENT_ENV_END,
            env=env_name,
            data={"status": status, "wall_time": end_wall},
        )
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
