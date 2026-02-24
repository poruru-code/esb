# Where: e2e/runner/deploy.py
# What: Deployment execution for E2E environments.
# Why: Keep deploy logic separate from lifecycle and test orchestration.
from __future__ import annotations

import json
import re
import threading
from pathlib import Path
from typing import Callable

import yaml

from e2e.runner.logging import LogSink, run_and_stream
from e2e.runner.models import RunContext
from e2e.runner.utils import PROJECT_ROOT

LOCAL_IMAGE_FIXTURE_ROOT = PROJECT_ROOT / "e2e" / "fixtures" / "images" / "lambda"
LOCAL_IMAGE_FIXTURES: dict[str, Path] = {
    "esb-e2e-image-python": LOCAL_IMAGE_FIXTURE_ROOT / "python",
    "esb-e2e-image-java": LOCAL_IMAGE_FIXTURE_ROOT / "java",
}
_PROXY_ENV_ALIASES: tuple[tuple[str, str], ...] = (
    ("HTTP_PROXY", "http_proxy"),
    ("HTTPS_PROXY", "https_proxy"),
    ("NO_PROXY", "no_proxy"),
)
_JAVA_FIXTURE_NAME = "esb-e2e-image-java"
_JAVA_FIXTURE_MAVEN_BASE_IMAGE = (
    "public.ecr.aws/sam/build-java21"
    "@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7"
)
_MAVEN_SHIM_OUTPUT_SCHEMA_VERSION = 1

_prepared_local_fixture_images: set[str] = set()
_prepared_maven_shim_images: dict[tuple[str, str], str] = {}
_prepared_local_fixture_lock = threading.Lock()
_DOCKERFILE_FROM_PATTERN = re.compile(
    r"^FROM(?:\s+--platform=[^\s]+)?\s+(?P<source>[^\s]+)",
    re.IGNORECASE,
)


def _artifactctl_bin(ctx: RunContext) -> str:
    # run_tests.py resolves ARTIFACTCTL_BIN(_RESOLVED) before runner starts.
    resolved = str(ctx.deploy_env.get("ARTIFACTCTL_BIN_RESOLVED", "")).strip()
    if resolved:
        return resolved
    configured = str(ctx.deploy_env.get("ARTIFACTCTL_BIN", "")).strip()
    if configured:
        return configured
    return "artifactctl"


