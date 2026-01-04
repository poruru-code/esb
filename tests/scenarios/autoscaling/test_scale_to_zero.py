"""
Scale-to-Zero E2E Tests

Tests container cleanup after idle timeout.
These tests require specific environment configuration and may take several minutes.

NOTE: These tests run against the gRPC Agent path.

Usage:
    GATEWAY_IDLE_TIMEOUT_SECONDS=60 pytest tests/scenarios/autoscaling/test_scale_to_zero.py -v
"""

import os
import subprocess
import time

import pytest
from tests.conftest import call_api
import grpc
from services.gateway.pb import agent_pb2, agent_pb2_grpc

GRPC_TIMEOUT_SECONDS = float(os.environ.get("GRPC_TIMEOUT_SECONDS", "1.0"))


def _normalize_function_name(function_name: str) -> str:
    if function_name.startswith("lambda-"):
        return function_name
    return f"lambda-{function_name}"


def _grpc_list_containers():
    address = os.environ.get("AGENT_GRPC_ADDRESS", "localhost:50051")
    with grpc.insecure_channel(address) as channel:
        grpc.channel_ready_future(channel).result(timeout=GRPC_TIMEOUT_SECONDS)
        stub = agent_pb2_grpc.AgentServiceStub(channel)
        resp = stub.ListContainers(
            agent_pb2.ListContainersRequest(), timeout=GRPC_TIMEOUT_SECONDS
        )
        return resp.containers


def get_container_ids(function_name: str) -> list[str]:
    """Get container IDs for a function name pattern"""
    target = _normalize_function_name(function_name)
    try:
        return [
            c.container_id for c in _grpc_list_containers() if c.function_name == target
        ]
    except (grpc.RpcError, grpc.FutureTimeoutError):
        # Fallback for local debugging when gRPC is not reachable.
        cmd = ["docker", "ps", "-q", "-f", f"name=lambda-{function_name}"]
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        return result.stdout.strip().splitlines()


def get_container_count(function_name: str) -> int:
    """Get the count of running containers for a function"""
    return len(get_container_ids(function_name))


# Skip this module unless idle timeout is set to a short value
IDLE_TIMEOUT_MINUTES = int(os.environ.get("IDLE_TIMEOUT_MINUTES", 5))
IDLE_TIMEOUT_SECONDS = max(
    1, int(os.environ.get("GATEWAY_IDLE_TIMEOUT_SECONDS", IDLE_TIMEOUT_MINUTES * 60))
)
HEARTBEAT_INTERVAL = int(os.environ.get("HEARTBEAT_INTERVAL", 30))
BUFFER_SECONDS = min(30, max(10, HEARTBEAT_INTERVAL * 2))
SKIP_REASON = (
    "Scale-to-zero tests require idle timeout <= 120s. "
    f"Current value: {IDLE_TIMEOUT_SECONDS}s. "
    "Run with: GATEWAY_IDLE_TIMEOUT_SECONDS=60 (or less) pytest ..."
)


