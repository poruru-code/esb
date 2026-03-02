from __future__ import annotations

import json
import os
import re
import shutil
import time
from dataclasses import dataclass
from pathlib import Path
from urllib import error as urlerror
from urllib import request as urlrequest

import yaml

from tools.cli.branding_constants_gen import DEFAULT_CTL_BIN
from tools.cli.common import run_command


@dataclass(frozen=True)
class StackDeployInput:
    artifact_path: str = ""


def execute_stack_deploy(input_data: StackDeployInput) -> None:
    repo_root = resolve_repo_root()
    artifact_path = resolve_artifact_path(repo_root, input_data.artifact_path)
    print(f"Using artifact: {artifact_path}")

    artifact_project, artifact_env = read_artifact_project_env(artifact_path)
    compose_file = Path(repo_root) / "docker-compose.yml"
    if not compose_file.exists() or not compose_file.is_file():
        raise RuntimeError(f"compose file not found: {compose_file}")

    env_file = Path(repo_root) / ".env"
    env_file_args: list[str] = []
    env_from_file: dict[str, str] = {}
    if env_file.exists() and env_file.is_file():
        env_file_args = ["--env-file", str(env_file)]
        env_from_file = read_env_file(str(env_file))

    command_env = dict(os.environ)
    command_env.update(env_from_file)

    base_project = artifact_project or command_env.get("PROJECT_NAME", "").strip()
    resolved_env = artifact_env or command_env.get("ENV", "").strip()
    if base_project == "":
        raise RuntimeError("project is empty (artifact project / PROJECT_NAME)")

    ctl_bin = command_env.get("CTL_BIN", "").strip() or DEFAULT_CTL_BIN
    if shutil.which(ctl_bin, path=command_env.get("PATH")) is None:
        raise RuntimeError(
            f"ctl command not found: {ctl_bin}; "
            "install via 'mise run setup' (or 'mise run build-ctl'), "
            "or set CTL_BIN to override"
        )

    if resolved_env != "":
        project_name_raw = f"{base_project}-{resolved_env}"
    else:
        project_name_raw = base_project
    project_name = normalize_compose_project_name(project_name_raw)
    if project_name == "":
        raise RuntimeError("resolved PROJECT_NAME is empty after normalization")

    command_env["PROJECT_NAME"] = project_name
    command_env["ENV"] = resolved_env

    ensure_registry_container_compatible(
        registry_container_name=resolve_registry_container_name(command_env),
        project_name=project_name,
        repo_root=repo_root,
        env_file_path=str(env_file) if env_file_args else "",
        env=command_env,
    )

    jwt_secret = command_env.get("JWT_SECRET_KEY", "")
    if len(jwt_secret) < 32:
        raise RuntimeError("JWT_SECRET_KEY must be set and >= 32 chars")

    compose_base = compose_base_args(
        compose_project=project_name,
        compose_file=str(compose_file),
        env_file_args=env_file_args,
    )

    print(f"Bringing up stack for project '{project_name}' (ENV='{resolved_env}')")
    run_command(["docker", *compose_base, "up", "-d"], env=command_env, check=True)

    port_registry_raw = command_env.get("PORT_REGISTRY", "5010").strip()
    if not port_registry_raw.isdigit():
        raise RuntimeError(f"PORT_REGISTRY must be numeric, got: '{port_registry_raw}'")
    port_registry = int(port_registry_raw)

    if port_registry == 0:
        print("PORT_REGISTRY=0 detected; resolving published host port for registry:5010")
        resolved_port_payload = run_command(
            ["docker", *compose_base, "port", "registry", "5010"],
            env=command_env,
            capture_output=True,
            check=False,
        )
        resolved_line = ""
        for line in (resolved_port_payload.stdout or "").splitlines():
            cleaned = line.strip()
            if cleaned != "":
                resolved_line = cleaned
        resolved_port = resolved_line.rsplit(":", 1)[-1].strip()
        if not resolved_port.isdigit() or int(resolved_port) == 0:
            raise RuntimeError(f"Failed to resolve published registry port (raw='{resolved_line}')")
        port_registry = int(resolved_port)
        print(f"Resolved registry host port: {port_registry}")

    timeout_raw = command_env.get("REGISTRY_WAIT_TIMEOUT", "60").strip()
    if not timeout_raw.isdigit() or int(timeout_raw) <= 0:
        raise RuntimeError(
            f"REGISTRY_WAIT_TIMEOUT must be a positive integer, got: '{timeout_raw}'"
        )
    timeout_seconds = int(timeout_raw)

    print(f"Waiting for registry on http://127.0.0.1:{port_registry}/v2/")
    wait_for_registry_ready(
        host_port=f"127.0.0.1:{port_registry}",
        timeout_seconds=timeout_seconds,
        compose_base=compose_base,
        env=command_env,
    )

    print(f"Deploying artifact via {ctl_bin}")
    run_command([ctl_bin, "deploy", "--artifact", artifact_path], env=command_env, check=True)

    print("Running explicit provision (recommended for restores)")
    provision_cmd = [
        ctl_bin,
        "provision",
        "--project",
        project_name,
        "--compose-file",
        str(compose_file),
    ]
    if env_file_args:
        provision_cmd.extend(env_file_args)
    provision_cmd.extend(["--project-dir", repo_root])
    run_command(provision_cmd, env=command_env, check=True)

    print("Showing compose ps")
    run_command(["docker", *compose_base, "ps"], env=command_env, check=True)

    print("Listing runtime config volume contents")
    run_command(
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
        env=command_env,
        check=False,
    )

    print("Done.")


