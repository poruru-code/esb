# Where: e2e/runner/tests/test_warmup_command.py
# What: Unit tests for Java warmup docker command generation.
# Why: Ensure proxy and Maven settings are propagated in constrained environments.
from __future__ import annotations

import os
from pathlib import Path

from e2e.runner import warmup


def test_docker_maven_command_injects_proxy_aliases_and_settings_fallback(monkeypatch, tmp_path):
    project_dir = tmp_path / "java-fixture"
    project_dir.mkdir()

    home_dir = tmp_path / "home"
    m2_dir = home_dir / ".m2"
    m2_dir.mkdir(parents=True)
    settings = m2_dir / "settings.xml"
    settings.write_text("<settings/>\n", encoding="utf-8")

    monkeypatch.setattr(warmup.Path, "home", staticmethod(lambda: home_dir))

    original_access = os.access

    def fake_access(path: os.PathLike[str] | str, mode: int) -> bool:
        if Path(path) == m2_dir and mode == os.W_OK:
            return False
        return original_access(path, mode)

    monkeypatch.setattr(warmup.os, "access", fake_access)
    monkeypatch.setenv("HTTP_PROXY", "http://proxy.example:8080")
    monkeypatch.delenv("http_proxy", raising=False)
    monkeypatch.setenv("NO_PROXY", "localhost,127.0.0.1")
    monkeypatch.delenv("no_proxy", raising=False)

    cmd = warmup._docker_maven_command(project_dir)
    joined = " ".join(str(item) for item in cmd)

    assert f"{settings}:{warmup.HOST_M2_SETTINGS_PATH}:ro" in joined
    assert "HTTP_PROXY=http://proxy.example:8080" in joined
    assert "http_proxy=http://proxy.example:8080" in joined
    assert "NO_PROXY=localhost,127.0.0.1" in joined
    assert "no_proxy=localhost,127.0.0.1" in joined
    assert f"cp {warmup.HOST_M2_SETTINGS_PATH} /tmp/m2/settings.xml" in cmd[-1]


def test_docker_maven_command_prefers_writable_m2_mount(monkeypatch, tmp_path):
    project_dir = tmp_path / "java-fixture"
    project_dir.mkdir()

    home_dir = tmp_path / "home"
    m2_dir = home_dir / ".m2"
    m2_dir.mkdir(parents=True)

    monkeypatch.setattr(warmup.Path, "home", staticmethod(lambda: home_dir))

    original_access = os.access

    def fake_access(path: os.PathLike[str] | str, mode: int) -> bool:
        if Path(path) == m2_dir and mode == os.W_OK:
            return True
        return original_access(path, mode)

    monkeypatch.setattr(warmup.os, "access", fake_access)

    cmd = warmup._docker_maven_command(project_dir)
    joined = " ".join(str(item) for item in cmd)

    assert f"{m2_dir}:/tmp/m2" in joined
    assert f":{warmup.HOST_M2_SETTINGS_PATH}:ro" not in joined
