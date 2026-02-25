# Where: e2e/runner/tests/test_run_tests_cli_requirement.py
# What: Unit tests for deciding whether run_tests.py needs ctl.
# Why: E2E deploy phase requires ctl, while test-only runs do not.
from __future__ import annotations

import json
import os
from pathlib import Path
from types import SimpleNamespace

import pytest

from e2e.run_tests import ensure_ctl_available, requires_ctl
from e2e.runner.ctl_contract import DEFAULT_CTL_BIN, ENV_CTL_BIN, ENV_CTL_BIN_RESOLVED


def _args(*, test_only: bool = False) -> SimpleNamespace:
    return SimpleNamespace(test_only=test_only)


def _probe_ok(args, **kwargs):
    del kwargs
    cmd = list(args)
    if cmd[-3:] == ["capabilities", "--output", "json"]:
        payload = {
            "schema_version": 1,
            "contracts": {
                "maven_shim_ensure_schema_version": 1,
                "fixture_image_ensure_schema_version": 1,
            },
        }
        return SimpleNamespace(returncode=0, stdout=json.dumps(payload))
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


def test_ensure_ctl_available_prefers_local_bin(monkeypatch, tmp_path) -> None:
    monkeypatch.delenv(ENV_CTL_BIN, raising=False)
    monkeypatch.setenv("HOME", str(tmp_path))
    local_bin = tmp_path / ".local" / "bin" / DEFAULT_CTL_BIN
    local_bin.parent.mkdir(parents=True)
    local_bin.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
    local_bin.chmod(0o755)
    monkeypatch.setattr(
        "e2e.run_tests.shutil.which", lambda name: f"/usr/local/bin/{DEFAULT_CTL_BIN}"
    )
    monkeypatch.setattr("e2e.run_tests.subprocess.run", _probe_ok)
    resolved = ensure_ctl_available()
    expected = str(local_bin.resolve())
    assert resolved == expected
    assert os.environ[ENV_CTL_BIN_RESOLVED] == expected


def test_ensure_ctl_available_uses_path(monkeypatch, tmp_path) -> None:
    monkeypatch.delenv(ENV_CTL_BIN, raising=False)
    monkeypatch.setenv("HOME", str(tmp_path))
    monkeypatch.setattr(
        "e2e.run_tests.shutil.which", lambda name: f"/usr/local/bin/{DEFAULT_CTL_BIN}"
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
    monkeypatch.delenv(ENV_CTL_BIN, raising=False)
    monkeypatch.setenv("HOME", "/tmp/esb-no-local-ctl")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: None)
    with pytest.raises(SystemExit):
        ensure_ctl_available()


def test_ensure_ctl_available_fails_when_override_missing(monkeypatch) -> None:
    monkeypatch.setenv(ENV_CTL_BIN, f"/missing/{DEFAULT_CTL_BIN}")
    monkeypatch.setattr("e2e.run_tests.shutil.which", lambda name: None)
    with pytest.raises(SystemExit):
        ensure_ctl_available()


def test_ensure_ctl_available_fails_when_subcommand_contract_missing(monkeypatch) -> None:
    monkeypatch.delenv(ENV_CTL_BIN, raising=False)
    monkeypatch.setenv("HOME", "/tmp/esb-no-local-ctl")
    monkeypatch.setattr(
        "e2e.run_tests.shutil.which", lambda name: f"/usr/local/bin/{DEFAULT_CTL_BIN}"
    )
    monkeypatch.setattr(
        "e2e.run_tests.subprocess.run",
        lambda *args, **kwargs: SimpleNamespace(returncode=1, stdout="unknown command: deploy"),
    )
    with pytest.raises(SystemExit):
        ensure_ctl_available()


def test_ensure_ctl_available_fails_when_fixture_ensure_subcommand_missing(
    monkeypatch,
) -> None:
    monkeypatch.delenv(ENV_CTL_BIN, raising=False)
    monkeypatch.setenv("HOME", "/tmp/esb-no-local-ctl")
    monkeypatch.setattr(
        "e2e.run_tests.shutil.which", lambda name: f"/usr/local/bin/{DEFAULT_CTL_BIN}"
    )

    def fake_probe(args, **kwargs):
        del kwargs
        cmd = list(args)
        if cmd[-4:] == ["internal", "fixture-image", "ensure", "--help"]:
            return SimpleNamespace(returncode=1, stdout="unknown command")
        return SimpleNamespace(returncode=0, stdout="usage")

    monkeypatch.setattr("e2e.run_tests.subprocess.run", fake_probe)
    with pytest.raises(SystemExit):
        ensure_ctl_available()


def test_ensure_ctl_available_fails_when_capabilities_contract_mismatch(
    monkeypatch,
) -> None:
    monkeypatch.delenv(ENV_CTL_BIN, raising=False)
    monkeypatch.setenv("HOME", "/tmp/esb-no-local-ctl")
    monkeypatch.setattr(
        "e2e.run_tests.shutil.which", lambda name: f"/usr/local/bin/{DEFAULT_CTL_BIN}"
    )

    def fake_probe(args, **kwargs):
        del kwargs
        if list(args)[-3:] == ["capabilities", "--output", "json"]:
            return SimpleNamespace(
                returncode=0,
                stdout='{"schema_version":1,"contracts":{"maven_shim_ensure_schema_version":1}}',
            )
        return SimpleNamespace(returncode=0, stdout="usage")

    monkeypatch.setattr("e2e.run_tests.subprocess.run", fake_probe)
    with pytest.raises(SystemExit):
        ensure_ctl_available()
