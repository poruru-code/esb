from __future__ import annotations

import pytest

from tools.cli import deploy_ops


def test_rewrite_registry_alias_rewrites_known_alias() -> None:
    rewritten, changed = deploy_ops.rewrite_registry_alias(
        "127.0.0.1:5010/esb-lambda-echo:latest",
        "registry.example:5000",
        ["127.0.0.1:5010", "registry:5010"],
    )
    assert changed is True
    assert rewritten == "registry.example:5000/esb-lambda-echo:latest"


def test_normalize_function_image_ref_for_runtime(monkeypatch) -> None:
    monkeypatch.setenv("CONTAINER_REGISTRY", "registry.example:5000")
    monkeypatch.delenv("HOST_REGISTRY_ADDR", raising=False)
    monkeypatch.delenv("REGISTRY", raising=False)

    rewritten, changed = deploy_ops.normalize_function_image_ref_for_runtime(
        "127.0.0.1:5010/esb-lambda-dynamo:e2e"
    )
    assert changed is True
    assert rewritten == "registry.example:5000/esb-lambda-dynamo:e2e"

    untouched, changed_untouched = deploy_ops.normalize_function_image_ref_for_runtime(
        "127.0.0.1:5010/custom-image:e2e"
    )
    assert changed_untouched is False
    assert untouched == "127.0.0.1:5010/custom-image:e2e"


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
