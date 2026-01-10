#!/usr/bin/env python3
# Where: e2e/run_tests.py
# What: E2E test runner for ESB CLI scenarios.
# Why: Provide a single entry point for scenario setup, execution, and teardown.
import argparse
import json
import os
import subprocess
import sys
from concurrent.futures import ProcessPoolExecutor, as_completed
from pathlib import Path
from typing import Any

import yaml
from dotenv import load_dotenv

# Project root
PROJECT_ROOT = Path(__file__).parent.parent.resolve()
GO_CLI_ROOT = PROJECT_ROOT / "cli"
E2E_STATE_ROOT = PROJECT_ROOT / "e2e" / "fixtures" / ".esb"

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
    cmd = ["go", "run", "./cmd/esb"] + args
    print(f"Running: {' '.join(cmd)}")
    return subprocess.run(cmd, cwd=GO_CLI_ROOT, check=check)


DEFAULT_NO_PROXY_TARGETS = [
    "agent",
    "database",
    "gateway",
    "local-proxy",
    "localhost",
    "registry",
    "runtime-node",
    "s3-storage",
    "victorialogs",
    "::1",
    "10.88.0.0/16",
    "10.99.0.1",
    "127.0.0.1",
    "172.20.0.0/16",
]


def apply_proxy_env() -> None:
    proxy_keys = ("HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy")
    extra_key = "ESB_NO_PROXY_EXTRA"

    has_proxy = any(os.environ.get(key) for key in proxy_keys)
    existing_no_proxy = os.environ.get("NO_PROXY") or os.environ.get("no_proxy")
    extra_no_proxy = os.environ.get(extra_key)
    if not (has_proxy or existing_no_proxy or extra_no_proxy):
        return

    def split_no_proxy(value: str | None) -> list[str]:
        if not value:
            return []
        parts = value.replace(";", ",").split(",")
        return [item.strip() for item in parts if item.strip()]

    merged: list[str] = []
    seen: set[str] = set()

    for item in split_no_proxy(existing_no_proxy):
        if item not in seen:
            merged.append(item)
            seen.add(item)

    for item in DEFAULT_NO_PROXY_TARGETS:
        if item and item not in seen:
            merged.append(item)
            seen.add(item)

    for item in split_no_proxy(extra_no_proxy):
        if item and item not in seen:
            merged.append(item)
            seen.add(item)

    if merged:
        merged_value = ",".join(merged)
        os.environ["NO_PROXY"] = merged_value
        os.environ["no_proxy"] = merged_value

    if os.environ.get("HTTP_PROXY") and "http_proxy" not in os.environ:
        os.environ["http_proxy"] = os.environ["HTTP_PROXY"]
    if os.environ.get("http_proxy") and "HTTP_PROXY" not in os.environ:
        os.environ["HTTP_PROXY"] = os.environ["http_proxy"]
    if os.environ.get("HTTPS_PROXY") and "https_proxy" not in os.environ:
        os.environ["https_proxy"] = os.environ["HTTPS_PROXY"]
    if os.environ.get("https_proxy") and "HTTPS_PROXY" not in os.environ:
        os.environ["HTTPS_PROXY"] = os.environ["https_proxy"]


def load_generator_env_entries(generator_path: Path) -> list[tuple[str, str]]:
    if not generator_path.exists():
        return []

    data = yaml.safe_load(generator_path.read_text()) or {}
    envs = data.get("environments", {})
    entries: list[tuple[str, str]] = []

    if isinstance(envs, dict):
        for name, mode in envs.items():
            env_name = str(name).strip()
            env_mode = str(mode).strip() if mode is not None else ""
            if env_name:
                entries.append((env_name, env_mode))
        return entries

    if isinstance(envs, list):
        for item in envs:
            if isinstance(item, str):
                env_name = item.strip()
                if env_name:
                    entries.append((env_name, ""))
                continue
            if isinstance(item, dict):
                env_name = str(item.get("name", "")).strip()
                env_mode = str(item.get("mode", "")).strip()
                if env_name:
                    entries.append((env_name, env_mode))
        return entries

    return entries


