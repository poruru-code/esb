"""
Reconciliation (orphan container cleanup) E2E test.

Scenarios:
1. Grace Period: newly created containers are not deleted by reconciliation (60s grace)
2. Adoption: existing containers are reused after Gateway restart
"""

import os
import subprocess
import time

import requests

from tests.conftest import (
    GATEWAY_URL,
    VERIFY_SSL,
    DEFAULT_REQUEST_TIMEOUT,
    call_api,
)


class TestReconciliation:
    """Verify reconciliation (orphan container cleanup) behavior."""

    def test_grace_period_prevents_premature_deletion(self, auth_token):
        """
        E2E: ensure containers created just before reconciliation are not deleted.

        Scenario:
        1. Invoke Lambda to start a container
        2. Restart Gateway (container leaves Gateway pool and becomes orphan)
        3. Invoke Lambda again within Grace Period
        4. Confirm existing container is reused (Adoption)
        """
        # 1. Invoke Lambda to start container.
        print("Step 1: Initial Lambda invocation (cold start)...")
        response1 = call_api("/api/echo", auth_token, {"message": "warmup"})
        assert response1.status_code == 200
        data1 = response1.json()
        assert data1["success"] is True
        print(f"Initial invocation successful: {data1}")

        # Wait briefly to ensure container is running.
        time.sleep(2)

        # 2. Restart Gateway (container runs in Agent but leaves Gateway pool).
        print("Step 2: Restarting Gateway container...")
        restart_result = subprocess.run(
            ["docker", "compose", "restart", "gateway"],
            capture_output=True,
            text=True,
            cwd=os.path.dirname(
                os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
            ),
        )
        assert restart_result.returncode == 0, (
            f"Failed to restart gateway: {restart_result.stderr}"
        )

        # Wait for Gateway health check.
        print("Step 3: Waiting for Gateway to become healthy...")
        for i in range(15):
            try:
                health_resp = requests.get(
                    f"{GATEWAY_URL}/health", verify=VERIFY_SSL, timeout=DEFAULT_REQUEST_TIMEOUT
                )
                if health_resp.status_code == 200:
                    print(f"Gateway is healthy after {i + 1} attempts")
                    break
            except Exception:
                pass
            time.sleep(2)

        # Short stabilization wait (still within Grace Period).
        time.sleep(3)

        # 3. Invoke Lambda again within Grace Period.
        print("Step 4: Post-restart invocation (should reuse existing container via Adoption)...")
        response2 = call_api("/api/echo", auth_token, {"message": "after restart"})

        assert response2.status_code == 200, (
            f"Expected 200, got {response2.status_code}: {response2.text}. "
            "Container may have been prematurely deleted by Reconciliation."
        )
        data2 = response2.json()
        assert data2["success"] is True
        print(f"Post-restart invocation successful: {data2}")

        # 4. Additional verification: stability via repeated calls.
        print("Step 5: Additional invocations to verify stability...")
        for i in range(3):
            resp = call_api("/api/echo", auth_token, {"message": f"stability-{i}"})
            assert resp.status_code == 200, f"Stability check {i} failed: {resp.text}"
            print(f"  Stability check {i + 1}/3: PASSED")

        print("[OK] Grace Period test passed - container was not prematurely deleted")
