from __future__ import annotations

import hashlib
import json
import os
import re
import shutil
import tempfile
import zipfile
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

from tools.cli import artifact
from tools.cli.branding_constants_gen import DEFAULT_BRAND_HOME_DIR, DEFAULT_CTL_BIN
from tools.cli.common import (
    append_proxy_build_args,
    docker_image_exists,
    run_command,
    sorted_unique_non_empty,
)
from tools.cli.maven_shim import EnsureInput as MavenShimEnsureInput
from tools.cli.maven_shim import ensure_image as ensure_maven_shim_image

_MAVEN_RUN_COMMAND_PATTERN = re.compile(r"(^|&&|\|\||;)[\s]*mvn([\s]|$)")
_MAVEN_WRAPPER_RUN_COMMAND_PATTERN = re.compile(r"(^|&&|\|\||;)[\s]*\./mvnw([\s]|$)")
_LAYER_CACHE_SCHEMA_VERSION = "v1"
_MAX_LAYER_EXTRACT_BYTES = 1 << 30
_RUNTIME_CONFIG_MOUNT_DESTINATION = "/app/runtime-config"


@dataclass(frozen=True)
class RuntimeConfigTarget:
    bind_path: str = ""
    volume_name: str = ""

    def normalized(self) -> RuntimeConfigTarget:
        return RuntimeConfigTarget(
            bind_path=self.bind_path.strip(),
            volume_name=self.volume_name.strip(),
        )

    def is_empty(self) -> bool:
        normalized = self.normalized()
        return normalized.bind_path == "" and normalized.volume_name == ""


@dataclass(frozen=True)
class DeployInput:
    artifact_path: str
    runtime_config_target: RuntimeConfigTarget = RuntimeConfigTarget()
    no_cache: bool = False
    staging_root_dir: str = ""


@dataclass(frozen=True)
class PrepareImagesResult:
    published_function_images: set[str]


@dataclass(frozen=True)
class ImageBuildTarget:
    function_name: str
    image_ref: str
    dockerfile: str


@dataclass(frozen=True)
class ProvisionInput:
    compose_project: str
    compose_files: list[str]
    env_file: str = ""
    project_dir: str = ""
    no_deps: bool = True
    verbose: bool = False
    no_warn_orphans: bool = False
    provisioner_name: str = "provisioner"


def default_ctl_command_name() -> str:
    return DEFAULT_CTL_BIN


def execute_deploy(input_data: DeployInput) -> list[str]:
    normalized = normalize_input(input_data)
    prepare_result = prepare_images_with_result(
        artifact_path=normalized.artifact_path,
        no_cache=normalized.no_cache,
        ensure_base=True,
    )
    staging_dir = create_staging_dir(normalized.staging_root_dir)
    try:
        warnings = artifact.execute_apply(normalized.artifact_path, staging_dir)
        normalize_output_function_images(staging_dir, prepare_result.published_function_images)
        sync_runtime_config(staging_dir, normalized.runtime_config_target)
        return list(warnings)
    finally:
        shutil.rmtree(staging_dir, ignore_errors=True)


def normalize_input(input_data: DeployInput) -> DeployInput:
    artifact_path = input_data.artifact_path.strip()
    if artifact_path == "":
        raise RuntimeError("artifact path is required")

    runtime_config_target = input_data.runtime_config_target.normalized()
    if runtime_config_target.is_empty():
        runtime_config_target = resolve_runtime_config_target()
    if runtime_config_target.is_empty():
        raise RuntimeError("runtime-config target is required")

    return DeployInput(
        artifact_path=artifact_path,
        runtime_config_target=runtime_config_target,
        no_cache=input_data.no_cache,
        staging_root_dir=input_data.staging_root_dir.strip(),
    )


def create_staging_dir(root: str) -> str:
    normalized = root.strip()
    if normalized != "":
        Path(normalized).mkdir(parents=True, exist_ok=True)
    return tempfile.mkdtemp(prefix="artifact-runtime-config-", dir=(normalized or None))


def resolve_runtime_config_target() -> RuntimeConfigTarget:
    project_name = os.environ.get("PROJECT_NAME", "").strip()
    if project_name != "":
        return _resolve_runtime_config_target_with_project(project_name)
    return _resolve_runtime_config_target_without_project()


def _resolve_runtime_config_target_with_project(project_name: str) -> RuntimeConfigTarget:
    for service in ("gateway", "provisioner"):
        container_ids = _list_compose_containers(
            f"label=com.docker.compose.project={project_name}",
            f"label=com.docker.compose.service={service}",
        )
        try:
            return _resolve_runtime_config_target_from_containers(container_ids)
        except RuntimeError as exc:
            if "runtime-config mount not found" in str(exc):
                continue
            raise
    raise RuntimeError(
        "runtime-config mount was not found for compose project "
        f'"{project_name}"; ensure stack is running'
    )


def _resolve_runtime_config_target_without_project() -> RuntimeConfigTarget:
    for service in ("gateway", "provisioner"):
        container_ids = _list_compose_containers(f"label=com.docker.compose.service={service}")
        if not container_ids:
            continue
        if len(container_ids) > 1:
            raise RuntimeError(
                f"multiple running {service} containers detected; set PROJECT_NAME to disambiguate"
            )
        try:
            return _resolve_runtime_config_target_from_containers(container_ids)
        except RuntimeError as exc:
            if "runtime-config mount not found" in str(exc):
                continue
            raise
    raise RuntimeError("no running gateway/provisioner compose container found")


