# Where: e2e/runner/tests/test_warmup_command.py
# What: Unit tests for Java warmup Maven proxy contract.
# Why: Keep Go/Python proxy settings behavior aligned via shared test vectors.
from __future__ import annotations

import json
from pathlib import Path

from e2e.runner import constants, warmup
from e2e.runner.models import Scenario

_EXPECTED_JAVA_BUILD_IMAGE = "public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7"
_CASES_PATH = (
    Path(__file__).resolve().parents[3] / "runtime-hooks/java/testdata/maven_proxy_cases.json"
)
_PROXY_ENV_KEYS = (
    "HTTP_PROXY",
    "http_proxy",
    "HTTPS_PROXY",
    "https_proxy",
    "NO_PROXY",
    "no_proxy",
    "MAVEN_OPTS",
    "JAVA_TOOL_OPTIONS",
)


def _load_cases() -> list[dict[str, object]]:
    return json.loads(_CASES_PATH.read_text(encoding="utf-8"))


def _apply_case_env(monkeypatch, env: dict[str, str]) -> None:
    for key in _PROXY_ENV_KEYS:
        monkeypatch.delenv(key, raising=False)
    for key, value in env.items():
        monkeypatch.setenv(key, value)


def test_render_maven_settings_matches_shared_cases(monkeypatch) -> None:
    for case in _load_cases():
        expected_xml = case.get("expected_xml")
        if expected_xml is None:
            continue
        _apply_case_env(monkeypatch, case["env"])
        rendered = warmup._render_maven_settings_xml()
        assert rendered == expected_xml, case["name"]


def test_render_maven_settings_rejects_invalid_shared_cases(monkeypatch) -> None:
    for case in _load_cases():
        expected_error = case.get("expected_error_substring")
        if expected_error is None:
            continue
        _apply_case_env(monkeypatch, case["env"])
        try:
            warmup._render_maven_settings_xml()
        except ValueError as exc:
            assert str(expected_error) in str(exc), case["name"]
        else:
            raise AssertionError(f"expected ValueError for case {case['name']}")


def test_docker_maven_command_always_uses_settings_mount_and_settings_only_proxy(
    monkeypatch, tmp_path
):
    project_dir = tmp_path / "java-fixture"
    project_dir.mkdir()

    settings_path = tmp_path / "generated-settings.xml"
    settings_path.write_text("<settings/>\n", encoding="utf-8")
    monkeypatch.setenv("HTTP_PROXY", "http://proxy.example:8080")
    monkeypatch.setenv("NO_PROXY", "localhost,127.0.0.1")

    cmd = warmup._docker_maven_command(project_dir, settings_path)
    joined = " ".join(str(item) for item in cmd)

    assert f"{settings_path}:{warmup.M2_SETTINGS_PATH}:ro" in joined
    assert f":{warmup.M2_REPOSITORY_PATH}" in joined
    assert (
        f"mvn -s {warmup.M2_SETTINGS_PATH} -q -Dmaven.repo.local={warmup.M2_REPOSITORY_PATH} "
        "-Dmaven.artifact.threads=1 -DskipTests package" in cmd[-1]
    )
    assert "if [ -f /tmp/m2/settings.xml ]" not in cmd[-1]
    assert "else mvn" not in cmd[-1]
    assert "HTTP_PROXY=" in joined
    assert "http_proxy=" in joined
    assert "HTTPS_PROXY=" in joined
    assert "https_proxy=" in joined
    assert "NO_PROXY=" in joined
    assert "no_proxy=" in joined
    assert "HTTP_PROXY=http://proxy.example:8080" not in joined
    assert warmup.JAVA_BUILD_IMAGE == _EXPECTED_JAVA_BUILD_IMAGE
    assert warmup.JAVA_BUILD_IMAGE in joined


def test_write_temp_maven_settings_uses_case_payload(monkeypatch):
    first_case = _load_cases()[0]
    _apply_case_env(monkeypatch, first_case["env"])

    settings_path = warmup._write_temp_maven_settings()
    try:
        content = settings_path.read_text(encoding="utf-8")
    finally:
        settings_path.unlink(missing_ok=True)

    assert content == first_case["expected_xml"]


