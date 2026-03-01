from __future__ import annotations

import errno
import os
import tempfile
import time
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

ARTIFACT_SCHEMA_VERSION_V1 = "1"
_MERGE_LOCK_FILE_NAME = ".artifact-merge.lock"
_MERGE_LOCK_WAIT_TIMEOUT_SECONDS = 30.0
_MERGE_LOCK_POLL_SECONDS = 0.05


class MissingReferencedPathError(RuntimeError):
    def __init__(self, path: str) -> None:
        normalized = str(path).strip()
        if normalized == "":
            super().__init__("referenced path not found")
        else:
            super().__init__(f"referenced path not found: {normalized}")


@dataclass(frozen=True)
class ArtifactSourceTemplate:
    path: str = ""
    sha256: str = ""
    path_set: bool = False
    sha_set: bool = False

    def validate(self, prefix: str) -> None:
        path = self.path.strip()
        if (self.path_set or self.path != "") and path == "":
            raise RuntimeError(f"{prefix}.path must not be blank")

        sha = self.sha256.strip()
        if (self.sha_set or self.sha256 != "") and sha == "":
            raise RuntimeError(f"{prefix}.sha256 must not be blank")
        if sha != "":
            if len(sha) != 64 or any(ch not in "0123456789abcdef" for ch in sha):
                raise RuntimeError(f"{prefix}.sha256 must be 64 lowercase hex characters")


@dataclass(frozen=True)
class ArtifactEntry:
    artifact_root: str
    runtime_config_dir: str
    source_template: ArtifactSourceTemplate | None = None

    def validate(self, index: int) -> None:
        prefix = f"artifacts[{index}]"
        validate_artifact_root(f"{prefix}.artifact_root", self.artifact_root)
        validate_relative_path(f"{prefix}.runtime_config_dir", self.runtime_config_dir)
        if self.source_template is not None:
            self.source_template.validate(f"{prefix}.source_template")


@dataclass(frozen=True)
class ArtifactManifest:
    schema_version: str
    project: str
    env: str
    mode: str
    artifacts: list[ArtifactEntry]

    def validate(self) -> None:
        schema = self.schema_version.strip()
        if schema == "":
            raise RuntimeError("schema_version is required")
        if schema != ARTIFACT_SCHEMA_VERSION_V1:
            raise RuntimeError(
                "unsupported schema_version: "
                f'"{schema}" (supported: "{ARTIFACT_SCHEMA_VERSION_V1}")'
            )
        if self.project.strip() == "":
            raise RuntimeError("project is required")
        if self.env.strip() == "":
            raise RuntimeError("env is required")
        if self.mode.strip() == "":
            raise RuntimeError("mode is required")
        if not self.artifacts:
            raise RuntimeError("artifacts must contain at least one entry")
        for index, entry in enumerate(self.artifacts):
            entry.validate(index)

    def resolve_artifact_root(self, manifest_path: str, index: int) -> str:
        if index < 0 or index >= len(self.artifacts):
            raise RuntimeError(f"artifact index out of range: {index}")
        return resolve_artifact_root_path(manifest_path, self.artifacts[index].artifact_root)

    def resolve_runtime_config_dir(self, manifest_path: str, index: int) -> str:
        if index < 0 or index >= len(self.artifacts):
            raise RuntimeError(f"artifact index out of range: {index}")
        artifact_root = resolve_artifact_root_path(
            manifest_path,
            self.artifacts[index].artifact_root,
        )
        return resolve_entry_relative_path(
            artifact_root,
            self.artifacts[index].runtime_config_dir,
            "runtime_config_dir",
        )


def validate_artifact_root(field: str, value: str) -> None:
    if value.strip() == "":
        raise RuntimeError(f"{field} is required")


def validate_relative_path(field: str, value: str) -> None:
    trimmed = value.strip()
    if trimmed == "":
        raise RuntimeError(f"{field} is required")
    clean = Path(trimmed)
    if clean.is_absolute():
        raise RuntimeError(f"{field} must be a relative path")
    normalized = clean.as_posix()
    if normalized == ".":
        raise RuntimeError(f"{field} must not be '.'")
    depth = 0
    for segment in normalized.split("/"):
        if segment in ("", "."):
            continue
        if segment == "..":
            depth -= 1
            if depth < 0:
                raise RuntimeError(f"{field} must not escape artifact root")
            continue
        depth += 1


