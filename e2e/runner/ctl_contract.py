"""Shared E2E contract for ctl command discovery."""

from __future__ import annotations

from collections.abc import Mapping

from tools.cli.branding_constants_gen import DEFAULT_CTL_BIN

ENV_CTL_BIN = "CTL_BIN"
ENV_CTL_BIN_RESOLVED = "CTL_BIN_RESOLVED"

CTL_REQUIRED_SUBCOMMANDS: tuple[tuple[str, ...], ...] = (
    ("deploy", "--help"),
    ("provision", "--help"),
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
