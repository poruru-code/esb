# Where: e2e/runner/cleanup.py
# What: Cleanup helpers for Docker resources created by E2E runs.
# Why: Keep reset/cleanup logic isolated from execution flow.
from __future__ import annotations

import json
import os
import subprocess
from typing import Callable

from e2e.runner.utils import BRAND_SLUG


def _emit(message: str, log: Callable[[str], None], printer: Callable[[str], None] | None) -> None:
    log(message)
    if printer:
        printer(message)


def _shared_registry_container_name() -> str:
    return os.environ.get("REGISTRY_CONTAINER_NAME", f"{BRAND_SLUG}-infra-registry")


def thorough_cleanup(
    env_name: str,
    *,
    log: Callable[[str], None],
    printer: Callable[[str], None] | None = None,
) -> None:
    """Exhaustively remove Docker resources associated with an environment."""
    project_label = f"{BRAND_SLUG}-{env_name}"

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
            _emit(f"  - Removing containers for {env_name} ({filt})...", log, printer)
            subprocess.run(["docker", "rm", "-f"] + container_ids, capture_output=True)

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
            _emit(f"  - Removing networks for {env_name} ({filt})...", log, printer)
            subprocess.run(["docker", "network", "rm"] + network_ids, capture_output=True)

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
            _emit(f"  - Removing volumes for {env_name} ({filt})...", log, printer)
            subprocess.run(["docker", "volume", "rm"] + to_remove, capture_output=True)
            seen_volumes.update(to_remove)


def cleanup_managed_images(
    env_name: str,
    project_name: str,
    *,
    log: Callable[[str], None],
    printer: Callable[[str], None] | None = None,
) -> None:
    """Remove ESB-managed images associated with an environment."""
    label_prefix = f"com.{BRAND_SLUG}"
    project_label = f"{project_name}-{env_name}"
    cmd = [
        "docker",
        "images",
        "-q",
        "--filter",
        f"label={label_prefix}.managed=true",
        "--filter",
        f"label={label_prefix}.project={project_label}",
        "--filter",
        f"label={label_prefix}.env={env_name}",
    ]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        _emit(
            f"[WARN] Failed to list images for cleanup ({env_name}): {result.stderr.strip()}",
            log,
            printer,
        )
        return
    image_ids = [img.strip() for img in result.stdout.splitlines() if img.strip()]
    if not image_ids:
        return
    _emit(f"  - Removing managed images for {env_name} ({len(image_ids)} images)...", log, printer)
    subprocess.run(["docker", "rmi", "-f"] + image_ids, check=False, capture_output=True)


def isolate_external_network(
    project_label: str,
    *,
    log: Callable[[str], None],
    printer: Callable[[str], None] | None = None,
) -> None:
    """Detach non-project containers from the external network to avoid DNS conflicts."""
    network_name = f"{project_label}-external"
    shared_registry = _shared_registry_container_name()
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
        if not name or name.startswith(project_label) or name == shared_registry:
            continue
        _emit(f"  - Detaching {name} from {network_name}...", log, printer)
        subprocess.run(
            ["docker", "network", "disconnect", "-f", network_name, name],
            capture_output=True,
        )