def resolve_artifact_root_path(manifest_path: str, artifact_root: str) -> str:
    validate_artifact_root("artifact_root", artifact_root)
    normalized = Path(artifact_root.strip())
    if normalized.is_absolute():
        return str(normalized.resolve())
    base_dir = Path(manifest_path).resolve().parent
    return str((base_dir / normalized).resolve())


def resolve_entry_relative_path(artifact_root: str, rel_path: str, field: str) -> str:
    validate_relative_path(field, rel_path)
    root = Path(artifact_root).resolve()
    resolved = (root / Path(rel_path.strip())).resolve()
    try:
        resolved.relative_to(root)
    except ValueError as exc:
        raise RuntimeError(f"{field} must not escape artifact root") from exc
    return str(resolved)


def _load_yaml_map(path: str) -> tuple[dict[str, Any] | None, bool]:
    try:
        raw = Path(path).read_text(encoding="utf-8")
    except FileNotFoundError:
        return None, False
    parsed = yaml.safe_load(raw)
    if parsed is None:
        return {}, True
    if not isinstance(parsed, dict):
        raise RuntimeError(f"YAML must decode as a map: {path}")
    return parsed, True


def _atomic_write_yaml(path: str, payload: dict[str, Any]) -> None:
    path_obj = Path(path)
    path_obj.parent.mkdir(parents=True, exist_ok=True)
    data = yaml.safe_dump(payload, sort_keys=False)
    with tempfile.NamedTemporaryFile(
        "w",
        encoding="utf-8",
        dir=str(path_obj.parent),
        prefix=".tmp-",
        delete=False,
    ) as tmp:
        tmp.write(data)
        tmp.flush()
        os.fsync(tmp.fileno())
        tmp_path = Path(tmp.name)
    tmp_path.replace(path_obj)


def _decode_source_template(raw: Any) -> ArtifactSourceTemplate | None:
    if raw is None:
        return None
    if not isinstance(raw, dict):
        raise RuntimeError("source_template must be map")
    path_set = "path" in raw
    sha_set = "sha256" in raw
    path_value = str(raw.get("path", "")) if path_set else ""
    sha_value = str(raw.get("sha256", "")) if sha_set else ""
    return ArtifactSourceTemplate(
        path=path_value,
        sha256=sha_value,
        path_set=path_set,
        sha_set=sha_set,
    )


def read_artifact_manifest(path: str, *, validate: bool = True) -> ArtifactManifest:
    path_obj = Path(path)
    try:
        raw = path_obj.read_text(encoding="utf-8")
    except FileNotFoundError as exc:
        raise RuntimeError(f"read artifact manifest: {MissingReferencedPathError(path)}") from exc
    payload = yaml.safe_load(raw)
    if not isinstance(payload, dict):
        raise RuntimeError("decode artifact manifest: top-level must be map")

    artifacts_raw = payload.get("artifacts", [])
    if artifacts_raw is None:
        artifacts_raw = []
    if not isinstance(artifacts_raw, list):
        raise RuntimeError("artifacts must be a list")

    artifacts: list[ArtifactEntry] = []
    for item in artifacts_raw:
        if not isinstance(item, dict):
            raise RuntimeError("artifact entry must be map")
        artifacts.append(
            ArtifactEntry(
                artifact_root=str(item.get("artifact_root", "")),
                runtime_config_dir=str(item.get("runtime_config_dir", "")),
                source_template=_decode_source_template(item.get("source_template")),
            )
        )

    manifest = ArtifactManifest(
        schema_version=str(payload.get("schema_version", "")),
        project=str(payload.get("project", "")),
        env=str(payload.get("env", "")),
        mode=str(payload.get("mode", "")),
        artifacts=artifacts,
    )
    if validate:
        manifest.validate()
    return manifest


def execute_apply(artifact_path: str, output_dir: str) -> list[str]:
    artifact_path = artifact_path.strip()
    output_dir = output_dir.strip()
    if artifact_path == "":
        raise RuntimeError("artifact path is required")
    if output_dir == "":
        raise RuntimeError("output dir is required")

    manifest = read_artifact_manifest(artifact_path, validate=True)
    merge_with_manifest(artifact_path, output_dir, manifest)
    return []


