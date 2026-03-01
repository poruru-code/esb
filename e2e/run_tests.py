#!/usr/bin/env python3
# Where: e2e/run_tests.py
# What: E2E test runner for artifact-based scenarios.
# Why: Provide a single entry point for scenario setup, execution, and teardown.
import os
import shutil
import subprocess
import sys
import warnings

import urllib3

from e2e.runner.cli import parse_args
from e2e.runner.config import load_test_matrix
from e2e.runner.ctl_contract import (
    CTL_REQUIRED_SUBCOMMANDS,
    DEFAULT_CTL_BIN,
    ENV_CTL_BIN,
    ENV_CTL_BIN_RESOLVED,
    configured_ctl_bin_from_env,
)
from e2e.runner.live_display import LiveDisplay
from e2e.runner.planner import apply_test_target, build_plan
from e2e.runner.proxy_harness import ProxyHarnessError, options_from_args, proxy_harness
from e2e.runner.runner import run_parallel
from e2e.runner.ui import PlainReporter
from e2e.runner.utils import PROJECT_ROOT

# Canonical E2E execution path: e2e.runner.runner (legacy executor removed).


def print_tail_logs(failed_entries: list[str], *, lines: int = 40) -> None:
    for env_name in failed_entries:
        log_path = PROJECT_ROOT / "e2e" / f".parallel-{env_name}.log"
        if not log_path.exists():
            print(f"[PARALLEL] No log file found for {env_name}: {log_path}")
            continue
        print(f"\n[PARALLEL] Last {lines} lines for {env_name} ({log_path}):")
        try:
            with log_path.open("r", encoding="utf-8") as f:
                content = f.read().splitlines()
        except OSError as exc:
            print(f"[PARALLEL] Failed to read log for {env_name}: {exc}")
            continue
        tail = content[-lines:] if len(content) > lines else content
        for line in tail:
            print(line)
        if hint := detect_public_ecr_hint(content):
            print(f"[HINT] {hint}")


def detect_public_ecr_hint(lines: list[str]) -> str:
    if not lines:
        return ""
    normalized = "\n".join(lines).lower()
    if "public.ecr.aws" not in normalized:
        return ""
    if "403" in normalized or "forbidden" in normalized or "unauthorized" in normalized:
        return (
            "public.ecr.aws denied the request. Docker credentials may be stale. "
            "Try `docker logout public.ecr.aws` and retry, or run "
            "`docker login public.ecr.aws` with valid credentials."
        )
    return ""


def resolve_env_label_width(env_scenarios: dict[str, object]) -> int:
    if not env_scenarios:
        return 0
    return max(len(env) for env in env_scenarios.keys())


def resolve_live_enabled(no_live: bool) -> bool:
    if no_live:
        return False
    if not sys.stdout.isatty():
        return False
    term = os.environ.get("TERM", "")
    if term.lower() == "dumb":
        return False
    if os.environ.get("NO_COLOR") or os.environ.get("NO_EMOJI"):
        return False
    return True


def requires_ctl(args, env_scenarios: dict[str, object]) -> bool:
    if args.test_only:
        return False
    return bool(env_scenarios)


def ensure_ctl_available() -> str:
    command_name = DEFAULT_CTL_BIN

    def _print_build_hint() -> None:
        print("        In this repository, run: mise run build-ctl")

    def _assert_supported(binary_path: str) -> None:
        for subcommand in CTL_REQUIRED_SUBCOMMANDS:
            probe = subprocess.run(
                [binary_path, *subcommand],
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                check=False,
            )
            out = (probe.stdout or "").lower()
            if probe.returncode != 0 or "unknown command" in out:
                command = " ".join(subcommand)
                print(f"[ERROR] {command_name} binary does not support `{command}`: {binary_path}")
                print(
                    f"        Ensure a current {command_name} is installed or set {ENV_CTL_BIN} explicitly."
                )
                _print_build_hint()
                sys.exit(1)

    override = configured_ctl_bin_from_env(os.environ)
    if override:
        resolved = shutil.which(override)
        if resolved is None:
            print(f"[ERROR] {ENV_CTL_BIN} is set but not executable: {override}")
            sys.exit(1)
        resolved_abs = os.path.abspath(resolved)
        _assert_supported(resolved_abs)
        os.environ[ENV_CTL_BIN_RESOLVED] = resolved_abs
        return resolved_abs

    preferred_local = os.path.expanduser(f"~/.local/bin/{command_name}")
    if os.path.isfile(preferred_local) and os.access(preferred_local, os.X_OK):
        resolved_abs = os.path.abspath(preferred_local)
        _assert_supported(resolved_abs)
        os.environ[ENV_CTL_BIN_RESOLVED] = resolved_abs
        return resolved_abs

    resolved = shutil.which(command_name)
    if resolved is not None:
        resolved_abs = os.path.abspath(resolved)
        _assert_supported(resolved_abs)
        os.environ[ENV_CTL_BIN_RESOLVED] = resolved_abs
        return resolved_abs

    print(f"[ERROR] {command_name} binary not found in PATH.")
    print(f"        Install {command_name} or set {ENV_CTL_BIN} to an executable path.")
    print("        In this repository, you can install it via: mise run setup")
    print(f"        Example: {ENV_CTL_BIN}=/path/to/{command_name} uv run e2e/run_tests.py ...")
    sys.exit(1)


