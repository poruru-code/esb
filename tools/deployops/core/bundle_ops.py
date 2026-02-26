"""High-level DinD bundle orchestration."""

from __future__ import annotations

import os
import shutil
from dataclasses import dataclass
from pathlib import Path
from typing import Sequence

from tools.deployops.core.artifact_manifest import (
    ArtifactManifest,
    FunctionBuildTarget,
    collect_function_images_and_build_targets,
    iter_runtime_config_dirs,
    load_artifact_manifest_from_dir,
    validate_runtime_config_dirs,
)
from tools.deployops.core.branding import brand_home_dir
from tools.deployops.core.certs import ensure_required_certs, resolve_cert_dir
from tools.deployops.core.compose import (
    list_compose_images,
    materialize_runtime_compose,
    resolve_bundle_compose_file,
)
from tools.deployops.core.envfile import normalize_bundle_env, read_env_file_value
from tools.deployops.core.images import (
    LOCAL_REGISTRY_IMAGE,
    ImagePreparationState,
    collect_missing_images,
    prepare_images,
)
from tools.deployops.core.runner import CommandRunner, RunnerError
from tools.deployops.core.runtime_config import merge_runtime_config_dirs

ASSETS_DIND_DIR = Path("tools/deployops/assets/dind")
ASSETS_DIND_DOCKERFILE = "Dockerfile"
ASSETS_DIND_ENTRYPOINT = "entrypoint.sh"


@dataclass(frozen=True)
class BundleOptions:
    artifact_dirs: list[str]
    env_file: str
    compose_file: str | None
    prepare_images: bool
    output_tag: str | None
    positional_tag: str | None
    build_dir: str


@dataclass(frozen=True)
class BundleInputs:
    project_root: Path
    artifact_dirs: list[Path]
    manifests: list[ArtifactManifest]
    project: str
    mode: str
    artifact_env: str
    env_file: Path
    compose_file: Path
    runtime_config_source_dirs: list[Path]
    function_images: list[str]
    function_build_targets: list[FunctionBuildTarget]


