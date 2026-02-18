# Where: e2e/runner/deploy.py
# What: Deployment execution for E2E environments.
# Why: Keep deploy logic separate from lifecycle and test orchestration.
from __future__ import annotations

import threading
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Callable

import yaml

from e2e.runner.logging import LogSink, run_and_stream
from e2e.runner.models import RunContext
from e2e.runner.utils import E2E_ARTIFACT_ROOT, PROJECT_ROOT

LOCAL_IMAGE_FIXTURES: dict[str, Path] = {
    "esb-e2e-lambda-python": PROJECT_ROOT / "tools" / "e2e-lambda-fixtures" / "python",
    "esb-e2e-lambda-java": PROJECT_ROOT / "tools" / "e2e-lambda-fixtures" / "java",
}

_prepared_local_fixture_images: set[str] = set()
_prepared_local_fixture_lock = threading.Lock()


def _resolve_deploy_driver(ctx: RunContext) -> str:
    raw = ctx.scenario.deploy_driver
    driver = str(raw).strip().lower()
    if driver != "artifact":
        raise RuntimeError(f"deploy_driver '{driver}' is not supported by E2E deploy runner")
    return driver


def deploy_templates(
    ctx: RunContext,
    templates: list[Path],
    *,
    no_cache: bool,
    verbose: bool,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    del templates
    del verbose
    _prepare_local_fixture_images(ctx, log=log, printer=printer)
    _resolve_deploy_driver(ctx)
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
        raise RuntimeError("CONFIG_DIR is required for deploy_driver=artifact")

    _prepare_function_images_from_artifact(
        ctx,
        manifest_path,
        no_cache=no_cache,
        log=log,
        printer=printer,
    )

    message = f"Applying artifact manifest for {ctx.scenario.env_name}..."
    log.write_line(message)
    if printer:
        printer(message)

    apply_cmd = [
        "artifactctl",
        "apply",
        "--artifact",
        str(manifest_path),
        "--out",
        config_dir,
    ]
    secret_env = str(ctx.scenario.extra.get("secret_env_file", "")).strip()
    if secret_env:
        apply_cmd.extend(["--secret-env", secret_env])
    rc = run_and_stream(
        apply_cmd,
        cwd=PROJECT_ROOT,
        env=ctx.deploy_env,
        log=log,
        printer=printer,
    )
    if rc != 0:
        raise RuntimeError(f"artifact apply failed with exit code {rc}")

    provision_cmd = _build_provision_command(ctx)
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


def _prepare_function_images_from_artifact(
    ctx: RunContext,
    manifest_path: Path,
    *,
    no_cache: bool,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    manifest = _load_yaml_map(manifest_path)
    artifacts = manifest.get("artifacts", [])
    if not isinstance(artifacts, list) or len(artifacts) == 0:
        raise RuntimeError(f"artifact manifest has no artifacts: {manifest_path}")

    built_base_runtime_refs: set[str] = set()
    built_function_images: set[str] = set()
    for idx, raw_entry in enumerate(artifacts):
        if not isinstance(raw_entry, dict):
            raise RuntimeError(f"artifact entry must be map: artifacts[{idx}]")

        artifact_root_raw = str(raw_entry.get("artifact_root", "")).strip()
        if artifact_root_raw == "":
            raise RuntimeError(f"artifact_root is required: artifacts[{idx}]")
        artifact_root = _resolve_artifact_root(manifest_path, artifact_root_raw)

        runtime_config_dir_raw = str(raw_entry.get("runtime_config_dir", "")).strip()
        if runtime_config_dir_raw == "":
            raise RuntimeError(f"runtime_config_dir is required: artifacts[{idx}]")
        runtime_config_dir = (artifact_root / runtime_config_dir_raw).resolve()
        functions_path = runtime_config_dir / "functions.yml"
        functions_payload = _load_yaml_map(functions_path)
        functions = functions_payload.get("functions", {})
        if not isinstance(functions, dict):
            raise RuntimeError(f"functions must be map in {functions_path}")

        build_targets: list[tuple[str, str, Path]] = []
        for function_name in sorted(functions):
            function_payload = functions.get(function_name)
            if not isinstance(function_payload, dict):
                continue
            image_ref = str(function_payload.get("image", "")).strip()
            if image_ref == "" or image_ref in built_function_images:
                continue

            function_dir = artifact_root / "functions" / str(function_name)
            dockerfile = function_dir / "Dockerfile"
            if not dockerfile.exists():
                continue
            build_targets.append((function_name, image_ref, dockerfile))

        if len(build_targets) == 0:
            continue

        function_names = [function_name for function_name, _, _ in build_targets]
        with _temporary_function_context_dockerignore(artifact_root, function_names):
            for _, image_ref, dockerfile in build_targets:
                for base_runtime_ref in _collect_dockerfile_base_images(dockerfile):
                    if "esb-lambda-base:" not in base_runtime_ref:
                        continue
                    if base_runtime_ref in built_base_runtime_refs:
                        continue
                    _build_and_push_lambda_base_image(
                        ctx,
                        base_runtime_ref,
                        no_cache=no_cache,
                        log=log,
                        printer=printer,
                    )
                    built_base_runtime_refs.add(base_runtime_ref)

                _build_and_push_function_image(
                    ctx,
                    image_ref,
                    dockerfile,
                    artifact_root,
                    no_cache=no_cache,
                    log=log,
                    printer=printer,
                )
                built_function_images.add(image_ref)


def _collect_dockerfile_base_images(dockerfile: Path) -> list[str]:
    refs: list[str] = []
    for line in dockerfile.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped.lower().startswith("from "):
            continue
        parts = stripped.split()
        if len(parts) < 2:
            continue
        image_ref = _extract_from_image_ref(parts)
        if image_ref:
            refs.append(image_ref)
    return refs


def _extract_from_image_ref(parts: list[str]) -> str:
    # Dockerfile FROM syntax allows options before the image:
    #   FROM --platform=linux/amd64 <image> [AS <name>]
    # We must skip option tokens to reliably find the base image reference.
    for token in parts[1:]:
        value = token.strip()
        if value == "":
            continue
        if value.startswith("--"):
            continue
        return value
    return ""


def _build_and_push_lambda_base_image(
    ctx: RunContext,
    runtime_ref: str,
    *,
    no_cache: bool,
    log: LogSink,
    printer: Callable[[str], None] | None,
) -> None:
    push_ref = _resolve_push_reference(ctx, runtime_ref)
    message = f"Preparing lambda base image: {runtime_ref}"
    log.write_line(message)
    if printer:
        printer(message)

    build_cmd = [
        "docker",
        "buildx",
        "build",
        "--platform",
        "linux/amd64",
        "--load",
        "--tag",
        push_ref,
        "--file",
        "runtime-hooks/python/docker/Dockerfile",
        ".",
    ]
    if no_cache:
        build_cmd.insert(3, "--no-cache")
    _run_or_raise(
        build_cmd,
        error_prefix=f"failed to build lambda base image {push_ref}",
        env=ctx.deploy_env,
        log=log,
        printer=printer,
    )

    if runtime_ref != push_ref:
        _run_or_raise(
            ["docker", "tag", push_ref, runtime_ref],
            error_prefix=f"failed to tag lambda base image {push_ref} -> {runtime_ref}",
            env=ctx.deploy_env,
            log=log,
            printer=printer,
        )

    _run_or_raise(
        ["docker", "push", push_ref],
        error_prefix=f"failed to push lambda base image {push_ref}",
        env=ctx.deploy_env,
        log=log,
        printer=printer,
    )


def _build_and_push_function_image(
    ctx: RunContext,
    image_ref: str,
    dockerfile: Path,
    artifact_root: Path,
    *,
    no_cache: bool,
    log: LogSink,
    printer: Callable[[str], None] | None,
) -> None:
    push_ref = _resolve_push_reference(ctx, image_ref)
    message = f"Building function image: {image_ref}"
    log.write_line(message)
    if printer:
        printer(message)

    build_cmd = [
        "docker",
        "buildx",
        "build",
        "--platform",
        "linux/amd64",
        "--load",
        "--tag",
        image_ref,
        "--file",
        str(dockerfile),
        str(artifact_root),
    ]
    if no_cache:
        build_cmd.insert(3, "--no-cache")

    _run_or_raise(
        build_cmd,
        error_prefix=f"failed to build function image {image_ref}",
        env=ctx.deploy_env,
        log=log,
        printer=printer,
    )

    if push_ref != image_ref:
        _run_or_raise(
            ["docker", "tag", image_ref, push_ref],
            error_prefix=f"failed to tag function image {image_ref} -> {push_ref}",
            env=ctx.deploy_env,
            log=log,
            printer=printer,
        )

    _run_or_raise(
        ["docker", "push", push_ref],
        error_prefix=f"failed to push function image {push_ref}",
        env=ctx.deploy_env,
        log=log,
        printer=printer,
    )


@contextmanager
def _temporary_function_context_dockerignore(
    artifact_root: Path,
    function_names: list[str],
):
    dockerignore = artifact_root / ".dockerignore"
    original = _read_bytes_if_exists(dockerignore)
    _write_function_context_dockerignore(dockerignore, function_names)
    try:
        yield
    finally:
        if original is None:
            dockerignore.unlink(missing_ok=True)
        else:
            dockerignore.write_bytes(original)


def _write_function_context_dockerignore(dockerignore: Path, function_names: list[str]) -> None:
    normalized_names: list[str] = []
    seen: set[str] = set()
    for raw in function_names:
        name = str(raw).strip()
        if name == "" or name in seen:
            continue
        seen.add(name)
        normalized_names.append(name)
    normalized_names.sort()

    lines = [
        "# Auto-generated by E2E artifact runner.",
        "# What: Permit function build context for all functions in this artifact root.",
        "*",
        "!.dockerignore",
        "!functions/",
    ]
    for name in normalized_names:
        lines.append(f"!functions/{name}/")
        lines.append(f"!functions/{name}/**")
    dockerignore.write_text("\n".join(lines) + "\n", encoding="utf-8")


def _read_bytes_if_exists(path: Path) -> bytes | None:
    if not path.exists():
        return None
    return path.read_bytes()


def _resolve_push_reference(ctx: RunContext, image_ref: str) -> str:
    runtime_registry = str(ctx.runtime_env.get("CONTAINER_REGISTRY", "")).strip().rstrip("/")
    host_registry = str(ctx.runtime_env.get("HOST_REGISTRY_ADDR", "")).strip().rstrip("/")
    if runtime_registry and host_registry and image_ref.startswith(runtime_registry + "/"):
        suffix = image_ref[len(runtime_registry) + 1 :]
        return f"{host_registry}/{suffix}"
    return image_ref


def _run_or_raise(
    cmd: list[str],
    *,
    error_prefix: str,
    env: dict[str, str],
    log: LogSink,
    printer: Callable[[str], None] | None,
) -> None:
    rc = run_and_stream(
        cmd,
        cwd=PROJECT_ROOT,
        env=env,
        log=log,
        printer=printer,
    )
    if rc != 0:
        raise RuntimeError(f"{error_prefix} (exit code {rc})")


def _load_yaml_map(path: Path) -> dict[str, Any]:
    if not path.exists():
        raise FileNotFoundError(path)
    raw = path.read_text(encoding="utf-8")
    value = yaml.safe_load(raw) or {}
    if not isinstance(value, dict):
        raise RuntimeError(f"yaml payload must be a map: {path}")
    return value


def _resolve_artifact_root(manifest_path: Path, artifact_root_raw: str) -> Path:
    root = Path(artifact_root_raw)
    if not root.is_absolute():
        root = manifest_path.parent / root
    return root.resolve()


def _resolve_artifact_manifest_path(ctx: RunContext) -> Path:
    raw = str(ctx.scenario.extra.get("artifact_manifest", "")).strip()
    if raw:
        path = Path(raw)
        if not path.is_absolute():
            path = PROJECT_ROOT / path
        return path.resolve()
    return (E2E_ARTIFACT_ROOT / ctx.scenario.env_name / "artifact.yml").resolve()


def _build_provision_command(ctx: RunContext) -> list[str]:
    cmd = [
        "docker",
        "compose",
        "--project-name",
        ctx.compose_project,
        "--file",
        str(ctx.compose_file),
    ]
    if ctx.env_file:
        cmd.extend(["--env-file", ctx.env_file])
    cmd.extend(
        [
            "--profile",
            "deploy",
            "run",
            "--rm",
            "--no-deps",
            "provisioner",
        ]
    )
    return cmd


def _prepare_local_fixture_images(
    ctx: RunContext,
    *,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    sources = _collect_local_fixture_image_sources(ctx.scenario.extra)
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

            build_cmd = [
                "docker",
                "buildx",
                "build",
                "--platform",
                "linux/amd64",
                "--load",
                "--tag",
                source,
                str(fixture_dir),
            ]
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


def _collect_local_fixture_image_sources(extra: dict[str, Any]) -> list[str]:
    values = _collect_function_overrides(extra.get("image_uri_overrides"), "image_uri_overrides")
    sources: set[str] = set()
    for value in values:
        _, _, source = value.partition("=")
        source = source.strip()
        if _is_local_fixture_image_source(source):
            sources.add(source)
    return sorted(sources)


def _is_local_fixture_image_source(source: str) -> bool:
    if not source:
        return False
    return _fixture_repo_name(source) in LOCAL_IMAGE_FIXTURES


def _fixture_repo_name(source: str) -> str:
    without_digest = source.split("@", 1)[0]
    last_segment = without_digest.rsplit("/", 1)[-1]
    return last_segment.split(":", 1)[0]


def _collect_function_overrides(raw: Any, field_name: str) -> list[str]:
    if raw is None:
        return []
    if isinstance(raw, dict):
        out: list[str] = []
        for key in sorted(raw):
            fn_name = str(key).strip()
            value = str(raw[key]).strip()
            if not fn_name or not value:
                raise ValueError(f"{field_name} entries must have non-empty function and value")
            out.append(f"{fn_name}={value}")
        return out
    if isinstance(raw, str):
        value = _normalize_function_override(raw, field_name)
        if value == "":
            return []
        return [value]
    if isinstance(raw, list):
        out: list[str] = []
        for item in raw:
            value = _normalize_function_override(str(item), field_name)
            if value != "":
                out.append(value)
        return out
    raise ValueError(f"{field_name} must be map or list")


def _normalize_function_override(raw: str, field_name: str) -> str:
    value = raw.strip()
    if value == "":
        return ""
    function_name, separator, override_value = value.partition("=")
    if separator == "" or function_name.strip() == "" or override_value.strip() == "":
        raise ValueError(f"{field_name} must use <function>=<value>: {raw!r}")
    return f"{function_name.strip()}={override_value.strip()}"
