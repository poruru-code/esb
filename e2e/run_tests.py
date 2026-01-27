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
from e2e.runner.config import build_env_scenarios, load_test_matrix
from e2e.runner.executor import (
    run_build_phase_parallel,
    run_build_phase_serial,
    run_profiles_with_executor,
    run_scenario,
    warmup_environment,
)
from e2e.runner.utils import BRAND_SLUG, GO_CLI_ROOT, PROJECT_ROOT


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

        env_entry = entry_by_env[args.profile]

        user_scenario = {
            "name": f"User-Specified on {args.profile}",
            "env_file": env_entry.get("env_file"),
            "esb_env": args.profile,
            "esb_project": BRAND_SLUG,
            "env_vars": env_entry.get("env_vars", {}),
            "targets": [args.test_target],
            "exclude": [],
        }

        run_scenario(args, user_scenario)
        sys.exit(0)

    # Build list of all scenarios to run
    # If args.profile is set, build_env_scenarios filters internally or we can pass a filter?
    # Actually build_env_scenarios builds everything, we filter later.
    # To optimize, let's update build_env_scenarios to accept a filter or filter here.
    env_scenarios = build_env_scenarios(matrix, suites, profile_filter=args.profile)

    if not env_scenarios and args.profile:
        # If profile was requested but not found via build_env_scenarios (unlikely if logic is correct)
        # logic inside build_env_scenarios handles basic filtering, but strict "found" check is good.
        pass

    if args.build_only or args.test_only:
        if not args.profile:
            print("[ERROR] --profile is required when using --build-only/--test-only.")
            sys.exit(1)
        for _, scenario in env_scenarios.items():
            scenario["perform_reset"] = args.reset
            scenario["perform_build"] = args.build
            scenario["build_only"] = args.build_only
            scenario["test_only"] = args.test_only
            try:
                run_scenario(args, scenario)
            except SystemExit as e:
                if e.code != 0:
                    sys.exit(e.code)
            except Exception as e:
                print(f"[ERROR] Scenario failed: {e}")
                sys.exit(1)
        sys.exit(0)

    # --- Global Reset & Warm-up ---
    # Perform this if we are in the main dispatcher process (not a parallel worker).
    # Validate shared inputs (e.g., template) once before dispatching.
    if os.environ.get("E2E_WORKER") != "1":
        warmup_environment(env_scenarios, matrix, args)

    failed_entries = []

    # --- Build Phase (Parallel/Serial, subprocess isolation) ---
    build_parallel = args.parallel and len(env_scenarios) > 1
    if build_parallel:
        print("\n=== Build Phase (Parallel) ===\n")
        build_failed = run_build_phase_parallel(
            env_scenarios,
            reset=args.reset,
            build=args.build,
            fail_fast=args.fail_fast,
            verbose=args.verbose,
        )
    else:
        print("\n=== Build Phase (Serial) ===\n")
        build_failed = run_build_phase_serial(
            env_scenarios,
            reset=args.reset,
            build=args.build,
            fail_fast=args.fail_fast,
            verbose=args.verbose,
        )
    if build_failed:
        print(f"\n❌ [FAILED] Build failed for: {', '.join(build_failed)}")
        print_tail_logs(build_failed)
        sys.exit(1)

    # --- Test Phase (Parallel) ---
    parallel_mode = args.parallel and len(env_scenarios) > 1
    max_workers = len(env_scenarios) if parallel_mode else 1

    if parallel_mode:
        print(
            f"\n[PARALLEL] Starting test phase for {len(env_scenarios)} environments: {', '.join(env_scenarios.keys())}"
        )
        print("[PARALLEL] Build phase completed; tests will run in parallel.\n")
    else:
        print("\nStarting Test Phase (Matrix-Based)\n")

    results = run_profiles_with_executor(
        env_scenarios,
        reset=False,
        build=False,
        cleanup=args.cleanup,
        fail_fast=args.fail_fast,
        max_workers=max_workers,
        verbose=args.verbose,
        test_only=True,
    )

    for _, (success, profile_failed) in results.items():
        if not success:
            failed_entries.extend(profile_failed)

    if failed_entries:
        print(f"\n❌ [FAILED] The following environments failed: {', '.join(failed_entries)}")
        print_tail_logs(failed_entries)
        sys.exit(1)

    print("\n🎉 [PASSED] ALL MATRIX ENTRIES PASSED!")
    sys.exit(0)


if __name__ == "__main__":
    main()
