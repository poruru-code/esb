from __future__ import annotations

import json
import subprocess
from pathlib import Path


def _write_manifest(
    path: Path, template_path: str, image_name: str, build_overrides: dict | None = None
) -> None:
    build = {
        "project": "esb-default",
        "env": "dev",
        "mode": "docker",
        "image_prefix": "esb",
        "image_tag": "latest",
        "git": {"commit": "deadbeef", "dirty": False},
    }
    if build_overrides:
        build.update(build_overrides)
    template = {
        "path": template_path,
        "sha256": "0" * 64,
        "parameters": {"ParamA": "value"},
    }
    payload = {
        "schema_version": "1.1",
        "generated_at": "2026-02-04T00:00:00Z",
        "templates": [template],
        "template": template,
        "build": build,
        "images": [
            {
                "name": image_name,
                "digest": "sha256:" + ("a" * 64),
                "kind": "function",
                "source": "template",
                "platform": "linux/amd64",
            }
        ],
    }
    path.write_text(json.dumps(payload), encoding="utf-8")


def test_merge_manifest(tmp_path: Path) -> None:
    repo_root = Path(__file__).resolve().parents[2]
    manifest_a = tmp_path / "manifest-a.json"
    manifest_b = tmp_path / "manifest-b.json"
    output = tmp_path / "merged.json"

    _write_manifest(manifest_a, "template-a.yaml", "esb-func-a:latest")
    _write_manifest(manifest_b, "template-b.yaml", "esb-func-b:latest")

    result = subprocess.run(
        [
            "python3",
            "tools/dind-bundler/merge_manifest.py",
            "--output",
            str(output),
            str(manifest_a),
            str(manifest_b),
        ],
        cwd=repo_root,
        check=True,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0

    data = json.loads(output.read_text(encoding="utf-8"))
    assert data["schema_version"] == "1.1"
    assert len(data["templates"]) == 2
    template_paths = {tpl["path"] for tpl in data["templates"]}
    assert template_paths == {"template-a.yaml", "template-b.yaml"}
    image_names = {img["name"] for img in data["images"]}
    assert image_names == {"esb-func-a:latest", "esb-func-b:latest"}


def test_merge_manifest_rejects_build_mismatch(tmp_path: Path) -> None:
    repo_root = Path(__file__).resolve().parents[2]
    manifest_a = tmp_path / "manifest-a.json"
    manifest_b = tmp_path / "manifest-b.json"
    output = tmp_path / "merged.json"

    _write_manifest(manifest_a, "template-a.yaml", "esb-func-a:latest")
    _write_manifest(
        manifest_b,
        "template-b.yaml",
        "esb-func-b:latest",
        build_overrides={"project": "other-project"},
    )

    result = subprocess.run(
        [
            "python3",
            "tools/dind-bundler/merge_manifest.py",
            "--output",
            str(output),
            str(manifest_a),
            str(manifest_b),
        ],
        cwd=repo_root,
        capture_output=True,
        text=True,
    )
    assert result.returncode != 0
