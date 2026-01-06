#!/usr/bin/env python3
# Where: tests/run_tests.py
# What: E2E test runner for ESB CLI scenarios.
# Why: Provide a single entry point for scenario setup, execution, and teardown.
import argparse
import os
import sys
import subprocess
from pathlib import Path
from dotenv import load_dotenv
from concurrent.futures import ProcessPoolExecutor, as_completed

# Project root
PROJECT_ROOT = Path(__file__).parent.parent.resolve()

# Terminal colors for parallel output
COLORS = [
    "\033[36m",  # Cyan
    "\033[32m",  # Green
    "\033[34m",  # Blue
    "\033[35m",  # Magenta
    "\033[33m",  # Yellow
]
COLOR_RESET = "\033[0m"


def run_esb(args: list[str], check: bool = True):
    """Helper to run the esb CLI."""
    # Use current source code instead of installed command.
    cmd = [sys.executable, "-m", "tools.cli.main"] + args
    print(f"Running: {' '.join(cmd)}")
    return subprocess.run(cmd, cwd=PROJECT_ROOT, check=check)


def ensure_firecracker_node_up() -> None:
    """Fail fast if compute services are not running in firecracker mode."""
    from tools.cli import config as cli_config
    from tools.cli import runtime_mode

    if runtime_mode.get_mode() != cli_config.ESB_MODE_FIRECRACKER:
        return

    result = run_esb(["node", "doctor", "--strict", "--require-up"], check=False)
    if result.returncode != 0:
        print("\n[FAILED] Compute node is not up. Run `esb node up` and retry.")
        sys.exit(result.returncode)


