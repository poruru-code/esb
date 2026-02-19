import sys
from pathlib import Path

import yaml

from e2e.runner.utils import BRAND_HOME_DIR, BRAND_SLUG, PROJECT_ROOT

MATRIX_ROOT = PROJECT_ROOT / "e2e" / "environments"
CONTRACT_FILE = PROJECT_ROOT / "e2e" / "contracts" / "runtime_env_contract.yaml"


_LEGACY_DEPLOY_FIELDS = ("deploy_driver", "artifact_generate")
_RUNTIME_NETWORK_KEYS = (
    "SUBNET_EXTERNAL",
    "RUNTIME_NET_SUBNET",
    "RUNTIME_NODE_IP",
    "LAMBDA_NETWORK",
)


def _load_runtime_network_contract() -> dict[str, dict[str, str]]:
    if not CONTRACT_FILE.exists():
        return {}
    with open(CONTRACT_FILE, "r", encoding="utf-8") as f:
        data = yaml.safe_load(f) or {}
    cases = data.get("cases", [])
    if not isinstance(cases, list):
        raise ValueError("runtime env contract 'cases' must be a list")

    field_map = {
        "subnet_external": "SUBNET_EXTERNAL",
        "runtime_net_subnet": "RUNTIME_NET_SUBNET",
        "runtime_node_ip": "RUNTIME_NODE_IP",
        "lambda_network": "LAMBDA_NETWORK",
    }
    contract: dict[str, dict[str, str]] = {}
    for raw_case in cases:
        if not isinstance(raw_case, dict):
            raise ValueError("runtime env contract case must be a map")
        env_name = str(raw_case.get("env", "")).strip()
        if not env_name:
            raise ValueError("runtime env contract case requires non-empty 'env'")
        values: dict[str, str] = {}
        for source_key, target_key in field_map.items():
            raw_value = raw_case.get(source_key)
            if raw_value is None:
                continue
            value = str(raw_value).strip()
            if value:
                values[target_key] = value
        if env_name in contract:
            raise ValueError(f"duplicate runtime env contract case: {env_name}")
        contract[env_name] = values
    return contract


def _inject_runtime_network_defaults(
    env_name: str,
    env_vars: dict[str, str],
    contract: dict[str, dict[str, str]],
) -> None:
    defaults = contract.get(env_name)
    if defaults is None:
        return
    conflicts = [key for key in _RUNTIME_NETWORK_KEYS if key in env_vars]
    if conflicts:
        joined = ", ".join(sorted(conflicts))
        raise ValueError(
            f"matrix env_vars for '{env_name}' must not set runtime network keys ({joined}); "
            f"use {CONTRACT_FILE.relative_to(PROJECT_ROOT)}"
        )
    for key, value in defaults.items():
        env_vars.setdefault(key, value)


def _reject_legacy_deploy_fields(entry: dict) -> None:
    for field in _LEGACY_DEPLOY_FIELDS:
        if field in entry:
            raise ValueError(f"legacy field '{field}' is no longer supported in E2E matrix")


def _require_non_empty_field(entry: dict, field: str) -> str:
    value = entry.get(field)
    if value is None:
        raise ValueError(f"matrix field '{field}' is required")
    normalized = str(value).strip()
    if normalized == "":
        raise ValueError(f"matrix field '{field}' must be non-empty")
    return normalized


def _resolve_esb_project(entry: dict) -> str:
    value = entry.get("esb_project")
    if value is None:
        return BRAND_SLUG
    normalized = str(value).strip()
    if normalized == "":
        raise ValueError("matrix field 'esb_project' must be non-empty when provided")
    return normalized


def _normalize_relative_path(path: str) -> str:
    normalized = path.replace("\\", "/").strip()
    while normalized.startswith("./"):
        normalized = normalized[2:]
    return str(Path(normalized).as_posix())


def _validate_config_dir(entry: dict, env_name: str, esb_project: str) -> str:
    raw = _require_non_empty_field(entry, "config_dir")
    normalized = _normalize_relative_path(raw)
    path_obj = Path(normalized)
    if path_obj.is_absolute():
        raise ValueError("matrix field 'config_dir' must be a repository-relative path")
    if ".." in path_obj.parts:
        raise ValueError("matrix field 'config_dir' must not contain '..'")
    env_key = str(env_name).strip().lower()
    expected = _normalize_relative_path(
        f"{BRAND_HOME_DIR}/staging/{esb_project}-{env_key}/{env_key}/config"
    )
    if normalized != expected:
        raise ValueError(
            f"config_dir mismatch for esb_env '{env_name}': {normalized} != {expected}"
        )
    return normalized


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
    runtime_network_contract = _load_runtime_network_contract()

    for entry in matrix:
        env_name = entry.get("esb_env")
        if not env_name:
            print(f"[ERROR] Invalid matrix entry format: {entry}")
            continue

        if profile_filter and env_name != profile_filter:
            continue

        _reject_legacy_deploy_fields(entry)
        esb_project = _resolve_esb_project(entry)
        config_dir = _validate_config_dir(entry, env_name, esb_project)
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
            _inject_runtime_network_defaults(env_name, env_vars, runtime_network_contract)

            env_scenarios[env_name] = {
                "name": f"Combined Scenarios for {env_name}",
                "env_file": env_file,
                "env_dir": f"e2e/environments/{env_dir}" if env_dir else env_dir,
                "config_dir": config_dir,
                "esb_env": env_name,
                "esb_project": esb_project,
                "mode": mode,
                "env_vars": env_vars,
                "targets": [],
                "exclude": [],
                "image_uri_overrides": entry.get("image_uri_overrides", {}) or {},
                "image_runtime_overrides": entry.get("image_runtime_overrides", {}) or {},
            }
            artifact_manifest_value = entry.get("artifact_manifest")
            if artifact_manifest_value is not None:
                artifact_manifest = str(artifact_manifest_value).strip()
                if artifact_manifest:
                    env_scenarios[env_name]["artifact_manifest"] = artifact_manifest
        else:
            existing = env_scenarios[env_name]
            if existing.get("config_dir") != config_dir:
                raise ValueError(
                    f"config_dir mismatch for esb_env '{env_name}': "
                    f"{existing.get('config_dir')} != {config_dir}"
                )
            if existing.get("esb_project") != esb_project:
                raise ValueError(
                    f"esb_project mismatch for esb_env '{env_name}': "
                    f"{existing.get('esb_project')} != {esb_project}"
                )

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
