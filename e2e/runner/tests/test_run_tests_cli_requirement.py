# Where: e2e/runner/tests/test_run_tests_cli_requirement.py
# What: Unit tests for deciding whether run_tests.py needs local helper binaries.
# Why: E2E execution path is artifact-only and requires artifactctl for deploy phases.
from __future__ import annotations

from types import SimpleNamespace

from e2e.run_tests import requires_local_artifactctl


def _args(*, unit_only: bool = False, test_only: bool = False) -> SimpleNamespace:
    return SimpleNamespace(unit_only=unit_only, test_only=test_only)


def test_requires_local_artifactctl_false_for_unit_only() -> None:
    scenarios = {"docker": object()}
    assert requires_local_artifactctl(_args(unit_only=True), scenarios) is False


def test_requires_local_artifactctl_false_for_test_only() -> None:
    scenarios = {"docker": object()}
    assert requires_local_artifactctl(_args(test_only=True), scenarios) is False


def test_requires_local_artifactctl_true_when_scenarios_exist() -> None:
    scenarios = {
        "docker": object(),
        "containerd": object(),
    }
    assert requires_local_artifactctl(_args(), scenarios) is True


def test_requires_local_artifactctl_false_without_scenarios() -> None:
    assert requires_local_artifactctl(_args(), {}) is False
