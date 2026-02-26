from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

import pytest

from tools.deployops.core.apply_ops import ApplyOptions, execute_apply
from tools.deployops.core.runner import CompletedCommand, RunnerError


@dataclass
class FakeRunner:
    dry_run: bool = True

    def __post_init__(self) -> None:
        self.commands: list[tuple[list[str], bool]] = []
        self.command_envs: list[dict[str, str] | None] = []
        self.messages: list[str] = []

    def emit(self, message: str) -> None:
        self.messages.append(message)

    def which(self, command: str) -> str | None:
        return f"/usr/bin/{command}"

    def run(
        self,
        cmd,
        *,
        capture_output: bool = False,
        check: bool = True,
        stream_output: bool = False,
        on_line=None,
        run_in_dry_run: bool = False,
        cwd=None,
        env=None,
    ) -> CompletedCommand:
        del capture_output, check, stream_output, on_line, cwd
        command = [str(token) for token in cmd]
        self.commands.append((command, run_in_dry_run))
        self.command_envs.append(dict(env) if isinstance(env, dict) else None)
        if self.dry_run and not run_in_dry_run:
            return CompletedCommand(tuple(command), 0, "", "")
        return CompletedCommand(tuple(command), 0, "", "")


def test_execute_apply_dry_run_skips_registry_probe(monkeypatch, tmp_path: Path) -> None:
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

    compose_file = tmp_path / "docker-compose.yml"
    compose_file.write_text("services: {}\n", encoding="utf-8")
    (tmp_path / ".env").write_text(
        "ENV=dev\nJWT_SECRET_KEY=test-secret-key-must-be-at-least-32-chars\n",
        encoding="utf-8",
    )

    calls: list[str] = []

    def _unexpected_probe(*args, **kwargs):
        del args, kwargs
        calls.append("called")
        raise AssertionError("wait_for_registry_ready should not be called in dry-run")

    monkeypatch.setattr("tools.deployops.core.apply_ops.wait_for_registry_ready", _unexpected_probe)

    options = ApplyOptions(
        artifact=str(manifest_path),
        compose_file=None,
        env_file=None,
        ctl_bin=None,
        registry_wait_timeout=None,
        registry_port=None,
        project_dir=str(tmp_path),
    )
    runner = FakeRunner(dry_run=True)

    rc = execute_apply(options, runner)

    assert rc == 0
    assert calls == []
    assert any("skip registry readiness probe" in msg for msg in runner.messages)

    commands = [" ".join(cmd) for cmd, _ in runner.commands]
    assert any(
        "docker compose -p acme-dev --env-file" in cmd and "up -d" in cmd for cmd in commands
    )
    assert any("internal fixture-image ensure --artifact" in cmd for cmd in commands)
    assert any("deploy --artifact" in cmd for cmd in commands)
    assert any("provision --project acme-dev" in cmd for cmd in commands)


def test_execute_apply_prefers_root_env_for_artifacts_layout(monkeypatch, tmp_path: Path) -> None:
    artifact_dir = tmp_path / "artifacts" / "dev"
    artifact_dir.mkdir(parents=True)
    manifest_path = artifact_dir / "artifact.yml"
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

    compose_file = tmp_path / "docker-compose.yml"
    compose_file.write_text("services: {}\n", encoding="utf-8")
    (tmp_path / ".env").write_text(
        "ENV=dev\nJWT_SECRET_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n", encoding="utf-8"
    )
    env_dir = tmp_path / "environments" / "dev"
    env_dir.mkdir(parents=True)
    (env_dir / ".env").write_text("ENV=dev\n", encoding="utf-8")

    monkeypatch.setattr(
        "tools.deployops.core.apply_ops.wait_for_registry_ready",
        lambda *_args, **_kwargs: None,
    )

    options = ApplyOptions(
        artifact=str(manifest_path),
        compose_file=None,
        env_file=None,
        ctl_bin=None,
        registry_wait_timeout=None,
        registry_port=None,
        project_dir=str(tmp_path),
    )
    runner = FakeRunner(dry_run=True)

    rc = execute_apply(options, runner)

    assert rc == 0
    commands = [" ".join(cmd) for cmd, _ in runner.commands]
    assert any(f"--env-file {tmp_path / '.env'}" in cmd for cmd in commands)


