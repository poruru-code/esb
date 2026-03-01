from __future__ import annotations

import json
import re
from dataclasses import dataclass
from pathlib import Path

from tools.cli import artifact
from tools.cli.common import append_proxy_build_args, run_command
from tools.cli.maven_shim import EnsureInput as MavenShimEnsureInput
from tools.cli.maven_shim import ensure_image as ensure_maven_shim_image

FIXTURE_IMAGE_ENSURE_SCHEMA_VERSION = 1
DEFAULT_FIXTURE_IMAGE_ROOT = "e2e/fixtures/images/lambda"
JAVA_FIXTURE_NAME = "esb-e2e-image-java"
JAVA_FIXTURE_MAVEN_BASE_IMAGE = (
    "public.ecr.aws/sam/build-java21@sha256:"
    "5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7"
)

_LOCAL_FIXTURE_SUBDIRS: dict[str, str] = {
    "esb-e2e-image-python": "python",
    "esb-e2e-image-java": "java",
}
_DOCKERFILE_FROM_PATTERN = re.compile(r"(?i)^FROM(?:\s+--platform=[^\s]+)?\s+([^\s]+)")
_JAVA_FIXTURE_MAVEN_ARG_PATTERN = re.compile(r"(?im)^\s*ARG\s+MAVEN_IMAGE(?:\s*=.*)?\s*$")
_JAVA_FIXTURE_FROM_PATTERN = re.compile(r"(?im)^\s*FROM\s+\$\{?MAVEN_IMAGE\}?\s+AS\s+builder\s*$")


@dataclass(frozen=True)
class FixtureImageEnsureInput:
    artifact_path: str
    no_cache: bool = False
    fixture_root: str = DEFAULT_FIXTURE_IMAGE_ROOT


@dataclass(frozen=True)
class FixtureImageEnsureResult:
    schema_version: int
    prepared_images: list[str]


def execute_fixture_image_ensure(input_data: FixtureImageEnsureInput) -> FixtureImageEnsureResult:
    manifest_path = input_data.artifact_path.strip()
    if manifest_path == "":
        raise RuntimeError("artifact manifest path is empty")
    manifest = artifact.read_artifact_manifest(manifest_path, validate=True)
    sources = collect_local_fixture_image_sources(manifest, manifest_path)
    if not sources:
        return FixtureImageEnsureResult(
            schema_version=FIXTURE_IMAGE_ENSURE_SCHEMA_VERSION,
            prepared_images=[],
        )

    fixture_root = input_data.fixture_root.strip() or DEFAULT_FIXTURE_IMAGE_ROOT
    fixture_root_abs = Path(fixture_root).resolve()
    prepared_images: list[str] = []
    resolved_shim_by_registry: dict[str, str] = {}

    for source in sources:
        fixture_name = fixture_repo_name(source)
        if fixture_name not in _LOCAL_FIXTURE_SUBDIRS:
            raise RuntimeError(f"unknown local fixture image source: {source}")
        fixture_dir = fixture_root_abs / _LOCAL_FIXTURE_SUBDIRS[fixture_name]
        if not fixture_dir.exists():
            raise RuntimeError(f"local fixture image source not found: {fixture_dir}")
        if not fixture_dir.is_dir():
            raise RuntimeError(f"local fixture image source is not a directory: {fixture_dir}")

        build_args: dict[str, str] = {}
        if fixture_name == JAVA_FIXTURE_NAME:
            assert_java_fixture_uses_maven_shim_contract(fixture_dir / "Dockerfile")
            registry_host = image_registry_host(source)
            cache_key = f"{JAVA_FIXTURE_MAVEN_BASE_IMAGE}|{registry_host}"
            shim_image = resolved_shim_by_registry.get(cache_key)
            if shim_image is None:
                shim_result = ensure_maven_shim_image(
                    MavenShimEnsureInput(
                        base_image=JAVA_FIXTURE_MAVEN_BASE_IMAGE,
                        host_registry=registry_host,
                        no_cache=input_data.no_cache,
                    )
                )
                shim_image = shim_result.shim_image.strip()
                if shim_image == "":
                    raise RuntimeError("maven shim ensure returned empty shim image")
                resolved_shim_by_registry[cache_key] = shim_image
            build_args["MAVEN_IMAGE"] = shim_image

        run_command(
            buildx_build_command_for_fixture(
                tag=source,
                context_dir=fixture_dir,
                no_cache=input_data.no_cache,
                build_args=build_args,
            ),
            check=True,
        )
        run_command(["docker", "push", source], check=True)
        prepared_images.append(source)

    return FixtureImageEnsureResult(
        schema_version=FIXTURE_IMAGE_ENSURE_SCHEMA_VERSION,
        prepared_images=prepared_images,
    )


