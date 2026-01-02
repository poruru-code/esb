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

# Project root
PROJECT_ROOT = Path(__file__).parent.parent.resolve()


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
    parser.add_argument("--build", action="store_true", help="Rebuild images before running")
    parser.add_argument("--cleanup", action="store_true", help="Stop containers after tests")
    parser.add_argument("--reset", action="store_true", help="Full reset before running")
    parser.add_argument("--unit", action="store_true", help="Run unit tests")
    parser.add_argument("--unit-only", action="store_true", help="Run unit tests only")
    parser.add_argument(
        "--test-target", type=str, help="Specific pytest target (e.g. tests/test_trace.py)"
    )
    parser.add_argument(
        "--env-file",
        type=str,
        default="tests/environments/.env.standard",
        help="Path to env file (default: tests/environments/.env.standard)",
    )

    args = parser.parse_args()

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

    # --- Scenarios Definition ---
    # Scenario definition: consolidate on Auto-Scaling (PoolManager) after removing Legacy Mode.
    SCENARIOS = [
        {
            "name": "Auto-Scaling",
            "env_file": "tests/environments/.env.autoscaling",
            "targets": [
                "tests/scenarios/autoscaling/",
                "tests/scenarios/standard/",
            ],
            "exclude": [],
        },
        {
            "name": "Auto-Scaling (Containerd)",
            "env_file": "tests/environments/.env.containerd",
            "runtime_mode": "containerd",
            "targets": [
                "tests/scenarios/autoscaling/",
                "tests/scenarios/standard/",
            ],
            "exclude": [],
        },
    ]

    # If target specified via CLI, run a single target (legacy compatible).
    if args.test_target:
        # User specified target, simple run
        # env_file defaults need update if user doesn't specify
        # Should we look in environments/? Default to .env.standard in environments/
        default_env = "tests/environments/.env.standard"

        user_scenario = {
            "name": "User-Specified",
            "env_file": args.env_file
            if args.env_file != "tests/environments/.env.standard"
            else default_env,
            # Note: parser default is "tests/.env.test", we should update parser default too or handle here.
            "targets": [args.test_target],
            "exclude": [],
        }
        run_scenario(args, user_scenario)
        sys.exit(0)

    # Run all scenarios.
    print("\nStarting Full E2E Test Suite (Scenario-Based)\n")
    failed_scenarios = []

    for scenario in SCENARIOS:
        print(f"\n[Scenario] Running: {scenario['name']}")
        try:
            run_scenario(args, scenario)
        except SystemExit as e:
            if e.code != 0:
                print(f"\n[FAILED] Scenario '{scenario['name']}' FAILED.")
                failed_scenarios.append(scenario["name"])
            else:
                print(f"\n[PASSED] Scenario '{scenario['name']}' PASSED.")
        except Exception as e:
            print(f"\n[FAILED] Scenario '{scenario['name']}' FAILED with exception: {e}")
            failed_scenarios.append(scenario["name"])

    if failed_scenarios:
        print(f"\n[FAILED] The following scenarios failed: {', '.join(failed_scenarios)}")
        sys.exit(1)

    print("\n[PASSED] ALL SCENARIOS PASSED!")
    sys.exit(0)


def run_scenario(args, scenario):
    """Run a single scenario."""
    # 0. Runtime Mode Setup (Optional)
    if "runtime_mode" in scenario:
        print(f"Switching runtime mode to: {scenario['runtime_mode']}")
        run_esb(["mode", "set", scenario["runtime_mode"]])

    # 1. Environment Setup
    base_env_path = PROJECT_ROOT / "tests" / ".env.test"
    if base_env_path.exists():
        load_dotenv(base_env_path, override=False)
        print(f"Loaded base environment from: {base_env_path}")
    else:
        print(f"Warning: Base environment file not found: {base_env_path}")

    # Ignore args.env_file and use scenario['env_file'].
    env_path = PROJECT_ROOT / scenario["env_file"]
    if env_path.exists():
        load_dotenv(env_path, override=True)  # Override previous scenario vars
        print(f"Loaded environment from: {env_path}")
    else:
        print(f"Warning: Environment file not found: {env_path}")

    # Reload env vars into dict to pass to subprocess
    # NOTE: os.environ is updated by load_dotenv, but we explicitly fetch fresh copy
    env = os.environ.copy()

    # ESB_TEMPLATE etc setup (Shared logic)
    esb_template = os.getenv("ESB_TEMPLATE", "tests/fixtures/template.yaml")
    env["ESB_TEMPLATE"] = str(PROJECT_ROOT / esb_template)

    # Environment Isolation Logic
    from tools.cli import config as cli_config
    env_name = os.environ.get("ESB_ENV", "default")
    env["ESB_ENV"] = env_name
    
    # Calculate ports and subnets to inject into pytest environment
    env.update(cli_config.get_port_mapping(env_name))
    env.update(cli_config.get_subnet_config(env_name))

    print(f"DEBUG: env_path={env_path}, exists={env_path.exists()}")
    print(f"DEBUG: Running in environment: {env_name}")
    print(f"DEBUG: Gateway Port: {env.get('ESB_PORT_GATEWAY_HTTPS')}")

    # Map ESB CLI ports to Test Suite expected variables
    env["GATEWAY_PORT"] = env.get("ESB_PORT_GATEWAY_HTTPS", "443")
    env["VICTORIALOGS_PORT"] = env.get("ESB_PORT_VICTORIALOGS", "9428")
    env["GATEWAY_URL"] = f"https://localhost:{env['GATEWAY_PORT']}"
    env["VICTORIALOGS_URL"] = f"http://localhost:{env['VICTORIALOGS_PORT']}"
    env["VICTORIALOGS_QUERY_URL"] = env["VICTORIALOGS_URL"]

    # Update current process env for helper calls
    os.environ.update(env)

    ensure_firecracker_node_up()

    # Define common override arguments
    override_args = ["-f", "tests/docker-compose.test.yml"]
    env_args = ["--env", env_name]

    try:
        # 2. Reset / Build
        # Stop containers from previous scenario to release ports/resources
        if args.reset:
             print(f"DEBUG: Reset requested. Running esb down for scenario {scenario['name']}")
             run_esb(["down", "-v"] + override_args + env_args, check=True)
             
             import shutil
             # Note: Checking global fixtures dir might be risky if concurrent tests delete it?
             # Ideally fixtures should be scoped too, but for verify we skip reset usually.
             esb_dir = PROJECT_ROOT / "tests" / "fixtures" / ".esb"
             if esb_dir.exists():
                 shutil.rmtree(esb_dir)
             run_esb(["build", "--no-cache"] + override_args + env_args)
        else:
             # Default behavior: Stop to preserve data/state (unless build requested, but even then stop is safer)
             print(f"DEBUG: Stopping previous services (preserving state)...")
             run_esb(["stop"] + override_args + env_args, check=True)
            
        if args.build and not args.reset:
             run_esb(["build", "--no-cache"] + override_args + env_args)

        # 3. UP
        up_args = ["up", "--detach", "--wait"] + override_args + env_args
        if args.build or args.reset:
            up_args.append("--build")

        run_esb(up_args)

        # 4. Run Tests
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
            run_esb(["down"] + override_args + env_args)
        # If not cleanup, we leave containers running for debugging last scenario
        # But next scenario execution will force down anyway.


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "run_scenario":
        # Internal call wrapper if needed? No, just call main().
        pass
    main()