def resolve_repo_root() -> str:
    result = run_command(
        ["git", "rev-parse", "--show-toplevel"],
        capture_output=True,
        check=False,
    )
    root = (result.stdout or "").strip()
    if result.returncode == 0 and root != "":
        return str(Path(root).resolve())
    return str(Path.cwd().resolve())


def resolve_artifact_path(repo_root: str, artifact_arg: str) -> str:
    normalized_arg = artifact_arg.strip()
    if normalized_arg != "":
        candidate = Path(normalized_arg)
        if not candidate.is_file():
            raise RuntimeError(f"artifact.yml not found: {artifact_arg}")
        return str(candidate.resolve())

    artifacts_root = Path(repo_root) / "artifacts"
    matches = sorted(str(path.resolve()) for path in artifacts_root.rglob("artifact.yml"))
    if not matches:
        raise RuntimeError(f"No artifact.yml found under {artifacts_root}. Provide as argument.")
    if len(matches) == 1:
        return matches[0]
    return choose_artifact_interactive(matches)


def choose_artifact_interactive(artifact_paths: list[str]) -> str:
    print("Multiple artifact.yml files found; choose one:")
    for idx, path in enumerate(artifact_paths, start=1):
        print(f"{idx:2d}) {path}")

    while True:
        try:
            selected = input(f"Select number (1-{len(artifact_paths)}) [1]: ").strip()
        except EOFError as exc:
            raise RuntimeError("artifact selection aborted") from exc
        if selected == "":
            selected = "1"
        if selected.isdigit():
            index = int(selected)
            if 1 <= index <= len(artifact_paths):
                return artifact_paths[index - 1]
        print("Invalid selection")


def read_artifact_project_env(path: str) -> tuple[str, str]:
    payload = yaml.safe_load(Path(path).read_text(encoding="utf-8"))
    if not isinstance(payload, dict):
        return "", ""
    project = str(payload.get("project", "")).strip()
    env_name = str(payload.get("env", "")).strip()
    return project, env_name


def read_env_file(path: str) -> dict[str, str]:
    env: dict[str, str] = {}
    env_path = Path(path)
    if not env_path.exists() or not env_path.is_file():
        return env
    for raw_line in env_path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if line == "" or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[len("export ") :].strip()
        if "=" not in line:
            continue
        key, _, value = line.partition("=")
        key = key.strip()
        if key == "":
            continue
        value = value.strip()
        if len(value) >= 2:
            if (value[0] == '"' and value[-1] == '"') or (value[0] == "'" and value[-1] == "'"):
                value = value[1:-1]
        env[key] = value
    return env


