from __future__ import annotations

import json
import subprocess
from pathlib import Path

import pytest

from tools.cli import stack_ops


def test_read_env_file_parses_export_and_quotes(tmp_path: Path) -> None:
    env_file = tmp_path / ".env"
    env_file.write_text(
        "\n".join(
            [
                "# comment",
                "export FOO=bar",
                'BAR="baz"',
                "BAZ='qux'",
                "SPACED = value ",
                "INVALID_LINE",
            ]
        ),
        encoding="utf-8",
    )

    env = stack_ops.read_env_file(str(env_file))

    assert env["FOO"] == "bar"
    assert env["BAR"] == "baz"
    assert env["BAZ"] == "qux"
    assert env["SPACED"] == "value"
    assert "INVALID_LINE" not in env


def test_normalize_compose_project_name() -> None:
    assert stack_ops.normalize_compose_project_name("_My Project__DEV_") == "my-project__dev"


def test_resolve_artifact_path_auto_detect_single(tmp_path: Path) -> None:
    artifact = tmp_path / "artifacts" / "a" / "artifact.yml"
    artifact.parent.mkdir(parents=True, exist_ok=True)
    artifact.write_text("project: esb\n", encoding="utf-8")

    resolved = stack_ops.resolve_artifact_path(str(tmp_path), "")

    assert resolved == str(artifact.resolve())


