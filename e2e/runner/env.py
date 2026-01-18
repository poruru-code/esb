import json
import os
import subprocess
from pathlib import Path

from e2e.runner.utils import (
    BRAND_HOME_DIR,
    GO_CLI_ROOT,
    build_esb_cmd,
    env_key,
)


def read_service_env(env_file: str | None, service: str) -> dict[str, str]:
    cmd = build_esb_cmd(["env", "var", service, "--format", "json"], env_file)
    result = subprocess.run(
        cmd,
        cwd=GO_CLI_ROOT,
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


DEFAULT_NO_PROXY_TARGETS = [
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
]


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
    for env_var, port in ports.items():
        os.environ[env_var] = str(port)

    gateway_key = env_key("PORT_GATEWAY_HTTPS")
    if gateway_key in ports:
        gateway_port = ports[gateway_key]
        os.environ["GATEWAY_PORT"] = str(gateway_port)
        os.environ["GATEWAY_URL"] = f"https://localhost:{gateway_port}"

    vl_key = env_key("PORT_VICTORIALOGS")
    if vl_key in ports:
        vl_port = ports[vl_key]
        os.environ["VICTORIALOGS_PORT"] = str(vl_port)
        os.environ["VICTORIALOGS_URL"] = f"http://localhost:{vl_port}"

    agent_key = env_key("PORT_AGENT_GRPC")
    if agent_key in ports:
        agent_port = ports[agent_key]
        os.environ["AGENT_GRPC_ADDRESS"] = f"localhost:{agent_port}"


def apply_gateway_env_from_container(env: dict[str, str], env_file: str | None) -> None:
    gateway_env = read_service_env(env_file, "gateway")
    required = ("AUTH_USER", "AUTH_PASS", "X_API_KEY", "RUSTFS_ACCESS_KEY", "RUSTFS_SECRET_KEY")
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


def ensure_firecracker_node_up() -> None:
    """Fail fast if compute services are not running in firecracker mode."""
    if os.environ.get(env_key("MODE")) != "firecracker":
        return
    print("[WARN] firecracker node check is not implemented for Go CLI")
