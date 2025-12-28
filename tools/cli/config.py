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
DEFAULT_CERT_DIR = Path.home() / ".esb" / "certs"


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
