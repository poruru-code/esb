"""
Gateway basic functionality tests.

- Health check
- Authentication flow
- Basic routing (401, 404)
"""

import requests
from tests.conftest import (
    GATEWAY_URL,
    VERIFY_SSL,
    call_api,
)


class TestGatewayBasics:
    """Verify Gateway basic functionality."""

    def test_health(self, gateway_health):
        """E2E: health check."""
        response = requests.get(f"{GATEWAY_URL}/health", verify=VERIFY_SSL)
        assert response.status_code == 200
        assert response.json()["status"] == "healthy"

    def test_auth(self, auth_token):
        """E2E: authentication flow."""
        assert auth_token is not None
        assert len(auth_token) > 0

    def test_routing_401(self, gateway_health):
        """E2E: no auth -> 401."""
        response = call_api("/api/echo", payload={"message": "test"})
        if response.status_code != 401:
            print(f"Debug 401 Error: {response.status_code} - {response.text}")
        assert response.status_code == 401

    def test_routing_404(self, auth_token):
        """E2E: non-existent route -> 404."""
        response = call_api("/api/nonexistent", auth_token, {"action": "test"})
        assert response.status_code == 404
