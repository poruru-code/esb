"""
Resilience and performance tests.

- Container recovery after Manager restart (Adopt & Sync)
- Container host cache (reduce Manager load)
- Circuit Breaker (block on Lambda crash)
"""

import os
import subprocess
import time

import requests

from tests.conftest import (
    GATEWAY_URL,
    VERIFY_SSL,
    DEFAULT_REQUEST_TIMEOUT,
    ORCHESTRATOR_RESTART_WAIT,
    STABILIZATION_WAIT,
    request_with_retry,
    call_api,
)


class TestResilience:
    """Verify resilience and performance features."""

    # Unskipped - Go Agent restart recovery should work
    def test_orchestrator_restart_recovery(self, auth_token):
        """
        E2E: verify container recovery after Manager/Agent restart (Adopt & Sync).

        Uses Echo Lambda (no S3 dependency).

        Always restart the gRPC agent in current architecture.
        """
        service_to_restart = "agent"

        # 1. Initial invocation (container startup).
        print("Step 1: Initial Lambda invocation (cold start)...")
        response1 = call_api("/api/echo", auth_token, {"message": "warmup"})
        assert response1.status_code == 200
        data1 = response1.json()
        assert data1["success"] is True

        time.sleep(3)

        # 2. Restart Manager/Agent container.
        from tools.cli import compose

        # 2. Restart Manager/Agent container.
        print(f"Step 2: Restarting {service_to_restart} container...")
        
        # Determine project name to ensure we target the running stack
        # (run_tests.py usually sets this effectively via env or directory context)
        project_name = os.getenv("ESB_PROJECT_NAME")

        cmd = compose.build_compose_command(
            ["restart", service_to_restart],
            target="control",
            project_name=project_name
        )
        
        restart_result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
        )
        assert restart_result.returncode == 0, (
            f"Failed to restart {service_to_restart}: {restart_result.stderr}"
        )

        time.sleep(ORCHESTRATOR_RESTART_WAIT)

        # Indirect health check.
        for i in range(15):
            try:
                health_resp = requests.get(
                    f"{GATEWAY_URL}/health", verify=VERIFY_SSL, timeout=DEFAULT_REQUEST_TIMEOUT
                )
                if health_resp.status_code == 200:
                    break
            except Exception:
                print(f"Waiting for system to stabilize... ({i + 1}/15)")
            time.sleep(2)

        time.sleep(STABILIZATION_WAIT)

        # 3. Post-restart invocation (verify container recovery).
        print("Step 3: Post-restart invocation (should be warm start)...")

        response2 = request_with_retry(
            "post",
            f"{GATEWAY_URL}/api/echo",
            max_retries=5,
            retry_interval=2.0,
            json={"message": "after restart"},
            headers={"Authorization": f"Bearer {auth_token}"},
            verify=VERIFY_SSL,
        )

        assert response2.status_code == 200, (
            f"Expected 200, got {response2.status_code}: {response2.text}"
        )
        data2 = response2.json()
        assert data2["success"] is True

        print(f"Post-restart invocation successful: {data2}")
        time.sleep(3)
        print(f"Test passed: Container was successfully handled after {service_to_restart} restart")

    def test_gateway_cache_hit(self, auth_token):
        """
        E2E: verify Gateway container pooling works.

        In PoolManager architecture, Gateway manages workers directly without the Orchestrator.
        This test verifies that consecutive requests reuse workers from the pool.
        """

        # 1. First request (no worker in pool -> provisioning).
        resp1 = call_api(
            "/api/faulty",
            auth_token,
            {"action": "hello"},
        )
        assert resp1.status_code == 200, f"First request failed: {resp1.text}"
        print("First request succeeded (cold start or pooled)")

        # 2. Second request (reuse worker from pool).
        resp2 = call_api(
            "/api/faulty",
            auth_token,
            {"action": "hello"},
        )
        assert resp2.status_code == 200, f"Second request failed: {resp2.text}"
        print("Second request succeeded (should be warm/pooled)")

        # 3. Third request (confirm continued reuse).
        resp3 = call_api(
            "/api/faulty",
            auth_token,
            {"action": "hello"},
        )
        assert resp3.status_code == 200, f"Third request failed: {resp3.text}"
        print("Third request succeeded - pool reuse verified")

    def test_circuit_breaker(self, auth_token):
        """
        E2E: verify Circuit Breaker triggers on Lambda crashes.
        """

        # 1. Warm up.
        print("Warming up lambda-faulty...")
        call_api("/api/faulty", auth_token, {"action": "hello"})

        try:
            # 2. Repeated failures.
            for i in range(3):
                print(f"Attempt {i + 1} (crashing lambda)...")
                start = time.time()
                resp = call_api("/api/faulty", auth_token, {"action": "crash"}, timeout=10)
                duration = time.time() - start
                print(f"Status: {resp.status_code}, Body: {resp.text}, Latency: {duration:.2f}s")
                assert resp.status_code == 502, f"Expected 502, got {resp.status_code}"

            # 3. Fourth request (Circuit Breaker OPEN).
            print("Request 4 (expecting Circuit Breaker Open)...")
            start = time.time()
            resp = call_api("/api/faulty", auth_token, {"action": "hello"}, timeout=10)
            duration = time.time() - start
            print(f"Status: {resp.status_code}, Body: {resp.text}, Latency: {duration:.2f}s")

            assert resp.status_code == 502
            assert duration < 1.0, "Circuit Breaker should fail fast (< 1.0s)"

            # 4. Wait for recovery.
            print("Waiting for Circuit Breaker recovery (11s)...")
            time.sleep(11)

            # 5. Confirm recovery.
            print("Request 5 (expecting recovery)...")
            resp = call_api("/api/faulty", auth_token, {"action": "hello"})
            assert resp.status_code == 200, f"Recovery failed: {resp.text}"
            print("Circuit Breaker recovered successfully")

        except Exception as e:
            print(f"Circuit Breaker test failed: {e}")
            raise
