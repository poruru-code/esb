# Where: tools/python_cli/core/proxy.py
# What: Proxy detection and propagation utilities for CLI workflows.
# Why: Normalize corporate proxy handling across build, compose, and provisioning flows.
from __future__ import annotations

import os
from typing import Iterable

# Default destinations that should bypass corporate proxies for local ESB services.
DEFAULT_NO_PROXY_TARGETS: tuple[str, ...] = (
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
)

_PROXY_KEYS = ("HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy")
_NO_PROXY_KEYS = ("NO_PROXY", "no_proxy")
_EXTRA_NO_PROXY_ENV = "ESB_NO_PROXY_EXTRA"


def _split_no_proxy(value: str | None) -> list[str]:
    if not value:
        return []
    parts = value.replace(";", ",").split(",")
    return [item.strip() for item in parts if item.strip()]


def merge_no_proxy(existing: str | None, extras: Iterable[str] | None = None) -> str:
    """
    Merge NO_PROXY entries from environment and defaults while preserving order.
    """
    merged: list[str] = []
    seen: set[str] = set()

    for item in _split_no_proxy(existing):
        if item not in seen:
            merged.append(item)
            seen.add(item)

    for item in extras or []:
        if item and item not in seen:
            merged.append(item)
            seen.add(item)

    for item in _split_no_proxy(os.environ.get(_EXTRA_NO_PROXY_ENV)):
        if item and item not in seen:
            merged.append(item)
            seen.add(item)

    return ",".join(merged)


def collect_proxy_env(extra_no_proxy: Iterable[str] | None = None) -> dict[str, str]:
    """
    Collect proxy-related environment variables with merged NO_PROXY values.
    """
    env: dict[str, str] = {}

    for key in (*_PROXY_KEYS, *_NO_PROXY_KEYS):
        value = os.environ.get(key)
        if value:
            env[key] = value

    if env.get("HTTP_PROXY") and "http_proxy" not in env:
        env["http_proxy"] = env["HTTP_PROXY"]
    if env.get("http_proxy") and "HTTP_PROXY" not in env:
        env["HTTP_PROXY"] = env["http_proxy"]
    if env.get("HTTPS_PROXY") and "https_proxy" not in env:
        env["https_proxy"] = env["HTTPS_PROXY"]
    if env.get("https_proxy") and "HTTPS_PROXY" not in env:
        env["HTTPS_PROXY"] = env["https_proxy"]

    has_proxy_vars = any(os.environ.get(key) for key in _PROXY_KEYS)
    existing_no_proxy = env.get("NO_PROXY") or env.get("no_proxy")
    has_extra = os.environ.get(_EXTRA_NO_PROXY_ENV)

    if has_proxy_vars or existing_no_proxy or has_extra:
        # Dynamically add project-specific service names if ESB_PROJECT_NAME is set
        project_name = os.environ.get("ESB_PROJECT_NAME")
        dynamic_targets = []
        if project_name:
            # Common internal services that might be accessed via service discovery
            services = [
                "s3-storage",
                "database",
                "victorialogs",
                "gateway",
                "agent",
                "registry",
            ]
            dynamic_targets = [f"{project_name}-{s}" for s in services]

        # Combine defaults, dynamic targets, and extras
        all_targets = list(DEFAULT_NO_PROXY_TARGETS) + dynamic_targets
        if extra_no_proxy:
            all_targets.extend(extra_no_proxy)

        merged_no_proxy = merge_no_proxy(
            existing_no_proxy,
            extras=all_targets,
        )
        if merged_no_proxy:
            env["NO_PROXY"] = merged_no_proxy
            env["no_proxy"] = merged_no_proxy

    return env


def apply_proxy_env(extra_no_proxy: Iterable[str] | None = None) -> dict[str, str]:
    """
    Update os.environ with merged proxy variables (returns the values applied).
    """
    env = collect_proxy_env(extra_no_proxy=extra_no_proxy)
    os.environ.update(env)
    return env


def prepare_env(
    base_env: dict[str, str] | None = None, extra_no_proxy: Iterable[str] | None = None
) -> dict[str, str]:
    """
    Prepare an environment mapping for subprocess calls with proxy variables injected.
    """
    env = dict(base_env or os.environ)
    env.update(collect_proxy_env(extra_no_proxy=extra_no_proxy))
    return env


def docker_build_args(extra_no_proxy: Iterable[str] | None = None) -> dict[str, str]:
    """
    Build arguments for Docker image builds so network calls honor proxies.
    """
    env = collect_proxy_env(extra_no_proxy=extra_no_proxy)
    build_args: dict[str, str] = {}

    for key in (*_PROXY_KEYS, *_NO_PROXY_KEYS):
        if key in env:
            build_args[key] = env[key]

    return build_args


def provision_proxy_data(extra_no_proxy: Iterable[str] | None = None) -> dict[str, str]:
    """
    Map proxy settings to provisioning metadata (pyinfra host.data).
    """
    env = collect_proxy_env(extra_no_proxy=extra_no_proxy)
    return {
        "http_proxy": env.get("HTTP_PROXY") or env.get("http_proxy") or "",
        "https_proxy": env.get("HTTPS_PROXY") or env.get("https_proxy") or "",
        "no_proxy": env.get("NO_PROXY") or env.get("no_proxy") or "",
    }