def deploy_artifacts(
    ctx: RunContext,
    *,
    no_cache: bool,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    _prepare_local_fixture_images(ctx, log=log, printer=printer)
    _deploy_via_artifact_driver(
        ctx,
        no_cache=no_cache,
        log=log,
        printer=printer,
    )


def _deploy_via_artifact_driver(
    ctx: RunContext,
    *,
    no_cache: bool,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    manifest_path = _resolve_artifact_manifest_path(ctx)
    if not manifest_path.exists():
        raise FileNotFoundError(f"artifact manifest not found: {manifest_path}")

    config_dir = str(ctx.runtime_env.get("CONFIG_DIR", "")).strip()
    if config_dir == "":
        raise RuntimeError("CONFIG_DIR is required for artifact apply")

    message = f"Deploying artifact manifest for {ctx.scenario.env_name}..."
    log.write_line(message)
    if printer:
        printer(message)
    artifactctl_bin = _artifactctl_bin(ctx)
    deploy_cmd = [
        artifactctl_bin,
        "deploy",
        "--artifact",
        str(manifest_path),
        "--out",
        config_dir,
    ]
    if no_cache:
        deploy_cmd.append("--no-cache")
    secret_env = str(ctx.scenario.extra.get("secret_env_file", "")).strip()
    if secret_env:
        deploy_cmd.extend(["--secret-env", secret_env])
    rc = run_and_stream(
        deploy_cmd,
        cwd=PROJECT_ROOT,
        env=ctx.deploy_env,
        log=log,
        printer=printer,
    )
    if rc != 0:
        raise RuntimeError(f"artifact deploy failed with exit code {rc}")

    provision_cmd = [
        artifactctl_bin,
        "provision",
        "--project",
        ctx.compose_project,
        "--compose-file",
        str(ctx.compose_file),
    ]
    if ctx.env_file:
        provision_cmd.extend(["--env-file", ctx.env_file])
    rc = run_and_stream(
        provision_cmd,
        cwd=PROJECT_ROOT,
        env=ctx.deploy_env,
        log=log,
        printer=printer,
    )
    if rc != 0:
        raise RuntimeError(f"provisioner failed with exit code {rc}")

    log.write_line("Done")
    if printer:
        printer("Done")


def _resolve_artifact_manifest_path(ctx: RunContext) -> Path:
    manifest_value = ctx.scenario.extra.get("artifact_manifest")
    if manifest_value is None:
        raise ValueError(f"artifact_manifest is required for scenario '{ctx.scenario.env_name}'")
    raw = str(manifest_value).strip()
    if raw == "":
        raise ValueError(f"artifact_manifest is required for scenario '{ctx.scenario.env_name}'")
    path = Path(raw)
    if not path.is_absolute():
        path = PROJECT_ROOT / path
    return path.resolve()


def _prepare_local_fixture_images(
    ctx: RunContext,
    *,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    manifest_path = _resolve_artifact_manifest_path(ctx)
    if not manifest_path.exists():
        return
    sources = _collect_local_fixture_image_sources(manifest_path)
    if not sources:
        return

    for source in sources:
        with _prepared_local_fixture_lock:
            if source in _prepared_local_fixture_images:
                continue
            fixture_name = _fixture_repo_name(source)
            fixture_dir = LOCAL_IMAGE_FIXTURES.get(fixture_name)
            if fixture_dir is None:
                raise RuntimeError(f"Unknown local fixture image source: {source}")
            if not fixture_dir.exists():
                raise FileNotFoundError(f"Local fixture image source not found: {fixture_dir}")
            message = f"Preparing local image fixture: {source}"
            log.write_line(message)
            if printer:
                printer(message)

            maven_shim_tag = ""
            if fixture_name == _JAVA_FIXTURE_NAME:
                shim_registry = _image_registry_host(source)
                artifactctl_bin = _artifactctl_bin(ctx)
                maven_shim_tag = _ensure_maven_shim_image(
                    _JAVA_FIXTURE_MAVEN_BASE_IMAGE,
                    registry=shim_registry,
                    artifactctl_bin=artifactctl_bin,
                    env=ctx.deploy_env,
                    log=log,
                    printer=printer,
                )

            build_cmd = [
                "docker",
                "buildx",
                "build",
                "--platform",
                "linux/amd64",
                "--load",
            ]
            build_cmd = _append_proxy_build_args(build_cmd, ctx.deploy_env)
            if maven_shim_tag:
                build_cmd.extend(
                    [
                        "--build-arg",
                        f"MAVEN_IMAGE={maven_shim_tag}",
                    ]
                )
            build_cmd.extend(
                [
                    "--tag",
                    source,
                    str(fixture_dir),
                ]
            )
            rc = run_and_stream(
                build_cmd,
                cwd=PROJECT_ROOT,
                env=ctx.deploy_env,
                log=log,
                printer=printer,
            )
            if rc != 0:
                raise RuntimeError(f"failed to build local fixture image {source} (exit code {rc})")

            push_cmd = ["docker", "push", source]
            rc = run_and_stream(
                push_cmd,
                cwd=PROJECT_ROOT,
                env=ctx.deploy_env,
                log=log,
                printer=printer,
            )
            if rc != 0:
                raise RuntimeError(f"failed to push local fixture image {source} (exit code {rc})")

            _prepared_local_fixture_images.add(source)


def _append_proxy_build_args(cmd: list[str], env: dict[str, str]) -> list[str]:
    for upper, lower in _PROXY_ENV_ALIASES:
        value = env.get(upper, "").strip() or env.get(lower, "").strip()
        if value == "":
            continue
        cmd.extend(["--build-arg", f"{upper}={value}"])
        cmd.extend(["--build-arg", f"{lower}={value}"])
    return cmd


def _image_registry_host(image_ref: str) -> str:
    without_digest = image_ref.split("@", 1)[0].strip()
    if "/" not in without_digest:
        return ""
    candidate = without_digest.split("/", 1)[0].strip()
    if candidate == "":
        return ""
    if candidate == "localhost" or "." in candidate or ":" in candidate:
        return candidate
    return ""


def _ensure_maven_shim_image(
    base_image: str,
    *,
    registry: str,
    artifactctl_bin: str,
    env: dict[str, str],
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> str:
    normalized_registry = registry.strip().rstrip("/")
    cache_key = (base_image, normalized_registry)
    cached = _prepared_maven_shim_images.get(cache_key)
    if cached:
        return cached

    ensure_cmd = [
        artifactctl_bin,
        "internal",
        "maven-shim",
        "ensure",
        "--base-image",
        base_image,
        "--output",
        "json",
    ]
    if normalized_registry:
        ensure_cmd.extend(["--host-registry", normalized_registry])
    output_lines: list[str] = []
    rc = run_and_stream(
        ensure_cmd,
        cwd=PROJECT_ROOT,
        env=env,
        log=log,
        printer=printer,
        on_line=lambda line: output_lines.append(line),
    )
    if rc != 0:
        raise RuntimeError(
            "failed to resolve maven shim image via "
            f"`{artifactctl_bin} internal maven-shim ensure` (exit code {rc}); "
            "ensure artifactctl supports this internal command"
        )
    shim_image = _parse_maven_shim_ensure_output(output_lines)
    _prepared_maven_shim_images[cache_key] = shim_image
    return shim_image


def _parse_maven_shim_ensure_output(lines: list[str]) -> str:
    for line in reversed(lines):
        raw = line.strip()
        if raw == "":
            continue
        try:
            payload = json.loads(raw)
        except json.JSONDecodeError:
            continue
        if not isinstance(payload, dict):
            continue
        if "schema_version" not in payload or "shim_image" not in payload:
            continue
        schema_version = payload.get("schema_version")
        if schema_version != _MAVEN_SHIM_OUTPUT_SCHEMA_VERSION:
            raise RuntimeError(
                "invalid maven shim ensure response schema: "
                f"{schema_version} (expected {_MAVEN_SHIM_OUTPUT_SCHEMA_VERSION})"
            )
        shim_image = str(payload.get("shim_image", "")).strip()
        if shim_image == "":
            raise RuntimeError("maven shim ensure response does not include shim_image")
        return shim_image
    raise RuntimeError("maven shim ensure returned no JSON payload with required fields")


def _collect_local_fixture_image_sources(manifest_path: Path) -> list[str]:
    with manifest_path.open("r", encoding="utf-8") as handle:
        payload = yaml.safe_load(handle) or {}
    if not isinstance(payload, dict):
        raise ValueError(f"artifact manifest must be a map: {manifest_path}")

    artifacts = payload.get("artifacts")
    if artifacts is None:
        return []
    if not isinstance(artifacts, list):
        raise ValueError(f"artifact manifest field 'artifacts' must be a list: {manifest_path}")

    sources: set[str] = set()
    for artifact in artifacts:
        if not isinstance(artifact, dict):
            raise ValueError(f"artifact entries must be maps: {manifest_path}")
        artifact_root_raw = str(artifact.get("artifact_root", "")).strip()
        if artifact_root_raw == "":
            continue
        artifact_root = Path(artifact_root_raw)
        if not artifact_root.is_absolute():
            artifact_root = manifest_path.parent / artifact_root
        artifact_root = artifact_root.resolve()
        if not artifact_root.exists():
            raise FileNotFoundError(f"artifact_root not found: {artifact_root}")
        sources.update(_collect_local_fixture_sources_from_artifact_root(artifact_root))
    return sorted(sources)


def _collect_local_fixture_sources_from_artifact_root(artifact_root: Path) -> set[str]:
    sources: set[str] = set()
    for dockerfile in sorted(artifact_root.rglob("Dockerfile")):
        sources.update(_collect_local_fixture_sources_from_dockerfile(dockerfile))
    return sources


def _collect_local_fixture_sources_from_dockerfile(dockerfile: Path) -> set[str]:
    sources: set[str] = set()
    content = dockerfile.read_text(encoding="utf-8")
    for line in content.splitlines():
        match = _DOCKERFILE_FROM_PATTERN.match(line.strip())
        if match is None:
            continue
        source = match.group("source").strip()
        if _is_local_fixture_image_source(source):
            sources.add(source)
    return sources


def _is_local_fixture_image_source(source: str) -> bool:
    if not source:
        return False
    return _fixture_repo_name(source) in LOCAL_IMAGE_FIXTURES


def _fixture_repo_name(source: str) -> str:
    without_digest = source.split("@", 1)[0]
    last_segment = without_digest.rsplit("/", 1)[-1]
    return last_segment.split(":", 1)[0]