def main():
    # Suppress warnings.
    import warnings
    import urllib3

    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    warnings.filterwarnings("ignore", category=DeprecationWarning)

    parser = argparse.ArgumentParser(description="E2E Test Runner (ESB CLI Wrapper)")
    parser.add_argument("--build", action="store_true", help="Rebuild images before running tests")
    parser.add_argument(
        "--cleanup", action="store_true", help="Cleanup environment after successful tests"
    )
    parser.add_argument(
        "--reset", action="store_true", help="Reset environment before running tests"
    )
    parser.add_argument("--unit", action="store_true", help="Run unit tests")
    parser.add_argument("--unit-only", action="store_true", help="Run unit tests only")
    parser.add_argument(
        "--test-target", type=str, help="Specific pytest target (e.g. tests/test_trace.py)"
    )
    parser.add_argument(
        "--profile",
        type=str,
        help="Profile to use for single target run (e.g. e2e-containerd)",
    )
    parser.add_argument("--fail-fast", action="store_true", help="Stop on first suite failure")
    parser.add_argument(
        "--parallel",
        action="store_true",
        help="Run profiles in parallel (e.g., e2e-docker and e2e-containerd simultaneously)",
    )
    args = parser.parse_args()

    # --- Load Test Matrix (Needed for Profile Info) ---
    import yaml

    matrix_file = PROJECT_ROOT / "tests" / "test_matrix.yaml"
    if not matrix_file.exists():
        print(f"[ERROR] Matrix file not found: {matrix_file}")
        sys.exit(1)

    with open(matrix_file, "r") as f:
        config_matrix = yaml.safe_load(f)

    suites = config_matrix.get("suites", {})
    profiles = config_matrix.get("profiles", {})
    matrix = config_matrix.get("matrix", [])

    # --- Single Target Mode (Legacy/Debug) ---
    # --- Single Target Mode (Legacy/Debug) ---
    if args.test_target:
        if not args.profile:
            print("[ERROR] --profile is required when using --test-target.")
            print(f"Available profiles: {', '.join(profiles.keys())}")
            sys.exit(1)

        if args.profile not in profiles:
            print(f"[ERROR] Profile '{args.profile}' not found in matrix.")
            sys.exit(1)

        profile_def = profiles[args.profile]

        user_scenario = {
            "name": f"User-Specified on {args.profile}",
            "env_file": profile_def.get("env_file"),
            "runtime_mode": profile_def.get("mode"),
            "esb_env": args.profile,  # Use profile name as environment name
            "targets": [args.test_target],
            "exclude": [],
        }

        run_scenario(args, user_scenario)
        sys.exit(0)

    # --- Unit Tests ---
    if args.unit or args.unit_only:
        os.environ["DISABLE_VICTORIALOGS"] = "1"
        print("\n=== Running Unit Tests ===\n")
        cmd = [sys.executable, "-m", "pytest", "services/gateway/tests", "tools/cli/tests", "-v"]
        res = subprocess.run(cmd, cwd=PROJECT_ROOT, check=False)
        if res.returncode != 0:
            print("\n[FAILED] Unit Tests failed.")
            sys.exit(res.returncode)
        print("\n[PASSED] Unit Tests passed!")

        if args.unit_only:
            sys.exit(0)

    # Load Base Environment is now skipped to ensure profile isolation

    print("\nStarting Full E2E Test Suite (Matrix-Based)\n")
    failed_entries = []

    initialized_profiles = set()

    # Build list of all scenarios to run, grouped by profile
    profile_scenarios: dict[str, list[dict]] = {}

    for entry in matrix:
        # Determine structure type: Profile-First (New) or Suite-First (Legacy)
        is_profile_first = "profile" in entry and "suites" in entry
        is_suite_first = "suite" in entry and "profiles" in entry

        if not is_profile_first and not is_suite_first:
            print(f"[ERROR] Invalid matrix entry format: {entry}")
            continue

        # normalize to list of (suite, profile) tuples to execute for this entry
        items_to_run = []

        if is_profile_first:
            profile_name = entry["profile"]
            target_suites = entry["suites"]

            if profile_name not in profiles:
                print(f"[ERROR] Profile '{profile_name}' not defined in profiles.")
                continue

            # Filter global profile arg
            if args.profile and profile_name != args.profile:
                continue

            for s_name in target_suites:
                if s_name not in suites:
                    print(f"[ERROR] Suite '{s_name}' not defined in suites.")
                    continue
                items_to_run.append((s_name, profile_name))

        else:  # is_suite_first
            suite_name = entry["suite"]
            target_profiles = entry["profiles"]

            if suite_name not in suites:
                print(f"[ERROR] Suite '{suite_name}' not defined in suites.")
                continue

            for p_name in target_profiles:
                if p_name not in profiles:
                    print(f"[ERROR] Profile '{p_name}' not defined in profiles.")
                    continue

                if args.profile and p_name != args.profile:
                    continue
                items_to_run.append((suite_name, p_name))

        # Build scenarios
        for suite_name, profile_name in items_to_run:
            profile_def = profiles[profile_name]
            suite_def = suites[suite_name]

            target_env = profile_name  # Use profile name as environment name

            if profile_name not in profile_scenarios:
                profile_scenarios[profile_name] = {
                    "name": f"Combined Scenarios for {profile_name}",
                    "env_file": profile_def.get("env_file"),
                    "runtime_mode": profile_def.get("mode"),
                    "esb_env": target_env,
                    "env_vars": profile_def.get("env_vars", {}),
                    "targets": [],
                    "exclude": [],
                }

            # Merge targets and exclusions
            profile_scenarios[profile_name]["targets"].extend(suite_def.get("targets", []))
            profile_scenarios[profile_name]["exclude"].extend(suite_def.get("exclude", []))

    # --- Global Reset (if requested) ---
    if args.reset:
        print("\n[RESET] Fully cleaning all test artifact directories in tests/fixtures/.esb/")
        import shutil

        for p_name in profiles.keys():
            esb_dir = PROJECT_ROOT / "tests" / "fixtures" / ".esb" / p_name
            if esb_dir.exists():
                print(f"  • Removing {esb_dir}")
                shutil.rmtree(esb_dir)

    # --- Parallel Execution Mode ---
    if args.parallel and len(profile_scenarios) > 1:
        print(
            f"\n[PARALLEL] Starting parallel execution for {len(profile_scenarios)} profiles: {', '.join(profile_scenarios.keys())}"
        )
        print(
            "[PARALLEL] Build, infrastructure setup, and tests will run simultaneously across profiles.\n"
        )

        results = run_profiles_parallel(
            profile_scenarios,
            reset=args.reset,
            build=args.build,
            cleanup=args.cleanup,
            fail_fast=args.fail_fast,
        )

        for profile_name, (success, profile_failed) in results.items():
            if not success:
                failed_entries.extend(profile_failed)

        if failed_entries:
            print(f"\n[FAILED] The following profiles failed: {', '.join(failed_entries)}")
            sys.exit(1)

        print("\n[PASSED] ALL MATRIX ENTRIES PASSED!")
        sys.exit(0)

    # --- Sequential Execution (Default) ---
    for profile_name, scenario in profile_scenarios.items():
        # Inject initialization flags into the scenario
        scenario["perform_reset"] = args.reset
        scenario["perform_build"] = args.build

        print(f"\n[Matrix] Running Profile: '{profile_name}'")
        print(f"         > Environment: {scenario['esb_env']}")
        print(f"         > Targets: {', '.join(scenario['targets'])}")

        try:
            run_scenario(args, scenario)
            initialized_profiles.add(profile_name)
        except SystemExit as e:
            if e.code != 0:
                print(f"\n[FAILED] Profile '{profile_name}' FAILED.")
                failed_entries.append(profile_name)
                if args.fail_fast:
                    print("\n[FAIL-FAST] Stopping due to --fail-fast option.")
                    sys.exit(1)
            else:
                print(f"\n[PASSED] Profile '{profile_name}' PASSED.")
        except Exception as e:
            print(f"\n[FAILED] Profile '{profile_name}' FAILED with exception: {e}")
            failed_entries.append(profile_name)
            if args.fail_fast:
                print("\n[FAIL-FAST] Stopping due to --fail-fast option.")
                sys.exit(1)

    if failed_entries:
        print(f"\n[FAILED] The following matrix entries failed: {', '.join(failed_entries)}")
        sys.exit(1)

    print("\n[PASSED] ALL MATRIX ENTRIES PASSED!")
    sys.exit(0)


