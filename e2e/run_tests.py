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
from e2e.runner.executor import run_profiles_with_executor, run_scenario, warmup_environment
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


def main():
    # Suppress warnings.
    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    warnings.filterwarnings("ignore", category=DeprecationWarning)

    args = parse_args()

    # --- Unit Tests ---
    if args.unit or args.unit_only:
        os.environ["DISABLE_VICTORIALOGS"] = "1"
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
    esb_project = config_matrix.get("esb_project")
    matrix = config_matrix.get("matrix", [])

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
            "esb_project": esb_project,
            "env_vars": env_entry.get("env_vars", {}),
            "targets": [args.test_target],
            "exclude": [],
        }

        run_scenario(args, user_scenario)
        sys.exit(0)

    # Only print for sequential mode (subprocess of parallel will print its own)
    if not args.parallel:
        print("\nStarting Full E2E Test Suite (Matrix-Based)\n")

    failed_entries = []

    # Build list of all scenarios to run
    # If args.profile is set, build_env_scenarios filters internally or we can pass a filter?
    # Actually build_env_scenarios builds everything, we filter later.
    # To optimize, let's update build_env_scenarios to accept a filter or filter here.
    env_scenarios = build_env_scenarios(matrix, suites, esb_project, profile_filter=args.profile)

    if not env_scenarios and args.profile:
        # If profile was requested but not found via build_env_scenarios (unlikely if logic is correct)
        # logic inside build_env_scenarios handles basic filtering, but strict "found" check is good.
        pass

    # --- Global Reset & Warm-up ---
    # Perform this once before any environment execution (unless we are in a sub-profile run)
    if not args.profile:
        warmup_environment(env_scenarios, matrix, esb_project, args)

    # --- Unified Execution Mode ---

    # If a specific profile is requested (either by user or as a parallel worker), execute it directly.
    if args.profile:
        for _, scenario in env_scenarios.items():
            # Inject initialization flags into the scenario
            scenario["perform_reset"] = args.reset
            scenario["perform_build"] = args.build

            # Run in-process
            try:
                run_scenario(args, scenario)
            except SystemExit as e:
                if e.code != 0:
                    sys.exit(e.code)
            except Exception as e:
                print(f"[ERROR] Scenario failed: {e}")
                sys.exit(1)
        sys.exit(0)

    # Dispatcher Mode
    parallel_mode = args.parallel and len(env_scenarios) > 1
    max_workers = len(env_scenarios) if parallel_mode else 1

    if parallel_mode:
        print(
            f"\n[PARALLEL] Starting parallel execution for {len(env_scenarios)} environments: {', '.join(env_scenarios.keys())}"
        )
        print(
            "[PARALLEL] Build, infrastructure setup, and tests will run simultaneously across environments.\n"
        )
    else:
        print("\nStarting Full E2E Test Suite (Matrix-Based)\n")

    results = run_profiles_with_executor(
        env_scenarios,
        reset=args.reset,
        build=args.build,
        cleanup=args.cleanup,
        fail_fast=args.fail_fast,
        max_workers=max_workers,
        verbose=args.verbose,
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
