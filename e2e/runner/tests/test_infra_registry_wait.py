# Where: e2e/runner/tests/test_infra_registry_wait.py
# What: Unit tests for local registry readiness probing.
# Why: Prevent proxy-related regressions in infra registry startup checks.
from __future__ import annotations

import subprocess
from http.client import HTTPException

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


def test_registry_v2_ready_returns_false_on_http_exception(monkeypatch):
    class _BrokenConnection:
        def __init__(self, _host: str, timeout: int) -> None:
            _ = timeout
            pass

        def request(self, _method: str, _path: str) -> None:
            return

        def getresponse(self):
            raise HTTPException("malformed response")

        def close(self) -> None:
            return

    monkeypatch.setattr(infra, "HTTPConnection", _BrokenConnection)
    assert infra._registry_v2_ready("127.0.0.1:5010", timeout=2) is False


def test_ensure_existing_registry_container_running_when_running(monkeypatch):
    monkeypatch.setattr(infra.subprocess, "check_output", lambda *args, **kwargs: "true\n")
    assert infra._ensure_existing_registry_container_running("esb-infra-registry") is True


def test_ensure_existing_registry_container_running_starts_stopped(monkeypatch):
    calls: list[list[str]] = []

    monkeypatch.setattr(infra.subprocess, "check_output", lambda *args, **kwargs: "false\n")

    def fake_check_call(cmd, **kwargs):
        calls.append(list(cmd))
        return 0

    monkeypatch.setattr(infra.subprocess, "check_call", fake_check_call)

    assert infra._ensure_existing_registry_container_running("esb-infra-registry") is True
    assert calls == [["docker", "start", "esb-infra-registry"]]


def test_ensure_existing_registry_container_running_missing_container(monkeypatch):
    def fake_check_output(*args, **kwargs):
        raise subprocess.CalledProcessError(returncode=1, cmd=args[0])

    monkeypatch.setattr(infra.subprocess, "check_output", fake_check_output)
    assert infra._ensure_existing_registry_container_running("esb-infra-registry") is False


def test_ensure_infra_up_reuses_existing_container(monkeypatch):
    monkeypatch.setattr(infra.os.path, "exists", lambda path: True)
    monkeypatch.setattr(
        infra, "_ensure_existing_registry_container_running", lambda container_name: True
    )
    monkeypatch.setattr(infra, "get_registry_config", lambda: ("127.0.0.1:5010", "registry:5010"))

    waits: list[str] = []
    monkeypatch.setattr(infra, "wait_for_registry_ready", lambda host_addr: waits.append(host_addr))

    compose_calls: list[list[str]] = []

    def fake_check_call(cmd, **kwargs):
        compose_calls.append(list(cmd))
        return 0

    monkeypatch.setattr(infra.subprocess, "check_call", fake_check_call)

    infra.ensure_infra_up("/tmp/project")

    assert waits == ["127.0.0.1:5010"]
    assert compose_calls == []
