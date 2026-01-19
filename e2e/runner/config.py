import sys

import yaml

from e2e.runner.utils import BRAND_SLUG, PROJECT_ROOT


def load_test_matrix() -> dict:
    matrix_file = PROJECT_ROOT / "e2e" / "test_matrix.yaml"
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
            env_scenarios[env_name] = {
                "name": f"Combined Scenarios for {env_name}",
                "env_file": entry.get("env_file"),
                "esb_env": env_name,
                "esb_project": BRAND_SLUG,
                "env_vars": entry.get("env_vars", {}),
                "targets": [],
                "exclude": [],
            }

        for suite_name in suite_names:
            suite_def = suites.get(suite_name)
            if not suite_def:
                print(f"[ERROR] Suite '{suite_name}' not defined in suites.")
                continue
            env_scenarios[env_name]["targets"].extend(suite_def.get("targets", []))
            env_scenarios[env_name]["exclude"].extend(suite_def.get("exclude", []))

    return env_scenarios
