import hashlib
import json
import os
import secrets
import subprocess
from pathlib import Path

from e2e.runner import constants
from e2e.runner.branding import (
    DEFAULT_ENV_PREFIX,
    brand_home_dir,
    buildkitd_config_path,
    buildx_builder_name,
    lambda_network_name,
    resolve_brand_slug,
    resolve_project_name,
    root_ca_mount_id,
)
from e2e.runner.branding import (
    cert_dir as default_cert_dir,
)
from e2e.runner.utils import PROJECT_ROOT, env_key, resolve_env_file_path

_DEFAULT_NO_PROXY_TARGETS = (
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
)


def hash_mod(value: str, mod: int) -> int:
    if mod <= 0:
        return 0
    m = hashlib.md5(value.encode("utf-8"))
    h_int = int(m.hexdigest(), 16)
    return h_int % mod


def env_external_subnet_index(env: str) -> int:
    if env == "default":
        return 50
    return 60 + hash_mod(env, 100)


def env_runtime_subnet_index(env: str) -> int:
    if env == "default":
        return 20
    return 100 + hash_mod(env, 100)


def read_env_file(path: str, *, brand_slug: str | None = None) -> dict[str, str]:
    env = {}
    env_path = Path(path)
    if not env_path.exists():
        return env
    resolved_brand_slug = resolve_brand_slug(brand_slug)
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            continue
        key, _, value = line.partition("=")
        value = value.strip()
        if "<brand>" in value:
            value = value.replace("<brand>", resolved_brand_slug)
        env[key.strip()] = value
    return env


def _split_no_proxy(value: str) -> list[str]:
    if not value:
        return []
    normalized = value.replace(";", ",")
    return [item.strip() for item in normalized.split(",") if item.strip()]


def _sync_proxy_aliases(env: dict[str, str], upper: str, lower: str) -> None:
    upper_value = env.get(upper, "").strip()
    lower_value = env.get(lower, "").strip()
    if upper_value and not lower_value:
        env[lower] = upper_value
        return
    if lower_value and not upper_value:
        env[upper] = lower_value


def apply_proxy_defaults(env: dict[str, str]) -> None:
    proxy_keys = ("HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy")
    has_proxy = any(env.get(key, "").strip() for key in proxy_keys)
    existing_no_proxy = env.get("NO_PROXY", "").strip() or env.get("no_proxy", "").strip()
    extra_no_proxy = env.get(env_key("NO_PROXY_EXTRA"), "").strip()

    if not has_proxy and not existing_no_proxy and not extra_no_proxy:
        return

    merged: list[str] = []
    seen: set[str] = set()
    for item in (
        _split_no_proxy(existing_no_proxy)
        + list(_DEFAULT_NO_PROXY_TARGETS)
        + _split_no_proxy(extra_no_proxy)
    ):
        if item in seen:
            continue
        seen.add(item)
        merged.append(item)

    if merged:
        no_proxy = ",".join(merged)
        env["NO_PROXY"] = no_proxy
        env["no_proxy"] = no_proxy

    _sync_proxy_aliases(env, "HTTP_PROXY", "http_proxy")
    _sync_proxy_aliases(env, "HTTPS_PROXY", "https_proxy")


