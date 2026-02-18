import sys
from pathlib import Path

import yaml

from e2e.runner.utils import BRAND_SLUG, PROJECT_ROOT

MATRIX_ROOT = PROJECT_ROOT / "e2e" / "environments"


def _normalize_deploy_driver(value: object) -> str:
    driver = str(value or "artifact").strip().lower()
    if driver in {"", "none"}:
        driver = "artifact"
    if driver != "artifact":
        raise ValueError(f"deploy_driver must be 'artifact': {value!r}")
    return driver


def _normalize_artifact_generate(value: object, *, deploy_driver: str) -> str:
    if deploy_driver != "artifact":
        return "none"
    mode = str(value or "none").strip().lower()
    if mode in {"", "none"}:
        return "none"
    raise ValueError(f"artifact_generate must be 'none': {value!r}")


def load_test_matrix() -> dict:
    matrix_file = MATRIX_ROOT / "test_matrix.yaml"
    if not matrix_file.exists():
        print(f"[ERROR] Matrix file not found: {matrix_file}")
        sys.exit(1)

    with open(matrix_file, "r") as f:
        config_matrix = yaml.safe_load(f)

    return config_matrix


def build_env_scenarios(matrix: list, suites: dict, profile_filter: str | None = None) -> dict:
    env_scenarios = {}

    for entry in matrix:
        env_name = entry.get("esb_env")
        if not env_name:
            print(f"[ERROR] Invalid matrix entry format: {entry}")
            continue

        if profile_filter and env_name != profile_filter:
            continue

        suite_names = entry.get("suites", [])
        if env_name not in env_scenarios:
            env_dir = entry.get("env_dir", env_name)
            env_file = entry.get("env_file", "")
            is_firecracker = "firecracker" in env_dir or "firecracker" in env_file
            if "containerd" in env_dir or "containerd" in env_file or is_firecracker:
                mode = "containerd"
            else:
                mode = "docker"

            if env_dir and not env_file:
                env_file = f"e2e/environments/{env_dir}/.env"

            env_vars = dict(entry.get("env_vars", {}))
            if is_firecracker:
                env_vars.setdefault("CONTAINERD_RUNTIME", "aws.firecracker")
            deploy_driver = _normalize_deploy_driver(entry.get("deploy_driver", "artifact"))
            artifact_generate = _normalize_artifact_generate(
                entry.get("artifact_generate", "none"),
                deploy_driver=deploy_driver,
            )

            env_scenarios[env_name] = {
                "name": f"Combined Scenarios for {env_name}",
                "env_file": env_file,
                "env_dir": f"e2e/environments/{env_dir}" if env_dir else env_dir,
                "esb_env": env_name,
                "esb_project": BRAND_SLUG,
                "mode": mode,
                "deploy_driver": deploy_driver,
                "artifact_generate": artifact_generate,
                "env_vars": env_vars,
                "targets": [],
                "exclude": [],
                "deploy_templates": entry.get("deploy_templates", []) or [],
                "image_prewarm": entry.get("image_prewarm", ""),
                "image_uri_overrides": entry.get("image_uri_overrides", {}) or {},
                "image_runtime_overrides": entry.get("image_runtime_overrides", {}) or {},
            }

        for suite_name in suite_names:
            suite_def = suites.get(suite_name)
            if not suite_def:
                print(f"[ERROR] Suite '{suite_name}' not defined in suites.")
                continue
            for target in suite_def.get("targets", []):
                target_path = Path(target)
                if not target_path.is_absolute():
                    target_path = (MATRIX_ROOT / target_path).resolve()
                try:
                    target = str(target_path.relative_to(PROJECT_ROOT))
                except ValueError:
                    target = str(target_path)
                env_scenarios[env_name]["targets"].append(target)
            env_scenarios[env_name]["exclude"].extend(suite_def.get("exclude", []))

    return env_scenarios