def run_profiles_parallel(
    profile_scenarios: dict[str, list[dict]],
    reset: bool,
    build: bool,
    cleanup: bool,
    fail_fast: bool,
) -> dict[str, tuple[bool, list[str]]]:
    """
    Run multiple profiles in parallel using subprocesses.
    Each profile runs its suites sequentially within its own subprocess.
    Returns: dict mapping profile_name to (success, failed_scenario_names)
    """
    results = {}

    with ProcessPoolExecutor(max_workers=len(profile_scenarios)) as executor:
        future_to_profile = {}

        for profile_name, scenario in profile_scenarios.items():
            # Build command for subprocess
            cmd = [
                sys.executable,
                "-m",
                "tests.run_tests",
                "--profile",
                profile_name,
            ]
            if reset:
                cmd.append("--reset")
            if build:
                cmd.append("--build")
            if cleanup:
                cmd.append("--cleanup")
            if fail_fast:
                cmd.append("--fail-fast")

            print(f"[PARALLEL] Starting profile: {profile_name}")
            # Assign a color based on the profile's index
            profile_index = list(profile_scenarios.keys()).index(profile_name)
            color_code = COLORS[profile_index % len(COLORS)]

            future = executor.submit(run_profile_subprocess, profile_name, cmd, color_code)
            future_to_profile[future] = profile_name

        for future in as_completed(future_to_profile):
            profile_name = future_to_profile[future]
            try:
                returncode, output = future.result()
                success = returncode == 0
                failed_list = [] if success else [f"Profile {profile_name}"]

                if success:
                    print(f"[PARALLEL] Profile {profile_name} PASSED")
                else:
                    print(f"[PARALLEL] Profile {profile_name} FAILED (exit code: {returncode})")

                results[profile_name] = (success, failed_list)
            except Exception as e:
                print(f"[PARALLEL] Profile {profile_name} FAILED with exception: {e}")
                results[profile_name] = (False, [f"Profile {profile_name} (exception)"])

    return results


def run_profile_subprocess(
    profile_name: str, cmd: list[str], color_code: str = ""
) -> tuple[int, str]:
    """Run a profile in a subprocess and stream output with prefix."""
    prefix = f"{color_code}[{profile_name}]{COLOR_RESET}"

    process = subprocess.Popen(
        cmd,
        cwd=PROJECT_ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        bufsize=1,  # Line buffered
    )

    output_lines = []

    # Read output line by line as it becomes available
    try:
        if process.stdout:
            for line in iter(process.stdout.readline, ""):
                clean_line = line.rstrip()
                print(f"{prefix} {clean_line}", flush=True)
                output_lines.append(line)
    except Exception as e:
        print(f"{prefix} Error reading output: {e}")

    returncode = process.wait()

    # Write output to a log file for debugging
    log_file = PROJECT_ROOT / "tests" / f".parallel-{profile_name}.log"
    with open(log_file, "w", encoding="utf-8") as f:
        f.write(f"=== {profile_name} combined output ===\n")
        f.writelines(output_lines)

    return returncode, "".join(output_lines)