def build_env_list(entries: list[tuple[str, str]]) -> str:
    parts = []
    for name, mode in entries:
        if mode:
            parts.append(f"{name}:{mode}")
        else:
            parts.append(name)
    return ",".join(parts)


def resolve_esb_home(env_name: str) -> Path:
    esb_home = os.environ.get("ESB_HOME")
    if esb_home:
        return Path(esb_home).expanduser()
    return Path.home() / ".esb" / env_name


def load_ports(env_name: str) -> dict[str, int]:
    port_file = resolve_esb_home(env_name) / "ports.json"
    if port_file.exists():
        return json.loads(port_file.read_text())
    return {}


def apply_ports_to_env(ports: dict[str, int]) -> None:
    for env_var, port in ports.items():
        os.environ[env_var] = str(port)

    if "ESB_PORT_GATEWAY_HTTPS" in ports:
        gateway_port = ports["ESB_PORT_GATEWAY_HTTPS"]
        os.environ["GATEWAY_PORT"] = str(gateway_port)
        os.environ["GATEWAY_URL"] = f"https://localhost:{gateway_port}"

    if "ESB_PORT_VICTORIALOGS" in ports:
        vl_port = ports["ESB_PORT_VICTORIALOGS"]
        os.environ["VICTORIALOGS_PORT"] = str(vl_port)
        os.environ["VICTORIALOGS_URL"] = f"http://localhost:{vl_port}"
        os.environ["VICTORIALOGS_QUERY_URL"] = os.environ["VICTORIALOGS_URL"]

    if "ESB_PORT_AGENT_GRPC" in ports:
        agent_port = ports["ESB_PORT_AGENT_GRPC"]
        os.environ["AGENT_GRPC_ADDRESS"] = f"localhost:{agent_port}"


def ensure_firecracker_node_up() -> None:
    """Fail fast if compute services are not running in firecracker mode."""
    if os.environ.get("ESB_MODE") != "firecracker":
        return
    print("[WARN] firecracker node check is not implemented for Go CLI")


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
        "--test-target", type=str, help="Specific pytest target (e.g. e2e/test_trace.py)"
    )
    parser.add_argument(
        "--profile",
        type=str,
        help="Environment to use for single target run (e.g. containerd)",
    )
    parser.add_argument("--fail-fast", action="store_true", help="Stop on first suite failure")
    parser.add_argument(
        "--parallel",
        action="store_true",
        help="Run environments in parallel (e.g., e2e-docker and e2e-containerd simultaneously)",
    )
    args = parser.parse_args()

    # --- Load Test Matrix (Needed for Env Info) ---
    matrix_file = PROJECT_ROOT / "e2e" / "test_matrix.yaml"
    if not matrix_file.exists():
        print(f"[ERROR] Matrix file not found: {matrix_file}")
        sys.exit(1)

    with open(matrix_file, "r") as f:
        config_matrix = yaml.safe_load(f)

    suites = config_matrix.get("suites", {})
    esb_project = config_matrix.get("esb_project")
    matrix = config_matrix.get("matrix", [])

    if not esb_project:
        print("[ERROR] esb_project is required in test_matrix.yaml.")
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
            "esb_project": esb_project,
            "env_vars": env_entry.get("env_vars", {}),
            "targets": [args.test_target],
            "exclude": [],
        }

        run_scenario(args, user_scenario)
        sys.exit(0)

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

    # Load Base Environment is now skipped to ensure environment isolation

    # Only print for sequential mode (subprocess of parallel will print its own)
    if not args.parallel:
        print("\nStarting Full E2E Test Suite (Matrix-Based)\n")

    failed_entries = []

    # Build list of all scenarios to run, grouped by ESB environment
    env_scenarios: dict[str, dict[str, Any]] = {}

    for entry in matrix:
        env_name = entry.get("esb_env")
        if not env_name:
            print(f"[ERROR] Invalid matrix entry format: {entry}")
            continue

        if args.profile and env_name != args.profile:
            continue

        suite_names = entry.get("suites", [])
        if env_name not in env_scenarios:
            env_scenarios[env_name] = {
                "name": f"Combined Scenarios for {env_name}",
                "env_file": entry.get("env_file"),
                "esb_env": env_name,
                "esb_project": esb_project,
                "env_vars": entry.get("env_vars", {}),
                "targets": [],
                "exclude": [],
            }

        for suite_name in suite_names:
            suite_def = suites.get(suite_name)
            if not suite_def:
                print(f"[ERROR] Suite '{suite_name}' not defined in suites.")
                continue
            env_scenarios[env_name]["targets"].extend(suite_def.get("targets", []))
            env_scenarios[env_name]["exclude"].extend(suite_def.get("exclude", []))

    # --- Global Reset & Warm-up ---
    # Perform this once before any environment execution (unless we are a child process)
    if not os.environ.get("ESB_TEST_CHILD_PROCESS"):
        warmup_environment(env_scenarios, matrix, esb_project, args)

    # --- Unified Execution Mode ---

    # If we are a child process (worker), execute the scenario directly.
    if os.environ.get("ESB_TEST_CHILD_PROCESS"):
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

    # Dispatcher Mode: Use executor for both parallel and sequential (max_workers=1) execution.
    # This ensures consistent environment isolation via subprocesses.

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
    )

    for _, (success, profile_failed) in results.items():
        if not success:
            failed_entries.extend(profile_failed)

    if failed_entries:
        print(f"\n[FAILED] The following environments failed: {', '.join(failed_entries)}")
        sys.exit(1)

    print("\n[PASSED] ALL MATRIX ENTRIES PASSED!")
    sys.exit(0)


