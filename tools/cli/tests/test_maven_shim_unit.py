from __future__ import annotations

from pathlib import Path

from tools.cli import maven_shim


def test_assets_dir_is_cli_owned() -> None:
    assets_dir = maven_shim._assets_dir()
    assert "tools/cli/assets/mavenshim" in str(assets_dir).replace("\\", "/")
    assert (assets_dir / "Dockerfile").exists()
    assert (assets_dir / "mvn-wrapper.sh").exists()


def test_derive_shim_image_tag_is_deterministic() -> None:
    first = maven_shim.derive_shim_image_tag("maven:3.9.9")
    second = maven_shim.derive_shim_image_tag("maven:3.9.9")
    third = maven_shim.derive_shim_image_tag("maven:3.9.8")
    assert first == second
    assert first != third
    assert first.startswith("esb-maven-shim:")


def test_buildx_command_contains_flags_and_build_args(tmp_path: Path) -> None:
    dockerfile = tmp_path / "Dockerfile"
    dockerfile.write_text("FROM scratch\n", encoding="utf-8")

    cmd = maven_shim._buildx_command(
        "esb-maven-shim:test",
        dockerfile,
        tmp_path,
        no_cache=True,
        build_args={"BASE_MAVEN_IMAGE": "maven:3.9", "EMPTY": "  "},
        env=None,
    )
    assert cmd[:5] == ["docker", "buildx", "build", "--platform", "linux/amd64"]
    assert "--no-cache" in cmd
    assert "--build-arg" in cmd
    assert "BASE_MAVEN_IMAGE=maven:3.9" in cmd
    assert "EMPTY=" not in " ".join(cmd)
    assert cmd[-3:] == ["--file", str(dockerfile), str(tmp_path)]


def test_ensure_image_skips_build_when_cached_and_pushes_when_registry_set(monkeypatch) -> None:
    commands: list[list[str]] = []
    released = {"done": False}

    monkeypatch.setattr(maven_shim, "docker_image_exists", lambda _: True)
    monkeypatch.setattr(
        maven_shim, "run_command", lambda cmd, check=True, env=None: commands.append(cmd)
    )
    monkeypatch.setattr(
        maven_shim,
        "_acquire_lock",
        lambda _: (lambda: released.__setitem__("done", True)),
    )

    result = maven_shim.ensure_image(
        maven_shim.EnsureInput(base_image="maven:3.9.9", host_registry="127.0.0.1:5010")
    )
    assert result.shim_image.startswith("127.0.0.1:5010/esb-maven-shim:")
    assert commands == [["docker", "push", result.shim_image]]
    assert released["done"] is True


def test_ensure_image_builds_when_cache_miss(monkeypatch, tmp_path: Path) -> None:
    commands: list[list[str]] = []

    monkeypatch.setattr(maven_shim, "docker_image_exists", lambda _: False)
    monkeypatch.setattr(
        maven_shim, "run_command", lambda cmd, check=True, env=None: commands.append(cmd)
    )
    monkeypatch.setattr(maven_shim, "_acquire_lock", lambda _: (lambda: None))
    monkeypatch.setattr(
        maven_shim,
        "_materialize_build_context",
        lambda: (tmp_path, lambda: None),
    )

    result = maven_shim.ensure_image(maven_shim.EnsureInput(base_image="maven:3.9.9"))
    assert result.shim_image.startswith("esb-maven-shim:")
    assert commands, "docker buildx command should be emitted on cache miss"
    assert commands[0][:3] == ["docker", "buildx", "build"]