def run_scenario(args, scenario):
    """Run a single scenario."""

    # Determine actions based on scenario overrides or global args
    do_reset = scenario.get("perform_reset", args.reset)
    do_build = scenario.get("perform_build", args.build)

    # 0. Set ESB_ENV first (needed for mode config path)
    env_name = scenario.get("esb_env", os.environ.get("ESB_ENV", "default"))
    os.environ["ESB_ENV"] = env_name

    # 0.5 Clear previous image tag to avoid leaks between scenarios in the same process
    os.environ.pop("ESB_IMAGE_TAG", None)

    # 1. Runtime Mode Setup (Optional)
    if "runtime_mode" in scenario:
        print(f"Switching runtime mode to: {scenario['runtime_mode']} (env: {env_name})")
        run_esb(["mode", "set", scenario["runtime_mode"]])

    # Load Scenario-Specific Env File (Required for isolation)
    if scenario.get("env_file"):
        env_file_path = PROJECT_ROOT / scenario["env_file"]
        if env_file_path.exists():
            load_dotenv(env_file_path, override=True)
            print(f"Loaded scenario environment from: {env_file_path}")
        else:
            print(f"Warning: Scenario environment file not found: {env_file_path}")
    else:
        print("Warning: No env_file specified for this scenario. Operating with system env only.")

    # Environment Isolation & Ports Logic
    from tools.cli import config as cli_config

    # 2. Setup the global context (os.environ) for this environment
    cli_config.setup_environment(env_name)

    # 2.5 Inject Proxy Settings (Ensure NO_PROXY includes all internal services)
    from tools.cli.core import proxy

    proxy.apply_proxy_env()

    # 3. Reload env vars into a dict for passing to subprocess (pytest)
    env = os.environ.copy()

    # Capture calculated values for convenience
    env["GATEWAY_PORT"] = env.get("ESB_PORT_GATEWAY_HTTPS", "443")
    env["VICTORIALOGS_PORT"] = env.get("ESB_PORT_VICTORIALOGS", "9428")
    env["GATEWAY_URL"] = f"https://localhost:{env['GATEWAY_PORT']}"
    env["VICTORIALOGS_URL"] = f"http://localhost:{env['VICTORIALOGS_PORT']}"
    env["VICTORIALOGS_QUERY_URL"] = env["VICTORIALOGS_URL"]
    env["AGENT_GRPC_ADDRESS"] = f"localhost:{env.get('ESB_PORT_AGENT_GRPC', '50051')}"

    # Merge scenario-specific environment variables
    env.update(scenario.get("env_vars", {}))

    # Explicitly set template path
    esb_template = os.getenv("ESB_TEMPLATE", "tests/fixtures/template.yaml")
    env["ESB_TEMPLATE"] = str(PROJECT_ROOT / esb_template)

    # Note: Do NOT update os.environ with 'env' here.
    # 'env' contains localhost URLs (e.g. VICTORIALOGS_URL=http://localhost:...) intended for pytest.
    # If we inject these into os.environ, 'run_esb' (esb up) will pick them up and pass them to Docker containers,
    # causing services to try to connect to localhost (themselves) instead of the correct container.

    ensure_firecracker_node_up()

    # Define common override arguments
    override_args = ["-f", "tests/docker-compose.test.yml"]
    env_args = ["--env", env_name]
    template_args = ["--template", env["ESB_TEMPLATE"]]

    try:
        # 2. Reset / Build
        if do_reset:
            print(f"➜ Resetting environment: {env_name}")
            run_esb(template_args + ["down", "-v"] + override_args + env_args, check=True)
            # Re-generate configurations via build
            run_esb(template_args + ["build", "--no-cache"] + override_args + env_args)
        elif do_build:
            print(f"➜ Building environment: {env_name}")
            run_esb(template_args + ["build", "--no-cache"] + override_args + env_args)
        else:
            print(f"➜ Using existing environment: {env_name}")
            # Ensure services are stopped before starting (without destroying data)
            run_esb(template_args + ["stop"] + override_args + env_args, check=True)

        # If build_only, skip UP and tests
        if scenario.get("build_only"):
            return

        # 3. UP
        up_args = template_args + ["up", "--detach", "--wait"] + override_args + env_args
        if do_build or do_reset:
            up_args.append("--build")

        run_esb(up_args)

        # 4. Run Tests
        if not scenario["targets"]:
            # No test targets specified, skip test execution
            return

        print(f"\n=== Running Tests for {scenario['name']} ===\n")

        pytest_cmd = [sys.executable, "-m", "pytest"] + scenario["targets"] + ["-v"]

        # Excludes
        for excl in scenario["exclude"]:
            pytest_cmd.extend(["--ignore", excl])

        # Pass the full env with calculated ports to pytest
        result = subprocess.run(pytest_cmd, cwd=PROJECT_ROOT, check=False, env=env)

        if result.returncode != 0:
            sys.exit(result.returncode)

    except subprocess.CalledProcessError as e:
        print(f"Error executing command: {e}")
        sys.exit(1)

    finally:
        # 5. Cleanup (Conditional)
        if args.cleanup:
            run_esb(template_args + ["down"] + override_args + env_args)
        # If not cleanup, we leave containers running for debugging last scenario
        # But next scenario execution will force down anyway.


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "run_scenario":
        # Internal call wrapper if needed? No, just call main().
        pass
    main()