def warmup_environment(env_scenarios: dict, matrix: list[dict], esb_project: str, args):
    """
    Perform global reset and warm-up actions.
    This includes cleaning up old artifacts and registering the ESB project
    in the global config before parallel execution.
    """
    # 1. Global Reset (if requested)
    if args.reset:
        print("\n[RESET] Fully cleaning all test artifact directories in e2e/fixtures/.esb/")
        import shutil

        for entry in matrix:
            env_name = entry.get("esb_env")
            if not env_name:
                continue
            esb_dir = E2E_STATE_ROOT / env_name
            if esb_dir.exists():
                print(f"  • Removing {esb_dir}")
                shutil.rmtree(esb_dir)

    generator_path = PROJECT_ROOT / "e2e" / "fixtures" / "generator.yml"
    env_entries = load_generator_env_entries(generator_path)
    if not env_entries:
        print(f"[ERROR] generator.yml not found or missing environments: {generator_path}")
        sys.exit(1)

    env_list = build_env_list(env_entries)
    env_names = {name for name, _ in env_entries}

    active_envs = list(env_scenarios.keys())
    for env_name in active_envs:
        if env_name not in env_names:
            print(f"[ERROR] Environment '{env_name}' not defined in generator.yml.")
            sys.exit(1)

    # 2. Register ESB project (Warm-up)
    if active_envs:
        print(f"\n[INIT] Registering ESB project for: {', '.join(active_envs)}")
        esb_template = os.getenv("ESB_TEMPLATE", "e2e/fixtures/template.yaml")
        subprocess.run(
            [
                "go",
                "run",
                "./cmd/esb",
                "--template",
                str(PROJECT_ROOT / esb_template),
                "init",
                "--env",
                env_list,
                "--name",
                esb_project,
            ],
            cwd=GO_CLI_ROOT,
            check=True,
        )


