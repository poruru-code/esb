# Where: e2e/runner/tests/test_env_contract.py
# What: Shared runtime env contract checks against fixture data.
# Why: Keep Python and Go runtime env defaults aligned via the same fixture.
from __future__ import annotations

import os
from unittest import mock

import yaml

from e2e.runner import constants
from e2e.runner.env import (
    calculate_runtime_env,
    env_external_subnet_index,
    env_runtime_subnet_index,
)
from e2e.runner.utils import PROJECT_ROOT


def _load_contract_cases() -> list[dict[str, object]]:
    path = PROJECT_ROOT / "e2e" / "contracts" / "runtime_env_contract.yaml"
    data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    return list(data.get("cases", []))


def test_runtime_env_subnet_indices_match_contract() -> None:
    for case in _load_contract_cases():
        env_name = str(case["env"])
        assert env_external_subnet_index(env_name) == int(case["external_subnet_index"])
        runtime_index = case.get("runtime_subnet_index")
        if runtime_index is not None:
            assert env_runtime_subnet_index(env_name) == int(runtime_index)


def test_calculate_runtime_env_matches_contract_defaults() -> None:
    with mock.patch.dict(os.environ, {}, clear=True):
        for case in _load_contract_cases():
            env_name = str(case["env"])
            env = calculate_runtime_env("esb", env_name, "docker")
            assert env[constants.ENV_SUBNET_EXTERNAL] == str(case["subnet_external"])
            assert env[constants.ENV_LAMBDA_NETWORK] == str(case["lambda_network"])
            if "runtime_net_subnet" in case:
                assert env[constants.ENV_RUNTIME_NET_SUBNET] == str(case["runtime_net_subnet"])
            if "runtime_node_ip" in case:
                assert env[constants.ENV_RUNTIME_NODE_IP] == str(case["runtime_node_ip"])


def test_calculate_runtime_env_prefers_explicit_overrides() -> None:
    explicit = {
        constants.ENV_SUBNET_EXTERNAL: "172.200.0.0/16",
        constants.ENV_RUNTIME_NET_SUBNET: "172.210.0.0/16",
        constants.ENV_RUNTIME_NODE_IP: "172.210.0.10",
        constants.ENV_LAMBDA_NETWORK: "esb_int_override",
    }

    with mock.patch.dict(os.environ, {}, clear=True):
        env = calculate_runtime_env("esb", "e2e-docker", "docker", env_overrides=explicit)

    for key, value in explicit.items():
        assert env[key] == value


def test_calculate_runtime_env_containerd_does_not_use_legacy_runtime_network_defaults() -> None:
    with mock.patch.dict(os.environ, {}, clear=True):
        env = calculate_runtime_env("esb", "e2e-containerd", "containerd")

    assert env[constants.ENV_SUBNET_EXTERNAL] == "172.70.0.0/16"
    assert env[constants.ENV_LAMBDA_NETWORK] == "esb_int_e2e-containerd"
    assert constants.ENV_RUNTIME_NET_SUBNET not in env
    assert constants.ENV_RUNTIME_NODE_IP not in env
