from pathlib import Path

from tools.deployops.core.artifact_manifest import (
    collect_function_images_and_build_targets,
    iter_runtime_config_dirs,
    load_artifact_manifest,
    resolve_runtime_config_dir,
)


def _write_fixture(tmp_path: Path) -> Path:
    runtime_dir = tmp_path / "entry" / "config"
    runtime_dir.mkdir(parents=True)
    (runtime_dir / "functions.yml").write_text(
        "functions:\n  hello:\n    image: repo/hello:latest\n", encoding="utf-8"
    )
    (runtime_dir / "routing.yml").write_text("routes: []\n", encoding="utf-8")

    manifest_path = tmp_path / "artifact.yml"
    manifest_path.write_text(
        """
schema_version: "1"
project: acme
env: dev
mode: docker
artifacts:
  - artifact_root: entry
    runtime_config_dir: config
""".strip()
        + "\n",
        encoding="utf-8",
    )
    return manifest_path


def test_load_manifest_and_collect_images(tmp_path: Path) -> None:
    manifest_path = _write_fixture(tmp_path)
    manifest = load_artifact_manifest(manifest_path)

    runtime_dirs = iter_runtime_config_dirs(manifest)
    assert len(runtime_dirs) == 1
    assert runtime_dirs[0].name == "config"

    images, targets = collect_function_images_and_build_targets(manifest)
    assert images == ["repo/hello:latest"]
    assert targets[0].dockerfile_rel == "functions/hello/Dockerfile"


def test_resolve_runtime_config_dir_rejects_absolute_path(tmp_path: Path) -> None:
    manifest_path = tmp_path / "artifact.yml"
    manifest_path.write_text(
        """
schema_version: "1"
project: acme
env: dev
mode: docker
artifacts:
  - artifact_root: entry
    runtime_config_dir: /abs/path
""".strip()
        + "\n",
        encoding="utf-8",
    )

    manifest = load_artifact_manifest(manifest_path)
    try:
        resolve_runtime_config_dir(manifest, manifest.artifacts[0])
    except ValueError:
        pass
    else:
        raise AssertionError("expected ValueError")