def run_profiles_with_executor(
    env_scenarios: dict[str, dict[str, Any]],
    reset: bool,
    build: bool,
    cleanup: bool,
    fail_fast: bool,
    max_workers: int,
) -> dict[str, tuple[bool, list[str]]]:
    """
    Run environments using a ProcessPoolExecutor.
    If max_workers is 1, they run sequentially but still in subprocesses for isolation.
    Returns: dict mapping env_name to (success, failed_scenario_names)
    """
    results = {}

    # We use 'spawn' or default context. For simple script execution, default is fine.
    with ProcessPoolExecutor(max_workers=max_workers) as executor:
        future_to_profile = {}

        # Submit all tasks
        for profile_name, _ in env_scenarios.items():
            # Build command for subprocess
            cmd = [
                sys.executable,
                "-u",  # Unbuffered output
                "-m",
                "e2e.run_tests",
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

            # Determine log prefix/color
            profile_index = list(env_scenarios.keys()).index(profile_name)
            # If sequential (max_workers=1), we don't strictly need colors, but it doesn't hurt.
            # However, if running sequentially, we might want to announce "Starting..." immediately before submission?
            # With Executor, submission happens first.

            if max_workers > 1:
                print(f"[PARALLEL] Scheduling environment: {profile_name}")
                color_code = COLORS[profile_index % len(COLORS)]
            else:
                # Sequential mode logging is handled more by the subprocess stream,
                # but we can log here too.
                color_code = ""

            future = executor.submit(run_profile_subprocess, profile_name, cmd, color_code)
            future_to_profile[future] = profile_name

        # Process results as they complete
        for future in as_completed(future_to_profile):
            profile_name = future_to_profile[future]
            try:
                returncode, output = future.result()
                success = returncode == 0
                failed_list = [] if success else [f"Environment {profile_name}"]

                prefix = "[PARALLEL]" if max_workers > 1 else "[MATRIX]"

                if success:
                    print(f"{prefix} Environment {profile_name} PASSED")
                else:
                    print(f"{prefix} Environment {profile_name} FAILED (exit code: {returncode})")

                results[profile_name] = (success, failed_list)
            except Exception as e:
                print(f"[ERROR] Environment {profile_name} FAILED with exception: {e}")
                results[profile_name] = (False, [f"Environment {profile_name} (exception)"])

    return results


def run_profile_subprocess(
    profile_name: str, cmd: list[str], color_code: str = ""
) -> tuple[int, str]:
    """Run a profile in a subprocess and stream output with prefix."""
    prefix = f"{color_code}[{profile_name}]{COLOR_RESET}"

    # Inject child process flag
    env = os.environ.copy()
    env["ESB_TEST_CHILD_PROCESS"] = "1"

    process = subprocess.Popen(
        cmd,
        cwd=PROJECT_ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        bufsize=1,  # Line buffered
        env=env,
    )

    output_lines = []

    # Read output line by line as it becomes available
    try:
        if process.stdout:
            for line in iter(process.stdout.readline, ""):
                clean_line = line.rstrip()
                if clean_line:  # Only print non-empty lines to avoid prefix spam
                    print(f"{prefix} {clean_line}", flush=True)
                output_lines.append(line)
    except Exception as e:
        print(f"{prefix} Error reading output: {e}")

    returncode = process.wait()

    # Write output to a log file for debugging
    log_file = PROJECT_ROOT / "e2e" / f".parallel-{profile_name}.log"
    with open(log_file, "w", encoding="utf-8") as f:
        f.write(f"=== {profile_name} combined output ===\n")
        f.writelines(output_lines)

    return returncode, "".join(output_lines)


def run_scenario(args, scenario):
    """Run a single scenario."""

    # Determine actions based on scenario overrides or global args
    do_reset = scenario.get("perform_reset", args.reset)
    do_build = scenario.get("perform_build", args.build)

    # 0. Set ESB_PROJECT/ESB_ENV for CLI resolution
    env_name = scenario.get("esb_env", os.environ.get("ESB_ENV", "e2e-docker"))
    project_name = scenario.get("esb_project", os.environ.get("ESB_PROJECT", ""))
    if not project_name:
        print("[ERROR] ESB project name is missing.")
        sys.exit(1)

    os.environ["ESB_PROJECT"] = project_name
    os.environ["ESB_ENV"] = env_name
    os.environ["ESB_ENV_SET"] = "1"  # Mark as explicitly set to bypass interactive prompt logic
    os.environ["ESB_HOME"] = str(E2E_STATE_ROOT / env_name)

    # 0.5 Clear previous image tag to avoid leaks between scenarios in the same process
    os.environ.pop("ESB_IMAGE_TAG", None)
    if not os.environ.get("ESB_PROJECT_NAME"):
        os.environ["ESB_PROJECT_NAME"] = f"esb-{env_name}".lower()
    if not os.environ.get("ESB_IMAGE_TAG"):
        os.environ["ESB_IMAGE_TAG"] = env_name

    # 1. Runtime Mode Setup (from generator.yml)
    generator_path = PROJECT_ROOT / "e2e" / "fixtures" / "generator.yml"
    env_entries = load_generator_env_entries(generator_path)
    env_mode = ""
    for name, mode in env_entries:
        if name == env_name:
            env_mode = mode
            break
    if env_mode:
        os.environ["ESB_MODE"] = env_mode
    else:
        os.environ.pop("ESB_MODE", None)

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

    # 2.5 Inject Proxy Settings (Ensure NO_PROXY includes all internal services)
    apply_proxy_env()

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
    esb_template = os.getenv("ESB_TEMPLATE", "e2e/fixtures/template.yaml")
    env["ESB_TEMPLATE"] = str(PROJECT_ROOT / esb_template)

    # Note: Do NOT update os.environ with 'env' here.
    # 'env' contains localhost URLs (e.g. VICTORIALOGS_URL=http://localhost:...) intended for pytest.
    # If we inject these into os.environ, 'run_esb' (esb up) will pick them up and pass them to Docker containers,
    # causing services to try to connect to localhost (themselves) instead of the correct container.

    ensure_firecracker_node_up()

    try:
        # 2. Reset / Build
        if do_reset:
            print(f"➜ Resetting environment: {env_name}")
            run_esb(["down", "-v"], check=True)
            # Re-generate configurations via build
            run_esb(["build", "--no-cache"])
        elif do_build:
            print(f"➜ Building environment: {env_name}")
            run_esb(["build", "--no-cache"])
        else:
            print(f"➜ Preparing environment: {env_name}")
            # Ensure services are stopped before starting (without destroying data)
            run_esb(["stop"], check=True)
            run_esb(["build"])

        # If build_only, skip UP and tests
        if scenario.get("build_only"):
            return

        # 3. UP
        run_esb(["up", "--detach", "--wait"])

        # 3.5 Load dynamic ports from ports.json (created by esb up)
        ports = load_ports(env_name)
        if ports:
            apply_ports_to_env(ports)
            # log_ports is redundant as 'esb up' already logs it
            # log_ports(env_name, ports)

            # Update env dict for pytest subprocess
            env["GATEWAY_PORT"] = str(
                ports.get("ESB_PORT_GATEWAY_HTTPS", env.get("GATEWAY_PORT", "443"))
            )
            env["VICTORIALOGS_PORT"] = str(
                ports.get("ESB_PORT_VICTORIALOGS", env.get("VICTORIALOGS_PORT", "9428"))
            )
            env["GATEWAY_URL"] = f"https://localhost:{env['GATEWAY_PORT']}"
            env["VICTORIALOGS_URL"] = f"http://localhost:{env['VICTORIALOGS_PORT']}"
            env["VICTORIALOGS_QUERY_URL"] = env["VICTORIALOGS_URL"]
            if "ESB_PORT_AGENT_GRPC" in ports:
                env["AGENT_GRPC_ADDRESS"] = f"localhost:{ports['ESB_PORT_AGENT_GRPC']}"

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
            run_esb(["down"])
        # If not cleanup, we leave containers running for debugging last scenario
        # But next scenario execution will force down anyway.


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "run_scenario":
        # Internal call wrapper if needed? No, just call main().
        pass
    main()
