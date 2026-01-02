# Where: tools/cli/config.py
# What: CLI configuration constants and path helpers.
# Why: Centralize CLI defaults and filesystem locations.
from pathlib import Path

import os


def find_project_root(current_path: Path = None) -> Path:
    """Find the project root by searching for pyproject.toml."""
    if current_path is None:
        current_path = Path.cwd()

    for path in [current_path] + list(current_path.parents):
        if (path / "pyproject.toml").exists():
            return path

    return Path(__file__).parent.parent.parent.resolve()


PROJECT_ROOT = find_project_root()
TOOLS_DIR = PROJECT_ROOT / "tools"
GENERATOR_DIR = TOOLS_DIR / "generator"
PROVISIONER_DIR = TOOLS_DIR / "provisioner"

# Environment Isolation Logic
def get_env_name() -> str:
    """Resolve the current environment name."""
    # Priority: 1. ESB_ENV env var (set by cli args usually), 2. "default"
    return os.getenv("ESB_ENV", "default")

def get_esb_home() -> Path:
    """Resolve the ESB home directory based on environment."""
    env_name = get_env_name()
    base = Path.home() / ".esb"
    if env_name == "default":
        return base
    return base / env_name

def get_cert_dir() -> Path:
    return get_esb_home() / "certs"

def get_mode_config_path() -> Path:
    return get_esb_home() / "mode.yaml"

# Backward compatibility constants (Try to use functions instead where possible)
ESB_HOME = Path.home() / ".esb" # Warning: This is static, might not match current env if used directly.
DEFAULT_CERT_DIR = ESB_HOME / "certs" # Deprecated use get_cert_dir()
MODE_CONFIG_PATH = ESB_HOME / "mode.yaml" # Deprecated use get_mode_config_path()

MODE_CONFIG_VERSION = 1
ESB_MODE_CONTAINERD = "containerd"
ESB_MODE_FIRECRACKER = "firecracker"
VALID_ESB_MODES = (ESB_MODE_CONTAINERD, ESB_MODE_FIRECRACKER)
DEFAULT_ESB_MODE = ESB_MODE_CONTAINERD
DEFAULT_AGENT_GRPC_PORT = 50051

def get_port_mapping(env_name: str = None) -> dict[str, str]:
    """Calculate port mappings based on environment name."""
    if env_name is None:
        env_name = get_env_name()
    
    if env_name == "default":
        offset = 0
    else:
        # Deterministic offset based on hash, restricted to avoid overflow
        # Using a simple hash that fits in 0-99 range to avoid collisions with other services?
        # Let's use a modulus 50 range, stepping by 100?
        # Actually, user wants multiple environments.
        # Let's simple offset: hash specific string to int.
        import hashlib
        h = int(hashlib.md5(env_name.encode()).hexdigest(), 16)
        offset = (h % 50) * 100 # Up to 50 concurrent environments, 100 ports apart.
        # Ensure offset is not 0 for non-default to verify isolation?
        # If hash collides with default (unlikely), it's fine if explicit default is used.
        # But for safety, maybe add 1000? 
        if offset == 0: offset = 1000

    # Base ports
    mapping = {
        "ESB_PORT_GATEWAY_HTTPS": str(443 + offset),
        "ESB_PORT_GATEWAY_HTTP": str(80 + offset),
        "ESB_PORT_AGENT_GRPC": str(50051 + offset),
        "ESB_PORT_STORAGE": str(9000 + offset),
        "ESB_PORT_STORAGE_MGMT": str(9001 + offset),
        "ESB_PORT_DATABASE": str(8001 + offset), # Host mapped port for Scylla
        "ESB_PORT_REGISTRY": str(5010 + offset),
        "ESB_PORT_VICTORIALOGS": str(9428 + offset),
    }
    return mapping

def get_subnet_config(env_name: str = None) -> dict[str, str]:
    """Calculate subnet configuration (simple offset strategy)."""
    if env_name is None:
        env_name = get_env_name()
    
    if env_name == "default":
        ext_idx = 50
        run_idx = 20
    else:
        import hashlib
        h = int(hashlib.md5(env_name.encode()).hexdigest(), 16)
        ext_idx = 60 + (h % 100) # 172.60.x.x to 172.159.x.x
        run_idx = 100 + (h % 100) # 172.100.x.x ...
    
    return {
        "ESB_SUBNET_EXTERNAL": f"172.{ext_idx}.0.0/16",
        "ESB_NETWORK_EXTERNAL": f"esb_ext_{env_name}",
        "RUNTIME_NET_SUBNET": f"172.{run_idx}.0.0/16",
        "RUNTIME_NODE_IP": f"172.{run_idx}.0.10",
        "LAMBDA_NETWORK": f"esb_int_{env_name}",
    }

COMPOSE_BASE_FILE = PROJECT_ROOT / "docker-compose.yml"
COMPOSE_CONTROL_FILE = PROJECT_ROOT / "docker-compose.yml"
COMPOSE_WORKER_FILE = PROJECT_ROOT / "docker-compose.worker.yml"
COMPOSE_FC_FILE = PROJECT_ROOT / "docker-compose.fc.yml"
COMPOSE_CONTAINERD_FILE = PROJECT_ROOT / "docker-compose.containerd.yml"

# Deprecated constants (kept for safety if referenced elsewhere)
COMPOSE_COMPUTE_FILE = PROJECT_ROOT / "docker-compose.node.yml" 
COMPOSE_ADAPTER_FILE = PROJECT_ROOT / "docker-compose.containerd.yml"
REMOTE_COMPOSE_DIR = ".esb/compose"


def _resolve_template_yaml() -> Path:
    """Resolve the template path (default search order)."""
    # Path priority:
    # 1. ESB_TEMPLATE environment variable
    # 2. template.yaml in the current directory
    # 3. template.yaml in the project root
    # 4. tests/fixtures/template.yaml (default)
    env_template = os.environ.get("ESB_TEMPLATE")
    if env_template:
        return Path(env_template).resolve()
    elif (Path.cwd() / "template.yaml").exists():
        return Path.cwd() / "template.yaml"
    elif (PROJECT_ROOT / "template.yaml").exists():
        return PROJECT_ROOT / "template.yaml"
    else:
        return PROJECT_ROOT / "tests" / "fixtures" / "template.yaml"


# Initialize with default values.
TEMPLATE_YAML = _resolve_template_yaml()
E2E_DIR = TEMPLATE_YAML.parent
DEFAULT_ROUTING_YML = E2E_DIR / "config" / "routing.yml"
DEFAULT_FUNCTIONS_YML = E2E_DIR / "config" / "functions.yml"


def set_template_yaml(template_path: str) -> None:
    """Set the template path from CLI arguments (highest priority)."""
    global TEMPLATE_YAML, E2E_DIR, DEFAULT_ROUTING_YML, DEFAULT_FUNCTIONS_YML

    # WSL compatibility: normalize /mnt/C/path... -> /mnt/c/path...
    parts = template_path.split("/")
    if len(parts) > 3 and parts[1] == "mnt" and len(parts[2]) == 1 and parts[2].isupper():
        parts[2] = parts[2].lower()
        template_path = "/".join(parts)

    TEMPLATE_YAML = Path(template_path).resolve()
    E2E_DIR = TEMPLATE_YAML.parent
    DEFAULT_ROUTING_YML = E2E_DIR / "config" / "routing.yml"
    DEFAULT_FUNCTIONS_YML = E2E_DIR / "config" / "functions.yml"
