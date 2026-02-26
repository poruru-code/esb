from __future__ import annotations

from pathlib import Path

import pytest

from tools.deployops.core.discovery import (
    resolve_artifact_manifest_path,
    resolve_compose_file_path,
    resolve_env_file_path,
)
from tools.deployops.core.runner import RunnerError


def test_resolve_artifact_manifest_path_prefers_env_hint(tmp_path: Path) -> None:
    docker_manifest = tmp_path / "e2e" / "artifacts" / "e2e-docker" / "artifact.yml"
    containerd_manifest = tmp_path / "e2e" / "artifacts" / "e2e-containerd" / "artifact.yml"
    docker_manifest.parent.mkdir(parents=True)
    containerd_manifest.parent.mkdir(parents=True)
    docker_manifest.write_text("schema_version: '1'\n", encoding="utf-8")
    containerd_manifest.write_text("schema_version: '1'\n", encoding="utf-8")

    resolved = resolve_artifact_manifest_path(
        project_root=tmp_path,
        artifact=None,
        env_hint="e2e-docker",
        allow_prompt=False,
    )

    assert resolved == docker_manifest.resolve()


def test_resolve_artifact_manifest_path_ambiguous_non_interactive(tmp_path: Path) -> None:
    (tmp_path / "artifacts" / "a").mkdir(parents=True)
    (tmp_path / "artifacts" / "b").mkdir(parents=True)
    (tmp_path / "artifacts" / "a" / "artifact.yml").write_text(
        "schema_version: '1'\n", encoding="utf-8"
    )
    (tmp_path / "artifacts" / "b" / "artifact.yml").write_text(
        "schema_version: '1'\n", encoding="utf-8"
    )

    with pytest.raises(RunnerError):
        resolve_artifact_manifest_path(
            project_root=tmp_path,
            artifact=None,
            allow_prompt=False,
        )


def test_resolve_env_file_path_prefers_env_hint(tmp_path: Path) -> None:
    docker_env = tmp_path / "e2e" / "environments" / "e2e-docker" / ".env"
    containerd_env = tmp_path / "e2e" / "environments" / "e2e-containerd" / ".env"
    docker_env.parent.mkdir(parents=True)
    containerd_env.parent.mkdir(parents=True)
    docker_env.write_text("ENV=e2e-docker\n", encoding="utf-8")
    containerd_env.write_text("ENV=e2e-containerd\n", encoding="utf-8")

    resolved = resolve_env_file_path(
        project_root=tmp_path,
        env_file=None,
        env_hint="e2e-docker",
        required=True,
        allow_prompt=False,
    )

    assert resolved == docker_env.resolve()


def test_resolve_env_file_path_missing_when_required(tmp_path: Path) -> None:
    with pytest.raises(FileNotFoundError):
        resolve_env_file_path(
            project_root=tmp_path,
            env_file=None,
            env_hint=None,
            required=True,
            allow_prompt=False,
        )


def test_resolve_compose_prefers_mode_specific_when_env_is_root(tmp_path: Path) -> None:
    env_file = tmp_path / ".env"
    env_file.write_text("ENV=e2e-containerd\n", encoding="utf-8")
    (tmp_path / "docker-compose.yml").write_text("services: {}\n", encoding="utf-8")
    mode_compose = tmp_path / "docker-compose.containerd.yml"
    mode_compose.write_text("services: { runtime-node: {} }\n", encoding="utf-8")

    resolved = resolve_compose_file_path(
        project_root=tmp_path,
        compose_file=None,
        env_file=env_file,
        mode_hint="containerd",
    )

    assert resolved == mode_compose.resolve()


def test_resolve_compose_prefers_env_local_compose(tmp_path: Path) -> None:
    env_file = tmp_path / "e2e" / "environments" / "e2e-containerd" / ".env"
    env_file.parent.mkdir(parents=True)
    env_file.write_text("ENV=e2e-containerd\n", encoding="utf-8")
    local_compose = env_file.parent / "docker-compose.yml"
    local_compose.write_text("services: {}\n", encoding="utf-8")
    (tmp_path / "docker-compose.containerd.yml").write_text(
        "services: { runtime-node: {} }\n", encoding="utf-8"
    )

    resolved = resolve_compose_file_path(
        project_root=tmp_path,
        compose_file=None,
        env_file=env_file,
        mode_hint="containerd",
    )

    assert resolved == local_compose.resolve()
