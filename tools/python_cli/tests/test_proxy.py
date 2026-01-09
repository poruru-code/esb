# Where: tools/python_cli/tests/test_proxy.py
# What: Unit tests for proxy helper utilities.
# Why: Ensure proxy detection and NO_PROXY merging behave consistently.

from tools.python_cli.core import proxy


def test_collect_proxy_env_skips_when_unset(monkeypatch):
    for key in (
        "HTTP_PROXY",
        "http_proxy",
        "HTTPS_PROXY",
        "https_proxy",
        "NO_PROXY",
        "no_proxy",
        "ESB_NO_PROXY_EXTRA",
    ):
        monkeypatch.delenv(key, raising=False)
    env = proxy.collect_proxy_env()
    assert env == {}


def test_collect_proxy_env_merges_defaults(monkeypatch):
    for key in (
        "HTTP_PROXY",
        "http_proxy",
        "HTTPS_PROXY",
        "https_proxy",
        "NO_PROXY",
        "no_proxy",
        "ESB_NO_PROXY_EXTRA",
    ):
        monkeypatch.delenv(key, raising=False)
    monkeypatch.setenv("HTTP_PROXY", "http://proxy.corp:3128")
    monkeypatch.delenv("NO_PROXY", raising=False)
    env = proxy.collect_proxy_env()

    assert env["HTTP_PROXY"] == "http://proxy.corp:3128"
    merged = env["NO_PROXY"].split(",")
    assert "localhost" in merged
    assert "registry" in merged


def test_prepare_env_respects_extra_no_proxy(monkeypatch):
    for key in ("HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy"):
        monkeypatch.delenv(key, raising=False)
    monkeypatch.setenv("HTTPS_PROXY", "https://secure.proxy:8443")
    monkeypatch.setenv("ESB_NO_PROXY_EXTRA", "corp.internal,.svc.cluster.local")

    prepared = proxy.prepare_env({})
    assert prepared["HTTPS_PROXY"] == "https://secure.proxy:8443"
    assert "corp.internal" in prepared["NO_PROXY"].split(",")
