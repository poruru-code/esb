from fastapi.testclient import TestClient
from unittest.mock import MagicMock, patch
import sys
import os

# Add the project root to sys.path to allow imports from services
sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), "../../../")))

# We will implement this next
from services.manager.main import app, manager

client = TestClient(app)


def test_ensure_container_starts_new():
    """Verify that a new container is started if it doesn't exist."""
    # Build Docker Client Mock
    mock_client = MagicMock()

    # Inject mock client into the global manager instance, and mock socket to avoid wait
    # Also ensure manager uses the network name present in our mock data
    with (
        patch.object(manager, "client", mock_client),
        patch("services.manager.service.socket.create_connection"),
        patch.object(manager, "network", "dind-network"),
    ):
        # Existing containers list is empty
        mock_client.containers.list.return_value = []
        # run return value
        mock_container = MagicMock()
        mock_container.attrs = {
            "NetworkSettings": {"Networks": {"dind-network": {"IPAddress": "10.0.0.5"}}}
        }
        mock_client.containers.run.return_value = mock_container

        # Execute API
        response = client.post("/containers/ensure", json={"function_name": "lambda-hello"})

        # Verify
        assert response.status_code == 200
        assert response.json()["host"] == "10.0.0.5"

        # Verify strict call arguments
        mock_client.containers.run.assert_called_once()
        args, kwargs = mock_client.containers.run.call_args
    # Image might be positional or kwarg
    actual_image = kwargs.get("image")
    if not actual_image and args:
        actual_image = args[0]

    assert actual_image == "lambda-hello:latest"
    assert kwargs.get("privileged", False) is False
