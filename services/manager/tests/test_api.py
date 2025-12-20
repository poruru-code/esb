from fastapi.testclient import TestClient
from unittest.mock import MagicMock, patch
import sys
import os
import docker

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
        assert response.json()["host"] == "lambda-hello"

        # Verify strict call arguments
        mock_client.containers.run.assert_called_once()
        args, kwargs = mock_client.containers.run.call_args
    # Image might be positional or kwarg
    actual_image = kwargs.get("image")
    if not actual_image and args:
        actual_image = args[0]

    assert actual_image == "lambda-hello:latest"
    assert kwargs.get("privileged", False) is False


def test_ensure_container_image_not_found():
    """Verify that 404 is returned if the image is not found."""
    import docker.errors

    mock_client = MagicMock()
    # Mocking containers.get to raise NotFound, and run to raise ImageNotFound
    mock_client.containers.get.side_effect = docker.errors.NotFound("Not found")
    mock_client.containers.run.side_effect = docker.errors.ImageNotFound(
        "No such image", response=MagicMock()
    )

    with (
        patch.object(manager, "client", mock_client),
        patch.object(manager, "network", "dind-network"),
    ):
        response = client.post("/containers/ensure", json={"function_name": "unknown-func"})

        assert response.status_code == 404
        assert "not found" in response.json()["detail"].lower()


def test_ensure_container_concurrency():
    """Verify that multiple concurrent requests result in only one creation."""
    from concurrent.futures import ThreadPoolExecutor

    mock_client = MagicMock()
    mock_client.containers.list.return_value = []

    # Slow start simulation
    def slow_run(*args, **kwargs):
        import time

        time.sleep(0.5)
        mock_container = MagicMock()
        mock_container.attrs = {
            "NetworkSettings": {"Networks": {"dind-network": {"IPAddress": "10.0.0.5"}}}
        }
        return mock_container

    mock_client.containers.run.side_effect = slow_run
    # Mocking get to return NotFound initially, then success for subsequent calls
    mock_running_container = MagicMock()
    mock_running_container.status = "running"
    mock_running_container.attrs = {
        "NetworkSettings": {"Networks": {"dind-network": {"IPAddress": "10.0.0.5"}}}
    }

    mock_client.containers.get.side_effect = [
        docker.errors.NotFound("Not found"),
        mock_running_container,
        mock_running_container,
        mock_running_container,
        mock_running_container,
        mock_running_container,
    ]

    with (
        patch.object(manager, "client", mock_client),
        patch("services.manager.service.socket.create_connection"),
        patch.object(manager, "network", "dind-network"),
    ):
        with ThreadPoolExecutor(max_workers=5) as executor:
            # Send 5 requests simultaneously
            futures = [
                executor.submit(
                    client.post, "/containers/ensure", json={"function_name": "busy-func"}
                )
                for _ in range(5)
            ]
            responses = [f.result() for f in futures]

        # All should succeed
        for resp in responses:
            assert resp.status_code == 200
            assert resp.json()["host"] == "busy-func"

        # Run should be called ONLY ONCE despite 5 requests
        assert mock_client.containers.run.call_count == 1


def test_ensure_container_ip_fallback():
    """Verify that it falls back to container name if IP address is missing."""
    mock_client = MagicMock()
    mock_container = MagicMock()
    mock_container.status = "running"
    # Network or IPAddress is missing
    mock_container.attrs = {"NetworkSettings": {"Networks": {}}}
    mock_client.containers.get.return_value = mock_container

    with (
        patch.object(manager, "client", mock_client),
        patch("services.manager.service.socket.create_connection"),
        patch.object(manager, "network", "dind-network"),
    ):
        response = client.post("/containers/ensure", json={"function_name": "fallback-func"})

        assert response.status_code == 200
        # Host should be container name if IP not found
        assert response.json()["host"] == "fallback-func"


def test_ensure_container_readiness_with_ip():
    """Verify that readiness check is performed using the IP address."""
    mock_client = MagicMock()
    mock_container = MagicMock()
    mock_container.status = "running"
    mock_container.attrs = {
        "NetworkSettings": {"Networks": {"dind-network": {"IPAddress": "10.0.0.99"}}}
    }
    mock_client.containers.get.return_value = mock_container

    with (
        patch.object(manager, "client", mock_client),
        patch("services.manager.service.socket.create_connection"),
        patch.object(manager, "network", "dind-network"),
    ):
        response = client.post("/containers/ensure", json={"function_name": "ip-check-func"})

        assert response.status_code == 200
        assert response.json()["host"] == "ip-check-func"

        # Verify create_connection was called with IP ADDRESS, not function name
        # args[0] is (address, port)


def test_network_name_from_env():
    """Verify that network name is read from CONTAINERS_NETWORK env var."""
    from services.manager.service import ContainerManager

    with patch.dict(os.environ, {"CONTAINERS_NETWORK": "custom-network"}):
        manager_inst = ContainerManager()
        assert manager_inst.network == "custom-network"


def test_ensure_container_returns_hostname_but_checks_readiness_with_ip():
    """
    Verify that ensure_container_running returns the container name (hostname),
    but internal readiness check uses the IP address.
    """
    from services.manager.service import ContainerManager

    mock_client = MagicMock()
    manager_inst = ContainerManager(network="test-net")
    manager_inst.client = mock_client

    mock_container = MagicMock()
    mock_container.status = "running"
    mock_container.name = "my-lambda-func"
    mock_container.attrs = {
        "NetworkSettings": {"Networks": {"test-net": {"IPAddress": "172.18.0.5"}}}
    }
    mock_client.containers.get.return_value = mock_container

    with patch("services.manager.service.socket.create_connection") as mock_conn:
        result_host = manager_inst.ensure_container_running("my-lambda-func")

        # Result should be the container name (hostname)
        assert result_host == "my-lambda-func"

        # Internal readiness check should use IP address
        args, _ = mock_conn.call_args
        assert args[0][0] == "172.18.0.5"
