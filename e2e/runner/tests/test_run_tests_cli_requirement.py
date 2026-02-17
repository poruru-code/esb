# Where: e2e/runner/tests/test_run_tests_cli_requirement.py
# What: Unit tests for deciding whether run_tests.py needs a local esb binary.
# Why: artifact driver may still need CLI when producer step uses artifact generate.
from __future__ import annotations

from types import SimpleNamespace

import pytest

from e2e.run_tests import requires_local_artifactctl, requires_local_esb_cli


def _args(*, unit_only: bool = False, test_only: bool = False) -> SimpleNamespace:
    return SimpleNamespace(unit_only=unit_only, test_only=test_only)


def _scenario(*, deploy_driver: str = "cli", artifact_generate: str = "none") -> SimpleNamespace:
    return SimpleNamespace(deploy_driver=deploy_driver, artifact_generate=artifact_generate)


def test_requires_local_esb_cli_false_for_unit_only() -> None:
    scenarios = {"docker": _scenario(deploy_driver="cli")}
    assert requires_local_esb_cli(_args(unit_only=True), scenarios) is False


def test_requires_local_esb_cli_false_for_test_only() -> None:
    scenarios = {"docker": _scenario(deploy_driver="cli")}
    assert requires_local_esb_cli(_args(test_only=True), scenarios) is False


def test_requires_local_esb_cli_true_when_cli_driver_exists() -> None:
    scenarios = {
        "docker": _scenario(deploy_driver="artifact", artifact_generate="none"),
        "containerd": _scenario(deploy_driver="cli"),
    }
    assert requires_local_esb_cli(_args(), scenarios) is True


def test_requires_local_esb_cli_true_when_artifact_generate_cli() -> None:
    scenarios = {
        "docker": _scenario(deploy_driver="artifact", artifact_generate="cli"),
        "containerd": _scenario(deploy_driver="artifact", artifact_generate="none"),
    }
    assert requires_local_esb_cli(_args(), scenarios) is True


def test_requires_local_esb_cli_false_when_all_artifact_without_generate() -> None:
    scenarios = {
        "docker": _scenario(deploy_driver="artifact", artifact_generate="none"),
        "containerd": _scenario(deploy_driver="artifact", artifact_generate="none"),
    }
    assert requires_local_esb_cli(_args(), scenarios) is False


def test_requires_local_esb_cli_rejects_unknown_driver() -> None:
    scenarios = {"docker": _scenario(deploy_driver="invalid")}
    with pytest.raises(ValueError, match="unsupported deploy_driver"):
        requires_local_esb_cli(_args(), scenarios)


def test_requires_local_esb_cli_rejects_unknown_artifact_generate() -> None:
    scenarios = {"docker": _scenario(deploy_driver="artifact", artifact_generate="invalid")}
    with pytest.raises(ValueError, match="unsupported artifact_generate"):
        requires_local_esb_cli(_args(), scenarios)


def test_requires_local_artifactctl_false_for_unit_only() -> None:
    scenarios = {"docker": _scenario(deploy_driver="artifact")}
    assert requires_local_artifactctl(_args(unit_only=True), scenarios) is False


def test_requires_local_artifactctl_false_for_test_only() -> None:
    scenarios = {"docker": _scenario(deploy_driver="artifact")}
    assert requires_local_artifactctl(_args(test_only=True), scenarios) is False


def test_requires_local_artifactctl_true_when_artifact_driver_exists() -> None:
    scenarios = {
        "docker": _scenario(deploy_driver="artifact"),
        "containerd": _scenario(deploy_driver="cli"),
    }
    assert requires_local_artifactctl(_args(), scenarios) is True


def test_requires_local_artifactctl_false_when_no_artifact_driver() -> None:
    scenarios = {
        "docker": _scenario(deploy_driver="cli"),
        "containerd": _scenario(deploy_driver="cli"),
    }
    assert requires_local_artifactctl(_args(), scenarios) is False


def test_requires_local_artifactctl_rejects_unknown_driver() -> None:
    scenarios = {"docker": _scenario(deploy_driver="invalid")}
    with pytest.raises(ValueError, match="unsupported deploy_driver"):
        requires_local_artifactctl(_args(), scenarios)
