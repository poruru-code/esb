from fastapi.testclient import TestClient
from services.gateway.main import app
from services.gateway.core.exceptions import ResourceExhaustedError
from unittest.mock import patch


def test_resource_exhausted_error_returns_429():
    """
    Ensure HTTP 429 is returned when ResourceExhaustedError occurs.
    """
    # Force an exception on an existing endpoint (e.g., /health).
    # Before implementation (RED), this would be 500 via global_exception_handler.
    with patch(
        "services.gateway.main.health_check", side_effect=ResourceExhaustedError("Queue full")
    ):
        client = TestClient(app)
        response = client.get("/health")

        # Expected: 429
        # Before implementation: 500 (or unhandled exception)
        assert response.status_code == 429
        assert response.json()["message"] == "Too Many Requests"