def _list_compose_containers(*filters: str) -> list[str]:
    cmd = ["docker", "ps", "-q"]
    for item in filters:
        normalized = item.strip()
        if normalized == "":
            continue
        cmd.extend(["--filter", normalized])
    result = run_command(cmd, capture_output=True, check=True)
    return [line.strip() for line in (result.stdout or "").splitlines() if line.strip() != ""]


def _resolve_runtime_config_target_from_containers(container_ids: list[str]) -> RuntimeConfigTarget:
    if not container_ids:
        raise RuntimeError("runtime-config mount not found")
    for container_id in container_ids:
        try:
            return _inspect_runtime_config_target(container_id)
        except RuntimeError as exc:
            if "runtime-config mount not found" in str(exc):
                continue
            raise
    raise RuntimeError("runtime-config mount not found")


def _inspect_runtime_config_target(container_id: str) -> RuntimeConfigTarget:
    normalized = container_id.strip()
    if normalized == "":
        raise RuntimeError("runtime-config mount not found")
    result = run_command(["docker", "inspect", normalized], capture_output=True, check=True)
    payload = json.loads(result.stdout or "[]")
    if not isinstance(payload, list) or not payload:
        raise RuntimeError(f"docker inspect returned no containers for {normalized}")
    mounts = payload[0].get("Mounts", [])
    if not isinstance(mounts, list):
        raise RuntimeError(f"docker inspect mount payload is invalid for {normalized}")
    for mount in mounts:
        if not isinstance(mount, dict):
            continue
        if str(mount.get("Destination", "")).strip() != _RUNTIME_CONFIG_MOUNT_DESTINATION:
            continue
        mount_type = str(mount.get("Type", "")).strip().lower()
        if mount_type == "volume":
            name = str(mount.get("Name", "")).strip()
            if name != "":
                return RuntimeConfigTarget(volume_name=name)
        source = str(mount.get("Source", "")).strip()
        if source == "":
            raise RuntimeError(f"runtime-config mount source is empty for container {normalized}")
        return RuntimeConfigTarget(bind_path=source)
    raise RuntimeError("runtime-config mount not found")


def sync_runtime_config(staging_dir: str, target: RuntimeConfigTarget) -> None:
    staging = staging_dir.strip()
    normalized_target = target.normalized()
    if staging == "":
        raise RuntimeError("staging runtime-config directory is required")
    if normalized_target.is_empty():
        raise RuntimeError("runtime-config target is required")
    if normalized_target.bind_path != "":
        sync_runtime_config_to_bind_path(staging, normalized_target.bind_path)
        return
    sync_runtime_config_to_volume(staging, normalized_target.volume_name)


def sync_runtime_config_to_bind_path(staging_dir: str, runtime_config_dir: str) -> None:
    target = runtime_config_dir.strip()
    if target == "":
        raise RuntimeError("runtime-config target is required")
    clear_directory(target)
    copy_directory(staging_dir, target)


def sync_runtime_config_to_volume(staging_dir: str, volume_name: str) -> None:
    volume = volume_name.strip()
    if volume == "":
        raise RuntimeError("runtime-config target is required")
    absolute_staging = str(Path(staging_dir).resolve())
    script = " && ".join(
        (
            "set -eu",
            "mkdir -p /runtime-config",
            "rm -rf /runtime-config/* /runtime-config/.[!.]* "
            "/runtime-config/..?* 2>/dev/null || true",
            "cp -a /src/. /runtime-config/",
        )
    )
    run_command(
        [
            "docker",
            "run",
            "--rm",
            "-v",
            f"{volume}:/runtime-config",
            "-v",
            f"{absolute_staging}:/src:ro",
            "alpine:3.20",
            "sh",
            "-c",
            script,
        ],
        check=True,
    )


def clear_directory(path: str) -> None:
    directory = Path(path)
    directory.mkdir(parents=True, exist_ok=True)
    for entry in directory.iterdir():
        if entry.is_dir() and not entry.is_symlink():
            shutil.rmtree(entry)
        else:
            entry.unlink(missing_ok=True)


def copy_directory(src_dir: str, dest_dir: str) -> None:
    src_path = Path(src_dir)
    if not src_path.exists():
        raise RuntimeError(f"staging runtime-config directory does not exist: {src_dir}")
    for current in src_path.rglob("*"):
        rel = current.relative_to(src_path)
        target = Path(dest_dir) / rel
        if current.is_symlink():
            raise RuntimeError(f"runtime-config sync does not support symlink: {current}")
        if current.is_dir():
            target.mkdir(parents=True, exist_ok=True)
            continue
        if not current.is_file():
            raise RuntimeError(f"runtime-config sync supports only regular files: {current}")
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(current, target)
        target.chmod(0o644)


