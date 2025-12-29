import os

# Mock environment for Pydantic config validation
os.environ["JWT_SECRET_KEY"] = "fake-secret-key-that-is-at-least-thirty-two-characters"
os.environ["X_API_KEY"] = "fake-api-key"
os.environ["AUTH_USER"] = "test-user"
os.environ["AUTH_PASS"] = "test-pass"
os.environ["CONTAINERS_NETWORK"] = "test-net"
os.environ["USE_GRPC_AGENT"] = "true"
os.environ["AGENT_GRPC_ADDRESS"] = "localhost:50051"

import pytest
import grpc
from unittest.mock import AsyncMock, patch, MagicMock
from services.gateway.services.grpc_backend import GrpcBackend
from services.gateway.core.exceptions import (
    OrchestratorUnreachableError,
    OrchestratorTimeoutError,
    ContainerStartError,
)
from services.gateway.pb import agent_pb2


class MockRpcError(grpc.RpcError, grpc.Call):
    def __init__(self, code, details=""):
        self._code = code
        self._details = details

    def code(self):
        return self._code

    def details(self):
        return self._details


@pytest.fixture
def mock_stub():
    with patch("services.gateway.pb.agent_pb2_grpc.AgentServiceStub") as mock:
        yield mock.return_value


@pytest.fixture
async def backend(mock_stub):
    backend = GrpcBackend("localhost:50051")
    # Stub is created in __init__, so we patch it
    backend.stub = mock_stub
    yield backend
    await backend.close()


@pytest.mark.asyncio
async def test_acquire_worker_success(backend, mock_stub):
    # Setup mock response
    mock_stub.EnsureContainer = AsyncMock()
    mock_stub.EnsureContainer.return_value = agent_pb2.WorkerInfo(
        id="cat-id", name="cat-name", ip_address="10.0.0.5", port=8080
    )

    with patch.object(backend, "_wait_for_readiness", new_callable=AsyncMock):
        worker = await backend.acquire_worker("test-func")

    assert worker.id == "cat-id"
    assert worker.name == "cat-name"
    assert worker.ip_address == "10.0.0.5"

    # Verify request
    mock_stub.EnsureContainer.assert_called_once()
    req = mock_stub.EnsureContainer.call_args[0][0]
    assert req.function_name == "test-func"


@pytest.mark.asyncio
async def test_evict_worker_success(backend, mock_stub):
    mock_stub.DestroyContainer = AsyncMock()
    mock_stub.DestroyContainer.return_value = agent_pb2.DestroyContainerResponse(success=True)

    worker = MagicMock()
    worker.id = "target-id"

    await backend.evict_worker("test-func", worker)

    mock_stub.DestroyContainer.assert_called_once()
    req = mock_stub.DestroyContainer.call_args[0][0]
    assert req.container_id == "target-id"


@pytest.mark.asyncio
async def test_handle_unreachable(backend, mock_stub):
    mock_stub.EnsureContainer = AsyncMock()
    # Mock gRPC error
    mock_stub.EnsureContainer.side_effect = MockRpcError(grpc.StatusCode.UNAVAILABLE)

    with pytest.raises(OrchestratorUnreachableError):
        await backend.acquire_worker("test-func")


@pytest.mark.asyncio
async def test_handle_timeout(backend, mock_stub):
    mock_stub.EnsureContainer = AsyncMock()
    mock_stub.EnsureContainer.side_effect = MockRpcError(grpc.StatusCode.DEADLINE_EXCEEDED)

    with pytest.raises(OrchestratorTimeoutError):
        await backend.acquire_worker("test-func")


@pytest.mark.asyncio
async def test_handle_resource_exhausted(backend, mock_stub):
    mock_stub.EnsureContainer = AsyncMock()
    mock_stub.EnsureContainer.side_effect = MockRpcError(grpc.StatusCode.RESOURCE_EXHAUSTED)

    with pytest.raises(ContainerStartError):
        await backend.acquire_worker("test-func")
