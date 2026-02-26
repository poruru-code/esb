"""High-level artifact apply orchestration."""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path

import yaml

from e2e.runner.ctl_contract import (
    CTL_CAPABILITIES_SCHEMA_VERSION,
    CTL_REQUIRED_CONTRACTS,
    CTL_REQUIRED_SUBCOMMANDS,
    parse_ctl_capabilities,
)
from tools.deployops.core.artifact_manifest import load_artifact_manifest
from tools.deployops.core.branding import resolve_compose_project_name, resolve_ctl_bin
from tools.deployops.core.compose import compose_base_cmd, resolve_registry_port_for_project
from tools.deployops.core.envfile import load_env_file
from tools.deployops.core.registry import emit_registry_diagnostics, wait_for_registry_ready
from tools.deployops.core.runner import CommandRunner, RunnerError

_DEFAULT_REGISTRY_PORT = 5010


@dataclass(frozen=True)
class ApplyOptions:
    artifact: str
    compose_file: str | None
    env_file: str | None
    ctl_bin: str | None
    registry_wait_timeout: int | None
    registry_port: int | None
    project_dir: str


def execute_apply(options: ApplyOptions, runner: CommandRunner) -> int:
    project_root = Path(options.project_dir).expanduser().resolve()
    artifact_path = Path(options.artifact).expanduser().resolve()
    manifest = load_artifact_manifest(artifact_path)

    env_file = _resolve_env_file(options.env_file, project_root)
    compose_file = _resolve_compose_file(
        options.compose_file,
        project_root,
    )
    _assert_manifest_mode_matches_compose(compose_file=compose_file, manifest_mode=manifest.mode)

    env_from_file = load_env_file(env_file) if env_file else {}
    command_env = os.environ.copy()
    command_env.update(env_from_file)

    base_project = manifest.project or command_env.get("PROJECT_NAME", "").strip()
    env_name = manifest.env or command_env.get("ENV", "").strip()
    if base_project == "":
        raise RunnerError("project is empty (artifact project / PROJECT_NAME)")

    project_name = resolve_compose_project_name(base_project, env_name)

    command_env["PROJECT_NAME"] = project_name
    command_env["ENV"] = env_name

    jwt_secret = command_env.get("JWT_SECRET_KEY", "")
    if len(jwt_secret) < 32:
        if runner.dry_run:
            runner.emit(
                "[dry-run] JWT_SECRET_KEY is missing/short; "
                "runtime execution would fail without a >=32 char value"
            )
        else:
            raise RunnerError("JWT_SECRET_KEY must be set and >= 32 chars")

    ctl_bin = _resolve_ctl_bin_path(runner, options.ctl_bin, command_env)
    if runner.dry_run:
        runner.emit(f"[dry-run] skip ctl capability probe: {ctl_bin}")
    else:
        _assert_ctl_capabilities(runner, ctl_bin=ctl_bin, env=command_env)

    runner.emit(f"Using artifact: {artifact_path}")
    runner.emit(f"Bringing up stack for project '{project_name}' (ENV='{env_name}')")

    compose_up_cmd = compose_base_cmd(
        project_name=project_name,
        compose_file=compose_file,
        env_file=env_file,
    )
    compose_up_cmd.extend(["up", "-d"])
    runner.run(compose_up_cmd, stream_output=True, env=command_env)

    registry_port = _resolve_registry_port(options.registry_port, command_env)
    if registry_port == 0:
        if runner.dry_run:
            runner.emit(
                "[dry-run] PORT_REGISTRY=0 detected; "
                "published host port would be resolved from compose"
            )
        else:
            resolved = resolve_registry_port_for_project(
                runner,
                project_name=project_name,
                compose_file=compose_file,
                env_file=env_file,
            )
            registry_port = resolved
            runner.emit(f"Resolved registry host port: {registry_port}")

    effective_registry_port = registry_port if registry_port > 0 else _DEFAULT_REGISTRY_PORT
    _apply_registry_env_defaults(
        command_env,
        mode=manifest.mode,
        registry_port=effective_registry_port,
    )

    wait_timeout = _resolve_registry_wait_timeout(options.registry_wait_timeout, command_env)
    registry_host_port = command_env.get("HOST_REGISTRY_ADDR", "").strip() or (
        f"127.0.0.1:{effective_registry_port}"
    )
    runner.emit(f"Waiting for registry on http://{registry_host_port}/v2/")
    if runner.dry_run:
        runner.emit("[dry-run] skip registry readiness probe")
    else:
        try:
            wait_for_registry_ready(registry_host_port, wait_timeout)
        except TimeoutError as exc:
            emit_registry_diagnostics(
                runner,
                project_name=project_name,
                compose_file=compose_file,
                env_file=env_file,
            )
            raise RunnerError(str(exc)) from exc

    runner.emit("Preparing local fixture images from artifact")
    runner.run(
        [
            ctl_bin,
            "internal",
            "fixture-image",
            "ensure",
            "--artifact",
            str(artifact_path),
            "--output",
            "json",
        ],
        stream_output=True,
        env=command_env,
        cwd=project_root,
    )

    runner.emit(f"Deploying artifact via {ctl_bin}")
    runner.run(
        [ctl_bin, "deploy", "--artifact", str(artifact_path)],
        stream_output=True,
        env=command_env,
        cwd=project_root,
    )

    runner.emit("Running explicit provision (recommended for restores)")
    provision_cmd = [
        ctl_bin,
        "provision",
        "--project",
        project_name,
        "--compose-file",
        str(compose_file),
    ]
    if env_file:
        provision_cmd.extend(["--env-file", str(env_file)])
    provision_cmd.extend(["--project-dir", str(project_root)])
    runner.run(provision_cmd, stream_output=True, env=command_env, cwd=project_root)

    runner.emit("Showing compose ps")
    compose_ps_cmd = compose_base_cmd(
        project_name=project_name,
        compose_file=compose_file,
        env_file=env_file,
    )
    compose_ps_cmd.append("ps")
    runner.run(compose_ps_cmd, stream_output=True, env=command_env)

    runner.emit("Listing runtime config volume contents")
    runner.run(
        [
            "docker",
            "run",
            "--rm",
            "-v",
            f"{project_name}_esb-runtime-config:/runtime-config",
            "alpine",
            "ls",
            "-1",
            "/runtime-config",
        ],
        stream_output=True,
        check=False,
    )

    runner.emit("Done.")
    return 0


