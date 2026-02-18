# Where: e2e/runner/tests/test_deploy_command.py
# What: Unit tests for artifact-only deploy command assembly used by E2E runner.
# Why: Keep E2E deploy contract stable without requiring esb CLI execution.
from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest
import yaml

from e2e.runner import deploy as deploy_module
from e2e.runner.deploy import _collect_local_fixture_image_sources, deploy_templates
from e2e.runner.logging import LogSink
from e2e.runner.models import RunContext, Scenario


def _make_context(
    tmp_path: Path,
    *,
    image_uri_overrides: dict[str, str] | None = None,
    image_runtime_overrides: dict[str, str] | None = None,
    deploy_driver: str = "artifact",
    artifact_manifest: str | None = None,
    runtime_env: dict[str, str] | None = None,
) -> RunContext:
    compose_file = tmp_path / "docker-compose.yml"
    compose_file.write_text("services: {}\n", encoding="utf-8")
    extra: dict[str, Any] = {}
    if image_uri_overrides is not None:
        extra["image_uri_overrides"] = image_uri_overrides
    if image_runtime_overrides is not None:
        extra["image_runtime_overrides"] = image_runtime_overrides
    if artifact_manifest is not None:
        extra["artifact_manifest"] = artifact_manifest
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
        deploy_driver=deploy_driver,
        artifact_generate="none",
        extra=extra,
    )
    resolved_runtime_env = {
        "CONFIG_DIR": str(tmp_path / "merged-config"),
        "HOST_REGISTRY_ADDR": "127.0.0.1:5010",
        "CONTAINER_REGISTRY": "127.0.0.1:5010",
    }
    if runtime_env:
        resolved_runtime_env.update(runtime_env)

    return RunContext(
        scenario=scenario,
        project_name="esb",
        compose_project="esb-e2e-docker",
        compose_file=compose_file,
        env_file=scenario.env_file,
        runtime_env=resolved_runtime_env,
        deploy_env={"EXAMPLE": "1", **resolved_runtime_env},
        pytest_env={},
    )


def _write_artifact_fixture(
    tmp_path: Path,
    *,
    image_ref: str,
    base_ref: str,
    function_name: str = "lambda-echo",
) -> Path:
    artifact_root = tmp_path / "fixture"
    function_dir = artifact_root / "functions" / function_name
    config_dir = artifact_root / "config"
    function_dir.mkdir(parents=True, exist_ok=True)
    config_dir.mkdir(parents=True, exist_ok=True)

    (function_dir / "Dockerfile").write_text(
        "\n".join(
            [
                f"FROM {base_ref}",
                "COPY functions/lambda-echo/src/ /var/task/",
            ]
        )
        + "\n",
        encoding="utf-8",
    )
    (function_dir / "src").mkdir(parents=True, exist_ok=True)
    (function_dir / "src" / "lambda_function.py").write_text(
        "def lambda_handler(event, context):\n    return {'ok': True}\n",
        encoding="utf-8",
    )

    functions_payload = {
        "functions": {
            function_name: {
                "image": image_ref,
                "timeout": 30,
                "memory_size": 128,
            }
        }
    }
    (config_dir / "functions.yml").write_text(
        yaml.safe_dump(functions_payload, sort_keys=False),
        encoding="utf-8",
    )
    (config_dir / "routing.yml").write_text("routes: []\n", encoding="utf-8")
    (config_dir / "resources.yml").write_text("resources: {}\n", encoding="utf-8")

    manifest = {
        "schema_version": "1",
        "project": "esb-e2e-docker",
        "env": "e2e-docker",
        "mode": "docker",
        "artifacts": [
            {
                "id": "template-e2e-1234abcd",
                "artifact_root": "fixture",
                "runtime_config_dir": "config",
                "source_template": {
                    "path": "e2e/fixtures/template.e2e.yaml",
                    "sha256": "sha",
                },
            }
        ],
    }
    manifest_path = tmp_path / "artifact.yml"
    manifest_path.write_text(yaml.safe_dump(manifest, sort_keys=False), encoding="utf-8")
    return manifest_path


def test_collect_local_fixture_image_sources_filters_non_fixture() -> None:
    extra = {
        "image_uri_overrides": {
            "lambda-image": "127.0.0.1:5010/esb-e2e-lambda-python:latest",
            "other": "public.ecr.aws/example/repo:v1",
        }
    }
    assert _collect_local_fixture_image_sources(extra) == [
        "127.0.0.1:5010/esb-e2e-lambda-python:latest"
    ]


def test_collect_local_fixture_image_sources_includes_java_fixture() -> None:
    extra = {
        "image_uri_overrides": {
            "lambda-image": "127.0.0.1:5010/esb-e2e-lambda-java:latest",
        }
    }
    assert _collect_local_fixture_image_sources(extra) == [
        "127.0.0.1:5010/esb-e2e-lambda-java:latest"
    ]


def test_deploy_templates_rejects_invalid_image_override(monkeypatch, tmp_path):
    ctx = _make_context(tmp_path)
    ctx.scenario.extra["image_uri_overrides"] = "invalid-override-format"

    monkeypatch.setattr(
        "e2e.runner.deploy._deploy_via_artifact_driver", lambda *args, **kwargs: None
    )

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        with pytest.raises(ValueError, match="image_uri_overrides"):
            deploy_templates(
                ctx,
                [],
                no_cache=False,
                verbose=False,
                log=log,
                printer=None,
            )
    finally:
        log.close()


