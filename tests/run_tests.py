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
    parser.add_argument("--build", action="store_true", help="Rebuild images before running tests")
    parser.add_argument("--cleanup", action="store_true", help="Cleanup environment after successful tests")
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
    args = parser.parse_args()

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
            "esb_env": args.profile, # Use profile name as environment name
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

    # Load Base Environment (Global)
    base_env_path = PROJECT_ROOT / "tests" / ".env.test"
    if base_env_path.exists():
        load_dotenv(base_env_path, override=False)
        print(f"Loaded base environment from: {base_env_path}")
    
    # --- Test Matrix Execution ---

    print("\nStarting Full E2E Test Suite (Matrix-Based)\n")
    failed_entries = []



    initialized_profiles = set()

    for entry in matrix:
        suite_name = entry["suite"]
        profile_names = entry["profiles"]

        if suite_name not in suites:
            print(f"[ERROR] Suite '{suite_name}' not defined in suites.")
            continue

        suite_def = suites[suite_name]
        
        for profile_name in profile_names:
            # Filter by profile if specified (Matrix Run)
            if args.profile and profile_name != args.profile:
                continue
            
            if profile_name not in profiles:
                print(f"[ERROR] Profile '{profile_name}' not defined in profiles.")
                continue

            profile_def = profiles[profile_name]
            
            # Construct Scenario Object for compatibility with run_scenario
            # Dynamic ESB_ENV Calculation
            # Use profile name directly as environment name (e.g. "e2e-containerd")
            target_env = profile_name
            
            scenario = {
                "name": f"{suite_name} on {profile_name}",
                "env_file": profile_def.get("env_file"),
                "runtime_mode": profile_def.get("mode"),
                "esb_env": target_env,
                "targets": suite_def.get("targets", []),
                "exclude": suite_def.get("exclude", []),
            }

            print(f"\n[Matrix] Running Suite: '{suite_name}' on Profile: '{profile_name}'")
            print(f"         > Environment: {target_env}")
            
            # Determine if we should upgrade/reset environment
            # Only reset if requested AND not yet initialized for this profile
            should_reset = args.reset and (profile_name not in initialized_profiles)
            
            # Determine if we should build
            # Only build if requested AND not yet initialized
            should_build = args.build and (profile_name not in initialized_profiles)
            
            # Determine cleanup
            # Ideally only cleanup at the very end of all suites for this profile?
            # For now, let's DISABLE cleanup between suites of same profile to preserve state.
            # But wait, matrix iterates by Suite then Profile.
            # Suite A [P1, P2] -> Suite B [P1, P2]
            # When Suite A P1 finishes, we keep it. Suite A P2 runs (different env).
            # When Suite B P1 runs, we reuse P1 env.
            should_cleanup = args.cleanup # This might need nuanced logic if we want to clean at VERY end.
            # For this specific fix (Smoke -> Main), we explicitly WANT to reuse.
            # So if we are reusing, we must NOT have cleaned up previously.
            # Implication: The previous run_scenario must NOT have cleaned up.
            
            # Update: run_scenario logic controls cleanup based on arg.
            # We need to pass specific instructions to run_scenario.
            
            scenario_args = {
                "perform_reset": should_reset,
                "perform_build": should_build,
                # If we plan to reuse this profile later, we should arguably NOT cleanup yet?
                # But detecting "is this the last time P1 is used" is complex.
                # Let's rely on the fact that if --cleanup is False (default), we keep it.
                # If --cleanup is True, user wants it gone. 
                # BUT for Smoke->Suite flow, user likely wants --cleanup set but applied only at end.
                # Let's enforce: If it's initialized (reused), don't reset.
            }
            
            scenario = {
                "name": f"{suite_name} on {profile_name}",
                "env_file": profile_def.get("env_file"),
                "runtime_mode": profile_def.get("mode"),
                "esb_env": target_env,
                "targets": suite_def.get("targets", []),
                "exclude": suite_def.get("exclude", []),
                **scenario_args
            }

            print(f"\n[Matrix] Running Suite: '{suite_name}' on Profile: '{profile_name}'")
            print(f"         > Environment: {target_env}")
            print(f"         > Action: Reset={should_reset}, Build={should_build}")
            
            try:
                run_scenario(args, scenario)
                # Mark as initialized after successful (or attempted) run
                initialized_profiles.add(profile_name)
            except SystemExit as e:
                if e.code != 0:
                    print(f"\n[FAILED] {scenario['name']} FAILED.")
                    failed_entries.append(scenario["name"])
                else:
                    print(f"\n[PASSED] {scenario['name']} PASSED.")
            except Exception as e:
                print(f"\n[FAILED] {scenario['name']} FAILED with exception: {e}")
                failed_entries.append(scenario["name"])

    if failed_entries:
        print(f"\n[FAILED] The following matrix entries failed: {', '.join(failed_entries)}")
        sys.exit(1)

    print("\n[PASSED] ALL MATRIX ENTRIES PASSED!")
    sys.exit(0)