def test_resolve_artifact_path_auto_detect_multiple_interactive_default(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    artifact_a = tmp_path / "artifacts" / "a" / "artifact.yml"
    artifact_b = tmp_path / "artifacts" / "b" / "artifact.yml"
    artifact_a.parent.mkdir(parents=True, exist_ok=True)
    artifact_b.parent.mkdir(parents=True, exist_ok=True)
    artifact_a.write_text("project: esb\n", encoding="utf-8")
    artifact_b.write_text("project: esb\n", encoding="utf-8")

    monkeypatch.setattr("builtins.input", lambda _prompt: "")

    resolved = stack_ops.resolve_artifact_path(str(tmp_path), "")

    assert resolved == str(artifact_a.resolve())


def test_wait_for_registry_ready_timeout_collects_diagnostics(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[list[str]] = []

    def fake_probe(_host_port: str) -> int | None:
        return None

    def fake_run_command(cmd: list[str], **_kwargs):
        calls.append(cmd)
        return subprocess.CompletedProcess(cmd, 0, "", "")

    monkeypatch.setattr(stack_ops, "probe_registry", fake_probe)
    monkeypatch.setattr(stack_ops, "run_command", fake_run_command)

    with pytest.raises(RuntimeError, match="Registry not responding"):
        stack_ops.wait_for_registry_ready(
            host_port="127.0.0.1:5010",
            timeout_seconds=0,
            compose_base=["compose", "-p", "esb-dev", "-f", "docker-compose.yml"],
            env={},
        )

    assert calls[0] == [
        "docker",
        "compose",
        "-p",
        "esb-dev",
        "-f",
        "docker-compose.yml",
        "ps",
        "registry",
    ]
    assert calls[1] == [
        "docker",
        "compose",
        "-p",
        "esb-dev",
        "-f",
        "docker-compose.yml",
        "logs",
        "--tail=50",
        "registry",
    ]


def test_probe_registry_bypasses_proxy_env(monkeypatch: pytest.MonkeyPatch) -> None:
    captured: dict[str, object] = {}
    proxy_handler_sentinel = object()

    class FakeResponse:
        status = 200

        def __enter__(self):
            return self

        def __exit__(self, exc_type, exc, tb):
            return False

    class FakeOpener:
        def open(self, request, timeout=0):
            captured["url"] = request.full_url
            captured["timeout"] = timeout
            return FakeResponse()

    def fake_proxy_handler(proxies=None):
        captured["proxies"] = proxies
        return proxy_handler_sentinel

    def fake_build_opener(handler):
        captured["handler"] = handler
        return FakeOpener()

    monkeypatch.setattr(stack_ops.urlrequest, "ProxyHandler", fake_proxy_handler)
    monkeypatch.setattr(stack_ops.urlrequest, "build_opener", fake_build_opener)

    assert stack_ops.probe_registry("127.0.0.1:5010") == 200
    assert captured["proxies"] == {}
    assert captured["handler"] is proxy_handler_sentinel
    assert captured["url"] == "http://127.0.0.1:5010/v2/"
    assert captured["timeout"] == 2


def test_execute_stack_deploy_runs_expected_commands(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repo_root = tmp_path
    compose_file = repo_root / "docker-compose.yml"
    compose_file.write_text("services: {}\n", encoding="utf-8")

    env_file = repo_root / ".env"
    env_file.write_text(
        "\n".join(
            [
                "JWT_SECRET_KEY=12345678901234567890123456789012",
                "PORT_REGISTRY=5010",
                "REGISTRY_WAIT_TIMEOUT=5",
            ]
        ),
        encoding="utf-8",
    )

    artifact_path = repo_root / "artifacts" / "demo" / "artifact.yml"
    artifact_path.parent.mkdir(parents=True, exist_ok=True)
    artifact_path.write_text("project: Demo\nenv: DEV\n", encoding="utf-8")

    run_calls: list[list[str]] = []
    wait_calls: list[tuple[str, int]] = []

    def fake_run_command(cmd: list[str], **_kwargs):
        run_calls.append(cmd)
        if cmd[:2] == ["docker", "inspect"]:
            payload = [
                {
                    "Config": {
                        "Labels": {"com.docker.compose.project": "demo-dev"},
                    }
                }
            ]
            return subprocess.CompletedProcess(cmd, 0, json.dumps(payload), "")
        return subprocess.CompletedProcess(cmd, 0, "", "")

    def fake_wait_for_registry_ready(*, host_port: str, timeout_seconds: int, **_kwargs) -> None:
        wait_calls.append((host_port, timeout_seconds))

    monkeypatch.setattr(stack_ops, "resolve_repo_root", lambda: str(repo_root))
    monkeypatch.setattr(stack_ops.shutil, "which", lambda _cmd, path=None: "/usr/bin/esb-ctl")
    monkeypatch.setattr(stack_ops, "run_command", fake_run_command)
    monkeypatch.setattr(stack_ops, "wait_for_registry_ready", fake_wait_for_registry_ready)

    stack_ops.execute_stack_deploy(stack_ops.StackDeployInput(artifact_path=str(artifact_path)))

    assert run_calls[0] == [
        "docker",
        "inspect",
        "esb-infra-registry",
    ]
    assert run_calls[1] == [
        "docker",
        "compose",
        "-p",
        "demo-dev",
        "--env-file",
        str(env_file),
        "-f",
        str(compose_file),
        "up",
        "-d",
    ]
    assert run_calls[2] == ["esb-ctl", "deploy", "--artifact", str(artifact_path.resolve())]
    assert run_calls[3] == [
        "esb-ctl",
        "provision",
        "--project",
        "demo-dev",
        "--compose-file",
        str(compose_file),
        "--env-file",
        str(env_file),
        "--project-dir",
        str(repo_root),
    ]
    assert run_calls[4] == [
        "docker",
        "compose",
        "-p",
        "demo-dev",
        "--env-file",
        str(env_file),
        "-f",
        str(compose_file),
        "ps",
    ]
    assert run_calls[5] == [
        "docker",
        "run",
        "--rm",
        "-v",
        "demo-dev_esb-runtime-config:/runtime-config",
        "alpine",
        "ls",
        "-1",
        "/runtime-config",
    ]
    assert wait_calls == [("127.0.0.1:5010", 5)]


def test_execute_stack_deploy_fails_when_registry_owned_by_other_project(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repo_root = tmp_path
    compose_file = repo_root / "docker-compose.yml"
    compose_file.write_text("services: {}\n", encoding="utf-8")

    env_file = repo_root / ".env"
    env_file.write_text(
        "\n".join(
            [
                "JWT_SECRET_KEY=12345678901234567890123456789012",
            ]
        ),
        encoding="utf-8",
    )

    artifact_path = repo_root / "artifacts" / "demo" / "artifact.yml"
    artifact_path.parent.mkdir(parents=True, exist_ok=True)
    artifact_path.write_text("project: Demo\nenv: DEV\n", encoding="utf-8")

    run_calls: list[list[str]] = []

    def fake_run_command(cmd: list[str], **_kwargs):
        run_calls.append(cmd)
        if cmd[:2] == ["docker", "inspect"]:
            payload = [
                {
                    "Config": {
                        "Labels": {"com.docker.compose.project": "padma-dev"},
                    }
                }
            ]
            return subprocess.CompletedProcess(cmd, 0, json.dumps(payload), "")
        return subprocess.CompletedProcess(cmd, 0, "", "")

    monkeypatch.setattr(stack_ops, "resolve_repo_root", lambda: str(repo_root))
    monkeypatch.setattr(stack_ops.shutil, "which", lambda _cmd, path=None: "/usr/bin/esb-ctl")
    monkeypatch.setattr(stack_ops, "run_command", fake_run_command)

    with pytest.raises(RuntimeError, match="shared registry container"):
        stack_ops.execute_stack_deploy(stack_ops.StackDeployInput(artifact_path=str(artifact_path)))

    assert run_calls == [["docker", "inspect", "esb-infra-registry"]]
