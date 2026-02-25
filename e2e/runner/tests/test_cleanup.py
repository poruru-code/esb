# Where: e2e/runner/tests/test_cleanup.py
# What: Unit tests for cleanup helper behavior.
# Why: Prevent shared registry detachment regressions during reset.
from __future__ import annotations

import json

from e2e.runner import cleanup


class _RunResult:
    def __init__(self, returncode: int = 0, stdout: str = "") -> None:
        self.returncode = returncode
        self.stdout = stdout


def test_isolate_external_network_keeps_shared_registry(monkeypatch):
    inspected = [
        {
            "Containers": {
                "1": {"Name": "esb-e2e-containerd-gateway"},
                "2": {"Name": "esb-infra-registry"},
                "3": {"Name": "other-helper"},
            }
        }
    ]
    disconnect_calls: list[list[str]] = []

    def fake_run(cmd, capture_output=True, text=True):
        if cmd[:3] == ["docker", "network", "inspect"]:
            return _RunResult(returncode=0, stdout=json.dumps(inspected))
        if cmd[:3] == ["docker", "network", "disconnect"]:
            disconnect_calls.append(list(cmd))
            return _RunResult(returncode=0, stdout="")
        return _RunResult(returncode=0, stdout="")

    monkeypatch.setattr(cleanup.subprocess, "run", fake_run)
    monkeypatch.setenv("REGISTRY_CONTAINER_NAME", "esb-infra-registry")

    cleanup.isolate_external_network(
        "esb-e2e-containerd",
        log=lambda _line: None,
        printer=None,
    )

    assert disconnect_calls == [
        ["docker", "network", "disconnect", "-f", "esb-e2e-containerd-external", "other-helper"]
    ]


def test_cleanup_managed_images_uses_brand_derived_labels(monkeypatch):
    calls: list[list[str]] = []

    def fake_run(cmd, capture_output=True, text=True, check=False):
        calls.append(list(cmd))
        if cmd[:3] == ["docker", "images", "-q"]:
            return _RunResult(returncode=0, stdout="")
        return _RunResult(returncode=0, stdout="")

    monkeypatch.setattr(cleanup.subprocess, "run", fake_run)

    cleanup.cleanup_managed_images(
        env_name="e2e-x",
        project_name="Acme Prod",
        log=lambda _line: None,
        printer=None,
    )

    assert calls
    image_cmd = calls[0]
    assert image_cmd[:3] == ["docker", "images", "-q"]
    assert "--filter" in image_cmd
    assert "label=com.acme-prod.managed=true" in image_cmd
    assert "label=com.acme-prod.project=Acme Prod-e2e-x" in image_cmd
    assert "label=com.acme-prod.env=e2e-x" in image_cmd
