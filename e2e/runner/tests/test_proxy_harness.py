# Where: e2e/runner/tests/test_proxy_harness.py
# What: Unit tests for integrated proxy harness behavior.
# Why: Ensure run_tests proxy integration is deterministic and reversible.
from __future__ import annotations

import io
import os
from pathlib import Path
from types import SimpleNamespace

from e2e.runner.proxy_harness import (
    DEFAULT_PROXY_AUTH_PASSWORD,
    DEFAULT_PROXY_AUTH_USER,
    FILTER_UPSTREAM_PLUGIN,
    ProxyHarnessError,
    ProxyHarnessOptions,
    ProxyProcess,
    _effective_no_proxy,
    _filtered_upstream_hosts_from_no_proxy,
    _start_java_proxy_proof_proxy,
    _start_proxy_process,
    _stop_proxy_process,
    build_proxy_env,
    build_proxy_url,
    options_from_args,
    proxy_harness,
)


class _DummyProcess:
    def __init__(self, *, pid: int = 1234) -> None:
        self.pid = pid


class _FakePopen:
    def __init__(self, args, **kwargs) -> None:
        del kwargs
        self.args = list(args)
        self._returncode = None

    def poll(self):
        return self._returncode

    def terminate(self) -> None:
        self._returncode = 0

    def wait(self, timeout=None) -> int:
        del timeout
        self._returncode = 0
        return 0

    def kill(self) -> None:
        self._returncode = -9


def test_options_from_args_enables_proxy_from_flag() -> None:
    args = SimpleNamespace(
        with_proxy=True,
    )

    options = options_from_args(args)

    assert options.enabled is True
    assert options.port == 18888
    assert options.bind_host == "0.0.0.0"


def test_build_proxy_env_sets_proxy_aliases_and_no_proxy_defaults() -> None:
    proxy_url = "http://proxy-user:proxy-pass@proxy.example:18888"
    env = build_proxy_env(
        {},
        proxy_url=proxy_url,
        proxy_host="proxy.example",
    )

    assert env["HTTP_PROXY"] == proxy_url
    assert env["http_proxy"] == proxy_url
    assert env["HTTPS_PROXY"] == proxy_url
    assert env["https_proxy"] == proxy_url
    no_proxy = set(env["NO_PROXY"].split(","))
    assert env["NO_PROXY"] == env["no_proxy"]
    assert "registry" in no_proxy
    assert "localhost" in no_proxy
    assert "proxy.example" in no_proxy
    assert env["E2E_WITH_PROXY"] == "1"


def test_build_proxy_url_includes_fixed_basicauth_and_redaction() -> None:
    full = build_proxy_url("proxy.example", 18888)
    redacted = build_proxy_url("proxy.example", 18888, redact_password=True)

    assert full == "http://proxy-user:proxy-pass@proxy.example:18888"
    assert redacted == "http://proxy-user:***@proxy.example:18888"


def test_filtered_upstream_hosts_from_no_proxy_normalizes_and_filters() -> None:
    hosts = _filtered_upstream_hosts_from_no_proxy(
        "127.0.0.1,localhost,.corp.local,10.0.0.0/8,api.internal:8443,[::1]:443,*.wild"
    )

    assert "127.0.0.1" in hosts
    assert "localhost" in hosts
    assert "corp.local" in hosts
    assert "api.internal" in hosts
    assert "::1" in hosts
    assert "10.0.0.0/8" not in hosts
    assert "*.wild" not in hosts


def test_effective_no_proxy_merges_upper_and_lower_keys() -> None:
    merged = _effective_no_proxy(
        {
            "NO_PROXY": "api.local,registry",
            "no_proxy": "db.local,registry",
        }
    )

    assert merged == "api.local,registry,db.local"


def test_start_proxy_process_injects_basicauth_and_filter_plugin(monkeypatch) -> None:
    captured: dict[str, list[str]] = {}

    monkeypatch.setattr(
        "e2e.runner.proxy_harness._resolve_proxy_command",
        lambda: ["/bin/proxy"],
    )
    monkeypatch.setattr("e2e.runner.proxy_harness._wait_for_port", lambda *_args, **_kwargs: None)

    def _fake_popen(args, **kwargs):
        captured["cmd"] = list(args)
        return _FakePopen(args, **kwargs)

    monkeypatch.setattr("e2e.runner.proxy_harness.subprocess.Popen", _fake_popen)

    proxy = _start_proxy_process(
        bind_host="0.0.0.0",
        host_port=18888,
        filtered_upstream_hosts=["localhost", "registry"],
    )
    try:
        cmd = captured["cmd"]
        assert "--basic-auth" in cmd
        assert f"{DEFAULT_PROXY_AUTH_USER}:{DEFAULT_PROXY_AUTH_PASSWORD}" in cmd
        assert "--plugins" in cmd
        assert FILTER_UPSTREAM_PLUGIN in cmd
        assert "--filtered-upstream-hosts" in cmd
        assert "localhost,registry" in cmd
    finally:
        _stop_proxy_process(proxy)


