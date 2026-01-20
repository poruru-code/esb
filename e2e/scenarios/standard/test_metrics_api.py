"""
Gateway metrics API tests.

- Metrics are exposed via Gateway.
- Resource limits are reflected in reported metrics.
"""

import time

import requests

from e2e.conftest import (
    DEFAULT_REQUEST_TIMEOUT,
    GATEWAY_URL,
    VERIFY_SSL,
    call_api,
)


class TestMetricsAPI:
    """Verify Gateway metrics endpoint behavior."""

    def test_metrics_api(self, gateway_health, auth_token):
        """E2E: metrics endpoint returns container metrics."""
        response = call_api("/api/echo", auth_token, {"message": "metrics"})
        assert response.status_code == 200

        headers = {"Authorization": f"Bearer {auth_token}"}
        expected_memory_max = 128 * 1024 * 1024
        expected_pool_names = {"echo", "lambda-echo"}

        pool_resp = requests.get(
            f"{GATEWAY_URL}/metrics/pools",
            headers=headers,
            verify=VERIFY_SSL,
            timeout=DEFAULT_REQUEST_TIMEOUT,
        )
        assert pool_resp.status_code == 200, (
            f"Pool metrics API failed with {pool_resp.status_code}: {pool_resp.text}"
        )
        pool_data = pool_resp.json()
        assert "pools" in pool_data
        assert "collected_at" in pool_data

        pool_entry = next(
            (
                item
                for item in pool_data.get("pools", [])
                if item.get("function_name") in expected_pool_names
            ),
            None,
        )
        assert pool_entry is not None
        for field in (
            "function_name",
            "total_workers",
            "idle",
            "busy",
            "provisioning",
            "max_capacity",
            "min_capacity",
            "acquire_timeout",
        ):
            assert field in pool_entry

        metrics_entry = None
        metrics_resp: requests.Response | None = None
        for _ in range(10):
            metrics_resp = requests.get(
                f"{GATEWAY_URL}/metrics/containers",
                headers=headers,
                verify=VERIFY_SSL,
                timeout=DEFAULT_REQUEST_TIMEOUT,
            )

            # Allow 200 OK or 501 Not Implemented (for Docker runtime)
            if metrics_resp.status_code == 200:
                data = metrics_resp.json()
                metrics_entry = next(
                    (
                        item
                        for item in data.get("containers", [])
                        if item.get("function_name") == "lambda-echo"
                    ),
                    None,
                )
                if metrics_entry and metrics_entry.get("memory_max", 0) > 0:
                    break
            elif metrics_resp.status_code == 501:
                # Docker runtime doesn't support metrics yet.
                # Validate error message to ensure it's the expected 501.
                # Note: Gateway exception handler returns {"message": "..."}
                error_body = metrics_resp.json()
                error_detail = error_body.get("message") or error_body.get("detail") or ""

                assert (
                    "metrics are not implemented" in error_detail.lower()
                    or "docker" in error_detail.lower()
                ), f"Unexpected 501 error detail: {error_detail}"

                # If valid 501, we consider the test passed for this runtime.
                # Return immediately as we can't verify metrics values.
                return
            else:
                # Other errors (4xx, 503, etc.) fail the assertions below
                pass

            time.sleep(1)

        # If we got 200 OK, verify metrics content
        assert metrics_resp is not None
        assert metrics_resp.status_code == 200, (
            f"Metrics API failed with {metrics_resp.status_code}: {metrics_resp.text}"
        )
        assert metrics_entry is not None
        assert metrics_entry["state"] in {"RUNNING", "PAUSED"}
        assert metrics_entry["memory_max"] == expected_memory_max
        assert metrics_entry["memory_current"] >= 0
        assert metrics_entry["cpu_usage_ns"] >= 0
