import os
import re
import subprocess
import time
from pathlib import Path
from typing import Optional

from e2e.runner.branding import (
    DEFAULT_ENV_PREFIX,
    brand_home_dir,
)

# Project root
# Assuming this file is in e2e/runner/utils.py, parent.parent is "e2e", parent.parent.parent is root.
PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent

ENV_PREFIX = DEFAULT_ENV_PREFIX
DEFAULT_BRAND_HOME_DIR = brand_home_dir()
E2E_STATE_ROOT = PROJECT_ROOT / DEFAULT_BRAND_HOME_DIR / "e2e" / "state"
E2E_ARTIFACT_ROOT = PROJECT_ROOT / "e2e" / "artifacts"


def env_key(suffix: str) -> str:
    # Transitional logic: some variables no longer use prefixes
    prefix_less = {
        "DATA_PLANE_HOST",
        "CONTAINER_REGISTRY_INSECURE",
        "PORT_GATEWAY_HTTPS",
        "PORT_GATEWAY_HTTP",
        "PORT_AGENT_GRPC",
        "PORT_VICTORIALOGS",
        "PORT_REGISTRY",
        "PORT_DATABASE",
        "PORT_S3",
        "PORT_S3_MGMT",
        "PORT_AGENT_METRICS",
    }
    if suffix in prefix_less:
        return suffix
    return f"{ENV_PREFIX}_{suffix}"


_TAG_PART_RE = re.compile(r"[^a-z0-9_.-]+")


def _sanitize_tag_part(value: str) -> str:
    cleaned = _TAG_PART_RE.sub("-", value.strip().lower()).strip("._-")
    return cleaned or "default"


def _git_short_sha(root: Path) -> str | None:
    try:
        result = subprocess.run(
            ["git", "-C", str(root), "rev-parse", "--short", "HEAD"],
            capture_output=True,
            text=True,
            check=True,
        )
    except Exception:
        return None
    value = result.stdout.strip()
    return value or None


def build_unique_tag(env_name: str) -> str:
    safe_env = _sanitize_tag_part(env_name or "default")
    # In CI, we use unique tags to prevent collision and ensure traceability.
    if os.environ.get("CI"):
        short_sha = _git_short_sha(PROJECT_ROOT)
        if short_sha:
            return f"e2e-{safe_env}-{short_sha}"
        return f"e2e-{safe_env}-{int(time.time())}"

    # Locally, we use a fixed tag to avoid flooding `docker images` with stale layers.
    return f"e2e-{safe_env}-latest"


def resolve_env_file_path(env_file: Optional[str]) -> Optional[str]:
    if not env_file:
        return None
    env_file_path = Path(env_file)
    if not env_file_path.is_absolute():
        env_file_path = PROJECT_ROOT / env_file_path
    return str(env_file_path.absolute())
