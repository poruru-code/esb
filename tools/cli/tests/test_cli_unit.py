from __future__ import annotations

import json
from types import SimpleNamespace

from tools.cli import cli


def test_run_requires_subcommand(capsys) -> None:
    rc = cli.run([])
    assert rc == 1
    captured = capsys.readouterr()
    assert "requires a subcommand" in captured.err


def test_deploy_dispatch_and_warning_output(monkeypatch, capsys) -> None:
    calls: dict[str, object] = {}

    def fake_execute_deploy(input_data):
        calls["input"] = input_data
        return ["warn-a", "warn-b"]

    monkeypatch.setattr(cli, "execute_deploy", fake_execute_deploy)

    rc = cli.run(["deploy", "--artifact", "/tmp/artifact.yml", "--no-cache"])
    assert rc == 0
    deploy_input = calls["input"]
    assert deploy_input.artifact_path == "/tmp/artifact.yml"
    assert deploy_input.no_cache is True
    captured = capsys.readouterr()
    assert "Warning: warn-a" in captured.err
    assert "Warning: warn-b" in captured.err


def test_provision_dispatch_and_compose_file_split(monkeypatch) -> None:
    calls: dict[str, object] = {}

    def fake_execute_provision(input_data):
        calls["input"] = input_data

    monkeypatch.setattr(cli, "execute_provision", fake_execute_provision)

    rc = cli.run(
        [
            "provision",
            "--project",
            "p",
            "--compose-file",
            "a.yml,b.yml",
            "--compose-file",
            "c.yml",
            "--env-file",
            ".env",
            "--project-dir",
            "/tmp/project",
            "--with-deps",
            "-v",
        ]
    )
    assert rc == 0
    provision_input = calls["input"]
    assert provision_input.compose_project == "p"
    assert provision_input.compose_files == ["a.yml", "b.yml", "c.yml"]
    assert provision_input.env_file == ".env"
    assert provision_input.project_dir == "/tmp/project"
    assert provision_input.no_deps is False
    assert provision_input.verbose is True


def test_stack_deploy_dispatch(monkeypatch) -> None:
    calls: dict[str, object] = {}

    def fake_execute_stack_deploy(input_data):
        calls["input"] = input_data

    monkeypatch.setattr(cli, "execute_stack_deploy", fake_execute_stack_deploy)

    rc = cli.run(["stack", "deploy", "--artifact", "/tmp/artifact.yml"])
    assert rc == 0
    stack_input = calls["input"]
    assert stack_input.artifact_path == "/tmp/artifact.yml"


def test_internal_capabilities_output_json(capsys) -> None:
    rc = cli.run(["internal", "capabilities", "--output", "json"])
    assert rc == 0
    payload = json.loads(capsys.readouterr().out.strip())
    assert payload["schema_version"] == 1
    assert payload["contracts"]["maven_shim_ensure_schema_version"] == 1
    assert payload["contracts"]["fixture_image_ensure_schema_version"] == 1


def test_internal_maven_shim_ensure_dispatch(monkeypatch, capsys) -> None:
    def fake_ensure(input_data):
        assert input_data.base_image == "maven:3.9"
        assert input_data.host_registry == "127.0.0.1:5010"
        assert input_data.no_cache is True
        return SimpleNamespace(shim_image="127.0.0.1:5010/esb-maven-shim:abc")

    monkeypatch.setattr(cli, "ensure_maven_shim_image", fake_ensure)

    rc = cli.run(
        [
            "internal",
            "maven-shim",
            "ensure",
            "--base-image",
            "maven:3.9",
            "--host-registry",
            "127.0.0.1:5010",
            "--no-cache",
            "--output",
            "json",
        ]
    )
    assert rc == 0
    payload = json.loads(capsys.readouterr().out.strip())
    assert payload["schema_version"] == 1
    assert payload["shim_image"] == "127.0.0.1:5010/esb-maven-shim:abc"


def test_internal_fixture_image_ensure_dispatch(monkeypatch, capsys) -> None:
    def fake_ensure(input_data):
        assert input_data.artifact_path == "/tmp/artifact.yml"
        assert input_data.no_cache is True
        return SimpleNamespace(
            schema_version=1,
            prepared_images=["127.0.0.1:5010/esb-e2e-image-python:latest"],
        )

    monkeypatch.setattr(cli, "execute_fixture_image_ensure", fake_ensure)

    rc = cli.run(
        [
            "internal",
            "fixture-image",
            "ensure",
            "--artifact",
            "/tmp/artifact.yml",
            "--no-cache",
            "--output",
            "json",
        ]
    )
    assert rc == 0
    payload = json.loads(capsys.readouterr().out.strip())
    assert payload["schema_version"] == 1
    assert payload["prepared_images"] == ["127.0.0.1:5010/esb-e2e-image-python:latest"]
