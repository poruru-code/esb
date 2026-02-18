# Where: e2e/runner/runner.py
# What: Orchestrates E2E runs per environment and in parallel.
# Why: Keep execution flow centralized in runner.py (legacy executor.py removed).
from __future__ import annotations

import subprocess
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Callable

from e2e.runner import infra
from e2e.runner.context import (
    _apply_ports_to_env_dict,
    _prepare_context,
    _sync_gateway_env,
)
from e2e.runner.deploy import deploy_artifacts
from e2e.runner.env import discover_ports
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
from e2e.runner.lifecycle import compose_down, compose_up, reset_environment, wait_for_gateway
from e2e.runner.live_display import LiveDisplay
from e2e.runner.logging import LogSink, make_prefix_printer
from e2e.runner.models import RunContext, Scenario
from e2e.runner.ports import _allocate_ports
from e2e.runner.tester import run_pytest
from e2e.runner.ui import Reporter
from e2e.runner.utils import BRAND_SLUG, PROJECT_ROOT
from e2e.runner.warmup import _warmup


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
    reporter.emit(Event(EVENT_SUITE_START))
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
        missing_envs = [env for env in scenarios if env not in results]
        failed_envs = sorted([env for env, ok in results.items() if not ok] + missing_envs)
        suite_status = STATUS_PASSED if not failed_envs else STATUS_FAILED
        if live_display:
            live_display.stop()
        reporter.emit(
            Event(
                EVENT_SUITE_END,
                data={
                    "status": suite_status,
                    "failed_envs": failed_envs,
                },
            )
        )
        reporter.close()


def _run_env(
    ctx: RunContext,
    reporter: Reporter,
    log: LogSink,
    phase_printer: Callable[[str], Callable[[str], None] | None] | None,
    args,
) -> bool:
    env_name = ctx.scenario.env_name
    reporter.emit(Event(EVENT_ENV_START, env=env_name))
    log.write_line(f"[{env_name}] started")
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
    deploy_artifacts(
        ctx,
        no_cache=args.no_cache,
        log=log,
        printer=printer,
    )


def _test(ctx: RunContext, reporter: Reporter, log: LogSink, printer) -> None:
    if not ctx.ports:
        ctx.ports = discover_ports(ctx.compose_project, ctx.compose_file, env_file=ctx.env_file)
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


def _emit_env_end(
    reporter: Reporter,
    log: LogSink,
    env_name: str,
    status: str,
) -> None:
    log.write_line(f"[{env_name}] done ... {status.upper()}")
    reporter.emit(
        Event(
            EVENT_ENV_END,
            env=env_name,
            data={"status": status},
        )
    )


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
