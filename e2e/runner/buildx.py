# Where: e2e/runner/buildx.py
# What: Docker Buildx helper for E2E runs.
# Why: Keep buildx management isolated from runner orchestration.
from __future__ import annotations

import logging
import os
import subprocess
from pathlib import Path

logger = logging.getLogger(__name__)

_PROXY_ENV_KEYS = (
    "HTTP_PROXY",
    "http_proxy",
    "HTTPS_PROXY",
    "https_proxy",
    "NO_PROXY",
    "no_proxy",
)

_PROXY_ALIASES = (
    ("HTTP_PROXY", "http_proxy"),
    ("HTTPS_PROXY", "https_proxy"),
    ("NO_PROXY", "no_proxy"),
)


def _resolve_proxy_value(env: dict[str, str], upper: str, lower: str) -> str:
    upper_value = env.get(upper, "").strip()
    if upper_value:
        return upper_value
    return env.get(lower, "").strip()


def _buildx_proxy_env_map(proxy_source: dict[str, str] | None = None) -> dict[str, str]:
    source = proxy_source or os.environ
    envs: dict[str, str] = {}
    for upper, lower in _PROXY_ALIASES:
        value = _resolve_proxy_value(source, upper, lower)
        if not value:
            continue
        envs[upper] = value
        envs[lower] = value
    return envs


def _quote_buildx_driver_opt(opt: str) -> str:
    if not any(ch in opt for ch in (",", '"', "\n", "\r")):
        return opt
    return '"' + opt.replace('"', '""') + '"'


def _buildx_proxy_driver_opts(proxy_env: dict[str, str]) -> list[str]:
    opts: list[str] = []
    for key in sorted(proxy_env):
        opts.append(_quote_buildx_driver_opt(f"env.{key}={proxy_env[key]}"))
    return opts


def _inspect_builder_network_mode(builder_name: str) -> str:
    inspect_net_cmd = [
        "docker",
        "inspect",
        "-f",
        "{{.HostConfig.NetworkMode}}",
        f"buildx_buildkit_{builder_name}0",
    ]
    net_result = subprocess.run(inspect_net_cmd, capture_output=True, text=True)
    if net_result.returncode != 0:
        return ""
    return net_result.stdout.strip()


def _inspect_builder_proxy_env(builder_name: str) -> dict[str, str]:
    inspect_env_cmd = [
        "docker",
        "inspect",
        "-f",
        "{{range .Config.Env}}{{println .}}{{end}}",
        f"buildx_buildkit_{builder_name}0",
    ]
    env_result = subprocess.run(inspect_env_cmd, capture_output=True, text=True)
    if env_result.returncode != 0:
        return {}
    envs: dict[str, str] = {}
    for line in env_result.stdout.splitlines():
        key, sep, value = line.partition("=")
        if not sep:
            continue
        if key in _PROXY_ENV_KEYS:
            envs[key] = value
    return envs


def _has_proxy_mismatch(existing: dict[str, str], desired: dict[str, str]) -> bool:
    for key in _PROXY_ENV_KEYS:
        desired_value = desired.get(key, "").strip()
        existing_value = existing.get(key, "").strip()
        if desired_value == "":
            if existing_value != "":
                return True
            continue
        if existing_value != desired_value:
            return True
    return False


def ensure_buildx_builder(
    builder_name: str,
    network_mode: str = "host",
    config_path: str | None = None,
    proxy_source: dict[str, str] | None = None,
) -> None:
    if not builder_name:
        return
    config_path = (config_path or os.environ.get("BUILDKITD_CONFIG", "")).strip()
    config_file = None
    if config_path:
        candidate = Path(config_path).expanduser()
        if candidate.exists() and candidate.is_file():
            config_file = str(candidate)
    inspect_cmd = [
        "docker",
        "buildx",
        "inspect",
        "--builder",
        builder_name,
    ]
    result = subprocess.run(inspect_cmd, capture_output=True, text=True)
    desired_proxy_env = _buildx_proxy_env_map(proxy_source)
    needs_recreate = False
    builder_exists = result.returncode == 0

    if builder_exists:
        if network_mode:
            current = _inspect_builder_network_mode(builder_name)
            if not current or current != network_mode:
                logger.warning(
                    "buildx builder '%s' network mode is '%s', expected '%s'. Recreating builder.",
                    builder_name,
                    current or "<unknown>",
                    network_mode,
                )
                needs_recreate = True
        existing_proxy_env = _inspect_builder_proxy_env(builder_name)
        if _has_proxy_mismatch(existing_proxy_env, desired_proxy_env):
            logger.info(
                "buildx builder '%s' proxy settings changed. Recreating builder.", builder_name
            )
            needs_recreate = True

    if builder_exists and not needs_recreate:
        subprocess.run(["docker", "buildx", "use", builder_name], capture_output=True)
        return

    if builder_exists and needs_recreate:
        subprocess.run(["docker", "buildx", "rm", builder_name], capture_output=True)

    create_cmd = [
        "docker",
        "buildx",
        "create",
        "--name",
        builder_name,
        "--driver",
        "docker-container",
        "--use",
        "--bootstrap",
    ]
    if network_mode:
        create_cmd.extend(["--driver-opt", f"network={network_mode}"])
    for opt in _buildx_proxy_driver_opts(desired_proxy_env):
        create_cmd.extend(["--driver-opt", opt])
    if config_file:
        create_cmd.extend(["--buildkitd-config", config_file])
    create_result = subprocess.run(create_cmd, capture_output=True, text=True)
    if create_result.returncode == 0:
        return
    combined = (create_result.stdout + create_result.stderr).lower()
    if "existing instance" in combined or "already exists" in combined:
        subprocess.run(["docker", "buildx", "use", builder_name], capture_output=True)
        return
    create_result.check_returncode()