def test_execute_apply_env_resolution_does_not_depend_on_artifact_path(
    monkeypatch,
    tmp_path: Path,
) -> None:
    root_env = tmp_path / ".env"
    root_env.write_text(
        "ENV=dev\nJWT_SECRET_KEY=test-secret-key-must-be-at-least-32-chars\n",
        encoding="utf-8",
    )
    (tmp_path / "docker-compose.yml").write_text("services: {}\n", encoding="utf-8")

    env_docker = tmp_path / "e2e" / "environments" / "e2e-docker"
    env_containerd = tmp_path / "e2e" / "environments" / "e2e-containerd"
    env_docker.mkdir(parents=True)
    env_containerd.mkdir(parents=True)
    (env_docker / ".env").write_text("ENV=e2e-docker\n", encoding="utf-8")
    (env_containerd / ".env").write_text("ENV=e2e-containerd\n", encoding="utf-8")

    docker_artifact = tmp_path / "e2e" / "artifacts" / "e2e-docker"
    containerd_artifact = tmp_path / "e2e" / "artifacts" / "e2e-containerd"
    docker_artifact.mkdir(parents=True)
    containerd_artifact.mkdir(parents=True)
    for artifact_dir, env_name in [
        (docker_artifact, "e2e-docker"),
        (containerd_artifact, "e2e-containerd"),
    ]:
        (artifact_dir / "artifact.yml").write_text(
            (
                'schema_version: "1"\n'
                "project: acme\n"
                f"env: {env_name}\n"
                "mode: docker\n"
                "artifacts:\n"
                "  - artifact_root: entry\n"
                "    runtime_config_dir: config\n"
            ),
            encoding="utf-8",
        )

    monkeypatch.setattr(
        "tools.deployops.core.apply_ops.wait_for_registry_ready",
        lambda *_args, **_kwargs: None,
    )

    for manifest_path in [
        docker_artifact / "artifact.yml",
        containerd_artifact / "artifact.yml",
    ]:
        runner = FakeRunner(dry_run=True)
        options = ApplyOptions(
            artifact=str(manifest_path),
            compose_file=None,
            env_file=None,
            ctl_bin=None,
            registry_wait_timeout=None,
            registry_port=None,
            project_dir=str(tmp_path),
        )
        rc = execute_apply(options, runner)
        assert rc == 0
        commands = [" ".join(cmd) for cmd, _ in runner.commands]
        assert any(f"--env-file {root_env}" in cmd for cmd in commands)


def test_execute_apply_requires_env_file_when_root_env_missing(tmp_path: Path) -> None:
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

    (tmp_path / "docker-compose.yml").write_text("services: {}\n", encoding="utf-8")

    options = ApplyOptions(
        artifact=str(manifest_path),
        compose_file=None,
        env_file=None,
        ctl_bin=None,
        registry_wait_timeout=None,
        registry_port=None,
        project_dir=str(tmp_path),
    )
    runner = FakeRunner(dry_run=True)

    with pytest.raises(RunnerError, match="Pass --env-file"):
        execute_apply(options, runner)


def test_execute_apply_rejects_mode_mismatch_for_root_compose(tmp_path: Path) -> None:
    manifest_path = tmp_path / "artifact.yml"
    manifest_path.write_text(
        """
schema_version: "1"
project: acme
env: dev
mode: containerd
artifacts:
  - artifact_root: entry
    runtime_config_dir: config
""".strip()
        + "\n",
        encoding="utf-8",
    )
    (tmp_path / "docker-compose.yml").write_text(
        """
include:
  - path: docker-compose.infra.yml
  - path: docker-compose.docker.yml
services: {}
""".strip()
        + "\n",
        encoding="utf-8",
    )
    (tmp_path / ".env").write_text(
        "ENV=dev\nJWT_SECRET_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n", encoding="utf-8"
    )

    options = ApplyOptions(
        artifact=str(manifest_path),
        compose_file=None,
        env_file=None,
        ctl_bin=None,
        registry_wait_timeout=None,
        registry_port=None,
        project_dir=str(tmp_path),
    )
    runner = FakeRunner(dry_run=True)

    with pytest.raises(RunnerError, match="artifact mode does not match compose mode"):
        execute_apply(options, runner)


def test_execute_apply_sets_registry_env_defaults_for_deploy(tmp_path: Path) -> None:
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
    (tmp_path / "docker-compose.yml").write_text("services: {}\n", encoding="utf-8")
    (tmp_path / ".env").write_text(
        "ENV=dev\nJWT_SECRET_KEY=test-secret-key-must-be-at-least-32-chars\n",
        encoding="utf-8",
    )

    options = ApplyOptions(
        artifact=str(manifest_path),
        compose_file=None,
        env_file=None,
        ctl_bin=None,
        registry_wait_timeout=None,
        registry_port=None,
        project_dir=str(tmp_path),
    )
    runner = FakeRunner(dry_run=True)

    rc = execute_apply(options, runner)
    assert rc == 0

    deploy_env: dict[str, str] | None = None
    for (cmd, _), env in zip(runner.commands, runner.command_envs, strict=True):
        if "--artifact" in cmd and "deploy" in cmd:
            deploy_env = env
            break

    assert deploy_env is not None
    assert deploy_env["HOST_REGISTRY_ADDR"] == "127.0.0.1:5010"
    assert deploy_env["CONTAINER_REGISTRY"] == "127.0.0.1:5010"
    assert deploy_env["REGISTRY"] == "127.0.0.1:5010/"
