# Where: e2e/runner/deploy.py
# What: Deployment execution for E2E environments.
# Why: Keep deploy logic separate from lifecycle and test orchestration.
from __future__ import annotations

import time
from pathlib import Path
from typing import Callable

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
            "--template",
            str(tmpl),
            "deploy",
            "--compose-file",
            str(ctx.compose_file),
            "--no-save-defaults",
            "--env",
            ctx.scenario.env_name,
            "--mode",
            ctx.scenario.mode,
        ]
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