def calculate_runtime_env(
    project_name: str,
    env_name: str,
    mode: str,
    env_file: str | None = None,
    env_overrides: dict[str, str] | None = None,
) -> dict[str, str]:
    """Replicates Go CLI's applyRuntimeEnv logic for the E2E runner."""
    env = os.environ.copy()
    project_name = resolve_project_name(project_name)
    brand_slug = resolve_brand_slug(project_name)

    # Load from env_file first, then apply explicit scenario overrides.
    env_from_file: dict[str, str] = {}
    if env_file:
        env_from_file = read_env_file(env_file, brand_slug=brand_slug)
        env.update(env_from_file)
    if env_overrides:
        env.update(env_overrides)

    if not env_name:
        env_name = "default"
    mode_normalized = mode.lower()

    env[constants.ENV_ENV] = env_name
    env[constants.ENV_MODE] = mode
    env[f"{DEFAULT_ENV_PREFIX}_ENV"] = env_name
    env[f"{DEFAULT_ENV_PREFIX}_MODE"] = mode

    env[constants.ENV_PROJECT_NAME] = project_name

    # Brand-scoped tag/registry
    tag_key = env_key(constants.ENV_TAG)
    registry_key = env_key(constants.ENV_REGISTRY)

    tag = env.get(tag_key, "").strip()
    if not tag:
        tag = "latest"
        env[tag_key] = tag

    registry = env.get(registry_key, "").strip()
    if registry_key not in env_from_file:
        if mode_normalized == "docker":
            registry = f"{constants.DEFAULT_AGENT_REGISTRY_HOST}/"
        else:
            registry = f"{constants.DEFAULT_AGENT_REGISTRY}/"
        env[registry_key] = registry
    if registry and not registry.endswith("/"):
        env[registry_key] = registry + "/"

    insecure_key = env_key(constants.ENV_CONTAINER_REGISTRY_INSECURE)
    if not env.get(insecure_key):
        env[insecure_key] = "1"

    # 2. Port Defaults (0 for dynamic)
    for port_suffix in (
        constants.PORT_GATEWAY_HTTPS,
        constants.PORT_GATEWAY_HTTP,
        constants.PORT_AGENT_GRPC,
        constants.PORT_S3,
        constants.PORT_S3_MGMT,
        constants.PORT_DATABASE,
        constants.PORT_REGISTRY,
        constants.PORT_VICTORIALOGS,
    ):
        key = env_key(port_suffix)
        # Check if set (possibly prefixed or not)
        if not env.get(key):
            env[key] = "0"

    # Use a stable registry port in E2E to match shared infra.
    registry_port_key = env_key(constants.PORT_REGISTRY)
    if env.get(registry_port_key) in ("", "0"):
        env[registry_port_key] = constants.DEFAULT_REGISTRY_PORT

    # 3. Subnets & Networks (Isolated per project-env)
    if not env.get(constants.ENV_NETWORK_EXTERNAL):
        env[constants.ENV_NETWORK_EXTERNAL] = f"{project_name}-{env_name}-external"

    if not env.get(constants.ENV_SUBNET_EXTERNAL):
        env[constants.ENV_SUBNET_EXTERNAL] = f"172.{env_external_subnet_index(env_name)}.0.0/16"

    if mode_normalized == "docker":
        if not env.get(constants.ENV_RUNTIME_NET_SUBNET):
            env[constants.ENV_RUNTIME_NET_SUBNET] = (
                f"172.{env_runtime_subnet_index(env_name)}.0.0/16"
            )

        if not env.get(constants.ENV_RUNTIME_NODE_IP):
            env[constants.ENV_RUNTIME_NODE_IP] = f"172.{env_runtime_subnet_index(env_name)}.0.10"
    else:
        # containerd/firecracker paths resolve CNI from stack identity;
        # keep legacy runtime subnet hints out of compose env to avoid forced CNI overrides.
        env.pop(constants.ENV_RUNTIME_NET_SUBNET, None)
        env.pop(constants.ENV_RUNTIME_NODE_IP, None)

    if not env.get(constants.ENV_LAMBDA_NETWORK):
        env[constants.ENV_LAMBDA_NETWORK] = lambda_network_name(project_name, env_name)

    # 4. Registry Defaults
    if constants.ENV_CONTAINER_REGISTRY not in env_from_file:
        if mode_normalized == "docker":
            env[constants.ENV_CONTAINER_REGISTRY] = constants.DEFAULT_AGENT_REGISTRY_HOST
        else:
            env[constants.ENV_CONTAINER_REGISTRY] = constants.DEFAULT_AGENT_REGISTRY

    # 5. Credentials (Simplified generation for E2E)
    if not env.get(constants.ENV_AUTH_USER):
        env[constants.ENV_AUTH_USER] = brand_slug
    if not env.get(constants.ENV_AUTH_PASS):
        env[constants.ENV_AUTH_PASS] = secrets.token_hex(16)
    if not env.get(constants.ENV_JWT_SECRET_KEY):
        env[constants.ENV_JWT_SECRET_KEY] = secrets.token_hex(32)
    if not env.get(constants.ENV_X_API_KEY):
        env[constants.ENV_X_API_KEY] = secrets.token_hex(32)
    if not env.get(constants.ENV_RUSTFS_ACCESS_KEY):
        env[constants.ENV_RUSTFS_ACCESS_KEY] = brand_slug
    if not env.get(constants.ENV_RUSTFS_SECRET_KEY):
        env[constants.ENV_RUSTFS_SECRET_KEY] = secrets.token_hex(16)

    # 6. Branding & Certificates (Replicating applyBrandingEnv in Go)
    env["ENV_PREFIX"] = DEFAULT_ENV_PREFIX
    env[constants.ENV_ROOT_CA_MOUNT_ID] = root_ca_mount_id(project_name)
    env.setdefault(constants.ENV_ROOT_CA_CERT_FILENAME, constants.DEFAULT_ROOT_CA_FILENAME)

    # Resolve CERT_DIR (repo-root scoped)
    cert_dir = Path(
        os.environ.get(
            constants.ENV_CERT_DIR,
            str(default_cert_dir(PROJECT_ROOT, project_name)),
        )
    ).expanduser()
    env.setdefault(constants.ENV_CERT_DIR, str(cert_dir))
    env.setdefault(
        constants.ENV_BUILDKITD_CONFIG,
        str(buildkitd_config_path(PROJECT_ROOT, project_name)),
    )

    # Calculate ROOT_CA_FINGERPRINT for build cache invalidation
    ca_path = cert_dir / constants.DEFAULT_ROOT_CA_FILENAME
    if ca_path.exists():
        try:
            content = ca_path.read_bytes()
            env[constants.ENV_ROOT_CA_FINGERPRINT] = hashlib.md5(content).hexdigest()
        except OSError:
            pass

    # 8. Docker BuildKit
    env.setdefault(constants.ENV_DOCKER_BUILDKIT, "1")
    env.setdefault("BUILDX_BUILDER", buildx_builder_name(project_name))
    env.setdefault("COMPOSE_DOCKER_CLI_BUILD", "1")

    # 9. E2E safety toggles
    env.setdefault("ESB_SKIP_GATEWAY_ALIGN", "1")
    apply_proxy_defaults(env)

    return env


