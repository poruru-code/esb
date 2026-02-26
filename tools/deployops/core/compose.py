"""Compose file resolution and rendering helpers."""

from __future__ import annotations

from pathlib import Path

import yaml

from tools.deployops.core.runner import CommandRunner, RunnerError


def resolve_bundle_compose_file(
    *,
    project_root: Path,
    env_file: Path,
    mode: str,
    compose_override: Path | None,
) -> Path:
    if compose_override is not None:
        compose_path = compose_override.resolve()
    else:
        candidate = env_file.resolve().parent / "docker-compose.yml"
        if candidate.is_file():
            compose_path = candidate
        else:
            normalized_mode = mode.strip().lower()
            if normalized_mode == "docker":
                compose_path = (project_root / "docker-compose.docker.yml").resolve()
            elif normalized_mode == "containerd":
                compose_path = (project_root / "docker-compose.containerd.yml").resolve()
            else:
                raise RunnerError(f"unsupported artifact mode for compose fallback: {mode!r}")

    if not compose_path.is_file():
        raise FileNotFoundError(f"compose file not found: {compose_path}")
    return compose_path


def list_compose_images(
    runner: CommandRunner,
    *,
    compose_file: Path,
    env_file: Path,
) -> list[str]:
    cmd = [
        "docker",
        "compose",
        "--env-file",
        str(env_file),
        "-f",
        str(compose_file),
        "--profile",
        "deploy",
        "config",
        "--images",
    ]
    out = runner.run(cmd, capture_output=True, run_in_dry_run=True)
    images: list[str] = []
    seen: set[str] = set()
    for raw_line in out.stdout.splitlines():
        image = raw_line.strip()
        if image == "" or image in seen:
            continue
        seen.add(image)
        images.append(image)
    return images


def materialize_runtime_compose(
    runner: CommandRunner,
    *,
    compose_file: Path,
    env_file: Path,
    output_path: Path,
) -> None:
    cmd = [
        "docker",
        "compose",
        "--env-file",
        str(env_file),
        "-f",
        str(compose_file),
        "--profile",
        "deploy",
        "config",
    ]
    rendered = runner.run(cmd, capture_output=True).stdout
    output_path.write_text(rendered, encoding="utf-8")

    data = yaml.safe_load(output_path.read_text(encoding="utf-8")) or {}
    services = data.get("services")
    if isinstance(services, dict):
        for service in services.values():
            if isinstance(service, dict):
                service.pop("build", None)

    output_path.write_text(yaml.safe_dump(data, sort_keys=False), encoding="utf-8")


def compose_base_cmd(
    *,
    project_name: str,
    compose_file: Path,
    env_file: Path | None = None,
) -> list[str]:
    cmd = ["docker", "compose", "-p", project_name]
    if env_file is not None:
        cmd.extend(["--env-file", str(env_file)])
    cmd.extend(["-f", str(compose_file)])
    return cmd


def resolve_registry_port_for_project(
    runner: CommandRunner,
    *,
    project_name: str,
    compose_file: Path,
    env_file: Path | None,
) -> int:
    cmd = compose_base_cmd(project_name=project_name, compose_file=compose_file, env_file=env_file)
    cmd.extend(["port", "registry", "5010"])
    raw = runner.run(cmd, capture_output=True).stdout.strip().splitlines()
    if not raw:
        raise RunnerError("failed to resolve published registry port")
    token = raw[-1].strip()
    if ":" not in token:
        raise RunnerError(f"unexpected registry port output: {token!r}")
    port_token = token.rsplit(":", 1)[-1]
    if not port_token.isdigit() or int(port_token) <= 0:
        raise RunnerError(f"invalid registry port output: {token!r}")
    return int(port_token)
