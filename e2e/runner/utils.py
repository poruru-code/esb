import os
import re
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import List, Optional

import toml

_SLUG_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")
_ENV_PREFIX_RE = re.compile(r"^[A-Z][A-Z0-9_]*$")

# Project root
# Assuming this file is in e2e/runner/utils.py, parent.parent is "e2e", parent.parent.parent is root.
PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
GO_CLI_ROOT = PROJECT_ROOT / "cli"


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


def _build_branding(brand: str) -> BrandingLite:
    slug = _normalize_slug(brand)
    env_prefix = _normalize_env_prefix(brand)
    paths = {
        "home_dir": f".{slug}",
        "output_dir": f".{slug}",
    }
    return BrandingLite(env_prefix=env_prefix, slug=slug, paths=paths)


def _load_slug_from_cert_config() -> str | None:
    config_path = PROJECT_ROOT / "tools" / "cert-gen" / "config.toml"
    if not config_path.exists():
        return None
    config = toml.load(config_path)
    output_dir = str(config.get("certificate", {}).get("output_dir", "")).strip()
    if not output_dir:
        return None
    expanded = Path(os.path.expanduser(output_dir))
    if expanded.name != "certs":
        return None
    parent_name = expanded.parent.name
    if not parent_name.startswith(".") or len(parent_name) <= 1:
        return None
    slug = parent_name[1:]
    return slug if _SLUG_RE.fullmatch(slug) else None


def load_branding():
    slug = _load_slug_from_cert_config()
    if slug:
        return _build_branding(slug)

    raise RuntimeError(
        "Branding config not found. Ensure tools/cert-gen/config.toml uses ~/.<slug>/certs."
    )


BRANDING = load_branding()
ENV_PREFIX = BRANDING.env_prefix
BRAND_SLUG = BRANDING.slug
BRAND_HOME_DIR = BRANDING.paths["home_dir"]
BRAND_OUTPUT_DIR = BRANDING.paths["output_dir"]
E2E_STATE_ROOT = PROJECT_ROOT / "e2e" / "fixtures" / BRAND_OUTPUT_DIR


def env_key(suffix: str) -> str:
    return f"{ENV_PREFIX}_{suffix}"


def apply_esb_aliases(env: dict[str, str]) -> None:
    """Expose ESB_* aliases for tests when the brand prefix differs."""
    if ENV_PREFIX == "ESB":
        return
    prefix = f"{ENV_PREFIX}_"
    for key, value in list(env.items()):
        if not key.startswith(prefix):
            continue
        esb_key = f"ESB_{key[len(prefix) :]}"
        env.setdefault(esb_key, value)


def resolve_env_file_path(env_file: Optional[str]) -> Optional[str]:
    if not env_file:
        return None
    env_file_path = Path(env_file)
    if not env_file_path.is_absolute():
        env_file_path = PROJECT_ROOT / env_file_path
    return str(env_file_path.absolute())


def build_esb_cmd(args: List[str], env_file: Optional[str]) -> List[str]:
    base_cmd = ["go", "run", "./cmd/esb"]
    env_file_path = resolve_env_file_path(env_file)
    if env_file_path:
        base_cmd.extend(["--env-file", env_file_path])
    return base_cmd + args


def run_esb(
    args: List[str], check: bool = True, env_file: Optional[str] = None, verbose: bool = False
) -> subprocess.CompletedProcess:
    """Helper to run the esb CLI."""
    if verbose and "build" in args:
        # Build command has its own verbose flag
        if "--verbose" not in args and "-v" not in args:
            args = ["build", "--verbose"] + [a for a in args if a != "build"]

    cmd = build_esb_cmd(args, env_file)
    if verbose:
        print(f"Running: {' '.join(cmd)}")
    return subprocess.run(cmd, cwd=GO_CLI_ROOT, check=check, stdin=subprocess.DEVNULL)
