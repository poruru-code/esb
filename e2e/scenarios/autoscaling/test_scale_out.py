"""
Scale-Out E2E Tests

Validate API behavior under concurrent load. Internal container counts are
covered by system-level tests outside E2E.
"""

import concurrent.futures
import os
import time

from e2e.conftest import call_api
from e2e.scenarios.autoscaling.pool_metrics import wait_for_pool_entry

# Check MAX_CAPACITY from environment for load sizing.
DEFAULT_MAX_CAPACITY = int(os.environ.get("DEFAULT_MAX_CAPACITY", 1))
SCALING_POOL_NAMES = {"scaling", "lambda-scaling"}


class TestScaleOut:
    """
    Tests for scale-related behavior under concurrent load.
    These are external API checks; internal container behavior is validated elsewhere.
    """

    def test_concurrent_requests_under_load(self, auth_token):
        """
        Verify that concurrent requests succeed under load.

        Steps:
        1. Send MAX_CAPACITY concurrent requests with long execution
        2. Verify that all requests succeed
        """
        max_capacity = max(1, DEFAULT_MAX_CAPACITY)
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

            # Allow requests to overlap so queueing/throughput is exercised.
            time.sleep(3)
            expected_min_workers = 1 if max_capacity <= 1 else 2
            pool_entry = wait_for_pool_entry(
                auth_token,
                SCALING_POOL_NAMES,
                predicate=lambda entry: entry.get("total_workers", 0) >= expected_min_workers,
                timeout_seconds=10,
            )
            assert pool_entry["total_workers"] >= 1
            assert pool_entry["total_workers"] <= pool_entry["max_capacity"]
            assert pool_entry["max_capacity"] == max_capacity

            # Wait for all requests to complete
            results = [f.result() for f in futures]

        # 2. Verify all requests succeeded
        print(f"[Step 3] Verifying {len(results)} responses...")
        for i, res in enumerate(results):
            assert res.status_code == 200, (
                f"Request {i} failed: {res.status_code} - {res.text[:100]}"
            )
            assert res.json()["message"] == "scaling-test"

        print("[OK] Concurrent load test passed.")

    def test_over_capacity_requests_succeed(self, auth_token):
        """
        Verify that requests beyond configured capacity still succeed (queued).

        Steps:
        1. Send more requests than MAX_CAPACITY
        2. Verify all requests succeed
        """
        max_capacity = max(1, DEFAULT_MAX_CAPACITY)
        num_requests = max_capacity * 2  # Send double the capacity
        print(
            f"\n[Capacity Limit] Testing {num_requests} requests against "
            f"MAX_CAPACITY={max_capacity}"
        )

        def invoke(req_id: int):
            return call_api(
                "/api/scaling",
                auth_token,
                {"message": f"capacity-limit-{req_id}", "sleep_ms": 500},
                timeout=60,
            )

        with concurrent.futures.ThreadPoolExecutor(max_workers=num_requests) as executor:
            futures = [executor.submit(invoke, i) for i in range(num_requests)]

            # Collect results
            results = [f.result() for f in futures]

        # All requests should succeed (some may queue)
        success_count = sum(1 for r in results if r.status_code == 200)
        print(f"[Result] {success_count}/{num_requests} requests succeeded")

        assert success_count == num_requests, (
            f"Expected all {num_requests} requests to succeed, got {success_count}"
        )
        pool_entry = wait_for_pool_entry(auth_token, SCALING_POOL_NAMES, timeout_seconds=10)
        assert pool_entry["total_workers"] <= pool_entry["max_capacity"]
        if max_capacity > 1:
            assert pool_entry["total_workers"] >= 2
