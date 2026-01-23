import json
import os
import subprocess
import sys
import time
from concurrent.futures import ProcessPoolExecutor, as_completed
from pathlib import Path
from typing import Any, Tuple

import requests
import urllib3
from dotenv import load_dotenv

from e2e.runner import constants
from e2e.runner.env import (
    apply_ports_to_env,
    calculate_runtime_env,
    discover_ports,
    ensure_firecracker_node_up,
)
from e2e.runner.utils import (
    BRAND_SLUG,
    E2E_STATE_ROOT,
    PROJECT_ROOT,
    env_key,
    run_esb,
)

# Terminal colors for parallel output
COLORS = [
    "\033[36m",  # Cyan
    "\033[32m",  # Green
    "\033[34m",  # Blue
    "\033[35m",  # Magenta
    "\033[33m",  # Yellow
]
COLOR_RESET = "\033[0m"


def _registry_port(project: str, compose_file: Path) -> int | None:
    """Resolve host port mapped to registry:5010 for the given compose project."""
    try:
        result = subprocess.run(
            [
                "docker",
                "compose",
                "-p",
                project,
                "-f",
                str(compose_file),
                "port",
                "registry",
                "5010",
            ],
            capture_output=True,
            text=True,
        )
        if result.returncode != 0 or not result.stdout.strip():
            return None
        return int(result.stdout.strip().split(":")[-1])
    except Exception:
        return None


def resolve_compose_file(scenario: dict[str, Any], mode: str) -> Path:
    env_dir = scenario.get("env_dir")
    if env_dir:
        compose_path = PROJECT_ROOT / env_dir / "docker-compose.yml"
        if compose_path.exists():
            return compose_path
        raise FileNotFoundError(f"Compose file not found in env_dir: {compose_path}")
    return PROJECT_ROOT / f"docker-compose.{mode}.yml"


