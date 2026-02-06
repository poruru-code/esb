import pytest
import requests

from e2e.conftest import DEFAULT_REQUEST_TIMEOUT, GATEWAY_URL, VERIFY_SSL, VICTORIALOGS_URL


class TestConnectivity:
    """Verify basic health of gateway and log storage."""

    def test_gateway_health(self):
        """Gateway is up and responding."""
        response = requests.get(
            f"{GATEWAY_URL}/health",
            timeout=DEFAULT_REQUEST_TIMEOUT,
            verify=VERIFY_SSL,
        )
        assert response.status_code == 200, f"Gateway health check failed: {response.text}"

    def test_victorialogs_health(self):
        """VictoriaLogs is up and responding."""
        try:
            response = requests.get(
                f"{VICTORIALOGS_URL}/health",
                timeout=DEFAULT_REQUEST_TIMEOUT,
            )
            # VictoriaLogs returns 200 or empty for /health
            assert response.status_code in (200, 204), (
                f"VictoriaLogs health check failed: {response.status_code}"
            )
        except Exception as e:
            pytest.fail(f"VictoriaLogs connection failed ({VICTORIALOGS_URL}): {e}")

    # smokeの詳細検証は smoke/test_smoke.py に集約
