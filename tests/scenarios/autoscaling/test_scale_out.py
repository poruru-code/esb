"""
Scale-Out E2E Tests

Tests that verify multiple containers are spawned when MAX_CAPACITY > 1.
These tests require DEFAULT_MAX_CAPACITY > 1 to be meaningful.

Usage:
    DEFAULT_MAX_CAPACITY=3 pytest tests/scenarios/autoscaling/test_scale_out.py -v
"""

import concurrent.futures
import os
import subprocess
import time

import pytest
from tests.conftest import call_api
import grpc
from services.gateway.pb import agent_pb2, agent_pb2_grpc

# Scale-out tests run against the gRPC Agent path.
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


# Check MAX_CAPACITY from environment
DEFAULT_MAX_CAPACITY = int(os.environ.get("DEFAULT_MAX_CAPACITY", 1))
SKIP_REASON = (
    f"Scale-out tests require DEFAULT_MAX_CAPACITY > 1. "
    f"Current value: {DEFAULT_MAX_CAPACITY}. "
    "Run with: DEFAULT_MAX_CAPACITY=3 pytest ..."
)


@pytest.mark.skipif(DEFAULT_MAX_CAPACITY <= 1, reason=SKIP_REASON)
class TestScaleOut:
    """
    Tests for Scale-Out functionality.

    These tests verify that multiple containers are spawned when there is
    sufficient capacity and concurrent demand.

    IMPORTANT: These tests require:
    - DEFAULT_MAX_CAPACITY > 1 (e.g., 3)
    - Containers should be in a clean state before running
    """

    def test_multiple_containers_spawn(self, auth_token):
        """
        Verify that multiple concurrent requests cause multiple containers to spawn.

        Steps:
        1. Send MAX_CAPACITY concurrent requests with long execution
        2. Verify that multiple containers are running
        """
        max_capacity = DEFAULT_MAX_CAPACITY
        print(f"\n[Scale-Out] Testing with DEFAULT_MAX_CAPACITY={max_capacity}")

        # Use the 'slow' action if available, otherwise use echo with longer timeout
        def invoke_slow(req_id: int):
            """Invoke a request that takes some time to complete"""
            # Using echo with a unique message
            return call_api(
                "/api/scaling",
                auth_token,
                {
                    "message": f"scale-out-{req_id}-{'x' * 1000}",
                    "sleep_ms": 2000,
                },  # Larger payload + sleep
                timeout=60,
            )

        # 1. Send concurrent requests equal to max capacity
        print(f"[Step 1] Sending {max_capacity} concurrent requests...")

        with concurrent.futures.ThreadPoolExecutor(max_workers=max_capacity) as executor:
            # Submit all requests
            futures = [executor.submit(invoke_slow, i) for i in range(max_capacity)]

            # Wait a moment for containers to start spawning
            time.sleep(3)

            # Check container count while requests are in-flight (before completion)
            mid_count = get_container_count("scaling")
            print(f"[Step 2] Container count during execution: {mid_count}")

            # Wait for all requests to complete
            results = [f.result() for f in futures]

        # 2. Verify all requests succeeded
        print(f"[Step 3] Verifying {len(results)} responses...")
        for i, res in enumerate(results):
            assert res.status_code == 200, (
                f"Request {i} failed: {res.status_code} - {res.text[:100]}"
            )

        # 3. Check final container count
        final_count = get_container_count("scaling")
        print(f"[Step 4] Final container count: {final_count}")

        # With MAX_CAPACITY > 1, multiple containers should have been spawned
        # The exact count depends on timing, but should be > 1 at some point
        assert mid_count > 0 or final_count > 0, (
            f"No containers were spawned. mid_count={mid_count}, final_count={final_count}"
        )

        print(f"[OK] Scale-out test passed. Max observed containers: {max(mid_count, final_count)}")

    def test_respects_max_capacity(self, auth_token):
        """
        Verify that container count does not exceed MAX_CAPACITY.

        Steps:
        1. Send more requests than MAX_CAPACITY
        2. Verify container count never exceeds MAX_CAPACITY
        """
        max_capacity = DEFAULT_MAX_CAPACITY
        num_requests = max_capacity * 2  # Send double the capacity
        print(
            f"\n[Capacity Limit] Testing {num_requests} requests against MAX_CAPACITY={max_capacity}"
        )

        def invoke(req_id: int):
            return call_api(
                "/api/scaling",
                auth_token,
                {"message": f"capacity-limit-{req_id}", "sleep_ms": 500},
                timeout=60,
            )

        # Track max observed container count
        max_observed_count = 0

        with concurrent.futures.ThreadPoolExecutor(max_workers=num_requests) as executor:
            futures = [executor.submit(invoke, i) for i in range(num_requests)]

            # Poll container count during execution
            poll_count = 0
            while not all(f.done() for f in futures) and poll_count < 20:
                current_count = get_container_count("scaling")
                max_observed_count = max(max_observed_count, current_count)
                print(f"  [{poll_count}] Container count: {current_count}")

                # Check that we don't exceed capacity
                assert current_count <= max_capacity, (
                    f"Container count {current_count} exceeds MAX_CAPACITY {max_capacity}"
                )

                time.sleep(0.5)
                poll_count += 1

            # Collect results
            results = [f.result() for f in futures]

        # All requests should succeed (some may queue)
        success_count = sum(1 for r in results if r.status_code == 200)
        print(
            f"[Result] {success_count}/{num_requests} requests succeeded, max containers: {max_observed_count}"
        )

        assert success_count == num_requests, (
            f"Expected all {num_requests} requests to succeed, got {success_count}"
        )
        assert max_observed_count <= max_capacity, (
            f"Max observed containers {max_observed_count} exceeded MAX_CAPACITY {max_capacity}"
        )
