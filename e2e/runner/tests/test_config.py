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
            "env": "e2e-docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
            "image_uri_overrides": {"lambda-image": "public.ecr.aws/example/repo:v1"},
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
    assert scenario["image_uri_overrides"] == {"lambda-image": "public.ecr.aws/example/repo:v1"}
    assert scenario["artifact_manifest"] == "e2e/artifacts/e2e-docker/artifact.yml"


def test_build_env_scenarios_defaults_env_file_to_env_dir_dotenv() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
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
            "env": "e2e-docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
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
            "env": "e2e-docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
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
        "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
        "image_uri_overrides": {},
    }


def test_build_plan_rejects_null_artifact_manifest() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
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

    with pytest.raises(ValueError, match="artifact_manifest"):
        build_plan(matrix, suites)


def test_build_env_scenarios_rejects_legacy_deploy_driver_field() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
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
            "env": "e2e-docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
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
            "env": "e2e-docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
            "suites": ["smoke"],
        },
        {
            "env": "e2e-docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
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


def test_build_env_scenarios_requires_config_dir() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    with pytest.raises(ValueError, match="config_dir"):
        build_env_scenarios(matrix, suites)


def test_build_env_scenarios_requires_artifact_manifest() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    with pytest.raises(ValueError, match="artifact_manifest"):
        build_env_scenarios(matrix, suites)


def test_build_env_scenarios_rejects_mismatched_config_dir() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "config_dir": ".esb/staging/wrong/e2e-docker/config",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    with pytest.raises(ValueError, match="config_dir mismatch"):
        build_env_scenarios(matrix, suites)


def test_build_env_scenarios_rejects_absolute_config_dir() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "config_dir": "/tmp/config",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    with pytest.raises(ValueError, match="repository-relative"):
        build_env_scenarios(matrix, suites)


def test_build_env_scenarios_accepts_matching_custom_project() -> None:
    matrix = [
        {
            "env": "custom",
            "esb_project": "alt",
            "config_dir": ".esb/staging/alt-custom/custom/config",
            "artifact_manifest": "e2e/artifacts/custom/artifact.yml",
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
    assert scenarios["custom"]["esb_project"] == "alt"


def test_build_env_scenarios_accepts_lowercase_config_dir_for_mixed_case_env() -> None:
    matrix = [
        {
            "env": "E2E-Docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
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
    assert scenarios["E2E-Docker"]["config_dir"] == ".esb/staging/esb-e2e-docker/e2e-docker/config"


def test_build_env_scenarios_uses_default_env_file() -> None:
    matrix = [
        {
            "env": "e2e-docker",
            "config_dir": ".esb/staging/esb-e2e-docker/e2e-docker/config",
            "artifact_manifest": "e2e/artifacts/e2e-docker/artifact.yml",
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
