# Where: e2e/runner/tests/test_deploy_command.py
# What: Unit tests for deploy command assembly used by E2E runner.
# Why: Keep E2E-to-CLI contract stable when CLI internals are refactored.
from __future__ import annotations

from pathlib import Path

import pytest

from e2e.runner.deploy import deploy_templates
from e2e.runner.logging import LogSink
from e2e.runner.models import RunContext, Scenario


def _make_context(tmp_path: Path, *, image_prewarm: str = "off") -> RunContext:
    compose_file = tmp_path / "docker-compose.yml"
    compose_file.write_text("services: {}\n", encoding="utf-8")
    scenario = Scenario(
        name="test",
        env_name="e2e-docker",
        mode="docker",
        env_file="e2e/environments/e2e-docker/.env",
        env_dir=None,
        env_vars={},
        targets=[],
        exclude=[],
        deploy_templates=[],
        project_name="esb",
        extra={"image_prewarm": image_prewarm},
    )
    return RunContext(
        scenario=scenario,
        project_name="esb",
        compose_project="esb-e2e-docker",
        compose_file=compose_file,
        env_file=scenario.env_file,
        runtime_env={},
        deploy_env={"EXAMPLE": "1"},
        pytest_env={},
    )


def test_deploy_templates_builds_expected_cli_args(monkeypatch, tmp_path):
    ctx = _make_context(tmp_path, image_prewarm="off")
    template = tmp_path / "template.yaml"
    template.write_text("Resources: {}\n", encoding="utf-8")
    captured: dict[str, object] = {}

    def fake_build_esb_cmd(args, env_file, env=None):
        captured["args"] = list(args)
        captured["env_file"] = env_file
        captured["env"] = dict(env or {})
        return ["esb", *args]

    def fake_run_and_stream(cmd, **kwargs):
        captured["cmd"] = list(cmd)
        captured["cwd"] = kwargs.get("cwd")
        captured["env_for_run"] = dict(kwargs.get("env", {}))
        return 0

    monkeypatch.setattr("e2e.runner.deploy.build_esb_cmd", fake_build_esb_cmd)
    monkeypatch.setattr("e2e.runner.deploy.run_and_stream", fake_run_and_stream)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        deploy_templates(
            ctx,
            [template],
            no_cache=True,
            verbose=True,
            log=log,
            printer=None,
        )
    finally:
        log.close()

    args = captured["args"]
    assert "--template" in args
    assert str(template) in args
    assert "deploy" in args
    assert "--verbose" in args
    assert "--compose-file" in args
    assert str(ctx.compose_file) in args
    assert "--no-deps" in args
    assert "--no-save-defaults" in args
    assert "--env" in args
    assert ctx.scenario.env_name in args
    assert "--mode" in args
    assert ctx.scenario.mode in args
    assert "--image-prewarm" in args
    assert "off" in args
    assert "--no-cache" in args
    assert captured["env_file"] == ctx.env_file
    assert captured["env"]["EXAMPLE"] == "1"
    assert captured["cmd"][0] == "esb"


def test_deploy_templates_raises_on_non_zero_exit(monkeypatch, tmp_path):
    ctx = _make_context(tmp_path, image_prewarm="")
    template = tmp_path / "template.yaml"
    template.write_text("Resources: {}\n", encoding="utf-8")

    monkeypatch.setattr(
        "e2e.runner.deploy.build_esb_cmd",
        lambda args, env_file, env=None: ["esb", *args],
    )
    monkeypatch.setattr("e2e.runner.deploy.run_and_stream", lambda *args, **kwargs: 2)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        with pytest.raises(RuntimeError, match="deploy failed with exit code 2"):
            deploy_templates(
                ctx,
                [template],
                no_cache=False,
                verbose=False,
                log=log,
                printer=None,
            )
    finally:
        log.close()
