# Where: e2e/runner/tests/test_lifecycle_gateway_proxy.py
# What: Unit tests for gateway wait proxy handling.
# Why: Ensure localhost health checks are not routed via host proxy settings.
from __future__ import annotations

from e2e.runner import constants, lifecycle
from e2e.runner.utils import env_key


class _FakeResponse:
    def __init__(self, status_code: int) -> None:
        self.status_code = status_code


class _FakeSession:
    instances: list["_FakeSession"] = []

    def __init__(self) -> None:
        self.trust_env = True
        self.calls: list[tuple[str, float, bool, bool]] = []

    def __enter__(self) -> "_FakeSession":
        type(self).instances.append(self)
        return self

    def __exit__(self, _exc_type, _exc, _tb) -> bool:
        return False

    def get(self, url: str, timeout: float, verify: bool) -> _FakeResponse:
        self.calls.append((url, timeout, verify, self.trust_env))
        return _FakeResponse(200)


def test_wait_for_gateway_disables_requests_trust_env(monkeypatch):
    _FakeSession.instances = []
    monkeypatch.setattr(lifecycle.requests, "Session", _FakeSession)

    lifecycle.wait_for_gateway(
        "e2e-docker",
        ports={env_key(constants.PORT_GATEWAY_HTTPS): 18443},
        timeout=1.0,
        interval=0.0,
    )

    assert _FakeSession.instances
    session = _FakeSession.instances[0]
    assert session.calls
    _url, _timeout, _verify, trust_env = session.calls[0]
    assert trust_env is False
