# Where: e2e/runner/deploy.py
# What: Deployment execution for E2E environments.
# Why: Keep deploy logic separate from lifecycle and test orchestration.
from __future__ import annotations

import time
from pathlib import Path
from typing import Any, Callable

from e2e.runner.logging import LogSink, run_and_stream
from e2e.runner.models import RunContext
from e2e.runner.utils import PROJECT_ROOT, build_esb_cmd


def deploy_templates(
    ctx: RunContext,
    templates: list[Path],
    *,
    no_cache: bool,
    verbose: bool,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    for idx, tmpl in enumerate(templates, start=1):
        label = f"{ctx.scenario.env_name}"
        if len(templates) > 1:
            label = f"{label} ({idx}/{len(templates)})"
        message = f"Deploying functions for {label}..."
        log.write_line(message)
        if printer:
            printer(message)

        args = [
            "deploy",
            "--template",
            str(tmpl),
            "--compose-file",
            str(ctx.compose_file),
            "--no-deps",
            "--no-save-defaults",
            "--env",
            ctx.scenario.env_name,
            "--mode",
            ctx.scenario.mode,
        ]
        image_prewarm = str(ctx.scenario.extra.get("image_prewarm", "")).strip().lower()
        if image_prewarm:
            args.extend(["--image-prewarm", image_prewarm])
        args.extend(_build_image_override_args(ctx.scenario.extra))
        if no_cache:
            args.append("--no-cache")
        if verbose and "--verbose" not in args and "-v" not in args:
            try:
                idx = args.index("deploy")
                args.insert(idx + 1, "--verbose")
            except ValueError:
                pass
        cmd = build_esb_cmd(args, ctx.env_file, env=ctx.deploy_env)
        rc = run_and_stream(
            cmd,
            cwd=PROJECT_ROOT,
            env=ctx.deploy_env,
            log=log,
            printer=printer,
        )
        if rc != 0:
            raise RuntimeError(f"deploy failed with exit code {rc}")
        time.sleep(2.0)
        log.write_line("Done")
        if printer:
            printer("Done")


def _build_image_override_args(extra: dict[str, Any]) -> list[str]:
    args: list[str] = []
    for value in _collect_function_overrides(
        extra.get("image_uri_overrides"), "image_uri_overrides"
    ):
        args.extend(["--image-uri", value])
    for value in _collect_function_overrides(
        extra.get("image_runtime_overrides"), "image_runtime_overrides"
    ):
        args.extend(["--image-runtime", value])
    return args


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