def prepare_images_with_result(
    *,
    artifact_path: str,
    no_cache: bool,
    ensure_base: bool,
) -> PrepareImagesResult:
    manifest_path = artifact_path.strip()
    if manifest_path == "":
        raise RuntimeError("artifact path is required")
    manifest = artifact.read_artifact_manifest(manifest_path, validate=True)

    built_function_images: set[str] = set()
    built_base_images: set[str] = set()
    published_function_images: set[str] = set()
    resolved_maven_shim_images: dict[str, str] = {}
    has_function_build_targets = False

    for index in range(len(manifest.artifacts)):
        artifact_root = manifest.resolve_artifact_root(manifest_path, index)
        repo_root = resolve_repo_root(manifest_path, artifact_root)
        runtime_config_dir = manifest.resolve_runtime_config_dir(manifest_path, index)
        functions_path = Path(runtime_config_dir) / "functions.yml"
        functions_payload, ok = _load_yaml_map(str(functions_path))
        if not ok:
            raise RuntimeError(f"functions config not found: {functions_path}")
        functions_raw = functions_payload.get("functions")
        if not isinstance(functions_raw, dict):
            raise RuntimeError(f"functions must be map in {functions_path}")

        build_targets = collect_image_build_targets(
            artifact_root=artifact_root,
            functions_raw=functions_raw,
            built_function_images=built_function_images,
        )
        if not build_targets:
            continue
        has_function_build_targets = True
        function_names = [target.function_name for target in build_targets]

        def _run_builds(
            context_root: str,
            *,
            _build_targets: list[ImageBuildTarget] = build_targets,
            _repo_root: str = repo_root,
        ) -> None:
            for target in _build_targets:
                build_and_push_function_image(
                    image_ref=target.image_ref,
                    function_name=target.function_name,
                    repo_root=_repo_root,
                    context_root=context_root,
                    no_cache=no_cache,
                    ensure_base=ensure_base,
                    built_base_images=built_base_images,
                    resolved_maven_shim_images=resolved_maven_shim_images,
                )
                built_function_images.add(target.image_ref)
                published_function_images.add(target.image_ref)

        with_function_build_workspace(artifact_root, function_names, _run_builds)

    if ensure_base and not has_function_build_targets:
        base_ref = resolve_default_lambda_base_ref()
        ensure_lambda_base_image(base_ref, no_cache, built_base_images)

    return PrepareImagesResult(published_function_images=published_function_images)


def collect_image_build_targets(
    *,
    artifact_root: str,
    functions_raw: dict[Any, Any],
    built_function_images: set[str],
) -> list[ImageBuildTarget]:
    targets: list[ImageBuildTarget] = []
    names = sorted(str(name) for name in functions_raw.keys())
    for function_name in names:
        payload = functions_raw.get(function_name)
        if not isinstance(payload, dict):
            continue
        raw_image = payload.get("image")
        if raw_image is None:
            continue
        image_ref = str(raw_image).strip()
        if image_ref == "":
            continue
        normalized_image_ref, _ = normalize_function_image_ref_for_runtime(image_ref)
        if normalized_image_ref == "" or normalized_image_ref in built_function_images:
            continue
        dockerfile = Path(artifact_root) / "functions" / function_name / "Dockerfile"
        if not dockerfile.exists():
            continue
        targets.append(
            ImageBuildTarget(
                function_name=function_name,
                image_ref=normalized_image_ref,
                dockerfile=str(dockerfile),
            )
        )
    return targets


def with_function_build_workspace(
    artifact_root: str,
    function_names: list[str],
    callback: Callable[[str], None],
) -> None:
    normalized = sorted_unique_non_empty(function_names)
    context_root = Path(tempfile.mkdtemp(prefix="esbctl-build-context-"))
    try:
        functions_root = context_root / "functions"
        functions_root.mkdir(parents=True, exist_ok=True)
        for name in normalized:
            source_dir = Path(artifact_root) / "functions" / name
            target_dir = functions_root / name
            shutil.copytree(source_dir, target_dir, symlinks=True, dirs_exist_ok=True)
        callback(str(context_root))
    finally:
        shutil.rmtree(context_root, ignore_errors=True)


def build_and_push_function_image(
    *,
    image_ref: str,
    function_name: str,
    repo_root: str,
    context_root: str,
    no_cache: bool,
    ensure_base: bool,
    built_base_images: set[str],
    resolved_maven_shim_images: dict[str, str],
) -> None:
    dockerfile = str(Path(context_root) / "functions" / function_name / "Dockerfile")
    push_ref = resolve_push_reference(image_ref)
    resolved_dockerfile, cleanup = resolve_function_build_dockerfile(
        dockerfile=dockerfile,
        no_cache=no_cache,
        resolved_maven_shim_images=resolved_maven_shim_images,
    )
    try:
        if ensure_base:
            ensure_lambda_base_image_from_dockerfile(
                resolved_dockerfile,
                no_cache,
                built_base_images,
            )
        layer_contexts = prepare_function_layer_build_contexts(
            repo_root,
            context_root,
            function_name,
        )
        build_cmd = buildx_build_command_with_build_args_and_contexts(
            tag=image_ref,
            dockerfile=resolved_dockerfile,
            context_dir=context_root,
            no_cache=no_cache,
            build_args=None,
            build_contexts=layer_contexts,
        )
        run_command(build_cmd, check=True)
        if push_ref != image_ref:
            run_command(["docker", "tag", image_ref, push_ref], check=True)
        run_command(["docker", "push", push_ref], check=True)
    finally:
        cleanup()


