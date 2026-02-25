"""Shared E2E contract for ctl command discovery and capability checks."""

from __future__ import annotations

import json
from collections.abc import Mapping

from e2e.runner.branding_constants_gen import DEFAULT_CTL_BIN

ENV_CTL_BIN = "CTL_BIN"
ENV_CTL_BIN_RESOLVED = "CTL_BIN_RESOLVED"

CTL_CAPABILITIES_SCHEMA_VERSION = 1
CTL_REQUIRED_CONTRACTS: dict[str, int] = {
    "maven_shim_ensure_schema_version": 1,
    "fixture_image_ensure_schema_version": 1,
}
CTL_REQUIRED_SUBCOMMANDS: tuple[tuple[str, ...], ...] = (
    ("deploy", "--help"),
    ("provision", "--help"),
    ("internal", "fixture-image", "ensure", "--help"),
    ("internal", "maven-shim", "ensure", "--help"),
    ("internal", "capabilities", "--help"),
)


def configured_ctl_bin_from_env(env: Mapping[str, object]) -> str:
    """Read explicit ctl binary override from env, if configured."""
    return str(env.get(ENV_CTL_BIN, "")).strip()


def resolve_ctl_bin_from_env(env: Mapping[str, object]) -> str:
    """Resolve ctl binary from orchestrator env values."""
    resolved = str(env.get(ENV_CTL_BIN_RESOLVED, "")).strip()
    if resolved:
        return resolved

    configured = configured_ctl_bin_from_env(env)
    if configured:
        return configured

    return DEFAULT_CTL_BIN


def parse_ctl_capabilities(raw_output: str) -> dict | None:
    """Extract trailing JSON payload from capabilities command output."""
    for line in reversed(raw_output.splitlines()):
        raw = line.strip()
        if raw == "":
            continue
        try:
            payload = json.loads(raw)
        except json.JSONDecodeError:
            continue
        if isinstance(payload, dict):
            return payload
    return None
