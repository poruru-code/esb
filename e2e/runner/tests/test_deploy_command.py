# Where: e2e/runner/tests/test_deploy_command.py
# What: Unit tests for artifact deploy orchestration used by E2E runner.
# Why: Keep fixture-prepare + deploy/provision sequencing stable without internal subprocess fallback.
from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest
import yaml

from e2e.runner import deploy as deploy_module
from e2e.runner.ctl_contract import DEFAULT_CTL_BIN, ENV_CTL_BIN_RESOLVED
from e2e.runner.deploy import deploy_artifacts
from e2e.runner.logging import LogSink
from e2e.runner.models import RunContext, Scenario
from e2e.runner.utils import PROJECT_ROOT
from tools.cli.fixture_image import DEFAULT_FIXTURE_IMAGE_ROOT


def _make_context(
    tmp_path: Path,
    *,
    artifact_manifest: str | None = None,
    runtime_env: dict[str, str] | None = None,
) -> RunContext:
    compose_file = tmp_path / "docker-compose.yml"
    compose_file.write_text("services: {}\n", encoding="utf-8")
    extra: dict[str, Any] = {}
    if artifact_manifest is not None:
        extra["artifact_manifest"] = artifact_manifest
    scenario = Scenario(
        name="test",
        env_name="e2e-docker",
        mode="docker",
        env_file="e2e/environments/e2e-docker/.env",
        env_dir=None,
        targets=[],
        exclude=[],
        project_name="esb",
        extra=extra,
    )
    resolved_runtime_env = {
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
                "artifact_root": "fixture",
                "runtime_config_dir": "config",
                "source_template": {
                    "path": "e2e/fixtures/template.e2e.yaml",
                    "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
                },
            }
        ],
    }
    manifest_path = tmp_path / "artifact.yml"
    manifest_path.write_text(yaml.safe_dump(manifest, sort_keys=False), encoding="utf-8")
    return manifest_path


def _fixture_result(images: list[str], *, schema_version: int = 1):
    class _Result:
        def __init__(self) -> None:
            self.schema_version = schema_version
            self.prepared_images = images

    return _Result()


def test_deploy_artifacts_reports_fixture_prepare_failure(monkeypatch, tmp_path):
    deploy_module._prepared_local_fixture_images.clear()
    deploy_module._prepared_maven_shim_images.clear()
    manifest = _write_artifact_fixture(
        tmp_path,
        image_ref="127.0.0.1:5010/esb-lambda-echo:e2e-test",
        base_ref="127.0.0.1:5010/esb-e2e-image-python:latest",
    )
    ctx = _make_context(tmp_path, artifact_manifest=str(manifest))

    monkeypatch.setattr("e2e.runner.deploy.run_and_stream", lambda *args, **kwargs: 0)

    def fake_fixture_prepare(_input):
        raise RuntimeError("prepare failed")

    monkeypatch.setattr("e2e.runner.deploy.execute_fixture_image_ensure", fake_fixture_prepare)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        with pytest.raises(RuntimeError, match="failed to prepare local fixture images"):
            deploy_artifacts(
                ctx,
                no_cache=False,
                log=log,
                printer=None,
            )
    finally:
        log.close()


def test_deploy_artifacts_prepares_local_fixture_image_via_module_call(monkeypatch, tmp_path):
    deploy_module._prepared_local_fixture_images.clear()
    deploy_module._prepared_maven_shim_images.clear()
    manifest = _write_artifact_fixture(
        tmp_path,
        image_ref="127.0.0.1:5010/esb-lambda-echo:e2e-test",
        base_ref="127.0.0.1:5010/esb-e2e-image-python:latest",
    )
    ctx = _make_context(
        tmp_path,
        artifact_manifest=str(manifest),
        runtime_env={
            "http_proxy": "http://proxy.example:8080",
            "HTTPS_PROXY": "http://secure-proxy.example:8443",
        },
    )

    fixture_calls: list[Any] = []

    def fake_fixture_prepare(input_data):
        fixture_calls.append(input_data)
        return _fixture_result(["127.0.0.1:5010/esb-e2e-image-python:latest"])

    monkeypatch.setattr("e2e.runner.deploy.execute_fixture_image_ensure", fake_fixture_prepare)
    monkeypatch.setattr(
        "e2e.runner.deploy._deploy_via_artifact_driver", lambda *args, **kwargs: None
    )
    monkeypatch.chdir(tmp_path)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        deploy_artifacts(
            ctx,
            no_cache=False,
            log=log,
            printer=None,
        )
    finally:
        log.close()

    assert len(fixture_calls) == 1
    fixture_input = fixture_calls[0]
    assert fixture_input.artifact_path == str(manifest.resolve())
    assert fixture_input.no_cache is False
    assert fixture_input.fixture_root == str((PROJECT_ROOT / DEFAULT_FIXTURE_IMAGE_ROOT).resolve())
    assert fixture_input.env["http_proxy"] == "http://proxy.example:8080"


