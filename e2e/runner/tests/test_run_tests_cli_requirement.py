# Where: e2e/runner/tests/test_run_tests_cli_requirement.py
# What: Unit tests for deciding whether run_tests.py needs artifactctl.
# Why: E2E deploy phase requires artifactctl, while test-only runs do not.
from __future__ import annotations

import os
from types import SimpleNamespace

import pytest

from e2e.run_tests import ensure_artifactctl_available, requires_artifactctl


def _args(*, test_only: bool = False) -> SimpleNamespace:
    return SimpleNamespace(test_only=test_only)


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


def test_ensure_artifactctl_available_uses_path(monkeypatch) -> None:
    monkeypatch.delenv("ARTIFACTCTL_BIN", raising=False)
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: "/usr/local/bin/artifactctl")
    resolved = ensure_artifactctl_available()
    assert resolved == "/usr/local/bin/artifactctl"
    assert os.environ["ARTIFACTCTL_BIN_RESOLVED"] == "/usr/local/bin/artifactctl"


def test_ensure_artifactctl_available_uses_override(monkeypatch) -> None:
    monkeypatch.setenv("ARTIFACTCTL_BIN", "/opt/bin/artifactctl")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: name)
    original_path = os.environ.get("PATH", "")
    resolved = ensure_artifactctl_available()
    assert resolved == "/opt/bin/artifactctl"
    assert os.environ["ARTIFACTCTL_BIN_RESOLVED"] == "/opt/bin/artifactctl"
    assert os.environ["PATH"].startswith("/opt/bin")
    monkeypatch.setenv("PATH", original_path)


def test_ensure_artifactctl_available_fails_when_missing(monkeypatch) -> None:
    monkeypatch.delenv("ARTIFACTCTL_BIN", raising=False)
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: None)
    with pytest.raises(SystemExit):
        ensure_artifactctl_available()


def test_ensure_artifactctl_available_fails_when_override_missing(monkeypatch) -> None:
    monkeypatch.setenv("ARTIFACTCTL_BIN", "/missing/artifactctl")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: None)
    with pytest.raises(SystemExit):
        ensure_artifactctl_available()