def buildx_build_command_for_fixture(
    *,
    tag: str,
    context_dir: Path,
    no_cache: bool,
    build_args: dict[str, str],
) -> list[str]:
    cmd = ["docker", "buildx", "build", "--platform", "linux/amd64", "--load"]
    if no_cache:
        cmd.append("--no-cache")
    cmd = append_proxy_build_args(cmd)
    for key in sorted(build_args.keys()):
        value = build_args[key].strip()
        if value == "":
            continue
        cmd.extend(["--build-arg", f"{key}={value}"])
    cmd.extend(["--tag", tag, str(context_dir)])
    return cmd


def collect_local_fixture_image_sources(
    manifest: artifact.ArtifactManifest,
    manifest_path: str,
) -> list[str]:
    sources: set[str] = set()
    for index in range(len(manifest.artifacts)):
        artifact_root = Path(manifest.resolve_artifact_root(manifest_path, index))
        for dockerfile in artifact_root.rglob("Dockerfile"):
            lines = dockerfile.read_text(encoding="utf-8").splitlines()
            for line in lines:
                trimmed = line.strip()
                match = _DOCKERFILE_FROM_PATTERN.match(trimmed)
                if not match:
                    continue
                source = match.group(1).strip()
                if is_local_fixture_image_source(source):
                    sources.add(source)
    return sorted(sources)


def is_local_fixture_image_source(source: str) -> bool:
    if source.strip() == "":
        return False
    return fixture_repo_name(source) in _LOCAL_FIXTURE_SUBDIRS


def fixture_repo_name(source: str) -> str:
    without_digest = source.strip().split("@", 1)[0]
    last_segment = without_digest.rsplit("/", 1)[-1]
    return last_segment.split(":", 1)[0]


def image_registry_host(image_ref: str) -> str:
    without_digest = image_ref.strip().split("@", 1)[0]
    if "/" not in without_digest:
        return ""
    candidate = without_digest.split("/", 1)[0]
    if candidate == "" or candidate == "localhost":
        return candidate
    if "." in candidate or ":" in candidate:
        return candidate
    return ""


def assert_java_fixture_uses_maven_shim_contract(dockerfile: Path) -> None:
    try:
        text = dockerfile.read_text(encoding="utf-8")
    except FileNotFoundError as exc:
        raise RuntimeError(f"java fixture Dockerfile not found: {dockerfile}") from exc
    if _JAVA_FIXTURE_MAVEN_ARG_PATTERN.search(text) is None:
        raise RuntimeError(
            "java fixture Dockerfile must define `ARG MAVEN_IMAGE` so E2E can inject maven-shim: "
            f"{dockerfile}"
        )
    if _JAVA_FIXTURE_FROM_PATTERN.search(text) is None:
        raise RuntimeError(
            "java fixture Dockerfile must use "
            "`FROM ${MAVEN_IMAGE} AS builder` for proxy-safe Maven "
            f"resolution: {dockerfile}"
        )


def fixture_image_ensure_result_to_json(result: FixtureImageEnsureResult) -> str:
    return json.dumps(
        {
            "schema_version": result.schema_version,
            "prepared_images": result.prepared_images,
        }
    )