def normalize_compose_project_name(value: str) -> str:
    normalized = value.lower()
    normalized = re.sub(r"[^a-z0-9_.-]+", "-", normalized)
    normalized = re.sub(r"^[^a-z0-9]+", "", normalized)
    normalized = re.sub(r"[^a-z0-9]+$", "", normalized)
    return normalized


def compose_base_args(
    *, compose_project: str, compose_file: str, env_file_args: list[str]
) -> list[str]:
    return [
        "compose",
        "-p",
        compose_project,
        *env_file_args,
        "-f",
        compose_file,
    ]


def resolve_registry_container_name(env: dict[str, str]) -> str:
    name = env.get("REGISTRY_CONTAINER_NAME", "").strip()
    if name != "":
        return name
    return "esb-infra-registry"


def ensure_registry_container_compatible(
    *,
    registry_container_name: str,
    project_name: str,
    repo_root: str,
    env_file_path: str,
    env: dict[str, str],
) -> None:
    name = registry_container_name.strip()
    if name == "":
        return

    inspect_result = run_command(
        ["docker", "inspect", name],
        env=env,
        capture_output=True,
        check=False,
    )
    if inspect_result.returncode != 0:
        return

    owner_project = read_compose_project_from_inspect(inspect_result.stdout or "")
    if owner_project == project_name:
        return

    fix_commands = []
    if owner_project != "":
        compose_fix_cmd = f"docker compose -p {owner_project}"
        if env_file_path.strip() != "":
            compose_fix_cmd += f" --env-file {env_file_path}"
        compose_fix_cmd += f" -f {Path(repo_root) / 'docker-compose.yml'} rm -sf registry"
        fix_commands.append(compose_fix_cmd)
    fix_commands.append(f"docker rm -f {name}")

    owner_label = owner_project if owner_project != "" else "unknown"
    raise RuntimeError(
        f"shared registry container '{name}' already exists "
        f"(owner project: '{owner_label}', current project: '{project_name}').\n"
        "Resolve and retry with one of:\n" + "\n".join(f"  {cmd}" for cmd in fix_commands)
    )


def read_compose_project_from_inspect(payload: str) -> str:
    raw = payload.strip()
    if raw == "":
        return ""
    try:
        parsed = json.loads(raw)
    except json.JSONDecodeError:
        return ""
    if not isinstance(parsed, list) or not parsed:
        return ""
    first = parsed[0]
    if not isinstance(first, dict):
        return ""
    config = first.get("Config")
    if not isinstance(config, dict):
        return ""
    labels = config.get("Labels")
    if not isinstance(labels, dict):
        return ""
    return str(labels.get("com.docker.compose.project", "")).strip()


def wait_for_registry_ready(
    *,
    host_port: str,
    timeout_seconds: int,
    compose_base: list[str],
    env: dict[str, str],
) -> None:
    started = time.time()
    while True:
        code = probe_registry(host_port)
        if code in (200, 401):
            return
        if time.time() - started >= timeout_seconds:
            run_command(
                ["docker", *compose_base, "ps", "registry"],
                env=env,
                check=False,
            )
            run_command(
                ["docker", *compose_base, "logs", "--tail=50", "registry"],
                env=env,
                check=False,
            )
            status = "n/a" if code is None else str(code)
            raise RuntimeError(
                f"Registry not responding at http://{host_port}/v2/ "
                f"after {timeout_seconds}s (last_status='{status}')"
            )
        time.sleep(1)


def probe_registry(host_port: str) -> int | None:
    url = f"http://{host_port}/v2/"
    request = urlrequest.Request(url=url, method="GET")
    opener = urlrequest.build_opener(urlrequest.ProxyHandler({}))
    try:
        with opener.open(request, timeout=2) as response:  # noqa: S310
            return int(response.status)
    except urlerror.HTTPError as exc:
        return int(exc.code)
    except (urlerror.URLError, TimeoutError, OSError):
        return None