def run_scenario(args, scenario):
    """Run a single scenario."""
    
    # Determine actions based on scenario overrides or global args
    do_reset = scenario.get("perform_reset", args.reset)
    do_build = scenario.get("perform_build", args.build)
    do_cleanup = args.cleanup # Currently global only
    
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
    
    # Use scenario-specific ESB_ENV (Matrix) or fallback to process env (Legacy)
    env_name = scenario.get("esb_env", os.environ.get("ESB_ENV", "default"))
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
    template_args = ["--template", env["ESB_TEMPLATE"]]

    try:
        # 2. Reset / Build
        # Stop containers from previous scenario to release ports/resources
        if do_reset:
            print(f"DEBUG: Reset requested. Running esb down for scenario {scenario['name']}")
            run_esb(template_args + ["down", "-v"] + override_args + env_args, check=True)

            import shutil
            # Note: Checking global fixtures dir might be risky if concurrent tests delete it?
            # Ideally fixtures should be scoped too, but for verify we skip reset usually.
            esb_dir = PROJECT_ROOT / "tests" / "fixtures" / ".esb"
            if esb_dir.exists():
                shutil.rmtree(esb_dir)
            run_esb(template_args + ["build", "--no-cache"] + override_args + env_args)
        else:
             # Default behavior: Stop to preserve data/state (unless build requested, but even then stop is safer)
             # MODIFICATION: If we are reusing the environment (Smoke -> Main), do we even STOP?
             # Smoke test leaves containers UP. Main test expects them UP (or restarts them).
             # If we stop here, we lose the "Smoke passed state" (though data persists).
             # But run_esb("up") will restart them anyway.
             # However, stopping allows mode switching (firecracker <-> containerd) without conflict 
             # if they share resources (like ports). 
             # Since we have distinct environments (e2e-containerd vs e2e-firecracker),
             # and they use DIFFERENT ports (mapped in config), we might not strictly need to stop.
             # BUT, to be safe and consistent:
             if scenario.get("perform_reset") is False:
                 print(f"DEBUG: Skipping Reset/Stop (Reusing environment)...")
             else:
                 print(f"DEBUG: Stopping previous services (preserving state)...")
                 run_esb(template_args + ["stop"] + override_args + env_args, check=True)
            
        if do_build and not do_reset:
             run_esb(template_args + ["build", "--no-cache"] + override_args + env_args)

        # 3. UP
        up_args = template_args + ["up", "--detach", "--wait"] + override_args + env_args
        if do_build or do_reset:
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
            run_esb(template_args + ["down"] + override_args + env_args)
        # If not cleanup, we leave containers running for debugging last scenario
        # But next scenario execution will force down anyway.


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "run_scenario":
        # Internal call wrapper if needed? No, just call main().
        pass
    main()
