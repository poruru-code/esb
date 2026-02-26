"""Image preparation helpers shared by bundle commands."""

from __future__ import annotations

import time
import urllib.request
from dataclasses import dataclass, field
from pathlib import Path

from tools.deployops.core.artifact_manifest import FunctionBuildTarget
from tools.deployops.core.runner import CommandRunner, RunnerError

LOCAL_REGISTRY_CONTAINER_PREFIX = "deployops-local-registry"
LOCAL_REGISTRY_IMAGE = "registry:2"


@dataclass
class ImagePreparationState:
    prepared_base_images: set[str] = field(default_factory=set)
    pushed_local_registry_images: set[str] = field(default_factory=set)
    ready_local_registries: set[str] = field(default_factory=set)


def image_exists(runner: CommandRunner, image_ref: str) -> bool:
    result = runner.run(
        ["docker", "image", "inspect", image_ref],
        capture_output=True,
        check=False,
    )
    return result.returncode == 0


def collect_missing_images(runner: CommandRunner, image_refs: list[str]) -> list[str]:
    missing: list[str] = []
    for image in image_refs:
        if not image_exists(runner, image):
            missing.append(image)
    return missing


def prepare_images(
    runner: CommandRunner,
    *,
    compose_file: Path,
    env_file: Path,
    function_build_targets: list[FunctionBuildTarget],
    project_root: Path,
    state: ImagePreparationState,
) -> None:
    runner.emit("Preparing missing images (--prepare-images)...")
    runner.run(
        [
            "docker",
            "compose",
            "--env-file",
            str(env_file),
            "-f",
            str(compose_file),
            "--profile",
            "deploy",
            "build",
        ],
        stream_output=True,
    )
    runner.run(
        [
            "docker",
            "compose",
            "--env-file",
            str(env_file),
            "-f",
            str(compose_file),
            "--profile",
            "deploy",
            "pull",
            "--ignore-pull-failures",
        ],
        stream_output=True,
        check=False,
    )
    runner.run(["docker", "pull", LOCAL_REGISTRY_IMAGE], check=False)
    prepare_missing_function_images(
        runner,
        function_build_targets=function_build_targets,
        project_root=project_root,
        state=state,
    )


def prepare_missing_function_images(
    runner: CommandRunner,
    *,
    function_build_targets: list[FunctionBuildTarget],
    project_root: Path,
    state: ImagePreparationState,
) -> None:
    for target in function_build_targets:
        image_ref = target.image_ref
        if image_exists(runner, image_ref):
            continue
        if not target.dockerfile_path.is_file():
            continue

        ensure_dockerfile_bases(
            runner,
            target.dockerfile_path,
            project_root=project_root,
            state=state,
        )
        build_with_buildx(
            runner,
            tag=image_ref,
            dockerfile=target.dockerfile_path,
            context_dir=target.context_dir,
        )


def ensure_dockerfile_bases(
    runner: CommandRunner,
    dockerfile_path: Path,
    *,
    project_root: Path,
    state: ImagePreparationState,
) -> None:
    for base_ref in dockerfile_base_images(dockerfile_path):
        ensure_base_image_available(
            runner,
            base_ref,
            project_root=project_root,
            state=state,
        )


def ensure_base_image_available(
    runner: CommandRunner,
    image_ref: str,
    *,
    project_root: Path,
    state: ImagePreparationState,
) -> None:
    if not image_ref:
        return

    if image_exists(runner, image_ref):
        push_to_local_registry_if_needed(runner, image_ref, state=state)
        return

    if image_ref in state.prepared_base_images:
        push_to_local_registry_if_needed(runner, image_ref, state=state)
        return

    state.prepared_base_images.add(image_ref)
    repo_name = image_repository_name(image_ref)
    runtime_hooks_dockerfile = project_root / "runtime-hooks/python/docker/Dockerfile"

    if repo_name.endswith("-lambda-base") or repo_name == "lambda-base":
        if not runtime_hooks_dockerfile.is_file():
            raise FileNotFoundError(
                f"runtime hooks dockerfile not found: {runtime_hooks_dockerfile}"
            )
        build_with_buildx(
            runner,
            tag=image_ref,
            dockerfile=runtime_hooks_dockerfile,
            context_dir=project_root,
        )
    else:
        prepare_fixture_image_if_known(runner, image_ref=image_ref, project_root=project_root)

    if not image_exists(runner, image_ref):
        runner.run(["docker", "pull", image_ref], check=False)

    if not image_exists(runner, image_ref):
        raise RunnerError(f"required base image not available: {image_ref}")

    push_to_local_registry_if_needed(runner, image_ref, state=state)


