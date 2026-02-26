"""Registry readiness and diagnostics helpers."""

from __future__ import annotations

import time
import urllib.request
from pathlib import Path

from tools.deployops.core.compose import compose_base_cmd
from tools.deployops.core.runner import CommandRunner


def registry_v2_ready(host_port: str, timeout: float = 2.0) -> tuple[bool, str]:
    url = f"http://{host_port}/v2/"
    request = urllib.request.Request(url, method="GET")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(request, timeout=timeout) as response:
            status = int(response.status)
            if status in (200, 401):
                return True, str(status)
            return False, str(status)
    except Exception:
        return False, ""


def wait_for_registry_ready(host_port: str, timeout_seconds: int) -> None:
    started = time.monotonic()
    while True:
        ready, _ = registry_v2_ready(host_port, timeout=2.0)
        if ready:
            return
        elapsed = time.monotonic() - started
        if elapsed >= float(timeout_seconds):
            raise TimeoutError(
                f"Registry not responding at http://{host_port}/v2/ after {timeout_seconds}s"
            )
        time.sleep(1)


def emit_registry_diagnostics(
    runner: CommandRunner,
    *,
    project_name: str,
    compose_file: Path,
    env_file: Path | None,
) -> None:
    ps_cmd = compose_base_cmd(
        project_name=project_name,
        compose_file=compose_file,
        env_file=env_file,
    )
    ps_cmd.extend(["ps", "registry"])
    logs_cmd = compose_base_cmd(
        project_name=project_name,
        compose_file=compose_file,
        env_file=env_file,
    )
    logs_cmd.extend(["logs", "--tail=50", "registry"])

    runner.run(ps_cmd, stream_output=True, check=False)
    runner.run(logs_cmd, stream_output=True, check=False)
