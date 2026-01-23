import os
import re
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import List, Optional

_SLUG_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")
_ENV_PREFIX_RE = re.compile(r"^[A-Z][A-Z0-9_]*$")

# Project root
# Assuming this file is in e2e/runner/utils.py, parent.parent is "e2e", parent.parent.parent is root.
PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
CLI_ROOT = PROJECT_ROOT / "cli"
GO_CLI_ROOT = CLI_ROOT  # Alias for backward compatibility if needed


@dataclass(frozen=True)
class BrandingLite:
    env_prefix: str
    slug: str
    paths: dict[str, str]


def _normalize_slug(value: str) -> str:
    cleaned = re.sub(r"[^a-z0-9]+", "-", value.strip().lower()).strip("-")
    if not cleaned or not _SLUG_RE.fullmatch(cleaned):
        raise RuntimeError(f"Invalid branding slug: {value!r}")
    return cleaned


def _normalize_env_prefix(value: str) -> str:
    cleaned = re.sub(r"[^A-Z0-9]+", "_", value.strip().upper()).strip("_")
    if not cleaned or not cleaned[0].isalpha() or not _ENV_PREFIX_RE.fullmatch(cleaned):
        raise RuntimeError(f"Invalid branding env_prefix: {value!r}")
    return cleaned


def _build_branding(project_name: str) -> BrandingLite:
    slug = _normalize_slug(project_name)
    env_prefix = _normalize_env_prefix(project_name)
    paths = {
        "home_dir": f".{slug}",
        "output_dir": f".{slug}",
    }
    return BrandingLite(env_prefix=env_prefix, slug=slug, paths=paths)


def _read_defaults_env() -> dict[str, str]:
    """Read all key-values from config/defaults.env."""
    defaults_path = PROJECT_ROOT / "config" / "defaults.env"
    if not defaults_path.exists():
        return {}

    values = {}
    for line in defaults_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            continue
        key, _, value = line.partition("=")
        values[key.strip()] = value.strip()
    return values


def load_branding():
    """Load branding configuration from config/defaults.env and inject into os.environ."""
    defaults = _read_defaults_env()

    # Inject into os.environ so child processes (CLI, Docker) inherit them
    for k, v in defaults.items():
        if k not in os.environ:
            os.environ[k] = v

    project_name = defaults.get("CLI_CMD")
    if project_name:
        return _build_branding(project_name)

    raise RuntimeError("Branding config not found. Ensure config/defaults.env contains CLI_CMD.")


BRANDING = load_branding()
ENV_PREFIX = BRANDING.env_prefix
BRAND_SLUG = BRANDING.slug
BRAND_HOME_DIR = BRANDING.paths["home_dir"]
BRAND_OUTPUT_DIR = BRANDING.paths["output_dir"]
E2E_STATE_ROOT = PROJECT_ROOT / "e2e" / "fixtures" / BRAND_OUTPUT_DIR


def env_key(suffix: str) -> str:
    # Transitional logic: some variables no longer use prefixes
    prefix_less = {
        "DATA_PLANE_HOST",
        "PORT_GATEWAY_HTTPS",
        "PORT_GATEWAY_HTTP",
        "PORT_AGENT_GRPC",
        "PORT_VICTORIALOGS",
        "PORT_REGISTRY",
        "PORT_DATABASE",
        "PORT_S3",
        "PORT_S3_MGMT",
        "PORT_AGENT_METRICS",
        "NO_PROXY_EXTRA",
    }
    if suffix in prefix_less:
        return suffix
    return f"{ENV_PREFIX}_{suffix}"


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


def resolve_env_file_path(env_file: Optional[str]) -> Optional[str]:
    if not env_file:
        return None
    env_file_path = Path(env_file)
    if not env_file_path.is_absolute():
        env_file_path = PROJECT_ROOT / env_file_path
    return str(env_file_path.absolute())


def build_esb_cmd(args: List[str], env_file: Optional[str]) -> List[str]:
    # Use the compiled binary from the path (installed via mise setup)
    defaults = _read_defaults_env()
    cli_cmd = defaults.get("CLI_CMD", "esb")
    base_cmd = [cli_cmd]
    env_file_path = resolve_env_file_path(env_file)
    if env_file_path:
        base_cmd.extend(["--env-file", env_file_path])
    return base_cmd + args


def run_esb(
    args: List[str],
    check: bool = True,
    env_file: Optional[str] = None,
    verbose: bool = False,
    env: Optional[dict[str, str]] = None,
) -> subprocess.CompletedProcess:
    """Helper to run the esb CLI."""
    if verbose and "build" in args:
        # Build command has its own verbose flag
        if "--verbose" not in args and "-v" not in args:
            args = ["build", "--verbose"] + [a for a in args if a != "build"]

    cmd = build_esb_cmd(args, env_file)
    if verbose:
        print(f"Running: {' '.join(cmd)}")

    # Use shell=False and pass the command as a list to rely on PATH
    # If env is provided, merge it with os.environ to preserve PATH, etc.
    run_env = os.environ.copy()
    if env:
        run_env.update(env)

    return subprocess.run(cmd, check=check, stdin=subprocess.DEVNULL, env=run_env)