def _resolve_compose_file(
    raw_value: str | None,
    project_root: Path,
) -> Path:
    if raw_value and raw_value.strip():
        path = Path(raw_value).expanduser().resolve()
        if not path.is_file():
            raise FileNotFoundError(f"compose file not found: {path}")
        return path

    root_compose = (project_root / "docker-compose.yml").resolve()
    if not root_compose.is_file():
        raise FileNotFoundError(f"compose file not found: {root_compose}")
    return root_compose


def _resolve_env_file(
    raw_value: str | None,
    project_root: Path,
) -> Path | None:
    if raw_value and raw_value.strip():
        env_file = Path(raw_value).expanduser().resolve()
        if not env_file.is_file():
            raise FileNotFoundError(f"env file not found: {env_file}")
        return env_file

    root_env = (project_root / ".env").resolve()
    if root_env.is_file():
        return root_env

    raise RunnerError(
        f"root env file not found: {root_env}\nPass --env-file to specify the runtime env file."
    )


def _resolve_ctl_bin_path(
    runner: CommandRunner,
    override: str | None,
    env: dict[str, str],
) -> str:
    token = resolve_ctl_bin(override=override, env=env)
    if "/" in token:
        candidate = Path(token).expanduser()
        if not candidate.is_absolute():
            candidate = (Path.cwd() / candidate).resolve()
        if not candidate.is_file() or not os.access(candidate, os.X_OK):
            raise RunnerError(f"ctl command not found: {token}")
        return str(candidate)

    resolved = runner.which(token)
    if resolved is None:
        raise RunnerError(
            f"ctl command not found: {token}\n"
            "Install via `mise run setup` (or `mise run build-ctl`), or set CTL_BIN to override"
        )
    return resolved


def _resolve_registry_port(arg_value: int | None, env: dict[str, str]) -> int:
    if arg_value is not None:
        if arg_value < 0:
            raise RunnerError(f"--registry-port must be >= 0, got: {arg_value}")
        return arg_value

    raw = str(env.get("PORT_REGISTRY", "5010")).strip()
    if raw == "":
        return 5010
    if not raw.isdigit():
        raise RunnerError(f"PORT_REGISTRY must be numeric, got: {raw!r}")
    return int(raw)


