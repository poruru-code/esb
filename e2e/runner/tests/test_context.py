# Where: e2e/runner/tests/test_context.py
# What: Unit tests for E2E run context assembly.
# Why: Verify environment synthesis without invoking Docker side effects.
from __future__ import annotations

from pathlib import Path

from e2e.runner import constants
from e2e.runner.context import _prepare_context
from e2e.runner.models import Scenario
from e2e.runner.utils import env_key


def test_prepare_context_merges_runtime_env_and_overrides(monkeypatch, tmp_path):
    template = tmp_path / "template.yaml"
    template.write_text("Resources: {}\n", encoding="utf-8")
    compose_file = tmp_path / "docker-compose.docker.yml"
    compose_file.write_text("services: {}\n", encoding="utf-8")
    staging_dir = tmp_path / "staging" / "config"
    monkeypatch.setattr(
        "e2e.runner.context.calculate_runtime_env",
        lambda *_args, **_kwargs: {
            env_key(constants.ENV_TAG): "latest",
            env_key(constants.PORT_GATEWAY_HTTPS): "0",
            constants.ENV_CONTAINER_REGISTRY: "registry:5010",
            constants.ENV_BUILDKITD_CONFIG: "/tmp/buildkitd.toml",
            "BUILDX_BUILDER": "esb-buildx",
        },
    )
    monkeypatch.setattr(
        "e2e.runner.context.calculate_staging_dir", lambda *_args, **_kwargs: staging_dir
    )
    monkeypatch.setattr(
        "e2e.runner.context.resolve_compose_file", lambda *_args, **_kwargs: compose_file
    )
    monkeypatch.setattr(
        "e2e.runner.context._resolve_templates", lambda *_args, **_kwargs: [template]
    )
    monkeypatch.setattr(
        "e2e.runner.context._load_state_env",
        lambda *_args, **_kwargs: {constants.ENV_AUTH_USER: "state-user"},
    )
    monkeypatch.setattr(
        "e2e.runner.context.build_unique_tag", lambda *_args, **_kwargs: "generated-tag"
    )
    monkeypatch.setattr(
        "e2e.runner.context.infra.get_registry_config", lambda: ("127.0.0.1:5010", "registry:5010")
    )
    scenario = Scenario(
        name="test",
        env_name="e2e-docker",
        mode="docker",
        env_file="e2e/environments/e2e-docker/.env",
        env_dir=None,
        env_vars={constants.ENV_AUTH_USER: "scenario-user", "EXTRA_KEY": "EXTRA_VAL"},
        targets=["e2e/scenarios/smoke/test_smoke.py"],
        exclude=[],
        deploy_templates=[str(template)],
        project_name="esb",
    )

    ctx = _prepare_context(
        scenario,
        {
            env_key(constants.PORT_GATEWAY_HTTPS): "18443",
            env_key(constants.PORT_AGENT_GRPC): "15051",
        },
    )

    assert ctx.compose_file == compose_file
    assert ctx.project_name == "esb"
    assert ctx.compose_project == "esb-e2e-docker"
    assert ctx.runtime_env[constants.ENV_AUTH_USER] == "scenario-user"
    assert ctx.runtime_env[env_key(constants.ENV_TAG)] == "generated-tag"
    assert ctx.runtime_env[env_key(constants.PORT_GATEWAY_HTTPS)] == "18443"
    assert ctx.runtime_env[env_key(constants.PORT_AGENT_GRPC)] == "15051"
    assert ctx.runtime_env["HOST_REGISTRY_ADDR"] == "127.0.0.1:5010"
    assert ctx.runtime_env[constants.ENV_CONTAINER_REGISTRY] == "127.0.0.1:5010"
    assert ctx.runtime_env[constants.ENV_CONFIG_DIR] == str(staging_dir)
    assert ctx.runtime_env[env_key("PROJECT")] == "esb"
    assert ctx.runtime_env[env_key("ENV")] == "e2e-docker"
    assert ctx.runtime_env[env_key("INTERACTIVE")] == "0"
    assert ctx.runtime_env[env_key("HOME")].endswith("/e2e/fixtures/.esb/e2e-docker")
    assert Path(ctx.runtime_env[env_key("HOME")]).is_absolute()
    assert ctx.deploy_env["PROJECT_NAME"] == "esb-e2e-docker"
    assert ctx.deploy_env["ESB_META_REUSE"] == "1"
    assert ctx.deploy_env["EXTRA_KEY"] == "EXTRA_VAL"
    assert ctx.pytest_env["EXTRA_KEY"] == "EXTRA_VAL"
    assert staging_dir.exists()


def test_prepare_context_reapplies_proxy_defaults_after_scenario_override(monkeypatch, tmp_path):
    template = tmp_path / "template.yaml"
    template.write_text("Resources: {}\n", encoding="utf-8")
    compose_file = tmp_path / "docker-compose.docker.yml"
    compose_file.write_text("services: {}\n", encoding="utf-8")
    staging_dir = tmp_path / "staging" / "config"

    monkeypatch.setattr(
        "e2e.runner.context.calculate_runtime_env",
        lambda *_args, **_kwargs: {
            env_key(constants.ENV_TAG): "latest",
            env_key(constants.PORT_GATEWAY_HTTPS): "0",
            constants.ENV_CONTAINER_REGISTRY: "registry:5010",
            constants.ENV_BUILDKITD_CONFIG: "/tmp/buildkitd.toml",
            "BUILDX_BUILDER": "esb-buildx",
        },
    )
    monkeypatch.setattr(
        "e2e.runner.context.calculate_staging_dir", lambda *_args, **_kwargs: staging_dir
    )
    monkeypatch.setattr(
        "e2e.runner.context.resolve_compose_file", lambda *_args, **_kwargs: compose_file
    )
    monkeypatch.setattr(
        "e2e.runner.context._resolve_templates", lambda *_args, **_kwargs: [template]
    )
    monkeypatch.setattr("e2e.runner.context._load_state_env", lambda *_args, **_kwargs: {})
    monkeypatch.setattr(
        "e2e.runner.context.build_unique_tag", lambda *_args, **_kwargs: "generated-tag"
    )
    monkeypatch.setattr(
        "e2e.runner.context.infra.get_registry_config", lambda: ("127.0.0.1:5010", "registry:5010")
    )
    scenario = Scenario(
        name="test",
        env_name="e2e-docker",
        mode="docker",
        env_file="e2e/environments/e2e-docker/.env",
        env_dir=None,
        env_vars={"HTTP_PROXY": "http://proxy.example:8080"},
        targets=["e2e/scenarios/smoke/test_smoke.py"],
        exclude=[],
        deploy_templates=[str(template)],
        project_name="esb",
    )

    ctx = _prepare_context(scenario, None)
    no_proxy = set(ctx.runtime_env["NO_PROXY"].split(","))

    assert ctx.runtime_env["HTTP_PROXY"] == "http://proxy.example:8080"
    assert ctx.runtime_env["http_proxy"] == "http://proxy.example:8080"
    assert "localhost" in no_proxy
    assert "127.0.0.1" in no_proxy
    assert "registry" in no_proxy
