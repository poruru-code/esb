"""
Auto-Scaling E2E Tests
Tests the Pool behavior, Reuse, and Concurrent Request Handling.
"""

import concurrent.futures
import subprocess

import pytest
from tests.conftest import call_api


def get_container_ids(function_name: str) -> list[str]:
    """Get container IDs for a function name pattern"""
    cmd = ["docker", "ps", "-q", "-f", f"name=lambda-{function_name}"]
    result = subprocess.run(cmd, capture_output=True, text=True, check=True)
    return result.stdout.strip().splitlines()


def get_container_count(function_name: str) -> int:
    """Get the count of running containers for a function"""
    return len(get_container_ids(function_name))


def cleanup_lambda_containers() -> None:
    """
    Clean up all lambda containers created by ESB.
    This ensures test idempotency regardless of previous state.
    """
    # Find all containers with our label
    cmd = ["docker", "ps", "-aq", "-f", "label=created_by=esb"]
    result = subprocess.run(cmd, capture_output=True, text=True, check=True)
    container_ids = result.stdout.strip().splitlines()

    if container_ids:
        # Force remove all matching containers
        subprocess.run(
            ["docker", "rm", "-f"] + container_ids,
            capture_output=True,
            check=False,  # Don't fail if some are already removed
        )


@pytest.fixture(scope="module", autouse=True)
def clean_lambda_containers_before_tests():
    """
    Module-scoped fixture that cleans up Lambda containers before running tests.
    This ensures idempotent test execution regardless of previous state.
    """
    cleanup_lambda_containers()
    yield
    # Optionally clean up after tests as well
    # cleanup_lambda_containers()


class TestAutoScaling:
    """Core autoscaling functionality tests"""

    def test_pool_provision_and_reuse(self, auth_token):
        """
        Verify that:
        1. First invocation provisions a container.
        2. Second invocation reuses the SAME container (container ID unchanged).
        """
        # 1. First Invocation
        response = call_api("/api/echo", auth_token, {"message": "autoscale-1"})
        assert response.status_code == 200
        data = response.json()
        assert data["message"] == "Echo: autoscale-1"

        # Check Container ID - exactly one should exist after cleanup
        ids_1 = get_container_ids("echo")
        assert len(ids_1) == 1, f"Expected 1 echo container, found {len(ids_1)}: {ids_1}"
        container_id_1 = ids_1[0]

        # 2. Second Invocation
        response = call_api("/api/echo", auth_token, {"message": "autoscale-2"})
        assert response.status_code == 200

        # Check Container ID again
        ids_2 = get_container_ids("echo")
        assert len(ids_2) == 1, f"Expected 1 echo container, found {len(ids_2)}: {ids_2}"
        container_id_2 = ids_2[0]

        # Assert Reuse
        assert container_id_1 == container_id_2, "Container should be reused in Pool Mode"

    def test_concurrent_queueing(self, auth_token):
        """
        Concurrent requests should be handled successfully.
        (With MAX_CAPACITY=1, they will be serialized by the semaphore).
        """

        def invoke(msg):
            return call_api("/api/echo", auth_token, {"message": msg}, timeout=60)

        with concurrent.futures.ThreadPoolExecutor(max_workers=3) as executor:
            futures = [executor.submit(invoke, f"concurrent-{i}") for i in range(3)]
            results = [f.result() for f in futures]

        # All requests should succeed
        for res in results:
            assert res.status_code == 200, f"Request failed: {res.text}"
            assert "Echo: concurrent-" in res.json()["message"]

        # With MAX_CAPACITY=1, exactly one container should be running
        ids = get_container_ids("echo")
        assert len(ids) == 1, f"Expected 1 echo container, found {len(ids)}"


class TestConcurrentStress:
    """Stress tests with high concurrency"""

    def test_concurrent_stress_10_requests(self, auth_token):
        """
        Send 10 concurrent requests and verify all succeed.
        Tests queue handling under moderate load.
        """
        num_requests = 10

        def invoke(req_id: int):
            return call_api(
                "/api/echo",
                auth_token,
                {"message": f"stress-{req_id}"},
                timeout=60,  # Longer timeout for queued requests
            )

        with concurrent.futures.ThreadPoolExecutor(max_workers=num_requests) as executor:
            futures = [executor.submit(invoke, i) for i in range(num_requests)]
            results = [f.result() for f in concurrent.futures.as_completed(futures)]

        # All requests should succeed
        success_count = sum(1 for r in results if r.status_code == 200)
        failed = [r for r in results if r.status_code != 200]

        assert success_count == num_requests, (
            f"Expected {num_requests} successes, got {success_count}. "
            f"Failed responses: {[(r.status_code, r.text[:100]) for r in failed]}"
        )

        # Verify response content
        for res in results:
            data = res.json()
            assert data["success"] is True
            assert "Echo: stress-" in data["message"]

    def test_concurrent_different_functions(self, auth_token):
        """
        Send concurrent requests to different functions.
        Verifies both functions respond successfully.
        """

        def invoke_echo(msg):
            return call_api("/api/echo", auth_token, {"message": msg}, timeout=60)

        def invoke_faulty_hello():
            return call_api("/api/faulty", auth_token, {"action": "hello"}, timeout=60)

        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as executor:
            futures = [
                executor.submit(invoke_echo, "multi-func-1"),
                executor.submit(invoke_echo, "multi-func-2"),
                executor.submit(invoke_faulty_hello),
                executor.submit(invoke_faulty_hello),
            ]
            results = [f.result() for f in futures]

        # All should succeed
        for i, res in enumerate(results):
            assert res.status_code == 200, f"Request {i} failed: {res.text}"

        # Both functions should have containers running
        # Note: /api/faulty maps to lambda-chaos (not lambda-faulty)
        echo_count = get_container_count("echo")
        chaos_count = get_container_count("chaos")

        assert echo_count >= 1, "Echo container should be running"
        assert chaos_count >= 1, "Chaos container should be running (serves /api/faulty)"