def _resolve_registry_wait_timeout(arg_value: int | None, env: dict[str, str]) -> int:
    if arg_value is not None:
        if arg_value <= 0:
            raise RunnerError("--registry-wait-timeout must be > 0")
        return arg_value

    raw = str(env.get("REGISTRY_WAIT_TIMEOUT", "60")).strip() or "60"
    if not raw.isdigit() or int(raw) <= 0:
        raise RunnerError(f"REGISTRY_WAIT_TIMEOUT must be a positive integer, got: {raw!r}")
    return int(raw)


def _apply_registry_env_defaults(
    env: dict[str, str],
    *,
    mode: str,
    registry_port: int,
) -> None:
    host_registry = env.get("HOST_REGISTRY_ADDR", "").strip().rstrip("/")
    if host_registry == "":
        host_registry = f"127.0.0.1:{registry_port}"
        env["HOST_REGISTRY_ADDR"] = host_registry

    container_registry = env.get("CONTAINER_REGISTRY", "").strip().rstrip("/")
    if container_registry == "":
        if mode.strip().lower() == "docker":
            container_registry = host_registry
        else:
            container_registry = f"registry:{registry_port}"
        env["CONTAINER_REGISTRY"] = container_registry

    env.setdefault("REGISTRY", f"{container_registry}/")


def _assert_manifest_mode_matches_compose(*, compose_file: Path, manifest_mode: str) -> None:
    manifest = manifest_mode.strip().lower()
    if manifest == "":
        return
    inferred = _infer_compose_mode(compose_file)
    if inferred in {"unknown", "mixed"}:
        return
    if inferred != manifest:
        raise RunnerError(
            "artifact mode does not match compose mode: "
            f"artifact.mode={manifest!r}, compose={inferred!r}, compose_file={compose_file}"
        )


def _infer_compose_mode(compose_file: Path) -> str:
    path = compose_file.resolve()
    candidates: list[Path] = [path]
    try:
        payload = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    except OSError:
        payload = {}
    except yaml.YAMLError:
        payload = {}

    include_entries = payload.get("include") if isinstance(payload, dict) else None
    if isinstance(include_entries, list):
        for raw in include_entries:
            include_path: str | None = None
            if isinstance(raw, str):
                include_path = raw
            elif isinstance(raw, dict):
                include_path = str(raw.get("path", "")).strip()
            if not include_path:
                continue
            candidates.append((path.parent / include_path).resolve())

    has_docker = False
    has_containerd = False
    for candidate in candidates:
        name = candidate.name.lower()
        full = str(candidate).lower()
        if "containerd" in name or "containerd" in full:
            has_containerd = True
        if name in {"docker-compose.yml", "docker-compose.docker.yml"} or ".docker." in name:
            has_docker = True
        if "e2e-docker" in full:
            has_docker = True
        if "e2e-containerd" in full:
            has_containerd = True

    if has_docker and has_containerd:
        return "mixed"
    if has_containerd:
        return "containerd"
    if has_docker:
        return "docker"
    return "unknown"


def _assert_ctl_capabilities(
    runner: CommandRunner,
    *,
    ctl_bin: str,
    env: dict[str, str],
) -> None:
    for subcommand in CTL_REQUIRED_SUBCOMMANDS:
        probe = runner.run([ctl_bin, *subcommand], capture_output=True, check=False, env=env)
        output = f"{probe.stdout or ''}\n{probe.stderr or ''}".lower()
        if probe.returncode != 0 or "unknown command" in output:
            joined = " ".join(subcommand)
            raise RunnerError(f"ctl binary does not support `{joined}`: {ctl_bin}")

    cap_probe = runner.run(
        [ctl_bin, "internal", "capabilities", "--output", "json"],
        capture_output=True,
        check=False,
        env=env,
    )
    if cap_probe.returncode != 0:
        raise RunnerError(f"ctl capability probe failed: {ctl_bin}")

    capabilities = parse_ctl_capabilities(cap_probe.stdout or "")
    if capabilities is None:
        raise RunnerError("ctl capability response did not include JSON payload")

    schema_version = capabilities.get("schema_version")
    if schema_version != CTL_CAPABILITIES_SCHEMA_VERSION:
        raise RunnerError(
            "ctl capability schema mismatch: "
            f"{schema_version} (expected {CTL_CAPABILITIES_SCHEMA_VERSION})"
        )

    contracts = capabilities.get("contracts")
    if not isinstance(contracts, dict):
        raise RunnerError("ctl capability response is missing contracts map")

    for key, expected in CTL_REQUIRED_CONTRACTS.items():
        if contracts.get(key) != expected:
            raise RunnerError(
                "ctl missing required contract version: "
                f"{key}={contracts.get(key)!r} (expected {expected})"
            )
