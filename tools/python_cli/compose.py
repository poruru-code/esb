# Where: tools/python_cli/compose.py
# What: Compose file selection helpers for CLI commands.
# Why: Switch docker compose invocation based on the runtime mode.
from pathlib import Path

from tools.python_cli import config as cli_config
from tools.python_cli import runtime_mode


def resolve_compose_files(mode: str | None = None, target: str = "control") -> list[Path]:
    resolved_mode = mode or runtime_mode.get_mode()
    files = []

    if target == "control":
        # Base setup (Control Plane) + Worker (Compute Plane)
        files.extend([cli_config.COMPOSE_BASE_FILE, cli_config.COMPOSE_WORKER_FILE])

        if resolved_mode == cli_config.ESB_MODE_FIRECRACKER:
            files.append(cli_config.COMPOSE_REGISTRY_FILE)
            files.append(cli_config.COMPOSE_FC_FILE)
        elif resolved_mode == cli_config.ESB_MODE_CONTAINERD:
            files.append(cli_config.COMPOSE_REGISTRY_FILE)
            files.append(cli_config.COMPOSE_CONTAINERD_FILE)
        elif resolved_mode == cli_config.ESB_MODE_DOCKER:
            files.append(cli_config.COMPOSE_DOCKER_FILE)
        else:
            # Fallback to docker for unknown modes
            files.append(cli_config.COMPOSE_DOCKER_FILE)

    elif target == "compute":
        # Compute only (Worker + Runtime Adapter)
        files.append(cli_config.COMPOSE_WORKER_FILE)

        if resolved_mode == cli_config.ESB_MODE_FIRECRACKER:
            files.append(cli_config.COMPOSE_FC_FILE)
        elif resolved_mode == cli_config.ESB_MODE_CONTAINERD:
            files.append(cli_config.COMPOSE_CONTAINERD_FILE)
        elif resolved_mode == cli_config.ESB_MODE_DOCKER:
            files.append(cli_config.COMPOSE_DOCKER_FILE)
        else:
            files.append(cli_config.COMPOSE_DOCKER_FILE)

    else:
        raise ValueError(f"Unsupported compose target: {target}")

    # Validate existence
    for path in files:
        if not path.exists():
            raise FileNotFoundError(f"Missing compose file: {path}")

    return files


def build_compose_command(
    args: list[str],
    mode: str | None = None,
    target: str = "control",
    extra_files: list[str] | None = None,
    project_name: str | None = None,
) -> list[str]:
    cmd = ["docker", "compose"]

    if project_name:
        cmd.extend(["-p", project_name])

    for path in resolve_compose_files(mode, target=target):
        cmd.extend(["-f", str(path)])

    if extra_files:
        for path in extra_files:
            if path:
                cmd.extend(["-f", path])

    cmd.extend(args)
    return cmd
