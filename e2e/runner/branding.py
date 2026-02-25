"""Shared branding defaults and derivation helpers for E2E runner code."""

from __future__ import annotations

import re
from pathlib import Path

DEFAULT_ENV_PREFIX = "ESB"
DEFAULT_BRAND_SLUG = "esb"

_SLUG_RE = re.compile(r"[^a-z0-9_-]+")


def sanitize_brand_slug(value: str | None) -> str:
    if value is None:
        return ""
    lowered = value.strip().lower()
    if lowered == "":
        return ""
    cleaned = _SLUG_RE.sub("-", lowered).strip("-_")
    return cleaned


def resolve_project_name(value: str | None) -> str:
    if value is None:
        return DEFAULT_BRAND_SLUG
    normalized = value.strip()
    if normalized == "":
        return DEFAULT_BRAND_SLUG
    return normalized


def resolve_brand_slug(project_name: str | None) -> str:
    slug = sanitize_brand_slug(project_name)
    if slug == "":
        return DEFAULT_BRAND_SLUG
    return slug


def brand_home_dir(project_name: str | None = None) -> str:
    return f".{resolve_brand_slug(project_name)}"


def cert_dir(project_root: Path, project_name: str | None = None) -> Path:
    return (project_root / brand_home_dir(project_name) / "certs").expanduser()


def buildkitd_config_path(project_root: Path, project_name: str | None = None) -> Path:
    return (project_root / brand_home_dir(project_name) / "buildkitd.toml").expanduser()


def lambda_network_name(project_name: str | None, env_name: str) -> str:
    return f"{resolve_brand_slug(project_name)}_int_{env_name}"


def root_ca_mount_id(project_name: str | None) -> str:
    return f"{resolve_brand_slug(project_name)}_root_ca"


def buildx_builder_name(project_name: str | None) -> str:
    return f"{resolve_brand_slug(project_name)}-buildx"


def infra_registry_container_name(project_name: str | None = None) -> str:
    return f"{resolve_brand_slug(project_name)}-infra-registry"
