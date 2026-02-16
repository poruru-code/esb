# Where: e2e/runner/tests/test_deploy_command.py
# What: Unit tests for deploy command assembly used by E2E runner.
# Why: Keep E2E-to-CLI contract stable when CLI internals are refactored.
from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest

from e2e.runner import deploy as deploy_module
from e2e.runner.deploy import _collect_local_fixture_image_sources, deploy_templates
from e2e.runner.logging import LogSink
from e2e.runner.models import RunContext, Scenario


def _make_context(
    tmp_path: Path,
    *,
    image_prewarm: str = "off",
    image_uri_overrides: dict[str, str] | None = None,
    image_runtime_overrides: dict[str, str] | None = None,
) -> RunContext:
    compose_file = tmp_path / "docker-compose.yml"
    compose_file.write_text("services: {}\n", encoding="utf-8")
    extra = {"image_prewarm": image_prewarm}
    if image_uri_overrides is not None:
        extra["image_uri_overrides"] = image_uri_overrides
    if image_runtime_overrides is not None:
        extra["image_runtime_overrides"] = image_runtime_overrides
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
        extra=extra,
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
    captured_args: list[str] = []
    captured_env_file: str | None = None
    captured_env: dict[str, str] = {}
    captured_cmd: list[str] = []

    def fake_build_esb_cmd(
        args: list[str], env_file: str | None, env: dict[str, str] | None = None
    ) -> list[str]:
        nonlocal captured_args, captured_env_file, captured_env
        captured_args = list(args)
        captured_env_file = env_file
        captured_env = dict(env or {})
        return ["esb", *args]

    def fake_run_and_stream(cmd: list[str], **kwargs: Any) -> int:
        del kwargs
        nonlocal captured_cmd
        captured_cmd = list(cmd)
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

    args = captured_args
    assert args[0] == "deploy"
    assert "--template" in args
    assert str(template) in args
    assert args.index("deploy") < args.index("--template")
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
    assert captured_env_file == ctx.env_file
    assert captured_env["EXAMPLE"] == "1"
    assert captured_cmd[0] == "esb"


def test_deploy_templates_appends_image_overrides(monkeypatch, tmp_path):
    ctx = _make_context(
        tmp_path,
        image_prewarm="off",
        image_uri_overrides={"lambda-image": "public.ecr.aws/example/repo:v1"},
        image_runtime_overrides={"lambda-image": "python"},
    )
    template = tmp_path / "template.yaml"
    template.write_text("Resources: {}\n", encoding="utf-8")
    captured_args: list[str] = []

    def fake_build_esb_cmd(
        args: list[str], env_file: str | None, env: dict[str, str] | None = None
    ) -> list[str]:
        del env_file, env
        nonlocal captured_args
        captured_args = list(args)
        return ["esb", *args]

    monkeypatch.setattr("e2e.runner.deploy.build_esb_cmd", fake_build_esb_cmd)
    monkeypatch.setattr("e2e.runner.deploy.run_and_stream", lambda *args, **kwargs: 0)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
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

    args = captured_args
    assert "--image-uri" in args
    assert "lambda-image=public.ecr.aws/example/repo:v1" in args
    assert "--image-runtime" in args
    assert "lambda-image=python" in args


def test_deploy_templates_rejects_invalid_image_override(tmp_path):
    ctx = _make_context(tmp_path)
    ctx.scenario.extra["image_uri_overrides"] = "invalid-override-format"
    template = tmp_path / "template.yaml"
    template.write_text("Resources: {}\n", encoding="utf-8")

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        with pytest.raises(ValueError, match="image_uri_overrides"):
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


def test_collect_local_fixture_image_sources_filters_non_fixture():
    extra = {
        "image_uri_overrides": {
            "lambda-image": "127.0.0.1:5010/e2e-minimal-lambda:latest",
            "other": "public.ecr.aws/example/repo:v1",
        }
    }
    assert _collect_local_fixture_image_sources(extra) == [
        "127.0.0.1:5010/e2e-minimal-lambda:latest"
    ]


def test_deploy_templates_prepares_local_fixture_image(monkeypatch, tmp_path):
    deploy_module._prepared_local_fixture_images.clear()
    ctx = _make_context(
        tmp_path,
        image_prewarm="off",
        image_uri_overrides={"lambda-image": "127.0.0.1:5010/e2e-minimal-lambda:latest"},
        image_runtime_overrides={"lambda-image": "python"},
    )
    template = tmp_path / "template.yaml"
    template.write_text("Resources: {}\n", encoding="utf-8")

    commands: list[list[str]] = []

    monkeypatch.setattr(
        "e2e.runner.deploy.build_esb_cmd",
        lambda args, env_file, env=None: ["esb", *args],
    )

    def fake_run_and_stream(cmd, **kwargs):
        commands.append(list(cmd))
        return 0

    monkeypatch.setattr("e2e.runner.deploy.run_and_stream", fake_run_and_stream)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
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

    assert commands[0][0:3] == ["docker", "buildx", "build"]
    assert commands[1] == ["docker", "push", "127.0.0.1:5010/e2e-minimal-lambda:latest"]
    assert commands[2][0] == "esb"


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