def read_service_env(project_name: str, service: str) -> dict[str, str]:
    """Read environment variables from a running container using docker inspect."""
    # We need the container name. Docker Compose usually names them: {project}-{service}-1
    # But it's safer to use labels.
    cmd = [
        "docker",
        "ps",
        "-q",
        "--filter",
        f"label=com.docker.compose.project={project_name}",
        "--filter",
        f"label=com.docker.compose.service={service}",
    ]
    result = subprocess.run(cmd, capture_output=True, text=True, check=True)
    container_id = result.stdout.strip()
    if not container_id:
        raise RuntimeError(
            f"No running container found for service: {service} in project: {project_name}"
        )

    cmd = [
        "docker",
        "inspect",
        "--format",
        "{{range .Config.Env}}{{println .}}{{end}}",
        container_id,
    ]
    result = subprocess.run(cmd, capture_output=True, text=True, check=True)

    env = {}
    for line in result.stdout.splitlines():
        if "=" in line:
            key, _, value = line.partition("=")
            env[key] = value
    return env


def build_env_list(entries: list[tuple[str, str]]) -> str:
    parts = []
    for name, mode in entries:
        if mode:
            parts.append(f"{name}:{mode}")
        else:
            parts.append(name)
    return ",".join(parts)


def resolve_esb_home(env_name: str) -> Path:
    prefixed_home = os.environ.get(env_key("HOME"))
    if prefixed_home:
        return Path(prefixed_home).expanduser()
    project_name = (
        os.environ.get(env_key("PROJECT")) or os.environ.get(constants.ENV_PROJECT_NAME) or None
    )
    return Path.home() / brand_home_dir(project_name) / env_name