def merge_with_manifest(manifest_path: str, output_dir: str, manifest: ArtifactManifest) -> None:
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)
    with_output_dir_lock(
        str(output_path),
        lambda: _merge_manifest_entries(manifest_path, str(output_path), manifest),
    )


def _merge_manifest_entries(
    manifest_path: str,
    output_dir: str,
    manifest: ArtifactManifest,
) -> None:
    for index in range(len(manifest.artifacts)):
        runtime_dir = manifest.resolve_runtime_config_dir(manifest_path, index)
        try:
            merge_one_runtime_config(runtime_dir, output_dir)
        except Exception as exc:  # noqa: BLE001
            raise RuntimeError(f"merge artifacts[{index}]: {exc}") from exc


def with_output_dir_lock(output_dir: str, action: Callable[[], None]) -> None:
    lock_path = Path(output_dir) / _MERGE_LOCK_FILE_NAME
    deadline = time.monotonic() + _MERGE_LOCK_WAIT_TIMEOUT_SECONDS
    while True:
        try:
            fd = os.open(str(lock_path), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        except OSError as exc:
            if exc.errno != errno.EEXIST:
                raise RuntimeError(f"create merge lock file: {exc}") from exc
            recovered = _try_recover_stale_lock(lock_path)
            if recovered:
                continue
            if time.monotonic() > deadline:
                raise RuntimeError(f"timed out waiting for merge lock: {lock_path}") from exc
            time.sleep(_MERGE_LOCK_POLL_SECONDS)
            continue

        try:
            with os.fdopen(fd, "w", encoding="utf-8") as lock_file:
                lock_file.write(f"{os.getpid()}\n")
            action()
            return
        finally:
            try:
                lock_path.unlink()
            except FileNotFoundError:
                pass


def _try_recover_stale_lock(lock_path: Path) -> bool:
    if not lock_path.exists():
        return False
    owner_pid = _read_lock_owner_pid(lock_path)
    if owner_pid is None:
        return False
    alive = _is_process_alive(owner_pid)
    if alive:
        return False
    try:
        lock_path.unlink()
    except FileNotFoundError:
        return False
    return True


def _read_lock_owner_pid(lock_path: Path) -> int | None:
    try:
        first_line = lock_path.read_text(encoding="utf-8").splitlines()[0].strip()
    except FileNotFoundError:
        return None
    except IndexError:
        return None
    if first_line.startswith("pid="):
        first_line = first_line[4:].strip()
    try:
        pid = int(first_line)
    except ValueError:
        return None
    if pid <= 0:
        return None
    return pid


def _is_process_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def merge_one_runtime_config(src_dir: str, dest_dir: str) -> None:
    merge_functions_yml(src_dir, dest_dir)
    merge_routing_yml(src_dir, dest_dir)
    merge_resources_yml(src_dir, dest_dir)


def _as_map(value: Any) -> dict[str, Any]:
    if isinstance(value, dict):
        out: dict[str, Any] = {}
        for key, val in value.items():
            if not isinstance(key, str):
                continue
            out[key] = val
        return out
    return {}


def _as_list(value: Any) -> list[Any]:
    if isinstance(value, list):
        return list(value)
    return []


def _route_key(route: dict[str, Any]) -> str:
    path = str(route.get("path", "")).strip()
    method = str(route.get("method", "")).strip()
    if path == "":
        return ""
    if method == "":
        method = "GET"
    return f"{path}:{method}"


def _wrap_required_source_load_error(src_path: str, exc: Exception) -> RuntimeError:
    if isinstance(exc, FileNotFoundError):
        return RuntimeError(f"required file not found: {MissingReferencedPathError(src_path)}")
    return RuntimeError(str(exc))


def merge_functions_yml(src_dir: str, dest_dir: str) -> None:
    src_path = str(Path(src_dir) / "functions.yml")
    dest_path = str(Path(dest_dir) / "functions.yml")
    try:
        src_data, src_exists = _load_yaml_map(src_path)
    except Exception as exc:  # noqa: BLE001
        raise _wrap_required_source_load_error(src_path, exc) from exc
    if not src_exists:
        raise RuntimeError(f"required file not found: {MissingReferencedPathError(src_path)}")
    if src_data is None:
        src_data = {}

    existing_data, _ = _load_yaml_map(dest_path)
    if existing_data is None:
        existing_data = {}

    src_functions = _as_map(src_data.get("functions"))
    existing_functions = _as_map(existing_data.get("functions"))
    for name, payload in src_functions.items():
        existing_functions[name] = payload

    src_defaults = _as_map(src_data.get("defaults"))
    existing_defaults = _as_map(existing_data.get("defaults"))
    _merge_defaults_section(existing_defaults, src_defaults, "environment")
    _merge_defaults_section(existing_defaults, src_defaults, "scaling")
    for key, value in src_defaults.items():
        if key in {"environment", "scaling"}:
            continue
        if key not in existing_defaults:
            existing_defaults[key] = value

    merged: dict[str, Any] = {"functions": existing_functions}
    if existing_defaults:
        merged["defaults"] = existing_defaults
    _atomic_write_yaml(dest_path, merged)


def _merge_defaults_section(
    existing_defaults: dict[str, Any],
    src_defaults: dict[str, Any],
    key: str,
) -> None:
    src_section = _as_map(src_defaults.get(key))
    if not src_section:
        return
    existing_section = _as_map(existing_defaults.get(key))
    for item_key, item_value in src_section.items():
        if item_key not in existing_section:
            existing_section[item_key] = item_value
    if existing_section:
        existing_defaults[key] = existing_section


def merge_routing_yml(src_dir: str, dest_dir: str) -> None:
    src_path = str(Path(src_dir) / "routing.yml")
    dest_path = str(Path(dest_dir) / "routing.yml")
    try:
        src_data, src_exists = _load_yaml_map(src_path)
    except Exception as exc:  # noqa: BLE001
        raise _wrap_required_source_load_error(src_path, exc) from exc
    if not src_exists:
        raise RuntimeError(f"required file not found: {MissingReferencedPathError(src_path)}")
    if src_data is None:
        src_data = {}

    existing_data, _ = _load_yaml_map(dest_path)
    if existing_data is None:
        existing_data = {}

    existing_routes = _as_list(existing_data.get("routes"))
    route_index: dict[str, int] = {}
    for idx, route in enumerate(existing_routes):
        key = _route_key(_as_map(route))
        if key == "":
            continue
        route_index[key] = idx

    src_routes = _as_list(src_data.get("routes"))
    for route in src_routes:
        route_map = _as_map(route)
        key = _route_key(route_map)
        if key == "":
            continue
        if key in route_index:
            existing_routes[route_index[key]] = route
        else:
            route_index[key] = len(existing_routes)
            existing_routes.append(route)

    _atomic_write_yaml(dest_path, {"routes": existing_routes})


def merge_resources_yml(src_dir: str, dest_dir: str) -> None:
    src_path = str(Path(src_dir) / "resources.yml")
    dest_path = str(Path(dest_dir) / "resources.yml")
    src_data, src_exists = _load_yaml_map(src_path)
    if not src_exists:
        return
    if src_data is None:
        src_data = {}

    existing_data, _ = _load_yaml_map(dest_path)
    if existing_data is None:
        existing_data = {}

    src_resources = _as_map(src_data.get("resources"))
    existing_resources = _as_map(existing_data.get("resources"))

    merged_dynamodb = _merge_resource_list(
        _as_list(existing_resources.get("dynamodb")),
        _as_list(src_resources.get("dynamodb")),
        "TableName",
    )
    if merged_dynamodb:
        existing_resources["dynamodb"] = merged_dynamodb

    merged_s3 = _merge_resource_list(
        _as_list(existing_resources.get("s3")),
        _as_list(src_resources.get("s3")),
        "BucketName",
    )
    if merged_s3:
        existing_resources["s3"] = merged_s3

    merged_layers = _merge_resource_list(
        _as_list(existing_resources.get("layers")),
        _as_list(src_resources.get("layers")),
        "Name",
    )
    if merged_layers:
        existing_resources["layers"] = merged_layers

    _atomic_write_yaml(dest_path, {"resources": existing_resources})


def _merge_resource_list(existing: list[Any], src: list[Any], key_field: str) -> list[Any]:
    index: dict[str, int] = {}
    for idx, item in enumerate(existing):
        key = str(_as_map(item).get(key_field, "")).strip()
        if key == "":
            continue
        index[key] = idx
    for item in src:
        key = str(_as_map(item).get(key_field, "")).strip()
        if key == "":
            continue
        if key in index:
            existing[index[key]] = item
        else:
            index[key] = len(existing)
            existing.append(item)
    return existing