def test_deploy_templates_prepares_local_fixture_image(monkeypatch, tmp_path):
    deploy_module._prepared_local_fixture_images.clear()
    ctx = _make_context(
        tmp_path,
        image_uri_overrides={"lambda-image": "127.0.0.1:5010/esb-e2e-lambda-python:latest"},
        image_runtime_overrides={"lambda-image": "python"},
    )

    commands: list[list[str]] = []

    monkeypatch.setattr(
        "e2e.runner.deploy._deploy_via_artifact_driver", lambda *args, **kwargs: None
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
            [],
            no_cache=False,
            verbose=False,
            log=log,
            printer=None,
        )
    finally:
        log.close()

    assert commands[0][0:3] == ["docker", "buildx", "build"]
    assert commands[1] == ["docker", "push", "127.0.0.1:5010/esb-e2e-lambda-python:latest"]


def test_deploy_templates_artifact_driver_runs_prepare_apply_and_provision(monkeypatch, tmp_path):
    config_dir = tmp_path / "merged-config"
    image_ref = "127.0.0.1:5010/esb-lambda-echo:e2e-test"
    base_ref = "127.0.0.1:5010/esb-lambda-base:e2e-test"
    manifest = _write_artifact_fixture(tmp_path, image_ref=image_ref, base_ref=base_ref)
    ctx = _make_context(
        tmp_path,
        artifact_manifest=str(manifest),
        runtime_env={
            "CONFIG_DIR": str(config_dir),
            "HOST_REGISTRY_ADDR": "127.0.0.1:5010",
            "CONTAINER_REGISTRY": "127.0.0.1:5010",
        },
    )

    commands: list[list[str]] = []

    def fake_run_and_stream(cmd: list[str], **kwargs: Any) -> int:
        del kwargs
        commands.append(list(cmd))
        return 0

    monkeypatch.setattr("e2e.runner.deploy.run_and_stream", fake_run_and_stream)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        deploy_templates(
            ctx,
            [],
            no_cache=False,
            verbose=False,
            log=log,
            printer=None,
        )
    finally:
        log.close()

    assert commands[0] == [
        "artifactctl",
        "prepare-images",
        "--artifact",
        str(manifest.resolve()),
    ]
    assert commands[1] == [
        "artifactctl",
        "apply",
        "--artifact",
        str(manifest.resolve()),
        "--out",
        str(config_dir),
    ]
    assert commands[2][0:8] == [
        "docker",
        "compose",
        "--project-name",
        ctx.compose_project,
        "--file",
        str(ctx.compose_file),
        "--env-file",
        ctx.env_file,
    ]
    assert commands[2][-4:] == ["run", "--rm", "--no-deps", "provisioner"]


def test_deploy_templates_artifact_driver_prepare_images_with_no_cache(monkeypatch, tmp_path):
    config_dir = tmp_path / "merged-config"
    image_ref = "127.0.0.1:5010/esb-lambda-echo:e2e-test"
    base_ref = "127.0.0.1:5010/esb-lambda-base:e2e-test"
    manifest = _write_artifact_fixture(tmp_path, image_ref=image_ref, base_ref=base_ref)
    ctx = _make_context(
        tmp_path,
        artifact_manifest=str(manifest),
        runtime_env={
            "CONFIG_DIR": str(config_dir),
            "HOST_REGISTRY_ADDR": "127.0.0.1:5010",
            "CONTAINER_REGISTRY": "127.0.0.1:5010",
        },
    )

    def fake_run_and_stream(cmd: list[str], **kwargs: Any) -> int:
        del kwargs
        commands.append(list(cmd))
        return 0

    commands: list[list[str]] = []
    monkeypatch.setattr("e2e.runner.deploy.run_and_stream", fake_run_and_stream)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        deploy_templates(
            ctx,
            [],
            no_cache=True,
            verbose=False,
            log=log,
            printer=None,
        )
    finally:
        log.close()

    assert commands[0] == [
        "artifactctl",
        "prepare-images",
        "--artifact",
        str(manifest.resolve()),
        "--no-cache",
    ]


def test_deploy_templates_artifact_driver_requires_manifest(tmp_path):
    ctx = _make_context(
        tmp_path,
        artifact_manifest=str(tmp_path / "missing-artifact.yml"),
    )

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        with pytest.raises(FileNotFoundError, match="artifact manifest not found"):
            deploy_templates(
                ctx,
                [],
                no_cache=False,
                verbose=False,
                log=log,
                printer=None,
            )
    finally:
        log.close()


def test_deploy_templates_rejects_unknown_driver(tmp_path):
    ctx = _make_context(tmp_path, deploy_driver="unknown")

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        with pytest.raises(RuntimeError, match="deploy_driver 'unknown'"):
            deploy_templates(
                ctx,
                [],
                no_cache=False,
                verbose=False,
                log=log,
                printer=None,
            )
    finally:
        log.close()
