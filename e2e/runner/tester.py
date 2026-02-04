# Where: e2e/runner/tester.py
# What: Pytest execution for E2E environments.
# Why: Keep test invocation isolated and reusable with progress hooks.
from __future__ import annotations

import re
import sys
from typing import Callable

from e2e.runner.logging import LogSink, run_and_stream
from e2e.runner.models import RunContext
from e2e.runner.utils import PROJECT_ROOT

_COLLECT_RE = re.compile(r"collected\s+(\d+)\s+items")
_TEST_RESULT_RE = re.compile(r"\b(PASSED|FAILED|SKIPPED|XFAIL|XPASS)\b")


def run_pytest(
    ctx: RunContext,
    *,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
    on_progress: Callable[[int, int | None], None] | None = None,
) -> None:
    cmd = (
        [
            sys.executable,
            "-m",
            "pytest",
            "--compose-file",
            str(ctx.compose_file),
        ]
        + ctx.scenario.targets
        + ["-v"]
    )
    for excl in ctx.scenario.exclude:
        cmd.extend(["--ignore", excl])

    total: int | None = None
    current = 0

    def _handle_line(line: str) -> None:
        nonlocal total, current
        collected = _COLLECT_RE.search(line)
        if collected:
            total = int(collected.group(1))
            if on_progress:
                on_progress(current, total)
            return
        if "::" in line and _TEST_RESULT_RE.search(line):
            current += 1
            if on_progress:
                on_progress(current, total)

    rc = run_and_stream(
        cmd,
        cwd=PROJECT_ROOT,
        env=ctx.pytest_env,
        log=log,
        printer=printer,
        on_line=_handle_line,
    )
    if rc != 0:
        raise RuntimeError(f"pytest failed with exit code {rc}")
