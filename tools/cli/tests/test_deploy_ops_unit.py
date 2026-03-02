from __future__ import annotations

import pytest

from tools.cli import deploy_ops

LAMBDA_PREFIX = deploy_ops.DEFAULT_LAMBDA_BASE_REPO.removesuffix("-base")


def test_rewrite_registry_alias_rewrites_known_alias() -> None:
    rewritten, changed = deploy_ops.rewrite_registry_alias(
        f"127.0.0.1:5010/{LAMBDA_PREFIX}-echo:latest",
        "registry.example:5000",
        ["127.0.0.1:5010", "registry:5010"],
    )
    assert changed is True
    assert rewritten == f"registry.example:5000/{LAMBDA_PREFIX}-echo:latest"


def test_normalize_function_image_ref_for_runtime(monkeypatch) -> None:
    monkeypatch.setenv("CONTAINER_REGISTRY", "registry.example:5000")
    monkeypatch.delenv("HOST_REGISTRY_ADDR", raising=False)
    monkeypatch.delenv("REGISTRY", raising=False)

    rewritten, changed = deploy_ops.normalize_function_image_ref_for_runtime(
        f"127.0.0.1:5010/{LAMBDA_PREFIX}-dynamo:e2e"
    )
    assert changed is True
    assert rewritten == f"registry.example:5000/{LAMBDA_PREFIX}-dynamo:e2e"

    rewritten_custom, changed_custom = deploy_ops.normalize_function_image_ref_for_runtime(
        "127.0.0.1:5010/custom-image:e2e"
    )
    assert changed_custom is True
    assert rewritten_custom == "registry.example:5000/custom-image:e2e"

    untouched, changed_untouched = deploy_ops.normalize_function_image_ref_for_runtime(
        "public.ecr.aws/lambda/python:3.12"
    )
    assert changed_untouched is False
    assert untouched == "public.ecr.aws/lambda/python:3.12"


def test_normalize_function_image_ref_for_runtime_handles_non_default_brand(monkeypatch) -> None:
    monkeypatch.setenv("CONTAINER_REGISTRY", "registry.example:5000")
    monkeypatch.delenv("HOST_REGISTRY_ADDR", raising=False)
    monkeypatch.delenv("REGISTRY", raising=False)

    rewritten, changed = deploy_ops.normalize_function_image_ref_for_runtime(
        "127.0.0.1:5010/padma-lambda-echo:e2e"
    )
    assert changed is True
    assert rewritten == "registry.example:5000/padma-lambda-echo:e2e"


def test_is_managed_lambda_base_ref_matches_only_default_repo() -> None:
    assert (
        deploy_ops.is_managed_lambda_base_ref(
            f"127.0.0.1:5010/{deploy_ops.DEFAULT_LAMBDA_BASE_REPO}:latest"
        )
        is True
    )
    assert deploy_ops.is_managed_lambda_base_ref("127.0.0.1:5010/padma-lambda-base:latest") is False
    assert deploy_ops.is_managed_lambda_base_ref("127.0.0.1:5010/custom-image:latest") is False


def test_read_lambda_base_ref_ignores_non_default_repo(tmp_path) -> None:
    dockerfile = tmp_path / "Dockerfile"
    dockerfile.write_text("FROM 127.0.0.1:5010/padma-lambda-base:latest\n", encoding="utf-8")
    base_ref, ok = deploy_ops.read_lambda_base_ref(str(dockerfile))
    assert ok is False
    assert base_ref == ""


def test_ensure_lambda_base_image_rejects_runtime_hooks_fallback_for_non_default_repo(
    monkeypatch,
) -> None:
    class _Result:
        returncode = 1

    monkeypatch.setattr(deploy_ops, "docker_image_exists", lambda _image_ref: False)

    def fake_run_command(cmd, **kwargs):  # noqa: ANN003
        assert cmd[0:2] == ["docker", "pull"]
        return _Result()

    monkeypatch.setattr(deploy_ops, "run_command", fake_run_command)

    with pytest.raises(RuntimeError, match="runtime-hooks fallback is supported only for"):
        deploy_ops.ensure_lambda_base_image(
            "127.0.0.1:5010/padma-lambda-base:latest",
            no_cache=False,
            built_base_images=set(),
        )


def test_rewrite_dockerfile_for_maven_shim_success() -> None:
    source = "\n".join(
        (
            "FROM maven:3.9 AS builder",
            "RUN mvn -B -q test",
            "FROM public.ecr.aws/lambda/java:21",
        )
    )
    rewritten, changed = deploy_ops.rewrite_dockerfile_for_maven_shim(
        source,
        lambda base_ref: f"shim-for-{base_ref}",
    )
    assert changed is True
    assert "FROM shim-for-maven:3.9 AS builder" in rewritten


def test_rewrite_dockerfile_for_maven_shim_raises_when_maven_run_without_maven_base() -> None:
    source = "\n".join(
        (
            "FROM public.ecr.aws/lambda/java:21 AS builder",
            "RUN mvn -B -q test",
        )
    )
    with pytest.raises(RuntimeError, match="maven run command detected"):
        deploy_ops.rewrite_dockerfile_for_maven_shim(source, lambda _: "unused")


def test_parse_layer_context_aliases_extracts_only_supported_copy_forms() -> None:
    dockerfile = "\n".join(
        (
            "FROM alpine AS layer_1_common",
            "COPY --from=layer_1_common / /opt",
            "COPY --from=layer_2_libs / /opt/",
            "COPY --from=builder / /opt",
            "COPY --from=layer_2_libs /tmp /opt",
        )
    )
    aliases = deploy_ops.parse_layer_context_aliases(dockerfile)
    assert aliases == ["layer_1_common", "layer_2_libs"]


def test_is_python_layer_layout_required_detects_pythonpath() -> None:
    dockerfile = "\n".join(
        (
            "FROM python:3.12",
            "ENV PYTHONPATH=/opt/python:/var/task",
        )
    )
    assert deploy_ops.is_python_layer_layout_required(dockerfile) is True


def test_compose_base_args_includes_expected_flags() -> None:
    req = deploy_ops.ProvisionInput(
        compose_project="demo",
        compose_files=["docker-compose.yml", "docker-compose.extra.yml"],
        env_file=".env",
        no_warn_orphans=True,
    )
    args = deploy_ops.compose_base_args(req)
    assert args == [
        "compose",
        "-f",
        "docker-compose.yml",
        "-f",
        "docker-compose.extra.yml",
        "--no-warn-orphans",
        "-p",
        "demo",
        "--env-file",
        ".env",
    ]