def prepare_fixture_image_if_known(
    runner: CommandRunner,
    *,
    image_ref: str,
    project_root: Path,
) -> None:
    repo_name = image_repository_name(image_ref)

    fixture_dir: Path | None = None
    if repo_name.endswith("-e2e-image-python") or repo_name == "e2e-image-python":
        fixture_dir = project_root / "e2e/fixtures/images/lambda/python"
    elif repo_name.endswith("-e2e-image-java") or repo_name == "e2e-image-java":
        fixture_dir = project_root / "e2e/fixtures/images/lambda/java"

    if fixture_dir is None or not fixture_dir.is_dir():
        return

    build_with_buildx(
        runner,
        tag=image_ref,
        dockerfile=fixture_dir / "Dockerfile",
        context_dir=fixture_dir,
    )


def build_with_buildx(
    runner: CommandRunner,
    *,
    tag: str,
    dockerfile: Path,
    context_dir: Path,
) -> None:
    runner.emit(f"Preparing image via buildx: {tag}")
    runner.run(
        [
            "docker",
            "buildx",
            "build",
            "--platform",
            "linux/amd64",
            "--load",
            "--pull",
            "--tag",
            tag,
            "--file",
            str(dockerfile),
            str(context_dir),
        ],
        stream_output=True,
    )


def dockerfile_base_images(dockerfile_path: Path) -> list[str]:
    images: list[str] = []
    for raw_line in dockerfile_path.read_text(encoding="utf-8").splitlines():
        stripped = raw_line.strip()
        if stripped == "" or stripped.startswith("#"):
            continue
        lower = stripped.lower()
        if not lower.startswith("from "):
            continue

        tokens = stripped.split()
        # FROM [--platform=..] <image> [AS name]
        image = ""
        for token in tokens[1:]:
            if token.startswith("--"):
                continue
            image = token
            break
        if image:
            images.append(image)
    return images


def push_to_local_registry_if_needed(
    runner: CommandRunner,
    image_ref: str,
    *,
    state: ImagePreparationState,
) -> None:
    host_port = local_registry_host_port_from_ref(image_ref)
    if host_port is None:
        return
    if image_ref in state.pushed_local_registry_images:
        return

    if not image_exists(runner, image_ref):
        raise RunnerError(f"image not found for local registry push: {image_ref}")

    ensure_local_registry(runner, host_port=host_port, state=state)
    runner.emit(f"Publishing local-registry image for buildx compatibility: {image_ref}")
    runner.run(["docker", "push", image_ref], stream_output=True)
    state.pushed_local_registry_images.add(image_ref)


def ensure_local_registry(
    runner: CommandRunner,
    *,
    host_port: str,
    state: ImagePreparationState,
) -> None:
    if host_port in state.ready_local_registries:
        return

    if registry_ping(host_port):
        state.ready_local_registries.add(host_port)
        return

    port = host_port.rsplit(":", 1)[-1]
    container_name = f"{LOCAL_REGISTRY_CONTAINER_PREFIX}-{port}"

    listed = runner.run(
        ["docker", "ps", "-a", "--format", "{{.Names}}"],
        capture_output=True,
        check=False,
    )
    existing = {line.strip() for line in listed.stdout.splitlines() if line.strip()}
    if container_name in existing:
        runner.run(["docker", "start", container_name], check=False)
        wait_for_local_registry(host_port)
        state.ready_local_registries.add(host_port)
        return

    started = runner.run(
        [
            "docker",
            "run",
            "-d",
            "--name",
            container_name,
            "-p",
            f"{port}:5000",
            LOCAL_REGISTRY_IMAGE,
        ],
        check=False,
    )
    if started.returncode != 0 and not registry_ping(host_port):
        raise RunnerError(f"failed to start local registry container: {container_name}")

    wait_for_local_registry(host_port)
    state.ready_local_registries.add(host_port)


def wait_for_local_registry(host_port: str, timeout_seconds: int = 30) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        if registry_ping(host_port):
            return
        time.sleep(1)
    raise RunnerError(f"local registry {host_port} did not become ready in time")


def registry_ping(host_port: str) -> bool:
    url = f"http://{host_port}/v2/"
    request = urllib.request.Request(url, method="GET")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(request, timeout=1) as response:
            return int(response.status) in (200, 401)
    except Exception:
        return False


def local_registry_host_port_from_ref(image_ref: str) -> str | None:
    if image_ref.startswith("127.0.0.1:") and "/" in image_ref:
        host_port = image_ref.split("/", 1)[0]
    elif image_ref.startswith("localhost:") and "/" in image_ref:
        host_port = image_ref.split("/", 1)[0]
    else:
        return None

    host, sep, port = host_port.partition(":")
    if sep != ":" or not port.isdigit():
        return None
    return f"{host}:{port}"


def image_repository_name(image_ref: str) -> str:
    last_slash = image_ref.rfind("/")
    last_colon = image_ref.rfind(":")
    if last_colon > last_slash:
        return image_ref[last_slash + 1 : last_colon]
    return image_ref[last_slash + 1 :]