def _resolve_compose_files_from_project(project_name: str) -> list[Path]:
    if not project_name:
        return []
    try:
        list_cmd = [
            "docker",
            "ps",
            "-a",
            "--filter",
            f"label=com.docker.compose.project={project_name}",
            "--format",
            "{{.ID}}",
        ]
        result = subprocess.run(list_cmd, capture_output=True, text=True)
        if result.returncode != 0 or not result.stdout.strip():
            return []
        container_ids = [line.strip() for line in result.stdout.splitlines() if line.strip()]
        if not container_ids:
            return []
        inspect = subprocess.run(
            ["docker", "inspect", *container_ids], capture_output=True, text=True
        )
        if inspect.returncode != 0 or not inspect.stdout.strip():
            return []
        containers = json.loads(inspect.stdout)
    except Exception:
        return []

    running = []
    for ctr in containers:
        state = str(ctr.get("State", {}).get("Status", "")).lower()
        if state == "running":
            running.append(ctr)
    candidates = running or containers

    sets: dict[tuple[str, ...], int] = {}
    order: list[tuple[str, ...]] = []
    for ctr in candidates:
        labels = ctr.get("Config", {}).get("Labels", {}) or {}
        raw = str(labels.get("com.docker.compose.project.config_files", "")).strip()
        if not raw:
            continue
        working_dir = str(labels.get("com.docker.compose.project.working_dir", "")).strip()
        files = []
        for part in raw.split(","):
            part = part.strip()
            if not part:
                continue
            path = Path(part)
            if not path.is_absolute() and working_dir:
                path = Path(working_dir) / path
            files.append(os.path.abspath(str(path)))
        if not files:
            continue
        key = tuple(files)
        if key not in sets:
            sets[key] = 0
            order.append(key)
        sets[key] += 1

    if not sets:
        return []

    best_key = order[0]
    best_count = sets[best_key]
    for key in order[1:]:
        if sets[key] > best_count:
            best_key = key
            best_count = sets[key]

    resolved = [Path(path) for path in best_key]
    existing = [path for path in resolved if path.exists()]
    if not existing:
        return []
    return existing


def build_compose_base_cmd(
    project_name: str, compose_file: Path, env_file: str | None = None
) -> list[str]:
    cmd = ["docker", "compose", "-p", project_name]
    env_file_path = resolve_env_file_path(env_file)
    if env_file_path:
        cmd.extend(["--env-file", env_file_path])
    compose_files = _resolve_compose_files_from_project(project_name)
    if compose_files:
        for file in compose_files:
            cmd.extend(["-f", str(file)])
        return cmd
    cmd.extend(["-f", str(compose_file)])
    return cmd


def discover_ports(
    project_name: str,
    compose_file: Path,
    env_file: str | None = None,
) -> dict[str, int]:
    """Discover host ports for mapped services using docker compose port."""
    services = {
        "gateway": [("8443", constants.PORT_GATEWAY_HTTPS), ("8080", constants.PORT_GATEWAY_HTTP)],
        "victorialogs": [("9428", constants.PORT_VICTORIALOGS)],
        "database": [("8000", constants.PORT_DATABASE)],
        "s3-storage": [("9000", constants.PORT_S3), ("9001", constants.PORT_S3_MGMT)],
        "agent": [("50051", constants.PORT_AGENT_GRPC), ("9091", constants.PORT_AGENT_METRICS)],
        "registry": [("5010", constants.PORT_REGISTRY)],
        "runtime-node": [
            ("8443", constants.PORT_GATEWAY_HTTPS),
            ("50051", constants.PORT_AGENT_GRPC),
            ("9091", constants.PORT_AGENT_METRICS),
        ],
    }

    ports = {}
    base_cmd = build_compose_base_cmd(project_name, compose_file, env_file=env_file)
    for service, port_mappings in services.items():
        for internal, env_key_suffix in port_mappings:
            try:
                cmd = base_cmd + ["port", service, internal]
                result = subprocess.run(cmd, capture_output=True, text=True)
                if result.returncode == 0 and result.stdout.strip():
                    # Format is 0.0.0.0:12345 or [::]:12345
                    host_port = result.stdout.strip().split(":")[-1]
                    ports[env_key(env_key_suffix)] = int(host_port)
            except Exception:
                continue
    return ports


