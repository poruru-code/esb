import hashlib
import json
import os
import secrets
import subprocess
from pathlib import Path

from e2e.runner import constants
from e2e.runner.utils import (
    BRAND_HOME_DIR,
    BRAND_SLUG,
    CLI_ROOT,
    DEFAULT_NO_PROXY_TARGETS,
    ENV_PREFIX,
    build_esb_cmd,
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

    # Normalize mode for tag lookup
    norm_mode = mode.lower() if mode else "docker"
    if norm_mode in ("docker", "containerd", "firecracker"):
        tag = norm_mode
    else:
        tag = env_name if env_name else "latest"

    env[constants.ENV_IMAGE_TAG] = tag
    env[f"{ENV_PREFIX}_IMAGE_TAG"] = tag
    env[constants.ENV_PROJECT_NAME] = project_name

    # Image Prefix
    if constants.ENV_IMAGE_PREFIX not in env:
        env[constants.ENV_IMAGE_PREFIX] = project_name

    # If using local dev, point to the Go CLI source root for template resolution
    # (matching logic in cli/internal/config/config.go)
    if "CLI_SRC_ROOT" not in env:
        env["CLI_SRC_ROOT"] = str(CLI_ROOT)

    # 2. Port Defaults (0 for dynamic)
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
    if not env.get(constants.ENV_CONTAINER_REGISTRY) and norm_mode in ("containerd", "firecracker"):
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
    env.setdefault("ROOT_CA_CERT_FILENAME", "rootCA.crt")

    # Resolve CERT_DIR
    cert_dir = Path(os.environ.get("CERT_DIR", Path.home() / BRAND_HOME_DIR / "certs")).expanduser()
    env.setdefault("CERT_DIR", str(cert_dir))

    # Calculate ROOT_CA_FINGERPRINT for build cache invalidation
    ca_path = cert_dir / "rootCA.crt"
    if ca_path.exists():
        try:
            content = ca_path.read_bytes()
            env["ROOT_CA_FINGERPRINT"] = hashlib.md5(content).hexdigest()
        except OSError:
            pass

    # 7. Proxy Defaults (Ensure NO_PROXY is consistent)
    apply_proxy_env_to_dict(env)

    # 8. Docker BuildKit
    env.setdefault(constants.ENV_DOCKER_BUILDKIT, "1")

    # 9. Project Configuration (generator.yml)
    # Replicates applyGeneratorConfigEnv logic
    repo_root = Path(env.get(env_key("REPO"), Path.cwd()))
    # In E2E, the fixtures are often in e2e/fixtures
    if (
        not (repo_root / "generator.yml").exists()
        and (repo_root / "e2e" / "fixtures" / "generator.yml").exists()
    ):
        repo_root = repo_root / "e2e" / "fixtures"

    gen_yaml_path = repo_root / "generator.yml"
    if gen_yaml_path.exists():
        try:
            import yaml

            with open(gen_yaml_path) as f:
                gen_cfg = yaml.safe_load(f)
            paths = gen_cfg.get("paths", {})
            if paths.get("functions_yml"):
                env[constants.ENV_GATEWAY_FUNCTIONS_YML] = paths["functions_yml"]
            if paths.get("routing_yml"):
                env[constants.ENV_GATEWAY_ROUTING_YML] = paths["routing_yml"]
        except Exception:
            pass

    # 10. Staging Config Dir
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


def apply_proxy_env_to_dict(env: dict[str, str]) -> None:
    """Replicates applyProxyDefaults logic into a dictionary for use in Compose."""
    proxy_keys = ("HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy")
    has_proxy = any(os.environ.get(key) or env.get(key) for key in proxy_keys)
    existing_no_proxy = (
        os.environ.get("NO_PROXY") or os.environ.get("no_proxy") or env.get("NO_PROXY")
    )
    extra_key = env_key("NO_PROXY_EXTRA")
    extra_no_proxy = os.environ.get(extra_key) or env.get(extra_key)

    if not (has_proxy or existing_no_proxy or extra_no_proxy):
        return

    def split_no_proxy(value: str | None) -> list[str]:
        if not value:
            return []
        parts = value.replace(";", ",").split(",")
        return [item.strip() for item in parts if item.strip()]

    merged: list[str] = []
    seen: set[str] = set()

    for item in split_no_proxy(existing_no_proxy):
        if item not in seen:
            merged.append(item)
            seen.add(item)

    for item in DEFAULT_NO_PROXY_TARGETS:
        if item and item not in seen:
            merged.append(item)
            seen.add(item)

    for item in split_no_proxy(extra_no_proxy):
        if item and item not in seen:
            merged.append(item)
            seen.add(item)

    if merged:
        val = ",".join(merged)
        env["NO_PROXY"] = val
        env["no_proxy"] = val


def read_service_env(env_file: str | None, service: str) -> dict[str, str]:
    cmd = build_esb_cmd(["env", "var", service, "--format", "json"], env_file)
    result = subprocess.run(
        cmd,
        cwd=CLI_ROOT,
        check=True,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    if result.stderr.strip():
        print(f"[WARN] esb env var output: {result.stderr.strip()}")
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"Failed to parse esb env var output: {exc}") from exc


def apply_proxy_env() -> None:
    """
    Apply proxy environment variable corrections for the Python runner process.

    Note: While the Go CLI ('esb') also implements similar NO_PROXY correction,
    this Python-side implementation remains necessary. Changes to environment
    variables in a child process (the CLI) do not propagate back to the
    parent process (this runner/pytest). Setting NO_PROXY here ensures that
    the Python 'requests' library and other tools bypass the proxy when
    communicating with local ESB services.
    """
    proxy_keys = ("HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy")
    extra_key = env_key("NO_PROXY_EXTRA")

    has_proxy = any(os.environ.get(key) for key in proxy_keys)
    existing_no_proxy = os.environ.get("NO_PROXY") or os.environ.get("no_proxy")
    extra_no_proxy = os.environ.get(extra_key)
    if not (has_proxy or existing_no_proxy or extra_no_proxy):
        return

    def split_no_proxy(value: str | None) -> list[str]:
        if not value:
            return []
        parts = value.replace(";", ",").split(",")
        return [item.strip() for item in parts if item.strip()]

    merged: list[str] = []
    seen: set[str] = set()

    for item in split_no_proxy(existing_no_proxy):
        if item not in seen:
            merged.append(item)
            seen.add(item)

    for item in DEFAULT_NO_PROXY_TARGETS:
        if item and item not in seen:
            merged.append(item)
            seen.add(item)

    for item in split_no_proxy(extra_no_proxy):
        if item and item not in seen:
            merged.append(item)
            seen.add(item)

    if merged:
        merged_value = ",".join(merged)
        os.environ["NO_PROXY"] = merged_value
        os.environ["no_proxy"] = merged_value

    if os.environ.get("HTTP_PROXY") and "http_proxy" not in os.environ:
        os.environ["http_proxy"] = os.environ["HTTP_PROXY"]
    if os.environ.get("http_proxy") and "HTTP_PROXY" not in os.environ:
        os.environ["HTTP_PROXY"] = os.environ["http_proxy"]
    if os.environ.get("HTTPS_PROXY") and "https_proxy" not in os.environ:
        os.environ["https_proxy"] = os.environ["HTTPS_PROXY"]
    if os.environ.get("https_proxy") and "HTTPS_PROXY" not in os.environ:
        os.environ["HTTPS_PROXY"] = os.environ["https_proxy"]


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


def load_ports(env_name: str) -> dict[str, int]:
    port_file = resolve_esb_home(env_name) / "ports.json"
    if port_file.exists():
        return json.loads(port_file.read_text())
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


def apply_gateway_env_from_container(env: dict[str, str], env_file: str | None) -> None:
    gateway_env = read_service_env(env_file, "gateway")
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

    auth_path = gateway_env.get("AUTH_ENDPOINT_PATH")
    if auth_path:
        env["AUTH_ENDPOINT_PATH"] = auth_path
    else:
        env.setdefault("AUTH_ENDPOINT_PATH", "/user/auth/v1")

    for key in ("VERIFY_SSL", "CONTAINERS_NETWORK", "GATEWAY_INTERNAL_URL"):
        value = gateway_env.get(key)
        if value:
            env[key] = value

    tls_enabled = gateway_env.get("AGENT_GRPC_TLS_ENABLED")
    if tls_enabled:
        env["AGENT_GRPC_TLS_ENABLED"] = tls_enabled
        cert_dir = Path(os.environ.get("CERT_DIR", f"~/{BRAND_HOME_DIR}/certs")).expanduser()
        env["AGENT_GRPC_TLS_CA_CERT_PATH"] = str(cert_dir / "rootCA.crt")
        env["AGENT_GRPC_TLS_CERT_PATH"] = str(cert_dir / "client.crt")
        env["AGENT_GRPC_TLS_KEY_PATH"] = str(cert_dir / "client.key")


def ensure_firecracker_node_up() -> None:
    """Fail fast if compute services are not running in firecracker mode."""
    if os.environ.get(env_key("MODE")) != "firecracker":
        return
    print("[WARN] firecracker node check is not implemented for Go CLI")
