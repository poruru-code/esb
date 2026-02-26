"""Branding and ctl binary resolution helpers."""

from __future__ import annotations

import re
from collections.abc import Mapping

_imported_default_brand_slug: str | None
_imported_default_ctl_bin: str | None

try:
    from e2e.runner.branding_constants_gen import (  # pragma: no cover - import guard
        DEFAULT_BRAND_SLUG as _GENERATED_DEFAULT_BRAND_SLUG,
    )
    from e2e.runner.branding_constants_gen import (
        DEFAULT_CTL_BIN as _GENERATED_DEFAULT_CTL_BIN,
    )
except Exception:  # pragma: no cover - fallback for minimal contexts
    _imported_default_brand_slug = None
    _imported_default_ctl_bin = None
else:
    _imported_default_brand_slug = str(_GENERATED_DEFAULT_BRAND_SLUG)
    _imported_default_ctl_bin = str(_GENERATED_DEFAULT_CTL_BIN)


DEFAULT_BRAND_SLUG = _imported_default_brand_slug or "brand"
DEFAULT_CTL_BIN = _imported_default_ctl_bin or "ctl"
ENV_CTL_BIN = "CTL_BIN"

_SLUG_RE = re.compile(r"[^a-z0-9_-]+")
_PROJECT_RE = re.compile(r"[^a-z0-9_.-]+")


def sanitize_brand_slug(value: str | None) -> str:
    if value is None:
        return ""
    lowered = value.strip().lower()
    if lowered == "":
        return ""
    return _SLUG_RE.sub("-", lowered).strip("-_")


def resolve_brand_slug(project_name: str | None) -> str:
    slug = sanitize_brand_slug(project_name)
    if slug == "":
        return DEFAULT_BRAND_SLUG
    return slug


def brand_home_dir(project_name: str | None = None) -> str:
    return f".{resolve_brand_slug(project_name)}"


def normalize_project_name(project_name: str) -> str:
    lowered = project_name.strip().lower()
    normalized = _PROJECT_RE.sub("-", lowered)
    normalized = re.sub(r"^[^a-z0-9]+", "", normalized)
    normalized = re.sub(r"[^a-z0-9]+$", "", normalized)
    return normalized


def resolve_compose_project_name(base_project: str, env_name: str | None) -> str:
    raw = base_project if not env_name else f"{base_project}-{env_name}"
    normalized = normalize_project_name(raw)
    if normalized == "":
        raise ValueError("resolved PROJECT_NAME is empty after normalization")
    return normalized


def configured_ctl_bin(env: Mapping[str, str] | None = None) -> str:
    if env is None:
        return ""
    return str(env.get(ENV_CTL_BIN, "")).strip()


def resolve_ctl_bin(
    *,
    override: str | None = None,
    env: Mapping[str, str] | None = None,
) -> str:
    if override and override.strip():
        return override.strip()
    configured = configured_ctl_bin(env)
    if configured:
        return configured
    return DEFAULT_CTL_BIN