def buildx_build_command_with_build_args_and_contexts(
    *,
    tag: str,
    dockerfile: str,
    context_dir: str,
    no_cache: bool,
    build_args: dict[str, str] | None,
    build_contexts: dict[str, str] | None,
) -> list[str]:
    cmd = ["docker", "buildx", "build", "--platform", "linux/amd64", "--load", "--pull"]
    if no_cache:
        cmd.append("--no-cache")
    cmd = append_proxy_build_args(cmd)
    for key, value in sorted((build_args or {}).items()):
        normalized = value.strip()
        if normalized == "":
            continue
        cmd.extend(["--build-arg", f"{key}={normalized}"])
    for key, value in sorted((build_contexts or {}).items()):
        if key.strip() == "" or value.strip() == "":
            continue
        cmd.extend(["--build-context", f"{key}={value.strip()}"])
    cmd.extend(["--tag", tag, "--file", dockerfile, context_dir])
    return cmd


def resolve_push_reference(image_ref: str) -> str:
    runtime_registry = os.environ.get("CONTAINER_REGISTRY", "").strip().rstrip("/")
    host_registry = os.environ.get("HOST_REGISTRY_ADDR", "").strip().rstrip("/")
    if runtime_registry == "" or host_registry == "":
        return image_ref
    prefix = f"{runtime_registry}/"
    if not image_ref.startswith(prefix):
        return image_ref
    suffix = image_ref.removeprefix(prefix)
    return f"{host_registry}/{suffix}"


def resolve_runtime_function_registry() -> str:
    for key in ("CONTAINER_REGISTRY", "REGISTRY", "HOST_REGISTRY_ADDR"):
        value = os.environ.get(key, "").strip().rstrip("/")
        if value != "":
            return value
    return ""


def resolve_host_function_registry() -> str:
    for key in ("HOST_REGISTRY_ADDR", "CONTAINER_REGISTRY", "REGISTRY"):
        value = os.environ.get(key, "").strip().rstrip("/")
        if value != "":
            return value
    return ""


def resolve_registry_aliases() -> list[str]:
    values = [
        os.environ.get("CONTAINER_REGISTRY", "").strip().rstrip("/"),
        os.environ.get("HOST_REGISTRY_ADDR", "").strip().rstrip("/"),
        os.environ.get("REGISTRY", "").strip().rstrip("/"),
        "127.0.0.1:5010",
        "localhost:5010",
        "registry:5010",
    ]
    return sorted_unique_non_empty(values)


def normalize_function_image_ref_for_runtime(image_ref: str) -> tuple[str, bool]:
    trimmed = image_ref.strip()
    if not is_lambda_function_ref(trimmed):
        return trimmed, False
    return rewrite_registry_alias(
        trimmed,
        resolve_runtime_function_registry(),
        resolve_registry_aliases(),
    )


def rewrite_lambda_base_ref_for_build(image_ref: str) -> tuple[str, bool]:
    trimmed = image_ref.strip()
    if not is_lambda_base_ref(trimmed):
        return trimmed, False
    return rewrite_registry_alias(
        trimmed,
        resolve_host_function_registry(),
        resolve_registry_aliases(),
    )


def rewrite_registry_alias(
    image_ref: str,
    target_registry: str,
    aliases: list[str],
) -> tuple[str, bool]:
    trimmed = image_ref.strip()
    target = target_registry.strip().rstrip("/")
    if trimmed == "" or target == "":
        return trimmed, False
    target_prefix = f"{target}/"
    if trimmed.startswith(target_prefix):
        return trimmed, False
    for alias in aliases:
        current = alias.strip().rstrip("/")
        if current == "" or current == target:
            continue
        prefix = f"{current}/"
        if trimmed.startswith(prefix):
            return f"{target_prefix}{trimmed.removeprefix(prefix)}", True
    return trimmed, False


def is_lambda_function_ref(image_ref: str) -> bool:
    last_segment = image_repo_last_segment(image_ref)
    return last_segment.startswith("esb-lambda-") and last_segment != "esb-lambda-base"


def is_lambda_base_ref(image_ref: str) -> bool:
    return image_repo_last_segment(image_ref) == "esb-lambda-base"


def image_repo_last_segment(image_ref: str) -> str:
    ref = image_ref.strip()
    if ref == "":
        return ""
    without_digest = ref.split("@", 1)[0]
    slash = without_digest.rfind("/")
    colon = without_digest.rfind(":")
    repo = without_digest
    if colon > slash:
        repo = without_digest[:colon]
    if "/" in repo:
        return repo.rsplit("/", 1)[-1]
    return repo


def resolve_function_build_dockerfile(
    *,
    dockerfile: str,
    no_cache: bool,
    resolved_maven_shim_images: dict[str, str],
) -> tuple[str, Callable[[], None]]:
    data = Path(dockerfile).read_text(encoding="utf-8")
    rewritten, changed = rewrite_dockerfile_for_build(
        data,
        resolve_host_function_registry(),
        resolve_registry_aliases(),
    )
    rewritten, maven_changed = rewrite_dockerfile_for_maven_shim(
        rewritten,
        lambda base_ref: ensure_maven_shim_image_cached(
            base_ref=base_ref,
            no_cache=no_cache,
            resolved_maven_shim_images=resolved_maven_shim_images,
        ),
    )
    if maven_changed:
        changed = True
    if not changed:
        return dockerfile, lambda: None
    tmp_path = f"{dockerfile}.artifact.build"
    Path(tmp_path).write_text(rewritten, encoding="utf-8")

    def _cleanup() -> None:
        Path(tmp_path).unlink(missing_ok=True)

    return tmp_path, _cleanup


