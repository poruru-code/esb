#!/usr/bin/env python3
# Where: e2e/run_tests.py
# What: E2E test runner for ESB CLI scenarios.
# Why: Provide a single entry point for scenario setup, execution, and teardown.
import os
import subprocess
import sys
import warnings

import urllib3

from e2e.runner.cli import parse_args
from e2e.runner.config import load_test_matrix
from e2e.runner.live_display import LiveDisplay
from e2e.runner.planner import apply_test_target, build_plan
from e2e.runner.runner import run_parallel
from e2e.runner.ui import PlainReporter
from e2e.runner.utils import GO_CLI_ROOT, PROJECT_ROOT


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
            "Try `docker logout public.ecr.aws` and retry, or login via "
            "`aws ecr-public get-login-password --region us-east-1 | "
            "docker login --username AWS --password-stdin public.ecr.aws`."
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


def main():
    # Suppress warnings.
    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    warnings.filterwarnings("ignore", category=DeprecationWarning)

    args = parse_args()

    # --- Unit Tests ---
    if args.unit or args.unit_only:
        print("\n=== Running Python Unit Tests ===\n")
        python_cmd = [
            sys.executable,
            "-m",
            "pytest",
            "services/gateway/tests",
            "-v",
        ]
        res = subprocess.run(python_cmd, cwd=PROJECT_ROOT, check=False)
        if res.returncode != 0:
            print("\n[FAILED] Python unit tests failed.")
            sys.exit(res.returncode)

        print("\n=== Running Go Unit Tests ===\n")
        go_cmd = ["go", "test", "./..."]
        go_res = subprocess.run(go_cmd, cwd=GO_CLI_ROOT, check=False)
        if go_res.returncode != 0:
            print("\n[FAILED] Go unit tests failed.")
            sys.exit(go_res.returncode)

        print("\n[PASSED] Unit Tests passed!")

        if args.unit_only:
            sys.exit(0)

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
            env_names = [entry.get("esb_env") for entry in matrix if entry.get("esb_env")]
            print(f"Available environments: {', '.join(env_names)}")
            sys.exit(1)

        entry_by_env = {entry.get("esb_env"): entry for entry in matrix if entry.get("esb_env")}
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


if __name__ == "__main__":
    main()
