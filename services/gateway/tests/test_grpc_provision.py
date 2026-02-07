import os
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from services.gateway.models.function import FunctionEntity
from services.gateway.pb import agent_pb2

# Mock environment
os.environ["CONTAINERS_NETWORK"] = "test-net"
OWNER_ID = "test-owner"


@pytest.fixture
def mock_stub():
    with patch("services.gateway.pb.agent_pb2_grpc.AgentServiceStub") as mock:
        yield mock.return_value


@pytest.fixture
def mock_registry():
    registry = MagicMock()
    registry.get_function_config.return_value = FunctionEntity(
        name="test-func",
        environment={"KEY": "VALUE"},
    )
    return registry


@pytest.fixture
def grpc_client(mock_stub, mock_registry):
    from services.gateway.services.grpc_provision import GrpcProvisionClient

    return GrpcProvisionClient(mock_stub, mock_registry, owner_id=OWNER_ID)


@pytest.mark.asyncio
async def test_provision_success(grpc_client, mock_stub, mock_registry):
    """Test successful provision with env var injection"""
    # 1. Setup mock
    mock_registry.get_function_config.return_value = FunctionEntity(
        name="my-func",
        environment={"USER_VAR": "val"},
        memory_size=256,
        timeout=60,
        image="registry:5010/public.ecr.aws/example/repo:latest",
    )

    mock_response = agent_pb2.WorkerInfo(
        id="worker-1",
        name="worker-1",
        ip_address="127.0.0.1",
        port=8080,
    )
    mock_stub.EnsureContainer = AsyncMock(return_value=mock_response)

    # Mock config.VICTORIALOGS_URL (config object)
    with (
        patch("services.gateway.config.config") as mock_config,
        patch.object(grpc_client, "_wait_for_readiness", new_callable=AsyncMock),
    ):
        mock_config.VICTORIALOGS_URL = "http://victorialogs:8428"
        mock_config.GATEWAY_VICTORIALOGS_URL = "http://victorialogs:8428"
        mock_config.DYNAMODB_ENDPOINT = ""
        mock_config.S3_ENDPOINT = ""
        mock_config.DATA_PLANE_HOST = "10.88.0.1"

        # 2. Call
        workers = await grpc_client.provision("my-func")

        # 3. Verify
        assert len(workers) == 1
        assert workers[0].id == "worker-1"

        # Verify arguments passed to EnsureContainer
        args, _ = mock_stub.EnsureContainer.call_args
        request = args[0]

        assert request.function_name == "my-func"
        assert request.image == "registry:5010/public.ecr.aws/example/repo:latest"
        assert request.owner_id == OWNER_ID

        # Check Env injection
        env = request.env
        assert env["USER_VAR"] == "val"
        assert env["AWS_LAMBDA_FUNCTION_NAME"] == "my-func"
        assert env["AWS_LAMBDA_FUNCTION_MEMORY_SIZE"] == "256"
        assert env["AWS_LAMBDA_FUNCTION_TIMEOUT"] == "60"
        assert env["AWS_LAMBDA_FUNCTION_VERSION"] == "$LATEST"
        assert env["AWS_REGION"] == "ap-northeast-1"
        assert env["VICTORIALOGS_URL"] == "http://victorialogs:8428"


@pytest.mark.asyncio
async def test_provision_fallback_containerd(grpc_client, mock_stub, mock_registry):
    """Test fallback logic when endpoints are not set (Containerd mode)."""
    # 1. Setup mock
    mock_registry.get_function_config.return_value = FunctionEntity(
        name="my-func",
        environment={},
    )

    mock_response = agent_pb2.WorkerInfo(id="w1", name="w1", ip_address="1.2.3.4", port=8080)
    mock_stub.EnsureContainer = AsyncMock(return_value=mock_response)

    # Mock empty endpoints but valid DATA_PLANE_HOST
    with (
        patch("services.gateway.config.config") as mock_config,
        patch.object(grpc_client, "_wait_for_readiness", new_callable=AsyncMock),
    ):
        mock_config.S3_ENDPOINT = ""
        mock_config.DYNAMODB_ENDPOINT = ""
        mock_config.GATEWAY_VICTORIALOGS_URL = ""
        mock_config.VICTORIALOGS_URL = ""  # Host URL ignored in fallback
        mock_config.DATA_PLANE_HOST = "10.99.99.99"  # Custom Data Plane Host

        # 2. Call
        await grpc_client.provision("my-func")

        # 3. Verify
        args, _ = mock_stub.EnsureContainer.call_args
        env = args[0].env

        # derived from DATA_PLANE_HOST
        assert env["AWS_ENDPOINT_URL_S3"] == "http://10.99.99.99:9000"
        assert env["AWS_ENDPOINT_URL_DYNAMODB"] == "http://10.99.99.99:8000"
        assert env["AWS_ENDPOINT_URL_CLOUDWATCH_LOGS"] == "http://10.99.99.99:9428"
        args, _ = mock_stub.EnsureContainer.call_args
        assert args[0].owner_id == OWNER_ID


@pytest.mark.asyncio
async def test_grpc_delete_container(mock_stub, mock_registry):
    """Ensure delete_container calls the agent DestroyContainer."""
    from services.gateway.services.grpc_provision import GrpcProvisionClient

    mock_stub.DestroyContainer = AsyncMock()
    client = GrpcProvisionClient(mock_stub, mock_registry, owner_id=OWNER_ID)

    await client.delete_container("cnt-1")

    mock_stub.DestroyContainer.assert_called_once()
    args = mock_stub.DestroyContainer.call_args[0][0]
    assert args.container_id == "cnt-1"
    assert args.owner_id == OWNER_ID


@pytest.mark.asyncio
async def test_grpc_list_containers(mock_stub, mock_registry):
    """Ensure list_containers calls ListContainers and returns compatible results."""
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

    client = GrpcProvisionClient(mock_stub, mock_registry, owner_id=OWNER_ID)
    workers = await client.list_containers()

    assert len(workers) == 1
    assert workers[0].id == "id-1"
    assert workers[0].name == "lambda-func-1-unique"
    assert workers[0].last_used_at == 123456789
    req = mock_stub.ListContainers.call_args[0][0]
    assert req.owner_id == OWNER_ID