def ensure_maven_shim_image_cached(
    *,
    base_ref: str,
    no_cache: bool,
    resolved_maven_shim_images: dict[str, str],
) -> str:
    normalized = base_ref.strip()
    if normalized == "":
        raise RuntimeError("maven base image reference is empty")
    if normalized in resolved_maven_shim_images:
        return resolved_maven_shim_images[normalized]
    result = ensure_maven_shim_image(
        MavenShimEnsureInput(
            base_image=normalized,
            host_registry=resolve_host_function_registry(),
            no_cache=no_cache,
        )
    )
    shim_ref = result.shim_image
    resolved_maven_shim_images[normalized] = shim_ref
    return shim_ref


def rewrite_dockerfile_for_build(
    content: str,
    host_registry: str,
    registry_aliases: list[str],
) -> tuple[str, bool]:
    lines = content.split("\n")
    changed = False
    for idx, line in enumerate(lines):
        trimmed = line.strip()
        if not trimmed.lower().startswith("from "):
            continue
        parts = trimmed.split()
        ref_index = from_image_token_index(parts)
        if ref_index < 0:
            continue
        rewritten_ref, rewritten = rewrite_dockerfile_from_ref(
            parts[ref_index],
            host_registry,
            registry_aliases,
        )
        if not rewritten:
            continue
        parts[ref_index] = rewritten_ref
        indent_len = len(line) - len(line.lstrip(" \t"))
        indent = line[:indent_len]
        lines[idx] = indent + " ".join(parts)
        changed = True
    if not changed:
        return content, False
    return "\n".join(lines), True


def rewrite_dockerfile_for_maven_shim(
    content: str,
    resolve_shim: Callable[[str], str],
) -> tuple[str, bool]:
    lines = content.split("\n")
    changed = False
    saw_maven_run = False
    rewritten_maven_base = False

    for idx, line in enumerate(lines):
        trimmed = line.strip()
        lower = trimmed.lower()
        if lower.startswith("run "):
            command = trimmed[4:].strip()
            if _MAVEN_RUN_COMMAND_PATTERN.search(
                command
            ) or _MAVEN_WRAPPER_RUN_COMMAND_PATTERN.search(command):
                saw_maven_run = True
            continue
        if not lower.startswith("from "):
            continue
        parts = trimmed.split()
        ref_index = from_image_token_index(parts)
        if ref_index < 0:
            continue
        base_ref = parts[ref_index].strip()
        if not is_maven_base_ref(base_ref):
            continue
        shim_ref = resolve_shim(base_ref)
        parts[ref_index] = shim_ref
        indent_len = len(line) - len(line.lstrip(" \t"))
        indent = line[:indent_len]
        lines[idx] = indent + " ".join(parts)
        rewritten_maven_base = True
        changed = True

    if saw_maven_run and not rewritten_maven_base:
        raise RuntimeError(
            "maven run command detected but no maven base stage is rewriteable; "
            "use 'FROM maven:...' (or equivalent maven repo) in Dockerfile"
        )
    if not changed:
        return content, False
    return "\n".join(lines), True


def rewrite_dockerfile_from_ref(
    ref: str,
    host_registry: str,
    registry_aliases: list[str],
) -> tuple[str, bool]:
    current = ref.strip()
    if current == "":
        return ref, False
    rewritten = current
    changed = False
    base_ref, rewritten_base = rewrite_lambda_base_ref_for_build(rewritten)
    if rewritten_base:
        rewritten = base_ref
        changed = True
    elif host_registry.strip() != "":
        generic_ref, rewritten_generic = rewrite_registry_alias(
            rewritten,
            host_registry,
            registry_aliases,
        )
        if rewritten_generic:
            rewritten = generic_ref
            changed = True
    return rewritten, changed


def from_image_token_index(parts: list[str]) -> int:
    for idx in range(1, len(parts)):
        token = parts[idx].strip()
        if token == "" or token.startswith("--"):
            continue
        return idx
    return -1


def is_maven_base_ref(image_ref: str) -> bool:
    ref = image_ref.strip()
    if ref == "":
        return False
    without_digest = ref.split("@", 1)[0]
    slash = without_digest.rfind("/")
    colon = without_digest.rfind(":")
    repo = without_digest
    if colon > slash:
        repo = without_digest[:colon]
    last_segment = repo.rsplit("/", 1)[-1]
    return last_segment == "maven"


def resolve_default_lambda_base_ref() -> str:
    registry = resolve_ensure_base_registry()
    if registry == "":
        raise RuntimeError(
            "lambda base registry is unresolved: "
            "set CONTAINER_REGISTRY or HOST_REGISTRY_ADDR (or REGISTRY)"
        )
    return f"{registry}/esb-lambda-base:latest"


def resolve_ensure_base_registry() -> str:
    for key in ("HOST_REGISTRY_ADDR", "CONTAINER_REGISTRY", "REGISTRY"):
        value = os.environ.get(key, "").strip().rstrip("/")
        if value != "":
            return value
    return ""


def ensure_lambda_base_image_from_dockerfile(
    function_dockerfile: str,
    no_cache: bool,
    built_base_images: set[str],
) -> None:
    base_ref, ok = read_lambda_base_ref(function_dockerfile)
    if not ok or base_ref == "":
        return
    ensure_lambda_base_image(base_ref, no_cache, built_base_images)


