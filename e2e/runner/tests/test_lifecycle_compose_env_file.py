# Where: e2e/runner/tests/test_lifecycle_compose_env_file.py
# What: Unit tests for docker compose env-file wiring in lifecycle commands.
# Why: Ensure E2E compose commands use scenario env files when configured.
from __future__ import annotations

from pathlib import Path

from e2e.runner import lifecycle
from e2e.runner.logging import LogSink
from e2e.runner.models import RunContext, Scenario


def _make_context(tmp_path: Path, *, env_file: str | None) -> RunContext:
    compose_file = tmp_path / "docker-compose.yml"
    compose_file.write_text("services: {}\n", encoding="utf-8")
    scenario = Scenario(
        name="test",
        env_name="e2e-docker",
        mode="docker",
        env_file=env_file,
        env_dir=None,
        targets=[],
        exclude=[],
        project_name="esb",
        extra={},
    )
    return RunContext(
        scenario=scenario,
        project_name="esb",
        compose_project="esb-e2e-docker",
        compose_file=compose_file,
        env_file=env_file,
        runtime_env={},
        deploy_env={},
        pytest_env={},
    )


def test_compose_up_includes_env_file_when_configured(monkeypatch, tmp_path):
    env_file = str((tmp_path / "profile.env").resolve())
    ctx = _make_context(tmp_path, env_file=env_file)

    captured: list[list[str]] = []
    monkeypatch.setattr(
        lifecycle,
        "run_and_stream",
        lambda cmd, **kwargs: captured.append(list(cmd)) or 0,
    )
    monkeypatch.setattr(
        lifecycle.infra,
        "connect_registry_to_network",
        lambda *_args, **_kwargs: None,
    )
    monkeypatch.setattr(lifecycle, "discover_ports", lambda *_args, **_kwargs: {})

    log = LogSink(tmp_path / "compose.log")
    log.open()
    try:
        lifecycle.compose_up(ctx, build=False, log=log, printer=None)
    finally:
        log.close()

    assert captured
    cmd = captured[0]
    assert "--env-file" in cmd
    assert env_file in cmd


def test_compose_down_omits_env_file_when_unset(monkeypatch, tmp_path):
    ctx = _make_context(tmp_path, env_file=None)

    captured: list[list[str]] = []
    monkeypatch.setattr(
        lifecycle,
        "run_and_stream",
        lambda cmd, **kwargs: captured.append(list(cmd)) or 0,
    )

    log = LogSink(tmp_path / "compose.log")
    log.open()
    try:
        lifecycle.compose_down(ctx, log=log, printer=None)
    finally:
        log.close()

    assert captured
    cmd = captured[0]
    assert "--env-file" not in cmd
