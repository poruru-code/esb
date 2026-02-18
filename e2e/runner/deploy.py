# Where: e2e/runner/deploy.py
# What: Deployment execution for E2E environments.
# Why: Keep deploy logic separate from lifecycle and test orchestration.
from __future__ import annotations

import threading
from pathlib import Path
from typing import Any, Callable

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

    message = f"Preparing artifact images for {ctx.scenario.env_name}..."
    log.write_line(message)
    if printer:
        printer(message)
    prepare_cmd = [
        "artifactctl",
        "prepare-images",
        "--artifact",
        str(manifest_path),
    ]
    if no_cache:
        prepare_cmd.append("--no-cache")
    rc = run_and_stream(
        prepare_cmd,
        cwd=PROJECT_ROOT,
        env=ctx.deploy_env,
        log=log,
        printer=printer,
    )
    if rc != 0:
        raise RuntimeError(f"artifact prepare-images failed with exit code {rc}")

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
