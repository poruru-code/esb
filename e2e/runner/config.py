import sys
from pathlib import Path

import yaml

from e2e.runner.utils import BRAND_SLUG, PROJECT_ROOT

MATRIX_ROOT = PROJECT_ROOT / "e2e" / "environments"


_LEGACY_DEPLOY_FIELDS = ("deploy_driver", "artifact_generate")


def _reject_legacy_deploy_fields(entry: dict) -> None:
    for field in _LEGACY_DEPLOY_FIELDS:
        if field in entry:
            raise ValueError(f"legacy field '{field}' is no longer supported in E2E matrix")


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

        _reject_legacy_deploy_fields(entry)
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

            env_scenarios[env_name] = {
                "name": f"Combined Scenarios for {env_name}",
                "env_file": env_file,
                "env_dir": f"e2e/environments/{env_dir}" if env_dir else env_dir,
                "esb_env": env_name,
                "esb_project": BRAND_SLUG,
                "mode": mode,
                "env_vars": env_vars,
                "targets": [],
                "exclude": [],
                "deploy_templates": entry.get("deploy_templates", []) or [],
                "image_prewarm": entry.get("image_prewarm", ""),
                "image_uri_overrides": entry.get("image_uri_overrides", {}) or {},
                "image_runtime_overrides": entry.get("image_runtime_overrides", {}) or {},
            }
            artifact_manifest = str(entry.get("artifact_manifest", "")).strip()
            if artifact_manifest:
                env_scenarios[env_name]["artifact_manifest"] = artifact_manifest

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
