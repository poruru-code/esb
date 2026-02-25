# Where: e2e/runner/tests/test_config.py
# What: Unit tests for matrix-to-scenario normalization.
# Why: Keep E2E matrix contract stable for artifact-only deployment.
from __future__ import annotations

import pytest

from e2e.runner.config import build_env_scenarios
from e2e.runner.planner import build_plan


def _smoke_suites() -> dict:
    return {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }


def test_build_env_scenarios_rejects_legacy_image_uri_overrides_field() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
            "image_uri_overrides": {"lambda-image": "public.ecr.aws/example/repo:v1"},
            "suites": ["smoke"],
        }
    ]

    with pytest.raises(ValueError, match="legacy field 'image_uri_overrides'"):
        build_env_scenarios(matrix, _smoke_suites())


def test_build_env_scenarios_defaults_env_file_to_env_dir_dotenv() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
            "suites": ["smoke"],
        }
    ]

    scenarios = build_env_scenarios(matrix, _smoke_suites())
    assert scenarios["e2e-docker"]["env_file"] == "e2e/environments/e2e-docker/.env"


def test_build_env_scenarios_preserves_explicit_env_file() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
            "env_dir": "e2e-docker",
            "env_file": "e2e/environments/e2e-docker/custom.env",
            "suites": ["smoke"],
        }
    ]

    scenarios = build_env_scenarios(matrix, _smoke_suites())
    assert scenarios["e2e-docker"]["env_file"] == "e2e/environments/e2e-docker/custom.env"


def test_build_plan_propagates_core_fields() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "suites": ["smoke"],
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
        }
    ]

    scenarios = build_plan(matrix, _smoke_suites())

    scenario = scenarios["e2e-docker"]
    assert scenario.env_name == "e2e-docker"
    assert scenario.mode == "docker"
    assert scenario.extra == {
        "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
    }


def test_build_plan_rejects_null_artifact_manifest() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "suites": ["smoke"],
            "artifact_manifest": None,
        }
    ]

    with pytest.raises(ValueError, match="artifact_manifest"):
        build_plan(matrix, _smoke_suites())


def test_build_env_scenarios_rejects_legacy_deploy_driver_field() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
            "deploy_driver": "artifact",
            "suites": ["smoke"],
        }
    ]

    with pytest.raises(ValueError, match="legacy field 'deploy_driver'"):
        build_env_scenarios(matrix, _smoke_suites())


def test_build_env_scenarios_rejects_legacy_artifact_generate_field() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
            "artifact_generate": "none",
            "suites": ["smoke"],
        }
    ]

    with pytest.raises(ValueError, match="legacy field 'artifact_generate'"):
        build_env_scenarios(matrix, _smoke_suites())


def test_build_env_scenarios_requires_artifact_manifest() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "suites": ["smoke"],
        }
    ]

    with pytest.raises(ValueError, match="artifact_manifest"):
        build_env_scenarios(matrix, _smoke_suites())


def test_build_env_scenarios_uses_default_env_file() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
            "suites": ["smoke"],
        }
    ]

    scenarios = build_env_scenarios(matrix, _smoke_suites())
    assert scenarios["e2e-docker"]["env_file"] == "e2e/environments/e2e-docker/.env"
