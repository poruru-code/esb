# Where: tools/tests/test_tinyproxy_runner.py
# What: Unit tests for tinyproxy E2E runner environment helpers.
# Why: Keep proxy env construction deterministic and regression-safe.
from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

from tools.e2e_proxy import run_with_tinyproxy


def test_split_no_proxy_supports_multiple_separators() -> None:
    assert run_with_tinyproxy.split_no_proxy("a;b, c ,,d") == ["a", "b", "c", "d"]


def test_merge_no_proxy_deduplicates_while_preserving_order() -> None:
    merged = run_with_tinyproxy.merge_no_proxy(
        "localhost,corp.internal",
        ["localhost", "127.0.0.1", "registry"],
        "registry;corp.internal;extra.local",
    )
    assert merged == "localhost,corp.internal,127.0.0.1,registry,extra.local"


def test_build_proxy_env_sets_aliases_and_merges_no_proxy() -> None:
    env = {"NO_PROXY": "corp.internal"}
    updated = run_with_tinyproxy.build_proxy_env(
        env,
        proxy_url="http://172.17.0.1:18888",
        proxy_host="172.17.0.1",
        no_proxy_extra="extra.local",
    )

    assert updated["HTTP_PROXY"] == "http://172.17.0.1:18888"
    assert updated["http_proxy"] == "http://172.17.0.1:18888"
    assert updated["HTTPS_PROXY"] == "http://172.17.0.1:18888"
    assert updated["https_proxy"] == "http://172.17.0.1:18888"
    assert updated["NO_PROXY"] == updated["no_proxy"]

    no_proxy_set = set(updated["NO_PROXY"].split(","))
    assert "corp.internal" in no_proxy_set
    assert "localhost" in no_proxy_set
    assert "127.0.0.1" in no_proxy_set
    assert "registry" in no_proxy_set
    assert "172.17.0.1" in no_proxy_set
    assert "extra.local" in no_proxy_set


def test_normalize_command_strips_double_dash_separator() -> None:
    assert run_with_tinyproxy.normalize_command(["--", "uv", "run"]) == ["uv", "run"]


def test_resolve_proxy_auth_requires_both_username_and_password() -> None:
    with pytest.raises(ValueError, match="requires both username and password"):
        run_with_tinyproxy.resolve_proxy_auth("user", "")


def test_resolve_proxy_auth_allows_url_safe_special_characters() -> None:
    assert run_with_tinyproxy.resolve_proxy_auth("user.name", "p@ss:word") == (
        "user.name",
        "p@ss:word",
    )


def test_resolve_proxy_auth_rejects_whitespace() -> None:
    with pytest.raises(ValueError, match="must not include whitespace"):
        run_with_tinyproxy.resolve_proxy_auth("user", "proxy pass")


def test_build_proxy_url_without_auth() -> None:
    assert run_with_tinyproxy.build_proxy_url("172.17.0.1", 18888) == "http://172.17.0.1:18888"


def test_build_proxy_url_with_auth_and_redaction() -> None:
    auth = ("proxyuser", "proxypass")
    assert (
        run_with_tinyproxy.build_proxy_url("172.17.0.1", 18888, auth=auth)
        == "http://proxyuser:proxypass@172.17.0.1:18888"
    )
    assert (
        run_with_tinyproxy.build_proxy_url(
            "172.17.0.1",
            18888,
            auth=auth,
            redact_password=True,
        )
        == "http://proxyuser:***@172.17.0.1:18888"
    )


def test_parse_args_supports_skip_java_proxy_proof_flag() -> None:
    args = run_with_tinyproxy.parse_args(["--skip-java-proxy-proof", "--check-only"])
    assert args.skip_java_proxy_proof is True
    assert args.check_only is True


def test_render_maven_proxy_settings_contains_proxy_blocks() -> None:
    settings = run_with_tinyproxy._render_maven_proxy_settings(
        "http://proxyuser:proxypass@172.17.0.1:18888",
        "localhost,.corp.internal,registry:5010",
    )
    assert "<proxy>" in settings
    assert "<protocol>http</protocol>" in settings
    assert "<protocol>https</protocol>" in settings
    assert "<username>proxyuser</username>" in settings
    assert "<password>proxypass</password>" in settings
    assert "<nonProxyHosts>localhost|*.corp.internal|registry</nonProxyHosts>" in settings


def test_run_java_proxy_proof_fails_when_broken_proxy_succeeds(monkeypatch) -> None:
    results = [
        subprocess.CompletedProcess(args=["docker"], returncode=0, stdout="ok", stderr=""),
        subprocess.CompletedProcess(args=["docker"], returncode=0, stdout="unexpected", stderr=""),
    ]

    def fake_run_java_maven_with_settings(*, repo_root: Path, settings_xml: str):
        assert repo_root == Path("/tmp/repo")
        assert "<settings>" in settings_xml
        return results.pop(0)

    monkeypatch.setattr(
        run_with_tinyproxy,
        "_run_java_maven_with_settings",
        fake_run_java_maven_with_settings,
    )
    monkeypatch.setattr(
        run_with_tinyproxy,
        "_start_java_proxy_proof_proxy",
        lambda *, image, auth: ("proof-container", "http://172.17.0.1:18888"),
    )
    removed: list[str] = []
    monkeypatch.setattr(run_with_tinyproxy, "_remove_container", lambda name: removed.append(name))

    with pytest.raises(RuntimeError, match="unexpectedly succeeded"):
        run_with_tinyproxy._run_java_proxy_proof(
            repo_root=Path("/tmp/repo"),
            no_proxy="localhost",
            image=run_with_tinyproxy.DEFAULT_IMAGE,
            auth=None,
        )
    assert removed == ["proof-container"]