def ensure_lambda_base_image(
    base_image_ref: str,
    no_cache: bool,
    built_base_images: set[str],
) -> None:
    base_ref = base_image_ref.strip()
    if base_ref == "":
        return
    push_ref = resolve_push_reference(base_ref)
    if base_ref in built_base_images or push_ref in built_base_images:
        built_base_images.add(base_ref)
        return

    if not docker_image_exists(base_ref):
        pull = run_command(["docker", "pull", base_ref], check=False)
        if pull.returncode != 0:
            base_dockerfile, build_context = resolve_runtime_hooks_build_paths()
            build_cmd = buildx_build_command_with_build_args_and_contexts(
                tag=base_ref,
                dockerfile=base_dockerfile,
                context_dir=build_context,
                no_cache=no_cache,
                build_args=None,
                build_contexts=None,
            )
            run_command(build_cmd, check=True)

    if push_ref != base_ref:
        run_command(["docker", "tag", base_ref, push_ref], check=True)
    run_command(["docker", "push", push_ref], check=True)
    built_base_images.add(base_ref)
    built_base_images.add(push_ref)


def resolve_runtime_hooks_build_paths() -> tuple[str, str]:
    current = Path.cwd().resolve()
    while True:
        candidate = current / "runtime-hooks" / "python" / "docker" / "Dockerfile"
        if candidate.exists() and candidate.is_file():
            return str(candidate), str(current)
        if current.parent == current:
            break
        current = current.parent
    raise RuntimeError(
        'lambda base image "esb-lambda-base" not found locally and '
        "runtime hooks dockerfile is unavailable "
        "(expected: runtime-hooks/python/docker/Dockerfile from working tree root)"
    )


def read_lambda_base_ref(dockerfile_path: str) -> tuple[str, bool]:
    for line in Path(dockerfile_path).read_text(encoding="utf-8").splitlines():
        trimmed = line.strip()
        if not trimmed.lower().startswith("from "):
            continue
        parts = trimmed.split()
        ref_index = from_image_token_index(parts)
        if ref_index < 0 or ref_index >= len(parts):
            continue
        ref = parts[ref_index].strip()
        if is_lambda_base_ref(ref):
            return ref, True
    return "", False


def prepare_function_layer_build_contexts(
    repo_root: str,
    context_root: str,
    function_name: str,
) -> dict[str, str] | None:
    dockerfile = Path(context_root) / "functions" / function_name / "Dockerfile"
    dockerfile_data = dockerfile.read_text(encoding="utf-8")
    aliases = parse_layer_context_aliases(dockerfile_data)
    if not aliases:
        return None
    stage_aliases = parse_dockerfile_stage_aliases(dockerfile_data)

    layers_dir = Path(context_root) / "functions" / function_name / "layers"
    zip_by_name = discover_layer_zip_files(layers_dir)
    cache_root = Path(repo_root) / resolve_brand_home_dir() / "cache" / "layers"
    cache_root.mkdir(parents=True, exist_ok=True)
    nest_python = is_python_layer_layout_required(dockerfile_data)
    contexts: dict[str, str] = {}

    for alias in aliases:
        target_name, ok = layer_alias_target_name(alias)
        if not ok:
            continue
        zip_path = zip_by_name.get(target_name)
        if zip_path is None:
            if alias in stage_aliases:
                continue
            raise RuntimeError(f'layer archive for alias "{alias}" not found in {layers_dir}')
        extracted = prepare_layer_archive_cache(cache_root, zip_path, nest_python)
        contexts[alias] = str(extracted)

    if not contexts:
        return None
    return contexts


def parse_layer_context_aliases(dockerfile: str) -> list[str]:
    seen: set[str] = set()
    aliases: list[str] = []
    for line in dockerfile_logical_lines(dockerfile):
        trimmed = line.strip()
        if trimmed == "" or trimmed.startswith("#"):
            continue
        fields = trimmed.split()
        if len(fields) < 4 or fields[0].lower() != "copy":
            continue
        from_alias = ""
        positional: list[str] = []
        for field in fields[1:]:
            if field.startswith("--from="):
                from_alias = field.removeprefix("--from=").strip()
                continue
            if field.startswith("--"):
                continue
            positional.append(field)
        if from_alias == "" or len(positional) < 2:
            continue
        src = positional[0].strip()
        dst = positional[1].strip()
        if src != "/" or dst not in {"/opt", "/opt/"}:
            continue
        _, ok = layer_alias_target_name(from_alias)
        if not ok or from_alias in seen:
            continue
        seen.add(from_alias)
        aliases.append(from_alias)
    aliases.sort()
    return aliases


def parse_dockerfile_stage_aliases(dockerfile: str) -> set[str]:
    aliases: set[str] = set()
    for line in dockerfile_logical_lines(dockerfile):
        trimmed = line.strip()
        if trimmed == "" or trimmed.startswith("#"):
            continue
        fields = trimmed.split()
        if len(fields) < 2 or fields[0].lower() != "from":
            continue
        for idx in range(1, len(fields) - 1):
            if fields[idx].lower() != "as":
                continue
            alias = fields[idx + 1].strip()
            if alias != "":
                aliases.add(alias)
            break
    return aliases