def execute_bundle_dind(options: BundleOptions, runner: CommandRunner) -> int:
    inputs = collect_bundle_inputs(options)

    output_tag = _resolve_output_tag(options, project=inputs.project)
    brand_home = brand_home_dir(inputs.project)

    env_name = os.environ.get("ESB_ENV", "").strip()
    if env_name == "":
        env_name = read_env_file_value(inputs.env_file, "ENV").strip()
    if env_name == "":
        env_name = inputs.artifact_env or "default"

    cert_dir = resolve_cert_dir(
        project_root=inputs.project_root,
        project_name=inputs.project,
        override=os.environ.get("CERT_DIR", ""),
    )
    run_uid = os.environ.get("RUN_UID", "").strip() or read_env_file_value(
        inputs.env_file,
        "RUN_UID",
    )
    run_gid = os.environ.get("RUN_GID", "").strip() or read_env_file_value(
        inputs.env_file,
        "RUN_GID",
    )
    run_uid = run_uid or "1000"
    run_gid = run_gid or "1000"

    compose_images = list_compose_images(
        runner,
        compose_file=inputs.compose_file,
        env_file=inputs.env_file,
    )
    all_images = _unique_images(compose_images + inputs.function_images + [LOCAL_REGISTRY_IMAGE])
    if not all_images:
        raise RunnerError("no images resolved from compose/artifact")

    artifact_dirs_label = " ".join(str(path) for path in inputs.artifact_dirs)
    runner.emit(f"Building DinD bundle from artifact dirs: {artifact_dirs_label}")
    runner.emit(f"Output tag: {output_tag}")
    runner.emit(f"Project: {inputs.project}")
    runner.emit(f"Mode: {inputs.mode}")
    runner.emit(f"Env: {env_name}")
    runner.emit(f"Env file: {inputs.env_file}")
    runner.emit(f"Compose file: {inputs.compose_file}")
    runner.emit(f"Prepare images: {options.prepare_images}")

    if runner.dry_run:
        _emit_dry_run_summary(
            inputs=inputs,
            output_tag=output_tag,
            env_name=env_name,
            compose_images=compose_images,
            all_images=all_images,
            cert_dir=cert_dir,
            run_uid=run_uid,
            run_gid=run_gid,
        )
        return 0

    ensure_required_certs(cert_dir)

    state = ImagePreparationState()
    missing_images = collect_missing_images(runner, all_images)
    if missing_images and options.prepare_images:
        prepare_images(
            runner,
            compose_file=inputs.compose_file,
            env_file=inputs.env_file,
            function_build_targets=inputs.function_build_targets,
            project_root=inputs.project_root,
            state=state,
        )
        missing_images = collect_missing_images(runner, all_images)
    if missing_images:
        formatted = "\n".join(f"  - {image}" for image in missing_images)
        raise RunnerError(
            "missing local images required for bundle:\n"
            f"{formatted}\n"
            "Hint: build/pull required images before bundling, or pass --prepare-images"
        )

    build_dir = (inputs.project_root / options.build_dir).resolve()
    runtime_config_dir = build_dir / "runtime-config"

    if build_dir.exists():
        shutil.rmtree(build_dir)
    build_dir.mkdir(parents=True, exist_ok=True)

    runner.emit("Saving images to tarball...")
    runner.run(
        ["docker", "save", "-o", str(build_dir / "images.tar"), *all_images],
        stream_output=True,
    )

    merge_runtime_config_dirs(inputs.runtime_config_source_dirs, runtime_config_dir)

    assets_dir = _resolve_assets_dind_dir(inputs.project_root)
    shutil.copy2(assets_dir / ASSETS_DIND_DOCKERFILE, build_dir / "Dockerfile")
    shutil.copy2(
        assets_dir / ASSETS_DIND_ENTRYPOINT,
        build_dir / "entrypoint.sh",
    )
    shutil.copy2(inputs.env_file, build_dir / ".env")

    normalize_bundle_env(build_dir / ".env", brand_home=brand_home)

    runner.emit("Generating bundle compose file...")
    materialize_runtime_compose(
        runner,
        compose_file=inputs.compose_file,
        env_file=build_dir / ".env",
        output_path=build_dir / "docker-compose.bundle.yml",
    )

    runner.emit("Copying certificates...")
    certs_target = build_dir / "certs"
    certs_target.mkdir(parents=True, exist_ok=True)
    for file in cert_dir.iterdir():
        if file.is_file():
            shutil.copy2(file, certs_target / file.name)

    runner.emit("Building DinD image...")
    runner.run(
        [
            "docker",
            "build",
            "-t",
            output_tag,
            "--build-arg",
            f"BRAND_HOME={brand_home}",
            "--build-arg",
            f"CERT_UID={run_uid}",
            "--build-arg",
            f"CERT_GID={run_gid}",
            str(build_dir),
        ],
        stream_output=True,
    )

    runner.emit(f"Done! Image {output_tag} created.")
    return 0


def execute_prepare_images(options: BundleOptions, runner: CommandRunner) -> int:
    inputs = collect_bundle_inputs(options)
    prepare_images(
        runner,
        compose_file=inputs.compose_file,
        env_file=inputs.env_file,
        function_build_targets=inputs.function_build_targets,
        project_root=inputs.project_root,
        state=ImagePreparationState(),
    )
    return 0


def collect_bundle_inputs(options: BundleOptions) -> BundleInputs:
    project_root = Path.cwd().resolve()
    _resolve_assets_dind_dir(project_root)

    env_file = Path(options.env_file).expanduser().resolve()
    if not env_file.is_file():
        raise FileNotFoundError(f"env file not found: {env_file}")

    manifests: list[ArtifactManifest] = []
    artifact_dirs: list[Path] = []
    for raw_dir in options.artifact_dirs:
        artifact_dir = Path(raw_dir).expanduser().resolve()
        if not artifact_dir.is_dir():
            raise FileNotFoundError(f"artifact dir not found: {artifact_dir}")
        manifests.append(load_artifact_manifest_from_dir(artifact_dir))
        artifact_dirs.append(artifact_dir)

    project, mode, artifact_env = _assert_manifest_consistency(manifests)

    compose_override = Path(options.compose_file).expanduser() if options.compose_file else None
    compose_file = resolve_bundle_compose_file(
        project_root=project_root,
        env_file=env_file,
        mode=mode,
        compose_override=compose_override,
    )

    runtime_config_source_dirs: list[Path] = []
    runtime_seen: set[Path] = set()
    function_images: list[str] = []
    image_seen: set[str] = set()
    function_build_targets: list[FunctionBuildTarget] = []
    build_seen: set[tuple[str, Path, str]] = set()

    for manifest in manifests:
        validate_runtime_config_dirs(manifest)

        for runtime_dir in iter_runtime_config_dirs(manifest):
            if runtime_dir in runtime_seen:
                continue
            runtime_seen.add(runtime_dir)
            runtime_config_source_dirs.append(runtime_dir)

        images, targets = collect_function_images_and_build_targets(manifest)
        for image in images:
            if image in image_seen:
                continue
            image_seen.add(image)
            function_images.append(image)

        for target in targets:
            key = (target.image_ref, target.context_dir, target.dockerfile_rel)
            if key in build_seen:
                continue
            build_seen.add(key)
            function_build_targets.append(target)

    if not runtime_config_source_dirs:
        raise RunnerError("runtime config directories not found from artifact manifests")

    return BundleInputs(
        project_root=project_root,
        artifact_dirs=artifact_dirs,
        manifests=manifests,
        project=project,
        mode=mode,
        artifact_env=artifact_env,
        env_file=env_file,
        compose_file=compose_file,
        runtime_config_source_dirs=runtime_config_source_dirs,
        function_images=function_images,
        function_build_targets=function_build_targets,
    )


