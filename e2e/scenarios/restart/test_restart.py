"""
Where: e2e/scenarios/restart/test_restart.py
What: Verify auto-restart behavior after service process crashes.
Why: Ensure gateway/agent recover automatically with restart policy unless-stopped.
"""

from __future__ import annotations

import json
import os
import subprocess
import time
from typing import Any

import pytest

from e2e.conftest import (
    DEFAULT_REQUEST_TIMEOUT,
    GATEWAY_URL,
    VERIFY_SSL,
    build_control_compose_command,
    request_with_retry,
    wait_for_gateway_ready,
)

_SERVICES = ("gateway", "agent")
_RESTART_WAIT_SECONDS = 45


def _inspect_container(container_id: str) -> dict[str, Any]:
    result = subprocess.run(
        ["docker", "inspect", container_id],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, f"docker inspect failed for {container_id}: {result.stderr}"
    payload = json.loads(result.stdout)
    assert payload and isinstance(payload, list), (
        f"unexpected docker inspect output: {result.stdout}"
    )
    return payload[0]


def _resolve_container_id(service: str) -> str:
    project_name = os.getenv("PROJECT_NAME")
    assert project_name, "PROJECT_NAME must be set by E2E runner"

    cmd = build_control_compose_command(
        ["ps", "-q", service],
        project_name=project_name,
    )
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    assert result.returncode == 0, (
        f"docker compose ps failed for service={service}: {result.stderr}"
    )

    container_id = result.stdout.strip()
    assert container_id, f"container id not found for service={service}"
    return container_id


def _crash_service_process(container_id: str, service: str) -> None:
    if service == "agent":
        crash_cmd = (
            'pid="$(for p in /proc/[0-9]*/comm; do '
            'name="$(cat "$p" 2>/dev/null || true)"; '
            'if [ "$name" = "agent" ]; then echo "${p#/proc/}" | cut -d/ -f1; fi; '
            'done | head -n1)"; '
            '[ -n "$pid" ] || { echo "agent process not found" >&2; exit 1; }; '
            'kill -TERM "$pid"'
        )
    else:
        crash_cmd = "kill -TERM 1"

    result = subprocess.run(
        ["docker", "exec", container_id, "sh", "-lc", crash_cmd],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, (
        f"failed to crash service={service} container={container_id}: "
        f"stdout={result.stdout} stderr={result.stderr}"
    )


def _wait_for_restart(
    container_id: str,
    *,
    before_restart_count: int,
    before_started_at: str,
    timeout: int = _RESTART_WAIT_SECONDS,
) -> dict[str, Any]:
    deadline = time.time() + timeout
    last_state = ""
    last_restart_count = before_restart_count
    last_started_at = before_started_at

    while time.time() < deadline:
        details = _inspect_container(container_id)
        status = str(details.get("State", {}).get("Status", ""))
        restart_count = int(details.get("RestartCount", 0))
        started_at = str(details.get("State", {}).get("StartedAt", ""))

        last_state = status
        last_restart_count = restart_count
        last_started_at = started_at

        restarted = restart_count > before_restart_count and started_at != before_started_at
        if status == "running" and restarted:
            return details

        time.sleep(1)

    raise AssertionError(
        "service did not restart in time: "
        f"container={container_id} "
        f"status={last_state} restart_count={last_restart_count} started_at={last_started_at}"
    )


def _assert_echo_ok(auth_token: str, message: str) -> None:
    response = request_with_retry(
        "post",
        f"{GATEWAY_URL}/api/echo",
        max_retries=8,
        retry_interval=2.0,
        timeout=DEFAULT_REQUEST_TIMEOUT,
        json={"message": message},
        headers={"Authorization": f"Bearer {auth_token}"},
        verify=VERIFY_SSL,
    )
    assert response.status_code == 200, (
        f"echo request failed: status={response.status_code} body={response.text}"
    )
    payload = response.json()
    assert payload.get("success") is True, f"echo response error: {payload}"


class TestRestart:
    """Verify gateway/agent auto-recovery after process crashes."""

    @pytest.mark.parametrize("service", _SERVICES)
    def test_service_process_crash_recovers(self, auth_token: str, service: str) -> None:
        wait_for_gateway_ready(timeout=DEFAULT_REQUEST_TIMEOUT)
        _assert_echo_ok(auth_token, f"pre-restart-{service}")

        container_id = _resolve_container_id(service)
        before = _inspect_container(container_id)

        restart_policy = str(before.get("HostConfig", {}).get("RestartPolicy", {}).get("Name", ""))
        assert restart_policy == "unless-stopped", (
            f"service={service} restart policy must be unless-stopped, got={restart_policy}"
        )

        before_status = str(before.get("State", {}).get("Status", ""))
        assert before_status == "running", f"service={service} is not running before crash"
        before_restart_count = int(before.get("RestartCount", 0))
        before_started_at = str(before.get("State", {}).get("StartedAt", ""))

        _crash_service_process(container_id, service)

        _wait_for_restart(
            container_id,
            before_restart_count=before_restart_count,
            before_started_at=before_started_at,
        )

        wait_for_gateway_ready(timeout=DEFAULT_REQUEST_TIMEOUT)
        _assert_echo_ok(auth_token, f"post-restart-{service}")
