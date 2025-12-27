import pytest
import os
from unittest.mock import AsyncMock, MagicMock, patch
from services.gateway.pb import agent_pb2

# Mock environment
os.environ["CONTAINERS_NETWORK"] = "test-net"


@pytest.fixture
def mock_stub():
    with patch("services.gateway.pb.agent_pb2_grpc.AgentServiceStub") as mock:
        yield mock.return_value


@pytest.fixture
def mock_registry():
    registry = MagicMock()
    registry.get_function_config.return_value = {
        "image": "test-image:latest",
        "environment": {"KEY": "VALUE"},
    }
    return registry


@pytest.mark.asyncio
async def test_grpc_provision_success(mock_stub, mock_registry):
    """provision(function_name) が成功し WorkerInfo を返すことを確認"""
    # Import inside to avoid dependency issues during RED step
    from services.gateway.services.grpc_provision import GrpcProvisionClient

    mock_stub.EnsureContainer = AsyncMock()
    mock_stub.EnsureContainer.return_value = agent_pb2.WorkerInfo(
        id="cnt-1", name="lambda-test-1", ip_address="10.0.0.10", port=8080
    )

    client = GrpcProvisionClient(mock_stub, mock_registry)
    with patch.object(client, "_wait_for_readiness", new_callable=AsyncMock) as mock_ready:
        workers = await client.provision("test-func")
        mock_ready.assert_called_once_with("test-func", "10.0.0.10", 8080)

    assert len(workers) == 1
    assert workers[0].id == "cnt-1"
    assert workers[0].ip_address == "10.0.0.10"

    # Verify Agent call
    mock_stub.EnsureContainer.assert_called_once()
    args = mock_stub.EnsureContainer.call_args[0][0]
    assert args.function_name == "test-func"
    assert args.image == "test-image:latest"
    assert args.env["KEY"] == "VALUE"


@pytest.mark.asyncio
async def test_grpc_delete_container(mock_stub, mock_registry):
    """delete_container がエージェントの DestroyContainer を呼ぶことを確認"""
    from services.gateway.services.grpc_provision import GrpcProvisionClient

    mock_stub.DestroyContainer = AsyncMock()
    client = GrpcProvisionClient(mock_stub, mock_registry)

    await client.delete_container("cnt-1")

    mock_stub.DestroyContainer.assert_called_once()
    args = mock_stub.DestroyContainer.call_args[0][0]
    assert args.container_id == "cnt-1"


@pytest.mark.asyncio
async def test_grpc_list_containers(mock_stub, mock_registry):
    """list_containers がエージェントの ListContainers を呼び、上位互換の結果を返すことを確認"""
    from services.gateway.services.grpc_provision import GrpcProvisionClient

    mock_stub.ListContainers = AsyncMock()
    mock_stub.ListContainers.return_value = agent_pb2.ListContainersResponse(
        containers=[
            agent_pb2.ContainerState(
                container_id="id-1",
                function_name="func-1",
                status="running",
                last_used_at=123456789,
                container_name="lambda-func-1-unique",
            )
        ]
    )

    client = GrpcProvisionClient(mock_stub, mock_registry)
    workers = await client.list_containers()

    assert len(workers) == 1
    assert workers[0].id == "id-1"
    assert workers[0].name == "lambda-func-1-unique"
    assert workers[0].last_used_at == 123456789