def discover_layer_zip_files(layers_dir: Path) -> dict[str, Path]:
    if not layers_dir.exists():
        return {}
    if not layers_dir.is_dir():
        raise RuntimeError(f"read layer directory {layers_dir}: not a directory")
    out: dict[str, Path] = {}
    for entry in layers_dir.iterdir():
        if entry.is_dir():
            continue
        if entry.suffix.lower() != ".zip":
            continue
        target_name = entry.stem
        if target_name == "":
            continue
        out[target_name] = entry
    return out


def layer_alias_target_name(alias: str) -> tuple[str, bool]:
    prefix = "layer_"
    if not alias.startswith(prefix):
        return "", False
    rest = alias[len(prefix) :]
    sep = rest.find("_")
    if sep <= 0:
        return "", False
    index_part = rest[:sep]
    if not index_part.isdigit():
        return "", False
    target_name = rest[sep + 1 :].strip()
    if target_name == "":
        return "", False
    return target_name, True


def is_python_layer_layout_required(dockerfile: str) -> bool:
    for line in dockerfile_logical_lines(dockerfile):
        trimmed = line.strip()
        if trimmed == "" or trimmed.startswith("#"):
            continue
        fields = trimmed.split()
        if len(fields) < 2 or fields[0].lower() != "env":
            continue
        assignments = fields[1:]
        for idx, assignment in enumerate(assignments):
            if "=" in assignment:
                key, value = assignment.split("=", 1)
                if key.strip().lower() == "pythonpath" and "/opt/python" in value:
                    return True
                continue
            if assignment.strip().lower() == "pythonpath" and idx + 1 < len(assignments):
                if "/opt/python" in assignments[idx + 1]:
                    return True
    return False


def dockerfile_logical_lines(dockerfile: str) -> list[str]:
    raw_lines = dockerfile.split("\n")
    lines: list[str] = []
    current: list[str] = []
    for raw in raw_lines:
        trimmed_right = raw.rstrip(" \t\r")
        continued = trimmed_right.endswith("\\")
        part = trimmed_right[:-1] if continued else trimmed_right
        part = part.rstrip(" \t")
        if current:
            current.append(" ")
        current.append(part)
        if continued:
            continue
        lines.append("".join(current))
        current = []
    if current:
        lines.append("".join(current))
    return lines


def prepare_layer_archive_cache(cache_root: Path, archive_path: Path, nest_python: bool) -> Path:
    archive_digest = hash_file_sha256(archive_path)
    mode = "plain"
    prefix = ""
    if nest_python:
        if zip_has_python_layout(archive_path):
            mode = "python-layout"
        else:
            mode = "python-prefixed"
            prefix = "python"

    cache_key = layer_cache_key(archive_digest, mode)
    dest = cache_root / cache_key
    if dest.exists():
        return dest

    tmp_dir = Path(f"{dest}.tmp-{os.getpid()}")
    if tmp_dir.exists():
        shutil.rmtree(tmp_dir)
    tmp_dir.mkdir(parents=True, exist_ok=True)
    try:
        extract_zip_to_dir_with_limit(archive_path, tmp_dir, prefix, _MAX_LAYER_EXTRACT_BYTES)
        tmp_dir.rename(dest)
    except Exception:  # noqa: BLE001
        shutil.rmtree(tmp_dir, ignore_errors=True)
        if dest.exists():
            return dest
        raise
    return dest


def layer_cache_key(archive_digest: str, mode: str) -> str:
    seed = f"{_LAYER_CACHE_SCHEMA_VERSION}:{archive_digest}:{mode}"
    return hashlib.sha256(seed.encode("utf-8")).hexdigest()[:16]