def test_java_build_image_is_digest_pinned() -> None:
    assert warmup.JAVA_BUILD_IMAGE == _EXPECTED_JAVA_BUILD_IMAGE
    assert ":latest" not in warmup.JAVA_BUILD_IMAGE
    assert "@sha256:" in warmup.JAVA_BUILD_IMAGE


def test_discover_java_fixture_projects_includes_tools_fixture(monkeypatch, tmp_path):
    java_root = tmp_path / "e2e" / "fixtures" / "functions" / "java"
    tool_root = tmp_path / "tools" / "e2e-lambda-fixtures" / "java"
    (java_root / "echo").mkdir(parents=True)
    (java_root / "echo" / "pom.xml").write_text("<project/>", encoding="utf-8")
    tool_root.mkdir(parents=True)
    (tool_root / "pom.xml").write_text("<project/>", encoding="utf-8")

    monkeypatch.setattr(warmup, "JAVA_FIXTURE_ROOTS", (java_root, tool_root))

    projects = warmup._discover_java_fixture_projects()

    assert projects == [(java_root / "echo").resolve(), tool_root.resolve()]


def _scenario(name: str, env_name: str) -> Scenario:
    return Scenario(
        name=name,
        env_name=env_name,
        mode="docker",
        env_file=None,
        env_dir=None,
        env_vars={},
        targets=[],
        exclude=[],
        deploy_templates=[],
        project_name="esb",
    )


def test_ensure_buildx_builders_dedupes_same_signature(monkeypatch) -> None:
    scenarios = {
        "a": _scenario("a", "e2e-a"),
        "b": _scenario("b", "e2e-b"),
    }
    runtime_env = {
        "BUILDX_BUILDER": "esb-buildx",
        constants.ENV_BUILDKITD_CONFIG: "/tmp/buildkitd.toml",
        "HTTP_PROXY": "http://proxy.example:8080",
        "http_proxy": "http://proxy.example:8080",
    }
    calls: list[tuple[str, str | None, str | None]] = []

    monkeypatch.setattr(
        warmup,
        "_scenario_runtime_env_for_buildx",
        lambda *_args, **_kwargs: dict(runtime_env),
    )
    monkeypatch.setattr(
        warmup,
        "ensure_buildx_builder",
        lambda builder_name, network_mode="host", config_path=None, proxy_source=None: calls.append(
            (
                builder_name,
                config_path,
                (proxy_source or {}).get("HTTP_PROXY"),
            )
        ),
    )

    warmup._ensure_buildx_builders(scenarios)

    assert calls == [("esb-buildx", "/tmp/buildkitd.toml", "http://proxy.example:8080")]


def test_ensure_buildx_builders_calls_when_signature_differs(monkeypatch) -> None:
    scenarios = {
        "a": _scenario("a", "e2e-a"),
        "b": _scenario("b", "e2e-b"),
    }
    envs = {
        "a": {
            "BUILDX_BUILDER": "esb-buildx",
            constants.ENV_BUILDKITD_CONFIG: "/tmp/buildkitd.toml",
            "HTTP_PROXY": "http://proxy-a.example:8080",
        },
        "b": {
            "BUILDX_BUILDER": "esb-buildx",
            constants.ENV_BUILDKITD_CONFIG: "/tmp/buildkitd.toml",
            "HTTP_PROXY": "http://proxy-b.example:8080",
        },
    }
    calls: list[str] = []

    monkeypatch.setattr(
        warmup,
        "_scenario_runtime_env_for_buildx",
        lambda scenario: dict(envs[scenario.name]),
    )
    monkeypatch.setattr(
        warmup,
        "ensure_buildx_builder",
        lambda builder_name, network_mode="host", config_path=None, proxy_source=None: calls.append(
            builder_name
        ),
    )

    warmup._ensure_buildx_builders(scenarios)

    assert calls == ["esb-buildx", "esb-buildx"]