def test_proxy_harness_applies_and_restores_env(monkeypatch) -> None:
    options = ProxyHarnessOptions(
        enabled=True,
        port=18888,
        bind_host="0.0.0.0",
    )

    calls: dict[str, int] = {"java": 0, "stop": 0}

    dummy_proxy = ProxyProcess(
        process=_DummyProcess(),  # type: ignore[arg-type]
        log_path=Path("/tmp/proxy-e2e-test.log"),
        log_stream=io.StringIO(),
        port=18888,
    )

    monkeypatch.setattr(
        "e2e.runner.proxy_harness._start_proxy_process", lambda **kwargs: dummy_proxy
    )
    monkeypatch.setattr(
        "e2e.runner.proxy_harness._stop_proxy_process",
        lambda *_args, **_kwargs: calls.__setitem__("stop", calls["stop"] + 1),
    )
    monkeypatch.setattr("e2e.runner.proxy_harness._probe_proxy", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(
        "e2e.runner.proxy_harness.resolve_bridge_gateway",
        lambda: "proxy.example",
    )
    monkeypatch.setattr(
        "e2e.runner.proxy_harness._run_java_proxy_proof",
        lambda **_kwargs: calls.__setitem__("java", calls["java"] + 1),
    )

    monkeypatch.setenv("HTTP_PROXY", "http://old.example:8080")
    monkeypatch.setenv("NO_PROXY", "old.local")
    monkeypatch.setenv("E2E_WITH_PROXY", "0")

    with proxy_harness(options):
        assert os.environ["HTTP_PROXY"] == "http://proxy-user:proxy-pass@proxy.example:18888"
        assert os.environ["http_proxy"] == "http://proxy-user:proxy-pass@proxy.example:18888"
        assert "registry" in os.environ["NO_PROXY"]
        assert os.environ["E2E_WITH_PROXY"] == "1"

    assert os.environ["HTTP_PROXY"] == "http://old.example:8080"
    assert os.environ["NO_PROXY"] == "old.local"
    assert os.environ["E2E_WITH_PROXY"] == "0"
    assert calls["java"] == 1
    assert calls["stop"] == 1


def test_proxy_harness_runs_java_proxy_proof(monkeypatch) -> None:
    options = ProxyHarnessOptions(
        enabled=True,
        port=18888,
        bind_host="0.0.0.0",
    )
    calls = {"java": 0}

    dummy_proxy = ProxyProcess(
        process=_DummyProcess(),  # type: ignore[arg-type]
        log_path=Path("/tmp/proxy-e2e-test.log"),
        log_stream=io.StringIO(),
        port=18888,
    )

    monkeypatch.setattr(
        "e2e.runner.proxy_harness._start_proxy_process", lambda **kwargs: dummy_proxy
    )
    monkeypatch.setattr(
        "e2e.runner.proxy_harness._stop_proxy_process", lambda *_args, **_kwargs: None
    )
    monkeypatch.setattr("e2e.runner.proxy_harness._probe_proxy", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(
        "e2e.runner.proxy_harness.resolve_bridge_gateway",
        lambda: "proxy.example",
    )
    monkeypatch.setattr(
        "e2e.runner.proxy_harness._run_java_proxy_proof",
        lambda **_kwargs: calls.__setitem__("java", calls["java"] + 1),
    )

    with proxy_harness(options):
        assert os.environ["HTTP_PROXY"] == "http://proxy-user:proxy-pass@proxy.example:18888"
    assert calls["java"] == 1


def test_proxy_harness_uses_lowercase_no_proxy_for_filtering(monkeypatch) -> None:
    options = ProxyHarnessOptions(
        enabled=True,
        port=18888,
        bind_host="0.0.0.0",
    )
    captured: dict[str, list[str]] = {}

    dummy_proxy = ProxyProcess(
        process=_DummyProcess(),  # type: ignore[arg-type]
        log_path=Path("/tmp/proxy-e2e-test.log"),
        log_stream=io.StringIO(),
        port=18888,
    )

    def _capture_start_proxy_process(**kwargs):
        filtered = kwargs.get("filtered_upstream_hosts", [])
        captured["filtered_upstream_hosts"] = list(filtered)
        return dummy_proxy

    monkeypatch.setattr(
        "e2e.runner.proxy_harness._start_proxy_process",
        _capture_start_proxy_process,
    )
    monkeypatch.setattr(
        "e2e.runner.proxy_harness._stop_proxy_process", lambda *_args, **_kwargs: None
    )
    monkeypatch.setattr("e2e.runner.proxy_harness._probe_proxy", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(
        "e2e.runner.proxy_harness.resolve_bridge_gateway",
        lambda: "proxy.example",
    )
    monkeypatch.setattr("e2e.runner.proxy_harness._run_java_proxy_proof", lambda **_kwargs: None)

    monkeypatch.delenv("NO_PROXY", raising=False)
    monkeypatch.setenv("no_proxy", "lower.internal")

    with proxy_harness(options):
        pass

    assert "lower.internal" in captured["filtered_upstream_hosts"]


def test_start_java_proxy_proof_proxy_stops_process_on_probe_failure(monkeypatch) -> None:
    dummy_proxy = ProxyProcess(
        process=_DummyProcess(),  # type: ignore[arg-type]
        log_path=Path("/tmp/proxy-e2e-java-proof-test.log"),
        log_stream=io.StringIO(),
        port=19999,
    )
    calls = {"stop": 0}

    monkeypatch.setattr("e2e.runner.proxy_harness._find_free_port", lambda: 19999)
    monkeypatch.setattr(
        "e2e.runner.proxy_harness._start_proxy_process", lambda **_kwargs: dummy_proxy
    )
    monkeypatch.setattr(
        "e2e.runner.proxy_harness._probe_proxy",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(ProxyHarnessError("probe failed")),
    )
    monkeypatch.setattr(
        "e2e.runner.proxy_harness._stop_proxy_process",
        lambda *_args, **_kwargs: calls.__setitem__("stop", calls["stop"] + 1),
    )

    try:
        _start_java_proxy_proof_proxy(bind_host="0.0.0.0", no_proxy="localhost")
    except ProxyHarnessError as exc:
        assert "probe failed" in str(exc)
    else:
        raise AssertionError("expected ProxyHarnessError")

    assert calls["stop"] == 1
