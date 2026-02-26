from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from tools.deployops.core.bundle_ops import BundleOptions, execute_bundle_dind
from tools.deployops.core.runner import CompletedCommand


@dataclass
class FakeRunner:
    dry_run: bool = True

    def __post_init__(self) -> None:
        self.commands: list[tuple[list[str], bool]] = []
        self.messages: list[str] = []

    def emit(self, message: str) -> None:
        self.messages.append(message)

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
        del check, stream_output, on_line, cwd, env
        command = [str(token) for token in cmd]
        self.commands.append((command, run_in_dry_run))

        if self.dry_run and not run_in_dry_run:
            return CompletedCommand(tuple(command), 0, "", "")

        if capture_output and "--images" in command:
            return CompletedCommand(tuple(command), 0, "repo/service:latest\n", "")

        return CompletedCommand(tuple(command), 0, "", "")


def test_execute_bundle_dind_dry_run_uses_compose_image_query(
    monkeypatch,
    tmp_path: Path,
    capsys,
) -> None:
    monkeypatch.chdir(tmp_path)

    (tmp_path / "tools/deployops/assets/dind").mkdir(parents=True)
    (tmp_path / "tools/deployops/assets/dind/Dockerfile").write_text(
        "FROM scratch\n",
        encoding="utf-8",
    )
    (tmp_path / "tools/deployops/assets/dind/entrypoint.sh").write_text(
        "#!/bin/sh\n",
        encoding="utf-8",
    )

    artifact_dir = tmp_path / "artifacts/demo"
    runtime_dir = artifact_dir / "entry/config"
    runtime_dir.mkdir(parents=True)
    (runtime_dir / "functions.yml").write_text(
        "functions:\n  hello:\n    image: local/hello:latest\n",
        encoding="utf-8",
    )
    (runtime_dir / "routing.yml").write_text("routes: []\n", encoding="utf-8")

    (artifact_dir / "artifact.yml").write_text(
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

    env_dir = tmp_path / "env"
    env_dir.mkdir(parents=True)
    env_file = env_dir / ".env"
    env_file.write_text("ENV=dev\n", encoding="utf-8")
    (env_dir / "docker-compose.yml").write_text("services: {}\n", encoding="utf-8")

    runner = FakeRunner(dry_run=True)
    options = BundleOptions(
        artifact_dirs=[str(artifact_dir)],
        env_file=str(env_file),
        compose_file=None,
        prepare_images=False,
        output_tag=None,
        positional_tag=None,
        build_dir="tools/deployops/.build/dind",
    )

    rc = execute_bundle_dind(options, runner)

    assert rc == 0
    assert any(run_in_dry_run for _, run_in_dry_run in runner.commands)

    output = capsys.readouterr().out
    assert "COMPOSE_IMAGES=repo/service:latest" in output
    assert "ALL_IMAGES=" in output