def main():
    # Suppress warnings.
    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    warnings.filterwarnings("ignore", category=DeprecationWarning)

    args = parse_args()
    proxy_options = options_from_args(args)

    try:
        with proxy_harness(proxy_options):
            # --- Load Test Matrix ---
            config_matrix = load_test_matrix()
            suites = config_matrix.get("suites", {})
            matrix = config_matrix.get("matrix", [])

            if args.build_only and args.test_only:
                print("[ERROR] --build-only and --test-only cannot be used together.")
                sys.exit(1)

            if args.test_target and (args.build_only or args.test_only):
                print("[ERROR] --build-only/--test-only cannot be used with --test-target.")
                sys.exit(1)

            # --- Single Target Mode (Legacy/Debug) ---
            if args.test_target:
                if not args.profile:
                    print("[ERROR] --profile is required when using --test-target.")
                    env_names = [entry.get("env") for entry in matrix if entry.get("env")]
                    print(f"Available environments: {', '.join(env_names)}")
                    sys.exit(1)

                entry_by_env = {entry.get("env"): entry for entry in matrix if entry.get("env")}
                if args.profile not in entry_by_env:
                    print(f"[ERROR] Environment '{args.profile}' not found in matrix.")
                    sys.exit(1)

                # Single-target execution uses the planner path with one environment.
                env_scenarios = build_plan(matrix, suites, profile_filter=args.profile)
                env_scenarios = apply_test_target(
                    env_scenarios,
                    env_name=args.profile,
                    target=args.test_target,
                )
                if not env_scenarios:
                    print(f"[ERROR] Environment '{args.profile}' not found in matrix.")
                    sys.exit(1)

                env_label_width = resolve_env_label_width(env_scenarios)
                reporter = PlainReporter(
                    verbose=args.verbose,
                    env_label_width=env_label_width,
                    color=args.color,
                    emoji=args.emoji,
                    show_progress=True,
                )
                if requires_ctl(args, env_scenarios):
                    ensure_ctl_available()
                results = run_parallel(
                    env_scenarios,
                    reporter=reporter,
                    parallel=False,
                    args=args,
                    env_label_width=env_label_width,
                    live_display=None,
                )
                failed = [env for env, ok in results.items() if not ok]
                if failed:
                    print_tail_logs(failed)
                    sys.exit(1)
                sys.exit(0)

            env_scenarios = build_plan(matrix, suites, profile_filter=args.profile)

            if args.build_only or args.test_only:
                if not args.profile:
                    print("[ERROR] --profile is required when using --build-only/--test-only.")
                    sys.exit(1)
                if args.profile not in env_scenarios:
                    print(f"[ERROR] Environment '{args.profile}' not found in matrix.")
                    sys.exit(1)

                env_label_width = resolve_env_label_width(env_scenarios)
                reporter = PlainReporter(
                    verbose=args.verbose,
                    env_label_width=env_label_width,
                    color=args.color,
                    emoji=args.emoji,
                    show_progress=True,
                )
                if requires_ctl(args, env_scenarios):
                    ensure_ctl_available()
                results = run_parallel(
                    env_scenarios,
                    reporter=reporter,
                    parallel=False,
                    args=args,
                    env_label_width=env_label_width,
                    live_display=None,
                )
                failed = [env for env, ok in results.items() if not ok]
                if failed:
                    print_tail_logs(failed)
                    sys.exit(1)
                sys.exit(0)

            parallel_mode = args.parallel and len(env_scenarios) > 1
            live_enabled = resolve_live_enabled(args.no_live) and parallel_mode and not args.verbose
            env_label_width = resolve_env_label_width(env_scenarios)
            live_display = (
                LiveDisplay(list(env_scenarios.keys()), label_width=env_label_width)
                if live_enabled
                else None
            )
            reporter = PlainReporter(
                verbose=args.verbose,
                env_label_width=env_label_width,
                color=args.color,
                emoji=args.emoji,
                live_display=live_display,
                show_progress=not (live_display and not args.verbose),
            )
            if requires_ctl(args, env_scenarios):
                ensure_ctl_available()
            results = run_parallel(
                env_scenarios,
                reporter=reporter,
                parallel=parallel_mode,
                args=args,
                env_label_width=env_label_width,
                live_display=live_display,
            )
            failed_entries = [env for env, ok in results.items() if not ok]

            if failed_entries:
                print_tail_logs(failed_entries)
                sys.exit(1)

            sys.exit(0)
    except ProxyHarnessError as exc:
        print(f"[proxy-e2e] ERROR: {exc}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
