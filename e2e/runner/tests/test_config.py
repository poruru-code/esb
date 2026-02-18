# Where: e2e/runner/tests/test_config.py
# What: Unit tests for matrix-to-scenario normalization.
# Why: Keep E2E matrix extra fields wired into deploy scenario extras.
from __future__ import annotations

from e2e.runner.config import build_env_scenarios
from e2e.runner.planner import build_plan


def test_build_env_scenarios_includes_image_overrides() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "deploy_templates": ["e2e/fixtures/template.e2e.yaml"],
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


def test_build_env_scenarios_defaults_deploy_driver_to_artifact() -> None:
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

    assert scenarios["e2e-docker"]["deploy_driver"] == "artifact"


def test_build_env_scenarios_accepts_artifact_deploy_driver() -> None:
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

    scenarios = build_env_scenarios(matrix, suites)

    assert scenarios["e2e-docker"]["deploy_driver"] == "artifact"


def test_build_env_scenarios_rejects_invalid_deploy_driver() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "deploy_driver": "invalid",
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    try:
        build_env_scenarios(matrix, suites)
    except ValueError as exc:
        assert "deploy_driver" in str(exc)
    else:
        raise AssertionError("expected ValueError for invalid deploy_driver")


def test_build_plan_propagates_deploy_driver() -> None:
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

    scenarios = build_plan(matrix, suites)

    assert scenarios["e2e-docker"].deploy_driver == "artifact"
    assert scenarios["e2e-docker"].artifact_generate == "none"


def test_build_env_scenarios_defaults_artifact_generate_to_none() -> None:
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

    scenarios = build_env_scenarios(matrix, suites)

    assert scenarios["e2e-docker"]["artifact_generate"] == "none"


def test_build_env_scenarios_rejects_non_artifact_deploy_driver() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "deploy_driver": "cli",
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    try:
        build_env_scenarios(matrix, suites)
    except ValueError as exc:
        assert "deploy_driver" in str(exc)
    else:
        raise AssertionError("expected ValueError for non-artifact deploy_driver")


def test_build_env_scenarios_rejects_invalid_artifact_generate() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "deploy_driver": "artifact",
            "artifact_generate": "invalid",
            "suites": ["smoke"],
        }
    ]
    suites = {
        "smoke": {
            "targets": ["../scenarios/smoke/test_smoke.py"],
            "exclude": [],
        }
    }

    try:
        build_env_scenarios(matrix, suites)
    except ValueError as exc:
        assert "artifact_generate" in str(exc)
    else:
        raise AssertionError("expected ValueError for invalid artifact_generate")