def isolate_external_network(project_label: str) -> None:
    """Detach non-project containers from the external network to avoid DNS conflicts."""
    network_name = f"{project_label}-external"
    result = subprocess.run(
        ["docker", "network", "inspect", network_name],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        return
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        return
    if not data:
        return
    containers = data[0].get("Containers") or {}
    for entry in containers.values():
        name = entry.get("Name", "")
        if not name or name.startswith(project_label):
            continue
        subprocess.run(
            ["docker", "network", "disconnect", "-f", network_name, name],
            capture_output=True,
        )


def _load_function_names(env_name: str) -> list[str]:
    config_path = E2E_STATE_ROOT / env_name / "config" / "functions.yml"
    if not config_path.exists():
        return []
    try:
        import yaml
    except Exception:
        return []
    try:
        data = yaml.safe_load(config_path.read_text()) or {}
    except Exception:
        return []
    functions = data.get("functions", {})
    if isinstance(functions, dict):
        return sorted([name for name in functions.keys() if isinstance(name, str)])
    return []


def _pick_manifest_digest(index: dict) -> str | None:
    manifests = index.get("manifests", [])
    if not isinstance(manifests, list) or not manifests:
        return None
    for entry in manifests:
        platform = entry.get("platform") or {}
        if platform.get("os") == "linux" and platform.get("architecture") == "amd64":
            return entry.get("digest")
    return manifests[0].get("digest")


def verify_registry_images(env_name: str, project: str, mode: str, compose_file: Path) -> None:
    """Validate that registry blobs exist for built function images."""
    if mode not in ("containerd", "firecracker"):
        return

    port = _registry_port(project, compose_file)
    if port is None:
        print("[WARN] Registry port not discovered; skipping registry integrity check.")
        return

    function_names = _load_function_names(env_name)
    if not function_names:
        print("[WARN] functions.yml not found or empty; skipping registry integrity check.")
        return

    base_url = f"https://localhost:{port}"
    missing: dict[str, list[str]] = {}

    headers_index = {"Accept": "application/vnd.oci.image.index.v1+json"}
    headers_manifest = {"Accept": "application/vnd.oci.image.manifest.v1+json"}

    for name in function_names:
        tag = mode
        try:
            resp = requests.get(
                f"{base_url}/v2/{name}/manifests/{tag}",
                headers=headers_index,
                timeout=10,
                verify=False,
            )
        except Exception as exc:
            missing.setdefault(name, []).append(f"manifest fetch failed: {exc}")
            continue

        if resp.status_code != 200:
            missing.setdefault(name, []).append(f"manifest status {resp.status_code}")
            continue

        try:
            index = resp.json()
        except json.JSONDecodeError:
            missing.setdefault(name, []).append("manifest is not valid JSON")
            continue

        digest = _pick_manifest_digest(index)
        if not digest:
            missing.setdefault(name, []).append("manifest digest not found")
            continue

        resp = requests.get(
            f"{base_url}/v2/{name}/manifests/{digest}",
            headers=headers_manifest,
            timeout=10,
            verify=False,
        )
        if resp.status_code != 200:
            missing.setdefault(name, []).append(f"image manifest status {resp.status_code}")
            continue

        try:
            manifest = resp.json()
        except json.JSONDecodeError:
            missing.setdefault(name, []).append("image manifest is not valid JSON")
            continue

        layers = manifest.get("layers", [])
        for layer in layers:
            blob = layer.get("digest")
            if not blob:
                continue
            head = requests.head(
                f"{base_url}/v2/{name}/blobs/{blob}",
                timeout=10,
                verify=False,
            )
            if head.status_code != 200:
                missing.setdefault(name, []).append(blob)

    if missing:
        print("\n[ERROR] Registry is missing image blobs; containerd pulls will fail.")
        for name, blobs in sorted(missing.items()):
            sample = ", ".join(blobs[:5])
            suffix = "" if len(blobs) <= 5 else f" (+{len(blobs) - 5} more)"
            print(f"  - {name}: {sample}{suffix}")
        raise RuntimeError("registry missing blobs")


def thorough_cleanup(env_name: str):
    """Exhaustively remove Docker resources associated with an environment."""
    project_label = f"{BRAND_SLUG}-{env_name}"

    # 1. Containers
    container_filters = [
        f"name={env_name}",
        f"label=com.docker.compose.project={project_label}",
    ]
    for filt in container_filters:
        result = subprocess.run(
            ["docker", "ps", "-aq", "--filter", filt],
            capture_output=True,
            text=True,
        )
        container_ids = [cid.strip() for cid in result.stdout.split() if cid.strip()]
        if container_ids:
            print(f"  • Removing containers for {env_name} ({filt})...")
            subprocess.run(["docker", "rm", "-f"] + container_ids, capture_output=True)

    # 2. Networks
    network_filters = [
        f"label=com.docker.compose.project={project_label}",
        f"name={project_label}-external",
        f"name={project_label}_default",
    ]
    for filt in network_filters:
        result = subprocess.run(
            ["docker", "network", "ls", "-q", "--filter", filt],
            capture_output=True,
            text=True,
        )
        network_ids = [nid.strip() for nid in result.stdout.split() if nid.strip()]
        if network_ids:
            print(f"  • Removing networks for {env_name} ({filt})...")
            # Networks might still be in use if some containers weren't properly removed
            subprocess.run(["docker", "network", "rm"] + network_ids, capture_output=True)

    # 3. Volumes
    # We check by label AND by name prefix for maximum safety
    volume_filters = [
        f"label=com.docker.compose.project={project_label}",
        f"name={project_label}_",
    ]
    seen_volumes = set()
    for filt in volume_filters:
        result = subprocess.run(
            ["docker", "volume", "ls", "-q", "--filter", filt],
            capture_output=True,
            text=True,
        )
        volume_ids = [vid.strip() for vid in result.stdout.split() if vid.strip()]
        to_remove = [v for v in volume_ids if v not in seen_volumes]
        if to_remove:
            print(f"  • Removing volumes for {env_name} ({filt})...")
            subprocess.run(["docker", "volume", "rm"] + to_remove, capture_output=True)
            seen_volumes.update(to_remove)
    # Note: Image pruning removed from per-scenario cleanup.
    # 'docker image prune' is a global operation that interferes with parallel execution.
    # Run it manually or via a post-test cleanup script instead.


def warmup_environment(env_scenarios: dict, matrix: list[dict], args):
    """
    Perform global reset and warm-up actions.
    This includes light validation to ensure shared inputs exist before parallel execution.
    """
    active_envs = list(env_scenarios.keys())
    if not active_envs:
        print("[ERROR] No active environments in matrix.")
        sys.exit(1)

    esb_template = PROJECT_ROOT / "e2e" / "fixtures" / "template.yaml"
    if not esb_template.exists():
        print(f"[ERROR] Missing E2E template: {esb_template}")
        sys.exit(1)

    print(f"\n[INIT] Using E2E template: {esb_template}")


def run_scenario(args, scenario):
    """Run a single scenario."""
    # 0. Resolve scenario-specific parameters once
    env_name = scenario.get("esb_env", os.environ.get(env_key("ENV"), "e2e-docker"))
    raw_env_file = scenario.get("env_file")
    env_file = str((PROJECT_ROOT / raw_env_file).absolute()) if raw_env_file else None
    project_name = scenario.get("esb_project", BRAND_SLUG)
    do_reset = scenario.get("perform_reset", args.reset)
    do_build = scenario.get("perform_build", args.build)
    build_only = scenario.get("build_only", False)
    env_vars_override = scenario.get("env_vars", {})

    # 0.1 Set ESB variables for resolution and safety
    # We set these in os.environ so the CLI doesn't prompt even if .env fails.
    os.environ[env_key("PROJECT")] = project_name
    os.environ[env_key("ENV")] = env_name
    os.environ[env_key("HOME")] = str((E2E_STATE_ROOT / env_name).absolute())

    # Load Scenario-Specific Env File (Required for isolation)
    if env_file:
        p = Path(env_file)
        if p.exists():
            load_dotenv(p, override=True)
            print(f"Loaded scenario environment from: {p}")
        else:
            print(f"Warning: Scenario environment file not found: {p}")
    else:
        print("Warning: No env_file specified for this scenario. Operating with system env only.")

    # 2.5 Inject Proxy Settings
    # 3. Reload env vars into a dict for passing to subprocess (pytest)
    env = os.environ.copy()

    # Capture calculated values for convenience
    env["GATEWAY_PORT"] = env.get(env_key("PORT_GATEWAY_HTTPS"), "443")
    env["VICTORIALOGS_PORT"] = env.get(env_key("PORT_VICTORIALOGS"), "9428")
    env["GATEWAY_URL"] = f"https://localhost:{env['GATEWAY_PORT']}"
    env["VICTORIALOGS_URL"] = f"http://localhost:{env['VICTORIALOGS_PORT']}"
    env["AGENT_GRPC_ADDRESS"] = f"localhost:{env.get(env_key('PORT_AGENT_GRPC'), '50051')}"
    env[env_key("PROJECT_NAME")] = f"{project_name}-{env_name}"

    # Merge scenario-specific environment variables
    env.update(env_vars_override)

    ensure_firecracker_node_up()

    # 1.5 Calculate Runtime Env (needed for reset/build too)
    mode = scenario.get("mode", "docker")
    compose_file_path = resolve_compose_file(scenario, mode)
    template_path = PROJECT_ROOT / "e2e" / "fixtures" / "template.yaml"
    if not template_path.exists():
        raise FileNotFoundError(f"Missing E2E template: {template_path}")
    runtime_env = calculate_runtime_env(project_name, env_name, mode, env_file)

    did_up = False
    try:
        # 2. Reset / Build
        build_args = [
            "--template",
            str(template_path.absolute()),
            "build",
            "--env",
            env_name,
            "--mode",
            mode,
        ]
        if do_reset:
            print(f"➜ Resetting environment: {env_name}")
            # 2.1 Robust cleanup using docker compose down
            # matches the "down" logic from legacy esb up replacement
            if compose_file_path.exists():
                proj_key = f"{BRAND_SLUG}-{env_name}"
                subprocess.run(
                    [
                        "docker",
                        "compose",
                        "--project-name",
                        proj_key,
                        "--file",
                        str(compose_file_path),
                        "down",
                        "--volumes",
                        "--remove-orphans",
                    ],
                    capture_output=True,
                )
            # Fallback to manual cleanup just in case (e.g. if compose file invalid or project name mismatch previously)
            thorough_cleanup(env_name)
            isolate_external_network(f"{BRAND_SLUG}-{env_name}")

            # 2.2 Clean artifact directory for this environment
            env_state_dir = E2E_STATE_ROOT / env_name
            if env_state_dir.exists():
                print(f"  • Cleaning artifact directory: {env_state_dir}")
                import shutil

                shutil.rmtree(env_state_dir)

            # Re-generate configurations and build images via ESB build
            # IMPORTANT: Pass runtime_env so (branding) is respected!
            build_env = runtime_env.copy()
            # Ensure build uses the same compose project as the runtime stack.
            # Otherwise containerd images get pushed to the wrong registry.
            build_env["PROJECT_NAME"] = f"{project_name}-{env_name}"

            run_esb(
                build_args + ["--no-cache"],
                env_file=env_file,
                verbose=args.verbose,
                env=build_env,
            )
            verify_registry_images(
                env_name,
                f"{project_name}-{env_name}",
                mode,
                compose_file_path,
            )

            if build_only:
                return True

            # Manual orchestration will handle starting the services in Step 3
            did_up = False
        elif do_build:
            if not args.verbose:
                print(f"➜ Building environment: {env_name}... ", end="", flush=True)
            else:
                print(f"➜ Building environment: {env_name}")
            run_esb(build_args + ["--no-cache"], env_file=env_file, verbose=args.verbose)
            verify_registry_images(
                env_name,
                f"{project_name}-{env_name}",
                mode,
                compose_file_path,
            )
            if not args.verbose:
                print("Done")
        else:
            if not args.verbose:
                print(f"➜ Preparing environment: {env_name}... ", end="", flush=True)
            else:
                print(f"➜ Preparing environment: {env_name}")
            # In Zero-Config, we just rebuild if needed. stop/sync are gone.
            run_esb(build_args, env_file=env_file, verbose=args.verbose)
            verify_registry_images(
                env_name,
                f"{project_name}-{env_name}",
                mode,
                compose_file_path,
            )
            if not args.verbose:
                print("Done")

        # If build_only, skip UP and tests
        if build_only:
            return

        # 3. UP (Manual Orchestration)
        if not did_up:
            # Critical: Override PROJECT_NAME to include env suffix for isolation (e.g. esb-e2e-docker)
            # This matches the logic in the Go CLI builder and ensures container names are unique.
            runtime_env["PROJECT_NAME"] = f"{project_name}-{env_name}"

            # Merge with existing system/process env to ensure PATH etc are preserved
            compose_env = os.environ.copy()
            compose_env.update(runtime_env)
            compose_env.update(env_vars_override)

            # Pass RESOURCES_YML to docker compose so provisioner can find it
            resources_yml_path = E2E_STATE_ROOT / env_name / "config" / "resources.yml"
            compose_env["RESOURCES_YML"] = str(resources_yml_path.absolute())

            if not compose_file_path.exists():
                print(f"[WARN] Compose file not found at {compose_file_path}.")
                raise FileNotFoundError(f"Compose file not found: {compose_file_path}")

            compose_cmd = [
                "docker",
                "compose",
                "--project-name",
                f"{BRAND_SLUG}-{env_name}",
                "--file",
                str(compose_file_path),
                "up",
                "--detach",
            ]

            if args.verbose:
                print(f"Running: {' '.join(compose_cmd)}")

            subprocess.run(compose_cmd, check=True, env=compose_env)

            # Sync is gone. Zero-Config provisioner service handles it.
            # We just need to discover ports for the host-side testing.
            ports = discover_ports(f"{BRAND_SLUG}-{env_name}", compose_file_path)

            # Wait for Gateway readiness (parity with legacy esb up)
            wait_for_gateway(env_name, verbose=args.verbose, ports=ports)

            # --- USER REQUEST: Print generated info ---
            if ports:
                print(f"\n🔌 Discovered Ports for {env_name}:")
                # Sort for stable output
                for k in sorted(ports.keys()):
                    print(f"   {k}: {ports[k]}")

            # Try to read generated credentials from the provisioner or env
            # The CLI generates them into .env, but we might be running in a clean env.
            # However, the compose environment `compose_env` has them if they were generated/loaded.
            # But `runtime_env` was calculated before build.

            # Actually, `esb build` might generate new credentials if they were missing.
            # We should try to read the .env file from the state dir if it exists.
            state_env_file = E2E_STATE_ROOT / env_name / "config" / ".env"
            if state_env_file.exists():
                print(f"\n🔑 Credentials (from {state_env_file}):")
                with open(state_env_file, "r") as f:
                    for line in f:
                        line = line.strip()
                        if line and not line.startswith("#"):
                            if any(k in line for k in ["AUTH_", "SECRET", "KEY"]):
                                print(f"   {line}")
                            elif "VICTORIALOGS" in line:  # Useful to see full URLs if present
                                print(f"   {line}")
            print("")
            # ------------------------------------------

            # Apply discovered ports to OS environment and local env dict for pytest
            apply_ports_to_env(ports)
            env.update({k: str(v) for k, v in ports.items()})
            # Re-apply composite variables that depend on ports
            if env_key(constants.PORT_GATEWAY_HTTPS) in ports:
                env["GATEWAY_PORT"] = str(ports[env_key(constants.PORT_GATEWAY_HTTPS)])
                env["GATEWAY_URL"] = f"https://localhost:{env['GATEWAY_PORT']}"
            if env_key(constants.PORT_VICTORIALOGS) in ports:
                env["VICTORIALOGS_PORT"] = str(ports[env_key(constants.PORT_VICTORIALOGS)])
                env["VICTORIALOGS_URL"] = f"http://localhost:{env['VICTORIALOGS_PORT']}"
        if ports:
            apply_ports_to_env(ports)

            # Update env dict for pytest subprocess
            for k, v in ports.items():
                env[k] = str(v)

            env["GATEWAY_PORT"] = str(
                ports.get(env_key("PORT_GATEWAY_HTTPS"), env.get("GATEWAY_PORT", "443"))
            )
            env["VICTORIALOGS_PORT"] = str(
                ports.get(env_key("PORT_VICTORIALOGS"), env.get("VICTORIALOGS_PORT", "9428"))
            )
            env["GATEWAY_URL"] = f"https://localhost:{env['GATEWAY_PORT']}"
            env["VICTORIALOGS_URL"] = f"http://localhost:{env['VICTORIALOGS_PORT']}"
            agent_key = env_key("PORT_AGENT_GRPC")
            if agent_key in ports:
                env["AGENT_GRPC_ADDRESS"] = f"localhost:{ports[agent_key]}"

            agent_metrics_key = env_key("PORT_AGENT_METRICS")
            if agent_metrics_key in ports:
                env["AGENT_METRICS_PORT"] = str(ports[agent_metrics_key])
                env["AGENT_METRICS_URL"] = f"http://localhost:{ports[agent_metrics_key]}"

        from e2e.runner.env import apply_gateway_env_from_container

        apply_gateway_env_from_container(env, f"{BRAND_SLUG}-{env_name}")

        # 4. Run Tests
        if not scenario["targets"]:
            # No test targets specified, skip test execution
            return

        print(f"\\n=== Running Tests for {scenario['name']} ===\n")

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
    except RuntimeError as e:
        print(f"Error executing command: {e}")
        sys.exit(1)

    finally:
        if args.cleanup:
            if not args.verbose:
                print(f"➜ Cleaning up environment: {env_name}... ", end="", flush=True)
            else:
                print(f"➜ Cleaning up environment: {env_name}")
            # esb down is gone, use docker compose directly
            proj_key = f"{BRAND_SLUG}-{env_name}"
            subprocess.run(
                ["docker", "compose", "-p", proj_key, "-f", str(compose_file_path), "down"],
                capture_output=True,
            )
            if not args.verbose:
                print("Done")


def run_profile_subprocess(
    profile_name: str,
    cmd: list[str],
    color_code: str = "",
    verbose: bool = False,
    label_width: int = 0,
) -> Tuple[int, str]:
    """Run a profile in a subprocess and stream output with prefix."""
    label = f"[{profile_name}]"
    if label_width > 0:
        label = label.ljust(label_width)
    prefix = f"{color_code}{label}{COLOR_RESET}"

    # Inject flags to force non-interactive behavior
    env = os.environ.copy()
    env["TERM"] = "dumb"
    env[env_key("INTERACTIVE")] = "0"
    env["E2E_WORKER"] = "1"

    process = subprocess.Popen(
        cmd,
        cwd=PROJECT_ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        stdin=subprocess.DEVNULL,
        text=True,
        bufsize=1,  # Line buffered
        env=env,
    )

    output_lines = []
    tests_started = False
    in_special_block = False
    last_line_was_blank = True
    early_failure = False
    early_failure_patterns = (
        "Error executing command:",
        "ERROR: failed to build",
        "failed to solve:",
    )

    # Read output line by line as it becomes available
    try:
        if process.stdout:
            for line in iter(process.stdout.readline, ""):
                clean_line = line.rstrip()
                if clean_line:
                    if "test session starts" in clean_line:
                        tests_started = True

                    # Detect special info blocks (Auth credentials and Discovered Ports)
                    # Starts with Key or Plug emoji
                    is_special_header = clean_line.startswith("🔑") or clean_line.startswith("🔌")
                    if is_special_header:
                        in_special_block = True

                    should_print = (
                        verbose or tests_started or clean_line.startswith("➜") or in_special_block
                    )

                    if should_print:
                        print(f"{prefix} {clean_line}", flush=True)
                        last_line_was_blank = False

                    # End of special block if we encounter a new progress line
                    # or if the line is not indented (and not the header itself)
                    if in_special_block and not is_special_header:
                        if clean_line.startswith("➜") or not clean_line.startswith(" "):
                            in_special_block = False

                    if not tests_started and any(
                        pat in clean_line for pat in early_failure_patterns
                    ):
                        early_failure = True
                        print(f"{prefix} ➜ Build failed; stopping this environment.", flush=True)
                        process.terminate()
                        break
                else:
                    # Empty line terminates a block
                    if in_special_block:
                        in_special_block = False

                    # Preserve blank lines for structure (e.g. before BlockStart)
                    if not last_line_was_blank:
                        print(prefix, flush=True)
                        last_line_was_blank = True

                output_lines.append(line)

    except Exception as e:
        print(f"{prefix} Error reading output: {e}")

    try:
        returncode = process.wait(timeout=15)
    except subprocess.TimeoutExpired:
        if early_failure:
            process.kill()
        returncode = process.wait()

    if returncode != 0 and not verbose and not tests_started:
        print(f"{prefix} ➜ Subprocess failed before tests started. Printing cached logs...\n")
        for line in output_lines:
            print(f"{prefix} {line.rstrip()}", flush=True)

    # Write output to a log file for debugging
    log_file = PROJECT_ROOT / "e2e" / f".parallel-{profile_name}.log"
    with open(log_file, "w", encoding="utf-8") as f:
        f.write(f"=== {profile_name} combined output ===\n")
        f.writelines(output_lines)

    return returncode, "".join(output_lines)


def run_profiles_with_executor(
    env_scenarios: dict[str, dict[str, Any]],
    reset: bool,
    build: bool,
    cleanup: bool,
    fail_fast: bool,
    max_workers: int,
    verbose: bool = False,
) -> dict[str, tuple[bool, list[str]]]:
    """
    Run environments using a ProcessPoolExecutor.
    If max_workers is 1, they run sequentially but still in subprocesses for isolation.
    Returns: dict mapping env_name to (success, failed_scenario_names)
    """
    results = {}

    # Calculate max profile name length for aligned logging (+2 for brackets)
    max_label_len = max(len(p) for p in env_scenarios.keys()) + 2 if env_scenarios else 0

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
            if verbose:
                cmd.append("--verbose")

            # Determine log prefix/color
            profile_index = list(env_scenarios.keys()).index(profile_name)
            color_code = COLORS[profile_index % len(COLORS)]

            if max_workers > 1:
                print(f"[PARALLEL] Scheduling environment: {profile_name}")

            future = executor.submit(
                run_profile_subprocess, profile_name, cmd, color_code, verbose, max_label_len
            )
            future_to_profile[future] = profile_name

        # Process results as they complete
        for future in as_completed(future_to_profile):
            profile_name = future_to_profile[future]
            try:
                returncode, output = future.result()
                success = returncode == 0
                failed_list = [] if success else [profile_name]

                prefix = "[PARALLEL]" if max_workers > 1 else "[MATRIX]"

                if success:
                    print(f"✅ {prefix} Environment {profile_name} PASSED")
                else:
                    print(
                        f"❌ {prefix} Environment {profile_name} FAILED (exit code: {returncode})"
                    )

                results[profile_name] = (success, failed_list)
            except Exception as e:
                print(f"[ERROR] Environment {profile_name} FAILED with exception: {e}")
                results[profile_name] = (False, [f"Environment {profile_name} (exception)"])

    return results


def wait_for_gateway(
    env_name: str,
    timeout: float = 60.0,
    interval: float = 1.0,
    verbose: bool = False,
    ports: dict | None = None,
) -> None:
    """
    Waits for the Gateway to be ready by polling its /health endpoint.
    Parity with cli/internal/helpers/wait.go.
    """
    if not ports:
        from e2e.runner.env import load_ports

        ports = load_ports(env_name)

    gw_port = ports.get(env_key("PORT_GATEWAY_HTTPS"))
    if not gw_port:
        if verbose:
            print(f"[WARN] Gateway port not found for {env_name}, skipping readiness wait.")
        return

    url = f"https://localhost:{gw_port}/health"
    if verbose:
        print(f"➜ Waiting for Gateway readiness at {url}...")

    # Suppress certificate warnings for local dev
    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

    deadline = time.time() + timeout
    last_err = None

    while time.time() < deadline:
        try:
            # We use a short timeout for the check itself
            response = requests.get(url, timeout=2.0, verify=False)
            if response.status_code == 200:
                if verbose:
                    print(f"✓ Gateway is ready ({response.status_code})")
                return
            last_err = f"Status code {response.status_code}"
        except requests.exceptions.RequestException as e:
            last_err = str(e)

        time.sleep(interval)

    raise RuntimeError(
        f"Gateway failed to start in time ({timeout}s) for {env_name}. Last error: {last_err}"
    )