def _assert_manifest_consistency(manifests: list[ArtifactManifest]) -> tuple[str, str, str]:
    project = ""
    mode = ""
    artifact_env = ""

    for manifest in manifests:
        if project == "":
            project = manifest.project
        elif project != manifest.project:
            raise RunnerError(
                "artifact project mismatch: "
                f"expected {project!r}, got {manifest.project!r} ({manifest.path})"
            )

        if mode == "":
            mode = manifest.mode
        elif mode != manifest.mode:
            raise RunnerError(
                "artifact mode mismatch: "
                f"expected {mode!r}, got {manifest.mode!r} ({manifest.path})"
            )

        if artifact_env == "":
            artifact_env = manifest.env
        elif artifact_env != manifest.env:
            raise RunnerError(
                "artifact env mismatch: "
                f"expected {artifact_env!r}, got {manifest.env!r} ({manifest.path})"
            )

    if project == "":
        raise RunnerError("artifact project not found in manifest metadata")
    if mode == "":
        raise RunnerError("artifact mode not found in manifest metadata")

    return project, mode, artifact_env


def _resolve_output_tag(options: BundleOptions, *, project: str) -> str:
    if options.output_tag and options.positional_tag:
        raise RunnerError("use either positional tag or --output-tag, not both")
    if options.output_tag:
        return options.output_tag
    if options.positional_tag:
        return options.positional_tag
    return f"{project}-dind-bundle:latest"


def _resolve_assets_dind_dir(project_root: Path) -> Path:
    assets_dir = (project_root / ASSETS_DIND_DIR).resolve()
    dockerfile_path = assets_dir / ASSETS_DIND_DOCKERFILE
    entrypoint_path = assets_dir / ASSETS_DIND_ENTRYPOINT
    if not dockerfile_path.is_file() or not entrypoint_path.is_file():
        raise RunnerError(
            "deployops DinD assets not found. Please run this command from the project root."
        )
    return assets_dir


def _unique_images(items: list[str]) -> list[str]:
    seen: set[str] = set()
    result: list[str] = []
    for item in items:
        value = item.strip()
        if value == "" or value in seen:
            continue
        seen.add(value)
        result.append(value)
    return result


def _emit_dry_run_summary(
    *,
    inputs: BundleInputs,
    output_tag: str,
    env_name: str,
    compose_images: list[str],
    all_images: list[str],
    cert_dir: Path,
    run_uid: str,
    run_gid: str,
) -> None:
    def join_csv(values: Sequence[object]) -> str:
        return ",".join(str(item) for item in values)

    print(f"ARTIFACT_DIRS={join_csv(inputs.artifact_dirs)}")
    print(f"ENV_FILE={inputs.env_file}")
    print(f"COMPOSE_FILE={inputs.compose_file}")
    print(f"OUTPUT_TAG={output_tag}")
    print(f"PROJECT={inputs.project}")
    print(f"MODE={inputs.mode}")
    print(f"ENV_NAME={env_name}")
    print(f"RUNTIME_CONFIG_SOURCE_DIRS={join_csv(inputs.runtime_config_source_dirs)}")
    print(f"FUNCTION_IMAGES={join_csv(inputs.function_images)}")
    print(f"COMPOSE_IMAGES={join_csv(compose_images)}")
    print(f"ALL_IMAGES={join_csv(all_images)}")
    print(f"LOCAL_REGISTRY_IMAGE={LOCAL_REGISTRY_IMAGE}")
    print(f"CERT_DIR={cert_dir}")
    print(f"RUN_UID={run_uid}")
    print(f"RUN_GID={run_gid}")
