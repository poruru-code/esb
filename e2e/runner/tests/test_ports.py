# Where: e2e/runner/tests/test_ports.py
# What: Unit tests for E2E host port allocation planning.
# Why: Ensure deterministic block allocation and collision avoidance.
from __future__ import annotations

from e2e.runner import constants
from e2e.runner.ports import _allocate_ports
from e2e.runner.utils import env_key


class _FakeSocket:
    def __init__(self, unavailable: set[int]) -> None:
        self._unavailable = unavailable

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, tb) -> None:
        return None

    def bind(self, address) -> None:
        _host, port = address
        if port in self._unavailable:
            raise OSError("port is unavailable")


def test_allocate_ports_uses_stable_sorted_blocks(monkeypatch):
    monkeypatch.setattr(constants, "E2E_PORT_BASE", 20000)
    monkeypatch.setattr(constants, "E2E_PORT_BLOCK", 10)
    monkeypatch.setattr(
        "e2e.runner.ports.socket.socket",
        lambda *_args, **_kwargs: _FakeSocket(set()),
    )

    plan = _allocate_ports(["beta", "alpha"])

    assert plan["alpha"][env_key(constants.PORT_GATEWAY_HTTPS)] == "20000"
    assert plan["alpha"][env_key(constants.PORT_AGENT_GRPC)] == "20002"
    assert plan["beta"][env_key(constants.PORT_GATEWAY_HTTPS)] == "20010"
    assert plan["beta"][env_key(constants.PORT_S3_MGMT)] == "20017"


def test_allocate_ports_skips_block_with_conflict(monkeypatch):
    monkeypatch.setattr(constants, "E2E_PORT_BASE", 21000)
    monkeypatch.setattr(constants, "E2E_PORT_BLOCK", 10)
    monkeypatch.setattr(
        "e2e.runner.ports.socket.socket",
        lambda *_args, **_kwargs: _FakeSocket({21000}),
    )

    plan = _allocate_ports(["env1"])

    assert plan["env1"][env_key(constants.PORT_GATEWAY_HTTPS)] == "21010"
