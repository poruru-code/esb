import os

# Mock environment
os.environ["JWT_SECRET_KEY"] = "fake-secret-key-that-is-at-least-thirty-two-characters"
os.environ["X_API_KEY"] = "fake-api-key"
os.environ["AUTH_USER"] = "test-user"
os.environ["AUTH_PASS"] = "test-pass"
os.environ["CONTAINERS_NETWORK"] = "test-net"
os.environ["USE_GRPC_AGENT"] = "true"
os.environ["AGENT_GRPC_ADDRESS"] = "localhost:50051"

import pytest
from unittest.mock import AsyncMock, patch, MagicMock
from services.gateway.services.grpc_backend import GrpcBackend
from services.gateway.core.concurrency import ConcurrencyManager
from services.gateway.core.exceptions import ResourceExhaustedError
from services.gateway.pb import agent_pb2


@pytest.fixture
def mock_stub():
    with patch("services.gateway.pb.agent_pb2_grpc.AgentServiceStub") as mock:
        yield mock.return_value


@pytest.mark.asyncio
async def test_grpc_backend_applies_throttling(mock_stub):
    """
    Ensure GrpcBackend.acquire_worker uses ConcurrencyManager to reserve slots.
    """
    mock_stub.EnsureContainer = AsyncMock()
    mock_stub.EnsureContainer.return_value = agent_pb2.WorkerInfo(
        id="cat-id", name="cat-name", ip_address="10.0.0.5", port=8080
    )

    # Create a manager with limit 1.
    manager = ConcurrencyManager(default_limit=1, default_timeout=1)
    backend = GrpcBackend("localhost:50051", concurrency_manager=manager)
    backend.stub = mock_stub

    # Skip readiness check (tested separately).
    with patch.object(backend, "_wait_for_readiness", new_callable=AsyncMock) as mock_ready:
        # 1. First request (success).
        worker1 = await backend.acquire_worker("test-func")
        assert worker1.id == "cat-id"
        mock_ready.assert_called_once_with("test-func", "10.0.0.5", 8080)

        # 2. Second request (timeout due to limit 1).
        with pytest.raises(ResourceExhaustedError):
            await backend.acquire_worker("test-func")

        # 3. Call release_worker.
        await backend.release_worker("test-func", worker1)

        # 4. Acquire again (should succeed).
        worker2 = await backend.acquire_worker("test-func")
        assert worker2.id == "cat-id"


@pytest.mark.asyncio
async def test_grpc_backend_release_on_error(mock_stub):
    """
    Ensure slots are released automatically when EnsureContainer fails.
    """
    mock_stub.EnsureContainer = AsyncMock()
    mock_stub.EnsureContainer.side_effect = Exception("Agent Error")

    manager = ConcurrencyManager(default_limit=1, default_timeout=1)
    backend = GrpcBackend("localhost:50051", concurrency_manager=manager)
    backend.stub = mock_stub

    # 1. Request fails.
    with pytest.raises(Exception) as exc:
        await backend.acquire_worker("test-func")
    assert "Agent Error" in str(exc.value)

    # 2. If slot is released, the next request should reach EnsureContainer without waiting.
    # Switch EnsureContainer to success for verification.
    mock_stub.EnsureContainer.side_effect = None
    mock_stub.EnsureContainer.return_value = agent_pb2.WorkerInfo(id="ok")

    with patch.object(backend, "_wait_for_readiness", new_callable=AsyncMock):
        worker = await backend.acquire_worker("test-func")
        assert worker.id == "ok"


@pytest.mark.asyncio
async def test_wait_for_readiness_success():
    """Readiness check succeeds."""
    backend = GrpcBackend("localhost:50051")

    with patch("asyncio.open_connection", new_callable=AsyncMock) as mock_conn:
        mock_reader = MagicMock()
        mock_writer = MagicMock()
        mock_writer.wait_closed = AsyncMock()  # AsyncMock to be awaitable
        mock_conn.return_value = (mock_reader, mock_writer)

        await backend._wait_for_readiness("test-func", "1.2.3.4", 8080, timeout=1.0)
        mock_conn.assert_called_once()
        mock_writer.close.assert_called_once()
        mock_writer.wait_closed.assert_called_once()


@pytest.mark.asyncio
async def test_wait_for_readiness_timeout():
    """Readiness check times out."""
    from services.gateway.core.exceptions import ContainerStartError

    backend = GrpcBackend("localhost:50051")

    with patch("asyncio.open_connection", side_effect=ConnectionRefusedError):
        with pytest.raises(ContainerStartError) as exc:
            await backend._wait_for_readiness("test-func", "1.2.3.4", 8080, timeout=0.2)
        assert "test-func" in str(exc.value)