def hash_file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as file:
        while True:
            chunk = file.read(1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
    return digest.hexdigest()


def zip_has_python_layout(path: Path) -> bool:
    with zipfile.ZipFile(path, "r") as archive:
        for item in archive.infolist():
            normalized = item.filename.lstrip("/")
            if normalized == "":
                continue
            head = normalized.split("/", 1)[0].strip().lower()
            if head in {"python", "site-packages"}:
                return True
    return False


def extract_zip_to_dir_with_limit(
    src: Path,
    dst: Path,
    prefix: str,
    max_extract_bytes: int,
) -> None:
    if max_extract_bytes <= 0:
        raise RuntimeError("zip extraction limit must be positive")

    base = dst.resolve()
    base.mkdir(parents=True, exist_ok=True)
    extracted_total = 0

    with zipfile.ZipFile(src, "r") as archive:
        for item in archive.infolist():
            name = Path(item.filename)
            clean_name = Path(str(name).replace("\\", "/")).as_posix().lstrip("/")
            if clean_name == "" or clean_name == ".":
                continue
            if clean_name.startswith("../") or clean_name == "..":
                raise RuntimeError(f"invalid zip entry: {item.filename}")
            if prefix:
                clean_name = f"{prefix}/{clean_name}"
            target = (base / clean_name).resolve()
            if not (str(target) == str(base) or str(target).startswith(str(base) + os.sep)):
                raise RuntimeError(f"zip entry escapes destination: {item.filename}")
            if item.is_dir():
                target.mkdir(parents=True, exist_ok=True)
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            remaining = max_extract_bytes - extracted_total
            if remaining <= 0:
                raise RuntimeError(f"zip extraction exceeds limit: {max_extract_bytes} bytes")
            if item.file_size > remaining:
                raise RuntimeError(f"zip extraction exceeds limit: {max_extract_bytes} bytes")
            with archive.open(item, "r") as src_file, target.open("wb") as dst_file:
                copied = 0
                while True:
                    chunk = src_file.read(min(1024 * 1024, remaining - copied))
                    if not chunk:
                        break
                    copied += len(chunk)
                    dst_file.write(chunk)
                    if copied > remaining:
                        raise RuntimeError(
                            f"zip extraction exceeds limit: {max_extract_bytes} bytes"
                        )
            extracted_total += copied
            if extracted_total > max_extract_bytes:
                raise RuntimeError(f"zip extraction exceeds limit: {max_extract_bytes} bytes")


def resolve_brand_home_dir() -> str:
    return DEFAULT_BRAND_HOME_DIR


def resolve_repo_root(manifest_path: str, artifact_root: str) -> str:
    candidates = [Path(artifact_root), Path(manifest_path).resolve().parent]
    for candidate in candidates:
        resolved, ok = find_ancestor_with_path(candidate, ".git")
        if ok:
            return resolved
    root = common_ancestor_path(Path(artifact_root), Path(manifest_path).resolve().parent)
    if root != "":
        return root
    return str(Path.cwd())


def find_ancestor_with_path(start: Path, name: str) -> tuple[str, bool]:
    current = start.resolve()
    while True:
        if (current / name).exists():
            return str(current), True
        if current.parent == current:
            return "", False
        current = current.parent


def common_ancestor_path(first: Path, second: Path) -> str:
    current = first.resolve()
    target = second.resolve()
    while True:
        if has_path_prefix(target, current):
            return str(current)
        if current.parent == current:
            return ""
        current = current.parent


def has_path_prefix(path: Path, prefix: Path) -> bool:
    clean_path = path.resolve()
    clean_prefix = prefix.resolve()
    if clean_path == clean_prefix:
        return True
    return str(clean_path).startswith(str(clean_prefix) + os.sep)


def normalize_output_function_images(output_dir: str, published_function_images: set[str]) -> None:
    functions_path = Path(output_dir) / "functions.yml"
    payload, ok = _load_yaml_map(str(functions_path))
    if not ok:
        return
    functions_raw = payload.get("functions")
    if not isinstance(functions_raw, dict):
        raise RuntimeError(f"functions must be map in {functions_path}")

    changed = False
    for function_name, raw in list(functions_raw.items()):
        if not isinstance(raw, dict):
            continue
        image_raw = raw.get("image")
        if image_raw is None:
            continue
        image_ref = str(image_raw).strip()
        if image_ref == "":
            continue
        normalized, rewritten = normalize_function_image_ref_for_runtime(image_ref)
        if not rewritten or normalized == image_ref:
            continue
        if normalized not in published_function_images:
            continue
        raw["image"] = normalized
        functions_raw[function_name] = raw
        changed = True

    if not changed:
        return
    _atomic_write_yaml(str(functions_path), payload)


def execute_provision(input_data: ProvisionInput) -> None:
    working_dir = input_data.project_dir.strip() or str(Path.cwd())

    def _run(args: list[str]) -> None:
        run_command(
            ["docker", *args],
            cwd=working_dir,
            quiet_stdout=not input_data.verbose,
            check=True,
        )

    if input_data.no_deps:
        _run(build_args(input_data))
    _run(run_args(input_data))


def build_args(req: ProvisionInput) -> list[str]:
    provisioner_name = req.provisioner_name.strip() or "provisioner"
    args = compose_base_args(req)
    args.extend(["--profile", "deploy", "build", provisioner_name])
    return args


def run_args(req: ProvisionInput) -> list[str]:
    provisioner_name = req.provisioner_name.strip() or "provisioner"
    args = compose_base_args(req)
    args.extend(["--profile", "deploy", "run", "--rm"])
    if req.no_deps:
        args.append("--no-deps")
    args.append(provisioner_name)
    return args


def compose_base_args(req: ProvisionInput) -> list[str]:
    args = ["compose"]
    for item in req.compose_files:
        normalized = item.strip()
        if normalized == "":
            continue
        args.extend(["-f", normalized])
    if req.no_warn_orphans:
        args.append("--no-warn-orphans")
    project = req.compose_project.strip()
    if project != "":
        args.extend(["-p", project])
    env_file = req.env_file.strip()
    if env_file != "":
        args.extend(["--env-file", env_file])
    return args


def _load_yaml_map(path: str) -> tuple[dict[str, Any], bool]:
    try:
        raw = Path(path).read_text(encoding="utf-8")
    except FileNotFoundError:
        return {}, False
    payload = yaml.safe_load(raw)
    if payload is None:
        return {}, True
    if not isinstance(payload, dict):
        raise RuntimeError(f"YAML must decode as a map: {path}")
    return payload, True


def _atomic_write_yaml(path: str, payload: dict[str, Any]) -> None:
    path_obj = Path(path)
    path_obj.parent.mkdir(parents=True, exist_ok=True)
    data = yaml.safe_dump(payload, sort_keys=False)
    with tempfile.NamedTemporaryFile(
        "w",
        encoding="utf-8",
        dir=str(path_obj.parent),
        prefix=".tmp-",
        delete=False,
    ) as tmp:
        tmp.write(data)
        tmp.flush()
        os.fsync(tmp.fileno())
        tmp_path = Path(tmp.name)
    tmp_path.replace(path_obj)