def test_deploy_artifacts_rejects_fixture_prepare_schema_mismatch(monkeypatch, tmp_path):
    deploy_module._prepared_local_fixture_images.clear()
    deploy_module._prepared_maven_shim_images.clear()
    manifest = _write_artifact_fixture(
        tmp_path,
        image_ref="127.0.0.1:5010/esb-lambda-echo:e2e-test",
        base_ref="127.0.0.1:5010/esb-e2e-image-python:latest",
    )
    ctx = _make_context(tmp_path, artifact_manifest=str(manifest))

    monkeypatch.setattr(
        "e2e.runner.deploy.execute_fixture_image_ensure",
        lambda _input: _fixture_result([], schema_version=999),
    )
    monkeypatch.setattr("e2e.runner.deploy.run_and_stream", lambda *args, **kwargs: 0)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        with pytest.raises(RuntimeError, match="invalid fixture image ensure response schema"):
            deploy_artifacts(
                ctx,
                no_cache=False,
                log=log,
                printer=None,
            )
    finally:
        log.close()


def test_deploy_artifacts_rejects_empty_fixture_image_ref(monkeypatch, tmp_path):
    deploy_module._prepared_local_fixture_images.clear()
    deploy_module._prepared_maven_shim_images.clear()
    manifest = _write_artifact_fixture(
        tmp_path,
        image_ref="127.0.0.1:5010/esb-lambda-echo:e2e-test",
        base_ref="127.0.0.1:5010/esb-e2e-image-python:latest",
    )
    ctx = _make_context(tmp_path, artifact_manifest=str(manifest))

    monkeypatch.setattr(
        "e2e.runner.deploy.execute_fixture_image_ensure",
        lambda _input: _fixture_result([""]),
    )
    monkeypatch.setattr("e2e.runner.deploy.run_and_stream", lambda *args, **kwargs: 0)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        with pytest.raises(RuntimeError, match="empty image reference"):
            deploy_artifacts(
                ctx,
                no_cache=False,
                log=log,
                printer=None,
            )
    finally:
        log.close()


def test_deploy_artifacts_runs_deploy_and_provision(monkeypatch, tmp_path):
    deploy_module._prepared_local_fixture_images.clear()
    deploy_module._prepared_maven_shim_images.clear()
    manifest = _write_artifact_fixture(
        tmp_path,
        image_ref="127.0.0.1:5010/esb-lambda-echo:e2e-test",
        base_ref="127.0.0.1:5010/esb-lambda-base:e2e-test",
    )
    ctx = _make_context(tmp_path, artifact_manifest=str(manifest))

    commands: list[list[str]] = []

    monkeypatch.setattr(
        "e2e.runner.deploy.execute_fixture_image_ensure",
        lambda _input: _fixture_result([]),
    )
    monkeypatch.setattr(
        "e2e.runner.deploy.run_and_stream",
        lambda cmd, **kwargs: commands.append(list(cmd)) or 0,
    )

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        deploy_artifacts(
            ctx,
            no_cache=False,
            log=log,
            printer=None,
        )
    finally:
        log.close()

    assert commands[0] == [
        DEFAULT_CTL_BIN,
        "deploy",
        "--artifact",
        str(manifest.resolve()),
    ]
    assert commands[1] == [
        DEFAULT_CTL_BIN,
        "provision",
        "--project",
        ctx.compose_project,
        "--compose-file",
        str(ctx.compose_file),
        "--env-file",
        ctx.env_file,
    ]


def test_deploy_artifacts_deploy_with_no_cache(monkeypatch, tmp_path):
    deploy_module._prepared_local_fixture_images.clear()
    deploy_module._prepared_maven_shim_images.clear()
    manifest = _write_artifact_fixture(
        tmp_path,
        image_ref="127.0.0.1:5010/esb-lambda-echo:e2e-test",
        base_ref="127.0.0.1:5010/esb-lambda-base:e2e-test",
    )
    ctx = _make_context(tmp_path, artifact_manifest=str(manifest))

    commands: list[list[str]] = []
    fixture_calls: list[Any] = []

    def fake_fixture_prepare(input_data):
        fixture_calls.append(input_data)
        return _fixture_result([])

    monkeypatch.setattr("e2e.runner.deploy.execute_fixture_image_ensure", fake_fixture_prepare)
    monkeypatch.setattr(
        "e2e.runner.deploy.run_and_stream",
        lambda cmd, **kwargs: commands.append(list(cmd)) or 0,
    )

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        deploy_artifacts(
            ctx,
            no_cache=True,
            log=log,
            printer=None,
        )
    finally:
        log.close()

    assert fixture_calls[0].no_cache is True
    assert commands[0] == [
        DEFAULT_CTL_BIN,
        "deploy",
        "--artifact",
        str(manifest.resolve()),
        "--no-cache",
    ]


