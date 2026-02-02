# Where: e2e/runner/buildx.py
# What: Docker Buildx helper for E2E runs.
# Why: Keep buildx management isolated from runner orchestration.
from __future__ import annotations

import logging
import subprocess

logger = logging.getLogger(__name__)


def ensure_buildx_builder(builder_name: str, network_mode: str = "host") -> None:
    if not builder_name:
        return
    inspect_cmd = [
        "docker",
        "buildx",
        "inspect",
        "--builder",
        builder_name,
    ]
    result = subprocess.run(inspect_cmd, capture_output=True, text=True)
    if result.returncode == 0:
        if network_mode:
            inspect_net_cmd = [
                "docker",
                "inspect",
                "-f",
                "{{.HostConfig.NetworkMode}}",
                f"buildx_buildkit_{builder_name}0",
            ]
            net_result = subprocess.run(inspect_net_cmd, capture_output=True, text=True)
            current = net_result.stdout.strip() if net_result.returncode == 0 else ""
            if current and current != network_mode:
                logger.warning(
                    "buildx builder '%s' network mode is '%s', expected '%s'. "
                    "Using existing builder.",
                    builder_name,
                    current,
                    network_mode,
                )
        subprocess.run(["docker", "buildx", "use", builder_name], capture_output=True)
        return

    create_cmd = [
        "docker",
        "buildx",
        "create",
        "--name",
        builder_name,
        "--driver",
        "docker-container",
        "--use",
        "--bootstrap",
    ]
    if network_mode:
        create_cmd.extend(["--driver-opt", f"network={network_mode}"])
    create_result = subprocess.run(create_cmd, capture_output=True, text=True)
    if create_result.returncode == 0:
        return
    combined = (create_result.stdout + create_result.stderr).lower()
    if "existing instance" in combined or "already exists" in combined:
        subprocess.run(["docker", "buildx", "use", builder_name], capture_output=True)
        return
    create_result.check_returncode()
