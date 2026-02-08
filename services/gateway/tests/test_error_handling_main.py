from unittest.mock import patch

import pytest

from services.gateway.core.exceptions import ResourceExhaustedError


@pytest.mark.asyncio
async def test_resource_exhausted_error_returns_429(async_client):
    """
    Ensure HTTP 429 is returned when ResourceExhaustedError occurs.
    """
    # Force an exception on an existing endpoint (e.g., /health).
    # Before implementation (RED), this would be 500 via global_exception_handler.
    with patch(
        "services.gateway.main.health_check", side_effect=ResourceExhaustedError("Queue full")
    ):
        response = await async_client.get("/health")

        # Expected: 429
        # Before implementation: 500 (or unhandled exception)
        assert response.status_code == 429
        assert response.json()["message"] == "Too Many Requests"
