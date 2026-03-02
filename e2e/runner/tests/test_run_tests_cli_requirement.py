# Where: e2e/runner/tests/test_run_tests_cli_requirement.py
# What: Unit tests for deciding whether run_tests.py needs ctl.
# Why: E2E deploy phase requires ctl, while test-only runs do not.
from __future__ import annotations

import os
from pathlib import Path
from types import SimpleNamespace

import pytest

from e2e.run_tests import ensure_ctl_available, ensure_project_scoped_ctl_wrapper, requires_ctl
from e2e.runner.ctl_contract import DEFAULT_CTL_BIN, ENV_CTL_BIN, ENV_CTL_BIN_RESOLVED


def _args(*, test_only: bool = False) -> SimpleNamespace:
    return SimpleNamespace(test_only=test_only)


def _probe_ok(args, **kwargs):
    del kwargs
    return SimpleNamespace(returncode=0, stdout="usage")


def test_requires_ctl_false_for_test_only() -> None:
    scenarios = {"docker": object()}
    assert requires_ctl(_args(test_only=True), scenarios) is False


def test_requires_ctl_true_when_scenarios_exist() -> None:
    scenarios = {
        "docker": object(),
        "containerd": object(),
    }
    assert requires_ctl(_args(), scenarios) is True


def test_requires_ctl_false_without_scenarios() -> None:
    assert requires_ctl(_args(), {}) is False


def test_ensure_project_scoped_ctl_wrapper_creates_wrapper(monkeypatch, tmp_path) -> None:
    monkeypatch.setattr("e2e.run_tests.PROJECT_ROOT", tmp_path)
    wrapper = ensure_project_scoped_ctl_wrapper(DEFAULT_CTL_BIN)
    expected = (tmp_path / ".e2e" / "bin" / DEFAULT_CTL_BIN).resolve()
    assert wrapper == str(expected)
    assert expected.exists()
    assert os.access(expected, os.X_OK)
    content = expected.read_text(encoding="utf-8")
    assert "tools.cli.cli" in content


def test_ensure_ctl_available_uses_repo_scoped_wrapper(monkeypatch, tmp_path) -> None:
    monkeypatch.delenv(ENV_CTL_BIN, raising=False)
    monkeypatch.setattr("e2e.run_tests.PROJECT_ROOT", tmp_path)
    monkeypatch.setattr("e2e.run_tests.subprocess.run", _probe_ok)
    resolved = ensure_ctl_available()
    expected = str((tmp_path / ".e2e" / "bin" / DEFAULT_CTL_BIN).resolve())
    assert resolved == expected
    assert os.environ[ENV_CTL_BIN_RESOLVED] == expected


def test_ensure_ctl_available_falls_back_to_path_when_wrapper_creation_fails(monkeypatch) -> None:
    monkeypatch.delenv(ENV_CTL_BIN, raising=False)
    monkeypatch.setattr(
        "e2e.run_tests.ensure_project_scoped_ctl_wrapper",
        lambda _: (_ for _ in ()).throw(OSError("read-only filesystem")),
    )
    monkeypatch.setattr(
        "e2e.run_tests.shutil.which",
        lambda name: f"/usr/local/bin/{DEFAULT_CTL_BIN}" if name == DEFAULT_CTL_BIN else None,
    )
    monkeypatch.setattr("e2e.run_tests.subprocess.run", _probe_ok)
    resolved = ensure_ctl_available()
    expected = f"/usr/local/bin/{DEFAULT_CTL_BIN}"
    assert resolved == expected
    assert os.environ[ENV_CTL_BIN_RESOLVED] == expected


def test_ensure_ctl_available_uses_override(monkeypatch) -> None:
    override = f"/opt/bin/{DEFAULT_CTL_BIN}"
    monkeypatch.setenv(ENV_CTL_BIN, override)
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: name)
    monkeypatch.setattr("e2e.run_tests.subprocess.run", _probe_ok)
    original_path = os.environ.get("PATH", "")
    resolved = ensure_ctl_available()
    assert resolved == override
    assert os.environ[ENV_CTL_BIN_RESOLVED] == override
    assert os.environ.get("PATH", "") == original_path


def test_ensure_ctl_available_normalizes_relative_override(monkeypatch, tmp_path) -> None:
    monkeypatch.chdir(tmp_path)
    monkeypatch.setenv(ENV_CTL_BIN, f"./bin/{DEFAULT_CTL_BIN}")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: f"./bin/{DEFAULT_CTL_BIN}")
    monkeypatch.setattr("e2e.run_tests.subprocess.run", _probe_ok)
    original_path = os.environ.get("PATH", "")
    resolved = ensure_ctl_available()
    expected = str((Path(tmp_path) / "bin" / DEFAULT_CTL_BIN).resolve())
    assert resolved == expected
    assert os.environ[ENV_CTL_BIN_RESOLVED] == expected
    assert os.environ.get("PATH", "") == original_path


def test_ensure_ctl_available_fails_when_missing(monkeypatch) -> None:
    monkeypatch.setenv(ENV_CTL_BIN, f"/missing/{DEFAULT_CTL_BIN}")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: None)
    with pytest.raises(SystemExit):
        ensure_ctl_available()


def test_ensure_ctl_available_fails_when_subcommand_contract_missing(monkeypatch, tmp_path) -> None:
    monkeypatch.delenv(ENV_CTL_BIN, raising=False)
    monkeypatch.setattr("e2e.run_tests.PROJECT_ROOT", tmp_path)
    monkeypatch.setattr(
        "e2e.run_tests.subprocess.run",
        lambda *args, **kwargs: SimpleNamespace(returncode=1, stdout="unknown command: deploy"),
    )
    with pytest.raises(SystemExit):
        ensure_ctl_available()


def test_ensure_ctl_available_fails_when_provision_subcommand_missing(
    monkeypatch, tmp_path
) -> None:
    monkeypatch.delenv(ENV_CTL_BIN, raising=False)
    monkeypatch.setattr("e2e.run_tests.PROJECT_ROOT", tmp_path)

    def fake_probe(args, **kwargs):
        del kwargs
        if list(args)[-2:] == ["provision", "--help"]:
            return SimpleNamespace(returncode=1, stdout="unknown command")
        return SimpleNamespace(returncode=0, stdout="usage")

    monkeypatch.setattr("e2e.run_tests.subprocess.run", fake_probe)
    with pytest.raises(SystemExit):
        ensure_ctl_available()
