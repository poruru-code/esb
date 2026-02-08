# Where: e2e/runner/ports.py
# What: Host port planning helpers for E2E parallel environments.
# Why: Isolate port-allocation policy from runner orchestration logic.
from __future__ import annotations

import socket

from e2e.runner import constants
from e2e.runner.utils import env_key


def _allocate_ports(env_names: list[str]) -> dict[str, dict[str, str]]:
    base = constants.E2E_PORT_BASE
    block = constants.E2E_PORT_BLOCK
    offsets = {
        constants.PORT_GATEWAY_HTTPS: 0,
        constants.PORT_GATEWAY_HTTP: 1,
        constants.PORT_AGENT_GRPC: 2,
        constants.PORT_AGENT_METRICS: 3,
        constants.PORT_VICTORIALOGS: 4,
        constants.PORT_DATABASE: 5,
        constants.PORT_S3: 6,
        constants.PORT_S3_MGMT: 7,
    }
    env_names_sorted = sorted(env_names)

    def _port_available(port: int) -> bool:
        # Bind to 0.0.0.0 so we catch conflicts with services bound to all interfaces.
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            try:
                sock.bind(("0.0.0.0", port))
            except OSError:
                return False
        return True

    # Prefer stable port blocks, but move the whole block-window up if any port
    # is already in use on the host.
    group_size = len(env_names_sorted)
    for shift in range(0, 200):
        bases: dict[str, int] = {}
        ok = True
        for idx, env_name in enumerate(env_names_sorted):
            env_base = base + (idx + shift * group_size) * block
            if env_base + max(offsets.values()) >= 65535:
                ok = False
                break
            ports = [env_base + offset for offset in offsets.values()]
            if not all(_port_available(port) for port in ports):
                ok = False
                break
            bases[env_name] = env_base
        if not ok:
            continue

        plan: dict[str, dict[str, str]] = {}
        for env_name, env_base in bases.items():
            env_ports: dict[str, str] = {}
            for key, offset in offsets.items():
                env_ports[env_key(key)] = str(env_base + offset)
            plan[env_name] = env_ports
        return plan

    raise RuntimeError(
        "Failed to allocate a free host port block for E2E. "
        f"base={base} block={block} envs={env_names_sorted}"
    )
