# Where: e2e/runner/deploy.py
# What: Deployment execution for E2E environments.
# Why: Keep deploy logic separate from lifecycle and test orchestration.
from __future__ import annotations

import json
import threading
from pathlib import Path
from typing import Callable

from e2e.runner.ctl_contract import resolve_ctl_bin_from_env
from e2e.runner.logging import LogSink, run_and_stream
from e2e.runner.models import RunContext
from e2e.runner.utils import PROJECT_ROOT
from tools.cli.fixture_image import (
    DEFAULT_FIXTURE_IMAGE_ROOT,
    FIXTURE_IMAGE_ENSURE_SCHEMA_VERSION,
    FixtureImageEnsureInput,
    execute_fixture_image_ensure,
)

_FIXTURE_PREPARE_CACHE_ENV_KEYS: tuple[str, ...] = (
    "HTTP_PROXY",
    "http_proxy",
    "HTTPS_PROXY",
    "https_proxy",
    "NO_PROXY",
    "no_proxy",
    "ALL_PROXY",
    "all_proxy",
    "HOST_REGISTRY_ADDR",
    "CONTAINER_REGISTRY",
)

# Backward-compatible state name retained for existing tests.
# Key is a deterministic condition hash for fixture preparation.
_prepared_local_fixture_images: set[str] = set()
# Kept for compatibility with older tests that clear this symbol.
_prepared_maven_shim_images: dict[tuple[str, str], str] = {}
_prepared_local_fixture_lock = threading.Lock()


def _ctl_bin(ctx: RunContext) -> str:
    # run_tests.py resolves CTL_BIN(_RESOLVED) before runner starts.
    return resolve_ctl_bin_from_env(ctx.deploy_env)


def deploy_artifacts(
    ctx: RunContext,
    *,
    no_cache: bool,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    _prepare_local_fixture_images(ctx, no_cache=no_cache, log=log, printer=printer)
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

    message = f"Deploying artifact manifest for {ctx.scenario.env_name}..."
    log.write_line(message)
    if printer:
        printer(message)
    ctl_bin = _ctl_bin(ctx)
    deploy_cmd = [
        ctl_bin,
        "deploy",
        "--artifact",
        str(manifest_path),
    ]
    if no_cache:
        deploy_cmd.append("--no-cache")
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
        ctl_bin,
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
    no_cache: bool,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    manifest_path = _resolve_artifact_manifest_path(ctx)
    if not manifest_path.exists():
        return

    cache_key = _fixture_prepare_cache_key(ctx, manifest_path)

    with _prepared_local_fixture_lock:
        if not no_cache and cache_key in _prepared_local_fixture_images:
            return

        message = f"Preparing local image fixtures from: {manifest_path}"
        log.write_line(message)
        if printer:
            printer(message)

        fixture_root = str((PROJECT_ROOT / DEFAULT_FIXTURE_IMAGE_ROOT).resolve())
        try:
            result = execute_fixture_image_ensure(
                FixtureImageEnsureInput(
                    artifact_path=str(manifest_path),
                    no_cache=no_cache,
                    fixture_root=fixture_root,
                    env=ctx.deploy_env,
                )
            )
        except Exception as exc:  # noqa: BLE001
            raise RuntimeError(f"failed to prepare local fixture images: {exc}") from exc

        if result.schema_version != FIXTURE_IMAGE_ENSURE_SCHEMA_VERSION:
            raise RuntimeError(
                "invalid fixture image ensure response schema: "
                f"{result.schema_version} (expected {FIXTURE_IMAGE_ENSURE_SCHEMA_VERSION})"
            )

        prepared_images = _normalize_prepared_images(result.prepared_images)
        for image_ref in prepared_images:
            image_message = f"Prepared local image fixture: {image_ref}"
            log.write_line(image_message)
            if printer:
                printer(image_message)

        if not no_cache:
            _prepared_local_fixture_images.add(cache_key)


def _fixture_prepare_cache_key(ctx: RunContext, manifest_path: Path) -> str:
    payload = {
        "manifest_path": str(manifest_path),
        "env_name": ctx.scenario.env_name,
        "cache_sensitive_env": {
            key: str(ctx.deploy_env.get(key, "")).strip() for key in _FIXTURE_PREPARE_CACHE_ENV_KEYS
        },
    }
    return json.dumps(payload, sort_keys=True, separators=(",", ":"))


def _normalize_prepared_images(prepared_images: list[str]) -> list[str]:
    normalized: list[str] = []
    for image_ref in prepared_images:
        value = str(image_ref).strip()
        if value == "":
            raise RuntimeError("fixture image ensure response contains empty image reference")
        normalized.append(value)
    return normalized
