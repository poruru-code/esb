import pytest
import requests

from e2e.conftest import AGENT_METRICS_URL, DEFAULT_REQUEST_TIMEOUT


class TestPrometheusMetrics:
    """Verify Prometheus metrics endpoint."""

    def test_metrics_endpoint_accessible(self):
        """
        E2E: Verify /metrics endpoint is reachable and returns Prometheus format.
        """
        url = f"{AGENT_METRICS_URL}/metrics"
        print(f"Checking metrics at: {url}")

        try:
            response = requests.get(url, timeout=DEFAULT_REQUEST_TIMEOUT)
            assert response.status_code == 200, f"Metrics endpoint returned {response.status_code}"
            assert response.headers.get("Content-Type", "").startswith("text/plain"), (
                "Invalid Content-Type"
            )

            content = response.text

            # 1. Check for standard Go metrics
            assert "go_goroutines" in content, "Missing go_goroutines metric"
            assert "go_memstats_alloc_bytes" in content, "Missing go_memstats_alloc_bytes metric"

            # 2. Check for gRPC server metrics (go-grpc-prometheus)
            # Note: valid only if at least one gRPC call has been made.
            # In E2E suite, gateway_health fixture or other tests usually warm up.
            # But strictly speaking, we might check for 0 values or just existence if initialized.
            # go-grpc-prometheus registers metrics on init.

            assert "grpc_server_handled_total" in content, (
                "Missing grpc_server_handled_total metric"
            )
            assert "grpc_server_handling_seconds_bucket" in content, (
                "Missing grpc_server_handling_seconds_bucket metric"
            )

            print(
                f"Metrics verification successful. Found {len(content.splitlines())} lines of metrics."
            )

        except requests.exceptions.ConnectionError:
            pytest.fail(
                f"Could not connect to Agent metrics endpoint at {url}. Ensure port {AGENT_METRICS_URL.split(':')[-1]} is exposed."
            )
