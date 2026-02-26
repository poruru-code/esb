"""Artifact manifest parsing and resolution helpers."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml


@dataclass(frozen=True)
class ArtifactEntry:
    artifact_root: str
    runtime_config_dir: str
    source_template: dict[str, Any] | None = None


@dataclass(frozen=True)
class ArtifactManifest:
    path: Path
    schema_version: str
    project: str
    env: str
    mode: str
    artifacts: tuple[ArtifactEntry, ...]


@dataclass(frozen=True)
class FunctionBuildTarget:
    image_ref: str
    context_dir: Path
    dockerfile_rel: str

    @property
    def dockerfile_path(self) -> Path:
        return self.context_dir / self.dockerfile_rel


def load_artifact_manifest(path: Path) -> ArtifactManifest:
    manifest_path = Path(path).resolve()
    if not manifest_path.is_file():
        raise FileNotFoundError(f"artifact manifest not found: {manifest_path}")

    payload = yaml.safe_load(manifest_path.read_text(encoding="utf-8")) or {}
    if not isinstance(payload, dict):
        raise ValueError(f"artifact manifest must be a mapping: {manifest_path}")

    schema_version = _require_non_empty(payload, "schema_version", manifest_path)
    project = _require_non_empty(payload, "project", manifest_path)
    env = _require_non_empty(payload, "env", manifest_path)
    mode = _require_non_empty(payload, "mode", manifest_path)

    artifacts_raw = payload.get("artifacts")
    if not isinstance(artifacts_raw, list) or len(artifacts_raw) == 0:
        raise ValueError(f"artifacts[] must be a non-empty list: {manifest_path}")

    artifacts: list[ArtifactEntry] = []
    for index, raw_entry in enumerate(artifacts_raw):
        if not isinstance(raw_entry, dict):
            raise ValueError(f"artifacts[{index}] must be a mapping in manifest: {manifest_path}")
        artifact_root = _require_non_empty(raw_entry, "artifact_root", manifest_path, index)
        runtime_config_dir = _require_non_empty(
            raw_entry, "runtime_config_dir", manifest_path, index
        )
        source_template = raw_entry.get("source_template")
        if source_template is not None and not isinstance(source_template, dict):
            raise ValueError(
                f"artifacts[{index}].source_template must be a mapping if present: {manifest_path}"
            )
        artifacts.append(
            ArtifactEntry(
                artifact_root=artifact_root,
                runtime_config_dir=runtime_config_dir,
                source_template=source_template,
            )
        )

    return ArtifactManifest(
        path=manifest_path,
        schema_version=schema_version,
        project=project,
        env=env,
        mode=mode,
        artifacts=tuple(artifacts),
    )


def load_artifact_manifest_from_dir(artifact_dir: Path) -> ArtifactManifest:
    directory = Path(artifact_dir).resolve()
    return load_artifact_manifest(directory / "artifact.yml")


def resolve_artifact_root(manifest: ArtifactManifest, entry: ArtifactEntry) -> Path:
    root = Path(entry.artifact_root)
    if root.is_absolute():
        return root.resolve()
    return (manifest.path.parent / root).resolve()


def resolve_runtime_config_dir(manifest: ArtifactManifest, entry: ArtifactEntry) -> Path:
    artifact_root = resolve_artifact_root(manifest, entry)
    runtime_dir = Path(entry.runtime_config_dir)
    if runtime_dir.is_absolute():
        raise ValueError(
            "runtime_config_dir must be relative to artifact_root: "
            f"{entry.runtime_config_dir!r} ({manifest.path})"
        )

    resolved = (artifact_root / runtime_dir).resolve()
    if not _is_within_directory(resolved, artifact_root):
        raise ValueError(
            "runtime_config_dir escapes artifact_root: "
            f"{entry.runtime_config_dir!r} ({manifest.path})"
        )
    return resolved


def iter_runtime_config_dirs(manifest: ArtifactManifest) -> list[Path]:
    result: list[Path] = []
    seen: set[Path] = set()
    for entry in manifest.artifacts:
        runtime_dir = resolve_runtime_config_dir(manifest, entry)
        if runtime_dir in seen:
            continue
        seen.add(runtime_dir)
        result.append(runtime_dir)
    return result


def collect_function_images_and_build_targets(
    manifest: ArtifactManifest,
) -> tuple[list[str], list[FunctionBuildTarget]]:
    images: list[str] = []
    seen_images: set[str] = set()
    targets: list[FunctionBuildTarget] = []
    seen_targets: set[tuple[str, Path, str]] = set()

    for entry in manifest.artifacts:
        runtime_dir = resolve_runtime_config_dir(manifest, entry)
        functions_path = runtime_dir / "functions.yml"
        if not functions_path.is_file():
            continue

        functions_payload = yaml.safe_load(functions_path.read_text(encoding="utf-8")) or {}
        functions_map = functions_payload.get("functions")
        if not isinstance(functions_map, dict):
            continue

        artifact_root = resolve_artifact_root(manifest, entry)

        for function_name, spec in functions_map.items():
            if not isinstance(spec, dict):
                continue
            image_ref = str(spec.get("image", "")).strip()
            if not image_ref:
                continue
            if image_ref not in seen_images:
                seen_images.add(image_ref)
                images.append(image_ref)

            dockerfile_rel = f"functions/{function_name}/Dockerfile"
            target = FunctionBuildTarget(
                image_ref=image_ref,
                context_dir=artifact_root,
                dockerfile_rel=dockerfile_rel,
            )
            target_key = (target.image_ref, target.context_dir, target.dockerfile_rel)
            if target_key in seen_targets:
                continue
            seen_targets.add(target_key)
            targets.append(target)

    return images, targets


def validate_runtime_config_dirs(manifest: ArtifactManifest) -> None:
    for runtime_dir in iter_runtime_config_dirs(manifest):
        if not runtime_dir.is_dir():
            raise FileNotFoundError(f"runtime config source not found: {runtime_dir}")


def _is_within_directory(path: Path, parent: Path) -> bool:
    try:
        path.relative_to(parent)
        return True
    except ValueError:
        return False


def _require_non_empty(
    payload: dict[str, Any],
    key: str,
    manifest_path: Path,
    entry_index: int | None = None,
) -> str:
    value = str(payload.get(key, "")).strip()
    if value:
        return value
    if entry_index is None:
        raise ValueError(f"{key} is required in manifest: {manifest_path}")
    raise ValueError(f"artifacts[{entry_index}].{key} is required: {manifest_path}")
