"""
Where: e2e/scenarios/smoke/test_containerd_network_identity.py
What: Containerd CNI identity and NAT wiring smoke checks.
Why: Ensure E2E containerd stacks use derived CNI identity instead of shared legacy subnet defaults.
"""

from __future__ import annotations

import ipaddress
import os
import subprocess
import time

import pytest

_RULE_WAIT_TIMEOUT_SECONDS = 30.0
_RULE_WAIT_INTERVAL_SECONDS = 1.0


def _assert_command_ok(cmd: list[str], *, context: str) -> str:
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    assert result.returncode == 0, (
        f"{context} failed (rc={result.returncode}): stderr={result.stderr} stdout={result.stdout}"
    )
    return result.stdout


def _container_names() -> tuple[str, str]:
    project_name = os.getenv("PROJECT_NAME", "").strip()
    assert project_name, "PROJECT_NAME must be set by E2E runner"
    return f"{project_name}-agent", f"{project_name}-runtime-node"


def _read_cni_identity(agent_container: str) -> dict[str, str]:
    text = _assert_command_ok(
        ["docker", "exec", agent_container, "sh", "-lc", "cat /var/lib/cni/esb-cni.env"],
        context="read CNI identity file",
    )
    values: dict[str, str] = {}
    for raw_line in text.splitlines():
        line = raw_line.strip()
        if not line or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()

    required_keys = ("CNI_NETWORK", "CNI_BRIDGE", "CNI_SUBNET", "CNI_GW_IP")
    missing = [key for key in required_keys if not values.get(key)]
    assert not missing, f"CNI identity file missing required keys: {missing} (content={values})"
    return values


def _wait_for_iptables_line(
    runtime_node_container: str,
    *,
    command: str,
    expected_line: str,
) -> None:
    deadline = time.time() + _RULE_WAIT_TIMEOUT_SECONDS
    last_output = ""
    while time.time() < deadline:
        result = subprocess.run(
            ["docker", "exec", runtime_node_container, "sh", "-lc", command],
            capture_output=True,
            text=True,
            check=False,
        )
        if result.returncode == 0:
            lines = [line.strip() for line in result.stdout.splitlines() if line.strip()]
            if expected_line in lines:
                return
            last_output = result.stdout
        else:
            last_output = result.stderr
        time.sleep(_RULE_WAIT_INTERVAL_SECONDS)

    raise AssertionError(
        f"iptables rule not found within timeout: expected={expected_line!r} "
        f"command={command!r} last_output={last_output!r}"
    )


def test_containerd_cni_identity_and_nat_rules() -> None:
    if os.getenv("MODE", "").strip().lower() != "containerd":
        pytest.skip("containerd-only check")

    agent_container, runtime_node_container = _container_names()
    identity = _read_cni_identity(agent_container)

    cni_network = identity["CNI_NETWORK"]
    cni_bridge = identity["CNI_BRIDGE"]
    cni_subnet = identity["CNI_SUBNET"]
    cni_gateway_ip = identity["CNI_GW_IP"]

    parsed_subnet = ipaddress.ip_network(cni_subnet, strict=False)
    parsed_gateway = ipaddress.ip_address(cni_gateway_ip)
    assert parsed_subnet.version == 4, f"unexpected CNI subnet family: {cni_subnet}"
    assert str(parsed_subnet.network_address).startswith("10."), (
        f"CNI subnet must be 10.x derived subnet, got: {cni_subnet}"
    )
    assert parsed_gateway in parsed_subnet, (
        f"CNI gateway must belong to subnet: gw={cni_gateway_ip} subnet={cni_subnet}"
    )
    assert cni_network.endswith("-net"), f"unexpected CNI network name: {cni_network}"
    assert cni_bridge.startswith("esb-"), f"unexpected CNI bridge name: {cni_bridge}"

    _wait_for_iptables_line(
        runtime_node_container,
        command="iptables -t nat -S",
        expected_line=f"-A POSTROUTING -s {cni_subnet} ! -d {cni_subnet} -j MASQUERADE",
    )
    _wait_for_iptables_line(
        runtime_node_container,
        command="iptables -S FORWARD",
        expected_line=f"-A FORWARD -i {cni_bridge} -j ACCEPT",
    )
    _wait_for_iptables_line(
        runtime_node_container,
        command="iptables -S FORWARD",
        expected_line=f"-A FORWARD -o {cni_bridge} -j ACCEPT",
    )
