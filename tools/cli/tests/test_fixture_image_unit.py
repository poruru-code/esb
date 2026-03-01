from __future__ import annotations

import subprocess
from pathlib import Path

from tools.cli import fixture_image


def test_buildx_build_command_for_fixture_includes_proxy_build_args() -> None:
    cmd = fixture_image.buildx_build_command_for_fixture(
        tag="127.0.0.1:5010/esb-e2e-image-python:latest",
        context_dir=Path("/tmp/ctx"),
        no_cache=False,
        build_args={},
        env={"http_proxy": "http://proxy.example:8080"},
    )
    joined = " ".join(cmd)
    assert "HTTP_PROXY=http://proxy.example:8080" in joined
    assert "http_proxy=http://proxy.example:8080" in joined


def test_execute_fixture_image_ensure_passes_env_to_run_command(
    monkeypatch, tmp_path: Path
) -> None:
    manifest_path = tmp_path / "artifact.yml"
    manifest_path.write_text("schema_version: '1'\nartifacts: []\n", encoding="utf-8")
    fixture_root = tmp_path / "fixtures"
    (fixture_root / "python").mkdir(parents=True, exist_ok=True)

    run_calls: list[tuple[list[str], dict[str, str] | None]] = []

    monkeypatch.setattr(
        fixture_image.artifact, "read_artifact_manifest", lambda *args, **kwargs: {}
    )
    monkeypatch.setattr(
        fixture_image,
        "collect_local_fixture_image_sources",
        lambda manifest, manifest_path: ["127.0.0.1:5010/esb-e2e-image-python:latest"],
    )

    def fake_run_command(cmd, **kwargs):
        run_calls.append((list(cmd), kwargs.get("env")))
        return subprocess.CompletedProcess(cmd, 0, "", "")

    monkeypatch.setattr(fixture_image, "run_command", fake_run_command)

    result = fixture_image.execute_fixture_image_ensure(
        fixture_image.FixtureImageEnsureInput(
            artifact_path=str(manifest_path),
            no_cache=False,
            fixture_root=str(fixture_root),
            env={"HTTP_PROXY": "http://proxy.example:8080"},
        )
    )

    assert result.prepared_images == ["127.0.0.1:5010/esb-e2e-image-python:latest"]
    assert run_calls[0][1] == {"HTTP_PROXY": "http://proxy.example:8080"}
    assert run_calls[1][1] == {"HTTP_PROXY": "http://proxy.example:8080"}