def test_deploy_artifacts_fixture_prepare_is_cached_by_conditions(monkeypatch, tmp_path):
    deploy_module._prepared_local_fixture_images.clear()
    deploy_module._prepared_maven_shim_images.clear()
    manifest = _write_artifact_fixture(
        tmp_path,
        image_ref="127.0.0.1:5010/esb-lambda-echo:e2e-test",
        base_ref="127.0.0.1:5010/esb-lambda-base:e2e-test",
    )
    ctx = _make_context(
        tmp_path,
        artifact_manifest=str(manifest),
        runtime_env={"http_proxy": "http://proxy.example:8080"},
    )

    fixture_calls: list[Any] = []

    def fake_fixture_prepare(input_data):
        fixture_calls.append(input_data)
        return _fixture_result([])

    monkeypatch.setattr("e2e.runner.deploy.execute_fixture_image_ensure", fake_fixture_prepare)
    monkeypatch.setattr("e2e.runner.deploy.run_and_stream", lambda *args, **kwargs: 0)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        deploy_artifacts(
            ctx,
            no_cache=False,
            log=log,
            printer=None,
        )
        deploy_artifacts(
            ctx,
            no_cache=False,
            log=log,
            printer=None,
        )
    finally:
        log.close()

    assert len(fixture_calls) == 1


def test_deploy_artifacts_fixture_prepare_reexecutes_with_no_cache(monkeypatch, tmp_path):
    deploy_module._prepared_local_fixture_images.clear()
    deploy_module._prepared_maven_shim_images.clear()
    manifest = _write_artifact_fixture(
        tmp_path,
        image_ref="127.0.0.1:5010/esb-lambda-echo:e2e-test",
        base_ref="127.0.0.1:5010/esb-lambda-base:e2e-test",
    )
    ctx = _make_context(
        tmp_path,
        artifact_manifest=str(manifest),
        runtime_env={"http_proxy": "http://proxy.example:8080"},
    )

    fixture_calls: list[Any] = []

    def fake_fixture_prepare(input_data):
        fixture_calls.append(input_data)
        return _fixture_result([])

    monkeypatch.setattr("e2e.runner.deploy.execute_fixture_image_ensure", fake_fixture_prepare)
    monkeypatch.setattr("e2e.runner.deploy.run_and_stream", lambda *args, **kwargs: 0)

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        deploy_artifacts(
            ctx,
            no_cache=True,
            log=log,
            printer=None,
        )
        deploy_artifacts(
            ctx,
            no_cache=True,
            log=log,
            printer=None,
        )
    finally:
        log.close()

    assert len(fixture_calls) == 2
    assert fixture_calls[0].no_cache is True
    assert fixture_calls[1].no_cache is True


def test_deploy_artifacts_uses_resolved_ctl_bin(monkeypatch, tmp_path):
    deploy_module._prepared_local_fixture_images.clear()
    deploy_module._prepared_maven_shim_images.clear()
    manifest = _write_artifact_fixture(
        tmp_path,
        image_ref="127.0.0.1:5010/esb-lambda-echo:e2e-test",
        base_ref="127.0.0.1:5010/esb-lambda-base:e2e-test",
    )
    custom_ctl_bin = f"/opt/tools/custom-{DEFAULT_CTL_BIN}"
    ctx = _make_context(
        tmp_path,
        artifact_manifest=str(manifest),
        runtime_env={ENV_CTL_BIN_RESOLVED: custom_ctl_bin},
    )

    commands: list[list[str]] = []

    monkeypatch.setattr(
        "e2e.runner.deploy.execute_fixture_image_ensure",
        lambda _input: _fixture_result([]),
    )
    monkeypatch.setattr(
        "e2e.runner.deploy.run_and_stream",
        lambda cmd, **kwargs: commands.append(list(cmd)) or 0,
    )

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        deploy_artifacts(
            ctx,
            no_cache=False,
            log=log,
            printer=None,
        )
    finally:
        log.close()

    assert commands[0][0] == custom_ctl_bin
    assert commands[1][0] == custom_ctl_bin


def test_deploy_artifacts_requires_manifest(tmp_path):
    ctx = _make_context(
        tmp_path,
        artifact_manifest=str(tmp_path / "missing-artifact.yml"),
    )

    log = LogSink(tmp_path / "deploy.log")
    log.open()
    try:
        with pytest.raises(FileNotFoundError, match="artifact manifest not found"):
            deploy_artifacts(
                ctx,
                no_cache=False,
                log=log,
                printer=None,
            )
    finally:
        log.close()


def test_resolve_artifact_manifest_path_rejects_none(tmp_path):
    ctx = _make_context(tmp_path)
    ctx.scenario.extra["artifact_manifest"] = None

    with pytest.raises(ValueError, match="artifact_manifest is required"):
        deploy_module._resolve_artifact_manifest_path(ctx)


def test_resolve_artifact_manifest_path_rejects_blank(tmp_path):
    ctx = _make_context(tmp_path)
    ctx.scenario.extra["artifact_manifest"] = "   "

    with pytest.raises(ValueError, match="artifact_manifest is required"):
        deploy_module._resolve_artifact_manifest_path(ctx)
