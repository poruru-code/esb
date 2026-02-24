# Where: e2e/runner/tests/test_run_tests_cli_requirement.py
# What: Unit tests for deciding whether run_tests.py needs artifactctl.
# Why: E2E deploy phase requires artifactctl, while test-only runs do not.
from __future__ import annotations

import os
from pathlib import Path
from types import SimpleNamespace

import pytest

from e2e.run_tests import ensure_artifactctl_available, requires_artifactctl


def _args(*, test_only: bool = False) -> SimpleNamespace:
    return SimpleNamespace(test_only=test_only)


def _probe_ok(*args, **kwargs):
    del args, kwargs
    return SimpleNamespace(returncode=0, stdout="usage")


def test_requires_artifactctl_false_for_test_only() -> None:
    scenarios = {"docker": object()}
    assert requires_artifactctl(_args(test_only=True), scenarios) is False


def test_requires_artifactctl_true_when_scenarios_exist() -> None:
    scenarios = {
        "docker": object(),
        "containerd": object(),
    }
    assert requires_artifactctl(_args(), scenarios) is True


def test_requires_artifactctl_false_without_scenarios() -> None:
    assert requires_artifactctl(_args(), {}) is False


def test_ensure_artifactctl_available_prefers_local_bin(monkeypatch, tmp_path) -> None:
    monkeypatch.delenv("ARTIFACTCTL_BIN", raising=False)
    monkeypatch.setenv("HOME", str(tmp_path))
    local_bin = tmp_path / ".local" / "bin" / "artifactctl"
    local_bin.parent.mkdir(parents=True)
    local_bin.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
    local_bin.chmod(0o755)
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: "/usr/local/bin/artifactctl")
    monkeypatch.setattr("e2e.run_tests.subprocess.run", _probe_ok)
    resolved = ensure_artifactctl_available()
    expected = str(local_bin.resolve())
    assert resolved == expected
    assert os.environ["ARTIFACTCTL_BIN_RESOLVED"] == expected


def test_ensure_artifactctl_available_uses_path(monkeypatch, tmp_path) -> None:
    monkeypatch.delenv("ARTIFACTCTL_BIN", raising=False)
    monkeypatch.setenv("HOME", str(tmp_path))
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: "/usr/local/bin/artifactctl")
    monkeypatch.setattr("e2e.run_tests.subprocess.run", _probe_ok)
    resolved = ensure_artifactctl_available()
    assert resolved == "/usr/local/bin/artifactctl"
    assert os.environ["ARTIFACTCTL_BIN_RESOLVED"] == "/usr/local/bin/artifactctl"


def test_ensure_artifactctl_available_uses_override(monkeypatch) -> None:
    monkeypatch.setenv("ARTIFACTCTL_BIN", "/opt/bin/artifactctl")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: name)
    monkeypatch.setattr("e2e.run_tests.subprocess.run", _probe_ok)
    original_path = os.environ.get("PATH", "")
    resolved = ensure_artifactctl_available()
    assert resolved == "/opt/bin/artifactctl"
    assert os.environ["ARTIFACTCTL_BIN_RESOLVED"] == "/opt/bin/artifactctl"
    assert os.environ.get("PATH", "") == original_path


def test_ensure_artifactctl_available_normalizes_relative_override(monkeypatch, tmp_path) -> None:
    monkeypatch.chdir(tmp_path)
    monkeypatch.setenv("ARTIFACTCTL_BIN", "./bin/artifactctl")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: "./bin/artifactctl")
    monkeypatch.setattr("e2e.run_tests.subprocess.run", _probe_ok)
    original_path = os.environ.get("PATH", "")
    resolved = ensure_artifactctl_available()
    expected = str((Path(tmp_path) / "bin" / "artifactctl").resolve())
    assert resolved == expected
    assert os.environ["ARTIFACTCTL_BIN_RESOLVED"] == expected
    assert os.environ.get("PATH", "") == original_path


def test_ensure_artifactctl_available_fails_when_missing(monkeypatch) -> None:
    monkeypatch.delenv("ARTIFACTCTL_BIN", raising=False)
    monkeypatch.setenv("HOME", "/tmp/esb-no-local-artifactctl")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: None)
    with pytest.raises(SystemExit):
        ensure_artifactctl_available()


def test_ensure_artifactctl_available_fails_when_override_missing(monkeypatch) -> None:
    monkeypatch.setenv("ARTIFACTCTL_BIN", "/missing/artifactctl")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: None)
    with pytest.raises(SystemExit):
        ensure_artifactctl_available()


def test_ensure_artifactctl_available_fails_when_subcommand_contract_missing(monkeypatch) -> None:
    monkeypatch.delenv("ARTIFACTCTL_BIN", raising=False)
    monkeypatch.setenv("HOME", "/tmp/esb-no-local-artifactctl")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: "/usr/local/bin/artifactctl")
    monkeypatch.setattr(
        "e2e.run_tests.subprocess.run",
        lambda *args, **kwargs: SimpleNamespace(returncode=1, stdout="unknown command: deploy"),
    )
    with pytest.raises(SystemExit):
        ensure_artifactctl_available()


def test_ensure_artifactctl_available_fails_when_internal_maven_shim_missing(monkeypatch) -> None:
    monkeypatch.delenv("ARTIFACTCTL_BIN", raising=False)
    monkeypatch.setenv("HOME", "/tmp/esb-no-local-artifactctl")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: "/usr/local/bin/artifactctl")

    def fake_probe(args, **kwargs):
        del kwargs
        if args[-3:] == ["maven-shim", "ensure", "--help"]:
            return SimpleNamespace(returncode=1, stdout="unknown command: internal")
        return SimpleNamespace(returncode=0, stdout="usage")

    monkeypatch.setattr("e2e.run_tests.subprocess.run", fake_probe)
    with pytest.raises(SystemExit):
        ensure_artifactctl_available()
