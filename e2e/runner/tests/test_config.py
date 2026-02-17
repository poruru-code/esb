# Where: e2e/runner/tests/test_config.py
# What: Unit tests for matrix-to-scenario normalization.
# Why: Keep E2E matrix extra fields wired into deploy scenario extras.
from __future__ import annotations

from e2e.runner.config import build_env_scenarios


def test_build_env_scenarios_includes_image_overrides() -> None:
    matrix = [
        {
            "esb_env": "e2e-docker",
            "deploy_templates": ["e2e/fixtures/template.image.yaml"],
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
