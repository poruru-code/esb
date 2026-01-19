"""
Auto-Scaling E2E Tests
Validate external API behavior under repeated and concurrent requests.
"""

import concurrent.futures

from e2e.conftest import call_api
from e2e.scenarios.autoscaling.pool_metrics import wait_for_pool_entry

ECHO_POOL_NAMES = {"echo", "lambda-echo"}
CHAOS_POOL_NAMES = {"chaos", "lambda-chaos"}


class TestAutoScaling:
    """Core autoscaling functionality tests"""

    def test_repeated_invocations(self, auth_token):
        """
        Verify that repeated invocations succeed and return expected results.
        """
        # 1. First Invocation
        response = call_api("/api/echo", auth_token, {"message": "autoscale-1"})
        assert response.status_code == 200
        data = response.json()
        assert data["message"] == "Echo: autoscale-1"
        pool_entry = wait_for_pool_entry(auth_token, ECHO_POOL_NAMES, timeout_seconds=10)
        assert pool_entry["total_workers"] >= 1

        # 2. Second Invocation
        response = call_api("/api/echo", auth_token, {"message": "autoscale-2"})
        assert response.status_code == 200
        data = response.json()
        assert data["message"] == "Echo: autoscale-2"
        pool_entry = wait_for_pool_entry(auth_token, ECHO_POOL_NAMES, timeout_seconds=10)
        assert pool_entry["total_workers"] <= pool_entry["max_capacity"]
        if pool_entry["max_capacity"] == 1:
            assert pool_entry["total_workers"] == 1

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
        pool_entry = wait_for_pool_entry(auth_token, ECHO_POOL_NAMES, timeout_seconds=10)
        assert pool_entry["total_workers"] <= pool_entry["max_capacity"]


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
        pool_entry = wait_for_pool_entry(auth_token, ECHO_POOL_NAMES, timeout_seconds=10)
        assert pool_entry["total_workers"] >= 1
        assert pool_entry["total_workers"] <= pool_entry["max_capacity"]

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

        echo_pool = wait_for_pool_entry(auth_token, ECHO_POOL_NAMES, timeout_seconds=10)
        chaos_pool = wait_for_pool_entry(auth_token, CHAOS_POOL_NAMES, timeout_seconds=10)
        assert echo_pool["total_workers"] >= 1
        assert chaos_pool["total_workers"] >= 1
