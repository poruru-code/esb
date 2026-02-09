# Where: e2e/runner/tests/test_infra_registry_wait.py
# What: Unit tests for local registry readiness probing.
# Why: Prevent proxy-related regressions in infra registry startup checks.
from __future__ import annotations

from e2e.runner import infra


class _FakeResponse:
    def __init__(self, status: int) -> None:
        self.status = status

    def read(self) -> bytes:
        return b""


class _FakeConnection:
    calls: list[tuple[str, str | int]] = []

    def __init__(self, host: str, timeout: int) -> None:
        type(self).calls.append(("init", host))
        type(self).calls.append(("timeout", timeout))

    def request(self, method: str, path: str) -> None:
        type(self).calls.append(("request", f"{method} {path}"))

    def getresponse(self) -> _FakeResponse:
        type(self).calls.append(("response", "ok"))
        return _FakeResponse(200)

    def close(self) -> None:
        type(self).calls.append(("close", "done"))


def test_registry_v2_ready_uses_direct_http_connection(monkeypatch):
    _FakeConnection.calls = []
    monkeypatch.setattr(infra, "HTTPConnection", _FakeConnection)

    assert infra._registry_v2_ready("127.0.0.1:5010", timeout=3)
    assert ("init", "127.0.0.1:5010") in _FakeConnection.calls
    assert ("timeout", 3) in _FakeConnection.calls
    assert ("request", "GET /v2/") in _FakeConnection.calls


def test_wait_for_registry_ready_retries_until_success(monkeypatch):
    attempts: list[int] = []

    def fake_ready(_host_addr: str, timeout: int = 2) -> bool:
        attempts.append(timeout)
        return len(attempts) >= 3

    sleeps: list[int] = []
    monkeypatch.setattr(infra, "_registry_v2_ready", fake_ready)
    monkeypatch.setattr(infra.time, "sleep", lambda sec: sleeps.append(sec))

    infra.wait_for_registry_ready("127.0.0.1:5010", timeout=5)

    assert attempts == [2, 2, 2]
    assert sleeps == [1, 1]
