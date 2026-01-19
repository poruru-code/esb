"""
Scale-to-Zero E2E Tests

Validate external API behavior around idle timeout windows.
These tests require specific environment configuration and may take several minutes.

Usage:
    GATEWAY_IDLE_TIMEOUT_SECONDS=60 pytest tests/scenarios/autoscaling/test_scale_to_zero.py -v
"""

import os
import time

import pytest

from e2e.conftest import call_api
from e2e.scenarios.autoscaling.pool_metrics import wait_for_pool_entry

# Skip this module unless idle timeout is set to a short value
IDLE_TIMEOUT_MINUTES = int(os.environ.get("IDLE_TIMEOUT_MINUTES", 5))
IDLE_TIMEOUT_SECONDS = max(
    1, int(os.environ.get("GATEWAY_IDLE_TIMEOUT_SECONDS", IDLE_TIMEOUT_MINUTES * 60))
)
HEARTBEAT_INTERVAL = int(os.environ.get("HEARTBEAT_INTERVAL", 30))
BUFFER_SECONDS = min(30, max(10, HEARTBEAT_INTERVAL * 2))
SCALING_POOL_NAMES = {"scaling", "lambda-scaling"}
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

    These tests verify that APIs remain responsive across idle windows,
    without depending on internal container state.

    IMPORTANT: These tests require:
    - GATEWAY_IDLE_TIMEOUT_SECONDS <= 120 (or IDLE_TIMEOUT_MINUTES <= 2)
    """

    def test_invocation_after_idle_window(self, auth_token):
        """
        Verify that APIs remain reachable after the idle timeout window.

        Steps:
        1. Invoke Lambda once
        2. Wait for IDLE_TIMEOUT + buffer
        3. Invoke again and verify success
        """
        print(
            "\n[Scale-to-Zero] Testing with "
            f"IDLE_TIMEOUT_SECONDS={IDLE_TIMEOUT_SECONDS} "
            f"(buffer={BUFFER_SECONDS}s)"
        )

        # 1. Initial invocation
        print("[Step 1] Invoking Lambda...")
        response = call_api(
            "/api/scaling", auth_token, {"message": "scale-to-zero-test", "sleep_ms": 100}
        )
        assert response.status_code == 200, f"Lambda invocation failed: {response.text}"
        assert response.json()["message"] == "scaling-test"
        pool_entry = wait_for_pool_entry(
            auth_token,
            SCALING_POOL_NAMES,
            predicate=lambda entry: entry.get("total_workers", 0) >= 1,
            timeout_seconds=10,
        )
        assert pool_entry["total_workers"] >= 1

        # 2. Wait for idle timeout
        # Add 30 seconds buffer for cleanup scheduler delay
        wait_time = IDLE_TIMEOUT_SECONDS + BUFFER_SECONDS
        print(f"[Step 2] Waiting {wait_time}s for idle timeout window...")
        time.sleep(wait_time)

        # 3. Invoke again and verify success
        pool_entry = wait_for_pool_entry(
            auth_token,
            SCALING_POOL_NAMES,
            predicate=lambda entry: entry.get("total_workers", 0) == 0,
            timeout_seconds=20,
            interval_seconds=2,
        )
        assert pool_entry["total_workers"] == 0
        print("[Step 3] Invoking Lambda after idle window...")
        response = call_api(
            "/api/scaling", auth_token, {"message": "scale-to-zero-after", "sleep_ms": 0}
        )
        assert response.status_code == 200, f"Post-idle invocation failed: {response.text}"
        assert response.json()["message"] == "scaling-test"

    def test_periodic_requests_across_idle_window(self, auth_token):
        """
        Verify that periodic requests remain successful across the idle window.

        Steps:
        1. Invoke Lambda once
        2. Send periodic requests for longer than IDLE_TIMEOUT
        3. Verify all requests succeed
        """
        print(
            "\n[Active Container] Testing with "
            f"IDLE_TIMEOUT_SECONDS={IDLE_TIMEOUT_SECONDS} "
            f"(buffer={BUFFER_SECONDS}s)"
        )

        # 1. Initial invocation
        print("[Step 1] Invoking Lambda...")
        response = call_api(
            "/api/scaling", auth_token, {"message": "active-test-init", "sleep_ms": 100}
        )
        assert response.status_code == 200
        assert response.json()["message"] == "scaling-test"
        pool_entry = wait_for_pool_entry(
            auth_token,
            SCALING_POOL_NAMES,
            predicate=lambda entry: entry.get("total_workers", 0) >= 1,
            timeout_seconds=10,
        )
        assert pool_entry["total_workers"] >= 1

        # 2. Keep container active with periodic requests
        # Wait slightly longer than idle timeout, but send requests frequently
        total_wait = IDLE_TIMEOUT_SECONDS + BUFFER_SECONDS
        # Higher frequency (every 10s or less) to avoid Janitor races
        request_interval = max(1, min(10, IDLE_TIMEOUT_SECONDS // 3))
        if request_interval >= IDLE_TIMEOUT_SECONDS:
            request_interval = max(1, IDLE_TIMEOUT_SECONDS - 1)
        elapsed = 0
        mid_check_done = False

        print(f"[Step 2] Keeping container active for {total_wait}s...")
        while elapsed < total_wait:
            time.sleep(request_interval)
            elapsed += request_interval

            # Send a keep-alive request
            response = call_api(
                "/api/scaling", auth_token, {"message": f"keepalive-{elapsed}"}, timeout=30
            )

            print(f"  [{elapsed}s] Request status: {response.status_code}")

            assert response.status_code == 200, "Keep-alive request should succeed"
            assert response.json()["message"] == "scaling-test"

            if elapsed >= total_wait / 2 and not mid_check_done:
                pool_entry = wait_for_pool_entry(
                    auth_token,
                    SCALING_POOL_NAMES,
                    predicate=lambda entry: entry.get("total_workers", 0) >= 1,
                    timeout_seconds=10,
                )
                assert pool_entry["total_workers"] >= 1
                mid_check_done = True

        print("[Step 3] Periodic requests completed successfully.")
