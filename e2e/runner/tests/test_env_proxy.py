# Where: e2e/runner/tests/test_env_proxy.py
# What: Unit tests for E2E runtime proxy environment defaults.
# Why: Keep proxy/no_proxy behavior aligned with CLI runtime env logic.
from __future__ import annotations

from e2e.runner import env as runner_env
from e2e.runner.utils import env_key


def test_calculate_runtime_env_applies_proxy_defaults(monkeypatch):
    monkeypatch.setenv("HTTP_PROXY", "http://proxy.example:8080")
    monkeypatch.delenv("http_proxy", raising=False)
    monkeypatch.delenv("HTTPS_PROXY", raising=False)
    monkeypatch.delenv("https_proxy", raising=False)
    monkeypatch.delenv("NO_PROXY", raising=False)
    monkeypatch.delenv("no_proxy", raising=False)
    monkeypatch.setenv(env_key("NO_PROXY_EXTRA"), "corp.internal")

    env = runner_env.calculate_runtime_env(
        "esb",
        "e2e-proxy",
        "docker",
    )

    assert env["HTTP_PROXY"] == "http://proxy.example:8080"
    assert env["http_proxy"] == "http://proxy.example:8080"
    assert env["NO_PROXY"] == env["no_proxy"]
    no_proxy = set(env["NO_PROXY"].split(","))
    assert "localhost" in no_proxy
    assert "127.0.0.1" in no_proxy
    assert "registry" in no_proxy
    assert "corp.internal" in no_proxy


def test_calculate_runtime_env_skips_proxy_defaults_without_proxy_inputs(monkeypatch):
    monkeypatch.delenv("HTTP_PROXY", raising=False)
    monkeypatch.delenv("http_proxy", raising=False)
    monkeypatch.delenv("HTTPS_PROXY", raising=False)
    monkeypatch.delenv("https_proxy", raising=False)
    monkeypatch.delenv("NO_PROXY", raising=False)
    monkeypatch.delenv("no_proxy", raising=False)
    monkeypatch.delenv(env_key("NO_PROXY_EXTRA"), raising=False)

    env = runner_env.calculate_runtime_env(
        "esb",
        "e2e-proxy",
        "docker",
    )

    assert env.get("NO_PROXY", "") == ""
    assert env.get("no_proxy", "") == ""
