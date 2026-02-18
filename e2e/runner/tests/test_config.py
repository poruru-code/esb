# Where: e2e/runner/tests/test_config.py
# What: Unit tests for matrix-to-scenario normalization.
# Why: Keep E2E matrix extra fields wired into deploy scenario extras.
from __future__ import annotations

import pytest

from e2e.runner.config import build_env_scenarios
from e2e.runner.planner import build_plan


def test_build_env_scenarios_includes_image_overrides() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "image_prewarm": "off",
            "image_uri_overrides": {"lambda-image": "public.ecr.aws/example/repo:v1"},
            "image_runtime_overrides": {"lambda-image": "python"},
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    scenarios = build_env_scenarios(matrix, suites)

    scenario = scenarios["e2e-docker"]
    assert scenario["image_prewarm"] == "off"
    assert scenario["image_uri_overrides"] == {"lambda-image": "public.ecr.aws/example/repo:v1"}
    assert scenario["image_runtime_overrides"] == {"lambda-image": "python"}


def test_build_env_scenarios_defaults_env_file_to_env_dir_dotenv() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    scenarios = build_env_scenarios(matrix, suites)

    assert scenarios["e2e-docker"]["env_file"] == "e2e/environments/e2e-docker/.env"


def test_build_env_scenarios_preserves_explicit_env_file() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "env_dir": "e2e-docker",
            "env_file": "e2e/environments/e2e-docker/custom.env",
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    scenarios = build_env_scenarios(matrix, suites)

    assert scenarios["e2e-docker"]["env_file"] == "e2e/environments/e2e-docker/custom.env"


def test_build_plan_propagates_core_fields() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "suites": ["smoke"],
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    scenarios = build_plan(matrix, suites)

    scenario = scenarios["e2e-docker"]
    assert scenario.env_name == "e2e-docker"
    assert scenario.mode == "docker"
    assert scenario.extra == {
        "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
        "image_prewarm": "",
        "image_uri_overrides": {},
        "image_runtime_overrides": {},
    }


def test_build_plan_treats_null_artifact_manifest_as_unset() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "suites": ["smoke"],
            "artifact_manifest": None,
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    scenarios = build_plan(matrix, suites)
    scenario = scenarios["e2e-docker"]

    assert scenario.extra == {
        "image_prewarm": "",
        "image_uri_overrides": {},
        "image_runtime_overrides": {},
    }


def test_build_env_scenarios_rejects_legacy_deploy_driver_field() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "deploy_driver": "artifact",
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    with pytest.raises(ValueError, match="legacy field 'deploy_driver'"):
        build_env_scenarios(matrix, suites)


def test_build_env_scenarios_rejects_legacy_artifact_generate_field() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "artifact_generate": "none",
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    with pytest.raises(ValueError, match="legacy field 'artifact_generate'"):
        build_env_scenarios(matrix, suites)


def test_build_env_scenarios_rejects_legacy_field_on_duplicate_env_entry() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "suites": ["smoke"],
        },
        {
            "esb_env": "e2e-docker",
            "deploy_driver": "artifact",
            "suites": ["smoke"],
        },
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    with pytest.raises(ValueError, match="legacy field 'deploy_driver'"):
        build_env_scenarios(matrix, suites)