def load_ports(env_name: str) -> dict[str, int]:
    """Deprecated: ports are now discovered dynamically in Zero-Config."""
    return {}


def apply_ports_to_env(ports: dict[str, int]) -> None:
    # We only apply ports that the host process (pytest/runner) needs to talk to directly.
    # Other services like Database or S3 are reached via Gateway or internal networking.
    # Over-applying here can cause endpoint mismatches if tests or Lambdas inherit host-mapped ports.

    gateway_key = env_key(constants.PORT_GATEWAY_HTTPS)
    if gateway_key in ports:
        gateway_port = ports[gateway_key]
        os.environ[gateway_key] = str(gateway_port)
        os.environ[constants.ENV_GatewayPort] = str(gateway_port)
        os.environ[constants.ENV_GatewayURL] = f"https://localhost:{gateway_port}"

    vl_key = env_key(constants.PORT_VICTORIALOGS)
    if vl_key in ports:
        vl_port = ports[vl_key]
        os.environ[vl_key] = str(vl_port)
        os.environ[constants.ENV_VictoriaLogsPort] = str(vl_port)
        os.environ[constants.ENV_VictoriaLogsURL] = f"http://localhost:{vl_port}"

    agent_key = env_key(constants.PORT_AGENT_GRPC)
    if agent_key in ports:
        agent_port = ports[agent_key]
        os.environ[agent_key] = str(agent_port)
        os.environ[constants.ENV_AgentGrpcAddress] = f"localhost:{agent_port}"


def apply_gateway_env_from_container(env: dict[str, str], project_name: str) -> None:
    gateway_env = read_service_env(project_name, "gateway")
    required = (
        constants.ENV_AUTH_USER,
        constants.ENV_AUTH_PASS,
        constants.ENV_X_API_KEY,
        constants.ENV_RUSTFS_ACCESS_KEY,
        constants.ENV_RUSTFS_SECRET_KEY,
    )
    missing = [key for key in required if not gateway_env.get(key)]
    if missing:
        raise RuntimeError(
            f"Missing required gateway env vars from container: {', '.join(missing)}"
        )

    for key in required:
        env[key] = gateway_env[key]

    auth_path = gateway_env.get(constants.ENV_AUTH_ENDPOINT_PATH)
    if auth_path:
        env[constants.ENV_AUTH_ENDPOINT_PATH] = auth_path
    else:
        env.setdefault(constants.ENV_AUTH_ENDPOINT_PATH, constants.DEFAULT_AUTH_Path)

    for key in (
        constants.ENV_VERIFY_SSL,
        constants.ENV_CONTAINERS_NETWORK,
        constants.ENV_GATEWAY_INTERNAL_URL,
    ):
        value = gateway_env.get(key)
        if value:
            env[key] = value

    tls_enabled = gateway_env.get(constants.ENV_AGENT_GRPC_TLS_ENABLED)
    if tls_enabled:
        env[constants.ENV_AGENT_GRPC_TLS_ENABLED] = tls_enabled
        default_project = (
            env.get(env_key("PROJECT"))
            or env.get(constants.ENV_PROJECT_NAME)
            or os.environ.get(constants.ENV_PROJECT_NAME)
        )
        cert_dir = Path(
            os.environ.get(
                constants.ENV_CERT_DIR,
                str(default_cert_dir(PROJECT_ROOT, default_project)),
            )
        ).expanduser()
        env[constants.ENV_AGENT_GRPC_TLS_CA_CERT_PATH] = str(
            cert_dir / constants.DEFAULT_ROOT_CA_FILENAME
        )
        env[constants.ENV_AGENT_GRPC_TLS_CERT_PATH] = str(cert_dir / "client.crt")
        env[constants.ENV_AGENT_GRPC_TLS_KEY_PATH] = str(cert_dir / "client.key")
