import hashlib
import os
import secrets
import subprocess
from pathlib import Path

from e2e.runner import constants
from e2e.runner.utils import (
    BRAND_HOME_DIR,
    BRAND_SLUG,
    CLI_ROOT,
    ENV_PREFIX,
    env_key,
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


def read_env_file(path: str) -> dict[str, str]:
    env = {}
    env_path = Path(path)
    if not env_path.exists():
        return env
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            continue
        key, _, value = line.partition("=")
        env[key.strip()] = value.strip()
    return env


def calculate_runtime_env(
    project_name: str, env_name: str, mode: str, env_file: str | None = None
) -> dict[str, str]:
    """Replicates Go CLI's applyRuntimeEnv logic for the E2E runner."""
    env = os.environ.copy()

    # Load from env_file if provided (prioritize file over system env if needed,
    # but Go CLI usually merges and prioritizes file for certain values)
    if env_file:
        env_from_file = read_env_file(env_file)
        env.update(env_from_file)

    if not env_name:
        env_name = "default"

    env[constants.ENV_ENV] = env_name
    env[constants.ENV_MODE] = mode
    env[f"{ENV_PREFIX}_ENV"] = env_name
    env[f"{ENV_PREFIX}_MODE"] = mode

    env[constants.ENV_PROJECT_NAME] = project_name

    # Normalize mode for registry checks
    norm_mode = mode.lower() if mode else "docker"

    # Brand-scoped tag/registry
    tag_key = env_key(constants.ENV_TAG)
    registry_key = env_key(constants.ENV_REGISTRY)

    tag = env.get(tag_key, "").strip()
    if not tag:
        tag = "latest"
        env[tag_key] = tag

    registry = env.get(registry_key, "").strip()
    if not registry:
        if norm_mode == "docker":
            registry = f"{constants.DEFAULT_AGENT_REGISTRY_HOST}/"
        else:
            registry = f"{constants.DEFAULT_AGENT_REGISTRY}/"
        env[registry_key] = registry
    if registry and not registry.endswith("/"):
        env[registry_key] = registry + "/"

    # If using local dev, point to the Go CLI source root for template resolution
    # (matching logic in cli/internal/config/config.go)
    if constants.ENV_CLI_SRC_ROOT not in env:
        env[constants.ENV_CLI_SRC_ROOT] = str(CLI_ROOT)

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

    # 3. Subnets & Networks (Isolated per project-env)
    if not env.get(constants.ENV_NETWORK_EXTERNAL):
        env[constants.ENV_NETWORK_EXTERNAL] = f"{project_name}-{env_name}-external"

    if not env.get(constants.ENV_SUBNET_EXTERNAL):
        env[constants.ENV_SUBNET_EXTERNAL] = f"172.{env_external_subnet_index(env_name)}.0.0/16"

    if not env.get(constants.ENV_RUNTIME_NET_SUBNET):
        env[constants.ENV_RUNTIME_NET_SUBNET] = f"172.{env_runtime_subnet_index(env_name)}.0.0/16"

    if not env.get(constants.ENV_RUNTIME_NODE_IP):
        env[constants.ENV_RUNTIME_NODE_IP] = f"172.{env_runtime_subnet_index(env_name)}.0.10"

    if not env.get(constants.ENV_LAMBDA_NETWORK):
        env[constants.ENV_LAMBDA_NETWORK] = f"esb_int_{env_name}"

    # 4. Registry Defaults
    if not env.get(constants.ENV_CONTAINER_REGISTRY):
        if norm_mode == "docker":
            env[constants.ENV_CONTAINER_REGISTRY] = constants.DEFAULT_AGENT_REGISTRY_HOST
        else:
            env[constants.ENV_CONTAINER_REGISTRY] = constants.DEFAULT_AGENT_REGISTRY

    # 5. Credentials (Simplified generation for E2E)
    if not env.get(constants.ENV_AUTH_USER):
        env[constants.ENV_AUTH_USER] = BRAND_SLUG
    if not env.get(constants.ENV_AUTH_PASS):
        env[constants.ENV_AUTH_PASS] = secrets.token_hex(16)
    if not env.get(constants.ENV_JWT_SECRET_KEY):
        env[constants.ENV_JWT_SECRET_KEY] = secrets.token_hex(32)
    if not env.get(constants.ENV_X_API_KEY):
        env[constants.ENV_X_API_KEY] = secrets.token_hex(32)
    if not env.get(constants.ENV_RUSTFS_ACCESS_KEY):
        env[constants.ENV_RUSTFS_ACCESS_KEY] = BRAND_SLUG
    if not env.get(constants.ENV_RUSTFS_SECRET_KEY):
        env[constants.ENV_RUSTFS_SECRET_KEY] = secrets.token_hex(16)

    # 6. Branding & Certificates (Replicating applyBrandingEnv in Go)
    env["ENV_PREFIX"] = ENV_PREFIX
    env[constants.ENV_CLI_CMD] = BRAND_SLUG
    env[constants.ENV_ROOT_CA_MOUNT_ID] = f"{BRAND_SLUG}_root_ca"
    env.setdefault(constants.ENV_ROOT_CA_CERT_FILENAME, constants.DEFAULT_ROOT_CA_FILENAME)

    # Resolve CERT_DIR
    cert_dir = Path(
        os.environ.get(constants.ENV_CERT_DIR, Path.home() / BRAND_HOME_DIR / "certs")
    ).expanduser()
    env.setdefault(constants.ENV_CERT_DIR, str(cert_dir))

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

    # 9. Staging Config Dir
    # Replicates applyConfigDirEnv / staging.ConfigDir logic
    config_dir = calculate_staging_dir(project_name, env_name)
    if config_dir.exists():
        env[constants.ENV_CONFIG_DIR] = str(config_dir)

    return env


def calculate_staging_dir(project_name: str, env_name: str) -> Path:
    """Replicates staging.ConfigDir logic from Go."""
    # ComposeProjectKey logic
    proj_key = project_name.strip()
    if not proj_key:
        proj_key = f"{BRAND_SLUG}-{env_name.lower()}" if env_name else BRAND_SLUG

    # stageKey logic
    seed = proj_key
    if env_name:
        seed = f"{seed}:{env_name.lower()}"

    h = hashlib.sha256(seed.encode()).hexdigest()
    # hex.EncodeToString(sum[:4]) in Go is 8 chars
    stage_key = f"{proj_key}-{h[:8]}"

    # RootDir logic
    cache_home = os.environ.get("XDG_CACHE_HOME")
    if cache_home:
        root = Path(cache_home) / BRAND_SLUG / "staging"
    else:
        root = Path.home() / f".{BRAND_SLUG}" / ".cache" / "staging"

    return root / stage_key / env_name / "config"


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
    return Path.home() / BRAND_HOME_DIR / env_name


def discover_ports(project_name: str, compose_file: Path) -> dict[str, int]:
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
    for service, port_mappings in services.items():
        for internal, env_key_suffix in port_mappings:
            try:
                # docker compose -p {project} -f {file} port {service} {internal}
                cmd = [
                    "docker",
                    "compose",
                    "-p",
                    project_name,
                    "-f",
                    str(compose_file),
                    "port",
                    service,
                    internal,
                ]
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
        cert_dir = Path(
            os.environ.get(constants.ENV_CERT_DIR, f"~/{BRAND_HOME_DIR}/certs")
        ).expanduser()
        env[constants.ENV_AGENT_GRPC_TLS_CA_CERT_PATH] = str(
            cert_dir / constants.DEFAULT_ROOT_CA_FILENAME
        )
        env[constants.ENV_AGENT_GRPC_TLS_CERT_PATH] = str(cert_dir / "client.crt")
        env[constants.ENV_AGENT_GRPC_TLS_KEY_PATH] = str(cert_dir / "client.key")