@pytest.mark.slow
@pytest.mark.skipif(IDLE_TIMEOUT_SECONDS > 120, reason=SKIP_REASON)
class TestScaleToZero:
    """
    Tests for Scale-to-Zero functionality.

    These tests verify that containers are automatically cleaned up after
    being idle for the configured timeout period.

    IMPORTANT: These tests require:
    - GATEWAY_IDLE_TIMEOUT_SECONDS <= 120 (or IDLE_TIMEOUT_MINUTES <= 2)
    - Containers must be in a clean state before running
    """

    def test_idle_container_cleanup(self, auth_token):
        """
        Verify that an idle container is cleaned up after IDLE_TIMEOUT.

        Steps:
        1. Invoke Lambda to provision a container
        2. Verify container is running
        3. Wait for IDLE_TIMEOUT + buffer
        4. Verify container is removed
        """
        print(
            "\n[Scale-to-Zero] Testing with "
            f"IDLE_TIMEOUT_SECONDS={IDLE_TIMEOUT_SECONDS} "
            f"(buffer={BUFFER_SECONDS}s)"
        )

        # 1. Provision a container
        print("[Step 1] Invoking Lambda to provision container...")
        response = call_api(
            "/api/scaling", auth_token, {"message": "scale-to-zero-test", "sleep_ms": 100}
        )
        assert response.status_code == 200, f"Lambda invocation failed: {response.text}"

        # 2. Verify container is running
        time.sleep(2)  # Allow container state to stabilize
        initial_count = get_container_count("scaling")
        print(f"[Step 2] Container count after invocation: {initial_count}")
        assert initial_count >= 1, "Container should be running after invocation"

        # 3. Wait for idle timeout
        # Add 30 seconds buffer for cleanup scheduler delay
        wait_time = IDLE_TIMEOUT_SECONDS + BUFFER_SECONDS
        print(f"[Step 3] Waiting {wait_time}s for idle timeout and cleanup...")

        # Poll periodically to detect early cleanup
        elapsed = 0
        poll_interval = min(15, max(5, IDLE_TIMEOUT_SECONDS // 2))
        while elapsed < wait_time:
            time.sleep(poll_interval)
            elapsed += poll_interval
            current_count = get_container_count("scaling")
            print(f"  [{elapsed}s] Container count: {current_count}")

            if current_count == 0:
                print(f"[Step 4] Container cleaned up after ~{elapsed}s")
                return  # SUCCESS

        # 4. Final check
        final_count = get_container_count("scaling")
        print(f"[Step 4] Final container count: {final_count}")

        assert final_count == 0, (
            f"Container should be cleaned up after {IDLE_TIMEOUT_SECONDS}s idle. "
            f"Still running: {final_count} containers"
        )

    def test_active_container_not_cleaned(self, auth_token):
        """
        Verify that containers receiving requests are NOT cleaned up.

        Steps:
        1. Invoke Lambda to provision a container
        2. Send periodic requests to keep it active
        3. Verify container remains running past IDLE_TIMEOUT
        """
        print(
            "\n[Active Container] Testing with "
            f"IDLE_TIMEOUT_SECONDS={IDLE_TIMEOUT_SECONDS} "
            f"(buffer={BUFFER_SECONDS}s)"
        )

        # 1. Provision a container
        print("[Step 1] Invoking Lambda to provision container...")
        response = call_api(
            "/api/scaling", auth_token, {"message": "active-test-init", "sleep_ms": 100}
        )
        assert response.status_code == 200

        time.sleep(2)
        initial_ids = get_container_ids("scaling")
        assert len(initial_ids) >= 1, "Container should be running"
        initial_id = initial_ids[0] if initial_ids else None
        print(f"[Step 1] Initial container ID: {initial_id}")

        # 2. Keep container active with periodic requests
        # Wait slightly longer than idle timeout, but send requests frequently
        total_wait = IDLE_TIMEOUT_SECONDS + BUFFER_SECONDS
        # Higher frequency (every 10s or less) to avoid Janitor races
        request_interval = max(1, min(10, IDLE_TIMEOUT_SECONDS // 3))
        if request_interval >= IDLE_TIMEOUT_SECONDS:
            request_interval = max(1, IDLE_TIMEOUT_SECONDS - 1)
        elapsed = 0

        print(f"[Step 2] Keeping container active for {total_wait}s...")
        while elapsed < total_wait:
            time.sleep(request_interval)
            elapsed += request_interval

            # Send a keep-alive request
            response = call_api(
                "/api/scaling", auth_token, {"message": f"keepalive-{elapsed}"}, timeout=30
            )
            current_ids = get_container_ids("scaling")

            print(
                f"  [{elapsed}s] Request status: {response.status_code}, Containers: {len(current_ids)}"
            )

            assert response.status_code == 200, "Keep-alive request should succeed"
            assert len(current_ids) >= 1, "Container should still be running"

        # 3. Final verification
        final_ids = get_container_ids("scaling")
        print(f"[Step 3] Final container count: {len(final_ids)}")

        assert len(final_ids) >= 1, "Active container should NOT be cleaned up"

        # Verify it's the same container (reuse)
        if initial_id and final_ids:
            assert initial_id == final_ids[0], (
                f"Container should be reused, not recreated. "
                f"Initial: {initial_id}, Final: {final_ids[0]}"
            )
